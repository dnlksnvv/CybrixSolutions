#!/usr/bin/env python3
"""
Потоковый синтез речи SaluteSpeech по gRPC (метод Synthesize → stream SynthesisResponse).

Это не «ultra-low-latency» TTS: сервер отдаёт чанки по мере генерации, но первый чанок
часто по размеру соответствует ~0.5–1 с готового PCM (см. лог: 48000 B ≈ 1 с при 24 kHz mono s16le).
Пока модель не выдала первый такой кусок, звука не будет — это норма API, не баг клиента.

Синхронный REST (salutespeech_tts.py) возвращает одним телом весь файл после полного синтеза:
для короткой фразы «всё сразу» может прийти так же быстро или быстрее по wall-clock;
для длинного текста gRPC даёт начать воспроизведение до конца всего синтеза.

Требуются сгенерированные stubs рядом со STT:
  scripts/_salutespeech_generated/synthesis_pb2.py
  scripts/_salutespeech_generated/synthesis_pb2_grpc.py
  (+ task_pb2.py — уже есть)

Пример:
  python salutespeech_tts_streaming.py -i --ca-bundle certs/russian_trusted_root_ca.pem
  # 2 gRPC channel, по 4 параллельных Synthesize на каждом (всего 8 RPC):
  python salutespeech_tts_streaming.py -i --grpc-channels 2 --streams-per-channel 4 \\
    --ca-bundle certs/russian_trusted_root_ca.pem
  # В -i следующая строка + Enter отменяет текущий синтез/воспроизведение и шлёт новый текст.
  python salutespeech_tts_streaming.py --text "Привет" --voice Nec_24000
  python salutespeech_tts_streaming.py --text "Hello" --encoding pcm \\
    --output /tmp/out.pcm --ca-bundle certs/russian_trusted_root_ca.pem
"""

from __future__ import annotations

import argparse
import base64
import contextlib
import importlib
import json
import os
import threading
import time
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path
from typing import Any, Callable

import grpc

OAUTH_URL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
GRPC_HOST = "smartspeech.sber.ru:443"
DEFAULT_SCOPE = "SALUTE_SPEECH_PERS"

# Готовый access_token от NGW (вставь свой; OAuth тогда не вызывается).
DEFAULT_ACCESS_TOKEN = ""

# Если токена нет: base64(client_id:client_secret) для OAuth (как в других скриптах Сбера).
DEFAULT_CREDENTIALS = "MDE5ZGMyMTUtZWU2OC03ODI0LTlmMjYtMzRkNjEyMDRiOThmOmY5MGM4ZWI2LWY3YTUtNGY1MS04ZDQ0LWFkYWIxZjFkZmY3Yg=="


def _normalize_credentials(raw: str) -> str:
    s = (raw or "").strip().rstrip("\"'")
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        s = s[1:-1].strip().rstrip("\"'")
    return s


def _normalize_bearer_token(raw: str) -> str:
    s = (raw or "").strip().rstrip("\"'")
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        s = s[1:-1].strip()
    if s.lower().startswith("bearer "):
        s = s[7:].strip()
    return s


def _require_existing_ca_bundle(ca_bundle: str) -> Path:
    """Проверка пути; понятная ошибка вместо сырого FileNotFoundError."""
    p = Path(ca_bundle).expanduser()
    if p.is_file():
        return p.resolve()
    default = Path(__file__).resolve().parent / "certs" / "russian_trusted_root_ca.pem"
    msg = (
        f"Не найден файл CA (--ca-bundle): {ca_bundle!r}. "
        f"Укажи реальный путь, не плейсхолдер из примера."
    )
    if default.is_file():
        msg += f"\n  Пример из этого репозитория: {default}"
    raise FileNotFoundError(msg)


def _ssl_context(no_verify: bool, ca_bundle: str = "") -> ssl.SSLContext:
    if no_verify:
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx
    if ca_bundle:
        cap = _require_existing_ca_bundle(ca_bundle)
        raw = cap.read_bytes()
        if b"-----BEGIN CERTIFICATE-----" in raw:
            return ssl.create_default_context(cafile=str(cap))
        b64 = base64.b64encode(raw)
        pem = (
            "-----BEGIN CERTIFICATE-----\n"
            + "\n".join(b64[i : i + 64].decode("ascii") for i in range(0, len(b64), 64))
            + "\n-----END CERTIFICATE-----\n"
        )
        return ssl.create_default_context(cadata=pem)
    return ssl.create_default_context()


