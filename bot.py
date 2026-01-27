"""
Telegram Bot with Work Classifier
For deployment on Fly.io
"""

import os
import random
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

# Statistics per user: {user_id: {'work': 0, 'personal': 0, 'name': ''}}
stats = {}

# Muted users (no tracking, no replies)
muted_users = set()

# Funny work detection messages
WORK_REPLIES = [
    "Здається, попався 🕵️",
    "Оце робота в чаті! Ай-ай-ай 👀",
    "Хтось тут працює замість того щоб відпочивати 🤨",
    "Воу-воу, полегше з роботою! 🛑",
    "Робота detected! Alarm! 🚨",
    "Знову ця робота... Коли вже відпочинеш? 😩",
    "Ловимо на гарячому! Робочі теми в чаті! 🔥",
    "Так-так, бачу шо робиш... працюєш 👁️",
    "Work-life balance порушено! ⚖️",
    "Ей, це ж робота! Фу таким бути 🙈",
    "Знову про роботу? Серйозно? 😒",
    "Робота в неробочий час? Ганьба! 🔔",
    "Стоп-стоп, тут пахне роботою 👃",
    "О, хтось кар'єрист тут 📈",
    "Менше роботи, більше мемів! 🐸",
    "Роботоголік spotted! 🎯",
    "Це що, продуктивність? В цьому чаті?! 😱",
    "Йой, знову ця корпоративна лексика 🏢",
    "Тихо! Чую звук роботи... 🔊",
    "А можна без роботи? Ні? Ок... 😔",
    "Оу, хтось дуже відповідальний 🫡",
    "Робота? В МОЄму чаті? 😤",
    "Увага! Зафіксовано робочу активність! 📡",
    "Ех, знову ці дорослі розмови про роботу 👴",
    "Так, я все бачу. Все записую. 📝",
    "Невже не можна просто покидати мемчики? 🤷",
    "От би замість роботи щось цікаве... 💭",
    "Ого, хтось тут серйозний! 🧐",
    "Пахне овертаймом... 🕐",
    "Стривай, це що - відповідальність?! 😰",
]

# Random cars for /car command
from cars_db import CARS, get_random_car, get_coolness_emoji, get_hp_comment


async def car(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Random car assignment"""
    car_data = get_random_car()
    user = update.effective_user
    name = user.first_name or user.username or "Анонім"
    
    coolness_emoji = get_coolness_emoji(car_data['coolness'])
    hp_comment = get_hp_comment(car_data['hp'])
    
    await update.message.reply_text(
        f"🎰 *{name}*, твоя машина:\n\n"
        f"🚗 *{car_data['name']}*\n"
        f"🐎 {car_data['hp']} к.с. — _{hp_comment}_\n"
        f"{coolness_emoji} Крутість: {car_data['coolness']}/10\n\n"
        f"💬 _{car_data['comment']}_",
        parse_mode="Markdown"
    )


async def start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    await update.message.reply_text(
        "👋 Hello! Я тут рішатиму чи твій текст робота чи персональний:\n\n"
        "💼 клята робота \n"
        "😎 персональне\n\n"
        "/check <text> - перевірити текст\n"
        "/stats - статистика\n"
        "/car - яка твоя машина? 🚗\n"
        "/mute - вимкнути трекінг\n"
        "/unmute - увімкнути трекінг"
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
    if not stats:
        await update.message.reply_text("📊 Немає статистики")
        return

    lines = ["📊 Статистика:\n"]

    # Sort by total messages
    sorted_users = sorted(
        stats.items(),
        key=lambda x: x[1]['work'] + x[1]['personal'],
        reverse=True
    )

    total_work = 0
    total_personal = 0

    for user_id, data in sorted_users:
        name = data.get('name', 'Unknown')
        work = data['work']
        personal = data['personal']
        total = work + personal
        total_work += work
        total_personal += personal

        if total > 0:
            work_pct = work / total * 100
            lines.append(f"👤 {name}: {total} msgs (💼 {work_pct:.0f}%)")

    grand_total = total_work + total_personal
    if grand_total > 0:
        lines.append(f"\n📈 загалом: {grand_total}")
        lines.append(f"💼 Робота: {total_work} ({total_work/grand_total*100:.0f}%)")
        lines.append(f"😎 Персональне: {total_personal} ({total_personal/grand_total*100:.0f}%)")

    await update.message.reply_text("\n".join(lines))


async def mute(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Disable tracking for user"""
    user_id = update.effective_user.id
    muted_users.add(user_id)
    await update.message.reply_text(
        "🔇 Трекінг вимкнено. Я більше не буду:\n"
        "• Відстежувати твої повідомлення\n"
        "• Писати про роботу\n\n"
        "/unmute щоб увімкнути назад"
    )


async def unmute(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Enable tracking for user"""
    user_id = update.effective_user.id
    muted_users.discard(user_id)
    await update.message.reply_text(
        "🔊 Трекінг увімкнено! Тепер я знову слідкую за тобою 👀"
    )


async def on_message(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Classifies every message, replies only if work with high confidence"""
    text = update.message.text

    if not text or text.startswith('/'):
        return

    # Get user info
    user = update.effective_user
    user_id = user.id
    
    # Skip if user is muted
    if user_id in muted_users:
        return
    
    user_name = user.first_name or user.username or str(user_id)

    result = classifier.predict(text)

    # Initialize user stats if needed
    if user_id not in stats:
        stats[user_id] = {'work': 0, 'personal': 0, 'name': user_name}

    # Update statistics
    if result['is_work']:
        stats[user_id]['work'] += 1
    else:
        stats[user_id]['personal'] += 1

    # Log
    logger.info(f"[{user_name}] [{result['label']}] ({result['confidence']:.0%}) {text[:50]}...")

    # Reply only if work with 95%+ confidence
    if result['is_work'] and result['confidence'] >= 0.95:
        reply = random.choice(WORK_REPLIES)
        await update.message.reply_text(
            f"{reply} ({result['confidence']:.0%})",
            quote=True
        )


def main():
    token = os.environ.get('TELEGRAM_BOT_TOKEN')
    if not token:
        raise ValueError("Set TELEGRAM_BOT_TOKEN")

    app = Application.builder().token(token).build()

    app.add_handler(CommandHandler("start", start))
    app.add_handler(CommandHandler("check", check))
    app.add_handler(CommandHandler("stats", get_stats))
    app.add_handler(CommandHandler("car", car))
    app.add_handler(CommandHandler("mute", mute))
    app.add_handler(CommandHandler("unmute", unmute))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, on_message))

    logger.info("Bot starting...")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
