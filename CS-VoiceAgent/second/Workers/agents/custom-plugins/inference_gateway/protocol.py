"""Typed templates for the inference-gateway v1 wire format.

This is the **single source of truth** for what we send to the gateway and
what we expect back. No transport, no env reading, no LiveKit imports — just
the JSON shapes mirroring ``inference-gateway/internal/protocol/v1/v1.go``.

Any change to gateway's protocol must be reflected here in lockstep.
"""

from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any, Literal

# --- event type constants (mirror v1 in Go) ----------------------------------


class EventType:
    SESSION_START = "session.start"
    SESSION_END = "session.end"
    DELTA = "delta"
    END = "end"
    ERROR = "error"
    INPUT_TEXT = "input.text"
    INPUT_COMMIT = "input.commit"
    INPUT_FINISH = "input.finish"
    CANCEL = "cancel"
    AUDIO_CHUNK = "audio.chunk"
    AUDIO_URL = "audio.url"
    AUDIO_END = "audio.end"
    TRANSCRIPT_PARTIAL = "transcript.partial"
    TRANSCRIPT_FINAL = "transcript.final"


# --- LLM (HTTP POST /v1/llm/chat) --------------------------------------------


@dataclass
class Message:
    """One chat-completion message (system / user / assistant)."""

    role: Literal["system", "user", "assistant", "developer", "tool"]
    content: str


@dataclass
class LLMInput:
    """Generation parameters for ``LLMRequest.input``.

    ``temperature`` and ``max_tokens`` are passed verbatim to the upstream
    when not ``None``. ``stream`` is informational; the endpoint always
    streams (NDJSON of ``GatewayEvent``).
    """

    messages: list[Message]
    temperature: float | None = None
    max_tokens: int | None = None
    stream: bool = True


@dataclass
class LLMRequest:
    """Body for ``POST /v1/llm/chat``."""

    request_id: str
    model: str
    input: LLMInput
    call_id: str = ""

    def to_json(self) -> dict[str, Any]:
        return _drop_empty(
            {
                "request_id": self.request_id,
                "call_id": self.call_id,
                "model": self.model,
                "input": _drop_empty(
                    {
                        "messages": [{"role": m.role, "content": m.content} for m in self.input.messages],
                        "temperature": self.input.temperature,
                        "max_tokens": self.input.max_tokens,
                        "stream": self.input.stream,
                    }
                ),
            }
        )


# --- TTS (WS /v1/tts/ws) -----------------------------------------------------


@dataclass
class TTSSessionStart:
    """First frame on the TTS WebSocket. Universal across all TTS models.

    Empty fields fall back to gateway's per-model ``.env`` defaults.
    """

    request_id: str
    model: str
    call_id: str = ""
    voice: str = ""
    language_type: str = ""
    """DashScope-only: ``Russian`` | ``Auto`` | ``English`` | …"""
    mode: Literal["", "commit", "server_commit"] = ""
    """DashScope-only: empty → gateway picks from its env."""
    sample_rate: int = 0
    audio_format: str = ""

    type: str = field(default=EventType.SESSION_START, init=False)

    def to_json(self) -> dict[str, Any]:
        return _drop_empty(asdict(self))


# --- STT (WS /v1/stt/ws) -----------------------------------------------------


@dataclass
class STTSessionStart:
    """First frame on the STT WebSocket. Universal across all STT models."""

    request_id: str
    model: str
    call_id: str = ""
    language: str = ""  # ISO: ru, en, …
    sample_rate: int = 0  # 16000 by default
    audio_format: str = ""  # ``pcm_s16le`` mono

    type: str = field(default=EventType.SESSION_START, init=False)

    def to_json(self) -> dict[str, Any]:
        return _drop_empty(asdict(self))


# --- streaming events (server → client and a few client → server) ------------


@dataclass
class GatewayEvent:
    """Mirror of v1.Event from the gateway. All fields optional, ``type`` is required.

    Used for both decoded incoming frames and a couple of outgoing frames
    (``input.text``, ``audio.chunk``).
    """

    type: str
    request_id: str = ""
    turn_id: str = ""
    text: str = ""
    seq: int = 0
    pcm_b64: str = ""
    url: str = ""
    media_type: str = ""
    sample_rate: int = 0
    duration_ms: int = 0
    start_ms: int | None = None
    end_ms: int | None = None
    code: str = ""
    message: str = ""
    retryable: bool = False

    @classmethod
    def from_json(cls, obj: dict[str, Any]) -> GatewayEvent:
        return cls(
            type=str(obj.get("type") or ""),
            request_id=str(obj.get("request_id") or ""),
            turn_id=str(obj.get("turn_id") or ""),
            text=str(obj.get("text") or ""),
            seq=int(obj.get("seq") or 0),
            pcm_b64=str(obj.get("pcm_b64") or ""),
            url=str(obj.get("url") or ""),
            media_type=str(obj.get("media_type") or ""),
            sample_rate=int(obj.get("sample_rate") or 0),
            duration_ms=int(obj.get("duration_ms") or 0),
            start_ms=_optional_int(obj.get("start_ms")),
            end_ms=_optional_int(obj.get("end_ms")),
            code=str(obj.get("code") or ""),
            message=str(obj.get("message") or ""),
            retryable=bool(obj.get("retryable") or False),
        )


# --- outgoing event helpers (client → server) --------------------------------


def input_text_frame(text: str, *, turn_id: str = "") -> dict[str, Any]:
    """``{"type":"input.text","text":...}`` — append text to the TTS buffer."""
    frame: dict[str, Any] = {"type": EventType.INPUT_TEXT, "text": text}
    if turn_id:
        frame["turn_id"] = turn_id
    return frame


def input_commit_frame() -> dict[str, Any]:
    """``{"type":"input.commit"}`` — flush TTS buffer / boundary marker."""
    return {"type": EventType.INPUT_COMMIT}


def input_finish_frame() -> dict[str, Any]:
    """``{"type":"input.finish"}`` — end of stream (TTS or STT)."""
    return {"type": EventType.INPUT_FINISH}


def cancel_frame() -> dict[str, Any]:
    """``{"type":"cancel"}`` — abort the current synthesis/recognition."""
    return {"type": EventType.CANCEL}


def audio_chunk_frame(pcm_b64: str) -> dict[str, Any]:
    """``{"type":"audio.chunk","pcm_b64":...}`` — STT input audio frame."""
    return {"type": EventType.AUDIO_CHUNK, "pcm_b64": pcm_b64}


# --- helpers -----------------------------------------------------------------


def _drop_empty(d: dict[str, Any]) -> dict[str, Any]:
    """Strip empty strings/zeros so optional fields stay out of the JSON."""
    out: dict[str, Any] = {}
    for k, v in d.items():
        if v is None:
            continue
        if isinstance(v, str) and v == "":
            continue
        if isinstance(v, int) and v == 0 and k not in ("temperature",):
            continue
        if isinstance(v, dict):
            sub = _drop_empty(v)
            if sub:
                out[k] = sub
            continue
        out[k] = v
    return out


def _optional_int(v: Any) -> int | None:
    if v is None:
        return None
    try:
        return int(v)
    except (TypeError, ValueError):
        return None
