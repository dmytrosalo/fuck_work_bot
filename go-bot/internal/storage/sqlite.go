package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type ClassifierStats struct {
	UserID   string
	Name     string
	Work     int
	Personal int
}

type DB struct {
	db *sql.DB
}

func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS stats (user_id TEXT PRIMARY KEY, name TEXT NOT NULL, work INTEGER NOT NULL DEFAULT 0, personal INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS daily_stats (user_id TEXT PRIMARY KEY, name TEXT NOT NULL, work INTEGER NOT NULL DEFAULT 0, personal INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS muted (user_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS chats (chat_id TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS feedback (id INTEGER PRIMARY KEY AUTOINCREMENT, text TEXT NOT NULL, label TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS roasts (id INTEGER PRIMARY KEY AUTOINCREMENT, category TEXT NOT NULL, target TEXT NOT NULL DEFAULT '', text TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS compliments (id INTEGER PRIMARY KEY AUTOINCREMENT, target TEXT NOT NULL DEFAULT '', text TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS quotes (id INTEGER PRIMARY KEY AUTOINCREMENT, author TEXT NOT NULL, text TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cards (id INTEGER PRIMARY KEY, name TEXT NOT NULL, rarity INTEGER NOT NULL, category TEXT NOT NULL, emoji TEXT NOT NULL, description TEXT NOT NULL, atk INTEGER, def INTEGER, special_name TEXT, special INTEGER)`,
		`CREATE TABLE IF NOT EXISTS collection (user_id TEXT NOT NULL, card_id INTEGER NOT NULL, count INTEGER NOT NULL DEFAULT 1, PRIMARY KEY(user_id, card_id))`,
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS transactions (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, name TEXT NOT NULL, activity TEXT NOT NULL, amount INTEGER NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS balances (user_id TEXT PRIMARY KEY, name TEXT NOT NULL, coins INTEGER NOT NULL DEFAULT 100)`,
		`CREATE TABLE IF NOT EXISTS slot_spins (user_id TEXT NOT NULL, date TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(user_id, date))`,
		`CREATE TABLE IF NOT EXISTS pack_opens (user_id TEXT NOT NULL, date TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(user_id, date))`,
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
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	// Add stage column to collection (migration for existing DBs)
	db.Exec(`ALTER TABLE collection ADD COLUMN stage INTEGER NOT NULL DEFAULT 1`)

	// Card ideas table
	db.Exec(`CREATE TABLE IF NOT EXISTS card_ideas (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id TEXT NOT NULL, name TEXT NOT NULL, idea TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)

	return nil
}

func (d *DB) SaveCardIdea(userID, name, idea string) {
	d.db.Exec(`INSERT INTO card_ideas (user_id, name, idea) VALUES (?, ?, ?)`, userID, name, idea)
}

type CardIdea struct {
	Name      string
	Idea      string
	CreatedAt string
}

func (d *DB) GetCardIdeas() []CardIdea {
	rows, err := d.db.Query(`SELECT name, idea, created_at FROM card_ideas ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ideas []CardIdea
	for rows.Next() {
		var ci CardIdea
		if err := rows.Scan(&ci.Name, &ci.Idea, &ci.CreatedAt); err == nil {
			ideas = append(ideas, ci)
		}
	}
	return ideas
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) UpdateStats(userID, name string, isWork bool) {
	if isWork {
		d.db.Exec(
			`INSERT INTO stats (user_id, name, work, personal) VALUES (?, ?, 1, 0) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name, work=work+1`,
			userID, name,
		)
	} else {
		d.db.Exec(
			`INSERT INTO stats (user_id, name, work, personal) VALUES (?, ?, 0, 1) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name, personal=personal+1`,
			userID, name,
		)
	}
}

func (d *DB) GetAllStats() ([]ClassifierStats, error) {
	rows, err := d.db.Query(`SELECT user_id, name, work, personal FROM stats ORDER BY (work + personal) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

func (d *DB) UpdateDailyStats(userID, name string, isWork bool) {
	if isWork {
		d.db.Exec(
			`INSERT INTO daily_stats (user_id, name, work, personal) VALUES (?, ?, 1, 0) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name, work=work+1`,
			userID, name,
		)
	} else {
		d.db.Exec(
			`INSERT INTO daily_stats (user_id, name, work, personal) VALUES (?, ?, 0, 1) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name, personal=personal+1`,
			userID, name,
		)
	}
}

func (d *DB) GetDailyStats() ([]ClassifierStats, error) {
	rows, err := d.db.Query(`SELECT user_id, name, work, personal FROM daily_stats ORDER BY (work + personal) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStats(rows)
}

func (d *DB) ResetDailyStats() {
	d.db.Exec(`DELETE FROM daily_stats`)
}

func (d *DB) Mute(userID string) {
	d.db.Exec(`INSERT OR IGNORE INTO muted (user_id) VALUES (?)`, userID)
}

func (d *DB) Unmute(userID string) {
	d.db.Exec(`DELETE FROM muted WHERE user_id = ?`, userID)
}

func (d *DB) IsMuted(userID string) bool {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM muted WHERE user_id = ?`, userID).Scan(&count)
	return count > 0
}

func (d *DB) TrackChat(chatID string) {
	d.db.Exec(`INSERT OR IGNORE INTO chats (chat_id) VALUES (?)`, chatID)
}

func (d *DB) GetActiveChats() ([]string, error) {
	rows, err := d.db.Query(`SELECT chat_id FROM chats`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		chats = append(chats, id)
	}
	return chats, rows.Err()
}

func (d *DB) SaveFeedback(text, label string) {
	d.db.Exec(`INSERT INTO feedback (text, label) VALUES (?, ?)`, text, label)
}

type Feedback struct {
	Text  string
	Label string
}

func (d *DB) GetAllFeedback() ([]Feedback, error) {
	rows, err := d.db.Query(`SELECT text, label FROM feedback`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fb []Feedback
	for rows.Next() {
		var f Feedback
		if err := rows.Scan(&f.Text, &f.Label); err != nil {
			return nil, err
		}
		fb = append(fb, f)
	}
	return fb, rows.Err()
}

// --- Roasts ---

func (d *DB) GetRandomRoast(target string) string {
	var text string
	// Try personal roast first
	if target != "" {
		err := d.db.QueryRow(`SELECT text FROM roasts WHERE target = ? ORDER BY RANDOM() LIMIT 1`, target).Scan(&text)
		if err == nil {
			return text
		}
	}
	// Fall back to generic work roast
	d.db.QueryRow(`SELECT text FROM roasts WHERE category = 'work' AND target = '' ORDER BY RANDOM() LIMIT 1`).Scan(&text)
	return text
}

func (d *DB) GetRandomPersonalRoast(target string) string {
	var text string
	d.db.QueryRow(`SELECT text FROM roasts WHERE target = ? ORDER BY RANDOM() LIMIT 1`, target).Scan(&text)
	return text
}

func (d *DB) AddRoast(category, target, text string) {
	d.db.Exec(`INSERT INTO roasts (category, target, text) VALUES (?, ?, ?)`, category, target, text)
}

// --- Compliments ---

func (d *DB) GetRandomCompliment(target string) string {
	var text string
	if target != "" {
		err := d.db.QueryRow(`SELECT text FROM compliments WHERE target = ? ORDER BY RANDOM() LIMIT 1`, target).Scan(&text)
		if err == nil {
			return text
		}
	}
	d.db.QueryRow(`SELECT text FROM compliments WHERE target = '' ORDER BY RANDOM() LIMIT 1`).Scan(&text)
	return text
}

func (d *DB) AddCompliment(target, text string) {
	d.db.Exec(`INSERT INTO compliments (target, text) VALUES (?, ?)`, target, text)
}

// --- Quotes ---

func (d *DB) GetRandomQuote() (author, text string) {
	d.db.QueryRow(`SELECT author, text FROM quotes ORDER BY RANDOM() LIMIT 1`).Scan(&author, &text)
	return
}

func (d *DB) AddQuote(author, text string) {
	d.db.Exec(`INSERT INTO quotes (author, text) VALUES (?, ?)`, author, text)
}

func (d *DB) HasContent() bool {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM roasts`).Scan(&count)
	return count > 0
}

func (d *DB) QuoteCount() int {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM quotes`).Scan(&count)
	return count
}

func (d *DB) ClearQuotes() {
	d.db.Exec(`DELETE FROM quotes`)
}

// --- Cards ---

func (d *DB) AddCard(id int, name string, rarity int, category, emoji, description string, atk, def int, specialName string, special int) {
	d.db.Exec(`INSERT OR IGNORE INTO cards (id, name, rarity, category, emoji, description, atk, def, special_name, special) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, name, rarity, category, emoji, description, atk, def, specialName, special)
}

type FullCard struct {
	ID          int
	Name        string
	Rarity      int
	Category    string
	Emoji       string
	Description string
	ATK         int
	DEF         int
	SpecialName string
	Special     int
	Count       int
	Stage       int
}

func (d *DB) GetRandomCard(rarity int) FullCard {
	var c FullCard
	c.Rarity = rarity
	d.db.QueryRow(`SELECT id, name, emoji, description, atk, def, special_name, special FROM cards WHERE rarity = ? ORDER BY RANDOM() LIMIT 1`, rarity).
		Scan(&c.ID, &c.Name, &c.Emoji, &c.Description, &c.ATK, &c.DEF, &c.SpecialName, &c.Special)
	return c
}

func (d *DB) AddToCollection(userID string, cardID int) {
	d.db.Exec(`INSERT INTO collection (user_id, card_id, count) VALUES (?, ?, 1) ON CONFLICT(user_id, card_id) DO UPDATE SET count = count + 1`, userID, cardID)
}

func (d *DB) GetCollectionStats(userID string) (unique, total int) {
	d.db.QueryRow(`SELECT COUNT(*) FROM collection WHERE user_id = ?`, userID).Scan(&unique)
	d.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&total)
	return
}

type CollectionCard struct {
	Emoji string
	Name  string
	Count int
}

func (d *DB) GetCollectionByRarity(userID string, rarity int) []CollectionCard {
	rows, err := d.db.Query(`SELECT c.emoji, c.name, col.count FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? AND c.rarity = ? ORDER BY c.name`, userID, rarity)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cards []CollectionCard
	for rows.Next() {
		var card CollectionCard
		if err := rows.Scan(&card.Emoji, &card.Name, &card.Count); err != nil {
			continue
		}
		cards = append(cards, card)
	}
	return cards
}

func (d *DB) GetFullCollection(userID string) []FullCard {
	rows, err := d.db.Query(`SELECT c.id, c.name, c.rarity, c.category, c.emoji, c.description, c.atk, c.def, c.special_name, c.special, col.count, col.stage
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? ORDER BY c.rarity DESC, c.name`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cards []FullCard
	for rows.Next() {
		var card FullCard
		if err := rows.Scan(&card.ID, &card.Name, &card.Rarity, &card.Category, &card.Emoji, &card.Description, &card.ATK, &card.DEF, &card.SpecialName, &card.Special, &card.Count, &card.Stage); err != nil {
			continue
		}
		cards = append(cards, card)
	}
	return cards
}

func (d *DB) GetUserName(userID string) string {
	var name string
	d.db.QueryRow(`SELECT name FROM balances WHERE user_id = ? LIMIT 1`, userID).Scan(&name)
	return name
}

func (d *DB) GetRarityCounts(userID string) map[int]int {
	counts := make(map[int]int)
	rows, err := d.db.Query(`SELECT c.rarity, SUM(col.count) FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? GROUP BY c.rarity`, userID)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var rarity, count int
		if err := rows.Scan(&rarity, &count); err != nil {
			continue
		}
		counts[rarity] = count
	}
	return counts
}

type BattleCard struct {
	ID          int
	Name        string
	Rarity      int
	Emoji       string
	ATK         int
	DEF         int
	SpecialName string
	Special     int
}

func (d *DB) GetRandomCollectionCard(userID string) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT c.id, c.name, c.rarity, c.emoji, c.atk, c.def, c.special_name, c.special
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? ORDER BY RANDOM() LIMIT 1`, userID).
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

// GetStealableCard returns a random card that is NOT stage 2 (stage 2 cards can't be stolen)
func (d *DB) GetStealableCard(userID string) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT c.id, c.name, c.rarity, c.emoji, c.atk, c.def, c.special_name, c.special
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? AND col.stage < 2 ORDER BY RANDOM() LIMIT 1`, userID).
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

func (d *DB) GetCardStage(userID string, cardID int) int {
	var stage int
	d.db.QueryRow(`SELECT stage FROM collection WHERE user_id = ? AND card_id = ?`, userID, cardID).Scan(&stage)
	return stage
}

func (d *DB) EvolveCard(userID string, cardID int) bool {
	var count, stage int
	d.db.QueryRow(`SELECT count, stage FROM collection WHERE user_id = ? AND card_id = ?`, userID, cardID).Scan(&count, &stage)
	if count < 2 || stage >= 2 {
		return false
	}
	d.db.Exec(`UPDATE collection SET count = count - 1, stage = 2 WHERE user_id = ? AND card_id = ?`, userID, cardID)
	return true
}

func (d *DB) FindUserByName(name string) (userID string, found bool) {
	err := d.db.QueryRow(`SELECT user_id FROM balances WHERE name = ? LIMIT 1`, name).Scan(&userID)
	return userID, err == nil
}

func (d *DB) FindCardByName(name string) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT id, name, rarity, emoji, atk, def, special_name, special FROM cards WHERE LOWER(name) LIKE LOWER(?) LIMIT 1`, "%"+name+"%").
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

func (d *DB) FindCardByID(id int) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT id, name, rarity, emoji, atk, def, special_name, special FROM cards WHERE id = ?`, id).
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

func (d *DB) RemoveFromCollection(userID string, cardID int) bool {
	var count int
	d.db.QueryRow(`SELECT count FROM collection WHERE user_id = ? AND card_id = ?`, userID, cardID).Scan(&count)
	if count <= 0 {
		return false
	}
	if count == 1 {
		d.db.Exec(`DELETE FROM collection WHERE user_id = ? AND card_id = ?`, userID, cardID)
	} else {
		d.db.Exec(`UPDATE collection SET count = count - 1 WHERE user_id = ? AND card_id = ?`, userID, cardID)
	}
	return true
}

func (d *DB) RemoveCardsByRarity(userID string, rarity, count int) int {
	rows, err := d.db.Query(`SELECT col.card_id, col.count FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? AND c.rarity = ? LIMIT ?`, userID, rarity, count)
	if err != nil {
		return 0
	}
	defer rows.Close()

	removed := 0
	for rows.Next() && removed < count {
		var cardID, cnt int
		rows.Scan(&cardID, &cnt)
		if cnt <= 1 {
			d.db.Exec(`DELETE FROM collection WHERE user_id = ? AND card_id = ?`, userID, cardID)
		} else {
			d.db.Exec(`UPDATE collection SET count = count - 1 WHERE user_id = ? AND card_id = ?`, userID, cardID)
		}
		removed++
	}
	return removed
}

func (d *DB) GetRarestCard(userID string) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT c.id, c.name, c.rarity, c.emoji, c.atk, c.def, c.special_name, c.special
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? ORDER BY c.rarity DESC, (c.atk+c.def+c.special) DESC LIMIT 1`, userID).
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

func (d *DB) GetSpecificCollectionCard(userID string, cardID int) BattleCard {
	var card BattleCard
	d.db.QueryRow(`SELECT c.id, c.name, c.rarity, c.emoji, c.atk, c.def, c.special_name, c.special
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? AND c.id = ? LIMIT 1`, userID, cardID).
		Scan(&card.ID, &card.Name, &card.Rarity, &card.Emoji, &card.ATK, &card.DEF, &card.SpecialName, &card.Special)
	return card
}

func (d *DB) CardCount() int {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM cards`).Scan(&count)
	return count
}

func (d *DB) GetMeta(key string) string {
	var val string
	d.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
	return val
}

func (d *DB) SetMeta(key, value string) {
	d.db.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
}

func (d *DB) GetPackOpensToday(userID, date string) int {
	var count int
	d.db.QueryRow(`SELECT count FROM pack_opens WHERE user_id = ? AND date = ?`, userID, date).Scan(&count)
	return count
}

func (d *DB) IncrementPackOpens(userID, date string) {
	d.db.Exec(`INSERT INTO pack_opens (user_id, date, count) VALUES (?, ?, 1) ON CONFLICT(user_id, date) DO UPDATE SET count = count + 1`, userID, date)
}

// --- Balances ---

func (d *DB) GetBalance(userID, name string) int {
	var coins int
	err := d.db.QueryRow(`SELECT coins FROM balances WHERE user_id = ?`, userID).Scan(&coins)
	if err != nil {
		// Create with starting balance
		d.db.Exec(`INSERT OR IGNORE INTO balances (user_id, name, coins) VALUES (?, ?, 100)`, userID, name)
		return 100
	}
	if name != "" {
		d.db.Exec(`UPDATE balances SET name = ? WHERE user_id = ?`, name, userID)
	}
	return coins
}

func (d *DB) UpdateBalance(userID, name string, amount int) int {
	d.db.Exec(`INSERT INTO balances (user_id, name, coins) VALUES (?, ?, 100 + ?) ON CONFLICT(user_id) DO UPDATE SET coins = coins + ?, name = CASE WHEN ? != '' THEN ? ELSE name END`,
		userID, name, amount, amount, name, name)
	return d.GetBalance(userID, "")
}

type BalanceEntry struct {
	UserID string
	Name   string
	Coins  int
}

func (d *DB) GetTopBalances(limit int) []BalanceEntry {
	rows, err := d.db.Query(`SELECT user_id, name, coins FROM balances ORDER BY coins DESC LIMIT ?`, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []BalanceEntry
	for rows.Next() {
		var e BalanceEntry
		if err := rows.Scan(&e.UserID, &e.Name, &e.Coins); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries
}

// --- Slot Spins ---

func (d *DB) GetSlotSpinsToday(userID, date string) int {
	var count int
	d.db.QueryRow(`SELECT count FROM slot_spins WHERE user_id = ? AND date = ?`, userID, date).Scan(&count)
	return count
}

func (d *DB) IncrementSlotSpins(userID, date string) {
	d.db.Exec(`INSERT INTO slot_spins (user_id, date, count) VALUES (?, ?, 1) ON CONFLICT(user_id, date) DO UPDATE SET count = count + 1`, userID, date)
}

// --- Transfer between users ---

func (d *DB) TransferCoins(fromID, toID string, amount int) {
	d.UpdateBalance(fromID, "", -amount)
	d.UpdateBalance(toID, "", amount)
}

func (d *DB) TransferCard(fromID, toID string, cardID int) bool {
	// Check if from has this card
	var count int
	d.db.QueryRow(`SELECT count FROM collection WHERE user_id = ? AND card_id = ?`, fromID, cardID).Scan(&count)
	if count <= 0 {
		return false
	}
	if count == 1 {
		d.db.Exec(`DELETE FROM collection WHERE user_id = ? AND card_id = ?`, fromID, cardID)
	} else {
		d.db.Exec(`UPDATE collection SET count = count - 1 WHERE user_id = ? AND card_id = ?`, fromID, cardID)
	}
	d.AddToCollection(toID, cardID)
	return true
}

// --- Transactions ---

func (d *DB) LogTransaction(userID, name, activity string, amount int) {
	d.db.Exec(`INSERT INTO transactions (user_id, name, activity, amount) VALUES (?, ?, ?, ?)`, userID, name, activity, amount)
}

type ActivityStat struct {
	Activity string
	Total    int
	Count    int
}

func (d *DB) GetUserActivityStats(userID, period string) []ActivityStat {
	var where string
	switch period {
	case "today":
		where = " AND DATE(created_at) = DATE('now')"
	case "week":
		where = " AND created_at >= DATETIME('now', '-7 days')"
	default:
		where = ""
	}

	query := fmt.Sprintf(`SELECT activity, SUM(amount), COUNT(*) FROM transactions WHERE user_id = ?%s GROUP BY activity ORDER BY SUM(amount) DESC`, where)
	rows, err := d.db.Query(query, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var stats []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Activity, &s.Total, &s.Count); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats
}

func (d *DB) GetAllActivityStats(period string) []ActivityStat {
	var where string
	switch period {
	case "today":
		where = " WHERE DATE(created_at) = DATE('now')"
	case "week":
		where = " WHERE created_at >= DATETIME('now', '-7 days')"
	default:
		where = ""
	}

	query := fmt.Sprintf(`SELECT activity, SUM(amount), COUNT(*) FROM transactions%s GROUP BY activity ORDER BY SUM(amount) DESC`, where)
	rows, err := d.db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var stats []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Activity, &s.Total, &s.Count); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats
}

func (d *DB) ClearDailyLimits() {
	d.db.Exec(`DELETE FROM meta WHERE key LIKE '%:20__-%'`)
	d.db.Exec(`DELETE FROM pack_opens`)
	d.db.Exec(`DELETE FROM slot_spins`)
}

type UserStats struct {
	DuelsWon         int
	DuelsLost        int
	CardsStolen      int
	CoinsRobbed      int
	SlotsPlayed      int
	SlotsWon         int
	SlotsMaxWin      int
	SlotsStreak      int
	SlotsMaxStreak   int
	BJPlayed         int
	BJWon            int
	BJBlackjacks     int
	RoastsGiven      int
	RoastsReceived   int
	ComplimentsGiven int
	QuotesAdded      int
	CardsGifted      int
	CardsBurned      int
	WordlePlayed     int
	DailyClaimed     int
	MaxBalance       int
	TotalEarned      int
	TotalSpent       int
	PacksOpened      int
	DuelStreak       int
	MaxDuelStreak    int
	LoseStreak       int
	MaxLoseStreak    int
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

func (d *DB) GetTotalCardsByRarity(rarity int) int {
	var count int
	d.db.QueryRow(`SELECT COUNT(*) FROM cards WHERE rarity = ?`, rarity).Scan(&count)
	return count
}

func (d *DB) GetMaxCardCopies(userID string) int {
	var count int
	d.db.QueryRow(`SELECT COALESCE(MAX(count), 0) FROM collection WHERE user_id = ?`, userID).Scan(&count)
	return count
}

func (d *DB) EnsureUser(userID, name string) {
	d.db.Exec(`INSERT OR IGNORE INTO balances (user_id, name, coins) VALUES (?, ?, 100)`, userID, name)
}

func (d *DB) ClearCards() {
	d.db.Exec(`DELETE FROM cards`)
}

func scanStats(rows *sql.Rows) ([]ClassifierStats, error) {
	var stats []ClassifierStats
	for rows.Next() {
		var s ClassifierStats
		if err := rows.Scan(&s.UserID, &s.Name, &s.Work, &s.Personal); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
