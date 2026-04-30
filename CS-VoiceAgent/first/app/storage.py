from __future__ import annotations

import json
import os
import time
from dataclasses import asdict, dataclass
from typing import Any

from .config import settings, load_settings


@dataclass
class Voiceprint:
    id: str
    preferred_name: str
    target_model: str
    created_at: float
    transcript: str | None = None


def _ensure_data_dir() -> None:
    s = load_settings()
    os.makedirs(s.data_dir, exist_ok=True)


def load_voiceprints() -> list[Voiceprint]:
    _ensure_data_dir()
    s = load_settings()
    if not os.path.exists(s.voices_json):
        return []
    with open(s.voices_json, "r", encoding="utf-8") as f:
        raw = json.load(f)
    out: list[Voiceprint] = []
    for item in raw if isinstance(raw, list) else []:
        try:
            out.append(
                Voiceprint(
                    id=str(item["id"]),
                    preferred_name=str(item.get("preferred_name", "")),
                    target_model=str(item.get("target_model", "")),
                    created_at=float(item.get("created_at", 0.0)),
                    transcript=(str(item["transcript"]) if item.get("transcript") is not None else None),
                )
            )
        except Exception:
            continue
    return out


def save_voiceprints(voices: list[Voiceprint]) -> None:
    _ensure_data_dir()
    s = load_settings()
    with open(s.voices_json, "w", encoding="utf-8") as f:
        json.dump([asdict(v) for v in voices], f, ensure_ascii=False, indent=2)


def upsert_voiceprint(v: Voiceprint) -> None:
    voices = load_voiceprints()
    idx = next((i for i, it in enumerate(voices) if it.id == v.id), None)
    if idx is None:
        voices.append(v)
    else:
        voices[idx] = v
    save_voiceprints(voices)


def new_voiceprint(id: str, preferred_name: str, target_model: str, transcript: str | None = None) -> Voiceprint:
    return Voiceprint(
        id=id,
        preferred_name=preferred_name,
        target_model=target_model,
        created_at=time.time(),
        transcript=transcript,
    )

