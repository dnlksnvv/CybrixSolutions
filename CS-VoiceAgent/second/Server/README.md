# Локальный LiveKit Server

Здесь лежит только способ **поднять медиа‑сервер** у себя на машине (не репозиторий LiveKit целиком — его не нужно клонировать).

## Вариант 1: Docker (удобно на macOS)

Из папки `Server/`:

```bash
cd CS-VoiceAgent/second/Server
docker compose up
```

Docker один раз **скачивает образ** `livekit/livekit-server` — это и есть «сервер». Исходники LiveKit при этом не качаются.

Остановка: `Ctrl+C` или в другом терминале `docker compose down`.

## Вариант 2: нативно на Mac (Homebrew)

Репозиторий не качаешь — ставится готовый бинарник:

```bash
brew install livekit
livekit-server --dev
```

Те же порты и те же ключи **`devkey` / `secret`** в режиме `--dev`.

## Что дальше

1. Сервер запущен (Docker или `livekit-server --dev`).
2. В `second/.env` и `Client/.env` — `LIVEKIT_URL=ws://127.0.0.1:7880` и ключи выше.
3. Запускаешь `python agent.py dev` и клиент (`pnpm dev` в `Client/`).
