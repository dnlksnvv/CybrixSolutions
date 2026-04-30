from __future__ import annotations

import json
import os
from typing import Any

import asyncio
import base64
import subprocess
import tempfile
from fastapi import Body, FastAPI, File, Form, Request, UploadFile, WebSocket, WebSocketDisconnect
from fastapi.responses import HTMLResponse, JSONResponse, StreamingResponse
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates

from . import alibaba_api
from .config import dashscope_realtime_ws_base, load_settings, save_settings
from .storage import load_voiceprints, new_voiceprint, save_voiceprints, upsert_voiceprint


BASE_DIR = os.path.dirname(os.path.dirname(__file__))
TEMPLATES_DIR = os.path.join(BASE_DIR, "templates")
STATIC_DIR = os.path.join(BASE_DIR, "static")

app = FastAPI(title="CS-VoiceAgent")
templates = Jinja2Templates(directory=TEMPLATES_DIR)

if os.path.exists(STATIC_DIR):
    app.mount("/static", StaticFiles(directory=STATIC_DIR), name="static")

LLM_MODELS = [
    "qwen-max",
    "qwen-plus",
    "qwen-turbo",
    "qwen-flash",
]

STT_MODELS = [
    "qwen3-asr-flash",
    "qwen3-asr-flash-2025-09-08",
    "qwen3-asr-flash-2026-02-10",
]

# Realtime ASR models must be used via /ws/stt (DashScope OmniRealtime WS).
ASR_REALTIME_MODELS = [
    "qwen3-asr-flash-realtime",
    "qwen3-asr-flash-realtime-2025-10-27",
    "qwen3-asr-flash-realtime-2026-02-10",
]

# Realtime TTS models must be used via /ws/tts (DashScope realtime WS).
TTS_REALTIME_MODELS = [
    "qwen3-tts-flash-realtime",
    "qwen3-tts-flash-realtime-2025-09-18",
    "qwen3-tts-flash-realtime-2025-11-27",
    "qwen3-tts-instruct-flash-realtime",
    "qwen3-tts-instruct-flash-realtime-2026-01-22",
    "qwen3-tts-vc-realtime-2026-01-15",
    "qwen3-tts-vd-realtime-2025-12-16",
    "qwen3-tts-vd-realtime-2026-01-15",
]

# Voice enrollment must specify a target TTS model; when you later synthesize, it must match.
VOICE_CLONE_TARGET_MODELS = [
    # voice clone (VC) family
    "qwen3-tts-vc-realtime-2026-01-15",
    "qwen3-tts-vc-2026-01-22",
    # keep flash as a convenient default too
    "qwen3-tts-flash",
]

STT_LANGUAGES = [
    ("", "Auto / not set"),
    ("ru", "ru (Russian)"),
    ("en", "en (English)"),
    ("zh", "zh (Chinese)"),
    ("yue", "yue (Cantonese)"),
    ("es", "es (Spanish)"),
    ("fr", "fr (French)"),
    ("de", "de (German)"),
    ("it", "it (Italian)"),
    ("pt", "pt (Portuguese)"),
    ("ja", "ja (Japanese)"),
    ("ko", "ko (Korean)"),
    ("th", "th (Thai)"),
]

# Qwen realtime ASR supports a large language set; we expose the common ones in UI.
# Docs: https://www.alibabacloud.com/help/en/model-studio/qwen-asr-realtime-python-sdk
ASR_REALTIME_LANGUAGES = [
    ("", "Auto / not set"),
    ("ru", "ru (Russian)"),
    ("en", "en (English)"),
    ("zh", "zh (Chinese)"),
    ("yue", "yue (Cantonese)"),
    ("ja", "ja (Japanese)"),
    ("ko", "ko (Korean)"),
    ("de", "de (German)"),
    ("fr", "fr (French)"),
    ("es", "es (Spanish)"),
    ("pt", "pt (Portuguese)"),
    ("it", "it (Italian)"),
    ("ar", "ar (Arabic)"),
    ("hi", "hi (Hindi)"),
    ("id", "id (Indonesian)"),
    ("th", "th (Thai)"),
    ("tr", "tr (Turkish)"),
    ("uk", "uk (Ukrainian)"),
    ("vi", "vi (Vietnamese)"),
    ("cs", "cs (Czech)"),
    ("da", "da (Danish)"),
    ("fi", "fi (Finnish)"),
    ("fil", "fil (Filipino)"),
    ("is", "is (Icelandic)"),
    ("ms", "ms (Malay)"),
    ("no", "no (Norwegian)"),
    ("pl", "pl (Polish)"),
    ("sv", "sv (Swedish)"),
]

