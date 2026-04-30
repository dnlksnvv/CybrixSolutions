#!/usr/bin/env python3
"""
Параллельно: N streaming Recognize (WAV → несколько очередей) + M streaming Synthesize.

Схема как в salutespeech_stt_realtime / salutespeech_tts_streaming:
  --stt-grpc-channels × --stt-streams-per-channel = число Recognize
  --tts-grpc-channels × --tts-streams-per-channel = число Synthesize

Пример: 5 Recognize + 5 Synthesize (10 gRPC channel к одному хосту: 5 на STT + 5 на TTS):
  python3 salutespeech_stt_tts_parallel_demo.py \\
    --stt-grpc-channels 5 --stt-streams-per-channel 1 \\
    --tts-grpc-channels 5 --tts-streams-per-channel 1 \\
    --wav ../out.wav --ca-bundle certs/russian_trusted_root_ca.pem

Один WAV дублируется во все STT-очереди; запросы уходят параллельно после старта воркеров.
"""

from __future__ import annotations

import argparse
import array
import base64
import contextlib
import importlib
import json
import os
import queue
import ssl
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import wave
from pathlib import Path
from typing import Any, Iterator

import grpc
from google.protobuf.duration_pb2 import Duration

OAUTH_URL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
GRPC_HOST = "smartspeech.sber.ru:443"
DEFAULT_SCOPE = "SALUTE_SPEECH_PERS"
DEFAULT_CREDENTIALS = "MDE5ZGMyMTUtZWU2OC03ODI0LTlmMjYtMzRkNjEyMDRiOThmOmY5MGM4ZWI2LWY3YTUtNGY1MS04ZDQ0LWFkYWIxZjFkZmY3Yg=="


def _normalize_credentials(raw: str) -> str:
    s = (raw or "").strip().rstrip("\"'")
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        s = s[1:-1].strip().rstrip("\"'")
    return s


def _oauth_ssl_context(no_verify: bool, ca_bundle: str = "") -> ssl.SSLContext:
    if no_verify:
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx
    if ca_bundle:
        raw = Path(ca_bundle).read_bytes()
        if b"-----BEGIN CERTIFICATE-----" in raw:
            return ssl.create_default_context(cafile=ca_bundle)
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


def _duration(sec: float) -> Duration:
    d = Duration()
    d.FromMilliseconds(int(sec * 1000))
    return d


def _load_recognition() -> tuple[Any, Any]:
    generated = Path(__file__).resolve().parent / "_salutespeech_generated"
    for name in ("recognition_pb2.py", "recognition_pb2_grpc.py"):
        if not (generated / name).exists():
            raise RuntimeError(f"Нет {name} в {generated}")
    sys.path.insert(0, str(generated))
    return importlib.import_module("recognition_pb2"), importlib.import_module(
        "recognition_pb2_grpc"
    )


def _load_synthesis() -> tuple[Any, Any]:
    generated = Path(__file__).resolve().parent / "_salutespeech_generated"
    for name in ("synthesis_pb2.py", "synthesis_pb2_grpc.py"):
        if not (generated / name).exists():
            raise RuntimeError(f"Нет {name} в {generated}")
    sys.path.insert(0, str(generated))
    return importlib.import_module("synthesis_pb2"), importlib.import_module(
        "synthesis_pb2_grpc"
    )


