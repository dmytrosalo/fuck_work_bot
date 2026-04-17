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