TTS_LANGUAGE_TYPES = [
    ("Auto", "Auto"),
    ("Chinese", "Chinese"),
    ("English", "English"),
    ("Russian", "Russian"),
    ("Spanish", "Spanish"),
    ("French", "French"),
    ("German", "German"),
    ("Italian", "Italian"),
    ("Portuguese", "Portuguese"),
    ("Japanese", "Japanese"),
    ("Korean", "Korean"),
    ("Thai", "Thai"),
]

SYSTEM_VOICES = [
    ("Cherry", "Cherry (system)"),
    ("Ryan", "Ryan (system)"),
]


@app.get("/", response_class=HTMLResponse)
def index(request: Request):
    settings = load_settings()
    voices = load_voiceprints()
    # Starlette's TemplateResponse signature differs across versions.
    # The most compatible call form is (request, name, context).
    return templates.TemplateResponse(
        request,
        "index.html",
        {
            "api_key_set": bool(settings.dashscope_api_key),
            "defaults": {
                "llm_model": settings.default_llm_model,
                "stt_model": settings.default_stt_model,
                "tts_model": settings.default_tts_model,
            },
            "voice_chat": {
                "llm_model": settings.voice_chat_llm_model or settings.default_llm_model,
                "system_prompt": settings.voice_chat_system_prompt,
                "openai_base_set": bool((settings.voice_chat_openai_base or "").strip()),
                "llm_api_key_set": bool((settings.voice_chat_llm_api_key or "").strip()),
            },
            "voices": voices,
            "llm_models": LLM_MODELS,
            "stt_models": STT_MODELS,
            "asr_realtime_models": ASR_REALTIME_MODELS,
            "tts_realtime_models": TTS_REALTIME_MODELS,
            "voice_clone_target_models": VOICE_CLONE_TARGET_MODELS,
            "stt_languages": STT_LANGUAGES,
            "asr_realtime_languages": ASR_REALTIME_LANGUAGES,
            "tts_language_types": TTS_LANGUAGE_TYPES,
            "system_voices": SYSTEM_VOICES,
        },
    )

@app.get("/api/settings")
def api_get_settings():
    s = load_settings()
    return {
        "ok": True,
        "dashscope_api_key_set": bool(s.dashscope_api_key),
        "dashscope_base_http": s.dashscope_base_http,
        "dashscope_openai_base": s.dashscope_openai_base,
        "default_llm_model": s.default_llm_model,
        "default_stt_model": s.default_stt_model,
        "default_tts_model": s.default_tts_model,
        "voice_chat_llm_model": s.voice_chat_llm_model or s.default_llm_model,
        "voice_chat_openai_base": s.voice_chat_openai_base,
        "voice_chat_llm_api_key_set": bool((s.voice_chat_llm_api_key or "").strip()),
    }


@app.post("/api/settings")
def api_set_settings(
    dashscope_api_key: str = Form(""),
    default_llm_model: str = Form(""),
    default_stt_model: str = Form(""),
    default_tts_model: str = Form(""),
    voice_chat_llm_model: str = Form(""),
    voice_chat_llm_api_key: str = Form(""),
    voice_chat_openai_base: str = Form(""),
    voice_chat_system_prompt: str = Form(""),
):
    # IMPORTANT: we never return the key back to the browser
    partial: dict[str, Any] = {}
    if dashscope_api_key.strip():
        partial["dashscope_api_key"] = dashscope_api_key.strip()
    if default_llm_model.strip():
        partial["default_llm_model"] = default_llm_model.strip()
    if default_stt_model.strip():
        partial["default_stt_model"] = default_stt_model.strip()
    if default_tts_model.strip():
        partial["default_tts_model"] = default_tts_model.strip()
    if voice_chat_llm_model.strip():
        partial["voice_chat_llm_model"] = voice_chat_llm_model.strip()
    if voice_chat_llm_api_key.strip():
        partial["voice_chat_llm_api_key"] = voice_chat_llm_api_key.strip()
    if voice_chat_openai_base.strip():
        partial["voice_chat_openai_base"] = voice_chat_openai_base.strip()
    if voice_chat_system_prompt.strip():
        partial["voice_chat_system_prompt"] = voice_chat_system_prompt.strip()
    s = save_settings(partial)
    return {"ok": True, "dashscope_api_key_set": bool(s.dashscope_api_key)}

