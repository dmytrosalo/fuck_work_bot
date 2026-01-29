"""
Telegram Bot with Work Classifier
For deployment on Fly.io
"""

import os
import re
import json
import random
import logging
import aiohttp
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
GENERATED_RIDDLES_FILE = DATA_DIR / 'generated_riddles.json'  # AI-generated riddles

# Gemini API
GEMINI_API_KEY = os.environ.get('GEMINI_API_KEY', '')


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
bonus_claims = load_json(BONUS_FILE, {})  # {user_id: {"date": "2024-01-15", "count": 5}}
riddle_state = load_json(RIDDLE_STATE_FILE, {})  # {user_id: {"riddle": ..., "answer": ...}}
generated_riddles = load_json(GENERATED_RIDDLES_FILE, {})  # {1: [...], 2: [...], ...}

logger.info(f"Loaded stats: {len(stats)} users, {len(daily_stats)} daily, {len(muted_users)} muted, {len(active_chats)} chats, {len(balances)} balances")


async def generate_riddles_with_gemini():
    """Generate new riddles using Gemini Flash API"""
    global generated_riddles

    if not GEMINI_API_KEY:
        logger.warning("GEMINI_API_KEY not set, skipping riddle generation")
        return False

    prompt = """Згенеруй по 40 питань на кожен рівень складності унікальних питань для вікторини українською мовою.

Формат JSON (без markdown, тільки чистий JSON):
{
    "1": [{"q": "питання", "a": ["відповідь1", "відповідь2"]}],
    "2": [{"q": "питання", "a": ["відповідь"]}],
    "3": [{"q": "питання", "a": ["відповідь"]}],
    "4": [{"q": "питання", "a": ["відповідь"]}],
    "5": [{"q": "питання", "a": ["відповідь"]}]
}

Рівні складності:
1 (Easy): 4 питання - базова математика, дитячі загадки
2 (Medium): 4 питання - географія, відомі фільми, музика
3 (Hard): 4 питання - історія, література, космос
4 (Expert): 4 питання - наука, мистецтво, складні факти
5 (Genius): 4 питання - дуже складна історія, філософія, рідкісні факти

Вимоги:
- Відповіді коротші, в нижньому регістрі
- Для чисел можна давати кілька варіантів: ["42", "сорок два"]
- Теми: кіно, музика, ігри, історія, географія, наука, спорт, кухня
- КАТЕГОРИЧНО НЕ ПИШИ про IT, програмування, роботу, офіс, технології
- НЕ згадуй росію і все що пов'язане з нею її культурою, політикою, географією, історією і тд
- Питання мають бути цікавими і відволікати від роботи"""

    try:
        url = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={GEMINI_API_KEY}"

        async with aiohttp.ClientSession() as session:
            async with session.post(
                url,
                json={
                    "contents": [{"parts": [{"text": prompt}]}],
                    "generationConfig": {"temperature": 0.9}
                },
                headers={"Content-Type": "application/json"}
            ) as response:
                if response.status != 200:
                    logger.error(f"Gemini API error: {response.status}")
                    return False

                data = await response.json()
                text = data['candidates'][0]['content']['parts'][0]['text']

                # Clean markdown if present
                text = text.strip()
                if text.startswith("```"):
                    text = text.split("\n", 1)[1]
                if text.endswith("```"):
                    text = text.rsplit("```", 1)[0]
                text = text.strip()

                new_riddles = json.loads(text)

                # Convert string keys to int
                generated_riddles = {int(k): v for k, v in new_riddles.items()}
                save_json(GENERATED_RIDDLES_FILE, generated_riddles)

                total = sum(len(v) for v in generated_riddles.values())
                logger.info(f"Generated {total} new riddles with Gemini")
                return True

    except Exception as e:
        logger.error(f"Error generating riddles: {e}")
        return False


async def refresh_riddles_job(context: ContextTypes.DEFAULT_TYPE):
    """Job to refresh riddles at noon"""
    success = await generate_riddles_with_gemini()

    if success:
        # Notify active chats
        for chat_id in active_chats:
            try:
                await context.bot.send_message(
                    chat_id=chat_id,
                    text="🧩 *Нові загадки!*\n\n"
                         "Gemini згенерував свіжі питання.\n"
                         "Напиши /bonus щоб відволіктись від роботи!",
                    parse_mode="Markdown"
                )
            except Exception as e:
                logger.error(f"Failed to notify {chat_id}: {e}")


