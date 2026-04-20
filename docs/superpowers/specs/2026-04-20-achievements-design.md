# Achievements System Design

## Overview

Achievement system for the Telegram bot TCG. Auto-tracked with notification on unlock + `/achievements` command to view progress. 50 achievements across 6 categories with coin rewards and display titles.

## Storage

### New table: `achievements`
```sql
CREATE TABLE IF NOT EXISTS achievements (
  user_id TEXT NOT NULL,
  achievement_id TEXT NOT NULL,
  unlocked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, achievement_id)
)
```

### New table: `user_stats`
```sql
CREATE TABLE IF NOT EXISTS user_stats (
  user_id TEXT PRIMARY KEY,
  duels_won INTEGER NOT NULL DEFAULT 0,
  duels_lost INTEGER NOT NULL DEFAULT 0,
  cards_stolen INTEGER NOT NULL DEFAULT 0,
  coins_robbed INTEGER NOT NULL DEFAULT 0,
  slots_played INTEGER NOT NULL DEFAULT 0,
  slots_won INTEGER NOT NULL DEFAULT 0,
  slots_max_win INTEGER NOT NULL DEFAULT 0,
  slots_streak INTEGER NOT NULL DEFAULT 0,
  slots_max_streak INTEGER NOT NULL DEFAULT 0,
  bj_played INTEGER NOT NULL DEFAULT 0,
  bj_won INTEGER NOT NULL DEFAULT 0,
  bj_blackjacks INTEGER NOT NULL DEFAULT 0,
  roasts_given INTEGER NOT NULL DEFAULT 0,
  roasts_received INTEGER NOT NULL DEFAULT 0,
  compliments_given INTEGER NOT NULL DEFAULT 0,
  quotes_added INTEGER NOT NULL DEFAULT 0,
  cards_gifted INTEGER NOT NULL DEFAULT 0,
  cards_burned INTEGER NOT NULL DEFAULT 0,
  wordle_played INTEGER NOT NULL DEFAULT 0,
  daily_claimed INTEGER NOT NULL DEFAULT 0,
  max_balance INTEGER NOT NULL DEFAULT 0,
  total_earned INTEGER NOT NULL DEFAULT 0,
  total_spent INTEGER NOT NULL DEFAULT 0,
  duel_streak INTEGER NOT NULL DEFAULT 0,
  max_duel_streak INTEGER NOT NULL DEFAULT 0,
  lose_streak INTEGER NOT NULL DEFAULT 0,
  max_lose_streak INTEGER NOT NULL DEFAULT 0,
  packs_opened INTEGER NOT NULL DEFAULT 0,
  commands_used TEXT NOT NULL DEFAULT ''
)
```

### New table: `user_titles`
```sql
CREATE TABLE IF NOT EXISTS user_titles (
  user_id TEXT PRIMARY KEY,
  active_title TEXT NOT NULL DEFAULT ''
)
```

## Achievement Definitions (hardcoded in Go)

Each achievement:
```go
type Achievement struct {
    ID          string
    Name        string
    Emoji       string
    Description string
    Category    string // "collection", "economy", "pvp", "gambling", "social", "secret"
    Reward      int    // coins
    Title       string // optional display title, empty if none
    Hidden      bool   // true for secret achievements
    Check       func(stats UserStatsRow, collectionStats CollectionStatsRow) bool
}
```

### Collection (10)
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| col_5 | Початківець | :card_file_box: | Зібрав 5 карток | 25 | |
| col_10 | Колекціонер | :flower_playing_cards: | Зібрав 10 карток | 50 | |
| col_25 | Знавець | :books: | Зібрав 25 карток | 100 | |
| col_50 | Справжній збирач | :trophy: | Зібрав 50 карток | 200 | Збирач |
| col_100 | Картковий магнат | :crown: | Зібрав 100 карток | 500 | Магнат |
| col_200 | Легенда колекцій | :star2: | Зібрав 200 карток | 1000 | Легенда |
| col_all_rarities | Райдуга | :rainbow: | Маєш картку кожної рідкості | 200 | |
| col_10_legendary | Золота жила | :sparkles: | Маєш 10+ легендарних карток | 500 | Золотий |
| col_full_common | Сірий кардинал | :black_circle: | Зібрав всі Common картки | 300 | |
| col_full_rare | Синій барон | :large_blue_diamond: | Зібрав всі Rare картки | 500 | Барон |