def _oauth_token(credentials: str, scope: str, ssl_context: ssl.SSLContext) -> str:
    form = urllib.parse.urlencode({"scope": scope}).encode("utf-8")
    req = urllib.request.Request(OAUTH_URL, data=form, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    req.add_header("Accept", "application/json")
    req.add_header("RqUID", str(uuid.uuid4()))
    req.add_header("Authorization", f"Basic {credentials}")
    try:
        with urllib.request.urlopen(req, timeout=60.0, context=ssl_context) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"OAuth failed ({e.code}): {body}") from e
    token = data.get("access_token")
    if not token:
        raise RuntimeError(f"OAuth response has no access_token: {data}")
    return token


def _load_pb2() -> tuple[Any, Any]:
    generated = Path(__file__).resolve().parent / "_salutespeech_generated"
    for name in ("synthesis_pb2.py", "synthesis_pb2_grpc.py"):
        if not (generated / name).exists():
            raise RuntimeError(
                f"Не найден {name} в {generated}.\n"
                "Сгенерируйте из synthesis.proto (см. комментарий в начале файла)."
            )
    sys.path.insert(0, str(generated))
    return importlib.import_module("synthesis_pb2"), importlib.import_module(
        "synthesis_pb2_grpc"
    )


def _sample_rate_from_voice(voice: str) -> int:
    """Nec_24000 → 24000; иначе 24000 по умолчанию."""
    parts = voice.rsplit("_", 1)
    if len(parts) == 2 and parts[1].isdigit():
        return int(parts[1])
    return 24000


def _encoding_enum(pb2: Any, name: str) -> int:
    n = name.strip().lower()
    if n in ("pcm", "pcm16", "pcm_s16le", "linear16"):
        return pb2.SynthesisRequest.PCM_S16LE
    if n in ("wav", "wav16"):
        return pb2.SynthesisRequest.WAV
    if n in ("opus", "ogg"):
        return pb2.SynthesisRequest.OPUS
    raise ValueError(f"unknown encoding: {name} (use pcm, wav, opus)")


def _content_enum(pb2: Any, name: str) -> int:
    n = name.strip().lower()
    if n == "text":
        return pb2.SynthesisRequest.TEXT
    if n == "ssml":
        return pb2.SynthesisRequest.SSML
    raise ValueError(f"unknown content_type: {name} (use text or ssml)")


