package poker

import "testing"

func TestAdjustSeatMovesChips(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)

	if got := tbl.AdjustSeat("a", 303); got != 303 {
		t.Errorf("applied = %d, want 303", got)
	}
	if tbl.Seats[0].Stack != 5303 {
		t.Errorf("stack = %d, want 5303", tbl.Seats[0].Stack)
	}
	if got := tbl.AdjustSeat("nobody", 100); got != 0 {
		t.Errorf("unseated user applied = %d, want 0", got)
	}
}

// A withdrawal can never take more than the chips actually in the stack:
// anything already in the pot belongs to the pot.
func TestAdjustSeatClampsWithdrawal(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.StartHand()

	seat := tbl.Seats[0]
	committed := seat.Committed
	stack := seat.Stack

	applied := tbl.AdjustSeat("a", -(stack + 9999))
	if applied != -stack {
		t.Errorf("applied = %d, want %d (clamped to the stack)", applied, -stack)
	}
	if seat.Stack != 0 {
		t.Errorf("stack = %d, want 0, never negative", seat.Stack)
	}
	if seat.Committed != committed {
		t.Errorf("committed chips were raided: %d -> %d", committed, seat.Committed)
	}
}

// The one that matters. An outside credit while a hand is live must NOT be
// reported as poker winnings at showdown — the balance already received it,
// so counting it again would mint богдудіки out of a /rob.
func TestAdjustSeatDoesNotInflateSettlement(t *testing.T) {
	run := func(robMidHand bool) map[string]int {
		tbl := NewTable("t", 1)
		_ = tbl.Sit("a", "A", 5000)
		_ = tbl.Sit("b", "B", 5000)
		_ = tbl.StartHand()
		if robMidHand {
			tbl.AdjustSeat("a", 303)
		}
		tbl.Seats[0].Hole = cards("Ah", "Ad")
		tbl.Seats[1].Hole = cards("2c", "7d")
		tbl.Board = cards("As", "Kh", "Qd", "3c", "9s")
		tbl.Stage = StageRiver
		return tbl.Showdown()
	}

	plain, robbed := run(false), run(true)
	for _, id := range []string{"a", "b"} {
		if plain[id] != robbed[id] {
			t.Errorf("seat %s settled %d with an outside credit but %d without — "+
				"the credit leaked into the hand result", id, robbed[id], plain[id])
		}
	}
	sum := 0
	for _, d := range robbed {
		sum += d
	}
	if sum != 0 {
		t.Errorf("settlement deltas sum to %d, want 0 — chips were created", sum)
	}
}

// Between hands the credit must survive into the next hand's opening stack,
// otherwise the chips would appear and then silently vanish on the deal.
func TestAdjustSeatSurvivesIntoNextHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	before := tbl.Seats[0].Stack
	tbl.AdjustSeat("a", 1000)
	_ = tbl.StartHand()
	if got := tbl.Seats[0].Stack + tbl.Seats[0].Committed; got != before+1000 {
		t.Errorf("next hand opened with %d chips, want %d", got, before+1000)
	}
}