def get_riddles_for_level(level: int) -> list:
    """Get riddles for a level, combining static and generated"""
    static = RIDDLES_BY_LEVEL.get(level, [])
    generated = generated_riddles.get(level, [])

    # Combine both, prefer generated if available
    combined = generated + static if generated else static
    return combined


# Helper for currency declension
def get_currency_name(amount: int) -> str:
    """Return correct form of 'богдудіки' based on amount"""
    # 1 богдудік
    # 2, 3, 4 богдудіка
    # 5-20, 0, 25-30, ... богдудіків

    n = abs(amount) % 100
    n1 = n % 10

    if 11 <= n <= 19:
        return "богдудіків"
    if n1 == 1:
        return "богдудік"
    if 2 <= n1 <= 4:
        return "богдудіка"
    return "богдудіків"


# -------------------------------------------------------------------------
# CONSTANTS & CONFIG
# -------------------------------------------------------------------------

# Reward amounts for riddle levels
LEVEL_REWARDS = {
    1: 20,   # 20 богдудіків
    2: 35,   # 35 богдудіків
    3: 50,   # 50 богдудіків
    4: 75,   # 75 богдудіків
    5: 100   # 100 богдудіків
}

LEVEL_NAMES = {
    1: "🟢 Easy",
    2: "🟡 Medium",
    3: "🟠 Hard",
    4: "🔴 Expert",
    5: "🟣 Genius"
}

