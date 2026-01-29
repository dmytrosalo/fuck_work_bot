"""
Telegram Bot with Work Classifier
For deployment on Fly.io
"""

import os
import json
import random
import logging
from datetime import time, datetime, timedelta
from pathlib import Path
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

# Data directory (persistent on Fly.io with volume)
DATA_DIR = Path(os.environ.get('DATA_DIR', '/data'))
DATA_DIR.mkdir(parents=True, exist_ok=True)

STATS_FILE = DATA_DIR / 'stats.json'
DAILY_STATS_FILE = DATA_DIR / 'daily_stats.json'
MUTED_FILE = DATA_DIR / 'muted.json'
CHATS_FILE = DATA_DIR / 'chats.json'
BALANCE_FILE = DATA_DIR / 'balance.json'
BONUS_FILE = DATA_DIR / 'bonus.json'  # Track last bonus claim
RIDDLE_STATE_FILE = DATA_DIR / 'riddle_state.json'  # Track active riddles


def load_json(filepath, default):
    """Load JSON file or return default"""
    try:
        if filepath.exists():
            with open(filepath, 'r') as f:
                return json.load(f)
    except Exception as e:
        logger.error(f"Error loading {filepath}: {e}")
    return default


def save_json(filepath, data):
    """Save data to JSON file"""
    try:
        with open(filepath, 'w') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
    except Exception as e:
        logger.error(f"Error saving {filepath}: {e}")


# Load persistent data
stats = load_json(STATS_FILE, {})
daily_stats = load_json(DAILY_STATS_FILE, {})
muted_users = set(load_json(MUTED_FILE, []))
active_chats = set(load_json(CHATS_FILE, []))
balances = load_json(BALANCE_FILE, {})
bonus_claims = load_json(BONUS_FILE, {})  # {user_id: "2024-01-15"}
riddle_state = load_json(RIDDLE_STATE_FILE, {})  # {user_id: {"riddle": ..., "answer": ...}}

logger.info(f"Loaded stats: {len(stats)} users, {len(daily_stats)} daily, {len(muted_users)} muted, {len(active_chats)} chats, {len(balances)} balances")

# === RIDDLES DATABASE BY DIFFICULTY ===
# Level 1: Easy (bonus 1-5) - 20 coins
# Level 2: Medium (bonus 6-10) - 35 coins
# Level 3: Hard (bonus 11-15) - 50 coins
# Level 4: Expert (bonus 16-20) - 75 coins
# Level 5: Genius (bonus 21+) - 100 coins

