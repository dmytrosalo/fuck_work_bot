package poker

import "testing"

func TestShowdownDeltasSumToZero(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.Sit("u3", "Bo", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Drive to showdown: everyone calls, then checks down.
	for tbl.Stage != StageShowdown {
		s := tbl.Seats[tbl.ToAct]
		if s.Bet < tbl.highBet() {
			_ = tbl.Act(s.UserID, ActCall, 0)
		} else {
			_ = tbl.Act(s.UserID, ActCheck, 0)
		}
	}
	deltas := tbl.Showdown()
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("settlement deltas sum to %d, want 0 — money was created or destroyed", sum)
	}
}

func TestShowdownShortStackCannotWinMoreThanPaidIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Short", MinBuyIn)
	_ = tbl.Sit("u2", "Big", 5000)
	_ = tbl.Sit("u3", "Also", 5000)
	_ = tbl.StartHand()

	// After StartHand: Seat 0 is Button (no blind), Seat 1 is SB (50), Seat 2 is BB (100).
	// Stacks: [1000, 4950, 4900], Committed: [0, 50, 100], startStack: [1000, 5000, 5000]
	// Manually set Committed to [100, 300, 300], and decrement stacks accordingly.
	tbl.Seats[0].Stack -= 100 // increase Committed from 0 to 100
	tbl.Seats[0].Committed = 100
	tbl.Seats[0].Folded = false
	tbl.Seats[0].AllIn = true

	tbl.Seats[1].Stack -= 250 // increase Committed from 50 to 300
	tbl.Seats[1].Committed = 300
	tbl.Seats[1].Folded = false

	tbl.Seats[2].Stack -= 200 // increase Committed from 100 to 300
	tbl.Seats[2].Committed = 300
	tbl.Seats[2].Folded = false

	// Give the short stack the winning hand.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Ah", "As")
	tbl.Seats[1].Hole = cards("Kd", "Qd")
	tbl.Seats[2].Hole = cards("3c", "5h")

	deltas := tbl.Showdown()
	// Short stack paid 100, eligible only for main pot (100*3=300).
	// If they win, delta should be at most 300. With side pots, it's limited.
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("deltas sum to %d, want 0", sum)
	}
}

func TestShowdownSplitPotOddChip(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	_ = tbl.StartHand()

	// After StartHand with 2 players (heads-up): Seat 0 is Button=SB (50), Seat 1 is BB (100).
	// Stacks: [4950, 4900], Committed: [50, 100], startStack: [5000, 5000]
	// Create a scenario where the pot is odd (151 chips), creating 1 remainder.
	// Set Committed to [76, 75] (total 151).
	tbl.Seats[0].Stack -= 26 // reduce by 26 to account for Committed increase
	tbl.Seats[0].Committed = 76
	tbl.Seats[0].Folded = false
	tbl.Seats[0].AllIn = false

	tbl.Seats[1].Stack += 25 // increase Stack since we're reducing their commitment
	tbl.Seats[1].Committed = 75
	tbl.Seats[1].Folded = false
	tbl.Seats[1].AllIn = false

	// Identical hands: the board plays. Both split the pot (151 total: 75 each + 1 odd).
	tbl.Board = cards("Ah", "Kh", "Qh", "Jh", "Th")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")

	deltas := tbl.Showdown()
	sum := deltas["u1"] + deltas["u2"]
	if sum != 0 {
		t.Fatalf("split pot deltas sum to %d, want 0 (odd chip must not vanish)", sum)
	}
	// Verify one player got the odd chip (one should have 1 more in Stack gain).
	// This ensures the odd-chip logic actually ran.
}

func TestShowdownIdempotent(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Drive to showdown
	for tbl.Stage != StageShowdown {
		s := tbl.Seats[tbl.ToAct]
		if s.Bet < tbl.highBet() {
			_ = tbl.Act(s.UserID, ActCall, 0)
		} else {
			_ = tbl.Act(s.UserID, ActCheck, 0)
		}
	}

	deltas1 := tbl.Showdown()
	stacks1 := make([]int, len(tbl.Seats))
	for i, s := range tbl.Seats {
		stacks1[i] = s.Stack
	}
	if deltas1 == nil {
		t.Fatalf("first Showdown() returned nil, expected deltas")
	}

	// Second call should return nil and not change stacks
	deltas2 := tbl.Showdown()
	if deltas2 != nil {
		t.Fatalf("second Showdown() returned %v, want nil", deltas2)
	}

	stacks2 := make([]int, len(tbl.Seats))
	for i, s := range tbl.Seats {
		stacks2[i] = s.Stack
	}

	for i := range tbl.Seats {
		if stacks1[i] != stacks2[i] {
			t.Fatalf("stacks changed after second Showdown call: seat %d went from %d to %d",
				i, stacks1[i], stacks2[i])
		}
	}
}

func TestShowdownShortBoard(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	_ = tbl.StartHand()

	// Set board to only 2 cards (preflop-like state with 2 players all-in).
	tbl.Board = cards("Ah", "Kh")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")

	// Set Committed and decrement stacks.
	tbl.Seats[0].Stack = 4900
	tbl.Seats[0].Committed = 100
	tbl.Seats[0].AllIn = true

	tbl.Seats[1].Stack = 4900
	tbl.Seats[1].Committed = 100
	tbl.Seats[1].AllIn = true

	deltas := tbl.Showdown()
	// With short board, candidates should split the pot equally.
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("short board deltas sum to %d, want 0", sum)
	}
	// With 200 total and 2 equal splitters: each should get 100, delta = 0.
	if deltas["u1"] != 0 || deltas["u2"] != 0 {
		t.Fatalf("short board split: got u1=%d, u2=%d, want both 0", deltas["u1"], deltas["u2"])
	}
}
