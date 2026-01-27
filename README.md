# Work Classifier Bot - Fly.io

## Files

```
flyio_bot/
├── .github/
│   └── workflows/
│       └── deploy.yml          # GitHub Actions deploy
├── bot.py                      # Telegram bot
├── work_classifier.py          # Classifier
├── work_classifier_light.joblib # Model (0.53 MB)
├── requirements.txt
├── Dockerfile
├── docker-compose.yml          # Local run
├── fly.toml
├── .env.example
├── .gitignore
└── README.md
```

## Features

- **Accuracy:** 99.08%
- **Model Size:** 0.53 MB
- **Speed:** <2ms
- **RAM:** ~50-100 MB

---

## Local Run (Docker)

```bash
# 1. Copy .env
cp .env.example .env

# 2. Add token to .env
nano .env

# 3. Run
docker-compose up --build

# Or in background
docker-compose up -d --build

# Logs
docker-compose logs -f

# Stop
docker-compose down
```

---


## Deploy to Fly.io (First time)

### 1. Install flyctl

```bash
# macOS
brew install flyctl

# Linux
curl -L https://fly.io/install.sh | sh
```

### 2. Login and launch app

```bash
fly auth login
fly launch --no-deploy
```

### 3. Secrets

```bash
fly secrets set TELEGRAM_BOT_TOKEN="your_token_here"
```

### 4. Deploy

```bash
fly deploy
```

---

## Deploy from GitHub (Automatic)

### 1. Get Fly.io API token

```bash
fly tokens create deploy -x 999999h
```

### 2. Add secret to GitHub

GitHub repo → Settings → Secrets and variables → Actions → New repository secret

- Name: `FLY_API_TOKEN`
- Value: token from previous step

### 3. Add bot secret to Fly.io

```bash
fly secrets set TELEGRAM_BOT_TOKEN="your_bot_token"
```

### 4. Push to main

```bash
git add .
git commit -m "Deploy"
git push origin main
```

Deploy will start automatically!

---

## Bot Commands

- `/start` - Welcome message
- `/check <text>` - Check message
- `/stats` - Chat statistics

---

## Fly.io Commands

```bash
# Status
fly status

# Logs
fly logs

# SSH into container
fly ssh console

# Restart
fly apps restart

# Scaling
fly scale memory 512
```

---

## Fly.io Pricing

- Free tier: 3 shared VMs, 256 MB RAM
- This bot: ~$0-2/month
