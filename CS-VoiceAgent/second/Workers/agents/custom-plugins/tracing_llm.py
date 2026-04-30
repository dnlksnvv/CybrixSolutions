"""Wraps an ``llm.LLM`` to emit Cybrix trace milestones (request + first text delta)."""

from __future__ import annotations

import logging
from typing import Any

from livekit.agents import llm
from livekit.agents.types import (
    DEFAULT_API_CONNECT_OPTIONS,
    NOT_GIVEN,
    APIConnectOptions,
    NotGivenOr,
)

from trace_emitter import emit

logger = logging.getLogger("cybrix-tracing-llm")


class TracingLLM(llm.LLM):
    def __init__(self, inner: llm.LLM) -> None:
        super().__init__()
        self._inner = inner

    @property
    def model(self) -> str:
        return self._inner.model

    @property
    def provider(self) -> str:
        return self._inner.provider

    def prewarm(self) -> None:
        self._inner.prewarm()

    async def warmup(self) -> None:
        """Open TCP+TLS to the LLM endpoint so the first ``chat`` doesn't pay handshake cost.

        Uses ``OpenAILLM._client`` (an ``openai.AsyncClient``) when available; falls back to
        ``inner.prewarm()`` (which is a no-op for plain LLMs).
        """
        client = getattr(self._inner, "_client", None)
        if client is None:
            try:
                self._inner.prewarm()
            except Exception:
                logger.exception("llm prewarm fallback failed")
            return
        try:
            await client.get("/", cast_to=str)
        except Exception:
            # Endpoint may 404 on `/`; the TCP+TLS handshake is what we wanted.
            pass

    async def aclose(self) -> None:
        await self._inner.aclose()

    def chat(
        self,
        *,
        chat_ctx: llm.ChatContext,
        tools: list[llm.Tool] | None = None,
        conn_options: APIConnectOptions = DEFAULT_API_CONNECT_OPTIONS,
        parallel_tool_calls: NotGivenOr[bool] = NOT_GIVEN,
        tool_choice: NotGivenOr[llm.ToolChoice] = NOT_GIVEN,
        extra_kwargs: NotGivenOr[dict[str, Any]] = NOT_GIVEN,
        **kwargs: Any,
    ) -> llm.LLMStream:
        emit("llm.request", model=self._inner.model, provider=self._inner.provider)
        inner_stream = self._inner.chat(
            chat_ctx=chat_ctx,
            tools=tools,
            conn_options=conn_options,
            parallel_tool_calls=parallel_tool_calls,
            tool_choice=tool_choice,
            extra_kwargs=extra_kwargs,
            **kwargs,
        )
        return _TracingLLMStream(inner_stream)


class _TracingLLMStream:
    def __init__(self, inner: llm.LLMStream) -> None:
        self._inner = inner
        self._first_text = False

    async def __aenter__(self) -> _TracingLLMStream:
        await self._inner.__aenter__()
        return self

    async def __aexit__(self, exc_type: Any, exc: Any, tb: Any) -> None:
        return await self._inner.__aexit__(exc_type, exc, tb)

    def __aiter__(self) -> _TracingLLMStream:
        return self

    async def __anext__(self) -> llm.ChatChunk:
        chunk = await self._inner.__anext__()
        if not self._first_text and chunk.delta and chunk.delta.content:
            self._first_text = True
            emit(
                "llm.first_token",
                model=self._inner._llm.model,
                provider=self._inner._llm.provider,
                text=chunk.delta.content[:500],
            )
        return chunk

    async def aclose(self) -> None:
        await self._inner.aclose()
