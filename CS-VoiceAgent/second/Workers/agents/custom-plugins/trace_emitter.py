from __future__ import annotations

import asyncio
import json
import os
import time
from typing import Any

import aiohttp

_url: str | None = None
_q: asyncio.Queue[dict[str, Any]] | None = None
_task: asyncio.Task[None] | None = None
_session: aiohttp.ClientSession | None = None


def _now_ms() -> int:
    return int(time.time() * 1000)


def init_from_env() -> None:
    """Initialize async emitter if CYBRIX_TRACE_URL is set."""
    global _url, _q, _task
    if _task is not None:
        return
    u = (os.environ.get("CYBRIX_TRACE_URL") or "").strip()
    if not u:
        return
    _url = u.rstrip("/") + "/emit"
    _q = asyncio.Queue(maxsize=2000)
    _task = asyncio.create_task(_run(), name="cybrix.trace_emitter")


async def _ensure_session() -> aiohttp.ClientSession:
    global _session
    if _session is None:
        _session = aiohttp.ClientSession()
    return _session


async def _run() -> None:
    assert _q is not None
    assert _url is not None
    while True:
        ev = await _q.get()
        try:
            sess = await _ensure_session()
            async with sess.post(_url, json=ev, timeout=aiohttp.ClientTimeout(total=1.5)) as r:
                await r.read()
        except Exception:
            # best-effort: drop on errors
            pass


def emit(
    event_type: str,
    *,
    room: str | None = None,
    job_id: str | None = None,
    pid: int | None = None,
    ts_ms: int | None = None,
    **fields: Any,
) -> None:
    """Fire-and-forget structured event."""
    if _q is None:
        return
    if not room or not job_id:
        try:
            from livekit.agents import get_job_context

            jc = get_job_context(required=False)
            if jc is not None:
                if not room:
                    room = jc.room.name
                if not job_id:
                    job_id = jc.job.id
        except Exception:
            pass
    ev: dict[str, Any] = {"type": event_type, "ts_ms": ts_ms if ts_ms is not None else _now_ms(), **fields}
    if room:
        ev["room"] = room
    if job_id:
        ev["job_id"] = job_id
    if pid is not None:
        ev["pid"] = pid
    try:
        _q.put_nowait(ev)
    except Exception:
        return

