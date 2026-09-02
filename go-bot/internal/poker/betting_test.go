package poker

import "testing"

func headsUp(t *testing.T) *Table {
	t.Helper()
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	return tbl
}

func TestActRejectsOutOfTurn(t *testing.T) {
	tbl := headsUp(t)
	wrong := tbl.Seats[(tbl.ToAct+1)%len(tbl.Seats)].UserID
	if err := tbl.Act(wrong, ActCall, 0); err == nil {
		t.Fatal("expected error acting out of turn")
	}
}

func TestActRejectsCheckWhenBetOutstanding(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActCheck, 0); err == nil {
		t.Fatal("expected error checking with a bet outstanding")
	}
}

func TestActRejectsRaiseBelowMin(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, BigBlind+1); err == nil {
		t.Fatal("expected error raising below the minimum")
	}
}

func TestActRejectsRaiseAboveStack(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct]
	if err := tbl.Act(actor.UserID, ActRaise, actor.Stack+9999); err == nil {
		t.Fatal("expected error raising more than the stack")
	}
}

func TestFoldToOneEndsHandImmediately(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActFold, 0); err != nil {
		t.Fatalf("fold: %v", err)
	}
	if tbl.Stage != StageShowdown {
		t.Errorf("stage = %v, want showdown after everyone folded", tbl.Stage)
	}
}

func TestCallThenCheckAdvancesToFlop(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(first, ActCall, 0); err != nil {
		t.Fatalf("call: %v", err)
	}
	second := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(second, ActCheck, 0); err != nil {
		t.Fatalf("check: %v", err)
	}
	if tbl.Stage != StageFlop {
		t.Fatalf("stage = %v, want flop", tbl.Stage)
	}
	if len(tbl.Board) != 3 {
		t.Errorf("board = %d cards, want 3", len(tbl.Board))
	}
	for _, s := range tbl.Seats {
		if s.Bet != 0 {
			t.Errorf("seat %s street bet = %d, want reset to 0", s.UserID, s.Bet)
		}
	}
}

// TestBothAllInBeforeRiverReachesShowdown verifies that when both players go
// all-in preflop, the board is dealt out to showdown without hanging.
func TestBothAllInBeforeRiverReachesShowdown(t *testing.T) {
	tbl := NewTable("t1", 1)
	// Give each player 1500 chips
	_ = tbl.Sit("u1", "Danya", 1500)
	_ = tbl.Sit("u2", "Data", 1500)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// u1 (small blind, 50) acts first preflop; u1 raises to 600
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 600); err != nil {
		t.Fatalf("raise to 600: %v", err)
	}

	// u2 (big blind, 100) goes all-in for their full stack (1500)
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 1500); err != nil {
		t.Fatalf("raise to 1500: %v", err)
	}

	// u1 calls, going all-in with remaining chips
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActCall, 0); err != nil {
		t.Fatalf("call: %v", err)
	}

	// Both players are all-in; advance() must deal all streets without hanging
	if tbl.Stage != StageShowdown {
		t.Errorf("stage = %v, want showdown; if this test hangs, advance() didn't loop", tbl.Stage)
	}

	// Verify the full board was dealt
	if len(tbl.Board) != 5 {
		t.Errorf("board = %d cards, want 5", len(tbl.Board))
	}
}
