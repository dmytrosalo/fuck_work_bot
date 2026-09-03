package poker

import (
	"testing"
	"time"
)

func TestForceTimeoutFoldsWhenBetOutstanding(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if !actor.Folded {
		t.Error("player facing a bet should be folded on timeout")
	}
}

func TestForceTimeoutChecksWhenFree(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	_ = tbl.Act(first, ActCall, 0) // now the big blind can check
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if actor.Folded {
		t.Error("player with no bet to call should be checked, not folded")
	}
}

func TestForceTimeoutNoopBeforeDeadline(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(time.Minute)
	if tbl.ForceTimeout() {
		t.Fatal("timeout must not fire before the deadline")
	}
}

// TestForceTimeoutNoopInWaitingStage proves the deadline check is gated on
// stage: a table that has never started a hand has a zero-value Deadline
// (already in the past) and ToAct=0 by default, so without the stage guard
// ForceTimeout would misfire on an empty table.
func TestForceTimeoutNoopInWaitingStage(t *testing.T) {
	tbl := NewTable("t1", 1)
	if tbl.ForceTimeout() {
		t.Fatal("timeout must not fire while the table is waiting for players")
	}
}

// TestForceTimeoutNoopAfterShowdown proves the deadline check is gated on
// stage even once the deadline has clearly expired: a hand that has already
// reached showdown must not be re-acted on by the sweeper.
func TestForceTimeoutNoopAfterShowdown(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(first, ActFold, 0); err != nil {
		t.Fatalf("Act: %v", err)
	}
	if tbl.Stage != StageShowdown {
		t.Fatalf("stage = %v, want StageShowdown after the only live player folds", tbl.Stage)
	}
	tbl.Deadline = time.Now().Add(-time.Hour)
	if tbl.ForceTimeout() {
		t.Fatal("timeout must not fire once the hand has reached showdown")
	}
}

// TestForceTimeoutFoldToShowdownReturnsTrue proves ForceTimeout still
// reports true (it acted) even when the auto-fold ends the hand outright —
// callers like the sweeper key their settle-and-broadcast logic off this
// return value.
func TestForceTimeoutFoldToShowdownReturnsTrue(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(-time.Second)
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire and fold the sole live player")
	}
	if tbl.Stage != StageShowdown {
		t.Fatalf("stage = %v, want StageShowdown after the auto-fold ends the hand", tbl.Stage)
	}
}
