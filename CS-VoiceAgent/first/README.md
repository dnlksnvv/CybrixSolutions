## CS-VoiceAgent (minimal)

FastAPI + plain HTML pages to test Alibaba Cloud Model Studio (DashScope, Singapore).

### Features

- **LLM**: OpenAI-compatible Chat Completions (`/compatible-mode/v1/chat/completions`)
- **STT**: Qwen3-ASR-Flash via OpenAI-compatible endpoint (upload file → Base64 Data URL)
- **TTS**: Qwen3-TTS-* via DashScope multimodal-generation endpoint
- **Voice cloning**: Qwen voice enrollment (`qwen-voice-enrollment`) → store voiceprints and select them for TTS

### Setup

Create a venv and install:

```bash
cd CS-VoiceAgent
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

Set your **Singapore** API key:

```bash
export DASHSCOPE_API_KEY='sk-...'
```

Run:

```bash
uvicorn app.main:app --reload --port 8008
```

Open:

- `http://127.0.0.1:8008/`

### Notes

- API keys are expected via environment variable `DASHSCOPE_API_KEY`.
- The UI keeps things intentionally simple; most actions return JSON and/or show results on the same page.
- Audio URLs returned by TTS are time-limited per Alibaba docs.
