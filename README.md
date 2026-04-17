# FuckingWorkTracking Bot

Telegram bot for a Ukrainian dev friend group. Detects work messages, roasts people, runs a card game economy, and has way too many features for a chat bot.

## Features

- **Work Classifier** — TF-IDF + LogReg distilled from a fine-tuned sentence transformer. 80% confidence threshold with keyword boost for colleague names.
- **Personalized Roasts & Compliments** — stored in SQLite, targeted per chat member
- **Trading Cards** — 301 cards with rendered images (Twemoji), 5 rarities, pack opening as album, collection, battles
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
- **Twemoji CDN** — emoji images for card rendering
- **Telegram Bot API** (gopkg.in/telebot.v3)
- **Gemini Flash** — horoscope generation
- **PokeAPI** — Pokemon data + images
- **cataas.com** — cat meme photos

## Commands

| Command | Description | Cost |
|---------|-------------|------|
| **Classifier** | | |
| `/start` | Welcome + commands | — |
| `/help` | Rules and mechanics | — |
| `/check <text>` | Classify a message | — |
| `/stats` | Work message stats | — |
| `/work` / `/notwork` | Label message (reply) | +10 🪙 |
| `/mute` / `/unmute` | Toggle tracking | — |
| **Cards (301)** | | |
| `/pack` | Open card pack (max 10/day) | 20 🪙 |
| `/gacha` | Premium pack (rare+ guaranteed) | 100 🪙 |
| `/collection` | Your cards by rarity | — |
| `/card <name>` | View card with image | — |
| `/showcase` | Flex your rarest card | — |
| `/battle` | Quick random card fight | ±10 🪙 + card |
| `/duel @user` | Pick your card to fight | ±15 🪙 + card |
| `/steal @user` | 30% steal card (1/day) | risk 20 🪙 |
| `/sacrifice <rarity>` | 3 cards → 1 higher rarity | 3 cards |
| `/burn <name>` | Destroy card for coins | 5-100 🪙 |
| `/gift @user <name>` | Give card to someone | card |
| `/auction <name>` | Auction card (60 sec) | — |
| `/bid <amount>` | Bid on auction | 🪙 |
| **Economy** | | |
| `/slots <bet>` | Slot machine (max 20/day) | 1-100 🪙 |
| `/daily` | Daily bonus | +50 🪙 |
| `/balance` | Check coins | — |
| `/top` | Leaderboard | — |
| `/rob @user` | 40% steal coins (1/hour) | risk 20 🪙 |
| **Games** | | |
| `/quiz` | Trivia questions (10/day) | +10-25 🪙 |
| `/guess` | Guess number 1-100 (multiplayer) | +30 🪙 |
| `/wordle` | Ukrainian wordle (1/day) | +10-50 🪙 |
| **Fun** | | |
| `/pokemon` | Daily Pokemon with image | — |
| `/horoscope` | Dev horoscope (Gemini, 1/day) | — |
| `/8ball <question>` | Magic 8 ball | — |
| `/cat` / `/dog` | Random pet photo | — |
| `/roast` | Roast (5 🪙 for others) | 5 🪙 |
| `/compliment` | Compliment | — |
| `/quote` / `/addquote` | Chat quotes | — |

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
