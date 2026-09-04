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
