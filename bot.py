"""
Telegram Bot with Work Classifier
For deployment on Fly.io
"""

import os
import random
import logging
from datetime import time
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

# Daily statistics per user: {user_id: {'work': 0, 'personal': 0, 'name': ''}}
daily_stats = {}

# Muted users (no tracking, no replies)
muted_users = set()

# Chat IDs where bot is active (for daily report)
active_chats = set()

# Savage work detection messages
WORK_REPLIES = [
    "О, хтось знову не може відпустити роботу навіть у чаті 🤡",
    "Так, ми всі вражені твоєю зайнятістю. Ні, насправді ні.",
    "Чат для відпочинку, а не для твоїх робочих драм",
    "Ти взагалі вмієш говорити про щось крім роботи?",
    "Вау, робота. Як оригінально. Всім дуже цікаво.",
    "Хтось явно не вміє відділяти роботу від життя",
    "Знову ця корпоративна нудьга в чаті...",
    "Ми зрозуміли, ти працюєш. Можна далі жити?",
    "Робота-робота... А особистість у тебе є?",
    "Чергова робоча тема? Як несподівано від тебе.",
    "Ти на годиннику чи просто не можеш зупинитись?",
    "Слухай, є інші теми для розмов. Google допоможе.",
    "О ні, знову хтось важливий зі своєю важливою роботою",
    "Так, так, дедлайни, мітинги, ми в захваті. Далі що?",
    "Може краще в робочий чат? Або в щоденник?",
    "Друже, це чат, а не твій LinkedIn",
    "Знову робочі проблеми? Психотерапевт дешевший",
    "Цікаво, ти й уві сні про роботу говориш?",
    "Нагадую: тут люди відпочивають від роботи. Ну, крім тебе.",
    "Ого, ще одне повідомлення про роботу! Який сюрприз!",
    "Може хоч раз поговоримо про щось людське?",
    "Твій роботодавець не платить за рекламу в цьому чаті",
    "Роботоголізм — це діагноз, до речі",
    "Дивно, що ти ще не створив окремий чат для своїх тікетів",
    "О, знову ти зі своїми важливими справами. Фанфари!",
    "Тут є правило: хто пише про роботу — той лох",
    "Знаєш що крутіше за роботу? Буквально все.",
    "А ти точно не бот? Бо тільки боти так багато про роботу",
    "Ми не твої колеги, можеш розслабитись",
    "Хтось забув вимкнути робочий режим 🙄",
]

# Random cars for /car command
from cars_db import CARS, get_random_car, get_coolness_emoji, get_hp_comment


def get_car_by_work_percentage(work_pct):
    """Returns car based on work percentage - more work = worse car"""
    if work_pct >= 80:
        # 80-100% work = worst cars (coolness 2)
        pool = [c for c in CARS if c[2] <= 2]
    elif work_pct >= 60:
        # 60-80% work = bad cars (coolness 3)
        pool = [c for c in CARS if c[2] == 3]
    elif work_pct >= 40:
        # 40-60% work = average cars (coolness 4-5)
        pool = [c for c in CARS if c[2] in [4, 5]]
    elif work_pct >= 20:
        # 20-40% work = good cars (coolness 6-7)
        pool = [c for c in CARS if c[2] in [6, 7]]
    elif work_pct >= 10:
        # 10-20% work = great cars (coolness 8-9)
        pool = [c for c in CARS if c[2] in [8, 9]]
    else:
        # <10% work = best cars (coolness 10)
        pool = [c for c in CARS if c[2] == 10]

    if not pool:
        pool = CARS

    car = random.choice(pool)
    return {
        'name': car[0],
        'hp': car[1],
        'coolness': car[2],
        'comment': car[3]
    }


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
    chat_id = update.effective_chat.id

    # Track active chats for daily report
    active_chats.add(chat_id)

    # Skip if user is muted
    if user_id in muted_users:
        return

    user_name = user.first_name or user.username or str(user_id)

    result = classifier.predict(text)

    # Initialize user stats if needed
    if user_id not in stats:
        stats[user_id] = {'work': 0, 'personal': 0, 'name': user_name}
    if user_id not in daily_stats:
        daily_stats[user_id] = {'work': 0, 'personal': 0, 'name': user_name}

    # Update statistics
    if result['is_work']:
        stats[user_id]['work'] += 1
        daily_stats[user_id]['work'] += 1
    else:
        stats[user_id]['personal'] += 1
        daily_stats[user_id]['personal'] += 1

    # Log
    logger.info(f"[{user_name}] [{result['label']}] ({result['confidence']:.0%}) {text[:50]}...")

    # Reply only if work with 95%+ confidence
    if result['is_work'] and result['confidence'] >= 0.95:
        # React with clown emoji
        try:
            from telegram import ReactionTypeEmoji
            await context.bot.set_message_reaction(
                chat_id=update.effective_chat.id,
                message_id=update.message.message_id,
                reaction=[ReactionTypeEmoji("🤡")]
            )
        except Exception as e:
            logger.warning(f"Reaction failed: {e}")

        # Text reply
        reply = random.choice(WORK_REPLIES)
        await update.message.reply_text(f"{reply} ({result['confidence']:.0%})")


async def daily_report(context: ContextTypes.DEFAULT_TYPE):
    """Send daily car assignment based on work stats"""
    global daily_stats

    if not daily_stats:
        return

    # Build report
    lines = ["🚗 *ЩОДЕННИЙ РОЗПОДІЛ МАШИН* 🚗\n"]
    lines.append("_Чим більше робочих повідомлень — тим гірша машина_\n")

    # Sort by work percentage (most work first = worst car first)
    sorted_users = []
    for user_id, data in daily_stats.items():
        total = data['work'] + data['personal']
        if total > 0:
            work_pct = data['work'] / total * 100
            sorted_users.append((user_id, data, work_pct, total))

    sorted_users.sort(key=lambda x: x[2], reverse=True)

    for user_id, data, work_pct, total in sorted_users:
        name = data['name']
        car = get_car_by_work_percentage(work_pct)
        coolness_emoji = get_coolness_emoji(car['coolness'])

        lines.append(f"👤 *{name}*")
        lines.append(f"   📊 {total} повідомлень ({work_pct:.0f}% робочих)")
        lines.append(f"   🚗 {car['name']}")
        lines.append(f"   {coolness_emoji} Крутість: {car['coolness']}/10")
        lines.append(f"   💬 _{car['comment']}_\n")

    report = "\n".join(lines)

    # Send to all active chats
    for chat_id in active_chats:
        try:
            await context.bot.send_message(
                chat_id=chat_id,
                text=report,
                parse_mode="Markdown"
            )
        except Exception as e:
            logger.error(f"Failed to send daily report to {chat_id}: {e}")

    # Reset daily stats
    daily_stats = {}
    logger.info("Daily report sent, stats reset")


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

    # Schedule daily report at 23:00 Kyiv time (UTC+2 or UTC+3)
    # Using UTC+2 (21:00 UTC)
    job_queue = app.job_queue
    job_queue.run_daily(
        daily_report,
        time=time(hour=21, minute=0, second=0),  # 23:00 Kyiv (UTC+2)
        name="daily_car_report"
    )
    logger.info("Daily report scheduled for 23:00 Kyiv time")

    logger.info("Bot starting...")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
