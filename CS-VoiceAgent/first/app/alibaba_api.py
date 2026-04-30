from __future__ import annotations

import base64
import mimetypes
from collections.abc import Iterator
from typing import Any

import requests

from .config import settings


class AlibabaApiError(RuntimeError):
    pass


def _auth_headers() -> dict[str, str]:
    if not settings.dashscope_api_key:
        raise AlibabaApiError("DASHSCOPE_API_KEY is not set")
    return {
        "Authorization": f"Bearer {settings.dashscope_api_key}",
        "Content-Type": "application/json",
    }


# -----------------------
# LLM (OpenAI-compatible)
# -----------------------
def llm_chat(model: str, messages: list[dict[str, Any]], temperature: float = 0.7) -> dict[str, Any]:
    url = f"{settings.dashscope_openai_base}/chat/completions"
    payload = {
        "model": model,
        "messages": messages,
        "stream": False,
        "temperature": temperature,
    }
    r = requests.post(url, headers=_auth_headers(), json=payload, timeout=120)
    if r.status_code != 200:
        raise AlibabaApiError(f"LLM error: {r.status_code} {r.text}")
    return r.json()


def llm_chat_stream_sse(
    base_url: str,
    api_key: str,
    model: str,
    messages: list[dict[str, Any]],
    temperature: float = 0.7,
) -> Iterator[str]:
    """
    Stream OpenAI-compatible SSE lines from chat/completions (stream=true).
    Yields each line as returned by the upstream (including "data: {...}" and empty lines).
    """
    if not api_key:
        raise AlibabaApiError("API key is not set for streaming LLM")
    url = f"{base_url.rstrip('/')}/chat/completions"
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": True,
        "temperature": temperature,
    }
    with requests.post(url, headers=headers, json=payload, stream=True, timeout=180) as r:
        if r.status_code != 200:
            err_body = r.text
            try:
                err_body = r.json()
            except Exception:
                pass
            raise AlibabaApiError(f"LLM stream error: {r.status_code} {err_body}")
        for line in r.iter_lines(decode_unicode=True):
            if line is None:
                continue
            yield line + "\n"


# -----------------------
# STT (Qwen3-ASR-Flash)
# -----------------------
def _to_data_url(filename: str, content: bytes) -> str:
    mime, _ = mimetypes.guess_type(filename)
    if not mime:
        mime = "audio/mpeg"
    b64 = base64.b64encode(content).decode("utf-8")
    return f"data:{mime};base64,{b64}"


def stt_transcribe(model: str, filename: str, content: bytes, language: str | None = None) -> dict[str, Any]:
    """
    Uses OpenAI-compatible endpoint per docs:
      POST https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions
    """
    url = f"{settings.dashscope_openai_base}/chat/completions"
    data_uri = _to_data_url(filename, content)
    payload: dict[str, Any] = {
        "model": model,
        "messages": [
            {
                "role": "user",
                "content": [
                    {"type": "input_audio", "input_audio": {"data": data_uri}},
                ],
            }
        ],
        "stream": False,
        "asr_options": {"enable_itn": False, **({"language": language} if language else {})},
    }
    r = requests.post(url, headers=_auth_headers(), json=payload, timeout=300)
    if r.status_code != 200:
        raise AlibabaApiError(f"STT error: {r.status_code} {r.text}")
    return r.json()


# -----------------------
# Voice cloning (enroll)
# -----------------------
def voice_clone_create(target_model: str, preferred_name: str, filename: str, content: bytes) -> dict[str, Any]:
    """
    Per docs:
      POST https://dashscope-intl.aliyuncs.com/api/v1/services/audio/tts/customization
      model: qwen-voice-enrollment
      input.action=create
      input.target_model=<tts model used later>
      output.voice is the voiceprint id used as `voice` in TTS
    """
    url = f"{settings.dashscope_base_http}/services/audio/tts/customization"
    data_uri = _to_data_url(filename, content)
    payload = {
        "model": "qwen-voice-enrollment",
        "input": {
            "action": "create",
            "target_model": target_model,
            "preferred_name": preferred_name,
            "audio": {"data": data_uri},
        },
    }
    r = requests.post(url, headers=_auth_headers(), json=payload, timeout=300)
    if r.status_code != 200:
        raise AlibabaApiError(f"Voice clone error: {r.status_code} {r.text}")
    return r.json()

def voice_clone_list(page_index: int = 0, page_size: int = 50) -> dict[str, Any]:
    """
    List voices created via qwen-voice-enrollment.
    Per docs: action=list on the same customization endpoint.
    """
    url = f"{settings.dashscope_base_http}/services/audio/tts/customization"
    payload = {
        "model": "qwen-voice-enrollment",
        "input": {
            "action": "list",
            "page_index": page_index,
            "page_size": page_size,
        },
    }
    r = requests.post(url, headers=_auth_headers(), json=payload, timeout=120)
    if r.status_code != 200:
        raise AlibabaApiError(f"Voice list error: {r.status_code} {r.text}")
    return r.json()


# -----------------------
# TTS (DashScope endpoint)
# -----------------------
def tts_synthesize(model: str, text: str, voice: str, language_type: str = "Auto") -> dict[str, Any]:
    """
    Per docs:
      POST https://dashscope-intl.aliyuncs.com/api/v1/services/aigc/multimodal-generation/generation
      body: { model, input: { text, voice, language_type } }
    """
    url = f"{settings.dashscope_base_http}/services/aigc/multimodal-generation/generation"
    payload = {
        "model": model,
        "input": {
            "text": text,
            "voice": voice,
            "language_type": language_type,
        },
    }
    r = requests.post(url, headers=_auth_headers(), json=payload, timeout=300)
    if r.status_code != 200:
        raise AlibabaApiError(f"TTS error: {r.status_code} {r.text}")
    return r.json()