### Economy (8)
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| eco_1k | Перша тисяча | :moneybag: | Заробив 1,000 монет загалом | 50 | |
| eco_5k | Бізнесмен | :chart_with_upwards_trend: | Заробив 5,000 монет загалом | 100 | |
| eco_25k | Олігарх | :gem: | Заробив 25,000 монет загалом | 500 | Олігарх |
| eco_spend_10k | Транжира | :money_with_wings: | Витратив 10,000 монет загалом | 200 | |
| eco_rich | Багатій | :dollar: | Мав 1,000+ монет одночасно | 100 | Багатій |
| eco_daily_30 | Стабільність | :calendar: | Забрав /daily 30 разів | 200 | |
| eco_generous | Щедра душа | :gift: | Подарував карток на 500+ монет | 150 | Щедрий |
| eco_packs_50 | Шопоголік | :package: | Відкрив 50 паків | 200 | |

### PvP (8)
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| pvp_win_1 | Перша перемога | :crossed_swords: | Виграв першу дуель | 25 | |
| pvp_win_5 | Боєць | :boxing_glove: | Виграв 5 дуелей | 50 | |
| pvp_win_15 | Воїн | :shield: | Виграв 15 дуелей | 150 | Воїн |
| pvp_win_30 | Чемпіон | :medal_sports: | Виграв 30 дуелей | 500 | Чемпіон |
| pvp_streak_3 | Серія | :fire: | Виграв 3 дуелі поспіль | 100 | |
| pvp_steal_5 | Злодій | :ninja: | Вкрав 5 карток | 100 | |
| pvp_steal_15 | Майстер крадій | :supervillain: | Вкрав 15 карток | 300 | Злодій |
| pvp_rob_1k | Грабіжник | :bandit: | Пограбував 1,000 монет загалом | 200 | Грабіжник |

### Gambling (9)
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| gam_slots_10 | Новачок казино | :slot_machine: | Зіграв 10 разів у слоти | 25 | |
| gam_slots_50 | Завсідник | :game_die: | Зіграв 50 разів у слоти | 100 | |
| gam_slots_200 | Залежний | :cyclone: | Зіграв 200 разів у слоти | 300 | Гемблер |
| gam_big_win | Великий куш | :boom: | Виграв 500+ монет за один спін | 200 | |
| gam_jackpot | ДЖЕКПОТ | :gem::gem::gem: | Вибив 3 діаманти | 500 | Джекпот |
| gam_bj_10 | Картяр | :spades: | Зіграв 10 разів у блекджек | 50 | |
| gam_bj_50 | Шулер | :clubs: | Зіграв 50 разів у блекджек | 200 | Шулер |
| gam_bj_blackjack_5 | Натурал | :black_joker: | Отримав Blackjack 5 разів | 300 | |
| gam_slots_streak_5 | Фартовий | :four_leaf_clover: | Виграв 5 слотів поспіль | 200 | Фартовий |

### Social (8)
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| soc_roast_10 | Тролер | :smiling_imp: | Підколов 10 людей | 50 | |
| soc_roast_50 | Токсик | :skull: | Підколов 50 людей | 200 | Токсик |
| soc_roasted_25 | Жертва | :clown_face: | Був підколотий 25 разів | 100 | Жертва |
| soc_quote_5 | Цитатник | :speech_balloon: | Додав 5 цитат | 50 | |
| soc_quote_20 | Архіваріус | :scroll: | Додав 20 цитат | 200 | Архіваріус |
| soc_gift_3 | Дарувальник | :gift_heart: | Подарував 3 картки | 50 | |
| soc_gift_10 | Санта | :santa: | Подарував 10 карток | 200 | Санта |
| soc_compliment_20 | Душа компанії | :revolving_hearts: | Зробив 20 компліментів | 100 | Душка |

### Secret (7) — hidden until unlocked
| ID | Name | Emoji | Description | Reward | Title |
|----|------|-------|-------------|--------|-------|
| sec_broke | Банкрут | :chart_with_downwards_trend: | Мав 0 монет | 50 | Банкрут |
| sec_robbed_10 | Магніт для злодіїв | :magnet: | Був пограбований 10 разів | 100 | |
| sec_burn_legendary | Божевілля | :brain: | Спалив легендарну картку | 200 | Божевільний |
| sec_5_copies | Дублікатор | :repeat: | Мав 5 копій однієї картки | 100 | |
| sec_lose_5 | Невдаха | :thumbsdown: | Програв 5 дуелей поспіль | 100 | Невдаха |
| sec_3am | Нічна зміна | :owl: | Грав о 3 ночі за Києвом | 100 | Сова |
| sec_wordle_1 | Геній | :brain: | Вгадав wordle з першої спроби | 200 | Геній |

