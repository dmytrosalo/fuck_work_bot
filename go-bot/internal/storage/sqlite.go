package storage

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type UserStats struct {
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
		`CREATE TABLE IF NOT EXISTS pack_opens (user_id TEXT NOT NULL, date TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(user_id, date))`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
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

func (d *DB) GetAllStats() ([]UserStats, error) {
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

func (d *DB) GetDailyStats() ([]UserStats, error) {
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

func (d *DB) GetRandomCard(rarity int) (id int, name, emoji, description, specialName string, special int) {
	d.db.QueryRow(`SELECT id, name, emoji, description, special_name, special FROM cards WHERE rarity = ? ORDER BY RANDOM() LIMIT 1`, rarity).
		Scan(&id, &name, &emoji, &description, &specialName, &special)
	return
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

func (d *DB) ClearCards() {
	d.db.Exec(`DELETE FROM cards`)
}

func scanStats(rows *sql.Rows) ([]UserStats, error) {
	var stats []UserStats
	for rows.Next() {
		var s UserStats
		if err := rows.Scan(&s.UserID, &s.Name, &s.Work, &s.Personal); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}
