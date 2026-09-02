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

// threeHanded creates a 3-player table with 5000 chips each, starting a hand.
func threeHanded(t *testing.T) *Table {
	t.Helper()
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 5000)
	_ = tbl.Sit("u2", "Bob", 5000)
	_ = tbl.Sit("u3", "Carol", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	return tbl
}

// totalChips sums all stacks and committed across the table.
func totalChips(tbl *Table) int {
	total := 0
	for _, s := range tbl.Seats {
		total += s.Stack + s.Committed
	}
	return total
}

// TestMinRaiseStaysNonNegativeWithShortAllIn is a regression test for C1:
// when a player goes all-in for less than a full raise, MinRaise must not go negative.
// Against the broken code, this test fails: MinRaise becomes negative, subsequent "raises"
// bypass the min-raise check, and post() is called with negative amounts, conjuring chips.
func TestMinRaiseStaysNonNegativeWithShortAllIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 3000)
	_ = tbl.Sit("u2", "Bob", 1050) // Small stack for short all-in
	_ = tbl.Sit("u3", "Carol", 3000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	initialChips := totalChips(tbl)

	// u1 (button) raises to 900
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 900); err != nil {
		t.Fatalf("raise to 900: %v", err)
	}

	// u2 (small blind, started with 1050, posted 50 SB, has 1000 left) goes all-in
	// Total commitment: all 1000 remaining + 50 already bet = 1050
	// This is less than a full raise from 900: full raise would be 900 + (900-100) = 1700
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 1050); err != nil {
		t.Fatalf("all-in for 1050: %v", err)
	}

	// u3 (big blind) re-raises to 2000 (genuine raise)
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 2000); err != nil {
		t.Fatalf("raise to 2000: %v", err)
	}

	// After u2's short all-in, MinRaise must not go negative
	if tbl.MinRaise < 0 {
		t.Errorf("MinRaise = %d after short all-in, want >= 0 (invariant C1 broken)", tbl.MinRaise)
	}

	// Total chips must be conserved (no conjuring via negative post)
	if finalChips := totalChips(tbl); finalChips != initialChips {
		t.Errorf("total chips = %d, want %d (chips conjured/destroyed)", finalChips, initialChips)
	}
}

// TestRaiseReopensForPreviousActor verifies that after a player raises,
// a player who already acted this street gets another turn (3-handed scenario).
func TestRaiseReopensForPreviousActor(t *testing.T) {
	tbl := threeHanded(t)

	// u1 (button) raises to 500
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 500); err != nil {
		t.Fatalf("u1 raise to 500: %v", err)
	}

	// u2 (small blind) calls
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActCall, 0); err != nil {
		t.Fatalf("u2 call: %v", err)
	}

	// u3 (big blind) re-raises to 1500
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 1500); err != nil {
		t.Fatalf("u3 re-raise to 1500: %v", err)
	}

	// u1 should get another turn (even though u1 already acted)
	// because u3's re-raise reopened betting
	if tbl.ToAct != 0 {
		t.Errorf("ToAct = %d (expected u1 at index 0), u1 should get another turn after re-raise", tbl.ToAct)
	}
}

// TestMinRaiseAfterLegitimateRaise verifies MinRaise is correctly calculated
// when a genuine raise occurs (not an all-in-for-less).
func TestMinRaiseAfterLegitimateRaise(t *testing.T) {
	tbl := threeHanded(t)

	// u1 raises to 500; MinRaise should be 500 - BigBlind = 400
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 500); err != nil {
		t.Fatalf("u1 raise to 500: %v", err)
	}

	expectedMinRaise := 500 - BigBlind
	if tbl.MinRaise != expectedMinRaise {
		t.Errorf("MinRaise = %d after u1's raise to 500, want %d", tbl.MinRaise, expectedMinRaise)
	}

	// u2 re-raises to 1500; MinRaise should become 1500 - 500 = 1000
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 1500); err != nil {
		t.Fatalf("u2 raise to 1500: %v", err)
	}

	expectedMinRaise = 1500 - 500
	if tbl.MinRaise != expectedMinRaise {
		t.Errorf("MinRaise = %d after u2's raise to 1500, want %d", tbl.MinRaise, expectedMinRaise)
	}
}

// TestShortCallBecomesAllIn verifies that a call for fewer chips than needed
// correctly marks the seat as all-in.
func TestShortCallBecomesAllIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 2000)
	_ = tbl.Sit("u2", "Bob", 2000)
	_ = tbl.Sit("u3", "Carol", 1000) // Will have exactly 900 left after BB (1000 - 100)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// u1 raises to 1000
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 1000); err != nil {
		t.Fatalf("u1 raise: %v", err)
	}

	// u2 calls
	actor = tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActCall, 0); err != nil {
		t.Fatalf("u2 call: %v", err)
	}

	// u3 (big blind, started with 1000, posted 100 BB, has 900 left)
	// Needs to call 900 more to match the 1000 bet, so goes all-in exactly
	u3Idx := 2
	u3 := tbl.Seats[u3Idx]
	actor = u3.UserID
	if err := tbl.Act(actor, ActCall, 0); err != nil {
		t.Fatalf("u3 call: %v", err)
	}

	if !u3.AllIn {
		t.Errorf("u3.AllIn = false, want true after short stack call")
	}
	if u3.Stack != 0 {
		t.Errorf("u3.Stack = %d after all-in, want 0", u3.Stack)
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

	// Both players are all-in; advance() must deal all streets without hanging.
	// Against the non-looping version, this assertion fails cleanly (stage is StageFlop
	// with only 3 board cards), but the production engine would wedge indefinitely.
	if tbl.Stage != StageShowdown {
		t.Errorf("stage = %v, want showdown (looping advance() must auto-deal to showdown when all-in)", tbl.Stage)
	}

	// Verify the full board was dealt
	if len(tbl.Board) != 5 {
		t.Errorf("board = %d cards, want 5", len(tbl.Board))
	}
}
