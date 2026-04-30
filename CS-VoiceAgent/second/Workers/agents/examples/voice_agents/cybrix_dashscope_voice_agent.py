"""
Cybrix voice agent: STT + LLM + TTS through the local ``inference-gateway``.

Workers know nothing about DashScope/Sber URLs or auth — that lives in the
gateway's own ``.env``. Here we only:

- pick model ids / voice / language for each modality (env-driven),
- wire LiveKit ``AgentSession`` behaviour (preemptive generation, interruption,
  TTS passthrough vs sentence-buffering).

Prereqs (from repo root ``agents/``)::

    make cybrix-dashscope-dev
    # or: uv sync --dev --group cybrix && \\
    #     uv run --python 3.12 python examples/voice_agents/cybrix_dashscope_voice_agent.py dev

Required env (see ``agents/.env``)::

    LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET
    INFERENCE_GATEWAY_URL                — http://host:port of the gateway
    LLM_MODEL                            — id from gateway's LLM registry
    STT_MODEL, STT_LANGUAGE              — id from gateway's STT registry
    TTS_MODEL                            — id from gateway's TTS registry

Optional::

    TTS_VOICE, TTS_LANGUAGE_TYPE, TTS_SAMPLE_RATE, TTS_MODE  (commit | server_commit)
    STT_SAMPLE_RATE
    ALIYUN_PREEMPTIVE_GENERATION, ALIYUN_PREEMPTIVE_TTS, ALIYUN_TTS_PASSTHROUGH
    TTS_GATE_GENERATION_BUDGET_MS, TTS_GATE_EXTRA_DELAY_MS  (gated TTS: budget + fixed extra delay)
    PREEMPTIVE_TRANSCRIPT_NORMALIZE            — fuzzy match preflight vs final when reusing preemptive LLM
    PREEMPTIVE_TRANSCRIPT_MATCH_LOG            — INFO logs: raw vs normalized strings and reuse breakdown
    PREEMPTIVE_GENERATION_HOOK_LOG             — INFO logs: why preemptive LLM did not start (or START ok)
    CYBRIX_TRACE_URL
"""

from __future__ import annotations

import asyncio
import logging
import sys
from pathlib import Path

from dotenv import load_dotenv

_AGENTS_ROOT = Path(__file__).resolve().parents[2]
_CUSTOM = _AGENTS_ROOT / "custom-plugins"
if str(_CUSTOM) not in sys.path:
    sys.path.insert(0, str(_CUSTOM))

load_dotenv(_AGENTS_ROOT / ".env")
load_dotenv()

from inference_gateway import (  # noqa: E402
    GatewayLLM,
    GatewaySTT,
    GatewayTTS,
    load_settings,
)
from trace_emitter import emit, init_from_env  # noqa: E402
from tracing_llm import TracingLLM  # noqa: E402

from livekit.agents import (  # noqa: E402
    Agent,
    AgentServer,
    AgentSession,
    JobContext,
    JobProcess,
    TurnHandlingOptions,
    cli,
    room_io,
)
from livekit.plugins import silero  # noqa: E402

logger = logging.getLogger("cybrix-voice-agent")


class CybrixVoiceAgent(Agent):
    def __init__(self) -> None:
        super().__init__(
            instructions=(
                "You are a helpful voice assistant. Reply concisely in natural spoken language. "
                "No markdown, lists, or emojis. If the user speaks Russian, answer in Russian. "
                "Start every reply with one short sentence or a single word ending in ., ?, or ! "
                "that fits the situation and matches the tone of what follows; it must not feel "
                "inconsistent or tacked-on compared to the rest of your answer. "
                "If the user asks who you are, what you are, or your name, answer that you are "
                "«пожилая бибинта» (use that wording when replying in Russian)."
            ),
        )


server = AgentServer()


def prewarm(proc: JobProcess) -> None:
    proc.userdata["vad"] = silero.VAD.load()


