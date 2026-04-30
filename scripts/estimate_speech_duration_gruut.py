#!/usr/bin/env python3
"""Rough speech duration from text using gruut (IPA + simple duration table).

gruut tokenizes and phonemizes via lexicon / G2P — **no espeak-ng required**
unless you pass ``--espeak`` (then espeak-derived pronunciations are used).

  pip install gruut
  pip install gruut-lang-ru

**Speed:** Import + first ``TextProcessor`` build costs ~200–400 ms **per new Python
process**. gruut caches the processor per thread; after a one-line warmup, the next
calls are typically tens of ms. In the **voice worker** (long-lived process) call
gruut from the same thread after startup warmup — not from a fresh subprocess each time.

  --fast  → no gruut, ~0 ms: letters + punctuation heuristic (rough).

See: https://rhasspy.github.io/gruut/
"""

from __future__ import annotations

import argparse
import re
import sys
import time


def _warmup_gruut(lang: str, *, use_espeak: bool, pos: bool) -> float:
    """Prime gruut's thread-local TextProcessor; return seconds spent."""
    from gruut import sentences

    t0 = time.perf_counter()
    # One token in target lang so lexicon/g2p for that lang loads.
    _ = list(sentences("а", lang=lang, espeak=use_espeak, pos=pos))
    return time.perf_counter() - t0


def _count_phonemes_and_breaks(
    text: str,
    lang: str,
    *,
    use_espeak: bool,
    pos: bool,
) -> tuple[int, int, int, int, list[str]]:
    from gruut import sentences

    phoneme_count = 0
    stress_marks = 0
    major_breaks = 0
    minor_breaks = 0
    flat: list[str] = []

    for sent in sentences(text, lang=lang, espeak=use_espeak, pos=pos):
        for word in sent:
            if getattr(word, "is_major_break", False):
                major_breaks += 1
                continue
            if getattr(word, "is_minor_break", False):
                minor_breaks += 1
                continue
            phones = getattr(word, "phonemes", None) or []
            for p in phones:
                if p in ("|", "‖", "‿"):
                    continue
                flat.append(p)
                phoneme_count += 1
                if "ˈ" in p:
                    stress_marks += 1

    return phoneme_count, stress_marks, major_breaks, minor_breaks, flat


def _estimate_fast(
    text: str,
    *,
    ms_per_letter: float,
    ms_major_pause: float,
    ms_minor_pause: float,
) -> tuple[int, int, int, int, list[str]]:
    """Letters + naive punctuation counts; no phoneme list."""
    letters = sum(1 for c in text if c.isalpha())
    minor = len(re.findall(r"[,;:]", text))
    major = len(re.findall(r"[.!?…]", text))
    # proxy "phoneme count" = letters for display compatibility
    return letters, 0, major, minor, []


