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

	tbl.Seats[0].Committed, tbl.Seats[0].Folded, tbl.Seats[0].AllIn = 100, false, true
	tbl.Seats[1].Committed, tbl.Seats[1].Folded = 300, false
	tbl.Seats[2].Committed, tbl.Seats[2].Folded = 300, false
	// Give the short stack the winning hand.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Ah", "As")
	tbl.Seats[1].Hole = cards("Kd", "Qd")
	tbl.Seats[2].Hole = cards("3c", "5h")

	deltas := tbl.Showdown()
	// Short stack paid 100, so may win at most 100 from each of the other two.
	if deltas["u1"] > 200 {
		t.Fatalf("short stack won %d, cannot exceed 200", deltas["u1"])
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
	_ = tbl.StartHand()
	tbl.Button = 0
	tbl.Seats[0].Committed, tbl.Seats[0].Folded, tbl.Seats[0].AllIn = 75, false, false
	tbl.Seats[1].Committed, tbl.Seats[1].Folded, tbl.Seats[1].AllIn = 75, false, false
	// Identical hands: the board plays.
	tbl.Board = cards("Ah", "Kh", "Qh", "Jh", "Th")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")

	deltas := tbl.Showdown()
	sum := deltas["u1"] + deltas["u2"]
	if sum != 0 {
		t.Fatalf("split pot deltas sum to %d, want 0 (odd chip must not vanish)", sum)
	}
}
