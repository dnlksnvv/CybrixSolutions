"""LiveKit ``stt.STT`` backed by the inference-gateway WS endpoint ``/v1/stt/ws``.

Sends 100 ms PCM s16le mono frames as ``audio.chunk`` (base64) and translates
``transcript.partial`` / ``transcript.final`` into LiveKit ``PREFLIGHT_TRANSCRIPT``
/ ``FINAL_TRANSCRIPT`` (partial uses preflight so preemptive generation can run
on streaming hypotheses). VAD / turn detection lives in the gateway (per-model).
"""

from __future__ import annotations

import asyncio
import base64
import contextlib
import logging
import os
from collections.abc import AsyncIterator

import aiohttp

from livekit import rtc
from livekit.agents import stt, utils
from livekit.agents._exceptions import APIConnectionError, APIStatusError
from livekit.agents.types import DEFAULT_API_CONNECT_OPTIONS, NOT_GIVEN, APIConnectOptions, NotGivenOr
from trace_emitter import emit

from .client import decode_text_event, open_stt
from .protocol import (
    EventType,
    STTSessionStart,
    audio_chunk_frame,
    input_finish_frame,
)

logger = logging.getLogger("inference-gateway-stt")


def _clip_transcript(text: str, max_len: int = 160) -> str:
    t = " ".join(text.split())
    if len(t) <= max_len:
        return t
    return t[: max_len - 1] + "…"


class GatewaySTT(stt.STT):
    def __init__(
        self,
        *,
        ws_url: str,
        model: str,
        language: str,
        sample_rate: int = 16000,
    ) -> None:
        super().__init__(
            capabilities=stt.STTCapabilities(streaming=True, interim_results=True),
        )
        if not language:
            raise ValueError("language is required")
        if not model:
            raise ValueError("model is required")
        self._ws_url = ws_url
        self._model = model
        self._language = language
        self._sample_rate = sample_rate
        self._session: aiohttp.ClientSession | None = None

    @property
    def model(self) -> str:
        return self._model

    @property
    def provider(self) -> str:
        return "inference-gateway"

    def _http(self) -> aiohttp.ClientSession:
        if self._session is None or self._session.closed:
            self._session = aiohttp.ClientSession()
        return self._session

    async def warmup(self) -> None:
        """Eagerly open STT WS once at session start."""
        start = STTSessionStart(
            request_id=utils.shortuuid("stt_warm_"),
            model=self._model,
            language=self._language,
            sample_rate=self._sample_rate,
            audio_format="pcm_s16le",
        )
        try:
            ws = await open_stt(self._http(), ws_url=self._ws_url, start=start)
        except Exception:  # noqa: BLE001
            return
        with contextlib.suppress(Exception):
            await ws.send_json(input_finish_frame())
        with contextlib.suppress(Exception):
            await ws.close()

    async def _recognize_impl(
        self,
        buffer: utils.AudioBuffer,
        *,
        language: NotGivenOr[str] = NOT_GIVEN,
        conn_options: APIConnectOptions,
    ) -> stt.SpeechEvent:
        raise NotImplementedError("inference-gateway STT supports streaming only")

    def stream(
        self,
        *,
        language: NotGivenOr[str] = NOT_GIVEN,
        conn_options: APIConnectOptions = DEFAULT_API_CONNECT_OPTIONS,
    ) -> _GatewaySTTStream:
        lang = self._language
        if language is not NOT_GIVEN and language:
            lang = str(language).strip() or self._language
        return _GatewaySTTStream(
            stt=self,
            language=lang,
            sample_rate=self._sample_rate,
            ws_url=self._ws_url,
            conn_options=conn_options,
        )

    async def aclose(self) -> None:
        if self._session is not None and not self._session.closed:
            await self._session.close()


