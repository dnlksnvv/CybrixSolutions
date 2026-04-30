"""Cybrix inference-gateway client for LiveKit Agents.

Single place for everything that talks to ``inference-gateway``:

- ``protocol``  — typed templates for the v1 wire format (``LLMRequest``,
  ``TTSSessionStart``, ``STTSessionStart`` …). Mirrors
  ``inference-gateway/internal/protocol/v1/v1.go``. **No business logic.**
- ``client``    — thin async transport (HTTP NDJSON / WebSocket) over aiohttp.
- ``config``    — env reader for gateway URL + per-modality model/voice/lang.
- ``llm`` / ``tts`` / ``stt`` — LiveKit adapters that plug into ``AgentSession``.

Workers should only import from this package; nothing else here is allowed to
talk to upstream APIs (DashScope/Sber/etc.) directly.
"""

from .config import GatewaySettings, load_settings
from .llm import GatewayLLM
from .protocol import (
    EventType,
    GatewayEvent,
    LLMInput,
    LLMRequest,
    Message,
    STTSessionStart,
    TTSSessionStart,
)
from .stt import GatewaySTT
from .tts import GatewayTTS

__all__ = [
    "EventType",
    "GatewayEvent",
    "GatewayLLM",
    "GatewaySTT",
    "GatewaySettings",
    "GatewayTTS",
    "LLMInput",
    "LLMRequest",
    "Message",
    "STTSessionStart",
    "TTSSessionStart",
    "load_settings",
]
