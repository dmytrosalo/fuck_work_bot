# Achievements System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 50 achievements with auto-tracking, coin rewards, display titles, stats on collection page.

**Architecture:** New `user_stats` and `achievements` tables track per-user counters and unlocked achievements. Achievement definitions are hardcoded in Go. After each relevant action, the handler increments a stat and calls `checkAchievements` which tests all locked achievements and sends a notification if one unlocks. The collection web page gets a stats panel and achievements section.

**Tech Stack:** Go stdlib, SQLite (modernc.org/sqlite), html/template.

---

### Task 1: Add DB tables and stat methods to storage

**Files:**
- Modify: `go-bot/internal/storage/sqlite.go`

- [ ] **Step 1: Add new tables to migrate()**

Add these 3 CREATE TABLE statements to the `stmts` slice in the `migrate()` function (after the existing `pack_opens` table):

```go
`CREATE TABLE IF NOT EXISTS user_stats (
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
  packs_opened INTEGER NOT NULL DEFAULT 0,
  duel_streak INTEGER NOT NULL DEFAULT 0,
  max_duel_streak INTEGER NOT NULL DEFAULT 0,
  lose_streak INTEGER NOT NULL DEFAULT 0,
  max_lose_streak INTEGER NOT NULL DEFAULT 0
)`,
`CREATE TABLE IF NOT EXISTS achievements (
  user_id TEXT NOT NULL,
  achievement_id TEXT NOT NULL,
  unlocked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY(user_id, achievement_id)
)`,
`CREATE TABLE IF NOT EXISTS user_titles (
  user_id TEXT PRIMARY KEY,
  active_title TEXT NOT NULL DEFAULT ''
)`,
```

- [ ] **Step 2: Add UserStats type and DB methods**

Add after the existing `ClearDailyLimits` function:

```go
type UserStats struct {
	DuelsWon        int
	DuelsLost       int
	CardsStolen     int
	CoinsRobbed     int
	SlotsPlayed     int
	SlotsWon        int
	SlotsMaxWin     int
	SlotsStreak     int
	SlotsMaxStreak  int
	BJPlayed        int
	BJWon           int
	BJBlackjacks    int
	RoastsGiven     int
	RoastsReceived  int
	ComplimentsGiven int
	QuotesAdded     int
	CardsGifted     int
	CardsBurned     int
	WordlePlayed    int
	DailyClaimed    int
	MaxBalance      int
	TotalEarned     int
	TotalSpent      int
	PacksOpened     int
	DuelStreak      int
	MaxDuelStreak   int
	LoseStreak      int
	MaxLoseStreak   int
}

func (d *DB) GetUserStats(userID string) UserStats {
	var s UserStats
	d.db.QueryRow(`SELECT duels_won, duels_lost, cards_stolen, coins_robbed,
		slots_played, slots_won, slots_max_win, slots_streak, slots_max_streak,
		bj_played, bj_won, bj_blackjacks,
		roasts_given, roasts_received, compliments_given, quotes_added,
		cards_gifted, cards_burned, wordle_played, daily_claimed,
		max_balance, total_earned, total_spent, packs_opened,
		duel_streak, max_duel_streak, lose_streak, max_lose_streak
		FROM user_stats WHERE user_id = ?`, userID).Scan(
		&s.DuelsWon, &s.DuelsLost, &s.CardsStolen, &s.CoinsRobbed,
		&s.SlotsPlayed, &s.SlotsWon, &s.SlotsMaxWin, &s.SlotsStreak, &s.SlotsMaxStreak,
		&s.BJPlayed, &s.BJWon, &s.BJBlackjacks,
		&s.RoastsGiven, &s.RoastsReceived, &s.ComplimentsGiven, &s.QuotesAdded,
		&s.CardsGifted, &s.CardsBurned, &s.WordlePlayed, &s.DailyClaimed,
		&s.MaxBalance, &s.TotalEarned, &s.TotalSpent, &s.PacksOpened,
		&s.DuelStreak, &s.MaxDuelStreak, &s.LoseStreak, &s.MaxLoseStreak)
	return s
}

func (d *DB) IncrementStat(userID, field string, amount int) {
	d.db.Exec(`INSERT INTO user_stats (user_id) VALUES (?) ON CONFLICT(user_id) DO NOTHING`, userID)
	d.db.Exec(fmt.Sprintf(`UPDATE user_stats SET %s = %s + ? WHERE user_id = ?`, field, field), amount, userID)
}

func (d *DB) SetStatMax(userID, field string, value int) {
	d.db.Exec(`INSERT INTO user_stats (user_id) VALUES (?) ON CONFLICT(user_id) DO NOTHING`, userID)
	d.db.Exec(fmt.Sprintf(`UPDATE user_stats SET %s = MAX(%s, ?) WHERE user_id = ?`, field, field), value, userID)
}

func (d *DB) SetStat(userID, field string, value int) {
	d.db.Exec(`INSERT INTO user_stats (user_id) VALUES (?) ON CONFLICT(user_id) DO NOTHING`, userID)
	d.db.Exec(fmt.Sprintf(`UPDATE user_stats SET %s = ? WHERE user_id = ?`, field), value, userID)
}

func (d *DB) GetUnlockedAchievements(userID string) []string {
	rows, err := d.db.Query(`SELECT achievement_id FROM achievements WHERE user_id = ?`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (d *DB) UnlockAchievement(userID, achievementID string) bool {
	res, err := d.db.Exec(`INSERT OR IGNORE INTO achievements (user_id, achievement_id) VALUES (?, ?)`, userID, achievementID)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

func (d *DB) GetActiveTitle(userID string) string {
	var title string
	d.db.QueryRow(`SELECT active_title FROM user_titles WHERE user_id = ?`, userID).Scan(&title)
	return title
}

func (d *DB) SetActiveTitle(userID, title string) {
	d.db.Exec(`INSERT INTO user_titles (user_id, active_title) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET active_title = excluded.active_title`, userID, title)
}

func (d *DB) CountAchievements(userID string) int {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM achievements WHERE user_id = ?`, userID).Scan(&count)
	return count
}