@app.post("/api/voices/refresh")
def api_voices_refresh():
    """
    Pull the latest voiceprints from Alibaba and store them locally.
    """
    try:
        existing = {v.id: v for v in load_voiceprints()}
        resp = alibaba_api.voice_clone_list(page_index=0, page_size=200)
        # Response schema differs a bit across docs; we rely on output.voices when present.
        voices_raw = resp.get("output", {}).get("voices") or resp.get("output", {}).get("data") or []
        voices = []
        for it in voices_raw if isinstance(voices_raw, list) else []:
            vid = it.get("voice") or it.get("id") or it.get("voice_id")
            if not vid:
                continue
            vid = str(vid)
            old = existing.get(vid)
            voices.append(
                new_voiceprint(
                    id=vid,
                    preferred_name=str(it.get("preferred_name") or it.get("name") or "voice"),
                    target_model=str(it.get("target_model") or ""),
                    transcript=(old.transcript if old else None),
                )
            )
        save_voiceprints(voices)
        return {"ok": True, "count": len(voices), "raw": resp}
    except Exception as e:
        return JSONResponse({"ok": False, "error": str(e)}, status_code=400)


@app.post("/api/llm")
def api_llm(
    model: str = Form(...),
    prompt: str = Form(...),
):
    try:
        resp = alibaba_api.llm_chat(
            model=model,
            messages=[
                {"role": "system", "content": "You are a helpful assistant."},
                {"role": "user", "content": prompt},
            ],
        )
        text = (resp.get("choices") or [{}])[0].get("message", {}).get("content", "")
        return {"ok": True, "text": text, "raw": resp}
    except Exception as e:
        return JSONResponse({"ok": False, "error": str(e)}, status_code=400)


@app.post("/api/llm/stream")
def api_llm_stream(body: dict[str, Any] = Body(...)):
    """
    OpenAI-compatible SSE stream for the voice-chat pipeline.
    System prompt and optional LLM base/key come from server settings (not spoofable from the client).
    Request body: {"messages":[{"role":"user"|"assistant","content":"..."}], "temperature": 0.7}
    """
    s = load_settings()
    messages_in = body.get("messages")
    if not isinstance(messages_in, list) or not messages_in:
        return JSONResponse({"ok": False, "error": "messages (non-empty list) required"}, status_code=400)
    hist: list[dict[str, Any]] = []
    for m in messages_in:
        if not isinstance(m, dict):
            continue
        role = m.get("role")
        content = (m.get("content") or "").strip() if isinstance(m.get("content"), str) else m.get("content")
        if role in ("user", "assistant") and content:
            hist.append({"role": role, "content": content})
    if not hist:
        return JSONResponse({"ok": False, "error": "no valid user/assistant messages"}, status_code=400)
    messages = [{"role": "system", "content": s.voice_chat_system_prompt}] + hist
    model = (s.voice_chat_llm_model or s.default_llm_model).strip()
    api_key = (s.voice_chat_llm_api_key or s.dashscope_api_key or "").strip()
    base = (s.voice_chat_openai_base or s.dashscope_openai_base).strip()
    if not api_key:
        return JSONResponse({"ok": False, "error": "No API key (DashScope or voice-chat LLM key in settings)"}, status_code=400)
    temp = float(body.get("temperature") or 0.7)

    def gen():
        try:
            for line in alibaba_api.llm_chat_stream_sse(base, api_key, model, messages, temperature=temp):
                yield line.encode("utf-8")
        except Exception as e:
            err = json.dumps({"object": "voiceagent.error", "message": str(e)}, ensure_ascii=False)
            yield f"data: {err}\n\n".encode("utf-8")

    return StreamingResponse(gen(), media_type="text/event-stream")


@app.post("/api/stt")
async def api_stt(
    model: str = Form(...),
    language: str | None = Form(None),
    audio: UploadFile = File(...),
):
    try:
        content = await audio.read()
        resp = alibaba_api.stt_transcribe(
            model=model,
            filename=audio.filename or "audio.wav",
            content=content,
            language=language or None,
        )
        text = (resp.get("choices") or [{}])[0].get("message", {}).get("content", "")
        return {"ok": True, "text": text, "raw": resp}
    except Exception as e:
        return JSONResponse({"ok": False, "error": str(e)}, status_code=400)