RIDDLES_BY_LEVEL = {
    1: [  # Easy - базова математика та загальні знання
        {"q": "Скільки буде 7 + 8?", "a": ["15"]},
        {"q": "Скільки буде 10 * 5?", "a": ["50"]},
        {"q": "Скільки буде 100 - 37?", "a": ["63"]},
        {"q": "Скільки буде 24 / 6?", "a": ["4"]},
        {"q": "Скільки днів у тижні?", "a": ["7"]},
        {"q": "Скільки місяців у році?", "a": ["12"]},
        {"q": "Скільки годин у добі?", "a": ["24"]},
        {"q": "Скільки хвилин у годині?", "a": ["60"]},
        {"q": "Скільки секунд у хвилині?", "a": ["60"]},
        {"q": "Скільки сторін у квадрата?", "a": ["4"]},
        {"q": "Скільки сторін у трикутника?", "a": ["3"]},
        {"q": "Скільки кольорів у веселці?", "a": ["7"]},
        {"q": "Яка столиця України?", "a": ["київ", "kyiv", "kiev"]},
        {"q": "Який день тижня йде після понеділка?", "a": ["вівторок"]},
        {"q": "Який день тижня йде після середи?", "a": ["четвер"]},
        {"q": "Скільки буде 9 * 9?", "a": ["81"]},
        {"q": "Скільки буде 12 * 12?", "a": ["144"]},
        {"q": "Скільки пальців на двох руках?", "a": ["10"]},
        {"q": "Яка валюта в Україні?", "a": ["гривня", "uah", "грн"]},
        {"q": "Скільки буде 50 + 50?", "a": ["100"]},
    ],
    2: [  # Medium - географія, базове IT
        {"q": "Скільки буде 7 * 8?", "a": ["56"]},
        {"q": "Скільки буде 144 / 12?", "a": ["12"]},
        {"q": "Скільки буде 15 * 15?", "a": ["225"]},
        {"q": "Яка столиця Франції?", "a": ["париж", "paris"]},
        {"q": "Яка столиця Німеччини?", "a": ["берлін", "berlin"]},
        {"q": "Яка столиця Польщі?", "a": ["варшава", "warsaw"]},
        {"q": "Яка столиця Італії?", "a": ["рим", "rome", "roma"]},
        {"q": "Яка столиця Великобританії?", "a": ["лондон", "london"]},
        {"q": "Скільки планет в Сонячній системі?", "a": ["8"]},
        {"q": "Яка хімічна формула води?", "a": ["h2o", "н2о"]},
        {"q": "Скільки градусів у прямому куті?", "a": ["90"]},
        {"q": "Скільки сантиметрів у метрі?", "a": ["100"]},
        {"q": "Скільки грам у кілограмі?", "a": ["1000"]},
        {"q": "Скільки бітів у байті?", "a": ["8"]},
        {"q": "Який порт для HTTP?", "a": ["80"]},
        {"q": "Хто написав 'Кобзар'?", "a": ["шевченко", "тарас шевченко"]},
        {"q": "Яка валюта в США?", "a": ["долар", "dollar", "usd"]},
        {"q": "Яка валюта в Європі (ЄС)?", "a": ["євро", "euro", "eur"]},
        {"q": "Скільки років у столітті?", "a": ["100"]},
        {"q": "Скільки нулів у мільйоні?", "a": ["6"]},
    ],
    3: [  # Hard - IT, історія, складніша математика
        {"q": "Скільки буде 2^10?", "a": ["1024"]},
        {"q": "Скільки буде sqrt(144)?", "a": ["12"]},
        {"q": "Скільки буде 17 * 6?", "a": ["102"]},
        {"q": "Скільки буде 15% від 200?", "a": ["30"]},
        {"q": "Скільки байт в кілобайті?", "a": ["1024"]},
        {"q": "Який порт для HTTPS?", "a": ["443"]},
        {"q": "Який порт для SSH?", "a": ["22"]},
        {"q": "Яка столиця Японії?", "a": ["токіо", "tokyo"]},
        {"q": "Яка столиця Канади?", "a": ["оттава", "ottawa"]},
        {"q": "Яка столиця Австралії?", "a": ["канберра", "canberra"]},
        {"q": "Хто CEO Apple?", "a": ["тім кук", "tim cook", "кук", "cook"]},
        {"q": "Хто CEO Tesla?", "a": ["ілон маск", "elon musk", "маск", "musk"]},
        {"q": "В якому році Україна стала незалежною?", "a": ["1991"]},
        {"q": "Яка найвища гора у світі?", "a": ["еверест", "everest", "джомолунгма"]},
        {"q": "Що означає HTML?", "a": ["hypertext markup language"]},
        {"q": "Що означає CSS?", "a": ["cascading style sheets"]},
        {"q": "Яка мова програмування починається на 'Py'?", "a": ["python", "пайтон"]},
        {"q": "Скільки градусів у колі?", "a": ["360"]},
        {"q": "Яке число Пі (перші 3 цифри)?", "a": ["3.14", "314"]},
        {"q": "Що повертає len('hello')?", "a": ["5"]},
    ],
    4: [  # Expert - глибоке IT, бізнес
        {"q": "Скільки буде 2^8?", "a": ["256"]},
        {"q": "Скільки буде 1024 / 2?", "a": ["512"]},
        {"q": "Який результат: 10 % 3?", "a": ["1"]},
        {"q": "Який результат: 10 // 3?", "a": ["3"]},
        {"q": "Що означає HTTP?", "a": ["hypertext transfer protocol"]},
        {"q": "Що означає API?", "a": ["application programming interface"]},
        {"q": "Що означає SQL?", "a": ["structured query language"]},
        {"q": "Що означає JSON?", "a": ["javascript object notation"]},
        {"q": "Що означає OOP?", "a": ["object oriented programming"]},
        {"q": "Що означає RAM?", "a": ["random access memory"]},
        {"q": "Що означає CPU?", "a": ["central processing unit"]},
        {"q": "Який рік заснування Apple?", "a": ["1976"]},
        {"q": "Який рік заснування Google?", "a": ["1998"]},
        {"q": "Який рік заснування Microsoft?", "a": ["1975"]},
        {"q": "Хто засновник Amazon?", "a": ["безос", "bezos", "джефф"]},
        {"q": "Хто CEO Microsoft?", "a": ["наделла", "nadella", "сатья"]},
        {"q": "Хто створив Facebook?", "a": ["цукерберг", "zuckerberg", "марк"]},
        {"q": "Яка країна виробляє Volvo?", "a": ["швеція", "sweden"]},
        {"q": "Яка країна виробляє Porsche?", "a": ["німеччина", "germany"]},
        {"q": "Що означає GTI у Volkswagen?", "a": ["grand touring injection"]},
    ],
    5: [  # Genius - найскладніше
        {"q": "Що означає BMW (повністю)?", "a": ["bayerische motoren werke"]},
        {"q": "Яка електрична модель Porsche?", "a": ["taycan", "тайкан"]},
        {"q": "Скільки буде 13^2?", "a": ["169"]},
        {"q": "Скільки буде sqrt(196)?", "a": ["14"]},
        {"q": "Скільки буде 2^16?", "a": ["65536"]},
        {"q": "Який порт для PostgreSQL за замовчуванням?", "a": ["5432"]},
        {"q": "Який порт для MySQL за замовчуванням?", "a": ["3306"]},
        {"q": "Який порт для MongoDB за замовчуванням?", "a": ["27017"]},
        {"q": "Який порт для Redis за замовчуванням?", "a": ["6379"]},
        {"q": "Що означає SOLID (перша літера)?", "a": ["single responsibility"]},
        {"q": "Що означає REST?", "a": ["representational state transfer"]},
        {"q": "Що означає JWT?", "a": ["json web token"]},
        {"q": "Що означає CORS?", "a": ["cross origin resource sharing", "cross-origin resource sharing"]},
        {"q": "Який HTTP код означає 'Not Found'?", "a": ["404"]},
        {"q": "Який HTTP код означає 'Internal Server Error'?", "a": ["500"]},
        {"q": "Який HTTP код означає 'Unauthorized'?", "a": ["401"]},
        {"q": "Який HTTP код означає 'Created'?", "a": ["201"]},
        {"q": "Яка часова складність binary search?", "a": ["o(log n)", "log n", "o(logn)"]},
        {"q": "Яка часова складність bubble sort?", "a": ["o(n^2)", "n^2", "o(n2)"]},
        {"q": "Що означає ACID в базах даних (перша літера)?", "a": ["atomicity", "атомарність"]},
    ],
}

