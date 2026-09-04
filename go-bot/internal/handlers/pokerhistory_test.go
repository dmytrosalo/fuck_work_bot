package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	pk "github.com/chehsunliu/poker"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// cardsFor mirrors the poker package's own test helper: its version is
// test-only and unexported, and this does not warrant widening the engine's
// API just to reach it.
func cardsFor(ss ...string) []pk.Card {
	out := make([]pk.Card, 0, len(ss))
	for _, s := range ss {
		out = append(out, pk.NewCard(s))
	}
	return out
}

func playHand(t *testing.T, h *PokerHub, tbl *poker.Table, winnerHole, loserHole, board []string) {
	t.Helper()
	tbl.Lock()
	defer tbl.Unlock()
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	tbl.Seats[0].Hole = cardsFor(winnerHole...)
	tbl.Seats[1].Hole = cardsFor(loserHole...)
	tbl.Board = cardsFor(board...)
	tbl.Stage = poker.StageRiver
	h.settle(tbl)
}

func historyTable(t *testing.T) (*PokerHub, *poker.Table) {
	t.Helper()
	h := NewPokerHub(nil, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	_ = tbl.Sit("42", "Dmytro", 100000)
	_ = tbl.Sit("43", "Danya", 100000)
	tbl.Unlock()
	return h, tbl
}

func TestHistoryRecordsTheFinishedHand(t *testing.T) {
	h, tbl := historyTable(t)
	playHand(t, h, tbl,
		[]string{"Ah", "Ad"}, []string{"2c", "7d"},
		[]string{"As", "Kh", "Qd", "3c", "9s"})

	rec := httptest.NewRecorder()
	h.handleHistory(rec, tbl)
	var got []handResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("history has %d entries, want 1", len(got))
	}
	e := got[0]
	if len(e.Board) != 5 {
		t.Errorf("board = %v, want the full river", e.Board)
	}
	if len(e.Winners) != 1 || e.Winners[0] != "Dmytro" {
		t.Errorf("winners = %v, want [Dmytro]", e.Winners)
	}
	if e.Combo != "трійка" {
		t.Errorf("combo = %q, want трійка", e.Combo)
	}
	// The pot must be captured before settlement zeroes Committed, or it
	// reads as 0 for every hand ever played.
	if e.Pot <= 0 {
		t.Errorf("pot = %d, want the chips the hand was played for", e.Pot)
	}
}

// A pot won by everyone folding names no combination — the same disclosure
// rule the showdown line follows.
func TestHistoryOmitsComboForAnUncontestedPot(t *testing.T) {
	h, tbl := historyTable(t)
	tbl.Lock()
	_ = tbl.StartHand()
	tbl.Seats[0].Hole = cardsFor("Ah", "Ad")
	tbl.Seats[1].Hole = cardsFor("2c", "7d")
	tbl.Board = cardsFor("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = poker.StageRiver
	tbl.Seats[1].Folded = true
	h.settle(tbl)
	tbl.Unlock()

	rec := httptest.NewRecorder()
	h.handleHistory(rec, tbl)
	var got []handResult
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("history has %d entries", len(got))
	}
	if got[0].Combo != "" {
		t.Errorf("combo = %q for an uncontested pot — that leaks a hand nobody saw", got[0].Combo)
	}
}

// Newest first, and never more than historyDepth.
func TestHistoryIsCappedAndNewestFirst(t *testing.T) {
	h, tbl := historyTable(t)
	for i := 0; i < historyDepth+6; i++ {
		playHand(t, h, tbl,
			[]string{"Ah", "Ad"}, []string{"2c", "7d"},
			[]string{"As", "Kh", "Qd", "3c", "9s"})
	}

	rec := httptest.NewRecorder()
	h.handleHistory(rec, tbl)
	var got []handResult
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got) != historyDepth {
		t.Fatalf("history has %d entries, want it capped at %d", len(got), historyDepth)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Hand < got[i].Hand {
			t.Fatalf("entries are not newest-first: %d before %d", got[i-1].Hand, got[i].Hand)
		}
	}
}

func TestHistoryEmptyBeforeAnyHand(t *testing.T) {
	h, tbl := historyTable(t)
	rec := httptest.NewRecorder()
	h.handleHistory(rec, tbl)
	var got []handResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("history = %v, want empty", got)
	}
}
