package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
)

func buyInHub(t *testing.T, balance int) (*PokerHub, *storage.DB) {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "buyin.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.UpdateBalance("42", "Dmytro", balance-db.GetBalance("42", "Dmytro"))
	return NewPokerHub(db, nil, "tok"), db
}

func join(h *PokerHub, tblID string, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/poker/"+tblID+"/join", strings.NewReader(body))
	h.handleJoin(rec, req, h.table(tblID), 42, "Dmytro", "")
	return rec
}

// A join with no amount is a question, not a request to sit.
func TestJoinWithoutAmountOffersTheRange(t *testing.T) {
	h, _ := buyInHub(t, 47320)
	tbl := h.Create(-1)

	rec := join(h, tbl.ID, `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["choose_buy_in"] != true {
		t.Fatalf("response did not offer a choice: %v", got)
	}
	if got["balance"].(float64) != 47320 {
		t.Errorf("balance = %v, want 47320", got["balance"])
	}
	if got["max"].(float64) != 10000 {
		t.Errorf("max = %v, want it capped at MaxBuyIn", got["max"])
	}
	if got["min"].(float64) != 1000 {
		t.Errorf("min = %v, want MinBuyIn", got["min"])
	}

	// Asking must not seat anyone...
	tbl.Lock()
	seats := len(tbl.Seats)
	tbl.Unlock()
	if seats != 0 {
		t.Errorf("asking for options seated %d players", seats)
	}
	// ...nor leave a hub-wide claim behind, which would lock this player
	// out of every table until the idle reclaim.
	h.mu.Lock()
	claim, held := h.seatedAt["42"]
	h.mu.Unlock()
	if held {
		t.Errorf("asking for options left a seat claim on %s", claim)
	}
}

func TestJoinSeatsWithTheChosenAmount(t *testing.T) {
	h, _ := buyInHub(t, 47320)
	tbl := h.Create(-1)

	if rec := join(h, tbl.ID, `{"buy_in":2500}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	tbl.Lock()
	defer tbl.Unlock()
	if len(tbl.Seats) == 0 {
		t.Fatal("not seated")
	}
	if got := tbl.Seats[0].Stack; got != 2500 {
		t.Errorf("stack = %d, want the 2500 that was chosen", got)
	}
}

// The buttons are a convenience; the server is the check. A crafted request
// naming more than the player owns must be clamped, not honoured.
func TestJoinClampsAmountToBalance(t *testing.T) {
	h, _ := buyInHub(t, 3000)
	tbl := h.Create(-1)

	if rec := join(h, tbl.ID, `{"buy_in":999999}`); rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	tbl.Lock()
	defer tbl.Unlock()
	if got := tbl.Seats[0].Stack; got != 3000 {
		t.Errorf("stack = %d, want it clamped to the 3000 balance", got)
	}
}

func TestJoinClampsAmountToMaxBuyIn(t *testing.T) {
	h, _ := buyInHub(t, 500000)
	tbl := h.Create(-1)

	join(h, tbl.ID, `{"buy_in":500000}`)
	tbl.Lock()
	defer tbl.Unlock()
	if got := tbl.Seats[0].Stack; got != 10000 {
		t.Errorf("stack = %d, want it capped at MaxBuyIn", got)
	}
}

func TestJoinRejectsAmountBelowMinimum(t *testing.T) {
	h, _ := buyInHub(t, 47320)
	tbl := h.Create(-1)

	rec := join(h, tbl.ID, `{"buy_in":100}`)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 for a buy-in under MinBuyIn", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "1000") {
		t.Errorf("error does not name the minimum: %q", rec.Body.String())
	}
}

// Too poor to sit at all: the range comes back with max below min so the
// client can say so instead of offering buttons that would all fail.
func TestJoinOffersNothingWhenTooPoor(t *testing.T) {
	h, _ := buyInHub(t, 250)
	tbl := h.Create(-1)

	var got map[string]any
	_ = json.Unmarshal(join(h, tbl.ID, `{}`).Body.Bytes(), &got)
	if got["max"].(float64) >= got["min"].(float64) {
		t.Errorf("max %v is not below min %v for a 250 balance", got["max"], got["min"])
	}
}

// TestBustedPlayerCanBuyBackIn covers the dead end a busted player hit:
// settlement leaves their seat in place with zero chips, StartHand only
// deals to seats that have a stack, and handleJoin's reconnect fast path
// short-circuits before any buy-in — so reopening the app returned them to
// a seat they could never be dealt into again.
func TestBustedPlayerCanBuyBackIn(t *testing.T) {
	h, _ := buyInHub(t, 47320)
	tbl := h.Create(-1)

	join(h, tbl.ID, `{"buy_in":2500}`)
	tbl.Lock()
	tbl.Seats[tbl.SeatIndexOf("42")].Stack = 0 // lost it all
	tbl.Stage = 0                              // hand over
	tbl.Unlock()

	// Reopening must now offer a fresh buy-in instead of silently handing
	// back the dead seat.
	rec := join(h, tbl.ID, `{}`)
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["choose_buy_in"] != true {
		t.Fatalf("busted player was not offered a re-buy: %v", got)
	}

	// And taking it must actually seat them with chips.
	if rec := join(h, tbl.ID, `{"buy_in":1000}`); rec.Code != http.StatusOK {
		t.Fatalf("re-buy status = %d (%s)", rec.Code, rec.Body.String())
	}
	tbl.Lock()
	defer tbl.Unlock()
	idx := tbl.SeatIndexOf("42")
	if idx < 0 {
		t.Fatal("not re-seated")
	}
	if got := tbl.Seats[idx].Stack; got != 1000 {
		t.Errorf("stack after re-buy = %d, want 1000", got)
	}
}

// A player who is all-in has zero chips but is still contesting the pot —
// reopening then is a reconnect, not a bust, and must NOT tear their seat
// out from under a live hand.
func TestAllInPlayerReconnectsRatherThanRebuying(t *testing.T) {
	h, _ := buyInHub(t, 47320)
	tbl := h.Create(-1)

	join(h, tbl.ID, `{"buy_in":2500}`)
	tbl.Lock()
	idx := tbl.SeatIndexOf("42")
	tbl.Seats[idx].Stack = 0
	tbl.Seats[idx].InHand = true
	tbl.Seats[idx].Folded = false
	tbl.Stage = 2 // a live betting stage
	tbl.Unlock()

	rec := join(h, tbl.ID, `{}`)
	if strings.Contains(rec.Body.String(), "choose_buy_in") {
		t.Error("all-in player was offered a re-buy mid-hand")
	}
	tbl.Lock()
	defer tbl.Unlock()
	if tbl.SeatIndexOf("42") < 0 {
		t.Error("all-in player was stood up mid-hand, deleting chips from a live pot")
	}
}