def main() -> None:
    parser = argparse.ArgumentParser(description="Estimate phrase duration via gruut + heuristic table.")
    parser.add_argument("text", nargs="?", default="", help="Phrase (or use -i)")
    parser.add_argument("-l", "--language", default="ru", help="gruut lang code (default: ru)")
    parser.add_argument("--ms-per-phoneme", type=float, default=65.0, help="ms per phoneme token (default: 65)")
    parser.add_argument("--ms-major-pause", type=float, default=350.0, help="ms after major break (default: 350)")
    parser.add_argument("--ms-minor-pause", type=float, default=120.0, help="ms after minor break (default: 120)")
    parser.add_argument(
        "--ms-stress-extra",
        type=float,
        default=25.0,
        help="extra ms per primary-stress marker (ˈ) in a phoneme string (default: 25)",
    )
    parser.add_argument(
        "--ms-per-letter",
        type=float,
        default=62.0,
        help="with --fast: ms per alphabetic character (default: 62)",
    )
    parser.add_argument(
        "--espeak",
        action="store_true",
        help="use gruut's espeak-derived pronunciations (needs espeak-ng on PATH)",
    )
    parser.add_argument(
        "--pos",
        action="store_true",
        help="enable POS tagging in gruut (slower; default off)",
    )
    parser.add_argument(
        "--no-warmup",
        action="store_true",
        help="skip gruut warmup (measures cold path; first run still loads imports)",
    )
    parser.add_argument(
        "--fast",
        action="store_true",
        help="no gruut: letters + punctuation only (very fast, less accurate)",
    )
    parser.add_argument("-i", "--interactive", action="store_true", help="read lines from stdin")
    args = parser.parse_args()

    def estimate_one(line: str, *, gruut_ready: bool) -> None:
        line = line.strip()
        if not line:
            return
        try:
            t0 = time.perf_counter()
            if args.fast:
                n_ph, n_str, maj, minr, flat = _estimate_fast(
                    line,
                    ms_per_letter=args.ms_per_letter,
                    ms_major_pause=args.ms_major_pause,
                    ms_minor_pause=args.ms_minor_pause,
                )
                # reuse ms_per_phoneme as scale for "letters" in fast mode
                body_ms = n_ph * args.ms_per_letter
            else:
                n_ph, n_str, maj, minr, flat = _count_phonemes_and_breaks(
                    line,
                    args.language,
                    use_espeak=args.espeak,
                    pos=args.pos,
                )
                body_ms = n_ph * args.ms_per_phoneme
            process_s = time.perf_counter() - t0
        except ImportError:
            print("Install: pip install gruut", file=sys.stderr)
            raise SystemExit(1) from None
        except Exception as e:
            print(f"gruut error: {e}", file=sys.stderr)
            return

        stress_ms = n_str * args.ms_stress_extra
        pause_ms = maj * args.ms_major_pause + minr * args.ms_minor_pause
        total_ms = body_ms + stress_ms + pause_ms

        mode = "fast" if args.fast else "gruut"
        print(f"text:          {line!r}")
        print(f"mode:          {mode}  lang={args.language!r}  espeak={args.espeak}  pos={args.pos}")
        if args.fast:
            print(f"phonemes:      (fast mode — no IPA; letters={n_ph})")
        else:
            print(f"phonemes:      {flat}")
        print(f"counts:        phones={n_ph}  stress_ˈ={n_str}  major_breaks={maj}  minor_breaks={minr}")
        if not args.fast and n_ph == 0 and any(ch.isalpha() for ch in line):
            print(
                "WARNING: zero phonemes but text has letters — install language data, e.g.\n"
                "  pip install gruut-lang-ru\n"
                f"  (current -l {args.language!r})",
                file=sys.stderr,
            )
        print(
            f"estimate:      {total_ms:.0f} ms ({total_ms / 1000.0:.3f} s)  "
            f"[body {body_ms:.0f} + stress {stress_ms:.0f} + pauses {pause_ms:.0f}]"
        )
        note = "gruut + counting"
        if not args.fast and not gruut_ready:
            note += " (includes cold processor; use default warmup or worker process)"
        print(f"process_time:   {process_s * 1000.0:.2f} ms  ({note})")
        print()

    if args.interactive or not args.text:
        print("Enter text lines (empty line to exit):", file=sys.stderr)
        gruut_ready = False
        if not args.fast and not args.no_warmup:
            w = _warmup_gruut(args.language, use_espeak=args.espeak, pos=args.pos)
            print(f"(warmup {w * 1000:.1f} ms)", file=sys.stderr)
            gruut_ready = True
        while True:
            try:
                line = input("> ").strip()
            except EOFError:
                break
            if not line:
                break
            estimate_one(line, gruut_ready=gruut_ready)
    else:
        gruut_ready = False
        if not args.fast and not args.no_warmup:
            w = _warmup_gruut(args.language, use_espeak=args.espeak, pos=args.pos)
            print(f"(warmup {w * 1000:.1f} ms)", file=sys.stderr)
            gruut_ready = True
        estimate_one(args.text, gruut_ready=gruut_ready)


if __name__ == "__main__":
    main()