class _GatewaySTTStream(stt.SpeechStream):
    def __init__(
        self,
        *,
        stt: GatewaySTT,
        language: str,
        sample_rate: int,
        ws_url: str,
        conn_options: APIConnectOptions,
    ) -> None:
        super().__init__(stt=stt, conn_options=conn_options, sample_rate=sample_rate)
        self._language = language
        self._ws_url = ws_url
        self._request_id = utils.shortuuid("stt_")
        self._speaking = False

    async def _run(self) -> None:
        parent: GatewaySTT = self._stt  # type: ignore[assignment]
        start = STTSessionStart(
            request_id=self._request_id,
            model=parent._model,
            language=self._language,
            sample_rate=parent._sample_rate,
            audio_format="pcm_s16le",
        )
        try:
            ws = await open_stt(parent._http(), ws_url=parent._ws_url, start=start)
        except aiohttp.ClientError as e:
            raise APIConnectionError(message=f"inference-gateway stt ws: {e}") from e

        try:
            await asyncio.gather(self._sender(ws), self._receiver(ws))
        except APIStatusError:
            raise
        except aiohttp.ClientError as e:
            raise APIStatusError(message=f"inference-gateway stt: {e}", status_code=-1, retryable=True) from e
        finally:
            with contextlib.suppress(Exception):
                await ws.close()

    async def _sender(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        parent: GatewaySTT = self._stt  # type: ignore[assignment]
        samples_100ms = parent._sample_rate // 10
        bstream = utils.audio.AudioByteStream(
            sample_rate=parent._sample_rate,
            num_channels=1,
            samples_per_channel=samples_100ms,
        )
        try:
            async for data in self._input_ch:
                frames: list[rtc.AudioFrame] = []
                if isinstance(data, rtc.AudioFrame):
                    frames.extend(bstream.write(data.data.tobytes()))
                elif isinstance(data, self._FlushSentinel):
                    frames.extend(bstream.flush())
                for frame in frames:
                    pcm_b64 = base64.b64encode(frame.data.tobytes()).decode("ascii")
                    await ws.send_json(audio_chunk_frame(pcm_b64))
        finally:
            with contextlib.suppress(Exception):
                await ws.send_json(input_finish_frame())

    async def _receiver(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        parent: GatewaySTT = self._stt  # type: ignore[assignment]
        async for msg in self._iter_ws(ws):
            ev = decode_text_event(msg)
            if ev is None:
                continue
            if ev.type == EventType.ERROR:
                raise APIStatusError(
                    message=f"inference-gateway stt: {ev.code or 'ERROR'}: {ev.message}",
                    status_code=-1,
                    retryable=ev.retryable,
                )
            if ev.type == EventType.TRANSCRIPT_PARTIAL and ev.text:
                if not self._speaking:
                    self._event_ch.send_nowait(
                        stt.SpeechEvent(type=stt.SpeechEventType.START_OF_SPEECH)
                    )
                    self._speaking = True
                # PREFLIGHT (not INTERIM): AudioRecognition only calls on_preemptive_generation
                # for PREFLIGHT_TRANSCRIPT + certain FINAL paths. Salute partials must use
                # PREFLIGHT so ALIYUN_PREEMPTIVE_GENERATION can start LLM on streaming text.
                self._event_ch.send_nowait(
                    stt.SpeechEvent(
                        type=stt.SpeechEventType.PREFLIGHT_TRANSCRIPT,
                        request_id=self._request_id,
                        alternatives=[
                            stt.SpeechData(
                                language=self._language,
                                text=ev.text,
                                start_time=0.0,
                                end_time=0.0,
                            )
                        ],
                    )
                )
                # PREFLIGHT drives LiveKit preemptive_generation (see AudioRecognition).
                lvl = logging.DEBUG if os.environ.get("CYBRIX_STT_PREFLIGHT_LOG") == "debug" else logging.INFO
                logger.log(
                    lvl,
                    "stt preflight → LiveKit PREFLIGHT_TRANSCRIPT (preemptive hook) request_id=%s text=%r",
                    self._request_id,
                    _clip_transcript(ev.text),
                )
                emit("stt.interim", model=parent._model, language=self._language, text=ev.text)
                continue
            if ev.type == EventType.TRANSCRIPT_FINAL and ev.text:
                self._event_ch.send_nowait(
                    stt.SpeechEvent(
                        type=stt.SpeechEventType.FINAL_TRANSCRIPT,
                        request_id=self._request_id,
                        alternatives=[
                            stt.SpeechData(
                                language=self._language,
                                text=ev.text,
                                start_time=0.0,
                                end_time=0.0,
                            )
                        ],
                    )
                )
                logger.info(
                    "stt final + END_OF_SPEECH request_id=%s text=%r",
                    self._request_id,
                    _clip_transcript(ev.text),
                )
                emit("stt.final", model=parent._model, language=self._language, text=ev.text)
                self._event_ch.send_nowait(
                    stt.SpeechEvent(
                        type=stt.SpeechEventType.END_OF_SPEECH, request_id=self._request_id
                    )
                )
                self._speaking = False
                continue

    async def _iter_ws(
        self, ws: aiohttp.ClientWebSocketResponse
    ) -> AsyncIterator[aiohttp.WSMessage]:
        async for msg in ws:
            if msg.type in (
                aiohttp.WSMsgType.CLOSE,
                aiohttp.WSMsgType.CLOSED,
                aiohttp.WSMsgType.CLOSING,
            ):
                return
            if msg.type == aiohttp.WSMsgType.ERROR:
                raise APIStatusError(
                    message=f"inference-gateway stt ws error: {ws.exception()}",
                    status_code=-1,
                    retryable=True,
                )
            yield msg