def _wav_to_mono_s16le(path: Path) -> tuple[bytes, int]:
    with wave.open(str(path), "rb") as w:
        nch = w.getnchannels()
        sw = w.getsampwidth()
        fr = w.getframerate()
        nframes = w.getnframes()
        raw = w.readframes(nframes)
    if sw != 2:
        raise ValueError(f"Нужен WAV 16-bit linear PCM, sampwidth={sw}")
    samples = array.array("h")
    samples.frombytes(raw)
    if nch == 2:
        mono = array.array("h", ((samples[i] + samples[i + 1]) // 2 for i in range(0, len(samples), 2)))
        raw = mono.tobytes()
    elif nch != 1:
        raise ValueError(f"Нужен mono или stereo WAV, channels={nch}")
    else:
        raw = samples.tobytes()
    return raw, fr


def _recognize_queue_iter(
    pb2: Any,
    audio_q: "queue.Queue[bytes | None]",
    *,
    sample_rate: int,
    language: str,
    eou_timeout: float,
) -> Iterator[Any]:
    opts = pb2.RecognitionOptions(
        audio_encoding=pb2.RecognitionOptions.PCM_S16LE,
        sample_rate=sample_rate,
        language=language,
        channels_count=1,
        hypotheses_count=1,
        enable_partial_results=True,
        enable_multi_utterance=False,
        no_speech_timeout=_duration(7.0),
        max_speech_timeout=_duration(60.0),
        hints=pb2.Hints(eou_timeout=_duration(eou_timeout)),
    )
    yield pb2.RecognitionRequest(options=opts)
    while True:
        chunk = audio_q.get()
        if chunk is None:
            return
        yield pb2.RecognitionRequest(audio_chunk=chunk)


def _sample_rate_from_voice(voice: str) -> int:
    parts = voice.rsplit("_", 1)
    if len(parts) == 2 and parts[1].isdigit():
        return int(parts[1])
    return 24000


def _open_channels(
    n: int, root_certs: bytes | None, connect_timeout: float
) -> list[Any]:
    out: list[Any] = []
    creds_tls = grpc.ssl_channel_credentials(root_certificates=root_certs)
    for i in range(n):
        ch = grpc.secure_channel(GRPC_HOST, creds_tls)
        try:
            grpc.channel_ready_future(ch).result(timeout=connect_timeout)
        except grpc.FutureTimeoutError:
            ch.close()
            for c in out:
                with contextlib.suppress(Exception):
                    c.close()
            raise
        out.append(ch)
    return out


def run_stt_worker(
    slot: int,
    stub: Any,
    rec_pb2: Any,
    audio_q: "queue.Queue[bytes | None]",
    metadata: tuple[tuple[str, str], ...],
    *,
    sample_rate: int,
    language: str,
    eou_timeout: float,
    stt_streams_per_ch: int,
    stt_n_ch: int,
    print_lock: threading.Lock,
    errors: list[BaseException],
) -> None:
    k = max(1, stt_streams_per_ch)
    ch_i = min(slot // k, stt_n_ch - 1)
    prefix = f"[STT #{slot} ch{ch_i}] "
    try:
        for resp in stub.Recognize(
            _recognize_queue_iter(
                rec_pb2,
                audio_q,
                sample_rate=sample_rate,
                language=language,
                eou_timeout=eou_timeout,
            ),
            metadata=metadata,
        ):
            text = ""
            if resp.results:
                best = resp.results[0]
                text = best.normalized_text or best.text
            if text.strip():
                label = "FINAL" if resp.eou else "PART"
                with print_lock:
                    print(f"{prefix}[{label}] {text}", flush=True)
    except grpc.RpcError as e:
        with print_lock:
            msg = f"{prefix}gRPC: {e.code()} {e.details()}"
            if e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
                msg += " — лимит RPC; уменьши STT M×K"
            print(msg, file=sys.stderr, flush=True)
        errors.append(e)
    except BaseException as e:
        errors.append(e)
        with print_lock:
            print(f"{prefix}ошибка: {e}", file=sys.stderr, flush=True)


def run_tts_worker(
    slot: int,
    stub: Any,
    syn_pb2: Any,
    text: str,
    metadata: tuple[tuple[str, str], ...],
    *,
    voice: str,
    language: str,
    no_play: bool,
    tts_streams_per_ch: int,
    tts_n_ch: int,
    print_lock: threading.Lock,
    errors: list[BaseException],
) -> None:
    k = max(1, tts_streams_per_ch)
    ch_i = min(slot // k, tts_n_ch - 1)
    prefix = f"[TTS #{slot} ch{ch_i}] "
    try:
        req = syn_pb2.SynthesisRequest(
            text=text,
            audio_encoding=syn_pb2.SynthesisRequest.PCM_S16LE,
            language=language,
            content_type=syn_pb2.SynthesisRequest.TEXT,
            voice=voice,
            rebuild_cache=False,
        )
        play_stream = None
        if not no_play:
            import sounddevice as sd

            sr = _sample_rate_from_voice(voice)
            play_stream = sd.RawOutputStream(samplerate=sr, channels=1, dtype="int16")
            play_stream.start()
        n_chunks = 0
        nbytes = 0
        call = stub.Synthesize(req, metadata=metadata)
        try:
            for resp in call:
                data = resp.data or b""
                if not data:
                    continue
                n_chunks += 1
                nbytes += len(data)
                if play_stream:
                    play_stream.write(data)
        finally:
            if play_stream:
                with contextlib.suppress(Exception):
                    play_stream.stop()
                    play_stream.close()
        with print_lock:
            print(f"{prefix}готово чанков={n_chunks} байт={nbytes}", flush=True)
    except grpc.RpcError as e:
        with print_lock:
            msg = f"{prefix}gRPC: {e.code()} {e.details()}"
            if e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
                msg += " — лимит RPC; уменьши TTS M×K"
            print(msg, file=sys.stderr, flush=True)
        errors.append(e)
    except BaseException as e:
        errors.append(e)
        with print_lock:
            print(f"{prefix}ошибка: {e}", file=sys.stderr, flush=True)


def _feeder_pcm(
    pcm: bytes,
    sample_rate: int,
    chunk_seconds: float,
    realtime_pace: bool,
    audio_queues: list["queue.Queue[bytes | None]"],
    print_lock: threading.Lock,
) -> None:
    bs = max(2, int(sample_rate * chunk_seconds) * 2)
    with print_lock:
        print(
            f"[feeder] {len(audio_queues)} STT-очередей, чанк {bs} B, realtime_pace={realtime_pace}",
            flush=True,
        )
    for off in range(0, len(pcm), bs):
        chunk = pcm[off : off + bs]
        if not chunk:
            break
        for aq in audio_queues:
            aq.put(chunk)
        if realtime_pace and len(chunk) >= 2:
            time.sleep(len(chunk) / (2 * sample_rate))
    for aq in audio_queues:
        aq.put(None)


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Параллельно M×K Recognize (WAV) + M×K Synthesize, SaluteSpeech gRPC"
    )
    default_wav = Path(__file__).resolve().parent.parent / "out.wav"
    parser.add_argument("--wav", type=Path, default=default_wav)
    parser.add_argument("--tts-text", default="1 2 3 4 5 6 7 8 9 0")
    parser.add_argument("--stt-grpc-channels", type=int, default=1, metavar="M_STT")
    parser.add_argument("--stt-streams-per-channel", type=int, default=1, metavar="K_STT")
    parser.add_argument("--tts-grpc-channels", type=int, default=1, metavar="M_TTS")
    parser.add_argument("--tts-streams-per-channel", type=int, default=1, metavar="K_TTS")
    parser.add_argument("--credentials", default="")
    parser.add_argument("--scope", default=DEFAULT_SCOPE)
    parser.add_argument("--stt-language", default="ru-RU")
    parser.add_argument("--tts-language", default="ru-RU")
    parser.add_argument("--voice", default="Bys_24000")
    parser.add_argument("--chunk-seconds", type=float, default=0.2)
    parser.add_argument("--no-realtime-pace", action="store_true")
    parser.add_argument("--eou-timeout", type=float, default=1.0)
    parser.add_argument("--no-ssl-verify", action="store_true")
    parser.add_argument(
        "--ca-bundle",
        default=os.environ.get("SALUTESPEECH_CA_BUNDLE_FILE", ""),
    )
    parser.add_argument("--connect-timeout-sec", type=float, default=8.0)
    parser.add_argument("--no-play", action="store_true")
    args = parser.parse_args()

    m_stt = max(1, args.stt_grpc_channels)
    k_stt = max(1, args.stt_streams_per_channel)
    n_stt = m_stt * k_stt
    m_tts = max(1, args.tts_grpc_channels)
    k_tts = max(1, args.tts_streams_per_channel)
    n_tts = m_tts * k_tts

    creds = _normalize_credentials(
        args.credentials or os.environ.get("SALUTESPEECH_CREDENTIALS", "") or DEFAULT_CREDENTIALS
    )
    if not creds:
        print("Нужен --credentials / SALUTESPEECH_CREDENTIALS / DEFAULT_CREDENTIALS", file=sys.stderr)
        return 1
    if not args.wav.is_file():
        print(f"Нет WAV: {args.wav}", file=sys.stderr)
        return 1

    ssl_ctx = _oauth_ssl_context(args.no_ssl_verify, args.ca_bundle)
    try:
        token = _oauth_token(creds, args.scope, ssl_ctx)
    except Exception as e:
        print(f"OAuth: {e}", file=sys.stderr)
        return 1

    root_certs: bytes | None = None
    if args.ca_bundle:
        root_certs = Path(args.ca_bundle).read_bytes()
        if b"-----BEGIN CERTIFICATE-----" not in root_certs:
            b64 = base64.b64encode(root_certs)
            pem = (
                "-----BEGIN CERTIFICATE-----\n"
                + "\n".join(b64[i : i + 64].decode("ascii") for i in range(0, len(b64), 64))
                + "\n-----END CERTIFICATE-----\n"
            )
            root_certs = pem.encode("utf-8")

    print_lock = threading.Lock()
    errors: list[BaseException] = []

    try:
        pcm, fr = _wav_to_mono_s16le(args.wav.resolve())
    except Exception as e:
        print(f"WAV: {e}", file=sys.stderr)
        return 1

    rec_pb2, rec_grpc = _load_recognition()
    syn_pb2, syn_grpc = _load_synthesis()
    metadata = (("authorization", f"Bearer {token}"),)

    with print_lock:
        print(
            f"Старт: STT {m_stt}×{k_stt}={n_stt} Recognize, TTS {m_tts}×{k_tts}={n_tts} Synthesize "
            f"(всего до {m_stt + m_tts} gRPC channel). WAV {len(pcm)} B @ {fr} Hz.",
            flush=True,
        )

    stt_channels: list[Any] = []
    tts_channels: list[Any] = []
    try:
        stt_channels = _open_channels(m_stt, root_certs, args.connect_timeout_sec)
        tts_channels = _open_channels(m_tts, root_certs, args.connect_timeout_sec)
    except Exception as e:
        print(f"gRPC: {e}", file=sys.stderr)
        for c in stt_channels + tts_channels:
            with contextlib.suppress(Exception):
                c.close()
        return 1

    stt_stubs = [rec_grpc.SmartSpeechStub(c) for c in stt_channels]
    tts_stubs = [syn_grpc.SmartSpeechStub(c) for c in tts_channels]

    audio_queues = [queue.Queue(maxsize=128) for _ in range(n_stt)]

    workers: list[threading.Thread] = []
    for slot in range(n_stt):
        ch_idx = min(slot // k_stt, m_stt - 1)
        t = threading.Thread(
            target=run_stt_worker,
            args=(slot, stt_stubs[ch_idx], rec_pb2, audio_queues[slot], metadata),
            kwargs={
                "sample_rate": fr,
                "language": args.stt_language,
                "eou_timeout": args.eou_timeout,
                "stt_streams_per_ch": k_stt,
                "stt_n_ch": m_stt,
                "print_lock": print_lock,
                "errors": errors,
            },
            daemon=True,
        )
        workers.append(t)
        t.start()

    for slot in range(n_tts):
        ch_idx = min(slot // k_tts, m_tts - 1)
        t = threading.Thread(
            target=run_tts_worker,
            args=(slot, tts_stubs[ch_idx], syn_pb2, args.tts_text, metadata),
            kwargs={
                "voice": args.voice,
                "language": args.tts_language,
                "no_play": args.no_play,
                "tts_streams_per_ch": k_tts,
                "tts_n_ch": m_tts,
                "print_lock": print_lock,
                "errors": errors,
            },
            daemon=True,
        )
        workers.append(t)
        t.start()

    feeder = threading.Thread(
        target=_feeder_pcm,
        args=(pcm, fr, args.chunk_seconds, not args.no_realtime_pace, audio_queues, print_lock),
        daemon=True,
    )
    feeder.start()

    feeder.join()
    for t in workers:
        t.join(timeout=120.0)

    for c in stt_channels + tts_channels:
        with contextlib.suppress(Exception):
            c.close()

    if errors:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
