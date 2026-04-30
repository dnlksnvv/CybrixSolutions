#!/usr/bin/env python3
"""
Интерактивный чат с GigaChat API в консоли.

Требуется ключ авторизации (Base64 от client_id:client_secret) из личного кабинета.
Переменная окружения: GIGACHAT_CREDENTIALS

Документация: https://developers.sber.ru/docs/ru/gigachat/api/authorization
Сертификаты НУЦ: https://developers.sber.ru/docs/ru/gigachat/certificates

Пример (macOS / без НУЦ в системе — иначе будет CERTIFICATE_VERIFY_FAILED):
  export GIGACHAT_CREDENTIALS='ваш_base64_ключ'
  python3 gigachat_console.py --no-ssl-verify
  # или: export GIGACHAT_VERIFY_SSL_CERTS=false
"""

from __future__ import annotations

import argparse
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from dataclasses import dataclass
from http.client import HTTPResponse
from typing import Any, Iterator


OAUTH_URL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
DEFAULT_CHAT_BASE = "https://gigachat.devices.sberbank.ru/api/v1"
DEFAULT_SCOPE = "GIGACHAT_API_PERS"
DEFAULT_MODEL = "GigaChat"


def _env_wants_ssl_verify() -> bool:
    """Как в gigachat SDK: GIGACHAT_VERIFY_SSL_CERTS=false отключает проверку TLS."""
    raw = (os.environ.get("GIGACHAT_VERIFY_SSL_CERTS") or "true").strip().lower()
    return raw not in ("0", "false", "no", "off")


@dataclass
class TokenState:
    access_token: str
    expires_at_unix: float  # seconds since epoch


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
) -> tuple[int, dict[str, Any] | list[Any] | str]:
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
            parsed: Any = json.loads(raw)
        except json.JSONDecodeError:
            return status, raw
        return status, parsed

    if not raw:
        return status, {}
    try:
        return status, json.loads(raw)
    except json.JSONDecodeError:
        return status, raw


def fetch_token(
    credentials: str,
    scope: str,
    ssl_context: ssl.SSLContext,
) -> TokenState:
    rq = str(uuid.uuid4())
    form = urllib.parse.urlencode({"scope": scope}).encode("utf-8")
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
        raise RuntimeError(f"OAuth failed ({status}): {data}")
    token = data.get("access_token")
    if not token or not isinstance(token, str):
        raise RuntimeError(f"OAuth: нет access_token в ответе: {data}")
    exp = data.get("expires_at")
    exp_unix: float
    if isinstance(exp, (int, float)) and exp > 0:
        # API иногда отдаёт миллисекунды
        exp_unix = float(exp) / 1000.0 if float(exp) > 1e12 else float(exp)
    else:
        exp_unix = time.time() + 25 * 60
    return TokenState(access_token=token, expires_at_unix=exp_unix)


def chat_completion(
    base: str,
    token: str,
    model: str,
    messages: list[dict[str, str]],
    ssl_context: ssl.SSLContext,
    temperature: float | None = None,
) -> str:
    url = base.rstrip("/") + "/chat/completions"
    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": False,
    }
    if temperature is not None:
        payload["temperature"] = temperature
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    headers = {
        "Content-Type": "application/json",
        "Accept": "application/json",
        "Authorization": f"Bearer {token}",
    }
    status, data = _http_json(
        url,
        method="POST",
        headers=headers,
        body=body,
        ssl_context=ssl_context,
        timeout=120.0,
    )
    if status != 200 or not isinstance(data, dict):
        return f"[ошибка {status}] {data}"
    choices = data.get("choices")
    if not isinstance(choices, list) or not choices:
        return f"[неожиданный ответ] {data}"
    msg = choices[0].get("message") if isinstance(choices[0], dict) else None
    if not isinstance(msg, dict):
        return f"[нет message] {data}"
    content = msg.get("content")
    if isinstance(content, str) and content.strip():
        return content
    if msg.get("function_call"):
        return f"[function_call] {msg.get('function_call')}"
    return f"[пустой ответ] {msg}"


