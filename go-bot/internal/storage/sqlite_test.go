package storage

import (
	"path/filepath"
	"testing"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := New(path)
	if err != nil {
		t.Fatalf("New(%q): %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestUpdateAndGetStats(t *testing.T) {
	db := newTestDB(t)

	// 2 work + 1 personal for same user
	db.UpdateStats("u1", "Alice", true)
	db.UpdateStats("u1", "Alice", true)
	db.UpdateStats("u1", "Alice", false)

	stats, err := db.GetAllStats()
	if err != nil {
		t.Fatalf("GetAllStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 user, got %d", len(stats))
	}
	if stats[0].UserID != "u1" || stats[0].Name != "Alice" {
		t.Errorf("unexpected user: %+v", stats[0])
	}
	if stats[0].Work != 2 {
		t.Errorf("expected Work=2, got %d", stats[0].Work)
	}
	if stats[0].Personal != 1 {
		t.Errorf("expected Personal=1, got %d", stats[0].Personal)
	}
}

func TestMuteUnmute(t *testing.T) {
	db := newTestDB(t)

	db.Mute("u1")
	if !db.IsMuted("u1") {
		t.Error("expected u1 to be muted")
	}

	db.Unmute("u1")
	if db.IsMuted("u1") {
		t.Error("expected u1 to be unmuted")
	}
}

func TestTrackChat(t *testing.T) {
	db := newTestDB(t)

	db.TrackChat("c1")
	db.TrackChat("c2")
	db.TrackChat("c1") // duplicate

	chats, err := db.GetActiveChats()
	if err != nil {
		t.Fatalf("GetActiveChats: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("expected 2 chats, got %d", len(chats))
	}
}

// --- SettlePoker (poker settlement atomicity, FIX 3) -----------------------

// TestSettlePokerAppliesAllDeltasInOneCall proves a multi-player settlement
// applies every entry: balances move by exactly their delta and a
// transaction row is logged for each, matching what the previous
// per-player UpdateBalance+LogTransaction loop did.
func TestSettlePokerAppliesAllDeltasInOneCall(t *testing.T) {
	db := newTestDB(t)
	db.UpdateBalance("u1", "Alice", 900) // seeds a new row at 100, +900 -> 1000
	db.UpdateBalance("u2", "Bob", 900)

	if err := db.SettlePoker([]PokerDelta{
		{UserID: "u1", Name: "Alice", Amount: 200},
		{UserID: "u2", Name: "Bob", Amount: -200},
	}); err != nil {
		t.Fatalf("SettlePoker: %v", err)
	}

	if got := db.GetBalance("u1", ""); got != 1200 {
		t.Errorf("u1 balance = %d, want 1200", got)
	}
	if got := db.GetBalance("u2", ""); got != 800 {
		t.Errorf("u2 balance = %d, want 800", got)
	}

	var txCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE activity = 'poker'`).Scan(&txCount); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	if txCount != 2 {
		t.Errorf("poker transaction rows = %d, want 2 (one per delta)", txCount)
	}
}

// TestSettlePokerPreservesDisplayNameButLogsItOnTransactions pins the exact
// semantics SettlePoker replaces: the balances upsert must ALWAYS preserve
// the player's existing stored display name (never overwrite it with
// whatever name happened to be on the Seat at settlement time — that name
// can be stale, e.g. after a Telegram display-name change mid-session),
// while the transactions audit row DOES record the name passed in, exactly
// as the old UpdateBalance(userID, "", d) / LogTransaction(userID, s.Name,
// "poker", d) pairing did.
func TestSettlePokerPreservesDisplayNameButLogsItOnTransactions(t *testing.T) {
	db := newTestDB(t)
	db.GetBalance("u1", "OriginalName") // seeds balances.name = "OriginalName"

	if err := db.SettlePoker([]PokerDelta{
		{UserID: "u1", Name: "StaleSeatName", Amount: 50},
	}); err != nil {
		t.Fatalf("SettlePoker: %v", err)
	}

	var storedName string
	if err := db.db.QueryRow(`SELECT name FROM balances WHERE user_id = 'u1'`).Scan(&storedName); err != nil {
		t.Fatalf("query balances.name: %v", err)
	}
	if storedName != "OriginalName" {
		t.Errorf("balances.name = %q, want unchanged %q (settlement must never overwrite the stored display name)", storedName, "OriginalName")
	}

	var txName string
	if err := db.db.QueryRow(`SELECT name FROM transactions WHERE user_id = 'u1' AND activity = 'poker'`).Scan(&txName); err != nil {
		t.Fatalf("query transactions.name: %v", err)
	}
	if txName != "StaleSeatName" {
		t.Errorf("transactions.name = %q, want %q (audit row records the name as of settlement time)", txName, "StaleSeatName")
	}
}

// TestSettlePokerRollsBackAllEntriesOnMidTransactionFailure is the
// discriminating test for FIX 3: a hand's settlement writes must be
// all-or-nothing. A trigger local to this test DB (never touching the
// production schema in sqlite.go) forces the SECOND delta's transaction-log
// insert to fail, simulating the kind of mid-write error a crash or SIGTERM
// could also produce. Without the transaction wrapper, the FIRST delta's
// balance update — which runs and would succeed before the failure — would
// stick around uncommitted-but-visible; with it, the whole call must roll
// back cleanly, leaving both balances untouched.
func TestSettlePokerRollsBackAllEntriesOnMidTransactionFailure(t *testing.T) {
	db := newTestDB(t)
	db.UpdateBalance("u1", "Alice", 900) // -> 1000
	db.UpdateBalance("u2", "Bob", 900)   // -> 1000

	const sentinelAmount = -999999
	if _, err := db.db.Exec(`CREATE TRIGGER settle_test_fail BEFORE INSERT ON transactions
		WHEN NEW.amount = -999999 BEGIN SELECT RAISE(ABORT, 'forced failure for atomicity test'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	err := db.SettlePoker([]PokerDelta{
		{UserID: "u1", Name: "Alice", Amount: 500},          // would succeed if applied in isolation
		{UserID: "u2", Name: "Bob", Amount: sentinelAmount}, // forced failure
	})
	if err == nil {
		t.Fatal("SettlePoker: want an error from the forced trigger failure, got nil")
	}

	if got := db.GetBalance("u1", ""); got != 1000 {
		t.Errorf("u1 balance = %d after a failed settlement, want unchanged 1000 (a partial write leaked past the failed transaction)", got)
	}
	if got := db.GetBalance("u2", ""); got != 1000 {
		t.Errorf("u2 balance = %d after a failed settlement, want unchanged 1000", got)
	}
}

// TestSettlePokerEmptyIsNoop proves SettlePoker tolerates a hand where every
// delta is zero (already filtered out by the caller) without touching the
// database or erroring.
func TestSettlePokerEmptyIsNoop(t *testing.T) {
	db := newTestDB(t)
	if err := db.SettlePoker(nil); err != nil {
		t.Fatalf("SettlePoker(nil): %v", err)
	}
}

func TestDailyStats(t *testing.T) {
	db := newTestDB(t)

	db.UpdateDailyStats("u1", "Alice", true)
	db.UpdateDailyStats("u1", "Alice", false)

	stats, err := db.GetDailyStats()
	if err != nil {
		t.Fatalf("GetDailyStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 user, got %d", len(stats))
	}
	if stats[0].Work != 1 || stats[0].Personal != 1 {
		t.Errorf("expected Work=1, Personal=1, got %+v", stats[0])
	}

	db.ResetDailyStats()

	stats, err = db.GetDailyStats()
	if err != nil {
		t.Fatalf("GetDailyStats after reset: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 users after reset, got %d", len(stats))
	}
}

// --- RemoveCardsByRarity (query-then-write deadlock fix) -------------------
//
// RemoveCardsByRarity used to run its DELETE/UPDATE writes while the
// selecting *sql.Rows was still open. With db.SetMaxOpenConns(1) (sqlite.go
// ~L41) that pins the pool's only connection on the open rows, so the
// in-loop d.db.Exec calls block forever waiting for a connection that can
// never be freed — a process-wide deadlock. This test drives the function
// with enough matching rows that the loop iterates more than once, so it
// would hang against the pre-fix code instead of completing.
func TestRemoveCardsByRarityDoesNotDeadlockAndRemovesCorrectCount(t *testing.T) {
	db := newTestDB(t)

	const userID = "u1"
	const rarity = 5

	// Three cards of the target rarity, plus a decoy of a different rarity
	// that must never be touched.
	db.AddCard(1, "Card One", rarity, "cat", "🃏", "desc", 1, 1, "", 0)
	db.AddCard(2, "Card Two", rarity, "cat", "🃏", "desc", 1, 1, "", 0)
	db.AddCard(3, "Card Three", rarity, "cat", "🃏", "desc", 1, 1, "", 0)
	db.AddCard(4, "Decoy", rarity+1, "cat", "🃏", "desc", 1, 1, "", 0)

	db.AddToCollection(userID, 1) // count = 1 -> should be deleted entirely
	db.AddToCollection(userID, 2)
	db.AddToCollection(userID, 2) // count = 2 -> should be decremented to 1
	db.AddToCollection(userID, 3)
	db.AddToCollection(userID, 3)
	db.AddToCollection(userID, 3) // count = 3 -> should be decremented to 2
	db.AddToCollection(userID, 4) // decoy, different rarity

	removed := db.RemoveCardsByRarity(userID, rarity, 3)
	if removed != 3 {
		t.Fatalf("expected 3 cards removed, got %d", removed)
	}

	counts := map[int]int{}
	rows, err := db.db.Query(`SELECT card_id, count FROM collection WHERE user_id = ?`, userID)
	if err != nil {
		t.Fatalf("query collection: %v", err)
	}
	for rows.Next() {
		var cardID, cnt int
		if err := rows.Scan(&cardID, &cnt); err != nil {
			t.Fatalf("scan: %v", err)
		}
		counts[cardID] = cnt
	}
	rows.Close()

	if _, ok := counts[1]; ok {
		t.Errorf("card 1 should have been fully removed, still present: %+v", counts)
	}
	if counts[2] != 1 {
		t.Errorf("expected card 2 count=1, got %d", counts[2])
	}
	if counts[3] != 2 {
		t.Errorf("expected card 3 count=2, got %d", counts[3])
	}
	if counts[4] != 1 {
		t.Errorf("decoy card 4 (different rarity) should be untouched, got count=%d", counts[4])
	}
}

func TestGetTopBalancesExcludesBotsAndBank(t *testing.T) {
	db := newTestDB(t)

	db.UpdateBalance("460670583", "Danya", 5000)
	db.UpdateBalance("bank:house", "Bank", 99999)
	db.UpdateBalance("bot:1", "Вася", 88888)

	for _, e := range db.GetTopBalances(10) {
		if e.UserID == "bank:house" || e.UserID == "bot:1" {
			t.Errorf("leaderboard leaked a non-player row: %s (%s)", e.UserID, e.Name)
		}
	}
	found := false
	for _, e := range db.GetTopBalances(10) {
		if e.UserID == "460670583" {
			found = true
		}
	}
	if !found {
		t.Error("real player missing from leaderboard")
	}
}