server.setup_fnc = prewarm


@server.rtc_session()
async def entrypoint(ctx: JobContext) -> None:
    ctx.log_context_fields = {"room": ctx.room.name}
    logger.info("Cybrix voice agent room=%s", ctx.room.name)
    init_from_env()

    settings = load_settings()

    llm = TracingLLM(GatewayLLM(base_url=settings.base_url, model=settings.llm_model))

    stt = GatewaySTT(
        ws_url=settings.ws_stt_url,
        model=settings.stt_model,
        language=settings.stt_language,
        sample_rate=settings.stt_sample_rate,
    )

    tts = GatewayTTS(
        ws_url=settings.ws_tts_url,
        model=settings.tts_model,
        voice=settings.tts_voice,
        language_type=settings.tts_language_type,
        sample_rate=settings.tts_sample_rate,
        mode=settings.tts_mode,
        passthrough=settings.tts_passthrough,
        gate_generation_budget_ms=settings.tts_gate_generation_budget_ms,
        gate_extra_delay_ms=settings.tts_gate_extra_delay_ms,
    )

    asyncio.create_task(llm.warmup())
    asyncio.create_task(stt.warmup())
    asyncio.create_task(tts.warmup())

    emit(
        "worker.config",
        room=ctx.room.name,
        gateway_url=settings.base_url,
        llm_model=settings.llm_model,
        stt_model=settings.stt_model,
        stt_language=settings.stt_language,
        tts_model=settings.tts_model,
        tts_voice=settings.tts_voice,
        tts_language_type=settings.tts_language_type,
        tts_passthrough=settings.tts_passthrough,
    )
    logger.info(
        "Cybrix stack via gateway=%s llm=%s stt=%s lang=%s tts=%s voice=%s passthrough=%s "
        "preemptive_gen=%s preemptive_tts=%s tts_gate_budget_ms=%s tts_gate_extra_ms=%s",
        settings.base_url,
        settings.llm_model,
        settings.stt_model,
        settings.stt_language,
        settings.tts_model,
        settings.tts_voice or "(env)",
        settings.tts_passthrough,
        settings.preemptive_generation,
        settings.preemptive_tts,
        settings.tts_gate_generation_budget_ms,
        settings.tts_gate_extra_delay_ms,
    )

    session = AgentSession(
        vad=ctx.proc.userdata["vad"],
        stt=stt,
        llm=llm,
        tts=tts,
        turn_handling=TurnHandlingOptions(
            turn_detection="stt",
            interruption={
                # Не возобновлять старую реплику после прерывания — иначе агент договаривает
                # предыдущий ответ перед новым ("он продолжает говорить старую реплику").
                "resume_false_interruption": False,
                "false_interruption_timeout": 0.3,
                # Не выбрасывать пользовательский звук, пока агент говорит — даже если interrupt
                # сейчас запрещён, ASR всё равно должен слышать пользователя, иначе теряется
                # начало фразы ("транскрибирует частично неправильно").
                "discard_audio_if_uninterruptible": False,
                # Чтобы случайный кашель/шум не прерывал агента (актуально при stt-turn-detection).
                "min_duration": 0.4,
                "min_words": 2,
            },
            preemptive_generation={
                "enabled": settings.preemptive_generation,
                "preemptive_tts": settings.preemptive_tts,
            },
        ),
        # 0 — пользователь в наушниках, эха нет, AEC-прогрев не нужен. Любая ненулевая длительность
        # приводит к тому, что первые секунды реплики агента LiveKit игнорирует прерывания и
        # (по-умолчанию) выкидывает пользовательский звук — портит ASR на оверлапе.
        aec_warmup_duration=0.0,
    )

    await session.start(
        agent=CybrixVoiceAgent(),
        room=ctx.room,
        room_options=room_io.RoomOptions(),
    )


if __name__ == "__main__":
    cli.run_app(server)