// GetRarityCardCounts returns count of unique cards per rarity for a user
func (d *DB) GetRarityCardCounts(userID string) map[int]int {
	counts := make(map[int]int)
	rows, err := d.db.Query(`SELECT c.rarity, COUNT(DISTINCT c.id) FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? GROUP BY c.rarity`, userID)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var rarity, count int
		if err := rows.Scan(&rarity, &count); err == nil {
			counts[rarity] = count
		}
	}
	return counts
}

// GetTotalCardsByRarity returns total number of distinct cards per rarity (in the game)
func (d *DB) GetTotalCardsByRarity(rarity int) int {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE rarity = ?`, rarity).Scan(&count)
	return count
}

// GetMaxCardCopies returns the max count of any single card for a user
func (d *DB) GetMaxCardCopies(userID string) int {
	var count int
	d.db.QueryRow(`SELECT COALESCE(MAX(count), 0) FROM collection WHERE user_id = ?`, userID).Scan(&count)
	return count
}
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./internal/storage/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add go-bot/internal/storage/sqlite.go
git commit -m "feat: add user_stats, achievements, user_titles tables and methods"
```

---

### Task 2: Create achievement definitions and check logic

**Files:**
- Create: `go-bot/internal/handlers/achievements.go`

- [ ] **Step 1: Create achievements.go with all 50 achievement definitions and check logic**

Create `go-bot/internal/handlers/achievements.go` with the following content. This file contains:
- Achievement struct type
- All 50 achievement definitions
- `checkAchievements` method that tests unlocked state and sends notifications
- `handleAchievements` command handler
- `handleTitle` command handler

```go
package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

type achievement struct {
	ID          string
	Name        string
	Emoji       string
	Description string
	Category    string
	Reward      int
	Title       string
	Hidden      bool
	Check       func(s storage.UserStats, unique int, rarityCounts map[int]int, maxCopies int) bool
}

var allAchievements = []achievement{
	// === Collection (10) ===
	{"col_5", "Початківець", "🗃", "Зібрав 5 карток", "collection", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 5 }},
	{"col_10", "Колекціонер", "🎴", "Зібрав 10 карток", "collection", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 10 }},
	{"col_25", "Знавець", "📚", "Зібрав 25 карток", "collection", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 25 }},
	{"col_50", "Справжній збирач", "🏆", "Зібрав 50 карток", "collection", 200, "Збирач", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 50 }},
	{"col_100", "Картковий магнат", "👑", "Зібрав 100 карток", "collection", 500, "Магнат", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 100 }},
	{"col_200", "Легенда колекцій", "🌟", "Зібрав 200 карток", "collection", 1000, "Легенда", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 200 }},
	{"col_all_rarities", "Райдуга", "🌈", "Маєш картку кожної рідкості", "collection", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool {
			for i := 1; i <= 6; i++ {
				if r[i] == 0 {
					return false
				}
			}
			return true
		}},
	{"col_10_legendary", "Золота жила", "✨", "Маєш 10+ легендарних карток", "collection", 500, "Золотий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return r[5]+r[6] >= 10 }},
	{"col_full_common", "Сірий кардинал", "⚫", "Зібрав всі Common картки", "collection", 300, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked separately
	{"col_full_rare", "Синій барон", "💎", "Зібрав всі Rare картки", "collection", 500, "Барон", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked separately

	// === Economy (8) ===
	{"eco_1k", "Перша тисяча", "💰", "Заробив 1,000 монет загалом", "economy", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 1000 }},
	{"eco_5k", "Бізнесмен", "📈", "Заробив 5,000 монет загалом", "economy", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 5000 }},
	{"eco_25k", "Олігарх", "💎", "Заробив 25,000 монет загалом", "economy", 500, "Олігарх", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 25000 }},
	{"eco_spend_10k", "Транжира", "💸", "Витратив 10,000 монет загалом", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalSpent >= 10000 }},
	{"eco_rich", "Багатій", "💵", "Мав 1,000+ монет одночасно", "economy", 100, "Багатій", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxBalance >= 1000 }},
	{"eco_daily_30", "Стабільність", "📅", "Забрав /daily 30 разів", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DailyClaimed >= 30 }},
	{"eco_generous", "Щедра душа", "🎁", "Подарував 10+ карток", "economy", 150, "Щедрий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 10 }},
	{"eco_packs_50", "Шопоголік", "📦", "Відкрив 50 паків", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.PacksOpened >= 50 }},

	// === PvP (8) ===
	{"pvp_win_1", "Перша перемога", "⚔️", "Виграв першу дуель", "pvp", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 1 }},
	{"pvp_win_5", "Боєць", "🥊", "Виграв 5 дуелей", "pvp", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 5 }},
	{"pvp_win_15", "Воїн", "🛡", "Виграв 15 дуелей", "pvp", 150, "Воїн", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 15 }},
	{"pvp_win_30", "Чемпіон", "🏅", "Виграв 30 дуелей", "pvp", 500, "Чемпіон", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 30 }},
	{"pvp_streak_3", "Серія", "🔥", "Виграв 3 дуелі поспіль", "pvp", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxDuelStreak >= 3 }},
	{"pvp_steal_5", "Злодій", "🥷", "Вкрав 5 карток", "pvp", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsStolen >= 5 }},
	{"pvp_steal_15", "Майстер крадій", "🦹", "Вкрав 15 карток", "pvp", 300, "Злодій", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsStolen >= 15 }},
	{"pvp_rob_1k", "Грабіжник", "💣", "Пограбував 1,000 монет загалом", "pvp", 200, "Грабіжник", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CoinsRobbed >= 1000 }},

	// === Gambling (9) ===
	{"gam_slots_10", "Новачок казино", "🎰", "Зіграв 10 разів у слоти", "gambling", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 10 }},
	{"gam_slots_50", "Завсідник", "🎲", "Зіграв 50 разів у слоти", "gambling", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 50 }},
	{"gam_slots_200", "Залежний", "🌀", "Зіграв 200 разів у слоти", "gambling", 300, "Гемблер", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 200 }},
	{"gam_big_win", "Великий куш", "💥", "Виграв 500+ монет за один спін", "gambling", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxWin >= 500 }},
	{"gam_jackpot", "ДЖЕКПОТ", "💎", "Вибив 3 діаманти", "gambling", 500, "Джекпот", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxWin >= 2500 }}, // jackpot = 50x max bet 500 = 25000, but 2500+ is a good threshold
	{"gam_bj_10", "Картяр", "♠️", "Зіграв 10 разів у блекджек", "gambling", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJPlayed >= 10 }},
	{"gam_bj_50", "Шулер", "♣️", "Зіграв 50 разів у блекджек", "gambling", 200, "Шулер", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJPlayed >= 50 }},
	{"gam_bj_blackjack_5", "Натурал", "🃏", "Отримав Blackjack 5 разів", "gambling", 300, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJBlackjacks >= 5 }},
	{"gam_slots_streak_5", "Фартовий", "🍀", "Виграв 5 слотів поспіль", "gambling", 200, "Фартовий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxStreak >= 5 }},

	// === Social (8) ===
	{"soc_roast_10", "Тролер", "😈", "Підколов 10 людей", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsGiven >= 10 }},
	{"soc_roast_50", "Токсик", "💀", "Підколов 50 людей", "social", 200, "Токсик", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsGiven >= 50 }},
	{"soc_roasted_25", "Жертва", "🤡", "Був підколотий 25 разів", "social", 100, "Жертва", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsReceived >= 25 }},
	{"soc_quote_5", "Цитатник", "💬", "Додав 5 цитат", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.QuotesAdded >= 5 }},
	{"soc_quote_20", "Архіваріус", "📜", "Додав 20 цитат", "social", 200, "Архіваріус", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.QuotesAdded >= 20 }},
	{"soc_gift_3", "Дарувальник", "💝", "Подарував 3 картки", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 3 }},
	{"soc_gift_10", "Санта", "🎅", "Подарував 10 карток", "social", 200, "Санта", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 10 }},
	{"soc_compliment_20", "Душа компанії", "💞", "Зробив 20 компліментів", "social", 100, "Душка", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.ComplimentsGiven >= 20 }},

	// === Secret (7) — Hidden ===
	{"sec_broke", "Банкрут", "📉", "Мав 0 монет", "secret", 50, "Банкрут", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked inline
	{"sec_robbed_10", "Магніт для злодіїв", "🧲", "Був пограбований 10 разів", "secret", 100, "", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // tracked separately
	{"sec_burn_legendary", "Божевілля", "🧠", "Спалив легендарну картку", "secret", 200, "Божевільний", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked inline
	{"sec_5_copies", "Дублікатор", "🔁", "Мав 5 копій однієї картки", "secret", 100, "", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return m >= 5 }},
	{"sec_lose_5", "Невдаха", "👎", "Програв 5 дуелей поспіль", "secret", 100, "Невдаха", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxLoseStreak >= 5 }},
	{"sec_3am", "Нічна зміна", "🦉", "Грав о 3 ночі за Києвом", "secret", 100, "Сова", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked inline
	{"sec_wordle_1", "Геній", "🧠", "Вгадав wordle з першої спроби", "secret", 200, "Геній", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }}, // checked inline
}

// achievementByID for quick lookup
var achievementByID = func() map[string]*achievement {
	m := make(map[string]*achievement)
	for i := range allAchievements {
		m[allAchievements[i].ID] = &allAchievements[i]
	}
	return m
}()

func (b *Bot) checkAchievements(c tele.Context, userID, userName string) {
	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	stats := b.db.GetUserStats(userID)
	unique, _ := b.db.GetCollectionStats(userID)
	rarityCounts := b.db.GetRarityCardCounts(userID)
	maxCopies := b.db.GetMaxCardCopies(userID)

	for i := range allAchievements {
		a := &allAchievements[i]
		if unlockedSet[a.ID] {
			continue
		}

		// Special checks for full-set achievements
		passed := false
		switch a.ID {
		case "col_full_common":
			total := b.db.GetTotalCardsByRarity(1)
			passed = total > 0 && rarityCounts[1] >= total
		case "col_full_rare":
			total := b.db.GetTotalCardsByRarity(3)
			passed = total > 0 && rarityCounts[3] >= total
		default:
			passed = a.Check(stats, unique, rarityCounts, maxCopies)
		}

		if passed {
			if b.db.UnlockAchievement(userID, a.ID) {
				b.db.UpdateBalance(userID, userName, a.Reward)
				msg := fmt.Sprintf("🏆 Досягнення розблоковано!\n%s *%s*\n_%s_\n+%d 🪙", a.Emoji, a.Name, a.Description, a.Reward)
				if a.Title != "" {
					msg += fmt.Sprintf("\n\nНовий титул: *%s* — /title %s", a.Title, a.Title)
				}
				c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
			}
		}
	}
}

// checkAchievementInline for achievements that need to be checked at specific moments (secret ones)
func (b *Bot) unlockSpecial(c tele.Context, userID, userName, achievementID string) {
	a, ok := achievementByID[achievementID]
	if !ok {
		return
	}
	if b.db.UnlockAchievement(userID, a.ID) {
		b.db.UpdateBalance(userID, userName, a.Reward)
		msg := fmt.Sprintf("🏆 Досягнення розблоковано!\n%s *%s*\n_%s_\n+%d 🪙", a.Emoji, a.Name, a.Description, a.Reward)
		if a.Title != "" {
			msg += fmt.Sprintf("\n\nНовий титул: *%s* — /title %s", a.Title, a.Title)
		}
		c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}
}

func (b *Bot) handleAchievements(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)

	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 *Досягнення* (%d/%d)\n\n", len(unlocked), len(allAchievements)))

	// Show unlocked
	count := 0
	for _, a := range allAchievements {
		if unlockedSet[a.ID] {
			sb.WriteString(fmt.Sprintf("✅ %s %s\n", a.Emoji, a.Name))
			count++
		}
	}

	if count == 0 {
		sb.WriteString("_Поки немає досягнень_\n")
	}

	// Show next closest (non-secret, non-unlocked)
	sb.WriteString("\n🔒 *Наступні:*\n")
	shown := 0
	stats := b.db.GetUserStats(userID)
	for _, a := range allAchievements {
		if unlockedSet[a.ID] || a.Hidden {
			continue
		}
		if shown >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%s %s — _%s_\n", a.Emoji, a.Name, a.Description))
		shown++
	}

	// Secret teaser
	secretCount := 0
	for _, a := range allAchievements {
		if a.Hidden && unlockedSet[a.ID] {
			secretCount++
		}
	}
	sb.WriteString(fmt.Sprintf("\n🔮 Секретні: %d/7\n", secretCount))

	sb.WriteString(fmt.Sprintf("\n🌐 https://fuck-work-bot.fly.dev/collection/%s", userID))

	_ = stats // used for future progress display
	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleTitle(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	payload := strings.TrimSpace(c.Message().Payload)

	if payload == "" {
		// Show available titles
		unlocked := b.db.GetUnlockedAchievements(userID)
		unlockedSet := make(map[string]bool)
		for _, id := range unlocked {
			unlockedSet[id] = true
		}

		var titles []string
		for _, a := range allAchievements {
			if a.Title != "" && unlockedSet[a.ID] {
				titles = append(titles, a.Title)
			}
		}

		current := b.db.GetActiveTitle(userID)
		if len(titles) == 0 {
			return c.Reply("У тебе ще немає титулів. Заробляй досягнення!")
		}

		var sb strings.Builder
		sb.WriteString("🏷 *Доступні титули:*\n\n")
		for _, t := range titles {
			if t == current {
				sb.WriteString(fmt.Sprintf("  ▸ *%s* (активний)\n", t))
			} else {
				sb.WriteString(fmt.Sprintf("  ▸ %s\n", t))
			}
		}
		sb.WriteString("\n/title <назва> — встановити\n/title off — прибрати")
		return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	if payload == "off" {
		b.db.SetActiveTitle(userID, "")
		return c.Reply("🏷 Титул прибрано")
	}

	// Verify user has this title
	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	for _, a := range allAchievements {
		if a.Title != "" && unlockedSet[a.ID] && strings.EqualFold(a.Title, payload) {
			b.db.SetActiveTitle(userID, a.Title)
			return c.Reply(fmt.Sprintf("🏷 Титул встановлено: *%s*", a.Title), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		}
	}

	return c.Reply("❌ Титул не знайдено або не розблоковано")
}

// check3AM checks if current Kyiv time is 3:xx AM
func is3AMKyiv() bool {
	kyiv := kyivLocation()
	hour := time.Now().In(kyiv).Hour()
	return hour == 3
}
```

- [ ] **Step 2: Build and verify**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./internal/handlers/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add go-bot/internal/handlers/achievements.go
git commit -m "feat: add 50 achievement definitions with check logic and commands"
```

---

### Task 3: Register achievement commands and wire stat tracking in all handlers

**Files:**
- Modify: `go-bot/internal/handlers/handlers.go` — register `/achievements` and `/title`
- Modify: `go-bot/internal/handlers/slots.go` — track slots_played, slots_won, slots_max_win, slots_streak, daily_claimed, total_earned, total_spent, max_balance
- Modify: `go-bot/internal/handlers/blackjack.go` — track bj_played, bj_won, bj_blackjacks
- Modify: `go-bot/internal/handlers/duel.go` — track duels_won, duels_lost, duel_streak, lose_streak
- Modify: `go-bot/internal/handlers/war.go` — track duels_won, duels_lost
- Modify: `go-bot/internal/handlers/rob.go` — track coins_robbed, sec_robbed_10, sec_broke
- Modify: `go-bot/internal/handlers/cardgames.go` — track cards_stolen, cards_gifted, cards_burned, sec_burn_legendary
- Modify: `go-bot/internal/handlers/cards.go` — track packs_opened
- Modify: `go-bot/internal/handlers/quotes.go` — track roasts_given, roasts_received, compliments_given, quotes_added
- Modify: `go-bot/internal/handlers/wordle.go` — track wordle_played, sec_wordle_1, sec_3am

- [ ] **Step 1: Register commands in handlers.go**

In the `Register` method, add before the `bot.Handle(tele.OnText, ...)` line:

```go
bot.Handle("/achievements", b.handleAchievements)
bot.Handle("/title", b.handleTitle)
```

- [ ] **Step 2: Add stat tracking to slots.go**

In `handleSlots`, after the `b.db.LogTransaction(...)` line (around line 133), add:

```go
b.db.IncrementStat(userID, "slots_played", 1)
if profit > 0 {
    b.db.IncrementStat(userID, "slots_won", 1)
    b.db.SetStatMax(userID, "slots_max_win", winnings)
    b.db.IncrementStat(userID, "total_earned", winnings)
    // Streak
    stats := b.db.GetUserStats(userID)
    b.db.SetStat(userID, "slots_streak", stats.SlotsStreak+1)
    b.db.SetStatMax(userID, "slots_max_streak", stats.SlotsStreak+1)
} else {
    b.db.SetStat(userID, "slots_streak", 0)
}
if profit < 0 {
    b.db.IncrementStat(userID, "total_spent", -profit)
}
bal := b.db.GetBalance(userID, "")
b.db.SetStatMax(userID, "max_balance", bal)
b.checkAchievements(c, userID, userName)
if is3AMKyiv() {
    b.unlockSpecial(c, userID, userName, "sec_3am")
}
```

In `handleDaily`, after the `b.db.LogTransaction(...)` line, add:

```go
b.db.IncrementStat(userID, "daily_claimed", 1)
b.db.IncrementStat(userID, "total_earned", dailyBonus)
b.db.SetStatMax(userID, "max_balance", newBalance)
b.checkAchievements(c, userID, userName)
```

- [ ] **Step 3: Add stat tracking to blackjack.go**

In `bjStand`, after the balance update and log transaction block (around line 272), add:

```go
b.db.IncrementStat(game.UserID, "bj_played", 1)
if winnings > game.Bet {
    b.db.IncrementStat(game.UserID, "bj_won", 1)
    b.db.IncrementStat(game.UserID, "total_earned", winnings)
} else if winnings == 0 {
    b.db.IncrementStat(game.UserID, "total_spent", game.Bet)
}
newBal := b.db.GetBalance(game.UserID, "")
b.db.SetStatMax(game.UserID, "max_balance", newBal)
b.checkAchievements(c, game.UserID, game.UserName)
```

In `handleBlackjack`, where blackjack (21 from 2 cards) is detected and pays 2.5x, add after the payout:

```go
b.db.IncrementStat(userID, "bj_played", 1)
b.db.IncrementStat(userID, "bj_won", 1)
b.db.IncrementStat(userID, "bj_blackjacks", 1)
b.db.IncrementStat(userID, "total_earned", winnings)
b.db.SetStatMax(userID, "max_balance", b.db.GetBalance(userID, ""))
b.checkAchievements(c, userID, userName)
```

- [ ] **Step 4: Add stat tracking to duel.go**

In `resolveDuel`, after the winner is determined (around line 246-256), add after each branch:

For challenger wins (power1 > power2):
```go
b.db.IncrementStat(duel.ChallengerID, "duels_won", 1)
b.db.IncrementStat(duel.OpponentID, "duels_lost", 1)
// Challenger streak
cs := b.db.GetUserStats(duel.ChallengerID)
b.db.SetStat(duel.ChallengerID, "duel_streak", cs.DuelStreak+1)
b.db.SetStatMax(duel.ChallengerID, "max_duel_streak", cs.DuelStreak+1)
b.db.SetStat(duel.ChallengerID, "lose_streak", 0)
// Opponent streak
os := b.db.GetUserStats(duel.OpponentID)
b.db.SetStat(duel.OpponentID, "lose_streak", os.LoseStreak+1)
b.db.SetStatMax(duel.OpponentID, "max_lose_streak", os.LoseStreak+1)
b.db.SetStat(duel.OpponentID, "duel_streak", 0)
```

For opponent wins (power2 > power1): same but swapped.

Note: `resolveDuel` doesn't have access to `tele.Context`. The achievement check must be deferred — add a `checkAchievementsBot` method that takes `*tele.Bot` and `chatID` instead. Or simpler: achievements will be checked on next user action. This is acceptable since duels are followed by other actions.

- [ ] **Step 5: Add stat tracking to rob.go**

In `handleRob`, on success (after `b.db.TransferCoins`), add:

```go
b.db.IncrementStat(userID, "coins_robbed", stolen)
b.db.IncrementStat(userID, "total_earned", stolen)
b.db.IncrementStat(targetID, "total_spent", stolen)
b.checkAchievements(c, userID, userName)
```

On fail, after penalty is applied:

```go
b.db.IncrementStat(userID, "total_spent", penalty)
bal := b.db.GetBalance(userID, "")
if bal <= 0 {
    b.unlockSpecial(c, userID, userName, "sec_broke")
}
b.checkAchievements(c, userID, userName)
```

- [ ] **Step 6: Add stat tracking to cardgames.go**

In `handleSteal` on success:
```go
b.db.IncrementStat(userID, "cards_stolen", 1)
b.checkAchievements(c, userID, userName)
```

In `handleGift` on success:
```go
b.db.IncrementStat(userID, "cards_gifted", 1)
b.checkAchievements(c, userID, userName)
```

In `handleBurn` on success, before the return:
```go
b.db.IncrementStat(userID, "cards_burned", 1)
b.db.IncrementStat(userID, "total_earned", reward)
if card.Rarity >= 5 {
    b.unlockSpecial(c, userID, userName, "sec_burn_legendary")
}
b.checkAchievements(c, userID, userName)
```

- [ ] **Step 7: Add stat tracking to cards.go**

In `handlePack`, after `b.db.IncrementPackOpens(userID, today)`:

```go
b.db.IncrementStat(userID, "packs_opened", 1)
b.db.IncrementStat(userID, "total_spent", packCost)
```

After the pack is fully opened (before return), add:
```go
b.checkAchievements(c, userID, userName)
```

- [ ] **Step 8: Add stat tracking to quotes.go**

In `handleRoast`, after the roast is sent (before return), add:

```go
roasterID := fmt.Sprintf("%d", c.Sender().ID)
b.db.IncrementStat(roasterID, "roasts_given", 1)
// Track target being roasted if reply
if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
    targetID := fmt.Sprintf("%d", c.Message().ReplyTo.Sender.ID)
    b.db.IncrementStat(targetID, "roasts_received", 1)
}
b.checkAchievements(c, roasterID, c.Sender().FirstName)
```

In `handleCompliment`, before return:
```go
senderID := fmt.Sprintf("%d", c.Sender().ID)
b.db.IncrementStat(senderID, "compliments_given", 1)
b.checkAchievements(c, senderID, c.Sender().FirstName)
```

In `handleAddQuote`, after `b.db.AddQuote(author, text)`:
```go
senderID := fmt.Sprintf("%d", c.Sender().ID)
b.db.IncrementStat(senderID, "quotes_added", 1)
b.checkAchievements(c, senderID, c.Sender().FirstName)
```

- [ ] **Step 9: Add stat tracking to wordle.go**

In `checkWordleAnswer`, on win (after reward is given), add:

```go
b.db.IncrementStat(userID, "wordle_played", 1)
b.db.IncrementStat(userID, "total_earned", reward)
if attempt == 1 {
    b.unlockSpecial(c, userID, userName, "sec_wordle_1")
}
if is3AMKyiv() {
    b.unlockSpecial(c, userID, userName, "sec_3am")
}
b.checkAchievements(c, userID, userName)
```

On loss:
```go
b.db.IncrementStat(userID, "wordle_played", 1)
```

- [ ] **Step 10: Build and verify**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./cmd/bot/`
Expected: no errors

