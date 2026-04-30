#!/usr/bin/env python3
"""Parallel comparison: fast estimator vs gruut estimator.

Default workload:
- 10 popular languages
- 100 phrases per language
- total 1000 phrases

Per phrase output:
1) fast estimate (ms)
2) gruut estimate (ms)
3) relative delta for fast vs gruut

Delta formula:
    delta_pct = (gruut_ms - fast_ms) / gruut_ms * 100

Interpretation:
- positive: fast predicts shorter duration than gruut (e.g. +20%)
- negative: fast predicts longer duration than gruut (e.g. -20%)
"""

from __future__ import annotations

import argparse
import concurrent.futures as cf
import importlib.util
import pathlib
import statistics
import time
from dataclasses import dataclass
from itertools import cycle, islice


SCRIPT_DIR = pathlib.Path(__file__).resolve().parent
_REPO_ROOT = SCRIPT_DIR.parent
FAST_PATH = (
    _REPO_ROOT
    / "CS-VoiceAgent"
    / "second"
    / "Workers"
    / "agents"
    / "custom-plugins"
    / "speech_duration_estimate.py"
)
GRUUT_PATH = SCRIPT_DIR / "estimate_speech_duration_gruut.py"


def _load_module(name: str, path: pathlib.Path):
    spec = importlib.util.spec_from_file_location(name, str(path))
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load module from {path}")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)  # type: ignore[attr-defined]
    return module


@dataclass
class Row:
    idx: int
    phrase: str
    lang: str
    fast_ms: float
    gruut_ms: float
    fast_compute_ms: float
    gruut_compute_ms: float

    @property
    def delta_pct(self) -> float:
        if self.gruut_ms <= 0:
            return 0.0
        return (self.gruut_ms - self.fast_ms) / self.gruut_ms * 100.0


