# Project: fuck-work-bot

Telegram bot for a Ukrainian dev friend group. Work classifier + entertainment features.

## Architecture

Go bot on Fly.io (free tier, 256MB RAM, Frankfurt). Auto-deploys via GitHub Actions on push to main.

```
go-bot/
├── cmd/bot/main.go              — entry point, scheduler, seed logic
├── cmd/seed/main.go             — standalone seed tool
├── internal/
│   ├── classifier/classifier.go — TF-IDF + LogReg classifier (pure Go)
│   ├── storage/sqlite.go        — SQLite: stats, cards, balances, collection, feedback
│   └── handlers/
│       ├── handlers.go          — core commands (/start, /help, /check, /stats, /mute, /work)
│       ├── roasts.go            — username mapping for personalized content
│       ├── quotes.go            — /quote, /addquote, /roast, /compliment
│       ├── cards.go             — /pack, /collection, /battle
│       ├── cardimage.go         — card image rendering (Twemoji CDN + Go image)
│       ├── slots.go             — /slots, /balance, /daily, /top
│       ├── pokemon.go           — /pokemon via PokeAPI
│       ├── horoscope.go         — /horoscope via Gemini Flash
│       ├── eightball.go         — /8ball
│       └── fun_apis.go          — /cat (cataas.com), /dog (dog.ceo)
├── model/tfidf_model.json       — distilled TF-IDF model (1.4MB)
├── seed_data.json               — roasts, compliments, quotes, cards (seeded into DB)
├── scripts/                     — Python scripts for model training
└── Dockerfile
```

## Classifier

- TF-IDF (15K vocab, trigrams) + Logistic Regression + keyword boost
- Distilled from fine-tuned `paraphrase-multilingual-MiniLM-L12-v2`
- Threshold: 80% confidence to trigger roast
- Keyword boost: colleague names and dev terms add to logit

## Content (stored in SQLite, seeded from seed_data.json)

- 232 roasts (30 generic + 202 personal per member)
- 382 compliments (96 generic + 286 personal)
- 1628 quotes from chat history
- 301 trading cards (5 rarities, 11 legendaries, rendered as images with Twemoji)

## Economy

- Currency: богдудіки (starting balance 100)
- `/daily`: +50/day
- `/slots`: bet 1-100, max 20 spins/day, rigged mode via env
- `/blackjack`: bet 1-100, Hit/Stand via inline buttons, BJ pays 2.5x
- `/pack`: 20 coins, max 10/day
- `/battle`: winner +10 coins + steals loser's card
- `/roast @user`: 5 coins (self-roast free)
- `/rob @user`: 40% steal 10-50% coins, 60% lose 20 (1/hour)
- `/steal @user`: 30% steal card, 70% lose 20 coins (1/day)
- `/gacha`: premium pack guaranteed rare+ (100 coins)
- `/sacrifice`: 3 cards → 1 higher rarity
- `/burn`: destroy card for coins (5-100 by rarity)
- `/auction`/`/bid`: card auction system (60 sec)
- `/quiz`: trivia +10-25 coins (10/day)
- `/guess`: multiplayer number guess +30 coins
- `/wordle`: Ukrainian wordle (1/day, +10-50 coins)
- `/work` `/notwork`: +10 coins per feedback label

## Seed System

Version-based: bot computes hash of seed_data.json size on startup. If changed, re-seeds cards/quotes/roasts. Collections preserved.

## Building

```bash
CGO_ENABLED=0 go build -o bot ./cmd/bot/
TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

## Environment Variables

- `TELEGRAM_BOT_TOKEN` — required
- `MODEL_PATH` — TF-IDF model (default: `./model/tfidf_model.json`)
- `DATA_DIR` — SQLite DB dir (default: `/data`)
- `SEED_PATH` — seed data (default: `./seed_data.json`)
- `GEMINI_API_KEY` — for /horoscope
- `RIGGED_CASINO_USERS` — comma-separated names for rigged slots
- `RIGGED_CASINO_ENABLED` — toggle (default: `true`)