# === RIDDLES DATABASE BY DIFFICULTY ===
RIDDLES_BY_LEVEL = {
    1: [  # Easy - прості факти
        {"q": "Скільки місяців у році мають 28 днів?", "a": ["усі", "12", "всі"]},
        {"q": "Яка планета третя від Сонця?", "a": ["земля", "earth"]},
        {"q": "Скільки буде 6 * 7?", "a": ["42"]},
        {"q": "З чого роблять родзинки?", "a": ["виноград", "з винограду"]},
        {"q": "Яка геометрична фігура не має кутів?", "a": ["коло", "овал", "круг"]},
        {"q": "Що йде, не рухаючись з місця?", "a": ["час", "годинник"]},
        {"q": "Скільки ніг у павука?", "a": ["8"]},
        {"q": "Який колір вийде, якщо змішати червоний і жовтий?", "a": ["оранжевий", "помаранчевий"]},
        {"q": "Скільки гравців у футбольній команді на полі?", "a": ["11"]},
        {"q": "Що більше: слон чи кит?", "a": ["кит"]},
        {"q": "Скільки кольорів у світлофорі?", "a": ["3", "три"]},
        {"q": "Що збирають бджоли?", "a": ["нектар", "мед", "пилок"]},
        {"q": "Як звати подружку Міккі Мауса?", "a": ["мінні", "minnie"]},
        {"q": "Скільки коліс у велосипеда?", "a": ["2", "два"]},
        {"q": "Що протилежне до 'день'?", "a": ["ніч"]},
        {"q": "Як називається замерзла вода?", "a": ["лід", "крига"]},
        {"q": "Скільки пальців на двох руках?", "a": ["10", "десять"]},
        {"q": "Хто дає нам молоко?", "a": ["корова", "коза"]},
        {"q": "Якого кольору сонце?", "a": ["жовте", "жовтий"]},
        {"q": "Скільки літер у слові 'Яблуко'?", "a": ["6", "шість"]},
    ],
    2: [  # Medium - географія, природа, культура
        {"q": "На якому материку знаходиться Єгипет?", "a": ["африка"]},
        {"q": "Хто співає пісню 'Show Must Go On'?", "a": ["queen", "квін", "фредді", "mercury"]},
        {"q": "Скільки кілець на олімпійському прапорі?", "a": ["5", "п'ять"]},
        {"q": "Який газ ми видихаємо?", "a": ["вуглекислий"]},
        {"q": "Яка найбільша тварина на Землі?", "a": ["синій кит", "кит", "blue whale"]},
        {"q": "Яка столиця Польщі?", "a": ["варшава", "warsaw"]},
        {"q": "Хто написав книгу 'Гаррі Поттер'?", "a": ["роулінг", "rowling"]},
        {"q": "З чого виготовляють папір?", "a": ["дерево", "деревина", "з дерева"]},
        {"q": "В якому місті знаходиться Ейфелева вежа?", "a": ["париж", "paris"]},
        {"q": "Як називається японське мистецтво складання паперу?", "a": ["орігамі"]},
        {"q": "Яка столиця Іспанії?", "a": ["мадрид", "madrid"]},
        {"q": "Хто співає 'Thriller'?", "a": ["майкл джексон", "jackson", "джексон"]},
        {"q": "Яка найшвидша тварина на суші?", "a": ["гепард", "cheetah"]},
        {"q": "Який океан найбільший?", "a": ["тихий", "pacific"]},
        {"q": "В якій країні знаходяться піраміди?", "a": ["єгипет", "egypt"]},
        {"q": "Яка столиця Китаю?", "a": ["пекін", "beijing"]},
        {"q": "Яка валюта у Великобританії?", "a": ["фунт", "pound"]},
        {"q": "Скільки гравців у баскетбольній команді на полі?", "a": ["5", "п'ять"]},
        {"q": "У якої ягоди насіння ззовні?", "a": ["полуниця", "суниця"]},
        {"q": "Хто такий Немо з мультика?", "a": ["риба", "рибка", "клоун"]},
    ],
    3: [  # Hard - історія, наука
        {"q": "В якому році сталася Чорнобильська катастрофа?", "a": ["1986"]},
        {"q": "Хто написав повість 'Тіні забутих предків'?", "a": ["коцюбинський"]},
        {"q": "Яка змія вважається найшвидшою у світі?", "a": ["чорна мамба", "мамба"]},
        {"q": "Яка столиця Туреччини?", "a": ["анкара", "ankara"]},
        {"q": "Який хімічний елемент має символ Ag?", "a": ["срібло", "silver"]},
        {"q": "В якому році затонув Титанік?", "a": ["1912"]},
        {"q": "Яка найвища вершина Карпат?", "a": ["говерла"]},
        {"q": "Хто відкрив Америку?", "a": ["колумб", "columbus"]},
        {"q": "Скільки зубів у дорослої людини?", "a": ["32"]},
        {"q": "Хто намалював Мону Лізу?", "a": ["да вінчі", "леонардо"]},
        {"q": "Хто винайшов телефон?", "a": ["белл", "bell"]},
        {"q": "Наука про зірки це...?", "a": ["астрономія"]},
        {"q": "Яка столиця Канади?", "a": ["оттава", "ottawa"]},
        {"q": "Яка планета має кільця?", "a": ["сатурн", "saturn"]},
        {"q": "Який символ у золота?", "a": ["au"]},
        {"q": "Хто написав про Шерлока Холмса?", "a": ["дойл", "doyle"]},
        {"q": "Перша жінка в космосі?", "a": ["терешкова"]},
        {"q": "В якому році закінчилась Друга світова?", "a": ["1945"]},
        {"q": "Найтвердіший природний матеріал?", "a": ["алмаз", "діамант"]},
        {"q": "Швидкість звуку в повітрі (м/с, приблизно)?", "a": ["340", "343", "330"]},
    ],
    4: [  # Expert - складні факти, мистецтво
        {"q": "Хто винайшов динаміт?", "a": ["нобель", "nobel"]},
        {"q": "Скільки пар хромосом у здорової людини?", "a": ["23"]},
        {"q": "Яка війна вважається найкоротшою в історії (38 хв)?", "a": ["англо-занзібарська", "занзібарська"]},
        {"q": "Хто написав картину 'Дівчина з перловою сережкою'?", "a": ["вермер", "vermeer"]},
        {"q": "Температура абсолютного нуля за Цельсієм?", "a": ["-273", "-273.15"]},
        {"q": "Як називається страх замкнутого простору?", "a": ["клаустрофобія"]},
        {"q": "Яка річка найдовша в Європі?", "a": ["волга"]},
        {"q": "Хто написав 'Портрет Доріана Грея'?", "a": ["уайльд", "wilde"]},
        {"q": "Яка країна подарувала США Статую Свободи?", "a": ["франція"]},
        {"q": "Який елемент найпоширеніший у Всесвіті?", "a": ["водень", "h"]},
        {"q": "Хто написав 'Старий і море'?", "a": ["хемінгуей", "hemingway"]},
        {"q": "Яка столиця Австралії?", "a": ["канберра"]},
        {"q": "Енергетична станція клітини?", "a": ["мітохондрія"]},
        {"q": "Хто написав 'Пори року' (музика)?", "a": ["вівальді", "vivaldi"]},
        {"q": "В якому році впав Берлінський мур?", "a": ["1989"]},
        {"q": "Найменша кістка в тілі людини?", "a": ["стремінце", "у вусі", "stapes"]},
        {"q": "Хто написав '1984'?", "a": ["орвелл", "orwell"]},
        {"q": "Елемент, що рідкий при кімнатній t (неметал)?", "a": ["бром", "bromine"]},
        {"q": "Хто намалював 'Герніку'?", "a": ["пікассо", "picasso"]},
        {"q": "Яка місія Аполлон висадилась на Місяць?", "a": ["11", "аполлон 11"]},
    ],
    5: [  # Genius - ерудиція
        {"q": "Скільки часу світло йде від Сонця до Землі (приблизно)?", "a": ["8 хв", "8 хвилин", "500 с"]},
        {"q": "Яка країна має найбільшу кількість озер?", "a": ["канада", "canada"]},
        {"q": "Який єдиний метал є рідким при кімнатній температурі?", "a": ["ртуть", "mercury"]},
        {"q": "Як звали давньогрецьку богиню мудрості?", "a": ["афіна", "athena"]},
        {"q": "Хто був вчителем Олександра Македонського?", "a": ["арістотель", "aristotle"]},
        {"q": "Як звали коня Дон Кіхота?", "a": ["росінант", "rossinante"]},
        {"q": "Яка країна першою надала жінкам право голосу?", "a": ["нова зеландія"]},
        {"q": "Хто написав 'Майстер і Маргарита'?", "a": ["булгаков"]},
        {"q": "Як звали першу собаку-космонавта?", "a": ["лайка"]},
        {"q": "У якому році винайшли пеніцилін?", "a": ["1928"]},
        {"q": "Яка столиця Казахстану?", "a": ["астана", "astana"]},
        {"q": "Хто написав 'Злочин і кара'?", "a": ["достоєвський"]},
        {"q": "Скільки клавіш на стандартному піаніно?", "a": ["88"]},
        {"q": "Що означає Carpe Diem?", "a": ["лови момент", "лови день"]},
        {"q": "Хто відкрив полоній?", "a": ["кюрі", "curie", "марія кюрі"]},
        {"q": "Яка висота Евересту (м)?", "a": ["8848", "8849"]},
        {"q": "Найглибше озеро у світі?", "a": ["байкал"]},
        {"q": "Хто написав 'Державець' (The Prince)?", "a": ["макіавеллі", "machiavelli"]},
        {"q": "В якому році почалася Велика французька революція?", "a": ["1789"]},
        {"q": "Яка валюта у Швейцарії?", "a": ["франк", "franc", "chf"]},
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
                await update.message.reply_text("❌ Мінімальна ставка: 1 🪙")
                return
        except ValueError:
            await update.message.reply_text("❌ Введіть число!")
            return

    # Check balance
    balance = get_balance(user_id)
    if balance < bet:
        currency = get_currency_name(bet)
        await update.message.reply_text(
            f"💸 Недостатньо {currency}!\n"
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
        f"🪙 {bal} шмеркелів\n\n"
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


def escape_markdown(text: str) -> str:
    """Escape characters for Markdown V2"""
    # Note: Telegram Markdown (V1) supports *bold*, _italic_, [link](url), `code`, ```pre```
    # But usually it's safer to just replace * and _ if we don't intend formatting.
    # However, user wants nice formatting.
    # The error "can't find end of the entity" suggests mismatched * or _.
    # We should escape * and _ in content that is NOT meant to be formatted.
    escape_chars = r"_*[]()~`>#+-=|{}.!"
    return "".join(f"\\{char}" if char in escape_chars else char for char in str(text))


async def daily_bonus(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Give daily bonus coins or riddle for extra coins"""
    global bonus_claims, riddle_state

    user = update.effective_user
    user_id = str(user.id)
    user_name = escape_markdown(user.first_name or user.username or "Анонім")
    today = datetime.now().strftime("%Y-%m-%d")

    # Check if user has active riddle
    if user_id in riddle_state:
        riddle = riddle_state[user_id]
        level = riddle.get('level', 1)
        reward = LEVEL_REWARDS.get(level, 50)
        level_name = LEVEL_NAMES.get(level, "🟢 Easy")

        # Escape riddle text just in case
        q_text = escape_markdown(riddle['q'])

        await update.message.reply_text(
            f"🧩 *У тебе вже є загадка!*\n\n"
            f"Рівень: {level_name}\n"
            f"❓ {q_text}\n"
            f"💰 Нагорода: {reward} 🪙\n\n"
            f"Відповідай в чат!",
            parse_mode="Markdown"
        )
        return

    # Get bonus count for today
    user_bonus_data = bonus_claims.get(user_id, {"date": "", "count": 0})

    if user_bonus_data.get("date") != today:
        # First bonus of the day — free 50 шмеркелів
        bonus = 50
        update_balance(user_id, bonus, user.first_name or "Unknown") # Store unescaped name in DB
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

        # Determine level based on bonus count (5 riddles per level)
        # count 1-5 = level 1
        # count 6-10 = level 2
        # ...
        # count 21-25 = level 5
        level = (count - 1) // 5 + 1

        if level > 5:
            await update.message.reply_text(
                f"🛑 *Ліміт вичерпано!*\n\n"
                f"Ти пройшов усі 5 рівнів на сьогодні.\n"
                f"Приходь завтра за новими шмеркелями!",
                parse_mode="Markdown"
            )
            return

        riddles_list = get_riddles_for_level(level)
        if not riddles_list:
            riddle = {"q": "Питання закінчились :(", "a": ["pass"]}
        else:
            # Use separate RNG seeded by date to get a consistent daily set
            # This ensures we pick different 5 riddles each day if the pool is large enough
            date_seed = int(datetime.now().strftime("%Y%m%d")) + level
            rng = random.Random(date_seed)

            # Shuffle a copy of the list
            daily_riddles = riddles_list.copy()
            rng.shuffle(daily_riddles)

            # Use simple modulo to cycle through the 5 daily selected riddles
            # (count - 1) % 5 gives 0, 1, 2, 3, 4
            riddle_index = (count - 1) % 5

            # Ensure we don't go out of bounds if list is small
            if riddle_index >= len(daily_riddles):
                riddle_index = riddle_index % len(daily_riddles)

            riddle = daily_riddles[riddle_index]

        riddle_with_meta = {**riddle, "level": level}
        riddle_state[user_id] = riddle_with_meta
        save_json(RIDDLE_STATE_FILE, riddle_state)

        reward = LEVEL_REWARDS.get(level, 50)
        level_name = LEVEL_NAMES.get(level, f"Level {level}")

        q_text = escape_markdown(riddle['q'])

        await update.message.reply_text(
            f"🧩 *Загадка #{count} (Рівень {level})*\n\n"
            f"Рівень: {level_name}\n"
            f"❓ {q_text}\n"
            f"💰 Нагорода: {reward} шмеркелів\n\n"
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

    # Check answer using regex word boundaries to avoid partial matches
    # e.g. "1" should NOT match "11" or "12"
    # e.g. "cat" should NOT match "caterpillar"
    # But "11" SHOULD match "I think it is 11"
    correct = False
    for ans in riddle['a']:
        ans_clean = ans.lower().strip()
        # Create regex pattern: \b(escaped_answer)\b
        pattern = r"\b" + re.escape(ans_clean) + r"\b"
        if re.search(pattern, text):
            correct = True
            break

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
        # Formula: (count - 1) // 5 + 1. Since next_count is essentially 'current count for next riddle'
        #Wait, if next_count is say 25. Next riddle is 26th.
        #Actually next_count is the count COMPLETED?
        #No, standard is: Count 1 = done free bonus.
        #We just incremented it. So count=2. This means we have done 1 riddle attempts?
        #Wait. Logic in daily_bonus: 'count = user_bonus_data.get("count", 0)'.
        #If I have count=1 (free bonus). I get riddle. I solve it. Count becomes 2.
        #Next call to daily_bonus sees count=2. riddle_index = (2-1)%5 = 1. This is the 2nd riddle. Correct.

        # Checking if next riddle is available
        next_level = (next_count - 1) // 5 + 1

        next_msg = ""
        if next_level > 5:
             next_msg = "\n🎉 *Ти пройшов усі рівні на сьогодні!*"
        else:
             next_level_name = LEVEL_NAMES.get(next_level, f"Level {next_level}")
             next_msg = f"\n_Наступна загадка: {next_level_name}_"

        await update.message.reply_text(
            f"✅ *Правильно!* {level_name}\n\n"
            f"+{bonus} 🪙\n"
            f"Баланс: {new_balance} шмеркелів{next_msg}",
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


async def reset_bonus(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Reset bonus history for user (back to level 1)"""
    global bonus_claims, riddle_state

    user = update.effective_user
    user_id = str(user.id)

    # Reset bonus count
    if user_id in bonus_claims:
        del bonus_claims[user_id]
        save_json(BONUS_FILE, bonus_claims)

    # Clear active riddle
    if user_id in riddle_state:
        del riddle_state[user_id]
        save_json(RIDDLE_STATE_FILE, riddle_state)

    await update.message.reply_text(
        "🔄 *Бонуси скинуто!*\n\n"
        "Твій рівень загадок повернувся на 🟢 Easy\n"
        "Напиши /bonus щоб почати заново!",
        parse_mode="Markdown"
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


async def generate_startup_idea_with_gemini() -> str:
    """Generate a funny/genius startup idea using Gemini"""
    if not GEMINI_API_KEY:
        return ""

    prompt = """Згенеруй одну коротку, смішну або геніальну ідею для стартапу українською мовою.
Це може бути щось абсурдне, але з ноткою логіки.
Приклад: "Uber для котів - щоб вони могли їздити в гості до інших котів без людей."
Без вступу, тільки сама ідея."""

    try:
        url = f"https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key={GEMINI_API_KEY}"

        async with aiohttp.ClientSession() as session:
            async with session.post(
                url,
                json={
                    "contents": [{"parts": [{"text": prompt}]}],
                    "generationConfig": {"temperature": 1.0}
                },
                headers={"Content-Type": "application/json"}
            ) as response:
                if response.status != 200:
                    logger.error(f"Gemini API error (startup): {response.status}")
                    return ""

                data = await response.json()
                text = data['candidates'][0]['content']['parts'][0]['text']
                return text.strip()

    except Exception as e:
        logger.error(f"Error generating startup idea: {e}")
        return ""


async def startup_idea_job(context: ContextTypes.DEFAULT_TYPE):
    """Job to post a startup idea"""
    idea = await generate_startup_idea_with_gemini()

    if idea:
        # Notify active chats
        for chat_id in active_chats:
            try:
                await context.bot.send_message(
                    chat_id=chat_id,
                    text=f"💡 *Ідея для стартапу на мільйон!*\n\n{idea}",
                    parse_mode="Markdown"
                )
            except Exception as e:
                logger.error(f"Failed to send startup idea to {chat_id}: {e}")


async def error_handler(update: object, context: ContextTypes.DEFAULT_TYPE) -> None:
    """Log the error and send a telegram message to notify the developer."""
    logger.error("Exception while handling an update:", exc_info=context.error)

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
    app.add_handler(CommandHandler("resetbonus", reset_bonus))
    app.add_handler(CommandHandler("roast", roast))
    app.add_handler(CommandHandler("compliment", compliment))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, on_message))

    # Add error handler
    app.add_error_handler(error_handler)

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

    # Schedule riddle refresh at 12:00 Kyiv time (10:00 UTC)
    if GEMINI_API_KEY:
        job_queue.run_daily(
            refresh_riddles_job,
            time=time(hour=10, minute=0, second=0),  # 12:00 Kyiv (UTC+2)
            name="refresh_riddles"
        )
        logger.info("Riddle refresh scheduled for 12:00 Kyiv time")

        # Schedule startup idea every 6 hours
        # First run after 10 seconds to verified it works
        job_queue.run_repeating(
            startup_idea_job,
            interval=timedelta(hours=6),
            first=10,
            name="startup_idea"
        )
        logger.info("Startup ideas scheduled every 6 hours")
    else:
        logger.warning("GEMINI_API_KEY not set, riddle refresh and startup ideas disabled")

    logger.info("Bot starting...")
    app.run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
