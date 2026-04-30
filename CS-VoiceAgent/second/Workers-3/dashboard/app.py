from __future__ import annotations

import asyncio
import json
import time
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Any

from fastapi import FastAPI, Request
from fastapi.responses import HTMLResponse, StreamingResponse
from fastapi.staticfiles import StaticFiles


app = FastAPI(title="Cybrix Worker Trace Dashboard")
app.mount("/static", StaticFiles(directory="static"), name="static")


@dataclass(frozen=True)
class _Client:
    q: asyncio.Queue[str]


_clients: set[_Client] = set()


def _now_ms() -> int:
    return int(time.time() * 1000)


@app.get("/", response_class=HTMLResponse)
async def index() -> str:
    with open("static/index.html", "r", encoding="utf-8") as f:
        return f.read()


@app.post("/emit")
async def emit(req: Request) -> dict[str, Any]:
    payload = await req.json()
    if "ts_ms" not in payload:
        payload["ts_ms"] = _now_ms()
    line = json.dumps(payload, ensure_ascii=False)

    dead: list[_Client] = []
    for c in list(_clients):
        try:
            c.q.put_nowait(line)
        except Exception:
            dead.append(c)
    for c in dead:
        _clients.discard(c)

    return {"ok": True, "clients": len(_clients)}


@app.get("/events")
async def events() -> StreamingResponse:
    client = _Client(q=asyncio.Queue(maxsize=500))
    _clients.add(client)

    async def gen() -> AsyncIterator[bytes]:
        # Initial hello so UI can show connected state
        yield b"event: hello\ndata: {}\n\n"
        try:
            while True:
                msg = await client.q.get()
                yield f"data: {msg}\n\n".encode("utf-8")
        finally:
            _clients.discard(client)

    return StreamingResponse(gen(), media_type="text/event-stream")

