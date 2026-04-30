# Cybrix custom plugins

Workers разговаривают с моделями исключительно через локальный
**inference-gateway** (`CS-VoiceAgent/second/inference-gateway`). Здесь живут
только LiveKit-адаптеры и поведенческие хелперы — никакой логики обращения
к DashScope/Sber/etc. в этом дереве быть не должно.

## Prerequisites: `uv`

Этот LiveKit **agents** репозиторий управляется через **[uv](https://docs.astral.sh/uv/)**.
Если `command not found: uv`, поставь:

- **macOS (Homebrew):** `brew install uv`
- **Официальный установщик:** `curl -LsSf https://astral.sh/uv/install.sh | sh`
- **pipx:** `pipx install uv`

Затем открой **новый терминал** (или `rehash` в zsh) и в каталоге `agents/`
выполни `uv sync --dev`.

### Apple Silicon + conda: `ImportError` / `_ssl` / `libssl.3.dylib` (wrong architecture)

Если `uv run` берёт Miniforge Python (`.../miniforge/base/...`), будет конфликт
x86_64 OpenSSL vs arm64. Команда **`make cybrix-dashscope-dev`** ставит
`UV_PYTHON_PREFERENCE=only-managed` и `--python 3.12`, так что **uv скачает
свой CPython arm64**. Если всё ещё не работает:

```bash
uv python install 3.12
```

Также сделай `conda deactivate`, чтобы `python` на `PATH` не был сломанным.

## Что лежит в этой папке

- `inference_gateway/` — пакет, через который воркеры говорят со шлюзом:
  - `protocol.py` — типизированные шаблоны v1 wire-формата (LLM/TTS/STT). **Единственное место**, где описаны JSON-форматы запросов в шлюз.
  - `client.py` — тонкий aiohttp-транспорт (HTTP NDJSON + WebSocket).
  - `config.py` — чтение `.env` (gateway URL + модели/голоса/языки).
  - `llm.py`, `tts.py`, `stt.py` — LiveKit-адаптеры (`llm.LLM`, `tts.TTS`, `stt.STT`), плагающиеся в `AgentSession`.
- `streaming_sentencizer.py` — нарезка LLM-токенов на предложения для TTS-режима `commit` (используется `inference_gateway.tts`).
- `trace_emitter.py` / `tracing_llm.py` — клиентский трейсер для дашборда (`Workers-3/dashboard`).

## Запуск примера

Из корня репозитория `agents/`:

```bash
# Не использовать ``uv run --with ...``: uv поставит PyPI ``livekit-agents`` (другой API).
# Только workspace + dependency-group ``cybrix``:

uv sync --dev --group cybrix
make cybrix-dashscope-dev
```

Перед запуском убедись, что:

1. Шлюз поднят — `cd ../../inference-gateway && go run ./cmd/server`.
2. В `agents/.env` заданы `LIVEKIT_*`, `INFERENCE_GATEWAY_URL`, `LLM_MODEL`,
   `STT_MODEL`/`STT_LANGUAGE`, `TTS_MODEL` (и опционально `TTS_VOICE`,
   `TTS_LANGUAGE_TYPE`, `TTS_MODE`).