- [ ] **Step 11: Run tests**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go test ./...`
Expected: all tests pass

- [ ] **Step 12: Commit**

```bash
git add go-bot/internal/handlers/
git commit -m "feat: wire achievement stat tracking in all handlers"
```

---

### Task 4: Add stats and achievements to collection web page

**Files:**
- Modify: `go-bot/internal/handlers/web.go`

- [ ] **Step 1: Update page data struct and handler**

Add to `collectionPageData`:
```go
Title        string
Stats        storage.UserStats
Achievements []achievementDisplay
AchCount     int
AchTotal     int
```

Add new type:
```go
type achievementDisplay struct {
    Emoji       string
    Name        string
    Description string
    Unlocked    bool
    Hidden      bool
}
```

In `handleCollectionPage`, after existing data gathering, add:
```go
title := db.GetActiveTitle(userID)
stats := db.GetUserStats(userID)
unlockedIDs := db.GetUnlockedAchievements(userID)
unlockedSet := make(map[string]bool)
for _, id := range unlockedIDs {
    unlockedSet[id] = true
}
var achDisplays []achievementDisplay
for _, a := range allAchievements {
    ad := achievementDisplay{
        Emoji:       a.Emoji,
        Name:        a.Name,
        Description: a.Description,
        Unlocked:    unlockedSet[a.ID],
        Hidden:      a.Hidden,
    }
    achDisplays = append(achDisplays, ad)
}
```

Set the new fields on `data`:
```go
data.Title = title
data.Stats = stats
data.Achievements = achDisplays
data.AchCount = len(unlockedIDs)
data.AchTotal = len(allAchievements)
```

- [ ] **Step 2: Update HTML template**

Add stats panel between header and sections. Add achievements section after header, before card grid.

In the header, after `<h1>{{.UserName}}</h1>`, add title display:
```html
{{if .Title}}<div class="title">{{.Title}}</div>{{end}}
```

Add stats panel HTML after the header div:
```html
<div class="stats-panel">
  <div class="stat-group">
    <div class="stat-title">Economy</div>
    <div class="stat-row"><span>Earned</span><span>{{.Stats.TotalEarned}}</span></div>
    <div class="stat-row"><span>Spent</span><span>{{.Stats.TotalSpent}}</span></div>
    <div class="stat-row"><span>Max bal</span><span>{{.Stats.MaxBalance}}</span></div>
    <div class="stat-row"><span>Packs</span><span>{{.Stats.PacksOpened}}</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">PvP</div>
    <div class="stat-row"><span>Duels W/L</span><span>{{.Stats.DuelsWon}}/{{.Stats.DuelsLost}}</span></div>
    <div class="stat-row"><span>Best streak</span><span>{{.Stats.MaxDuelStreak}}</span></div>
    <div class="stat-row"><span>Cards stolen</span><span>{{.Stats.CardsStolen}}</span></div>
    <div class="stat-row"><span>Coins robbed</span><span>{{.Stats.CoinsRobbed}}</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">Casino</div>
    <div class="stat-row"><span>Slots</span><span>{{.Stats.SlotsPlayed}}</span></div>
    <div class="stat-row"><span>Blackjack</span><span>{{.Stats.BJPlayed}}</span></div>
    <div class="stat-row"><span>Best win</span><span>{{.Stats.SlotsMaxWin}}</span></div>
    <div class="stat-row"><span>BJ naturals</span><span>{{.Stats.BJBlackjacks}}</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">Social</div>
    <div class="stat-row"><span>Roasts</span><span>{{.Stats.RoastsGiven}}</span></div>
    <div class="stat-row"><span>Quotes</span><span>{{.Stats.QuotesAdded}}</span></div>
    <div class="stat-row"><span>Gifts</span><span>{{.Stats.CardsGifted}}</span></div>
    <div class="stat-row"><span>Wordle</span><span>{{.Stats.WordlePlayed}}</span></div>
  </div>
