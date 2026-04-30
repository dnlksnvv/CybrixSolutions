#!/usr/bin/env python3
"""
Синхронный синтез речи (TTS) через SaluteSpeech REST API.

Нужен отдельный проект SaluteSpeech в Studio и Authorization Key из настроек API.
Это не тот же ключ, что у GigaChat.

Документация:
  https://developers.sber.ru/docs/ru/salutespeech/overview
  https://developers.sber.ru/docs/ru/salutespeech/api/authentication
  https://developers.sber.ru/docs/ru/salutespeech/rest/sync-general

Пример:
  export SALUTESPEECH_CREDENTIALS='MDE5ZGMyMTUtZWU2OC03ODI0LTlmMjYtMzRkNjEyMDRiOThmOmY5MGM4ZWI2LWY3YTUtNGY1MS04ZDQ0LWFkYWIxZjFkZmY3Yg=='
  python3 salutespeech_tts.py --no-ssl-verify "Привет, это тест" -o out.wav

Интерактив (как salutespeech_tts_streaming.py -i: строка + Enter, время до первых данных, воспроизведение):
  python3 salutespeech_tts.py --no-ssl-verify -i
  python3 salutespeech_tts.py -i --interactive-save-dir ./tts_saved   # оставлять файлы на диске
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import tempfile
import platform
import shutil
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass
from typing import Any

OAUTH_URL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
SYNTH_URL = "https://smartspeech.sber.ru/rest/v1/text:synthesize"
DEFAULT_SCOPE = "SALUTE_SPEECH_PERS"
DEFAULT_VOICE = "Bys_24000"
DEFAULT_FORMAT = "wav16"
MAX_CHARS = 4000


@dataclass
class TokenState:
    access_token: str
    expires_at_unix: float


def _env_wants_ssl_verify() -> bool:
    raw = (os.environ.get("SALUTESPEECH_VERIFY_SSL_CERTS") or "true").strip().lower()
    return raw not in ("0", "false", "no", "off")


def _normalize_salutespeech_credentials(raw: str) -> str:
    """Убирает пробелы и типичные ошибки копирования (лишние кавычки в конце / обёртка)."""
    s = (raw or "").strip()
    s = s.rstrip("\"'")
    if len(s) >= 2 and s[0] == s[-1] and s[0] in ("'", '"'):
        s = s[1:-1].strip().rstrip("\"'")
    return s


def _ssl_context(no_verify: bool, ca_bundle: str | None) -> ssl.SSLContext:
    if no_verify:
        ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        return ctx
    if ca_bundle:
        return ssl.create_default_context(cafile=ca_bundle)
    return ssl.create_default_context()


def _http_json(
    url: str,
    *,
    method: str = "GET",
    headers: dict[str, str] | None = None,
    body: bytes | None = None,
    ssl_context: ssl.SSLContext,
    timeout: float = 120.0,
) -> tuple[int, Any]:
    req = urllib.request.Request(url, data=body, method=method)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=timeout, context=ssl_context) as resp:
            raw = resp.read().decode("utf-8")
            status = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace")
        status = e.code
        try:
            return status, json.loads(raw)
        except json.JSONDecodeError:
            return status, raw

    if not raw:
        return status, {}
    try:
        return status, json.loads(raw)
    except json.JSONDecodeError:
        return status, raw


def fetch_token(credentials: str, scope: str, ssl_context: ssl.SSLContext) -> TokenState:
    rq = str(uuid.uuid4())
    form = urllib.parse.urlencode({"scope": scope.strip()}).encode("utf-8")
    headers = {
        "Content-Type": "application/x-www-form-urlencoded",
        "Accept": "application/json",
        "RqUID": rq,
        "Authorization": f"Basic {credentials}",
    }
    status, data = _http_json(
        OAUTH_URL,
        method="POST",
        headers=headers,
        body=form,
        ssl_context=ssl_context,
        timeout=60.0,
    )
    if status != 200 or not isinstance(data, dict):
        raise RuntimeError(f"OAuth не удался ({status}): {data}")
    token = data.get("access_token")
    if not token or not isinstance(token, str):
        raise RuntimeError(f"Нет access_token в ответе: {data}")
    exp = data.get("expires_at")
    if isinstance(exp, (int, float)) and exp > 0:
        exp_unix = float(exp) / 1000.0 if float(exp) > 1e12 else float(exp)
    else:
        exp_unix = time.time() + 25 * 60
    return TokenState(access_token=token, expires_at_unix=exp_unix)


def _synthesize_build_request(
    access_token: str,
    text: str,
    *,
    voice: str,
    audio_format: str,
    ssml: bool,
    rebuild_cache: bool,
    bypass_cache: bool,
) -> urllib.request.Request:
    if len(text) > MAX_CHARS:
        raise ValueError(f"Текст длиннее {MAX_CHARS} символов (лимит API)")

    q: dict[str, str] = {"voice": voice, "format": audio_format}
    if rebuild_cache:
        q["rebuild_cache"] = "true"
    if bypass_cache:
        q["bypass_cache"] = "true"
    url = SYNTH_URL + "?" + urllib.parse.urlencode(q)

    content_type = "application/ssml" if ssml else "application/text"
    body = text.encode("utf-8")
    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", content_type)
    req.add_header("Accept", "audio/*, application/octet-stream")
    req.add_header("Authorization", f"Bearer {access_token}")
    req.add_header("X-Request-ID", str(uuid.uuid4()))
    return req


def synthesize(
    access_token: str,
    text: str,
    *,
    voice: str,
    audio_format: str,
    ssml: bool,
    ssl_context: ssl.SSLContext,
    rebuild_cache: bool = False,
    bypass_cache: bool = False,
) -> bytes:
    """POST text:synthesize; тело — UTF-8 текст, параметры — в query (как в REST-клиентах API)."""
    req = _synthesize_build_request(
        access_token,
        text,
        voice=voice,
        audio_format=audio_format,
        ssml=ssml,
        rebuild_cache=rebuild_cache,
        bypass_cache=bypass_cache,
    )
    try:
        with urllib.request.urlopen(req, timeout=120.0, context=ssl_context) as resp:
            if resp.status != 200:
                raise RuntimeError(f"HTTP {resp.status}")
            return resp.read()
    except urllib.error.HTTPError as e:
        err = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Синтез: HTTP {e.code}: {err}") from e


def synthesize_measured(
    access_token: str,
    text: str,
    *,
    voice: str,
    audio_format: str,
    ssml: bool,
    ssl_context: ssl.SSLContext,
    rebuild_cache: bool = False,
    bypass_cache: bool = False,
    latency_anchor: float,
    read_block: int = 65536,
) -> tuple[bytes, float | None, float]:
    """
    Тот же синтез, но чтение ответа блоками: время до первого блока (аналог «первого чанка» gRPC)
    и длительность цикла чтения. REST отдаёт тело целиком или чанками — зависит от сервера.
    """
    req = _synthesize_build_request(
        access_token,
        text,
        voice=voice,
        audio_format=audio_format,
        ssml=ssml,
        rebuild_cache=rebuild_cache,
        bypass_cache=bypass_cache,
    )
    t_stream = time.perf_counter()
    t_first: float | None = None
    parts: list[bytes] = []
    try:
        with urllib.request.urlopen(req, timeout=120.0, context=ssl_context) as resp:
            if resp.status != 200:
                raise RuntimeError(f"HTTP {resp.status}")
            while True:
                block = resp.read(read_block)
                if not block:
                    break
                if t_first is None:
                    t_first = time.perf_counter() - latency_anchor
                parts.append(block)
    except urllib.error.HTTPError as e:
        err = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Синтез: HTTP {e.code}: {err}") from e

    elapsed = time.perf_counter() - t_stream
    return b"".join(parts), t_first, elapsed


def _default_output_path(fmt: str) -> str:
    ext = {DEFAULT_FORMAT: "wav", "wav16": "wav", "pcm16": "pcm", "opus": "opus"}.get(
        fmt.lower(),
        "bin",
    )
    return f"salutespeech_out.{ext}"


def _interactive_save_extension(audio_format: str) -> str:
    """Расширение файла при сохранении синтеза в интерактивном режиме."""
    return {DEFAULT_FORMAT: "wav", "wav16": "wav", "pcm16": "pcm", "opus": "opus"}.get(
        audio_format.lower(),
        "bin",
    )


def _next_interactive_audio_path(save_dir: str, audio_format: str) -> str:
    """Уникальный путь: salutespeech_YYYYMMDD_HHMMSS_<uuid>.<ext>"""
    os.makedirs(save_dir, exist_ok=True)
    ts = time.strftime("%Y%m%d_%H%M%S")
    ext = _interactive_save_extension(audio_format)
    name = f"salutespeech_{ts}_{uuid.uuid4().hex[:8]}.{ext}"
    return os.path.join(save_dir, name)


def play_audio_file(path: str, audio_format: str) -> None:
    """Воспроизведение без лишних зависимостей (afplay / ffplay / aplay)."""
    system = platform.system()
    if system == "Darwin":
        subprocess.run(["afplay", path], check=True)
        return
    if system == "Linux":
        if shutil.which("ffplay"):
            subprocess.run(
                [
                    "ffplay",
                    "-nodisp",
                    "-autoexit",
                    "-loglevel",
                    "quiet",
                    path,
                ],
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
            return
        if audio_format in ("wav16", "wav") and shutil.which("aplay"):
            subprocess.run(["aplay", "-q", path], check=True)
            return
        raise RuntimeError(
            "Linux: установите ffmpeg (ffplay) или для wav16 — alsa-utils (aplay)",
        )
    if system == "Windows":
        import winsound

        winsound.PlaySound(path, winsound.SND_FILENAME | winsound.SND_SYNC)
        return
    raise RuntimeError(f"Воспроизведение не реализовано для ОС: {system}")


def run_interactive(args: argparse.Namespace, ssl_ctx: ssl.SSLContext, creds: str) -> int:
    """
    REPL в стиле salutespeech_tts_streaming.py -i: строка + Enter, метрики, воспроизведение.
    Голос / формат / SSML — только аргументами при запуске (не /команды в чате).
    """
    voice: str = args.voice
    audio_format: str = args.audio_format
    ssml: bool = args.ssml
    token_state: TokenState | None = None
    save_dir = (getattr(args, "interactive_save_dir", None) or "").strip()

    def ensure_token() -> str:
        nonlocal token_state
        now = time.time()
        if token_state is None or (
            token_state.expires_at_unix > 0 and now > token_state.expires_at_unix - 60
        ):
            token_state = fetch_token(creds, args.scope, ssl_ctx)
        return token_state.access_token

    print(
        "Режим чата (REST): строка + Enter → синтез и воспроизведение. "
        "Команды: /q /quit — выход.\n"
        f"Голос: {voice}, формат: {audio_format}, ssml: {'да' if ssml else 'нет'}"
        + (
            f"\nФайлы сохраняются в: {save_dir}"
            if save_dir
            else "\nВременный файл — удаляется после воспроизведения (см. --interactive-save-dir)."
        ),
        flush=True,
    )

    while True:
        try:
            line = input("🎙 > ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            break
        if not line:
            continue
        low = line.lower()
        if low in ("/q", "/quit", "/exit"):
            break

        if len(line) > MAX_CHARS:
            print(f"Слишком длинно (>{MAX_CHARS} символов). Разбейте на части.", file=sys.stderr)
            continue

        t_after_enter = time.perf_counter()
        path = ""
        keep_file = bool(save_dir)
        try:
            if keep_file:
                path = _next_interactive_audio_path(save_dir, audio_format)
            else:
                ext = _interactive_save_extension(audio_format)
                fd, path = tempfile.mkstemp(suffix="." + ext)
                os.close(fd)

            tok = ensure_token()
            audio, t_first, elapsed = synthesize_measured(
                tok,
                line,
                voice=voice,
                audio_format=audio_format,
                ssml=ssml,
                ssl_context=ssl_ctx,
                rebuild_cache=args.rebuild_cache,
                bypass_cache=args.bypass_cache,
                latency_anchor=t_after_enter,
            )
            with open(path, "wb") as f:
                f.write(audio)

            if t_first is not None:
                print(f"до первого чанка: {t_first:.3f} с", flush=True)
            else:
                print("до первого чанка: —", flush=True)

            play_audio_file(path, audio_format)

            wall = time.perf_counter() - t_after_enter
            extra = f" (от Enter до конца ответа: {wall:.2f} с)" if t_first is not None else ""
            print(
                f"готово: байт={len(audio)}, ответ {elapsed:.2f} с{extra}",
                flush=True,
            )
            if keep_file:
                print(f"файл: {path}", flush=True)
        except subprocess.CalledProcessError as e:
            print(f"Проигрыватель вернул ошибку: {e}", file=sys.stderr)
        except Exception as e:
            err = str(e)
            if "CERTIFICATE_VERIFY_FAILED" in err or "certificate verify failed" in err.lower():
                print("TLS: --no-ssl-verify или --ca-bundle", file=sys.stderr)
            print(f"Ошибка: {e}", file=sys.stderr)
        finally:
            if path and not keep_file and os.path.isfile(path):
                with contextlib.suppress(OSError):
                    os.unlink(path)
            elif path and keep_file and os.path.isfile(path) and os.path.getsize(path) == 0:
                with contextlib.suppress(OSError):
                    os.unlink(path)

    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        description="SaluteSpeech: синхронный синтез речи в файл",
    )
    parser.add_argument(
        "text",
        nargs="?",
        default="",
        help="Текст для озвучки (или используйте --file)",
    )
    parser.add_argument(
        "--file",
        "-f",
        metavar="PATH",
        help="Взять текст из файла (UTF-8)",
    )
    parser.add_argument(
        "-o",
        "--output",
        help=f"Куда сохранить аудио (по умолчанию по формату: {_default_output_path(DEFAULT_FORMAT)})",
    )
    parser.add_argument(
        "--credentials",
        default=os.environ.get("SALUTESPEECH_CREDENTIALS", ""),
        help="Authorization Key Base64 или env SALUTESPEECH_CREDENTIALS",
    )
    parser.add_argument(
        "--scope",
        default=os.environ.get("SALUTESPEECH_SCOPE", DEFAULT_SCOPE),
        help=f"OAuth scope (по умолчанию {DEFAULT_SCOPE})",
    )
    parser.add_argument("--voice", default=DEFAULT_VOICE, help="Голос, напр. May_24000, Nec_24000")
    parser.add_argument(
        "--format",
        dest="audio_format",
        default=DEFAULT_FORMAT,
        choices=("wav16", "pcm16", "opus"),
        help="Формат аудио",
    )
    parser.add_argument(
        "--ssml",
        action="store_true",
        help="Текст — SSML (Content-Type application/ssml)",
    )
    parser.add_argument("--no-ssl-verify", action="store_true", help="Отключить проверку TLS")
    parser.add_argument(
        "--ca-bundle",
        default=os.environ.get("SALUTESPEECH_CA_BUNDLE_FILE", "") or None,
        help="PEM НУЦ или env SALUTESPEECH_CA_BUNDLE_FILE",
    )
    parser.add_argument("--rebuild-cache", action="store_true", help="Параметр API rebuild_cache")
    parser.add_argument("--bypass-cache", action="store_true", help="Параметр API bypass_cache")
    parser.add_argument(
        "-i",
        "--interactive",
        action="store_true",
        help="Интерактив: каждая строка (Enter) — сохранение в файл и воспроизведение",
    )
    parser.add_argument(
        "--interactive-save-dir",
        default="",
        metavar="DIR",
        help="При -i: сохранять каждый синтез в каталог; иначе временный файл удаляется после проигрыша",
    )
    args = parser.parse_args()

    ssl_off = args.no_ssl_verify or not _env_wants_ssl_verify()
    creds = (args.credentials or "").strip()
    if not creds:
        print("Задайте --credentials или SALUTESPEECH_CREDENTIALS", file=sys.stderr)
        return 1

    ssl_ctx = _ssl_context(ssl_off, args.ca_bundle)
    if args.interactive:
        if not ssl_off and not args.ca_bundle:
            print(
                "Подсказка: без НУЦ часто нужен --no-ssl-verify.\n"
                "  https://developers.sber.ru/docs/ru/gigachat/certificates",
                file=sys.stderr,
            )
        return run_interactive(args, ssl_ctx, creds)

    if args.file:
        with open(args.file, encoding="utf-8") as f:
            text = f.read()
    else:
        text = args.text
    text = text.strip()
    if not text:
        print("Укажите текст аргументом или --file", file=sys.stderr)
        return 1

    out_path = args.output or _default_output_path(args.audio_format)

    if not ssl_off and not args.ca_bundle:
        print(
            "Подсказка: без сертификата НУЦ TLS может падать — см.\n"
            "  https://developers.sber.ru/docs/ru/gigachat/certificates\n"
            "  Или: --no-ssl-verify",
            file=sys.stderr,
        )

    try:
        token = fetch_token(creds, args.scope, ssl_ctx)
        audio = synthesize(
            token.access_token,
            text,
            voice=args.voice,
            audio_format=args.audio_format,
            ssml=args.ssml,
            ssl_context=ssl_ctx,
            rebuild_cache=args.rebuild_cache,
            bypass_cache=args.bypass_cache,
        )
    except Exception as e:
        err = str(e)
        if "CERTIFICATE_VERIFY_FAILED" in err or "certificate verify failed" in err.lower():
            print(
                "TLS: используйте --no-ssl-verify или --ca-bundle (корень НУЦ).",
                file=sys.stderr,
            )
        print(f"Ошибка: {e}", file=sys.stderr)
        return 1

    with open(out_path, "wb") as f:
        f.write(audio)
    print(f"Сохранено: {out_path} ({len(audio)} байт)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
