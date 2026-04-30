"""LiveKit ``llm.LLM`` backed by the inference-gateway HTTP NDJSON endpoint.

Translates LiveKit ``ChatContext`` → ``protocol.LLMRequest`` and emits
``ChatChunk`` deltas as they arrive. Tool calls are not supported (the
gateway intentionally limits LLM to plain chat completion for now).
"""

from __future__ import annotations

import logging
from typing import Any

import aiohttp

from livekit.agents import llm, utils
from livekit.agents._exceptions import APIConnectionError, APIStatusError
from livekit.agents.types import (
    DEFAULT_API_CONNECT_OPTIONS,
    NOT_GIVEN,
    APIConnectOptions,
    NotGivenOr,
)

from .client import GatewayError, chat_stream
from .protocol import EventType, LLMInput, LLMRequest, Message
from trace_emitter import emit

logger = logging.getLogger("inference-gateway-llm")


class GatewayLLM(llm.LLM):
    """LLM that proxies through ``inference-gateway`` ``/v1/llm/chat``."""

    def __init__(
        self,
        *,
        base_url: str,
        model: str,
        temperature: float | None = None,
        max_tokens: int | None = None,
    ) -> None:
        super().__init__()
        self._base_url = base_url.rstrip("/")
        self._model = model
        self._temperature = temperature
        self._max_tokens = max_tokens
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

    async def aclose(self) -> None:
        if self._session is not None and not self._session.closed:
            await self._session.close()

    async def warmup(self) -> None:
        """Open the TCP+keepalive pool to the gateway eagerly."""
        sess = self._http()
        try:
            async with sess.get(self._base_url + "/", timeout=aiohttp.ClientTimeout(total=2.0)):
                pass
        except Exception:  # noqa: BLE001 — best-effort handshake
            pass

    def chat(
        self,
        *,
        chat_ctx: llm.ChatContext,
        tools: list[llm.Tool] | None = None,
        conn_options: APIConnectOptions = DEFAULT_API_CONNECT_OPTIONS,
        parallel_tool_calls: NotGivenOr[bool] = NOT_GIVEN,
        tool_choice: NotGivenOr[llm.ToolChoice] = NOT_GIVEN,
        extra_kwargs: NotGivenOr[dict[str, Any]] = NOT_GIVEN,
    ) -> llm.LLMStream:
        if tools:
            logger.warning("inference-gateway LLM ignores tools; falling back to plain chat")
        return _GatewayLLMStream(
            llm=self,
            chat_ctx=chat_ctx,
            tools=tools or [],
            conn_options=conn_options,
        )


class _GatewayLLMStream(llm.LLMStream):
    async def _run(self) -> None:
        parent: GatewayLLM = self._llm  # type: ignore[assignment]
        request = LLMRequest(
            request_id=utils.shortuuid("llm_"),
            model=parent._model,
            input=LLMInput(
                messages=_messages_from_chat_ctx(self._chat_ctx),
                temperature=parent._temperature,
                max_tokens=parent._max_tokens,
                stream=True,
            ),
        )
        full_text_parts: list[str] = []

        try:
            session = parent._http()
            async for ev in chat_stream(session, base_url=parent._base_url, request=request):
                if ev.type == EventType.ERROR:
                    raise APIStatusError(
                        message=f"inference-gateway: {ev.code or 'ERROR'}: {ev.message}",
                        status_code=-1,
                        retryable=ev.retryable,
                    )
                if ev.type == EventType.DELTA and ev.text:
                    full_text_parts.append(ev.text)
                    self._event_ch.send_nowait(
                        llm.ChatChunk(
                            id=request.request_id,
                            delta=llm.ChoiceDelta(role="assistant", content=ev.text),
                        )
                    )
                elif ev.type == EventType.END:
                    full_text = "".join(full_text_parts).strip()
                    emit(
                        "llm.output.final",
                        request_id=request.request_id,
                        model=parent._model,
                        text=full_text,
                    )
                    logger.info("llm output final", extra={"request_id": request.request_id, "text": full_text})
                    return
        except APIStatusError:
            raise
        except GatewayError as e:
            raise APIStatusError(message=str(e), status_code=-1, retryable=e.retryable) from e
        except aiohttp.ClientError as e:
            raise APIConnectionError(message=f"inference-gateway: {e}") from e


def _messages_from_chat_ctx(ctx: llm.ChatContext) -> list[Message]:
    """Project LiveKit ``ChatMessage`` items to gateway ``Message`` (text only)."""
    out: list[Message] = []
    for item in ctx.messages():
        text = (item.text_content or "").strip()
        if not text:
            continue
        role = item.role if item.role in ("system", "user", "assistant", "developer", "tool") else "user"
        out.append(Message(role=role, content=text))  # type: ignore[arg-type]
    return out