@app.post("/api/voice-clone")
async def api_voice_clone(
    target_model: str = Form(...),
    preferred_name: str = Form(...),
    transcript: str | None = Form(None),
    audio: UploadFile = File(...),
):
    try:
        content = await audio.read()
        filename = audio.filename or "voice"
        ctype = (audio.content_type or "").lower()

        needs_convert = False
        if filename.lower().endswith((".webm", ".ogg")):
            needs_convert = True
        if ctype in ("audio/webm", "audio/ogg", "video/webm"):
            needs_convert = True

        # Voice enrollment supports WAV/MP3/M4A. Convert browser recordings if needed.
        if needs_convert:
            try:
                with tempfile.TemporaryDirectory() as td:
                    in_path = os.path.join(td, "in.webm" if "webm" in (ctype + filename.lower()) else "in.ogg")
                    out_path = os.path.join(td, "out.wav")
                    with open(in_path, "wb") as f:
                        f.write(content)

                    cmd = [
                        "ffmpeg",
                        "-y",
                        "-i",
                        in_path,
                        "-ac",
                        "1",
                        "-ar",
                        "24000",
                        "-f",
                        "wav",
                        out_path,
                    ]
                    p = subprocess.run(cmd, capture_output=True, text=True)
                    if p.returncode != 0:
                        raise RuntimeError((p.stderr or p.stdout or "ffmpeg conversion failed").strip())

                    with open(out_path, "rb") as f:
                        content = f.read()
                    filename = "voice.wav"
                    ctype = "audio/wav"
            except FileNotFoundError:
                raise RuntimeError("ffmpeg not found. Install it or upload WAV/MP3/M4A.")

        resp = alibaba_api.voice_clone_create(
            target_model=target_model,
            preferred_name=preferred_name,
            filename=filename,
            content=content,
        )
        voice_id = resp.get("output", {}).get("voice")
        if not voice_id:
            raise RuntimeError(f"Missing output.voice in response: {resp}")
        upsert_voiceprint(
            new_voiceprint(
                voice_id,
                preferred_name=preferred_name,
                target_model=target_model,
                transcript=(transcript.strip() if transcript and transcript.strip() else None),
            )
        )
        return {"ok": True, "voice_id": voice_id, "raw": resp}
    except Exception as e:
        return JSONResponse({"ok": False, "error": str(e)}, status_code=400)


@app.post("/api/tts")
def api_tts(
    model: str = Form(...),
    voice: str = Form(...),
    language_type: str = Form("Auto"),
    text: str = Form(...),
):
    try:
        # Realtime models must be called via WebSocket realtime API, not REST multimodal-generation.
        if "-realtime" in model:
            return JSONResponse(
                {
                    "ok": False,
                    "error": (
                        "This model is realtime (contains '-realtime'). "
                        "Use the 'Realtime TTS (browser)' block (WebSocket), "
                        "or switch REST TTS model to a non-realtime one, e.g. 'qwen3-tts-vc-2026-01-22' or 'qwen3-tts-flash'."
                    ),
                },
                status_code=400,
            )

        resp = alibaba_api.tts_synthesize(
            model=model,
            text=text,
            voice=voice,
            language_type=language_type,
        )
        audio_url = resp.get("output", {}).get("audio", {}).get("url")
        return {"ok": True, "audio_url": audio_url, "raw": resp}
    except Exception as e:
        return JSONResponse({"ok": False, "error": str(e)}, status_code=400)