PHRASES_BY_LANG: dict[str, list[str]] = {
    "en": [
        "Hello, how are you today?",
        "Let's compare two duration estimators.",
        "I need a quick approximation right now.",
        "Please repeat the previous sentence.",
        "How long will this response take?",
        "The system must stay responsive under load.",
        "Punctuation should add realistic pauses.",
        "Let's run a larger benchmark in parallel.",
        "Please show percentage difference clearly.",
        "Great, the comparison looks stable.",
    ],
    "ru": [
        "Привет, как дела сегодня?",
        "Давай сравним два оценщика длительности.",
        "Мне нужна быстрая приблизительная оценка.",
        "Пожалуйста, повтори предыдущее предложение.",
        "Сколько времени займет этот ответ?",
        "Система должна оставаться отзывчивой под нагрузкой.",
        "Нужно учитывать паузы и знаки препинания.",
        "Давай запустим большой параллельный бенчмарк.",
        "Покажи процентную разницу максимально ясно.",
        "Отлично, сравнение выглядит стабильным.",
    ],
    "es": [
        "Hola, ¿como estas hoy?",
        "Comparemos dos estimadores de duracion.",
        "Necesito una estimacion rapida ahora mismo.",
        "Por favor, repite la frase anterior.",
        "¿Cuanto tardara esta respuesta?",
        "El sistema debe seguir siendo estable bajo carga.",
        "La puntuacion debe agregar pausas realistas.",
        "Hagamos una prueba grande en paralelo.",
        "Muestra la diferencia porcentual con claridad.",
        "Genial, la comparacion se ve estable.",
    ],
    "de": [
        "Hallo, wie geht es dir heute?",
        "Vergleichen wir zwei Dauerschatzer.",
        "Ich brauche jetzt eine schnelle Naherung.",
        "Bitte wiederhole den vorherigen Satz.",
        "Wie lange wird diese Antwort dauern?",
        "Das System muss unter Last reaktionsfahig bleiben.",
        "Die Zeichensetzung sollte realistische Pausen bringen.",
        "Lass uns einen grossen Paralleltest starten.",
        "Zeig die prozentuale Differenz deutlich an.",
        "Super, der Vergleich wirkt stabil.",
    ],
    "fr": [
        "Bonjour, comment vas-tu aujourd'hui ?",
        "Comparons deux estimateurs de duree.",
        "J'ai besoin d'une estimation rapide maintenant.",
        "Repete la phrase precedente, s'il te plait.",
        "Combien de temps prendra cette reponse ?",
        "Le systeme doit rester reactif sous charge.",
        "La ponctuation doit ajouter des pauses realistes.",
        "Lançons un grand benchmark en parallele.",
        "Affiche clairement la difference en pourcentage.",
        "Parfait, la comparaison semble stable.",
    ],
    "it": [
        "Ciao, come stai oggi?",
        "Confrontiamo due stimatori di durata.",
        "Ho bisogno di una stima rapida adesso.",
        "Per favore, ripeti la frase precedente.",
        "Quanto tempo richiedera questa risposta?",
        "Il sistema deve restare reattivo sotto carico.",
        "La punteggiatura deve aggiungere pause realistiche.",
        "Facciamo un grande benchmark in parallelo.",
        "Mostra chiaramente la differenza percentuale.",
        "Ottimo, il confronto sembra stabile.",
    ],
    "pt": [
        "Ola, como voce esta hoje?",
        "Vamos comparar dois estimadores de duracao.",
        "Preciso de uma estimativa rapida agora.",
        "Por favor, repita a frase anterior.",
        "Quanto tempo esta resposta vai levar?",
        "O sistema deve continuar responsivo sob carga.",
        "A pontuacao deve adicionar pausas realistas.",
        "Vamos rodar um benchmark grande em paralelo.",
        "Mostre a diferenca percentual com clareza.",
        "Otimo, a comparacao parece estavel.",
    ],
    "nl": [
        "Hallo, hoe gaat het vandaag?",
        "Laten we twee duur-schatters vergelijken.",
        "Ik heb nu een snelle schatting nodig.",
        "Herhaal alsjeblieft de vorige zin.",
        "Hoe lang zal dit antwoord duren?",
        "Het systeem moet onder belasting responsief blijven.",
        "Interpunctie moet realistische pauzes toevoegen.",
        "Laten we een grote paralleltest draaien.",
        "Toon het procentuele verschil duidelijk.",
        "Prima, de vergelijking lijkt stabiel.",
    ],
    "ar": [
        "مرحبا كيف حالك اليوم",
        "لنقارن مقدرين لمدة الكلام",
        "احتاج تقديرا سريعا الان",
        "من فضلك اعد الجملة السابقة",
        "كم سيستغرق هذا الرد",
        "يجب ان يبقى النظام سريعا تحت الضغط",
        "علامات الترقيم تضيف توقفات واقعية",
        "لنشغل اختبارا كبيرا بالتوازي",
        "اعرض الفرق النسبي بشكل واضح",
        "ممتاز المقارنة تبدو مستقرة",
    ],
    "sv": [
        "Hej, hur mar du idag?",
        "Låt oss jamfora tva varaktighetsmodeller.",
        "Jag behover en snabb uppskattning nu.",
        "Upprepa den forra meningen, tack.",
        "Hur lang tid kommer svaret att ta?",
        "Systemet maste vara responsivt under belastning.",
        "Interpunktion bor ge realistiska pauser.",
        "Låt oss kora ett stort parallelltest.",
        "Visa procentuell skillnad tydligt.",
        "Toppen, jamforelsen ser stabil ut.",
    ],
}


def build_phrases(total_per_lang: int, langs: list[str]) -> list[tuple[str, str]]:
    out: list[tuple[str, str]] = []
    punct = [".", "!", "?", "..."]
    for lang in langs:
        base = PHRASES_BY_LANG[lang]
        for i, p in enumerate(islice(cycle(base), total_per_lang), start=1):
            # Add tiny variation to avoid identical duplicates.
            suffix = punct[i % len(punct)]
            out.append((f"{p} {i}{suffix}", lang))
    return out


