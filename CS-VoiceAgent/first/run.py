#!/usr/bin/env python3
"""
Run the CS-VoiceAgent dev server.

Usage:
  source .venv/bin/activate
  python run.py
"""

import uvicorn


if __name__ == "__main__":
    uvicorn.run("app.main:app", host="127.0.0.1", port=8008, reload=True)