@app.websocket("/ws/tts")
async def ws_tts(websocket: WebSocket):
    """
    Browser realtime TTS proxy:
      - client sends JSON: {"type":"start","model":"...","voice":"...","response_format":"pcm_24k"}
      - client sends JSON: {"type":"text","text":"..."} repeatedly (sentence chunks)
      - client sends JSON: {"type":"finish"}

    Server connects to DashScope realtime WS via DashScope SDK and forwards audio deltas:
      {"type":"audio","pcm_b64":"..."} (PCM s16le 24k mono)
    """
    await websocket.accept()

    settings = load_settings()
    if not settings.dashscope_api_key:
        await websocket.send_json({"type": "error", "error": "DASHSCOPE_API_KEY is not set"})
        await websocket.close()
        return

    try:
        import dashscope
        from dashscope.audio.qwen_tts_realtime import AudioFormat, QwenTtsRealtime, QwenTtsRealtimeCallback
    except Exception as e:
        await websocket.send_json({"type": "error", "error": f"dashscope dependency missing: {e}"})
        await websocket.close()
        return

    dashscope.api_key = settings.dashscope_api_key

    loop = asyncio.get_running_loop()
    audio_q: asyncio.Queue[str] = asyncio.Queue()
    done_evt = asyncio.Event()
    err_holder: dict[str, str] = {}

    client_holder: dict[str, Any] = {"client": None}

    class Callback(QwenTtsRealtimeCallback):
        def on_open(self) -> None:
            loop.call_soon_threadsafe(lambda: None)

        def on_close(self, close_status_code, close_msg) -> None:
            if close_msg:
                err_holder["close"] = str(close_msg)
            loop.call_soon_threadsafe(done_evt.set)

        def on_event(self, response: dict) -> None:
            try:
                et = response.get("type")
                b64 = None
                if et == "response.audio.delta":
                    b64 = response.get("delta")
                elif et == "response.output_audio.delta":
                    b64 = response.get("delta")
                if b64:
                    loop.call_soon_threadsafe(audio_q.put_nowait, b64)
                elif et == "session.finished":
                    loop.call_soon_threadsafe(done_evt.set)
            except Exception:
                return

    async def pump_audio():
        while True:
            if done_evt.is_set() and audio_q.empty():
                return
            try:
                b64 = await asyncio.wait_for(audio_q.get(), timeout=0.2)
            except asyncio.TimeoutError:
                continue
            await websocket.send_json({"type": "audio", "pcm_b64": b64})

    # Wait for "start"
    msg = await websocket.receive_json()
    if msg.get("type") != "start":
        await websocket.send_json({"type": "error", "error": "Expected {type:'start'} first"})
        await websocket.close()
        return

    model = msg.get("model") or "qwen3-tts-flash-realtime"
    voice = msg.get("voice") or "Cherry"
    language_type = msg.get("language_type") or "Auto"

    # Start DashScope realtime client
    try:
        c = QwenTtsRealtime(
            model=model,
            callback=Callback(),
            url=dashscope_realtime_ws_base(settings),
        )
        client_holder["client"] = c
        c.connect()
        # Not all SDK versions/models accept language params. Try, then fallback.
        try:
            c.update_session(
                voice=voice,
                language_type=language_type,
                response_format=AudioFormat.PCM_24000HZ_MONO_16BIT,
                mode="commit",
            )
        except TypeError:
            c.update_session(
                voice=voice,
                response_format=AudioFormat.PCM_24000HZ_MONO_16BIT,
                mode="commit",
            )
    except Exception as e:
        await websocket.send_json({"type": "error", "error": str(e)})
        await websocket.close()
        return

    pump_task = asyncio.create_task(pump_audio())
    await websocket.send_json({"type": "ready"})

    try:
        while True:
            try:
                data = await websocket.receive_json()
            except WebSocketDisconnect:
                break

            t = data.get("type")
            if t == "text":
                txt = str(data.get("text") or "").strip()
                if not txt:
                    continue
                try:
                    c.append_text(txt)
                    c.commit()
                except Exception as e:
                    await websocket.send_json({"type": "error", "error": str(e)})
                    break
            elif t == "finish":
                try:
                    c.finish()
                except Exception:
                    pass
                break
    finally:
        done_evt.set()
        try:
            await pump_task
        except Exception:
            pass
        try:
            await websocket.close()
        except Exception:
            pass


