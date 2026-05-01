#!/usr/bin/env python3
import argparse
import base64
import json
import os
import re
import sys
import urllib.error
import urllib.request


DEFAULT_PROMPT = """Ты анализируешь ЗАПИСЬ РАЗГОВОРА. Критично важно разделить два независимых слоя и НЕ смешивать их:
A) ВОКАЛ / ПАРАЛИНГВИСТИКА — только то, как сказано (темп, паузы, высота тона, напряжение голоса, «острота», монотонность при «вежливых» фразах, сарказм, выдохи, сжатые согласные, форсированная вежливость).
B) ЛЕКСИКА — только буквальный смысл слов и фраз (без «додумывания» хорошего настроя из текста, если голос этому противоречит).

Сначала опиши слой A, затем слой B. Если слова звучат благодарно/позитивно, а голос — раздражён, холоден, устал, сухой, рубит фразы, с иронией — это ОБЯЗАТЕЛЬНО высокий mismatch (сарказм, пассивная агрессия, вынужденная вежливость).

Обязательные поля в JSON (имена ключей сохрани):
1) voice_analysis:
- emotions_intensity: объект с числами 0-10 для frustration, irritation, satisfaction, enthusiasm, confusion и при необходимости gratitude, warmth (если уместно)
- vocal_valence: строка negative | neutral | positive (только по голосу, не по тексту)
- vocal_arousal: low | medium | high (возбуждение/напряжение голоса)
- prosody_notes: массив из 3-6 коротких наблюдений именно про звучание (не пересказ текста)
- dynamics_segments: массив {start, end, description} — что происходит с интонацией по времени
- sarcasm_or_passive_aggressive: boolean
- confidence_level, tension_level, urgency, escalation_risk — как раньше (строки или уровни)

2) content_analysis:
- summary, sentiment (positive|neutral|negative), sentiment_score 0-1 — строго по СМЫСЛУ СЛОВ
- transcript_or_key_phrases: массив строк — дословные или близкие к дословным фрагменты того, что сказано (если уверенность низкая, пометь в uncertainty_note)
- intent, churn_triggers

3) mismatch_analysis:
- mismatch: true если vocal_valence и sentiment по тексту явно расходятся ИЛИ sarcasm_or_passive_aggressive true ИЛИ высокие irritation/frustration при позитивных словах
- mismatch_score: 0-10 (0 только если голос и текст согласованы; при твоём кейсе «злым голосом — милые слова» обычно 7-10)
- explanation: почему так, с опорой на prosody_notes vs transcript_or_key_phrases
- listener_risk: кратко — как это может быть воспринято собеседником

4) overall: churn_risk, tactic, recommended_reply_tone, suggested_next_actions (учитывай: при высоком mismatch отвечать нейтрально-спокойно, валидировать эмоцию, не «лить сахар» только из-за вежливых слов)

Ответь строго одним JSON-объектом, без markdown и без текста вне JSON."""


def build_payload(model: str, audio_b64: str, prompt: str) -> dict:
    audio_data_url = f"data:;base64,{audio_b64}"
    return {
        "model": model,
        "modalities": ["text"],
        "messages": [
            {
                "role": "user",
                "content": [
                    {"type": "text", "text": prompt},
                    {
                        "type": "input_audio",
                        "input_audio": {
                            "data": audio_data_url,
                            "format": "wav",
                        },
                    },
                ],
            }
        ],
        "temperature": 0.2,
    }


def extract_json_text(content: str) -> str:
    if not content:
        return ""

    match = re.search(r"```(?:json)?\s*(.*?)\s*```", content, flags=re.DOTALL)
    if match:
        return match.group(1).strip()
    return content.strip()


def print_clean_response(parsed_response: dict) -> None:
    choices = parsed_response.get("choices", [])
    message_content = ""
    if choices:
        message = choices[0].get("message", {})
        message_content = message.get("content", "")

    cleaned = extract_json_text(message_content)

    try:
        cleaned_json = json.loads(cleaned)
        print(json.dumps(cleaned_json, ensure_ascii=False, indent=2))
    except Exception:
        # Fallback: print full API response when model output is not strict JSON.
        print(json.dumps(parsed_response, ensure_ascii=False, indent=2))


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Send WAV audio to Qwen Omni Plus and print JSON response."
    )
    parser.add_argument(
        "--audio",
        default="out.wav",
        help="Path to wav file (default: out.wav)",
    )
    parser.add_argument(
        "--model",
        default="qwen3.5-omni-plus",
        help="Model name (default: qwen3.5-omni-plus)",
    )
    parser.add_argument(
        "--base-url",
        default="https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions",
        help="Qwen compatible API URL",
    )
    parser.add_argument(
        "--prompt",
        default=DEFAULT_PROMPT,
        help="Task prompt for audio analysis",
    )
    args = parser.parse_args()

    api_key = os.getenv("QWEN_API_KEY")
    if not api_key:
        print("Error: set QWEN_API_KEY environment variable.", file=sys.stderr)
        return 1

    if not os.path.exists(args.audio):
        print(f"Error: file not found: {args.audio}", file=sys.stderr)
        return 1

    with open(args.audio, "rb") as f:
        audio_b64 = base64.b64encode(f.read()).decode("utf-8")

    payload = build_payload(args.model, audio_b64, args.prompt)
    body = json.dumps(payload).encode("utf-8")

    req = urllib.request.Request(
        args.base_url,
        data=body,
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=180) as resp:
            response_body = resp.read().decode("utf-8")
            parsed = json.loads(response_body)
            print_clean_response(parsed)
            return 0
    except urllib.error.HTTPError as e:
        print(f"HTTPError: {e.code}", file=sys.stderr)
        print(e.read().decode("utf-8", errors="replace"), file=sys.stderr)
        return 2
    except Exception as e:
        print(f"Request failed: {e}", file=sys.stderr)
        return 3


if __name__ == "__main__":
    raise SystemExit(main())