LEVEL_REWARDS = {
    1: 20,   # Easy
    2: 35,   # Medium
    3: 50,   # Hard
    4: 75,   # Expert
    5: 100,  # Genius
}

LEVEL_NAMES = {
    1: "🟢 Easy",
    2: "🟡 Medium",
    3: "🟠 Hard",
    4: "🔴 Expert",
    5: "💀 Genius",
}


# === ROASTS AND COMPLIMENTS ===
from jokes_db import get_random_roast, get_random_compliment


async def roast(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Roast a user"""
    # Get target user
    if update.message.reply_to_message:
        target = update.message.reply_to_message.from_user
        target_name = target.first_name or target.username or "Анонім"
    elif context.args:
        target_name = " ".join(context.args).replace("@", "")
    else:
        # Roast yourself
        target_name = update.effective_user.first_name or "Хтось"

    roast_text = get_random_roast(target_name)

    await update.message.reply_text(f"🔥 {roast_text}")


async def compliment(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Compliment a user"""
    # Get target user
    if update.message.reply_to_message:
        target = update.message.reply_to_message.from_user
        target_name = target.first_name or target.username or "Анонім"
    elif context.args:
        target_name = " ".join(context.args).replace("@", "")
    else:
        # Compliment yourself
        target_name = update.effective_user.first_name or "Ти"

    compliment_text = get_random_compliment(target_name)

    await update.message.reply_text(f"💖 {compliment_text}")
SLOT_SYMBOLS = ['🍒', '🍋', '🍊', '🍇', '🔔', '⭐', '7️⃣', '💎']
SLOT_WEIGHTS = [25, 20, 18, 15, 10, 7, 4, 1]  # probability weights

SLOT_PAYOUTS = {
    ('💎', '💎', '💎'): 100,  # Jackpot
    ('7️⃣', '7️⃣', '7️⃣'): 50,
    ('⭐', '⭐', '⭐'): 25,
    ('🔔', '🔔', '🔔'): 15,
    ('🍇', '🍇', '🍇'): 10,
    ('🍊', '🍊', '🍊'): 8,
    ('🍋', '🍋', '🍋'): 5,
    ('🍒', '🍒', '🍒'): 3,
}

STARTING_BALANCE = 100
DEFAULT_BET = 10


def get_balance(user_id: str) -> int:
    """Get user balance, create if not exists"""
    if user_id not in balances:
        balances[user_id] = {'coins': STARTING_BALANCE, 'name': ''}
    return balances[user_id]['coins']


def update_balance(user_id: str, amount: int, name: str = ''):
    """Update user balance"""
    if user_id not in balances:
        balances[user_id] = {'coins': STARTING_BALANCE, 'name': name}
    balances[user_id]['coins'] += amount
    if name:
        balances[user_id]['name'] = name
    save_json(BALANCE_FILE, balances)


def spin_slots():
    """Spin the slot machine"""
    return tuple(random.choices(SLOT_SYMBOLS, weights=SLOT_WEIGHTS, k=3))


def calculate_winnings(result: tuple, bet: int) -> int:
    """Calculate winnings based on result"""
    # Check for three of a kind
    if result in SLOT_PAYOUTS:
        return bet * SLOT_PAYOUTS[result]

    # Two of a kind
    if result[0] == result[1] or result[1] == result[2] or result[0] == result[2]:
        return bet  # Return bet (no loss)

    return 0  # Loss


async def slots(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Play slots"""
    user = update.effective_user
    user_id = str(user.id)
    user_name = user.first_name or user.username or "Анонім"

    # Parse bet amount
    bet = DEFAULT_BET
    if context.args:
        try:
            bet = int(context.args[0])
            if bet < 1:
                bet = 1
            elif bet > 1000:
                bet = 1000
        except ValueError:
            pass

    # Check balance
    balance = get_balance(user_id)
    if balance < bet:
        await update.message.reply_text(
            f"💸 Недостатньо коінів!\n"
            f"Твій баланс: {balance} 🪙\n"
            f"Ставка: {bet} 🪙\n\n"
            f"_Почекай завтра на поповнення або грай менше_",
            parse_mode="Markdown"
        )
        return

    # Spin!
    result = spin_slots()
    winnings = calculate_winnings(result, bet)
    profit = winnings - bet

    # Update balance
    update_balance(user_id, profit, user_name)
    new_balance = get_balance(user_id)

    # Build message
    slot_display = f"╔══════════╗\n║ {result[0]} │ {result[1]} │ {result[2]} ║\n╚══════════╝"

    if winnings > bet:
        # Big win
        if result == ('💎', '💎', '💎'):
            msg = f"🎰 *ДЖЕКПОТ!!!* 🎰\n\n{slot_display}\n\n💎💎💎 НЕЙМОВІРНО! 💎💎💎\n\n"
        elif result == ('7️⃣', '7️⃣', '7️⃣'):
            msg = f"🎰 *MEGA WIN!* 🎰\n\n{slot_display}\n\n🔥🔥🔥 КРАСАВА! 🔥🔥🔥\n\n"
        else:
            msg = f"🎰 *ВИГРАШ!* 🎰\n\n{slot_display}\n\n"
        msg += f"Ставка: {bet} 🪙\nВиграш: +{winnings} 🪙\nБаланс: {new_balance} 🪙"
    elif winnings == bet:
        msg = f"🎰 Майже! 🎰\n\n{slot_display}\n\nСтавка повернута\nБаланс: {new_balance} 🪙"
    else:
        msg = f"🎰 Не пощастило 🎰\n\n{slot_display}\n\nВтрата: -{bet} 🪙\nБаланс: {new_balance} 🪙"

    await update.message.reply_text(msg, parse_mode="Markdown")


async def balance(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Check balance"""
    user = update.effective_user
    user_id = str(user.id)
    user_name = user.first_name or user.username or "Анонім"

    bal = get_balance(user_id)
    if user_id in balances:
        balances[user_id]['name'] = user_name

    await update.message.reply_text(
        f"💰 *Баланс {user_name}*\n\n"
        f"🪙 {bal} коінів\n\n"
        f"_/slots <ставка> - грати (за замовч. {DEFAULT_BET})_",
        parse_mode="Markdown"
    )


async def leaderboard(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Show casino leaderboard"""
    if not balances:
        await update.message.reply_text("🏆 Ще немає гравців!")
        return

    # Sort by coins
    sorted_players = sorted(
        balances.items(),
        key=lambda x: x[1]['coins'],
        reverse=True
    )[:10]  # Top 10

    lines = ["🏆 *ЛІДЕРБОРД КАЗИНО* 🏆\n"]

    medals = ['🥇', '🥈', '🥉']
    for i, (user_id, data) in enumerate(sorted_players):
        medal = medals[i] if i < 3 else f"{i+1}."
        name = data.get('name', 'Unknown')
        coins = data['coins']
        lines.append(f"{medal} {name}: {coins} 🪙")

    await update.message.reply_text("\n".join(lines), parse_mode="Markdown")


async def daily_bonus(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Give daily bonus coins or riddle for extra coins"""
    global bonus_claims, riddle_state

    user = update.effective_user
    user_id = str(user.id)
    user_name = user.first_name or user.username or "Анонім"
    today = datetime.now().strftime("%Y-%m-%d")

    # Check if user has active riddle
    if user_id in riddle_state:
        riddle = riddle_state[user_id]
        level = riddle.get('level', 1)
        reward = LEVEL_REWARDS.get(level, 50)
        level_name = LEVEL_NAMES.get(level, "🟢 Easy")

        await update.message.reply_text(
            f"🧩 *У тебе вже є загадка!*\n\n"
            f"Рівень: {level_name}\n"
            f"❓ {riddle['q']}\n"
            f"💰 Нагорода: {reward} 🪙\n\n"
            f"Відповідай в чат!",
            parse_mode="Markdown"
        )
        return

    # Get bonus count for today
    user_bonus_data = bonus_claims.get(user_id, {"date": "", "count": 0})

    if user_bonus_data.get("date") != today:
        # First bonus of the day — free 50 coins
        bonus = 50
        update_balance(user_id, bonus, user_name)
        new_balance = get_balance(user_id)

        bonus_claims[user_id] = {"date": today, "count": 1}
        save_json(BONUS_FILE, bonus_claims)

        await update.message.reply_text(
            f"🎁 *Щоденний бонус!*\n\n"
            f"+{bonus} 🪙\n"
            f"Баланс: {new_balance} 🪙\n\n"
            f"_Хочеш ще? Напиши /bonus для загадки!_",
            parse_mode="Markdown"
        )
    else:
        # Already claimed — give riddle based on count
        count = user_bonus_data.get("count", 0)

        # Determine level based on bonus count (every 5 bonuses = next level)
        # count 1-5 = level 1, count 6-10 = level 2, etc.
        level = min(5, (count // 5) + 1)

        riddle = random.choice(RIDDLES_BY_LEVEL[level])
        riddle_with_meta = {**riddle, "level": level}
        riddle_state[user_id] = riddle_with_meta
        save_json(RIDDLE_STATE_FILE, riddle_state)

        reward = LEVEL_REWARDS[level]
        level_name = LEVEL_NAMES[level]

        await update.message.reply_text(
            f"🧩 *Загадка #{count + 1}*\n\n"
            f"Рівень: {level_name}\n"
            f"❓ {riddle['q']}\n"
            f"💰 Нагорода: {reward} 🪙\n\n"
            f"_Напиши відповідь в чат!_",
            parse_mode="Markdown"
        )


async def check_riddle_answer(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Check if message is a riddle answer"""
    global riddle_state, bonus_claims

    user = update.effective_user
    user_id = str(user.id)

    if user_id not in riddle_state:
        return False

    text = update.message.text.lower().strip()
    riddle = riddle_state[user_id]

    # Check answer
    correct = any(ans.lower() in text or text in ans.lower() for ans in riddle['a'])

    if correct:
        # Correct answer!
        level = riddle.get('level', 1)
        bonus = LEVEL_REWARDS.get(level, 50)
        level_name = LEVEL_NAMES.get(level, "🟢 Easy")

        user_name = user.first_name or user.username or "Анонім"
        update_balance(user_id, bonus, user_name)
        new_balance = get_balance(user_id)

        # Update bonus count
        today = datetime.now().strftime("%Y-%m-%d")
        user_bonus_data = bonus_claims.get(user_id, {"date": today, "count": 0})
        if user_bonus_data.get("date") == today:
            user_bonus_data["count"] = user_bonus_data.get("count", 0) + 1
        else:
            user_bonus_data = {"date": today, "count": 1}
        bonus_claims[user_id] = user_bonus_data
        save_json(BONUS_FILE, bonus_claims)

        del riddle_state[user_id]
        save_json(RIDDLE_STATE_FILE, riddle_state)

        # Calculate next level
        next_count = user_bonus_data["count"]
        next_level = min(5, (next_count // 5) + 1)
        next_level_name = LEVEL_NAMES.get(next_level, "🟢 Easy")

        await update.message.reply_text(
            f"✅ *Правильно!* {level_name}\n\n"
            f"+{bonus} 🪙\n"
            f"Баланс: {new_balance} 🪙\n\n"
            f"_Наступна загадка: {next_level_name}_",
            parse_mode="Markdown"
        )
        return True

    return False


async def midnight_bonus(context: ContextTypes.DEFAULT_TYPE):
    """Give +100 coins to all players at midnight"""
    global balances

    if not balances:
        return

    bonus = 100

    for user_id in balances:
        balances[user_id]['coins'] += bonus

    save_json(BALANCE_FILE, balances)
    logger.info(f"Midnight bonus: +{bonus} coins to {len(balances)} users")

    # Notify active chats
    for chat_id in active_chats:
        try:
            await context.bot.send_message(
                chat_id=chat_id,
                text=f"🌙 *Опівнічний бонус!*\n\n"
                     f"Всі гравці отримали +{bonus} 🪙\n"
                     f"Солодких снів! 💤",
                parse_mode="Markdown"
            )
        except Exception as e:
            logger.error(f"Failed to send midnight bonus to {chat_id}: {e}")

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
        "*Команди:*\n"
        "/check <text> - перевірити текст\n"
        "/stats - статистика\n"
        "/mute - вимкнути трекінг\n"
        "/unmute - увімкнути трекінг\n\n"
        "*Розваги:*\n"
        "/car - яка твоя машина? 🚗\n"
        "/slots <ставка> - слоти 🎰\n"
        "/balance - баланс 💰\n"
        "/top - лідерборд 🏆\n"
        "/bonus - щоденний бонус 🎁\n"
        "/roast - підколка 🔥\n"
        "/compliment - комплімент 💖",
        parse_mode="Markdown"
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
    save_json(MUTED_FILE, list(muted_users))
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
    save_json(MUTED_FILE, list(muted_users))
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

    # Check if this is a riddle answer first
    if await check_riddle_answer(update, context):
        return  # Was a riddle answer, don't process further

    # Skip if user is muted
    if user_id in muted_users:
        return

    user_name = user.first_name or user.username or str(user_id)

    result = classifier.predict(text)

    # Initialize user stats if needed (use string keys for JSON)
    user_id_str = str(user_id)
    if user_id_str not in stats:
        stats[user_id_str] = {'work': 0, 'personal': 0, 'name': user_name}
    if user_id_str not in daily_stats:
        daily_stats[user_id_str] = {'work': 0, 'personal': 0, 'name': user_name}

    # Update statistics
    if result['is_work']:
        stats[user_id_str]['work'] += 1
        daily_stats[user_id_str]['work'] += 1
    else:
        stats[user_id_str]['personal'] += 1
        daily_stats[user_id_str]['personal'] += 1

    # Save stats
    save_json(STATS_FILE, stats)
    save_json(DAILY_STATS_FILE, daily_stats)
    save_json(CHATS_FILE, list(active_chats))

    # Log
    logger.info(f"[{user_name}] [{result['label']}] ({result['confidence']:.0%}) {text[:50]}...")

    # Reply only if work with 95%+ confidence
    if result['is_work'] and result['confidence'] >= 0.95:
        # React with clown emoji
        try:
            await update.message.set_reaction(reaction="🤡")
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
    daily_stats.clear()
    save_json(DAILY_STATS_FILE, daily_stats)
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
    app.add_handler(CommandHandler("slots", slots))
    app.add_handler(CommandHandler("slot", slots))
    app.add_handler(CommandHandler("balance", balance))
    app.add_handler(CommandHandler("bal", balance))
    app.add_handler(CommandHandler("top", leaderboard))
    app.add_handler(CommandHandler("leaderboard", leaderboard))
    app.add_handler(CommandHandler("bonus", daily_bonus))
    app.add_handler(CommandHandler("roast", roast))
    app.add_handler(CommandHandler("compliment", compliment))
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

    # Schedule midnight bonus at 00:00 Kyiv time (22:00 UTC)
    job_queue.run_daily(
        midnight_bonus,
        time=time(hour=22, minute=0, second=0),  # 00:00 Kyiv (UTC+2)
        name="midnight_bonus"
    )
    logger.info("Midnight bonus scheduled for 00:00 Kyiv time")

    logger.info("Bot starting...")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