## Tracking Logic

### Stat Increments in Existing Handlers
Each handler that affects a stat calls `b.db.IncrementStat(userID, "field_name", amount)`. After the increment, call `b.checkAchievements(c, userID)`.

Handlers to modify:
- `slots.go` — slots_played, slots_won, slots_max_win, slots_streak
- `blackjack.go` — bj_played, bj_won, bj_blackjacks
- `quotes.go` — roasts_given, roasts_received, compliments_given, quotes_added
- `cardgames.go` — cards_stolen, cards_gifted, cards_burned, duel_streak, lose_streak
- `duel.go` — duels_won, duels_lost, duel_streak, lose_streak
- `war.go` — duels_won, duels_lost
- `rob.go` — coins_robbed
- `cards.go` — packs_opened
- `wordle.go` — wordle_played
- `slots.go` (handleDaily) — daily_claimed, max_balance, total_earned

### Balance Tracking
After every `UpdateBalance` call that adds coins: increment `total_earned`.
After every `UpdateBalance` call that removes coins: increment `total_spent`.
After every balance change: update `max_balance` if current > max.

### Achievement Check
`checkAchievements(c, userID)` runs after stat changes:
1. Load user stats from `user_stats`
2. Load collection stats (unique count, rarity counts)
3. For each achievement not yet unlocked by this user:
   - Run `Check(stats, collectionStats)`
   - If true: insert into `achievements`, award coins, send notification
4. Only checks achievements relevant to the category that changed (optimization)

## Collection Page Additions

### Stats Panel (between header and card grid)
Four columns on desktop, stacked on mobile:

**Economy**
- Total earned / Total spent
- Max balance
- Packs opened

**Cards**
- Cards burned / Cards gifted
- Cards stolen

**PvP**
- Duels: W/L (win rate%)
- Best streak

**Casino**
- Slots played / BJ played
- Biggest slot win

### Achievements Section (after stats, before cards)
- Progress bar: "23/50 achievements"
- Grid of earned achievement badges (emoji + name)
- Secret achievements show as "??? — ???" with lock emoji until unlocked
- Active title shown in header next to username: "Dmytro | Чемпіон"

## Commands

### `/achievements`
Shows in Telegram:
```
🏆 Досягнення (23/50)

✅ 🃏 Колекціонер — Зібрав 10 карток
✅ 💰 Перша тисяча — Заробив 1,000 монет
...

🔒 Наступні:
📦 Шопоголік — Відкрив 50 паків (32/50)
⚔️ Воїн — Виграв 15 дуелей (11/15)

🌐 https://fuck-work-bot.fly.dev/collection/{userID}
```
Shows top 5 completed + top 3 closest to completing. Link to web page for full list.

### `/title <name>`
Set active title from earned achievement titles.
`/title` with no args shows available titles.
`/title off` removes active title.

## Notification
When achievement unlocked:
```
🏆 Досягнення розблоковано!
{emoji} {name}
{description}
+{reward} 🪙

{if title: 'Новий титул доступний: "{title}" — /title {title}'}
```

## Files

### New
- `go-bot/internal/handlers/achievements.go` — achievement definitions, check logic, `/achievements` and `/title` handlers

### Modified
- `go-bot/internal/storage/sqlite.go` — new tables in migrate(), IncrementStat(), GetUserStats(), achievement CRUD, title CRUD
- `go-bot/internal/handlers/handlers.go` — register `/achievements` and `/title`
- `go-bot/internal/handlers/web.go` — stats panel + achievements section on collection page
- `go-bot/internal/handlers/slots.go` — increment stats
- `go-bot/internal/handlers/blackjack.go` — increment stats
- `go-bot/internal/handlers/quotes.go` — increment stats
- `go-bot/internal/handlers/cardgames.go` — increment stats
- `go-bot/internal/handlers/duel.go` — increment stats
- `go-bot/internal/handlers/war.go` — increment stats
- `go-bot/internal/handlers/rob.go` — increment stats
- `go-bot/internal/handlers/cards.go` — increment stats
- `go-bot/internal/handlers/wordle.go` — increment stats
