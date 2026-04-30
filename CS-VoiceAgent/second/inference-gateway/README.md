# inference-gateway

Унифицированный шлюз к LLM/STT/TTS-провайдерам (Sber GigaChat, Sber SaluteSpeech, Alibaba DashScope Qwen) за единым API `v1`.

## Что внутри

- **HTTP `POST /v1/llm/chat`** — стрим LLM ответов в NDJSON (`delta` → `end` / `error`).
- **WebSocket `GET /v1/tts/ws`** — потоковый TTS (`session.start` → `input.text`/`input.commit` → `audio.chunk`/`audio.end`).
- **WebSocket `GET /v1/stt/ws`** — потоковый STT (`session.start` → `audio.chunk`/`input.finish` → `transcript.partial`/`transcript.final` → `session.end`).
- `GET /healthz` — список зарегистрированных моделей.

## Архитектура

```
cmd/server/                 # entrypoint, DI wiring
internal/
  config/                   # единственное место чтения env
  protocol/v1/              # envelope-структуры
  registry/                 # model_id -> provider impl
  services/
    llm/                    # контракт + реализации LLM
    stt/                    # контракт + реализации STT
    tts/                    # контракт TTS (iface.go)
      qwen/                 # DashScope Qwen TTS (YAML, realtime WS)
      sber/                 # SaluteSpeech REST (YAML, профили как у qwen)
  dashscopews/              # общий WS-клиент DashScope realtime
  eventbus/                 # стрим событий v1
  sberauth/                 # общий OAuth Sber NGW
  transport/
    http/                   # HTTP роутер + LLM хендлер
    ws/                     # WS хендлеры TTS/STT
  logger/
```

См. правила в `.cursor/rules/` — они описывают границы слоёв и стиль.

## Зарегистрированные модели

Регистрируется только то, для чего заданы ключи в `.env`:

| Модель                  | Модальность | Upstream                            | Транспорт |
|-------------------------|-------------|-------------------------------------|-----------|
| `giga-chat`             | LLM         | Sber GigaChat (OpenAI-compat)       | HTTP SSE  |
| `qwen-plus`             | LLM         | DashScope (OpenAI-compat)           | HTTP SSE  |
| `qwen-turbo`            | LLM         | DashScope (OpenAI-compat)           | HTTP SSE  |
| `qwen-max`              | LLM         | DashScope (OpenAI-compat)           | HTTP SSE  |
| `salute`                | TTS         | SaluteSpeech REST `text:synthesize` | HTTP→WS   |
| `qwen3-tts-realtime`    | TTS         | DashScope realtime (skeleton)       | WS        |
| `qwen3-asr-realtime`    | STT         | DashScope realtime (skeleton)       | WS        |

`qwen3-*-realtime` — каркас (`NOT_IMPLEMENTED`). Допишите upstream-mapping в `internal/services/{tts/qwen,stt}/…`, когда понадобятся.

## Запуск

```bash
cp .env.example .env
# заполнить нужные ключи
go run ./cmd/server
# или
make run
```

## Примеры запросов

**LLM (HTTP NDJSON стрим):**
```bash
curl -N -X POST http://localhost:8080/v1/llm/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "request_id":"00000000-0000-0000-0000-000000000001",
    "call_id":"call-1",
    "model":"giga-chat",
    "mode":"llm",
    "input":{
      "messages":[{"role":"user","content":"Скажи привет одним предложением"}],
      "stream":true
    }
  }'
```

**TTS (WebSocket):**
```
1. wss://localhost:8080/v1/tts/ws
2. {"type":"session.start","request_id":"r1","call_id":"c1","model":"salute","voice":"Nec_24000","audio_format":"pcm_s16le","sample_rate":24000}
3. {"type":"input.text","request_id":"r1","text":"Здравствуйте! "}
4. {"type":"input.commit","request_id":"r1"}
   <- audio.chunk (много)
   <- audio.end
```

**STT (WebSocket):**
```
1. wss://localhost:8080/v1/stt/ws
2. {"type":"session.start","request_id":"r1","model":"qwen3-asr-realtime","language":"ru","audio_format":"pcm_s16le","sample_rate":16000}
3. {"type":"audio.chunk","request_id":"r1","seq":1,"pcm_b64":"..."}  // повторяется
4. {"type":"input.finish","request_id":"r1"}
   <- transcript.partial / transcript.final
   <- session.end
```

## Out of scope (пока)

Без БД, без Redis, без аутентификации пользователей и без подсчёта токенов. Эти слои заложены в архитектуру (`registry` принимает интерфейсы) и добавляются как отдельные middleware/сервисы поверх текущей DI-схемы.
