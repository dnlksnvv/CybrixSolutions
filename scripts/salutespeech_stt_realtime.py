#!/usr/bin/env python3
"""
Простой realtime STT для SaluteSpeech gRPC.

Использует готовые файлы:
  scripts/_salutespeech_generated/recognition_pb2.py
  scripts/_salutespeech_generated/recognition_pb2_grpc.py

Без автоскачивания proto и без grpc_tools во время запуска.

Несколько параллельных Recognize (как TTS: M channel × K стримов на каждом, всего M×K):
  python salutespeech_stt_realtime.py --grpc-channels 2 --streams-per-channel 5 \\
    --ca-bundle certs/russian_trusted_root_ca.pem
"""

from __future__ import annotations

import argparse
import contextlib
import base64
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
from pathlib import Path
from typing import Any, Iterator

import grpc
from google.protobuf.duration_pb2 import Duration

OAUTH_URL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
GRPC_HOST = "smartspeech.sber.ru:443"
DEFAULT_SCOPE = "SALUTE_SPEECH_PERS"
DEFAULT_CREDENTIALS = "MDE5ZGMyMTUtZWU2OC03ODI0LTlmMjYtMzRkNjEyMDRiOThmOmY5MGM4ZWI2LWY3YTUtNGY1MS04ZDQ0LWFkYWIxZjFkZmY3Yg=="


def _normalize_credentials(raw: str) -> str:
    s = (raw or "").strip()
    s = s.rstrip("\"'")
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        s = s[1:-1].strip().rstrip("\"'")
    return s


