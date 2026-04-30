"""LiveKit ``tts.TTS`` backed by the inference-gateway WS endpoint ``/v1/tts/ws``.

Two modes, controlled by ``passthrough``:

- **commit** (``passthrough=False``)
    LLM tokens go through ``StreamingSentencizer`` first; each completed
    sentence is sent as ``input.text`` + ``input.commit``. Sentences are
    **gated**: the next commit is sent only after ``max(0, est_ms - budget_ms) + extra_ms``
    (seconds) from the **first audio chunk** of the previous sentence (``est_ms``
    from :mod:`speech_duration_estimate`, ``budget_ms`` from ``TTS_GATE_GENERATION_BUDGET_MS``,
    ``extra_ms`` from ``TTS_GATE_EXTRA_DELAY_MS``). ``audio.end`` does **not** open the gate early.

- **server_commit** (``passthrough=True``)
    Every LLM token is forwarded immediately. The gateway / upstream decides
    sentence boundaries (DashScope ``mode=server_commit``). Lowest TTFA.

The chosen mode is reflected in ``TTSSessionStart.mode`` so the gateway
configures upstream accordingly.
"""

from __future__ import annotations

import asyncio
import base64
import contextlib
import logging
import time
from collections import deque
from typing import Any

import aiohttp

from livekit.agents import tts, utils
from livekit.agents._exceptions import APIConnectionError, APIStatusError
from livekit.agents.types import DEFAULT_API_CONNECT_OPTIONS, APIConnectOptions
from speech_duration_estimate import estimate_sentence_duration_ms
from streaming_sentencizer import StreamingSentencizer
from trace_emitter import emit

from .client import decode_text_event, open_tts
from .protocol import (
    EventType,
    TTSSessionStart,
    input_commit_frame,
    input_finish_frame,
    input_text_frame,
)

logger = logging.getLogger("inference-gateway-tts")

# Feeder blocks on ``playback_gate`` until the reader arms the gate from the first
# audio chunk of the previous commit. If upstream never sends audio, unlock so the
# session does not wedge forever (and ``_turn_lock`` stays held).
_GATE_WAIT_SAFETY_S = 120.0
# If the gateway emits one ``audio.end`` for several client commits, the reader would
# otherwise wait forever for extra ends. After this idle gap with feeder finished,
# we close the reader and release the turn lock.
_COALESCED_AUDIO_END_IDLE_S = 30.0


