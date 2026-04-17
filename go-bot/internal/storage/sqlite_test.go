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
