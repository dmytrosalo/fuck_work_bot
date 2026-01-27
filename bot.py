"""
Telegram Bot with Work Classifier
For deployment on Fly.io
"""

import os
import logging
from telegram import Update
from telegram.ext import Application, MessageHandler, CommandHandler, filters, ContextTypes
from work_classifier import WorkClassifier

logging.basicConfig(
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    level=logging.INFO
)
logger = logging.getLogger(__name__)

# Initialize classifier
classifier = WorkClassifier()

# Statistics
stats = {'work': 0, 'personal': 0}


async def start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text(
        "👋 Hello! I classify messages:\n\n"
        "💼 work = work-related\n"
        "😎 personal = personal\n\n"
        "/check <text> - check message\n"
        "/stats - statistics"
    )


async def check(update: Update, context: ContextTypes.DEFAULT_TYPE):
    if not context.args:
        await update.message.reply_text("Usage: /check <text>")
        return

    text = " ".join(context.args)
    result = classifier.predict(text)
    emoji = "💼" if result['is_work'] else "😎"

    await update.message.reply_text(
        f"{emoji} {result['label'].upper()}\n"
        f"Confidence: {result['confidence']:.0%}"
    )


async def get_stats(update: Update, context: ContextTypes.DEFAULT_TYPE):
    total = stats['work'] + stats['personal']
    if total == 0:
        await update.message.reply_text("📊 No statistics yet")
        return

    work_pct = stats['work'] / total * 100
    await update.message.reply_text(
        f"📊 Statistics:\n\n"
        f"Total: {total}\n"
        f"💼 Work: {stats['work']} ({work_pct:.1f}%)\n"
        f"😎 Personal: {stats['personal']} ({100-work_pct:.1f}%)"
    )


async def on_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Classifies every message"""
    text = update.message.text

    if not text or text.startswith('/'):
        return

    result = classifier.predict(text)

    # Update statistics
    if result['is_work']:
        stats['work'] += 1
    else:
        stats['personal'] += 1

    # Log
    logger.info(f"[{result['label']}] {text[:50]}...")

    # Optional: reply to work messages with high confidence
    # if result['is_work'] and result['confidence'] > 0.95:
    #     await update.message.reply_text(f"💼 Work ({result['confidence']:.0%})", quote=True)


def main():
    token = os.environ.get('TELEGRAM_BOT_TOKEN')
    if not token:
        raise ValueError("Set TELEGRAM_BOT_TOKEN")

    app = Application.builder().token(token).build()

    app.add_handler(CommandHandler("start", start))
    app.add_handler(CommandHandler("check", check))
    app.add_handler(CommandHandler("stats", get_stats))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, on_message))

    logger.info("Bot starting...")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
