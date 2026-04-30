"""Streaming sentence segmentation for LLM token streams.

Wraps :func:`razdel.sentenize` for incremental sentence boundary detection. Whitespace inside a
sentence is preserved as-is (critical for TTS prosody).

**Eager split:** a clause ending with ``[!?…]{1,}`` optionally followed by ASCII ``...`` (so
``?...``, ``!...``, ``!?...``, ``?!...``, etc.) and then whitespace is emitted immediately — we do
not wait for the next razdel-stable sentence. Unicode ``…`` alone still counts. The same pattern at
**end of buffer** (only surrounding whitespace) flushes early (e.g. ``Привет!``). Bare trailing
``...`` is also a terminal for phrases like ``Подожди...``.

**Last sentence:** when the LLM stream ends, :meth:`flush` runs and emits whatever remains (no
``!/?/…`` required). Nothing is lost at end-of-reply.

razdel still handles abbreviations (``т. д.``, ``3.14``, …) for the *remainder* after eager cuts
and on :meth:`flush`.
"""

from __future__ import annotations

import re

from razdel import sentenize

# Conservative emoji/pictographic stripper — covers the common BMP/SMP blocks used in chat output.
_EMOJI_RE = re.compile(
    "["
    "\U0001f300-\U0001faff"
    "\U00002600-\U000027bf"
    "\U0001f000-\U0001f2ff"
    "\u2700-\u27bf"
    "\u2300-\u23ff"
    "\ufe0f"
    "]+",
    flags=re.UNICODE,
)

# After one of these, allow repeated !/?/… (e.g. "Really?!") before optional "..." then whitespace.
_EAGER_CLUSTER = frozenset("!?…")


def _eager_punct_run_end(buf: str, start: int) -> int:
    """``buf[start]`` must be in ``_EAGER_CLUSTER``. Return exclusive end after ``[!?…]+`` and optional ``...``."""
    n = len(buf)
    j = start
    while j < n and buf[j] in _EAGER_CLUSTER:
        j += 1
    if j + 3 <= n and buf[j : j + 3] == "...":
        j += 3
    return j


# Suffix that closes an eager "sentence" for terminal detection (end of buffer or before we rely on flush).
_EAGER_TERMINAL_SUFFIX = re.compile(
    r"(?:[!?…]{1,}(?:\.{3})?|\.\.\.|…)$",
)


def _ends_with_eager_terminal(t: str) -> bool:
    if len(t) < 2:
        return False
    return _EAGER_TERMINAL_SUFFIX.search(t) is not None


def _take_eager_clause_before_space(buf: str) -> tuple[str | None, str]:
    """``prefix + ([!?…]+(?:...)?) + whitespace + rest`` → ``(prefix+punct, rest)``."""
    n = len(buf)
    i = 0
    while i < n:
        if buf[i] not in _EAGER_CLUSTER:
            i += 1
            continue
        j = _eager_punct_run_end(buf, i)
        if j < n and buf[j].isspace():
            clause = buf[:j].strip()
            if clause:
                return clause, buf[j:]
        i += 1
    return None, buf


def _take_eager_terminal(buf: str) -> tuple[str | None, str]:
    """Buffer is only (optional) surrounding whitespace around ``t``, and ``t`` ends with !/?/…."""
    t = buf.strip()
    if len(t) < 2 or not _ends_with_eager_terminal(t):
        return None, buf
    return t, ""


class StreamingSentencizer:
    """Incremental sentence segmenter: eager ``[!?…]+`` / optional ``...`` + razdel for the rest."""

    def __init__(self, *, remove_emoji: bool = False) -> None:
        self._buf: str = ""
        self._remove_emoji = remove_emoji

    def push(self, text: str) -> list[str]:
        """Append ``text`` and return completed sentences (eager + razdel-stable)."""
        if not text:
            return []
        if self._remove_emoji:
            text = _EMOJI_RE.sub("", text)
            if not text:
                return []
        self._buf += text
        out: list[str] = []

        while True:
            clause, rest = _take_eager_clause_before_space(self._buf)
            if clause is not None:
                out.append(clause)
                self._buf = rest if not rest.isspace() else ""
                continue
            term, _rest = _take_eager_terminal(self._buf)
            if term is not None:
                out.append(term)
                self._buf = ""
                continue
            break

        sents = list(sentenize(self._buf))
        if len(sents) <= 1:
            return out
        stable = [s.text for s in sents[:-1]]
        last = sents[-1]
        self._buf = self._buf[last.start :]
        return out + stable

    def flush(self) -> list[str]:
        """Emit remaining text (last sentence needs no following clause)."""
        if not self._buf.strip():
            self._buf = ""
            return []
        out = [s.text for s in sentenize(self._buf) if s.text.strip()]
        self._buf = ""
        return out
