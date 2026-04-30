"""Fast heuristic speech-duration estimate (no heavy deps).

Import and call :func:`estimate_sentence_duration_ms` from workers code (e.g. TTS after
razdel). Optional ``langid`` improves Latin-script detection.

Manual check from ``agents/custom-plugins``::

    uv run python speech_duration_estimate.py --calibrate "Hello, world."
"""

from __future__ import annotations

import argparse
import re
import time
import unicodedata
from typing import Any

LANG_PRESETS: dict[str, tuple[float, float, float, float]] = {
    # Slavic / Cyrillic
    "ru": (55, 25, 120, 280),
    "uk": (56, 25, 120, 280),
    "be": (56, 25, 120, 280),
    "bg": (54, 24, 110, 270),
    "sr": (54, 24, 110, 270),
    "mk": (54, 24, 110, 270),
    "bs": (53, 23, 110, 260),
    "hr": (53, 23, 110, 260),
    "sl": (52, 23, 105, 250),
    "cs": (52, 22, 100, 240),
    "sk": (52, 22, 100, 240),
    "pl": (52, 22, 100, 240),
    # Germanic / Romance
    "en": (48, 20, 100, 220),
    "de": (50, 21, 105, 230),
    "nl": (50, 21, 100, 225),
    "sv": (49, 20, 100, 220),
    "no": (49, 20, 100, 220),
    "da": (49, 20, 100, 220),
    "is": (50, 21, 105, 225),
    "fr": (50, 21, 110, 230),
    "it": (49, 21, 100, 220),
    "es": (50, 21, 105, 230),
    "pt": (50, 21, 105, 230),
    "ro": (50, 21, 105, 230),
    "ca": (50, 21, 105, 230),
    "gl": (50, 21, 105, 230),
    # Uralic / Baltic
    "fi": (52, 22, 100, 240),
    "et": (52, 22, 100, 240),
    "hu": (52, 22, 100, 240),
    "lv": (51, 22, 100, 235),
    "lt": (51, 22, 100, 235),
    # Greek / Albanian
    "el": (52, 22, 105, 240),
    "sq": (51, 22, 100, 235),
    # Semitic
    "ar": (57, 26, 130, 300),
    "he": (52, 23, 110, 250),
    # Indic
    "hi": (56, 24, 120, 280),
    "bn": (56, 24, 120, 280),
    "pa": (56, 24, 120, 280),
    "gu": (56, 24, 120, 280),
    "mr": (56, 24, 120, 280),
    "ne": (56, 24, 120, 280),
    "ur": (57, 25, 125, 290),
    "ta": (57, 24, 120, 290),
    "te": (57, 24, 120, 290),
    "kn": (57, 24, 120, 290),
    "ml": (57, 24, 120, 290),
    "si": (57, 24, 120, 290),
    # SEA
    "id": (51, 22, 100, 230),
    "ms": (51, 22, 100, 230),
    "tl": (51, 22, 100, 230),
    "vi": (54, 23, 105, 250),
    "th": (58, 24, 120, 300),
    "km": (58, 24, 120, 300),
    "lo": (58, 24, 120, 300),
    "my": (58, 24, 120, 300),
    # East Asian
    "zh": (85, 8, 120, 260),
    "ja": (80, 10, 110, 240),
    "ko": (75, 12, 110, 240),
    # Turkic / Caucasus
    "tr": (51, 22, 100, 230),
    "az": (52, 22, 100, 235),
    "kk": (54, 24, 110, 260),
    "uz": (52, 22, 100, 235),
    "ky": (54, 24, 110, 260),
    "ka": (56, 24, 110, 270),
    "hy": (56, 24, 110, 270),
    # Iranian
    "fa": (56, 24, 120, 280),
    "ps": (57, 25, 125, 290),
    # African major
    "sw": (52, 22, 100, 235),
    "am": (58, 25, 120, 300),
    "ha": (53, 23, 105, 245),
    "yo": (53, 23, 105, 245),
    "ig": (53, 23, 105, 245),
    "zu": (53, 23, 105, 245),
}

LANG_ALIASES: dict[str, str] = {
    "af": "nl",
    "lb": "de",
    "fy": "nl",
    "ga": "en",
    "gd": "en",
    "cy": "en",
    "mt": "it",
    "oc": "fr",
    "eo": "es",
    "la": "it",
    "mo": "ro",
    "as": "hi",
    "or": "hi",
    "sa": "hi",
    "sd": "ur",
    "jv": "id",
    "su": "id",
    "ceb": "tl",
    "tk": "tr",
    "tt": "kk",
    "rw": "sw",
    "so": "sw",
    "xh": "zu",
    "sn": "sw",
    "st": "sw",
    "ny": "sw",
    "ln": "sw",
    "mn": "kk",
    "eu": "es",
}