def _sse_delta_content(obj: dict[str, Any]) -> str:
    """Текст из одного события chat.completion (delta.content)."""
    choices = obj.get("choices")
    if not isinstance(choices, list) or not choices:
        return ""
    ch0 = choices[0]
    if not isinstance(ch0, dict):
        return ""
    delta = ch0.get("delta")
    if not isinstance(delta, dict):
        return ""
    c = delta.get("content")
    return c if isinstance(c, str) else ""


def _iter_sse_data_lines(resp: HTTPResponse, charset: str = "utf-8") -> Iterator[str]:
    """Строки полезной нагрузки после префикса data: (без многострочных data в примерах доки)."""
    while True:
        raw = resp.readline()
        if not raw:
            break
        line = raw.decode(charset, errors="replace").rstrip("\r\n")
        if not line or line.startswith(":"):
            continue
        if not line.startswith("data:"):
            continue
        yield line[5:].strip()


def chat_completion_stream(
    base: str,
    token: str,
    model: str,
    messages: list[dict[str, str]],
    ssl_context: ssl.SSLContext,
    *,
    temperature: float | None = None,
    update_interval: float | None = None,
    out: Any = None,
) -> str:
    """
    Потоковая генерация (SSE). Печатает фрагменты в out по мере прихода.
    Возвращает полный текст ассистента для истории.
    Документация: https://developers.sber.ru/docs/ru/gigachat/guides/response-token-streaming
    """
    if out is None:
        out = sys.stdout
    url = base.rstrip("/") + "/chat/completions"
    payload: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "stream": True,
    }
    if temperature is not None:
        payload["temperature"] = temperature
    if update_interval is not None:
        payload["update_interval"] = update_interval
    body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "text/event-stream")
    req.add_header("Authorization", f"Bearer {token}")

    try:
        resp = urllib.request.urlopen(req, timeout=300.0, context=ssl_context)
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {e.code}: {err_body}") from e

    collected: list[str] = []
    charset = resp.headers.get_content_charset() or "utf-8"
    try:
        for data_str in _iter_sse_data_lines(resp, charset):
            if data_str == "[DONE]":
                break
            try:
                obj: Any = json.loads(data_str)
            except json.JSONDecodeError:
                continue
            if not isinstance(obj, dict):
                continue
            if "error" in obj:
                raise RuntimeError(obj.get("error", obj))
            piece = _sse_delta_content(obj)
            if piece:
                collected.append(piece)
                out.write(piece)
                out.flush()
            # function_call и прочее в потоке — редко; при пустом content пропускаем
        return "".join(collected)
    finally:
        resp.close()


