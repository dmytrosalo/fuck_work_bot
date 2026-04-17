# Project: fuck-work-bot

Telegram bot that detects work-related messages in a friend group chat and roasts people for talking about work.

## Architecture

Go bot deployed on Fly.io (free tier, 256MB RAM, Frankfurt).

```
go-bot/
├── cmd/bot/main.go              — entry point, scheduler
├── internal/
│   ├── classifier/classifier.go — TF-IDF + LogReg classifier (pure Go, no CGO)
│   ├── storage/sqlite.go        — SQLite persistence (pure Go, modernc.org/sqlite)
│   └── handlers/
│       ├── handlers.go          — Telegram command & message handlers
│       └── roasts.go            — roasts & compliments database
├── model/tfidf_model.json       — distilled TF-IDF model (1.4MB)
├── scripts/                     — Python scripts for model training (not deployed)
└── Dockerfile
```

## Classifier

- TF-IDF (15K vocab, trigrams) + Logistic Regression + keyword boost
- Distilled from a fine-tuned `paraphrase-multilingual-MiniLM-L12-v2` embedding model
- Trained on 43K messages labeled by the fine-tuned teacher model
- Threshold: 80% confidence to trigger roast
- Keyword boost: colleague names (Руді, Маріт, Делна, etc.) and dev terms add to logit

## Key Commands

- `/check <text>` — manual classification
- `/stats` — per-user message stats
- `/roast` / `/compliment` — targeted roasts/compliments (reply or @username)
- `/work` / `/notwork` — reply to message to label for future retraining
- `/mute` / `/unmute` — toggle tracking

## Building

```bash
# No CGO needed
CGO_ENABLED=0 go build -o bot ./cmd/bot/

# Run locally
TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

## Retraining the classifier

1. Export feedback: `sqlite3 /data/bot.db "SELECT text,label FROM feedback" > feedback.csv`
2. Run `go-bot/scripts/finetune_model.py` to fine-tune the embedding model
3. Run distillation to regenerate `tfidf_model.json`

## Deployment

```bash
fly deploy -a fuck-work-bot
fly status -a fuck-work-bot
fly logs -a fuck-work-bot
```

## Environment Variables

- `TELEGRAM_BOT_TOKEN` — required
- `MODEL_PATH` — path to tfidf_model.json (default: `./model/tfidf_model.json`)
- `DATA_DIR` — SQLite DB directory (default: `/data`)
