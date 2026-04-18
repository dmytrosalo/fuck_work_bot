# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Telegram bot for a Ukrainian dev friend group. Work classifier + entertainment features. Go bot on Fly.io (free tier, 256MB RAM, Frankfurt). Auto-deploys via GitHub Actions on push to main.

## Build & Run

```bash
# Build (from repo root)
cd go-bot && CGO_ENABLED=0 go build -o bot ./cmd/bot/

# Run locally
TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot

# Run tests
cd go-bot && go test ./...

# Run a single test
cd go-bot && go test ./internal/handlers/ -run TestHandleDuel
cd go-bot && go test ./internal/storage/ -run TestBalance

# Seed tool (standalone)
cd go-bot && go run ./cmd/seed/
```

## Architecture

All Go code lives under `go-bot/`. Module: `github.com/dmytrosalo/fuck-work-bot`.

**Core flow:** `cmd/bot/main.go` creates storage, classifier, and `handlers.Bot`, then calls `bot.Register()` to wire all Telegram command handlers. Also starts an HTTP server on `:8080` for the collection web page. On startup it runs seed logic and scheduled daily resets.

**Key packages:**
- `internal/handlers/` — All Telegram command handlers live on the `Bot` struct. `handlers.go` has `Register()` which maps commands → handler methods. Each feature area is a separate file (slots.go, cards.go, quiz.go, etc.). `web.go` serves the collection web page at `/collection/:userID`.
- `internal/storage/sqlite.go` — Single file with all DB operations. Uses `modernc.org/sqlite` (pure Go, no CGO). Schema migrations in `migrate()` function. Tables: stats, cards, collection, balances, daily_limits, transactions, etc.
- `internal/classifier/classifier.go` — TF-IDF + Logistic Regression classifier (pure Go). Loads model from `model/tfidf_model.json`. Keyword boost for colleague names/dev terms.

**Handler pattern:** Every handler is a method on `*Bot` with signature `func (b *Bot) handleX(c tele.Context) error`. Uses `gopkg.in/telebot.v3`. Inline buttons use `tele.Btn{Unique: "name"}` for callbacks.

**Content system:** `seed_data.json` contains roasts, compliments, quotes, and cards. Seeded into SQLite on startup via version check (hash of file size). Collections/balances survive re-seeding.

**Web UI:** HTTP server on `:8080` serves collection pages. `handlers.RegisterWeb()` sets up routes. Fly.io expects this port (`internal_port = 8080` in fly.toml). Collection page: dark theme, mobile-first, card grid with click-to-expand.

**Economy:** Currency is "богдудіки". All economy operations go through `storage.DB` methods (UpdateBalance, GetBalance, etc.). Daily limits reset at 00:00 Kyiv time. All UI text is in Ukrainian.

## Environment Variables

- `TELEGRAM_BOT_TOKEN` — required
- `MODEL_PATH` — TF-IDF model (default: `./model/tfidf_model.json`)
- `DATA_DIR` — SQLite DB dir (default: `/data`)
- `SEED_PATH` — seed data (default: `./seed_data.json`)
- `GEMINI_API_KEY` — for /horoscope
- `RIGGED_CASINO_USERS` — comma-separated usernames for rigged slots
- `RIGGED_CASINO_ENABLED` — toggle (default: `true`)

## Important Conventions

- All user-facing text is in Ukrainian
- `roasts.go` has `usernameMap` mapping Telegram usernames to display names for personalized content
- Daily limits use Kyiv timezone (Europe/Kyiv) — see `helpers.go`
- CGO is disabled (`CGO_ENABLED=0`) — SQLite uses pure Go driver `modernc.org/sqlite`
- Deployment: push to `main` auto-deploys to Fly.io via `.github/workflows/deploy.yml`
