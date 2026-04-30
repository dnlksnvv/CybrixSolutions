# Клиент в этом репозитории

Исходник: **[livekit-examples/agent-starter-react](https://github.com/livekit-examples/agent-starter-react)** — официальный Next.js‑клиент для **LiveKit Agents** (голос, транскрипты, UI).

Чтобы подтянуть обновления апстрима вручную:

```bash
cd Client
git init && git remote add origin https://github.com/livekit-examples/agent-starter-react.git
git fetch --depth 1 origin main && git checkout FETCH_HEAD
# осторожно: перезапишет файлы; лучше делать в отдельной ветке или копировать нужные коммиты
```

## Куда смотрит `Client/.env`

Переменные **`LIVEKIT_*`** — это **только адрес сервера LiveKit** (медиа, WebRTC). Это **не** твой Python‑агент и **не** DashScope. Для работы **без livekit.cloud** подними LiveKit локально: папка **`../Server/`** (`docker compose up`) или `brew install livekit` — см. **`../Server/README.md`**. В `.env` клиента: `LIVEKIT_URL=ws://127.0.0.1:7880`, `devkey` / `secret`.

Ключи **DashScope** живут в **`../.env`** у `agent.py` — клиент их не видит.

## Запуск вместе с `../agent.py`

1. **LiveKit Server** — Docker из `../Server/` (`docker compose up`) или `brew install livekit` + `livekit-server --dev` (см. `../Server/README.md`).
2. **Воркер агента** (из папки `second/`):

   ```bash
   cd ..
   source .venv/bin/activate
   python agent.py dev
   ```

3. **Клиент** (из этой папки):

   ```bash
   cd Client
   cp .env.example .env
   # Заполни те же LIVEKIT_URL, LIVEKIT_API_KEY, LIVEKIT_API_SECRET, что и у агента.
   # Локально: LIVEKIT_URL=ws://127.0.0.1:7880  devkey / secret
   pnpm install
   pnpm dev
   ```

4. Открой **http://localhost:3000**, введи имя **комнаты** (любое уникальное), подключись с микрофоном.

`AGENT_NAME` в `.env` клиента можно оставить пустым для автоматического dispatch (как в примере LiveKit).

## Зависимости

Нужен **Node.js** и **pnpm** (`npm i -g pnpm`).