def _synthesize_stream(
    stub: Any,
    metadata: tuple[tuple[str, str], ...],
    pb2: Any,
    *,
    text: str,
    voice: str,
    language: str,
    encoding_name: str,
    content_type_name: str,
    output_path: str,
    want_play: bool,
    log_each_chunk: bool,
    on_first_chunk: Callable[[float], None] | None = None,
    latency_anchor: float | None = None,
    cancel_event: threading.Event | None = None,
    grpc_call_slot: list[Any] | None = None,
    call_slot_lock: threading.Lock | None = None,
    pcm_buffer: bytearray | None = None,
    on_grpc_call_started: Callable[[Any], None] | None = None,
) -> tuple[int, int, float | None, float, bool]:
    """
    Один вызов Synthesize. on_first_chunk(dt_sec) — при первом непустом data;
    dt от latency_anchor (если задан, напр. сразу после Enter) или от входа в функцию.

    cancel_event: если установлен, цикл прерывается и вызывается call.cancel().
    grpc_call_slot: сюда кладётся объект вызова для отмены с другого потока (длина 0 или 1).
    Возвращает (…, was_cancelled).
    """
    req = pb2.SynthesisRequest(
        text=text,
        audio_encoding=_encoding_enum(pb2, encoding_name),
        language=language,
        content_type=_content_enum(pb2, content_type_name),
        voice=voice,
        rebuild_cache=False,
    )

    out_f = open(output_path, "wb") if output_path else None
    play_stream = None
    if want_play:
        import sounddevice as sd

        sr = _sample_rate_from_voice(voice)
        play_stream = sd.RawOutputStream(
            samplerate=sr,
            channels=1,
            dtype="int16",
        )
        play_stream.start()

    anchor = latency_anchor if latency_anchor is not None else time.perf_counter()
    t_stream = time.perf_counter()
    n_chunks = 0
    total_bytes = 0
    t_first: float | None = None
    was_cancelled = False
    call = stub.Synthesize(req, metadata=metadata)
    if on_grpc_call_started is not None:
        on_grpc_call_started(call)
    if grpc_call_slot is not None:
        if call_slot_lock:
            with call_slot_lock:
                grpc_call_slot.clear()
                grpc_call_slot.append(call)
        else:
            grpc_call_slot.clear()
            grpc_call_slot.append(call)
    try:
        for resp in call:
            if cancel_event is not None and cancel_event.is_set():
                with contextlib.suppress(Exception):
                    call.cancel()
                was_cancelled = True
                break
            data = resp.data or b""
            if not data:
                continue
            if t_first is None:
                t_first = time.perf_counter() - anchor
                if on_first_chunk:
                    on_first_chunk(t_first)
            n_chunks += 1
            total_bytes += len(data)
            if out_f:
                out_f.write(data)
            if pcm_buffer is not None:
                pcm_buffer.extend(data)
            if play_stream:
                play_stream.write(data)
            if log_each_chunk:
                dur = ""
                if resp.HasField("audio_duration"):
                    d = resp.audio_duration
                    dur = f" duration={d.seconds}.{d.nanos:09d}s"
                print(f"chunk #{n_chunks} +{len(data)} B{dur}", file=sys.stderr)
    except grpc.RpcError as e:
        if e.code() == grpc.StatusCode.CANCELLED and (
            cancel_event is not None and cancel_event.is_set()
        ):
            was_cancelled = True
        else:
            raise
    finally:
        if grpc_call_slot is not None:
            if call_slot_lock:
                with call_slot_lock:
                    if grpc_call_slot and grpc_call_slot[0] is call:
                        grpc_call_slot.clear()
            else:
                if grpc_call_slot and grpc_call_slot[0] is call:
                    grpc_call_slot.clear()
        if play_stream:
            with contextlib.suppress(Exception):
                play_stream.stop()
                play_stream.close()
        if out_f:
            out_f.close()

    elapsed = time.perf_counter() - t_stream
    return n_chunks, total_bytes, t_first, elapsed, was_cancelled