CALIBRATION_MULTIPLIER: dict[str, float] = {
    "ru": 1.08,
    "en": 1.12,
    "es": 1.08,
    "de": 1.10,
    "fr": 1.10,
    "it": 1.10,
    "pt": 1.10,
    "nl": 1.10,
    "ar": 1.12,
    "sv": 1.12,
}


def resolve_lang_preset(lang_code: str) -> tuple[float, float, float, float]:
    code = (lang_code or "en").lower().split("-")[0]
    if code in LANG_PRESETS:
        return LANG_PRESETS[code]
    aliased = LANG_ALIASES.get(code)
    if aliased and aliased in LANG_PRESETS:
        return LANG_PRESETS[aliased]
    return LANG_PRESETS["en"]


def _has_range(text: str, start: int, end: int) -> bool:
    return any(start <= ord(ch) <= end for ch in text)


def detect_language_fast(text: str) -> tuple[str, str]:
    """Return ``(lang_code, source)`` for preset lookup."""
    clean = text.strip()
    if not clean:
        return "en", "default"
    try:
        import langid  # type: ignore[import-untyped]

        code, _score = langid.classify(clean)
        if code:
            return code.lower(), "langid"
    except Exception:
        pass

    if _has_range(clean, 0x4E00, 0x9FFF):
        return "zh", "script"
    if _has_range(clean, 0x3040, 0x30FF):
        return "ja", "script"
    if _has_range(clean, 0xAC00, 0xD7AF):
        return "ko", "script"
    if _has_range(clean, 0x0400, 0x04FF):
        return "ru", "script"
    if _has_range(clean, 0x0600, 0x06FF):
        return "ar", "script"
    if _has_range(clean, 0x0590, 0x05FF):
        return "he", "script"
    if _has_range(clean, 0x0900, 0x097F):
        return "hi", "script"
    if _has_range(clean, 0x0E00, 0x0E7F):
        return "th", "script"
    return "en", "default"


def language_hint_to_code(hint: str | None) -> str | None:
    """Map gateway ``language_type`` (e.g. ``ru-RU``) to a preset code, if known."""
    raw = (hint or "").strip().lower()
    if not raw:
        return None
    code = raw.split("-", 1)[0].split("_", 1)[0]
    if len(code) < 2:
        return None
    if code in LANG_PRESETS or code in LANG_ALIASES:
        return code
    return None


def count_units(text: str) -> int:
    cjk_like = 0
    for ch in text:
        code = ord(ch)
        if (0x4E00 <= code <= 0x9FFF) or (0x3400 <= code <= 0x4DBF) or (0xAC00 <= code <= 0xD7AF):
            cjk_like += 1
    if cjk_like:
        return cjk_like
    return sum(ch.isalpha() for ch in text)


