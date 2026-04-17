# FuckingWorkTracking Bot

Telegram bot for a Ukrainian dev friend group. Detects work messages, roasts people, runs a card game economy, and has way too many features for a chat bot.

## Features

- **Work Classifier** — TF-IDF + LogReg distilled from a fine-tuned sentence transformer. 80% confidence threshold with keyword boost for colleague names.
- **Personalized Roasts & Compliments** — stored in SQLite, targeted per chat member
- **Trading Cards** — 301 cards (Keyo/chat + Ukrainian memes/culture), 5 rarities, pack opening, collection, battles
- **Economy** — богдудіки currency, slots casino, daily bonus, leaderboard
- **Pokemon** — daily Pokemon with official artwork via PokeAPI
- **Horoscope** — Gemini-powered dev horoscope, 1/day
- **Cat Memes** — cat photos with personalized text overlay
- **Quotes** — 1600+ real quotes from chat history
- **Feedback Loop** — `/work` and `/notwork` for model retraining (+10 coins reward)

## Stack

- **Go** — single binary, no CGO, ~22MB RAM
- **SQLite** (modernc.org/sqlite) — pure Go
- **Fly.io** — free tier (256MB, Frankfurt)
- **Telegram Bot API** (gopkg.in/telebot.v3)
- **Gemini Flash** — horoscope generation
- **PokeAPI** — Pokemon data + images
- **cataas.com** — cat meme photos

## Commands

| Command | Description | Cost |
|---------|-------------|------|
| `/start` | Welcome message | — |
| `/check <text>` | Classify a message | — |
| `/stats` | Work message statistics | — |
| `/roast` | Roast (self, reply, or @user) | — |
| `/compliment` | Compliment (self, reply, or @user) | — |
| `/quote` | Random quote from chat history | — |
| `/addquote` | Save a quote (reply to message) | — |
| `/pokemon` | Your daily Pokemon with image | — |
| `/horoscope` | Dev horoscope (1/day) | — |
| `/8ball <question>` | Magic 8 ball | — |
| `/cat` | Cat meme about you | — |
| `/cat @user` | Cat meme about someone | — |
| `/dog` | Random dog photo | — |
| `/daily` | Daily bonus | +50 🪙 |
| `/slots <bet>` | Slot machine (max 20/day) | 1-100 🪙 |
| `/pack` | Open card pack (max 10/day) | 20 🪙 |
| `/collection` | Your card collection | — |
| `/battle` | Battle cards (reply to someone) | ±10 🪙 + card |
| `/balance` | Check your coins | — |
| `/top` | Richest players leaderboard | — |
| `/work` | Label message as work (reply) | +10 🪙 |
| `/notwork` | Label message as not work (reply) | +10 🪙 |
| `/mute` / `/unmute` | Toggle work tracking | — |

## Deploy

Auto-deploys on push to `main` via GitHub Actions.

```bash
# Manual deploy
fly deploy -a fuck-work-bot

# Logs
fly logs -a fuck-work-bot
```

## Local Development

```bash
cd go-bot
CGO_ENABLED=0 go build -o bot ./cmd/bot/
TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

## Retraining

1. Export feedback: `sqlite3 /data/bot.db "SELECT text,label FROM feedback"`
2. Run `go-bot/scripts/finetune_model.py`
3. Distill back to TF-IDF
4. Replace `go-bot/model/tfidf_model.json`
5. Push to main (auto-deploys)

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TELEGRAM_BOT_TOKEN` | Bot token (required) | — |
| `MODEL_PATH` | TF-IDF model path | `./model/tfidf_model.json` |
| `DATA_DIR` | SQLite DB directory | `/data` |
| `SEED_PATH` | Seed data JSON | `./seed_data.json` |
| `GEMINI_API_KEY` | For /horoscope | — |
| `RIGGED_CASINO_USERS` | Comma-separated names for rigged slots | — |
| `RIGGED_CASINO_ENABLED` | Toggle rigged mode | `true` |
