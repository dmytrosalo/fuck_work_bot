package poker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestViewHidesOtherPlayersHoleCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	// Set deterministic cards to avoid flaky substring checks
	tbl.Seats[0].Hole = cards("2c", "3d")
	tbl.Seats[1].Hole = cards("Ah", "Ks")

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u2" && len(s.Hole) != 0 {
			t.Fatalf("viewer u1 can see u2's hole cards: %v", s.Hole)
		}
	}

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "Ah") || strings.Contains(string(raw), "Ks") {
		t.Fatalf("serialized view leaks another player's cards: %s", raw)
	}
}

func TestViewShowsOwnHoleCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u1" && len(s.Hole) != 2 {
			t.Fatalf("viewer cannot see own cards: %v", s.Hole)
		}
	}
}

func TestViewJsonOmitsHoleForOthers(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	tbl.Seats[1].Hole = cards("Ah", "Ks")

	v := tbl.ViewFor("u1")
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Parse back to verify structure
	var parsed TableView
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// u2's seat should not have Hole populated
	found := false
	for _, s := range parsed.Seats {
		if s.UserID == "u2" {
			found = true
			if s.Hole != nil && len(s.Hole) > 0 {
				t.Fatalf("u2's Hole should be empty in JSON, got: %v", s.Hole)
			}
		}
	}
	if !found {
		t.Fatalf("u2 seat not found in view")
	}
}

func TestViewShowdownPublicCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	tbl.Seats[1].Hole = cards("Ah", "Ks")
	tbl.Stage = StageShowdown
	tbl.Seats[1].Folded = false
	tbl.Seats[1].InHand = true

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u2" && len(s.Hole) != 2 {
			t.Fatalf("at showdown, non-folded player's cards should be visible: %v", s.Hole)
		}
	}
}

func TestViewShowdownFoldedHidesCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	tbl.Seats[1].Hole = cards("Ah", "Ks")
	tbl.Stage = StageShowdown
	tbl.Seats[1].Folded = true // folded, so cards stay hidden
	tbl.Seats[1].InHand = true

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u2" && len(s.Hole) > 0 {
			t.Fatalf("folded player's cards should be hidden at showdown: %v", s.Hole)
		}
	}
}

func TestViewYouSeatAssignment(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()

	v := tbl.ViewFor("u1")
	if v.YouSeat != 0 {
		t.Fatalf("viewer u1 should have YouSeat=0, got %d", v.YouSeat)
	}

	v = tbl.ViewFor("u2")
	if v.YouSeat != 1 {
		t.Fatalf("viewer u2 should have YouSeat=1, got %d", v.YouSeat)
	}
}

func TestViewBoardConversion(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	tbl.Board = cards("2h", "3d", "5c")
	tbl.Stage = StageFlop

	v := tbl.ViewFor("u1")
	if len(v.Board) != 3 {
		t.Fatalf("expected 3 board cards, got %d", len(v.Board))
	}
	if v.Board[0] != "2h" || v.Board[1] != "3d" || v.Board[2] != "5c" {
		t.Fatalf("board conversion failed: %v", v.Board)
	}
}

func TestViewPotCalculation(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	// After StartHand: u1 posts 50 (SB), u2 posts 100 (BB)
	// So pot should be 150

	v := tbl.ViewFor("u1")
	if v.Pot != 150 {
		t.Fatalf("expected pot=150 (50+100 blinds), got %d", v.Pot)
	}
}

// TestViewExposesRaiseBounds pins the two fields the Mini App sizes a raise
// from. The client cannot derive min_raise itself: it starts at BigBlind but
// widens after a raise, so a client guessing "+BigBlind" builds amounts the
// engine rejects with ErrRaiseTooLow — which is invisible to the player
// because the raise control simply appears not to work.
func TestViewExposesRaiseBounds(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 10000)
	_ = tbl.Sit("u2", "B", 10000)
	_ = tbl.Sit("u3", "C", 10000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	v := tbl.ViewFor("u1")
	if v.HighBet != BigBlind {
		t.Errorf("preflop high_bet = %d, want %d", v.HighBet, BigBlind)
	}
	if v.MinRaise != BigBlind {
		t.Errorf("preflop min_raise = %d, want %d", v.MinRaise, BigBlind)
	}

	// A raise to 500 over a high bet of 100 sets the next legal increment to
	// 400, so the smallest legal re-raise is 900 — not 600.
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, 500); err != nil {
		t.Fatalf("Act(raise 500): %v", err)
	}
	v = tbl.ViewFor("u1")
	if v.HighBet != 500 {
		t.Errorf("high_bet after raise = %d, want 500", v.HighBet)
	}
	if v.MinRaise != 400 {
		t.Errorf("min_raise after raise = %d, want 400", v.MinRaise)
	}

	// The bound the client computes from these two must be accepted by the
	// engine — this is the whole point of shipping them.
	next := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(next, ActRaise, v.HighBet+v.MinRaise); err != nil {
		t.Errorf("engine rejected the client's minimum re-raise %d: %v", v.HighBet+v.MinRaise, err)
	}
}

// TestViewReportsHandResultAtShowdown covers the number the win banner shows.
// It must be the seat's NET result for the hand, so a player who won a 300
// pot after putting in 100 sees +200, not +300.
func TestViewReportsHandResultAtShowdown(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 10000)
	_ = tbl.Sit("u2", "B", 10000)
	_ = tbl.StartHand()

	before := map[string]int{}
	for _, s := range tbl.Seats {
		before[s.UserID] = s.Stack
	}

	// Not showdown yet: nothing to report.
	for _, sv := range tbl.ViewFor("u1").Seats {
		if sv.Won != 0 {
			t.Errorf("seat %s reported won=%d before showdown", sv.UserID, sv.Won)
		}
	}

	tbl.Seats[0].Hole = cards("Ah", "Ad")
	tbl.Seats[1].Hole = cards("2c", "7d")
	tbl.Board = cards("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = StageRiver
	deltas := tbl.Showdown()

	v := tbl.ViewFor("u1")
	sum := 0
	for _, sv := range v.Seats {
		if got, want := sv.Won, deltas[sv.UserID]; got != want {
			t.Errorf("seat %s won = %d, want %d (settlement delta)", sv.UserID, got, want)
		}
		sum += sv.Won
	}
	if sum != 0 {
		t.Errorf("reported results sum to %d, want 0 — the banner would invent chips", sum)
	}
	if v.Seats[0].Won <= 0 {
		t.Errorf("aces full winner reported won=%d, want > 0", v.Seats[0].Won)
	}
}