def estimate_duration_ms(
    text: str,
    *,
    ms_per_letter: float,
    ms_per_word: float,
    ms_minor_pause: float,
    ms_major_pause: float,
) -> tuple[int, dict[str, int]]:
    units = count_units(text)
    words = len(re.findall(r"\b\w+\b", text, flags=re.UNICODE))
    if words <= 1 and units > 1 and any(unicodedata.east_asian_width(ch) in ("W", "F") for ch in text):
        words = max(1, units // 2)
    minor = len(re.findall(r"[,;:]", text))
    major = len(re.findall(r"[.!?…]", text))

    total = int(
        units * ms_per_letter
        + words * ms_per_word
        + minor * ms_minor_pause
        + major * ms_major_pause
    )
    return total, {"units": units, "words": words, "minor": minor, "major": major}


def estimate_sentence_duration_ms(
    text: str,
    *,
    lang: str = "auto",
    lang_hint: str | None = None,
    calibrate: bool = True,
) -> tuple[int, dict[str, Any]]:
    """Rough duration for one TTS sentence.

    Precedence: explicit ``lang`` (not ``auto``) → ``lang_hint`` if recognized →
    :func:`detect_language_fast`.
    """
    line = text.strip()
    if not line:
        return 0, {"lang": "en", "lang_source": "empty", "duration_ms": 0, "raw_ms": 0, "cal_mult": 1.0}

    detected_lang: str
    lang_source: str
    if lang and lang != "auto":
        detected_lang = lang.lower().split("-")[0]
        lang_source = "arg"
    elif (hint_code := language_hint_to_code(lang_hint)) is not None:
        detected_lang = hint_code
        lang_source = "hint"
    else:
        detected_lang, lang_source = detect_language_fast(line)

    u, w, mi, ma = resolve_lang_preset(detected_lang)
    raw_ms, stats = estimate_duration_ms(
        line,
        ms_per_letter=u,
        ms_per_word=w,
        ms_minor_pause=mi,
        ms_major_pause=ma,
    )
    code_key = detected_lang.lower().split("-")[0]
    cal_mult = CALIBRATION_MULTIPLIER.get(code_key, 1.0) if calibrate else 1.0
    total_ms = int(round(raw_ms * cal_mult))
    detail: dict[str, Any] = {
        "lang": detected_lang,
        "lang_source": lang_source,
        "raw_ms": raw_ms,
        "duration_ms": total_ms,
        "cal_mult": cal_mult,
        **stats,
    }
    return total_ms, detail


def _cli_main() -> None:
    parser = argparse.ArgumentParser(description="Fast rough speech duration from text.")
    parser.add_argument("text", nargs="?", default="", help="Text to estimate (or use -i)")
    parser.add_argument("-i", "--interactive", action="store_true", help="Interactive mode")
    parser.add_argument(
        "--lang",
        default="auto",
        help="Language code or auto. Use 'list' for preset codes.",
    )
    parser.add_argument("--ms-per-unit", type=float, default=None)
    parser.add_argument("--ms-per-word", type=float, default=None)
    parser.add_argument("--ms-minor-pause", type=float, default=None)
    parser.add_argument("--ms-major-pause", type=float, default=None)
    parser.add_argument("--calibrate", action="store_true")
    parser.add_argument("--calibration-mult", type=float, default=None)
    args = parser.parse_args()

    if args.lang == "list":
        supported = sorted(set(LANG_PRESETS) | set(LANG_ALIASES))
        print("Supported preset/alias codes:")
        print(" ".join(supported))
        return

    def run_one(line: str) -> None:
        line = line.strip()
        if not line:
            return
        t0 = time.perf_counter()
        if (
            args.ms_per_unit is None
            and args.ms_per_word is None
            and args.ms_minor_pause is None
            and args.ms_major_pause is None
            and args.calibration_mult is None
        ):
            lang_kw = args.lang if args.lang != "auto" else "auto"
            total_ms, detail = estimate_sentence_duration_ms(
                line,
                lang=lang_kw,
                lang_hint=None,
                calibrate=args.calibrate,
            )
            elapsed_ms = (time.perf_counter() - t0) * 1000.0
            print(f"text:        {line!r}")
            print(f"lang:        {detail['lang']} ({detail['lang_source']})")
            print(
                f"counts:      units={detail['units']} words={detail['words']} "
                f"minor={detail['minor']} major={detail['major']}"
            )
            print(f"calibration: mult={detail['cal_mult']:.3f}")
            print(f"estimate:    {total_ms} ms ({total_ms / 1000.0:.3f} s)")
            if args.calibrate and detail["cal_mult"] != 1.0:
                print(f"raw_estimate:{detail['raw_ms']} ms ({detail['raw_ms'] / 1000.0:.3f} s)")
            print(f"compute_ms:  {elapsed_ms:.3f}\n")
            return

        detected_lang, source = (args.lang, "arg")
        if args.lang == "auto":
            detected_lang, source = detect_language_fast(line)
        u, w, mi, ma = resolve_lang_preset(detected_lang)
        ms_u = args.ms_per_unit if args.ms_per_unit is not None else u
        ms_w = args.ms_per_word if args.ms_per_word is not None else w
        ms_mi = args.ms_minor_pause if args.ms_minor_pause is not None else mi
        ms_ma = args.ms_major_pause if args.ms_major_pause is not None else ma
        code = detected_lang.lower().split("-")[0]
        cal_mult = CALIBRATION_MULTIPLIER.get(code, 1.0) if args.calibrate else 1.0
        if args.calibration_mult is not None:
            cal_mult *= args.calibration_mult
        raw_ms, stats = estimate_duration_ms(
            line,
            ms_per_letter=ms_u,
            ms_per_word=ms_w,
            ms_minor_pause=ms_mi,
            ms_major_pause=ms_ma,
        )
        total_ms = int(round(raw_ms * cal_mult))
        elapsed_ms = (time.perf_counter() - t0) * 1000.0
        print(f"text:        {line!r}")
        print(f"lang:        {detected_lang} ({source})")
        print(f"coeffs_ms:   unit={ms_u} word={ms_w} minor={ms_mi} major={ms_ma}")
        print(f"calibration: mult={cal_mult:.3f}")
        print(
            f"counts:      units={stats['units']} words={stats['words']} "
            f"minor={stats['minor']} major={stats['major']}"
        )
        print(f"estimate:    {total_ms} ms ({total_ms / 1000.0:.3f} s)")
        if cal_mult != 1.0:
            print(f"raw_estimate:{raw_ms} ms ({raw_ms / 1000.0:.3f} s)")
        print(f"compute_ms:  {elapsed_ms:.3f}\n")

    if args.interactive or not args.text:
        print("Enter text (empty line to exit):")
        while True:
            try:
                line = input("> ")
            except EOFError:
                break
            if not line.strip():
                break
            run_one(line)
    else:
        run_one(args.text)


if __name__ == "__main__":
    _cli_main()
