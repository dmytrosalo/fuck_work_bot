# FuckingWorkTracking Bot

Telegram bot that detects work-related messages in a friend group chat and roasts people for talking about work.

## Features

- **Work Classifier** — TF-IDF + LogReg model distilled from a fine-tuned sentence transformer. Detects work messages with ~95% accuracy including colleague names, project terms, and dev jargon.
- **Personalized Roasts** — 600+ roasts tailored per chat member (Data, Dmytro, Danya, Bo)
- **Compliments** — `/compliment` for when you need positivity
- **Feedback Loop** — `/work` and `/notwork` commands to label messages for model retraining
- **Stats** — per-user work message tracking with daily reports at 23:00 Kyiv time
- **SQLite** — persistent storage on Fly.io volume

## Stack

- **Go** — single binary, no CGO, ~22MB RAM
- **SQLite** (modernc.org/sqlite) — pure Go, no external dependencies
- **Fly.io** — free tier (256MB shared VM, Frankfurt)
- **Telegram Bot API** (gopkg.in/telebot.v3)

## Commands

| Command | Description |
|---------|-------------|
| `/start` | Welcome message |
| `/check <text>` | Classify a message |
| `/stats` | User statistics |
| `/roast` | Roast yourself or reply to someone |
| `/compliment` | Compliment yourself or reply to someone |
| `/work` | Reply to a message to mark as work |
| `/notwork` | Reply to a message to mark as not work |
| `/mute` | Disable tracking |
| `/unmute` | Enable tracking |

## Deploy

```bash
fly deploy -a fuck-work-bot
```

## Local Development

```bash
cd go-bot
CGO_ENABLED=0 go build -o bot ./cmd/bot/
TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

## Retraining

The bot collects feedback via `/work` and `/notwork` commands. To retrain:

1. Export feedback from SQLite
2. Run `go-bot/scripts/finetune_model.py`
3. Distill back to TF-IDF
4. Replace `go-bot/model/tfidf_model.json`
5. Redeploy