class GatewayTTS(tts.TTS):
    """TTS for any model registered in inference-gateway (Qwen / Salute / …)."""

    def __init__(
        self,
        *,
        ws_url: str,
        model: str,
        voice: str = "",
        language_type: str = "",
        sample_rate: int = 24000,
        mode: str = "",
        passthrough: bool = False,
        gate_generation_budget_ms: int = 600,
        gate_extra_delay_ms: int = 0,
    ) -> None:
        super().__init__(
            capabilities=tts.TTSCapabilities(streaming=True),
            sample_rate=sample_rate,
            num_channels=1,
        )
        self._ws_url = ws_url
        self._model = model
        self._voice = voice
        self._language_type = language_type
        # Keep worker side model-agnostic: mode comes only from settings/env.
        # Empty value means "gateway decides via its own per-model config".
        self._mode = mode
        self._passthrough = passthrough
        self._gate_generation_budget_ms = max(0, gate_generation_budget_ms)
        self._gate_extra_delay_ms = max(0, gate_extra_delay_ms)
        self._session: aiohttp.ClientSession | None = None
        self._ws: aiohttp.ClientWebSocketResponse | None = None
        self._ws_lock = asyncio.Lock()
        self._turn_lock = asyncio.Lock()
        self._last_chunk_seq: int | None = None
        self._session_request_id = utils.shortuuid("tts_sess_")

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
        """Eagerly establish the long-lived TTS WS before first reply."""
        try:
            await self.acquire_ws()
        except Exception:  # noqa: BLE001
            # Warmup is best-effort; normal lazy connect path still works.
            pass

    def synthesize(
        self, text: str, *, conn_options: APIConnectOptions = DEFAULT_API_CONNECT_OPTIONS
    ) -> tts.ChunkedStream:
        return self._synthesize_with_stream(text, conn_options=conn_options)

    def stream(self, *, conn_options: APIConnectOptions = DEFAULT_API_CONNECT_OPTIONS) -> tts.SynthesizeStream:
        return _GatewayTTSStream(tts=self, conn_options=conn_options)

    async def aclose(self) -> None:
        if self._ws is not None and not self._ws.closed:
            with contextlib.suppress(Exception):
                await self._ws.send_json(input_finish_frame())
            with contextlib.suppress(Exception):
                await self._ws.close()
        self._ws = None
        if self._session is not None and not self._session.closed:
            await self._session.close()
            self._session = None

    async def acquire_ws(self) -> aiohttp.ClientWebSocketResponse:
        async with self._ws_lock:
            if self._ws is not None and not self._ws.closed:
                return self._ws
            start = TTSSessionStart(
                request_id=self._session_request_id,
                model=self._model,
                voice=self._voice,
                language_type=self._language_type,
                mode=self._mode,
                sample_rate=self.sample_rate,
                audio_format="pcm_s16le",
            )
            try:
                ws = await open_tts(self._http(), ws_url=self._ws_url, start=start)
            except aiohttp.ClientError as e:
                raise APIConnectionError(message=f"inference-gateway tts ws: {e}") from e
            self._ws = ws
            return ws

    async def mark_ws_broken(self, ws: aiohttp.ClientWebSocketResponse) -> None:
        async with self._ws_lock:
            if self._ws is ws:
                self._ws = None


