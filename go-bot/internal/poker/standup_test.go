package poker

import "testing"

func TestHasLiveStake(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)

	if tbl.HasLiveStake("a") {
		t.Error("stake reported before any hand was dealt")
	}
	_ = tbl.StartHand()
	if !tbl.HasLiveStake("a") {
		t.Error("no stake reported mid-hand while holding cards")
	}
	tbl.Seats[0].Folded = true
	if tbl.HasLiveStake("a") {
		t.Error("stake reported after folding")
	}
	tbl.Seats[0].Folded = false
	tbl.Stage = StageShowdown
	if tbl.HasLiveStake("a") {
		t.Error("stake reported at showdown, when the hand is already settled")
	}
	if tbl.HasLiveStake("nobody") {
		t.Error("stake reported for a user with no seat here")
	}
}

// Standing up mid-hand would delete chips that are already part of a pot
// other players are contesting.
func TestStandUpRefusedMidHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.StartHand()

	if tbl.StandUp("a") {
		t.Error("StandUp succeeded mid-hand")
	}
	if len(tbl.Seats) != 2 {
		t.Errorf("seats = %d, want 2 — the seat was removed anyway", len(tbl.Seats))
	}
}

func TestStandUpBetweenHandsRepairsIndices(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.Sit("c", "C", 5000)
	_ = tbl.StartHand()
	tbl.Stage = StageShowdown // hand over: standing up is now allowed

	tbl.Button = 2
	tbl.ToAct = 2
	if !tbl.StandUp("c") {
		t.Fatal("StandUp refused between hands")
	}
	if len(tbl.Seats) != 2 {
		t.Fatalf("seats = %d, want 2", len(tbl.Seats))
	}
	// Every surviving index must still address a real seat.
	if tbl.Button < -1 || tbl.Button >= len(tbl.Seats) {
		t.Errorf("Button = %d, out of range for %d seats", tbl.Button, len(tbl.Seats))
	}
	if tbl.ToAct != -1 && (tbl.ToAct < 0 || tbl.ToAct >= len(tbl.Seats)) {
		t.Errorf("ToAct = %d, out of range for %d seats", tbl.ToAct, len(tbl.Seats))
	}
	// The removed seat must be gone, and the others untouched.
	for _, s := range tbl.Seats {
		if s.UserID == "c" {
			t.Error("seat c survived StandUp")
		}
	}
	// The table must still be able to deal.
	if err := tbl.StartHand(); err != nil {
		t.Errorf("table unusable after StandUp: %v", err)
	}
}

// Removing a seat BELOW the button must shift the button down with it, or it
// would silently point at a different player.
func TestStandUpShiftsButtonBelowRemovedSeat(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.Sit("c", "C", 5000)
	tbl.Button = 2 // seat "c"

	if !tbl.StandUp("a") { // remove index 0, below the button
		t.Fatal("StandUp refused on a waiting table")
	}
	if tbl.Seats[tbl.Button].UserID != "c" {
		t.Errorf("button now points at %q, want it to follow c", tbl.Seats[tbl.Button].UserID)
	}
}
