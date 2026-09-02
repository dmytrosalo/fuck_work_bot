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
	// With correct awarding:
	// Main pot (300): u1 (AA) wins all → delta +200
	// Side pot (400): u2 (KQ) wins → delta +100
	// u3 loses → delta -300
	if deltas["u1"] != 200 {
		t.Errorf("u1 delta = %d, want 200", deltas["u1"])
	}
	if deltas["u2"] != 100 {
		t.Errorf("u2 delta = %d, want 100", deltas["u2"])
	}
	if deltas["u3"] != -300 {
		t.Errorf("u3 delta = %d, want -300", deltas["u3"])
	}
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
	_ = tbl.Sit("u3", "C", 5000)
	_ = tbl.StartHand()

	// After StartHand: Seat 0 is Button, Seat 1 is SB (50), Seat 2 is BB (100).
	// Stacks: [5000, 4950, 4900], Committed: [0, 50, 100], startStack: [5000, 5000, 5000]
	// Manually set to create odd chip from dead chips (folded contributor).
	// Seats A(0) and B(1): Committed 75 each, live, tied winning hand
	// Seat C(2): Committed 1, FOLDED (dead chips)
	// This creates pots at levels [1, 75]:
	// - Level 1: 3 chips (1 from each), eligible [A,B] → split 1 each, remainder 1
	// - Level 75: 148 chips (74 from A, 74 from B), eligible [A,B] → split 74 each

	tbl.Seats[0].Stack -= 75 // increase Committed from 0 to 75
	tbl.Seats[0].Committed = 75
	tbl.Seats[0].Folded = false
	tbl.Seats[0].AllIn = false

	tbl.Seats[1].Stack -= 25 // increase Committed from 50 to 75
	tbl.Seats[1].Committed = 75
	tbl.Seats[1].Folded = false
	tbl.Seats[1].AllIn = false

	tbl.Seats[2].Stack += 99 // decrease Committed from 100 to 1
	tbl.Seats[2].Committed = 1
	tbl.Seats[2].Folded = true // FOLDED: dead chips
	tbl.Seats[2].AllIn = false

	// Identical hands for A and B: the board plays. C is folded, chips are dead.
	tbl.Board = cards("Ah", "Kh", "Qh", "Jh", "Th")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")
	tbl.Seats[2].Hole = cards("6h", "7h") // not evaluated (folded)

	// Set Button explicitly so we can verify odd chip goes to first left-of-button winner
	tbl.Button = 0 // A is button

	deltas := tbl.Showdown()
	// With 3-chip pot, 2 equal winners: 1 chip each + 1 remainder
	// Remainder goes to first winner left of button (button=0, so (0+1)%3=1 → B)
	// Stacks: A=4925+1+74=5000 (delta 0), B=4925+2+74=5001 (delta 1), C=4999+0=4999 (delta -1)
	if deltas["u1"] != 0 {
		t.Errorf("u1 (A, first seat) delta = %d, want 0 (shouldn't get odd chip)", deltas["u1"])
	}
	if deltas["u2"] != 1 {
		t.Errorf("u2 (B, second seat) delta = %d, want 1 (should get odd chip)", deltas["u2"])
	}
	if deltas["u3"] != -1 {
		t.Errorf("u3 (C, folded) delta = %d, want -1", deltas["u3"])
	}
	sum := deltas["u1"] + deltas["u2"] + deltas["u3"]
	if sum != 0 {
		t.Fatalf("split pot deltas sum to %d, want 0 (odd chip must not vanish)", sum)
	}
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