class _GatewayTTSStream(tts.SynthesizeStream):
    async def _run(self, emitter: tts.AudioEmitter) -> None:
        parent: GatewayTTS = self._tts  # type: ignore[assignment]
        request_id = utils.shortuuid("tts_")
        turn_id = utils.shortuuid("turn_")
        emitter.initialize(
            request_id=request_id,
            sample_rate=parent.sample_rate,
            mime_type="audio/pcm",
            stream=True,
            num_channels=1,
            frame_size_ms=30,
        )

        async with parent._turn_lock:
            ws = await parent.acquire_ws()

            segment_id = utils.shortuuid()
            segment_started = False
            first_audio_emitted = False
            t0 = time.perf_counter()

            if parent._passthrough:
                expected_audio_end = await self._feed_passthrough(ws, parent=parent, turn_id=turn_id)
                if expected_audio_end <= 0:
                    return
                await self._drain_reader(
                    ws,
                    emitter,
                    parent,
                    turn_id,
                    segment_id,
                    expected_audio_end,
                    t0,
                    playback_gate=None,
                    state_box=None,
                )
                return

            # Sentencizer + gate: reader runs concurrently with feeder.
            playback_gate = asyncio.Event()
            state_box: dict[str, Any] = {
                "commits_sent": 0,
                "feeder_done": False,
                "gate_est_ms_queue": deque[int](),
                "gate_delay_task": None,
                "arm_gate_on_first_chunk": False,
            }

            reader_task = asyncio.create_task(
                self._drain_reader(
                    ws,
                    emitter,
                    parent,
                    turn_id,
                    segment_id,
                    -1,
                    t0,
                    playback_gate=playback_gate,
                    state_box=state_box,
                ),
                name="gateway-tts-read-gated",
            )
            commits = 0
            try:
                commits = await self._feed_with_sentencizer(
                    ws, parent=parent, turn_id=turn_id, playback_gate=playback_gate, state_box=state_box
                )
            finally:
                state_box["feeder_done"] = True
                if commits == 0:
                    reader_task.cancel()
                    with contextlib.suppress(BaseException):
                        await reader_task
                else:
                    try:
                        await asyncio.wait_for(reader_task, timeout=300.0)
                    except asyncio.TimeoutError:
                        logger.warning("tts: gated reader did not finish within 300s, cancelling")
                        reader_task.cancel()
                        with contextlib.suppress(BaseException):
                            await reader_task

    async def _drain_reader(
        self,
        ws: aiohttp.ClientWebSocketResponse,
        emitter: tts.AudioEmitter,
        parent: GatewayTTS,
        turn_id: str,
        segment_id: str,
        expected_audio_end: int,
        t0: float,
        *,
        playback_gate: asyncio.Event | None,
        state_box: dict[str, Any] | None,
    ) -> None:
        """Drain WS audio. If ``playback_gate`` / ``state_box`` are set (gated
        sentencizer mode), open the gate only after the duration-based delay from
        the first audio chunk after each ``input.commit`` (feeder sets
        ``arm_gate_on_first_chunk``). Exit when feeder is done and the reader has
        observed enough ``audio.end`` events, or after an idle timeout if upstream
        coalesces multiple commits into a single ``audio.end``.
        """
        segment_started = False
        first_audio_emitted = False
        seen_audio_end = 0

        def _commits_sent() -> int:
            return int(state_box["commits_sent"]) if state_box else 0

        def _feeder_done() -> bool:
            return bool(state_box["feeder_done"]) if state_box else False

        async def _cancel_gate_delay_task() -> None:
            if state_box is None:
                return
            task = state_box.get("gate_delay_task")
            if isinstance(task, asyncio.Task) and not task.done():
                task.cancel()
                with contextlib.suppress(BaseException):
                    await task
            state_box["gate_delay_task"] = None

        while True:
            recv_timeout: float | None = None
            if (
                playback_gate is not None
                and state_box is not None
                and _feeder_done()
                and seen_audio_end >= 1
                and seen_audio_end < _commits_sent()
            ):
                recv_timeout = _COALESCED_AUDIO_END_IDLE_S

            try:
                if recv_timeout is not None:
                    msg = await asyncio.wait_for(ws.receive(), timeout=recv_timeout)
                else:
                    msg = await ws.receive()
            except TimeoutError:
                if (
                    playback_gate is not None
                    and state_box is not None
                    and _feeder_done()
                    and seen_audio_end >= 1
                    and seen_audio_end < _commits_sent()
                ):
                    logger.warning(
                        "tts gated reader: idle %.1fs after audio.end (commits=%d ends=%d); "
                        "assuming coalesced upstream — closing reader",
                        _COALESCED_AUDIO_END_IDLE_S,
                        _commits_sent(),
                        seen_audio_end,
                    )
                    return
                raise

            if msg.type in (aiohttp.WSMsgType.CLOSE, aiohttp.WSMsgType.CLOSED, aiohttp.WSMsgType.CLOSING):
                await parent.mark_ws_broken(ws)
                return
            if msg.type == aiohttp.WSMsgType.ERROR:
                await parent.mark_ws_broken(ws)
                raise APIStatusError(
                    message=f"inference-gateway tts ws error: {ws.exception()}",
                    status_code=-1,
                    retryable=True,
                )
            ev = decode_text_event(msg)
            if ev is None:
                continue
            if ev.type == EventType.ERROR:
                if ev.turn_id and ev.turn_id != turn_id:
                    continue
                raise APIStatusError(
                    message=f"inference-gateway tts: {ev.code or 'ERROR'}: {ev.message}",
                    status_code=-1,
                    retryable=ev.retryable,
                )
            if ev.type == EventType.AUDIO_CHUNK and ev.pcm_b64:
                if ev.turn_id and ev.turn_id != turn_id:
                    continue

                if playback_gate is not None and bool(state_box.get("arm_gate_on_first_chunk")):
                    assert state_box is not None
                    state_box["arm_gate_on_first_chunk"] = False
                    await _cancel_gate_delay_task()
                    q = state_box.get("gate_est_ms_queue")
                    est_ms = 0
                    if isinstance(q, deque) and q:
                        est_ms = int(q.popleft())
                    else:
                        logger.warning(
                            "tts playback gate: first audio but no queued est_ms (queue=%r); "
                            "opening immediately",
                            q,
                        )
                    budget = parent._gate_generation_budget_ms
                    extra_ms = parent._gate_extra_delay_ms
                    delay_s = max(0.0, (est_ms - budget) / 1000.0) + extra_ms / 1000.0
                    if delay_s <= 0:
                        playback_gate.set()
                        logger.info(
                            "tts playback gate: open now (est_ms=%d budget_ms=%d extra_ms=%d)",
                            est_ms,
                            budget,
                            extra_ms,
                        )
                    else:

                        async def _delayed_gate() -> None:
                            await asyncio.sleep(delay_s)
                            playback_gate.set()
                            logger.info(
                                "tts playback gate: open after delay "
                                "(est_ms=%d budget_ms=%d extra_ms=%d delay_s=%.3f)",
                                est_ms,
                                budget,
                                extra_ms,
                                delay_s,
                            )

                        state_box["gate_delay_task"] = asyncio.create_task(
                            _delayed_gate(),
                            name="tts-playback-gate-delay",
                        )

                if not segment_started:
                    emitter.start_segment(segment_id=segment_id)
                    segment_started = True
                if not first_audio_emitted:
                    first_audio_emitted = True
                    emit(
                        "tts.first_audio",
                        model=parent._model,
                        ts_ms=int(time.time() * 1000),
                    )
                    logger.info(
                        "tts first audio",
                        extra={"spent_s": round(time.perf_counter() - t0, 3)},
                    )
                try:
                    emitter.push(data=base64.b64decode(ev.pcm_b64))
                except (ValueError, TypeError) as e:
                    logger.warning("tts: invalid pcm_b64: %s", e)
                if ev.seq >= 0:
                    parent._last_chunk_seq = ev.seq
            if ev.type == EventType.AUDIO_END:
                if ev.turn_id and ev.turn_id != turn_id:
                    continue
                seen_audio_end += 1
                if playback_gate is None:
                    if expected_audio_end > 0 and seen_audio_end >= expected_audio_end:
                        return
                else:
                    assert state_box is not None
                    commits_sent = int(state_box["commits_sent"])
                    done = bool(state_box["feeder_done"])
                    if done and commits_sent > 0 and seen_audio_end >= commits_sent:
                        return
            if ev.type == EventType.SESSION_END:
                return

    async def _feed_with_sentencizer(
        self,
        ws: aiohttp.ClientWebSocketResponse,
        *,
        parent: GatewayTTS,
        turn_id: str,
        playback_gate: asyncio.Event | None = None,
        state_box: dict[str, Any] | None = None,
    ) -> int:
        splitter = StreamingSentencizer(remove_emoji=True)
        aborted = False
        commits = 0
        gated = playback_gate is not None and state_box is not None

        async def _push_sentences(sents: list[str]) -> None:
            nonlocal aborted, commits
            for s in sents:
                text = s.strip()
                if not text:
                    continue
                if ws.closed:
                    aborted = True
                    return
                if gated and commits > 0:
                    try:
                        await asyncio.wait_for(
                            playback_gate.wait(),  # type: ignore[union-attr]
                            timeout=_GATE_WAIT_SAFETY_S,
                        )
                    except TimeoutError:
                        logger.warning(
                            "tts playback gate: safety unlock after %.0fs (missing first audio?)",
                            _GATE_WAIT_SAFETY_S,
                        )
                        if state_box is not None:
                            state_box["arm_gate_on_first_chunk"] = False
                            t = state_box.get("gate_delay_task")
                            if isinstance(t, asyncio.Task) and not t.done():
                                t.cancel()
                        playback_gate.set()  # type: ignore[union-attr]
                    playback_gate.clear()  # type: ignore[union-attr]
                emit(
                    "tts.sentence.start",
                    model=parent._model,
                    voice=parent._voice,
                    language_type=parent._language_type,
                    text=text,
                )
                t_est = time.perf_counter()
                est_ms = 0
                try:
                    est_ms, est_detail = estimate_sentence_duration_ms(
                        text,
                        lang_hint=parent._language_type or None,
                        calibrate=True,
                    )
                    compute_ms = (time.perf_counter() - t_est) * 1000.0
                    logger.info(
                        "tts sentence duration estimate: %d ms (lang=%s, source=%s, "
                        "compute_ms=%.3f, text=%r)",
                        est_ms,
                        est_detail["lang"],
                        est_detail["lang_source"],
                        compute_ms,
                        text[:200] + ("…" if len(text) > 200 else ""),
                    )
                except Exception:  # noqa: BLE001
                    compute_ms = (time.perf_counter() - t_est) * 1000.0
                    logger.debug(
                        "tts sentence duration estimate failed (compute_ms=%.3f)",
                        compute_ms,
                        exc_info=True,
                    )
                    est_ms = 0
                try:
                    await ws.send_json(input_text_frame(text, turn_id=turn_id))
                    await ws.send_json(input_commit_frame())
                    commits += 1
                    if gated:
                        state_box["commits_sent"] = commits  # type: ignore[index]
                        state_box["gate_est_ms_queue"].append(est_ms)  # type: ignore[index]
                        # Allow the reader to arm from the first chunk of *this* commit only,
                        # using the matching est_ms (not a shared field overwritten later).
                        state_box["arm_gate_on_first_chunk"] = True  # type: ignore[index]
                    emit(
                        "tts.input.sent",
                        model=parent._model,
                        mode=parent._mode,
                        passthrough=parent._passthrough,
                        turn_id=turn_id,
                        text=text,
                        commit=True,
                    )
                    logger.info("tts input sent", extra={"mode": parent._mode, "text": text, "commit": True})
                except (aiohttp.ClientConnectionResetError, ConnectionResetError):
                    aborted = True
                    return
                emit("tts.sentence.end", model=parent._model, text=text)

        async for token in self._input_ch:
            if aborted:
                return commits
            if isinstance(token, self._FlushSentinel):
                await _push_sentences(splitter.flush())
            else:
                await _push_sentences(splitter.push(text=token))
        await _push_sentences(splitter.flush())
        return commits

    async def _feed_passthrough(
        self, ws: aiohttp.ClientWebSocketResponse, *, parent: GatewayTTS, turn_id: str
    ) -> int:
        sent_any = False
        emit(
            "tts.sentence.start",
            model=parent._model,
            voice=parent._voice,
            language_type=parent._language_type,
            text="(passthrough)",
        )
        async for token in self._input_ch:
            if isinstance(token, self._FlushSentinel):
                continue
            if not token:
                continue
            # Gateway may close the WS mid-stream when upstream fails (e.g.
            # invalid voice/language combo, quota, network reset). aiohttp
            # marks ``closed``/``closing`` and any further ``send_json`` would
            # raise ``ClientConnectionResetError``. We exit the feed loop
            # silently so the surrounding ``_run`` finalizer can drain
            # ``_read_audio``, which surfaces the gateway's error event as a
            # proper ``APIStatusError``.
            if ws.closed:
                return 0
            try:
                await ws.send_json(input_text_frame(token, turn_id=turn_id))
                emit(
                    "tts.input.sent",
                    model=parent._model,
                    mode=parent._mode,
                    passthrough=parent._passthrough,
                    turn_id=turn_id,
                    text=token,
                    commit=False,
                )
                logger.info("tts input sent", extra={"mode": parent._mode, "text": token, "commit": False})
            except (aiohttp.ClientConnectionResetError, ConnectionResetError):
                return 0
            sent_any = True
        if sent_any and not ws.closed:
            with contextlib.suppress(Exception):
                await ws.send_json(input_commit_frame())
        emit("tts.sentence.end", model=parent._model, text="(passthrough)")
        return 1 if sent_any else 0


def _http_base(ws_url: str) -> str:
    if ws_url.startswith("wss://"):
        return "https://" + ws_url[len("wss://") :].split("/", 1)[0]
    if ws_url.startswith("ws://"):
        return "http://" + ws_url[len("ws://") :].split("/", 1)[0]
    return ws_url
