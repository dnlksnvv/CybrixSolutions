"""Thin async transport for the inference-gateway v1 protocol.

- ``chat_stream`` → POST ``/v1/llm/chat``, yields decoded ``GatewayEvent`` from NDJSON.
- ``open_tts``    → connects to ``/v1/tts/ws`` and sends ``session.start``.
- ``open_stt``    → connects to ``/v1/stt/ws`` and sends ``session.start``.

Higher-level adapters (``llm.py``, ``tts.py``, ``stt.py``) wrap this with
LiveKit semantics (``LLMStream``, ``SynthesizeStream``, ``SpeechStream``).
"""

from __future__ import annotations

import json
from collections.abc import AsyncIterator
from typing import Any

import aiohttp

from .protocol import GatewayEvent, LLMRequest, STTSessionStart, TTSSessionStart


class GatewayError(RuntimeError):
    """Transport-level failure or v1 ``error`` event from the gateway."""

    def __init__(self, message: str, *, code: str = "", retryable: bool = False) -> None:
        super().__init__(message)
        self.code = code
        self.retryable = retryable


async def chat_stream(
    session: aiohttp.ClientSession, *, base_url: str, request: LLMRequest
) -> AsyncIterator[GatewayEvent]:
    """POST ``/v1/llm/chat`` and yield decoded NDJSON ``GatewayEvent`` lines.

    Caller is responsible for the surrounding ``ClientSession``. The HTTP
    body is read line-by-line, so back-pressure propagates naturally.
    """
    url = base_url.rstrip("/") + "/v1/llm/chat"
    body = request.to_json()
    async with session.post(url, json=body) as resp:
        if resp.status // 100 != 2:
            text = (await resp.text())[:500]
            raise GatewayError(
                f"llm http {resp.status}: {text}",
                code="UPSTREAM_4XX" if resp.status // 100 == 4 else "UPSTREAM_5XX",
                retryable=resp.status >= 500,
            )
        async for raw_line in resp.content:
            line = raw_line.decode("utf-8", errors="replace").strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            yield GatewayEvent.from_json(obj)


async def open_tts(
    session: aiohttp.ClientSession,
    *,
    ws_url: str,
    start: TTSSessionStart,
) -> aiohttp.ClientWebSocketResponse:
    """Open WS, send ``session.start``, return the connected WS."""
    ws = await session.ws_connect(ws_url, max_msg_size=0)
    await ws.send_json(start.to_json())
    return ws


async def open_stt(
    session: aiohttp.ClientSession,
    *,
    ws_url: str,
    start: STTSessionStart,
) -> aiohttp.ClientWebSocketResponse:
    """Open WS, send ``session.start``, return the connected WS."""
    ws = await session.ws_connect(ws_url, max_msg_size=0)
    await ws.send_json(start.to_json())
    return ws


def decode_text_event(msg: aiohttp.WSMessage) -> GatewayEvent | None:
    """Decode one WS text frame into a ``GatewayEvent``; return ``None`` on parse error."""
    if msg.type != aiohttp.WSMsgType.TEXT:
        return None
    try:
        obj: Any = json.loads(msg.data)
    except (json.JSONDecodeError, TypeError):
        return None
    if not isinstance(obj, dict):
        return None
    return GatewayEvent.from_json(obj)