def main() -> None:
    parser = argparse.ArgumentParser(description="Compare fast vs gruut duration estimators in parallel.")
    parser.add_argument("--workers", type=int, default=20, help="Thread pool size (default: 20)")
    parser.add_argument("--per-lang", type=int, default=100, help="Phrases per language (default: 100)")
    parser.add_argument(
        "--langs",
        default="en,ru,es,de,fr,it,pt,nl,ar,sv",
        help="Comma-separated language codes from built-in set.",
    )
    parser.add_argument("--print-limit", type=int, default=1000, help="Max rows to print (default: 1000)")
    parser.add_argument("--gruut-ms-per-phoneme", type=float, default=65.0)
    parser.add_argument("--gruut-ms-major-pause", type=float, default=350.0)
    parser.add_argument("--gruut-ms-minor-pause", type=float, default=120.0)
    parser.add_argument("--gruut-ms-stress-extra", type=float, default=25.0)
    args = parser.parse_args()

    fast_mod = _load_module("est_fast", FAST_PATH)
    gruut_mod = _load_module("est_gruut", GRUUT_PATH)

    langs = [x.strip() for x in args.langs.split(",") if x.strip()]
    unknown = [x for x in langs if x not in PHRASES_BY_LANG]
    if unknown:
        raise SystemExit(f"Unsupported langs: {unknown}. Known: {sorted(PHRASES_BY_LANG)}")
    phrases = build_phrases(max(1, args.per_lang), langs)

    # Warmup gruut once in main thread (threads still may have first-call overhead).
    try:
        for l in langs:
            gruut_mod._warmup_gruut(l, use_espeak=False, pos=False)
    except Exception:
        pass

    def run_one(idx_phrase: tuple[int, tuple[str, str]]) -> Row:
        idx, (phrase, lang) = idx_phrase

        t0 = time.perf_counter()
        det_lang, _source = fast_mod.detect_language_fast(phrase)
        a, b, c, d = fast_mod.resolve_lang_preset(det_lang)
        fast_ms, _stats = fast_mod.estimate_duration_ms(
            phrase,
            ms_per_letter=a,
            ms_per_word=b,
            ms_minor_pause=c,
            ms_major_pause=d,
        )
        fast_compute_ms = (time.perf_counter() - t0) * 1000.0

        t1 = time.perf_counter()
        n_ph, n_str, maj, minr, _flat = gruut_mod._count_phonemes_and_breaks(
            phrase,
            lang,
            use_espeak=False,
            pos=False,
        )
        gruut_ms = (
            n_ph * args.gruut_ms_per_phoneme
            + n_str * args.gruut_ms_stress_extra
            + maj * args.gruut_ms_major_pause
            + minr * args.gruut_ms_minor_pause
        )
        gruut_compute_ms = (time.perf_counter() - t1) * 1000.0

        return Row(
            idx=idx + 1,
            phrase=phrase,
            lang=lang,
            fast_ms=float(fast_ms),
            gruut_ms=float(gruut_ms),
            fast_compute_ms=fast_compute_ms,
            gruut_compute_ms=gruut_compute_ms,
        )

    rows: list[Row] = []
    with cf.ThreadPoolExecutor(max_workers=max(1, args.workers)) as ex:
        futures = [ex.submit(run_one, item) for item in enumerate(phrases)]
        for f in cf.as_completed(futures):
            rows.append(f.result())

    rows.sort(key=lambda r: r.idx)

    for r in rows[: max(1, args.print_limit)]:
        print(f"[{r.idx:02d}] ({r.lang}) {r.phrase}")
        print(f"  fast:   {r.fast_ms:.0f} ms   (compute {r.fast_compute_ms:.2f} ms)")
        print(f"  gruut:  {r.gruut_ms:.0f} ms   (compute {r.gruut_compute_ms:.2f} ms)")
        print(f"  delta:  {r.delta_pct:+.1f}%  (plus=fast shorter, minus=fast longer)")
        print()

    deltas = [r.delta_pct for r in rows if r.gruut_ms > 0]
    if deltas:
        print("=== Summary ===")
        print(f"phrases:        {len(rows)}")
        print(f"langs:          {','.join(langs)}")
        print(f"delta mean:     {statistics.mean(deltas):+.2f}%")
        print(f"delta median:   {statistics.median(deltas):+.2f}%")
        print(f"delta min/max:  {min(deltas):+.2f}% / {max(deltas):+.2f}%")
        print(
            f"compute mean:   fast {statistics.mean(r.fast_compute_ms for r in rows):.2f} ms, "
            f"gruut {statistics.mean(r.gruut_compute_ms for r in rows):.2f} ms"
        )


if __name__ == "__main__":
    main()
