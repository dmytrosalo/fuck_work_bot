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
