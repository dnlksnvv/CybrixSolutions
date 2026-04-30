from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any
from urllib.parse import urlparse


def _data_dir() -> str:
    return os.environ.get("CS_VOICEAGENT_DATA_DIR", "data")


def _settings_path() -> str:
    return os.path.join(_data_dir(), "settings.json")


def _load_settings_file() -> dict[str, Any]:
    try:
        path = _settings_path()
        if not os.path.exists(path):
            return {}
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f) if f.readable() else {}
    except Exception:
        return {}


def _save_settings_file(data: dict[str, Any]) -> None:
    os.makedirs(_data_dir(), exist_ok=True)
    with open(_settings_path(), "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


@dataclass
class Settings:
    # Region: Singapore / International
    dashscope_base_http: str = "https://dashscope-intl.aliyuncs.com/api/v1"
    dashscope_openai_base: str = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"

    # API keys
    dashscope_api_key: str = ""

    # Defaults
    default_llm_model: str = "qwen-plus"
    default_stt_model: str = "qwen3-asr-flash"
    default_tts_model: str = "qwen3-tts-flash"

    # Voice chat pipeline (LLM): optional separate OpenAI-compatible base/key; empty → DashScope defaults
    voice_chat_llm_model: str = ""
    voice_chat_llm_api_key: str = ""
    voice_chat_openai_base: str = ""
    voice_chat_system_prompt: str = (
        "You are a helpful voice assistant. Reply clearly and concisely in the same language as the user."
    )

    # Storage
    data_dir: str = "data"
    voices_json: str = os.path.join("data", "voices.json")


def dashscope_realtime_ws_base(settings: Settings) -> str:
    """
    Realtime TTS/ASR WebSocket host must match the API key region (Singapore intl vs Beijing cn).
    Derive from the same bases used for HTTP/OpenAI-compatible calls.
    """
    for base in (settings.dashscope_openai_base, settings.dashscope_base_http):
        host = (urlparse(base).hostname or "").lower()
        if "dashscope-intl" in host:
            return "wss://dashscope-intl.aliyuncs.com/api-ws/v1/realtime"
        if host.endswith("dashscope.aliyuncs.com") and "intl" not in host:
            return "wss://dashscope.aliyuncs.com/api-ws/v1/realtime"
    return "wss://dashscope-intl.aliyuncs.com/api-ws/v1/realtime"


def load_settings() -> Settings:
    file_data = _load_settings_file()

    # env overrides always win
    env_key = os.environ.get("DASHSCOPE_API_KEY", "").strip()

    data_dir = _data_dir()
    def_vc_model = str(file_data.get("default_llm_model") or os.environ.get("DEFAULT_LLM_MODEL", "qwen-plus"))
    s = Settings(
        dashscope_base_http=str(file_data.get("dashscope_base_http") or "https://dashscope-intl.aliyuncs.com/api/v1"),
        dashscope_openai_base=str(
            file_data.get("dashscope_openai_base") or "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
        ),
        dashscope_api_key=str(file_data.get("dashscope_api_key") or ""),
        default_llm_model=str(file_data.get("default_llm_model") or os.environ.get("DEFAULT_LLM_MODEL", "qwen-plus")),
        default_stt_model=str(file_data.get("default_stt_model") or os.environ.get("DEFAULT_STT_MODEL", "qwen3-asr-flash")),
        default_tts_model=str(file_data.get("default_tts_model") or os.environ.get("DEFAULT_TTS_MODEL", "qwen3-tts-flash")),
        voice_chat_llm_model=str(file_data.get("voice_chat_llm_model") or def_vc_model),
        voice_chat_llm_api_key=str(file_data.get("voice_chat_llm_api_key") or ""),
        voice_chat_openai_base=str(file_data.get("voice_chat_openai_base") or ""),
        voice_chat_system_prompt=str(
            file_data.get("voice_chat_system_prompt")
            or "You are a helpful voice assistant. Reply clearly and concisely in the same language as the user."
        ),
        data_dir=data_dir,
        voices_json=os.path.join(data_dir, "voices.json"),
    )

    if env_key:
        s.dashscope_api_key = env_key
    return s


def save_settings(partial: dict[str, Any]) -> Settings:
    current = _load_settings_file()
    current.update({k: v for k, v in partial.items() if v is not None})
    _save_settings_file(current)
    return load_settings()


settings = load_settings()