def main() -> int:
    parser = argparse.ArgumentParser(description="Консольный чат с GigaChat API")
    parser.add_argument(
        "--credentials",
        default=os.environ.get("GIGACHAT_CREDENTIALS", ""),
        help="Ключ авторизации Base64 (или env GIGACHAT_CREDENTIALS)",
    )
    parser.add_argument(
        "--scope",
        default=os.environ.get("GIGACHAT_SCOPE", DEFAULT_SCOPE),
        help=f"OAuth scope (по умолчанию {DEFAULT_SCOPE})",
    )
    parser.add_argument(
        "--base-url",
        default=os.environ.get("GIGACHAT_BASE_URL", DEFAULT_CHAT_BASE),
        help="Базовый URL API v1",
    )
    parser.add_argument("--model", default=DEFAULT_MODEL, help="Идентификатор модели")
    parser.add_argument(
        "--system",
        default="",
        help="Необязательный системный промпт для всей сессии",
    )
    parser.add_argument(
        "--no-ssl-verify",
        action="store_true",
        help="Отключить проверку TLS (macOS без НУЦ; аналог env GIGACHAT_VERIFY_SSL_CERTS=false)",
    )
    parser.add_argument(
        "--ca-bundle",
        default=os.environ.get("GIGACHAT_CA_BUNDLE_FILE", "") or None,
        help="Путь к PEM с корнем НУЦ (или env GIGACHAT_CA_BUNDLE_FILE)",
    )
    parser.add_argument("--temperature", type=float, default=None, help="Temperature для запросов")
    parser.add_argument(
        "--no-stream",
        action="store_true",
        help="Отключить SSE: ждать ответ целиком (без потока в консоль)",
    )
    parser.add_argument(
        "--update-interval",
        type=float,
        default=None,
        help="Интервал между чанками в потоке (сек), см. API update_interval",
    )
    args = parser.parse_args()

    ssl_verify_off = args.no_ssl_verify or not _env_wants_ssl_verify()

    creds = (args.credentials or "").strip()
    if not creds:
        print(
            "Укажите ключ: переменная GIGACHAT_CREDENTIALS или --credentials",
            file=sys.stderr,
        )
        return 1

    ssl_ctx = _ssl_context(ssl_verify_off, args.ca_bundle)
    if not ssl_verify_off and not args.ca_bundle:
        print(
            "Подсказка: на многих системах без сертификата НУЦ TLS к шлюзу Сбера падает.\n"
            "  Скачайте корень: https://developers.sber.ru/docs/ru/gigachat/certificates\n"
            "  Или временно: --no-ssl-verify  или  export GIGACHAT_VERIFY_SSL_CERTS=false",
            file=sys.stderr,
        )

    messages: list[dict[str, str]] = []
    if args.system.strip():
        messages.append({"role": "system", "content": args.system.strip()})

    token_state: TokenState | None = None

    def ensure_token() -> str:
        nonlocal token_state
        now = time.time()
        if token_state is None or (
            token_state.expires_at_unix > 0 and now > token_state.expires_at_unix - 60
        ):
            token_state = fetch_token(creds, args.scope, ssl_ctx)
        return token_state.access_token

    stream_on = not args.no_stream
    print("GigaChat консоль. Команды: /exit, /clear, /model <имя>, /stream, /nostream")
    print(
        f"Модель: {args.model} | base: {args.base_url} | поток: {'да' if stream_on else 'нет'}",
    )

    while True:
        try:
            line = input("Вы> ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            break
        if not line:
            continue
        if line in ("/exit", "/quit"):
            break
        if line == "/clear":
            messages.clear()
            if args.system.strip():
                messages.append({"role": "system", "content": args.system.strip()})
            print("(история очищена)")
            continue
        if line.startswith("/model "):
            args.model = line.split(" ", 1)[1].strip() or args.model
            print(f"Модель: {args.model}")
            continue
        if line == "/stream":
            stream_on = True
            print("Потоковый вывод включён.")
            continue
        if line == "/nostream":
            stream_on = False
            print("Потоковый вывод выключён (ответ целиком после генерации).")
            continue

        messages.append({"role": "user", "content": line})
        try:
            tok = ensure_token()
            if stream_on:
                print("GigaChat> ", end="", flush=True)
                reply = chat_completion_stream(
                    args.base_url,
                    tok,
                    args.model,
                    messages,
                    ssl_ctx,
                    temperature=args.temperature,
                    update_interval=args.update_interval,
                )
                print()
            else:
                reply = chat_completion(
                    args.base_url,
                    tok,
                    args.model,
                    messages,
                    ssl_ctx,
                    temperature=args.temperature,
                )
                print(f"GigaChat> {reply}\n")
        except Exception as e:
            err = str(e)
            if "CERTIFICATE_VERIFY_FAILED" in err or "certificate verify failed" in err.lower():
                print(
                    "TLS: установите корень НУЦ (--ca-bundle) или отключите проверку:\n"
                    "  python gigachat_console.py --no-ssl-verify\n"
                    "  export GIGACHAT_VERIFY_SSL_CERTS=false\n"
                    "Документация: https://developers.sber.ru/docs/ru/gigachat/certificates",
                    file=sys.stderr,
                )
            print(f"Ошибка: {e}", file=sys.stderr)
            messages.pop()
            continue

        if not reply and stream_on:
            print("(пустой ответ)\n", file=sys.stderr)
        messages.append({"role": "assistant", "content": reply})

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