def _run_interactive(
    stubs: list[Any],
    metadata: tuple[tuple[str, str], ...],
    pb2: Any,
    *,
    voice: str,
    language: str,
    encoding_name: str,
    content_type_name: str,
    no_play: bool,
    verbose_chunks: bool,
    parallel: int,
    streams_per_channel: int,
) -> None:
    want_play = not no_play and encoding_name == "pcm"
    if not no_play and encoding_name != "pcm":
        print(
            "Интерактив: озвучка только при --encoding pcm; сейчас без воспроизведения.",
            file=sys.stderr,
        )

    print_lock = threading.Lock()
    active_calls: list[Any] = []
    calls_lock = threading.Lock()
    active_workers: list[threading.Thread] = []
    workers_lock = threading.Lock()
    active_cancel: threading.Event | None = None
    k = max(1, streams_per_channel)
    n_ch = max(1, len(stubs))

    def _register_call(call: Any) -> None:
        with calls_lock:
            active_calls.append(call)

    def _unregister_call(call: Any) -> None:
        with calls_lock:
            with contextlib.suppress(ValueError):
                active_calls.remove(call)

    def cancel_in_flight() -> None:
        if active_cancel is not None:
            active_cancel.set()
        with calls_lock:
            calls = list(active_calls)
        for c in calls:
            with contextlib.suppress(Exception):
                c.cancel()

    print(
        "Режим чата: строка + Enter → синтез и воспроизведение. "
        "Новая строка отменяет текущий запрос и воспроизведение. "
        "Выход: q или /q или /quit + Enter.\n"
        f"Голос: {voice}, encoding: {encoding_name}"
        + (
            f", gRPC: {n_ch} channel × {k} стрим(ов) = {parallel} Synthesize"
            if parallel > 1
            else ""
        ),
        flush=True,
    )
    if parallel > 1:
        print(
            "Подсказка: лимит «N потоков» у провайдера обычно про N одновременных RPC, "
            "а не про число TCP/gRPC channel. Один channel, много Synthesize — всё равно N запросов.",
            flush=True,
        )
        if want_play:
            print(
                "Каждый поток — свой вывод в динамик по мере чанков; звук наложится (микширование обычно делает ОС). "
                "Может быть громко или с артефактами.",
                flush=True,
            )

    def run_utterance(
        text: str,
        job_cancel: threading.Event,
        t_after_enter: float,
        slot_index: int,
    ) -> None:
        def _on_first(dt: float) -> None:
            with print_lock:
                ch_i = slot_index // k
                ch_tag = f"ch{ch_i} " if len(stubs) > 1 else ""
                label = f"#{slot_index} " if parallel > 1 else ""
                print(f"{label}{ch_tag}до первого чанка: {dt:.3f} с", flush=True)

        current_call: list[Any | None] = [None]

        def _note_call(c: Any) -> None:
            current_call[0] = c
            _register_call(c)

        use_play_stream = want_play
        ch_i = min(slot_index // k, len(stubs) - 1)
        stub = stubs[ch_i]
        n_chunks = 0
        total_bytes = 0
        t_first: float | None = None
        elapsed = 0.0
        was_cancelled = False
        try:
            n_chunks, total_bytes, t_first, elapsed, was_cancelled = _synthesize_stream(
                stub,
                metadata,
                pb2,
                text=text,
                voice=voice,
                language=language,
                encoding_name=encoding_name,
                content_type_name=content_type_name,
                output_path="",
                want_play=use_play_stream,
                log_each_chunk=verbose_chunks,
                on_first_chunk=_on_first,
                latency_anchor=t_after_enter,
                cancel_event=job_cancel,
                grpc_call_slot=None,
                call_slot_lock=None,
                pcm_buffer=None,
                on_grpc_call_started=_note_call,
            )
        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.CANCELLED and job_cancel.is_set():
                with print_lock:
                    print(
                        f"#{slot_index} отменено." if parallel > 1 else "отменено.",
                        flush=True,
                    )
                return
            with print_lock:
                ch_err = f"ch{slot_index // k} " if len(stubs) > 1 else ""
                msg = (
                    f"gRPC error #{slot_index} {ch_err}{e.code()} {e.details()}"
                    if parallel > 1
                    else f"gRPC error: {e.code()} {e.details()}"
                )
                if e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
                    msg += (
                        " — обычно лимит одновременных RPC на стороне SaluteSpeech; "
                        "уменьши --grpc-channels и/или --streams-per-channel."
                    )
                print(msg, file=sys.stderr)
            return
        finally:
            c = current_call[0]
            if c is not None:
                _unregister_call(c)
                current_call[0] = None

        wall = time.perf_counter() - t_after_enter
        extra = f" (от Enter до конца стрима: {wall:.2f} с)" if t_first is not None else ""
        with print_lock:
            prefix = f"#{slot_index} " if parallel > 1 else ""
            chpfx = f"ch{slot_index // k} " if len(stubs) > 1 else ""
            if was_cancelled:
                print(
                    f"{prefix}{chpfx}отменено: чанков={n_chunks}, байт={total_bytes}, стрим {elapsed:.2f} с{extra}",
                    flush=True,
                )
            else:
                print(
                    f"{prefix}{chpfx}готово: чанков={n_chunks}, байт={total_bytes}, стрим {elapsed:.2f} с{extra}",
                    flush=True,
                )

    while True:
        try:
            line = input("🎙 > ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            cancel_in_flight()
            with workers_lock:
                for w in active_workers:
                    w.join(timeout=2.0)
            break
        if not line:
            continue
        low = line.lower()
        if low in ("q", "/q", "/quit", "/exit"):
            cancel_in_flight()
            with workers_lock:
                for w in active_workers:
                    w.join(timeout=2.0)
            break

        cancel_in_flight()
        with workers_lock:
            for w in active_workers:
                w.join(timeout=2.0)
            active_workers.clear()
        with calls_lock:
            active_calls.clear()

        t_after_enter = time.perf_counter()
        job_cancel = threading.Event()
        active_cancel = job_cancel

        n_parallel = max(1, parallel)
        for slot in range(n_parallel):
            worker = threading.Thread(
                target=run_utterance,
                args=(line, job_cancel, t_after_enter, slot),
                daemon=True,
            )
            with workers_lock:
                active_workers.append(worker)
            worker.start()


def main() -> int:
    parser = argparse.ArgumentParser(description="SaluteSpeech streaming TTS (gRPC)")
    parser.add_argument(
        "--access-token",
        default="",
        help="готовый Bearer access_token (иначе OAuth по credentials)",
    )
    parser.add_argument("--credentials", default="", help="Basic для OAuth; иначе SALUTESPEECH_CREDENTIALS / DEFAULT_CREDENTIALS")
    parser.add_argument("--scope", default=DEFAULT_SCOPE)
    parser.add_argument(
        "--text",
        default="",
        help="одна фраза без интерактива (см. -i)",
    )
    parser.add_argument(
        "-i",
        "--interactive",
        action="store_true",
        help="чат: каждая строка после Enter — синтез и воспроизведение",
    )
    parser.add_argument(
        "--grpc-channels",
        type=int,
        default=1,
        metavar="M",
        help="только с -i: M отдельных gRPC secure_channel (TCP). "
        "Всего параллельных Synthesize: M × K (см. --streams-per-channel). По умолчанию 1",
    )
    parser.add_argument(
        "--streams-per-channel",
        type=int,
        default=1,
        metavar="K",
        help="только с -i: на каждом channel одновременно K вызовов Synthesize; "
        "слоты 0…K−1 → ch0, K…2K−1 → ch1, … Всего M×K. По умолчанию 1",
    )
    parser.add_argument(
        "--verbose-chunks",
        action="store_true",
        help="логировать каждый аудио-чанк (по умолчанию только сводка)",
    )
    parser.add_argument("--voice", default="Bys_24000")
    parser.add_argument("--language", default="ru-RU")
    parser.add_argument(
        "--encoding",
        default="pcm",
        choices=("pcm", "wav", "opus"),
        help="pcm = PCM_S16LE, wav = WAV с заголовком, opus = OGG/Opus",
    )
    parser.add_argument("--content-type", default="text", choices=("text", "ssml"))
    parser.add_argument("--output", default="", help="файл для сырых чанков (дописываются по мере стрима)")
    parser.add_argument(
        "--no-play",
        action="store_true",
        help="не воспроизводить в динамик (только запись в --output или тихий режим)",
    )
    parser.add_argument("--no-ssl-verify", action="store_true")
    parser.add_argument(
        "--ca-bundle",
        default=os.environ.get("SALUTESPEECH_CA_BUNDLE_FILE", ""),
    )
    parser.add_argument(
        "--connect-timeout-sec",
        type=float,
        default=8.0,
        help="таймаут принудительного подключения gRPC при старте",
    )
    args = parser.parse_args()

    if not args.interactive and not args.text.strip():
        print("Укажи --text «фраза» или запусти с -i (интерактив).", file=sys.stderr)
        return 1

    access_token = _normalize_bearer_token(
        args.access_token
        or os.environ.get("SALUTESPEECH_ACCESS_TOKEN", "")
        or DEFAULT_ACCESS_TOKEN
    )
    creds = _normalize_credentials(
        args.credentials or os.environ.get("SALUTESPEECH_CREDENTIALS", "") or DEFAULT_CREDENTIALS
    )

    try:
        pb2, pb2_grpc = _load_pb2()
    except Exception as e:
        print(f"Proto load error: {e}", file=sys.stderr)
        return 1

    if access_token:
        token = access_token
    elif creds:
        ssl_ctx = _ssl_context(args.no_ssl_verify, args.ca_bundle)
        try:
            token = _oauth_token(creds, args.scope, ssl_ctx)
        except Exception as e:
            print(f"OAuth error: {e}", file=sys.stderr)
            return 1
    else:
        print(
            "Нужен access_token (--access-token / SALUTESPEECH_ACCESS_TOKEN / DEFAULT_ACCESS_TOKEN) "
            "или credentials для OAuth (--credentials / SALUTESPEECH_CREDENTIALS / DEFAULT_CREDENTIALS).",
            file=sys.stderr,
        )
        return 1

    root_certs: bytes | None = None
    if args.ca_bundle:
        try:
            cap = _require_existing_ca_bundle(args.ca_bundle)
            raw = cap.read_bytes()
            if b"-----BEGIN CERTIFICATE-----" in raw:
                root_certs = raw
            else:
                b64 = base64.b64encode(raw)
                pem = (
                    "-----BEGIN CERTIFICATE-----\n"
                    + "\n".join(b64[i : i + 64].decode("ascii") for i in range(0, len(b64), 64))
                    + "\n-----END CERTIFICATE-----\n"
                )
                root_certs = pem.encode("utf-8")
        except Exception as e:
            print(f"Не удалось прочитать --ca-bundle: {e}", file=sys.stderr)
            return 1

    if args.no_ssl_verify and not root_certs:
        print(
            "Внимание: --no-ssl-verify не отключает TLS для gRPC; без --ca-bundle используются системные корни.",
            file=sys.stderr,
        )

    n_grpc_channels = max(1, args.grpc_channels) if args.interactive else 1
    if not args.interactive and (args.grpc_channels > 1 or args.streams_per_channel > 1):
        print(
            "Подсказка: --grpc-channels / --streams-per-channel только с -i; для --text одно подключение.",
            file=sys.stderr,
        )

    channels: list[Any] = []
    try:
        creds_tls = grpc.ssl_channel_credentials(root_certificates=root_certs)
        for _ in range(n_grpc_channels):
            ch = grpc.secure_channel(GRPC_HOST, creds_tls)
            try:
                grpc.channel_ready_future(ch).result(timeout=args.connect_timeout_sec)
            except grpc.FutureTimeoutError:
                print(
                    f"gRPC connect timeout after {args.connect_timeout_sec:.1f}s "
                    f"(host={GRPC_HOST}, channel {len(channels) + 1}/{n_grpc_channels})",
                    file=sys.stderr,
                )
                ch.close()
                for c in channels:
                    with contextlib.suppress(Exception):
                        c.close()
                return 1
            channels.append(ch)

        stubs = [pb2_grpc.SmartSpeechStub(c) for c in channels]
        stub = stubs[0]
        metadata = (("authorization", f"Bearer {token}"),)

        want_play = not args.no_play and args.encoding == "pcm"
        if not args.no_play and args.encoding != "pcm":
            print(
                "Озвучка в реальном времени только для --encoding pcm; для wav/opus укажи --no-play.",
                file=sys.stderr,
            )

        if args.interactive:
            M = max(1, args.grpc_channels)
            K = max(1, args.streams_per_channel)
            parallel = M * K
            _run_interactive(
                stubs,
                metadata,
                pb2,
                voice=args.voice,
                language=args.language,
                encoding_name=args.encoding,
                content_type_name=args.content_type,
                no_play=args.no_play,
                verbose_chunks=args.verbose_chunks,
                parallel=parallel,
                streams_per_channel=K,
            )
            return 0

        t_anchor = time.perf_counter()

        def _on_first(dt: float) -> None:
            print(f"до первого чанка: {dt:.3f} с", file=sys.stderr)

        try:
            n_chunks, total_bytes, t_first, elapsed, _cancelled = _synthesize_stream(
                stub,
                metadata,
                pb2,
                text=args.text,
                voice=args.voice,
                language=args.language,
                encoding_name=args.encoding,
                content_type_name=args.content_type,
                output_path=args.output,
                want_play=want_play,
                log_each_chunk=args.verbose_chunks,
                on_first_chunk=_on_first,
                latency_anchor=t_anchor,
            )
        except grpc.RpcError as e:
            print(f"gRPC error: {e.code()} {e.details()}", file=sys.stderr)
            return 1

        first_s = f", до 1-го чанка {t_first:.2f}с" if t_first is not None else ""
        print(
            f"Готово: чанков={n_chunks}, байт={total_bytes}, стрим {elapsed:.2f}с{first_s}",
            file=sys.stderr,
        )
        if args.output and args.encoding == "pcm":
            sr = _sample_rate_from_voice(args.voice)
            print(
                f"PCM mono s16le {sr} Hz → можно: ffplay -f s16le -ar {sr} -ac 1 {args.output}",
                file=sys.stderr,
            )
        return 0
    finally:
        for ch in channels:
            with contextlib.suppress(Exception):
                ch.close()


if __name__ == "__main__":
    raise SystemExit(main())
