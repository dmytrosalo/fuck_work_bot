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
