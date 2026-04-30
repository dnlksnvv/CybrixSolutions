"""Env-driven configuration for the inference-gateway client.

Workers only specify *what* they want (model id, voice, language). The
gateway's own ``.env`` decides *how* to talk to upstream (URLs, keys,
defaults). This keeps Workers free of provider-specific knobs.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True)
class GatewaySettings:
    base_url: str  # http://host:port (no trailing slash)

    # LLM
    llm_model: str

    # STT
    stt_model: str
    stt_language: str
    stt_sample_rate: int

    # TTS
    tts_model: str
    tts_voice: str
    tts_language_type: str
    tts_sample_rate: int
    tts_mode: str  # "" | "commit" | "server_commit"

    # behaviour (kept verbatim from the original Workers .env)
    preemptive_generation: bool
    preemptive_tts: bool
    tts_passthrough: bool
    #: Generation headroom (ms). Next ``input.commit`` is allowed only after
    #: ``max(0, estimated_playback_ms - budget)`` **wall time** from the first audio
    #: chunk of the current sentence (estimate from :mod:`speech_duration_estimate`).
    tts_gate_generation_budget_ms: int
    #: Added to the gate delay (ms) after the duration-based part; fixed padding, e.g. 2000 = +2 s.
    tts_gate_extra_delay_ms: int

    @property
    def http_chat_url(self) -> str:
        return self.base_url.rstrip("/") + "/v1/llm/chat"

    @property
    def ws_tts_url(self) -> str:
        return _ws_url(self.base_url) + "/v1/tts/ws"

    @property
    def ws_stt_url(self) -> str:
        return _ws_url(self.base_url) + "/v1/stt/ws"


def load_settings() -> GatewaySettings:
    return GatewaySettings(
        base_url=_req("INFERENCE_GATEWAY_URL"),
        llm_model=_req("LLM_MODEL"),
        stt_model=_req("STT_MODEL"),
        stt_language=_req("STT_LANGUAGE"),
        stt_sample_rate=_int("STT_SAMPLE_RATE", 16000),
        tts_model=_req("TTS_MODEL"),
        tts_voice=_str("TTS_VOICE", ""),
        tts_language_type=_str("TTS_LANGUAGE_TYPE", ""),
        tts_sample_rate=_int("TTS_SAMPLE_RATE", 24000),
        tts_mode=_str("TTS_MODE", ""),
        preemptive_generation=_bool("ALIYUN_PREEMPTIVE_GENERATION", False),
        preemptive_tts=_bool("ALIYUN_PREEMPTIVE_TTS", False),
        tts_passthrough=_bool("ALIYUN_TTS_PASSTHROUGH", False),
        tts_gate_generation_budget_ms=_int("TTS_GATE_GENERATION_BUDGET_MS", 600),
        tts_gate_extra_delay_ms=_int("TTS_GATE_EXTRA_DELAY_MS", 0),
    )


def _req(name: str) -> str:
    v = (os.environ.get(name) or "").strip()
    if not v:
        raise ValueError(f"{name} must be set in agents/.env")
    return v


def _str(name: str, default: str) -> str:
    return (os.environ.get(name) or default).strip()


def _int(name: str, default: int) -> int:
    raw = (os.environ.get(name) or "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError:
        return default


def _bool(name: str, default: bool) -> bool:
    raw = (os.environ.get(name) or "").strip().lower()
    if not raw:
        return default
    return raw in ("1", "true", "yes", "on")


def _ws_url(http_url: str) -> str:
    if http_url.startswith("https://"):
        return "wss://" + http_url[len("https://") :]
    if http_url.startswith("http://"):
        return "ws://" + http_url[len("http://") :]
    return http_url