</div>
```

Add achievements section:
```html
<div class="ach-section">
  <div class="section-header" style="border-color:#fbbf24; color:#fbbf24">
    🏆 Achievements ({{.AchCount}}/{{.AchTotal}})
  </div>
  <div class="ach-grid">
    {{range .Achievements}}
    {{if .Unlocked}}
    <div class="ach-badge unlocked">
      <span class="ach-emoji">{{.Emoji}}</span>
      <span class="ach-name">{{.Name}}</span>
    </div>
    {{else if .Hidden}}
    <div class="ach-badge locked">
      <span class="ach-emoji">🔒</span>
      <span class="ach-name">???</span>
    </div>
    {{else}}
    <div class="ach-badge locked">
      <span class="ach-emoji">{{.Emoji}}</span>
      <span class="ach-name">{{.Name}}</span>
    </div>
    {{end}}
    {{end}}
  </div>
</div>
```

Add CSS for stats and achievements (add before `</style>`):
```css
.title {
  font-size: 13px;
  font-weight: 600;
  color: #fbbf24;
  margin-bottom: 4px;
}
.stats-panel {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
  margin-bottom: 24px;
}
@media (min-width: 600px) {
  .stats-panel { grid-template-columns: repeat(4, 1fr); }
}
.stat-group {
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 12px;
  padding: 12px;
}
.stat-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: rgba(255,255,255,0.4);
  margin-bottom: 8px;
  letter-spacing: 0.5px;
}
.stat-row {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  padding: 2px 0;
}
.stat-row span:first-child { color: rgba(255,255,255,0.5); }
.stat-row span:last-child { font-weight: 600; }
.ach-section { margin-bottom: 24px; }
.ach-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
@media (min-width: 500px) {
  .ach-grid { grid-template-columns: repeat(5, 1fr); }
}
.ach-badge {
  text-align: center;
  padding: 10px 4px;
  border-radius: 10px;
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
}
.ach-badge.unlocked {
  background: rgba(251,191,36,0.08);
  border-color: rgba(251,191,36,0.2);
}
.ach-badge.locked { opacity: 0.4; }
.ach-emoji { font-size: 24px; display: block; }
.ach-name { font-size: 9px; font-weight: 600; display: block; margin-top: 4px; }
```

- [ ] **Step 3: Build and verify**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./cmd/bot/`
Expected: no errors

- [ ] **Step 4: Run tests**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go test ./...`
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/web.go
git commit -m "feat: add stats panel and achievements to collection web page"
```

---

### Task 5: Final build, test, and verify

- [ ] **Step 1: Full build**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build -o bot ./cmd/bot/`
Expected: binary built

- [ ] **Step 2: Run all tests**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go test ./...`
Expected: all pass

- [ ] **Step 3: Clean up**

Run: `rm /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot/bot`