def _ssl_context(no_verify: bool, ca_bundle: str = "") -> ssl.SSLContext:
    if no_verify:
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx
    if ca_bundle:
        # Поддерживаем и обычный PEM, и DER(.cer) без ручной конвертации.
        raw = Path(ca_bundle).read_bytes()
        if b"-----BEGIN CERTIFICATE-----" in raw:
            return ssl.create_default_context(cafile=ca_bundle)
        pem = (
            "-----BEGIN CERTIFICATE-----\n"
            + "\n".join(
                raw_b64.decode("ascii")
                for raw_b64 in [base64.b64encode(raw)[i : i + 64] for i in range(0, len(base64.b64encode(raw)), 64)]
            )
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


def _load_pb2() -> tuple[Any, Any]:
    generated = Path(__file__).resolve().parent / "_salutespeech_generated"
    pb2 = generated / "recognition_pb2.py"
    pb2_grpc = generated / "recognition_pb2_grpc.py"
    if not (pb2.exists() and pb2_grpc.exists()):
        raise RuntimeError(
            "Не найдены pb2-файлы в scripts/_salutespeech_generated.\n"
            "Сгенерируйте один раз и запустите снова."
        )
    sys.path.insert(0, str(generated))
    return importlib.import_module("recognition_pb2"), importlib.import_module(
        "recognition_pb2_grpc"
    )


def _recognition_worker(
    slot: int,
    stub: Any,
    pb2: Any,
    metadata: tuple[tuple[str, str], ...],
    audio_q: "queue.Queue[bytes | None]",
    *,
    sample_rate: int,
    language: str,
    hypotheses_count: int,
    partial: bool,
    multi_utterance: bool,
    eou_timeout: float,
    grpc_channels: int,
    streams_per_channel: int,
    n_parallel: int,
    print_lock: threading.Lock,
) -> None:
    k = max(1, streams_per_channel)
    ch_i = min(slot // k, grpc_channels - 1)
    if n_parallel > 1 or grpc_channels > 1:
        prefix = f"[#{slot} ch{ch_i}] "
    else:
        prefix = ""
    try:
        responses = stub.Recognize(
            _request_iter(
                pb2,
                audio_q,
                sample_rate=sample_rate,
                language=language,
                hypotheses_count=hypotheses_count,
                partial=partial,
                multi_utterance=multi_utterance,
                eou_timeout=eou_timeout,
            ),
            metadata=metadata,
        )
        for resp in responses:
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
                msg += (
                    " — лимит параллельных RPC; уменьши --grpc-channels и/или --streams-per-channel"
                )
            print(msg, file=sys.stderr)
    except Exception as e:
        with print_lock:
            print(f"{prefix}error: {e}", file=sys.stderr)


def _request_iter(
    pb2: Any,
    audio_q: "queue.Queue[bytes | None]",
    *,
    sample_rate: int,
    language: str,
    hypotheses_count: int,
    partial: bool,
    multi_utterance: bool,
    eou_timeout: float,
) -> Iterator[Any]:
    opts = pb2.RecognitionOptions(
        audio_encoding=pb2.RecognitionOptions.PCM_S16LE,
        sample_rate=sample_rate,
        language=language,
        channels_count=1,
        hypotheses_count=hypotheses_count,
        enable_partial_results=partial,
        enable_multi_utterance=multi_utterance,
        no_speech_timeout=_duration(7.0),
        max_speech_timeout=_duration(20.0),
        hints=pb2.Hints(eou_timeout=_duration(eou_timeout)),
    )
    yield pb2.RecognitionRequest(options=opts)
    while True:
        chunk = audio_q.get()
        if chunk is None:
            return
        yield pb2.RecognitionRequest(audio_chunk=chunk)


def main() -> int:
    parser = argparse.ArgumentParser(description="SaluteSpeech realtime STT (simple)")
    parser.add_argument("--credentials", default="")
    parser.add_argument("--scope", default=DEFAULT_SCOPE)
    parser.add_argument("--language", default="ru-RU")
    parser.add_argument("--sample-rate", type=int, default=16000)
    parser.add_argument("--chunk-seconds", type=float, default=0.2)
    parser.add_argument("--eou-timeout", type=float, default=1.0)
    parser.add_argument("--hypotheses-count", type=int, default=1)
    parser.add_argument("--partial", action="store_true")
    parser.add_argument("--multi-utterance", action="store_true")
    parser.add_argument("--no-ssl-verify", action="store_true")
    parser.add_argument(
        "--ca-bundle",
        default=os.environ.get("SALUTESPEECH_CA_BUNDLE_FILE", ""),
        help="PEM bundle с доверенным корнем НУЦ для gRPC TLS",
    )
    parser.add_argument(
        "--grpc-channels",
        type=int,
        default=1,
        metavar="M",
        help="M отдельных gRPC secure_channel (TCP). Всего Recognize: M × K (см. --streams-per-channel). По умолчанию 1",
    )
    parser.add_argument(
        "--streams-per-channel",
        type=int,
        default=1,
        metavar="K",
        help="на каждом channel одновременно K потоков Recognize; слоты 0…K−1 → ch0, K…2K−1 → ch1, … Всего M×K. По умолчанию 1",
    )
    parser.add_argument(
        "--connect-timeout-sec",
        type=float,
        default=8.0,
        help="таймаут установки каждого gRPC channel при старте",
    )
    args = parser.parse_args()

    creds = _normalize_credentials(
        args.credentials or DEFAULT_CREDENTIALS or os.environ.get("SALUTESPEECH_CREDENTIALS", "")
    )
    if not creds:
        print("Нужен credentials (в коде/--credentials/SALUTESPEECH_CREDENTIALS)", file=sys.stderr)
        return 1

    try:
        pb2, pb2_grpc = _load_pb2()
    except Exception as e:
        print(f"Proto load error: {e}", file=sys.stderr)
        return 1

    ssl_ctx = _ssl_context(args.no_ssl_verify, args.ca_bundle)
    try:
        token = _oauth_token(creds, args.scope, ssl_ctx)
    except Exception as e:
        print(f"OAuth error: {e}", file=sys.stderr)
        return 1

    import sounddevice as sd

    n_grpc = max(1, args.grpc_channels)
    k = max(1, args.streams_per_channel)
    n_parallel = n_grpc * k
    audio_queues: list[queue.Queue[bytes | None]] = [
        queue.Queue(maxsize=32) for _ in range(n_parallel)
    ]
    blocksize = max(1, int(args.sample_rate * args.chunk_seconds))

    def cb(indata: Any, frames: int, t: Any, status: Any) -> None:
        if status:
            print(f"[audio] {status}", file=sys.stderr)
        b = bytes(indata)
        for aq in audio_queues:
            with contextlib.suppress(queue.Full):
                aq.put_nowait(b)

    root_certs: bytes | None = None
    if args.ca_bundle:
        try:
            root_certs = Path(args.ca_bundle).read_bytes()
        except Exception as e:
            print(f"Не удалось прочитать --ca-bundle: {e}", file=sys.stderr)
            return 1

    # В grpc-python нет полноценного аналога verify=False как у requests/urllib:
    # для TLS нужен доверенный root CA. Поэтому --no-ssl-verify влияет только на OAuth HTTP.
    if args.no_ssl_verify and not root_certs:
        print(
            "Внимание: --no-ssl-verify не отключает проверку TLS для gRPC.\n"
            "Укажите --ca-bundle <path/to/ca.pem> (или SALUTESPEECH_CA_BUNDLE_FILE).",
            file=sys.stderr,
        )

    creds_tls = grpc.ssl_channel_credentials(root_certificates=root_certs)
    channels: list[Any] = []
    try:
        for i in range(n_grpc):
            ch = grpc.secure_channel(GRPC_HOST, creds_tls)
            try:
                grpc.channel_ready_future(ch).result(timeout=args.connect_timeout_sec)
            except grpc.FutureTimeoutError:
                print(
                    f"gRPC connect timeout after {args.connect_timeout_sec:.1f}s "
                    f"(host={GRPC_HOST}, channel {len(channels) + 1}/{n_grpc})",
                    file=sys.stderr,
                )
                ch.close()
                for c in channels:
                    with contextlib.suppress(Exception):
                        c.close()
                return 1
            channels.append(ch)
    except Exception as e:
        print(f"gRPC channel error: {e}", file=sys.stderr)
        for c in channels:
            with contextlib.suppress(Exception):
                c.close()
        return 1

    stubs = [pb2_grpc.SmartSpeechStub(c) for c in channels]
    metadata = (("authorization", f"Bearer {token}"),)
    print_lock = threading.Lock()

    if n_parallel > 1:
        print(
            f"STT: gRPC {n_grpc} channel × {k} стрим(ов) = {n_parallel} Recognize "
            f"(слот s → ch s // {k}). Один микрофон, квота API общая.",
            flush=True,
        )
    print("Говорите в микрофон. Остановка: Ctrl+C", flush=True)

    workers: list[threading.Thread] = []
    for slot in range(n_parallel):
        ch_idx = min(slot // k, n_grpc - 1)
        stub = stubs[ch_idx]
        t = threading.Thread(
            target=_recognition_worker,
            args=(
                slot,
                stub,
                pb2,
                metadata,
                audio_queues[slot],
            ),
            kwargs={
                "sample_rate": args.sample_rate,
                "language": args.language,
                "hypotheses_count": args.hypotheses_count,
                "partial": args.partial,
                "multi_utterance": args.multi_utterance,
                "eou_timeout": args.eou_timeout,
                "grpc_channels": n_grpc,
                "streams_per_channel": k,
                "n_parallel": n_parallel,
                "print_lock": print_lock,
            },
            daemon=True,
        )
        workers.append(t)
        t.start()

    try:
        with sd.RawInputStream(
            samplerate=args.sample_rate,
            blocksize=blocksize,
            channels=1,
            dtype="int16",
            callback=cb,
        ):
            while True:
                time.sleep(0.25)
    except KeyboardInterrupt:
        print("\nОстановлено.", flush=True)
    except Exception as e:
        print(f"Audio error: {e}", file=sys.stderr)
        return 1
    finally:
        for aq in audio_queues:
            with contextlib.suppress(Exception):
                aq.put_nowait(None)
        for t in workers:
            t.join(timeout=5.0)
        for ch in channels:
            with contextlib.suppress(Exception):
                ch.close()

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