@app.websocket("/ws/stt")
async def ws_stt(websocket: WebSocket):
    """
    Browser realtime ASR proxy (Qwen3 ASR Flash Realtime):
      - client sends JSON: {"type":"start","model":"qwen3-asr-flash-realtime","language":"ru","input_sample_rate":16000}
      - client sends JSON: {"type":"audio","pcm_b64":"..."} repeatedly (PCM s16le mono; preferred 16000Hz)
      - client sends JSON: {"type":"finish"}

    Server connects to DashScope OmniRealtime WS via DashScope SDK and forwards transcription events:
      {"type":"partial","text":"..."} / {"type":"final","text":"..."} / {"type":"event","raw":{...}}
    """
    await websocket.accept()

    settings = load_settings()
    if not settings.dashscope_api_key:
        await websocket.send_json({"type": "error", "error": "DASHSCOPE_API_KEY is not set"})
        await websocket.close()
        return

    try:
        import dashscope
        from dashscope.audio.qwen_omni import MultiModality, OmniRealtimeCallback, OmniRealtimeConversation
        from dashscope.audio.qwen_omni.omni_realtime import AudioFormat, TranscriptionParams
    except Exception as e:
        await websocket.send_json({"type": "error", "error": f"dashscope dependency missing: {e}"})
        await websocket.close()
        return

    dashscope.api_key = settings.dashscope_api_key
    loop = asyncio.get_running_loop()
    c: Any | None = None

    upstream_gone: dict[str, bool] = {"v": False}

    class AsrCallback(OmniRealtimeCallback):
        def on_open(self) -> None:
            return

        def on_close(self, close_status_code, close_msg) -> None:
            upstream_gone["v"] = True
            # Normal teardown uses 1000; still surface unexpected closes (auth, policy, etc.).
            if close_status_code == 1000 and not (close_msg or "").strip():
                return
            detail = f"dashscope realtime closed: code={close_status_code} msg={close_msg!r}"
            try:
                asyncio.run_coroutine_threadsafe(
                    websocket.send_json({"type": "error", "error": detail}),
                    loop,
                )
            except Exception:
                return

        def on_event(self, response: dict) -> None:
            try:
                et = response.get("type")
                if et == "conversation.item.input_audio_transcription.text":
                    stash = response.get("stash")
                    if stash:
                        asyncio.run_coroutine_threadsafe(
                            websocket.send_json({"type": "partial", "text": str(stash)}),
                            loop,
                        )
                elif et == "conversation.item.input_audio_transcription.completed":
                    tr = response.get("transcript")
                    if tr:
                        asyncio.run_coroutine_threadsafe(
                            websocket.send_json({"type": "final", "text": str(tr)}),
                            loop,
                        )
                elif et in ("error", "response.error", "session.error") or "error" in response:
                    asyncio.run_coroutine_threadsafe(
                        websocket.send_json({"type": "error", "error": str(response)}),
                        loop,
                    )
            except Exception:
                return

    # Wait for "start"
    msg = await websocket.receive_json()
    if msg.get("type") != "start":
        await websocket.send_json({"type": "error", "error": "Expected {type:'start'} first"})
        await websocket.close()
        return

    model = msg.get("model") or "qwen3-asr-flash-realtime"
    language = (msg.get("language") or "").strip() or None
    input_sample_rate = int(msg.get("input_sample_rate") or 16000)
    if input_sample_rate not in (8000, 16000):
        # Client should resample to 16k where possible; keep a safe default.
        input_sample_rate = 16000

    try:
        # Match Model Studio ASR realtime docs: format is "pcm" or "opus", not the enum alias "pcm16".
        # https://www.alibabacloud.com/help/en/model-studio/qwen-asr-realtime-python-sdk
        transcription_params = TranscriptionParams(
            language=language,
            sample_rate=input_sample_rate,
            input_audio_format="pcm",
        )

        c = OmniRealtimeConversation(
            model=model,
            callback=AsrCallback(),
            url=dashscope_realtime_ws_base(settings),
            api_key=settings.dashscope_api_key,
        )
        c.connect()
        c.update_session(
            output_modalities=[MultiModality.TEXT],
            input_audio_format=AudioFormat.PCM_16000HZ_MONO_16BIT,
            enable_turn_detection=True,
            turn_detection_type="server_vad",
            turn_detection_threshold=float(msg.get("turn_detection_threshold", 0.0)),
            turn_detection_silence_duration_ms=int(msg.get("turn_detection_silence_duration_ms", 400)),
            enable_input_audio_transcription=True,
            transcription_params=transcription_params,
        )
    except Exception as e:
        await websocket.send_json({"type": "error", "error": str(e)})
        await websocket.close()
        return

    await websocket.send_json({"type": "ready"})

    try:
        while True:
            try:
                data = await websocket.receive_json()
            except WebSocketDisconnect:
                break

            t = data.get("type")
            if t == "audio":
                b64 = data.get("pcm_b64")
                if not b64:
                    continue
                if upstream_gone["v"]:
                    await websocket.send_json(
                        {"type": "error", "error": "DashScope realtime connection already closed; check prior error or region (intl vs cn)."},
                    )
                    break
                try:
                    c.append_audio(str(b64))
                except Exception as e:
                    await websocket.send_json({"type": "error", "error": str(e)})
                    break
            elif t == "finish":
                break
            elif t == "event":
                # optional debug
                await websocket.send_json({"type": "event", "raw": data})
    finally:
        if c is not None:
            try:
                # Gracefully end ASR session (async variant; avoids blocking too long)
                c.end_session_async()
            except Exception:
                try:
                    c.close()
                except Exception:
                    pass
            await asyncio.sleep(0.2)
            try:
                c.close()
            except Exception:
                pass
        try:
            await websocket.close()
        except Exception:
            pass

