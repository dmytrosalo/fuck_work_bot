package handlers

import (
	"testing"
)

// TestStaleClaimReleasedOnNewTable reproduces the dead end: run /poker
// again, get a brand-new table, and be refused with "Ти вже за іншим
// столом" because the claim still points at the old one — with no way to
// clear it short of the 30-minute idle reclaim or a redeploy.
func TestStaleClaimReleasedOnNewTable(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	old := h.Create(-1)
	fresh := h.Create(-1)

	old.Lock()
	_ = old.Sit("42", "Dmytro", 5000)
	old.Unlock()
	h.mu.Lock()
	h.seatedAt["42"] = old.ID
	h.mu.Unlock()

	// Before: the claim blocks the new table.
	if _, ok, _ := h.claimSeat("42", fresh.ID); ok {
		t.Fatal("setup wrong: claim did not block the new table")
	}

	h.releaseStaleClaim("42", fresh.ID)

	_, ok, live := h.claimSeat("42", fresh.ID)
	if !live {
		t.Fatal("new table reported dead")
	}
	if !ok {
		t.Error("still locked out of the new table after releasing a stale claim")
	}
	// The abandoned seat must be gone, or it would keep paying blinds into
	// settlements against a bankroll now committed elsewhere.
	old.Lock()
	orphan := old.SeatIndexOf("42")
	old.Unlock()
	if orphan >= 0 {
		t.Errorf("old table still seats the player at index %d — two funded seats", orphan)
	}
}

// A player genuinely mid-hand elsewhere must still be refused: releasing
// them would abandon chips already in a pot other players are contesting.
func TestLiveClaimNotReleased(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	old := h.Create(-1)
	fresh := h.Create(-1)

	old.Lock()
	_ = old.Sit("42", "Dmytro", 5000)
	_ = old.Sit("43", "Danya", 5000)
	_ = old.StartHand()
	old.Unlock()
	h.mu.Lock()
	h.seatedAt["42"] = old.ID
	h.mu.Unlock()

	h.releaseStaleClaim("42", fresh.ID)

	h.mu.Lock()
	still := h.seatedAt["42"]
	h.mu.Unlock()
	if still != old.ID {
		t.Error("claim released while the player had chips in a live pot")
	}
	old.Lock()
	seated := old.SeatIndexOf("42") >= 0
	old.Unlock()
	if !seated {
		t.Error("player was stood up mid-hand, deleting chips from a live pot")
	}
}

// Re-opening the SAME table must be left completely alone — that is the
// reconnect path, not a stale claim.
func TestSameTableClaimUntouched(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	_ = tbl.Sit("42", "Dmytro", 5000)
	tbl.Unlock()
	h.mu.Lock()
	h.seatedAt["42"] = tbl.ID
	h.mu.Unlock()

	h.releaseStaleClaim("42", tbl.ID)

	h.mu.Lock()
	still := h.seatedAt["42"]
	h.mu.Unlock()
	if still != tbl.ID {
		t.Error("reconnecting to the same table cleared its own claim")
	}
	tbl.Lock()
	seated := tbl.SeatIndexOf("42") >= 0
	tbl.Unlock()
	if !seated {
		t.Error("reconnecting to the same table stood the player up")
	}
}

// A claim pointing at a table the sweeper already reclaimed must clear
// without needing that table to still exist.
func TestClaimOnVanishedTableReleased(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	fresh := h.Create(-1)
	h.mu.Lock()
	h.seatedAt["42"] = "long-gone-table"
	h.mu.Unlock()

	h.releaseStaleClaim("42", fresh.ID)

	if _, ok, _ := h.claimSeat("42", fresh.ID); !ok {
		t.Error("still locked out by a claim on a table that no longer exists")
	}
}
