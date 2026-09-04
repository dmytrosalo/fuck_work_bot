package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func avatarReq(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/poker/t/avatar", strings.NewReader(body))
}

func TestAvatarIsStoredAndShownOnTheSeat(t *testing.T) {
	h, db := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	rec := httptest.NewRecorder()
	h.handleAvatar(rec, avatarReq(`{"idx":4}`), tbl, 42)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}

	var env tableEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Seats[env.YouSeat].Avatar != 4 {
		t.Errorf("seat avatar = %d, want 4", env.Seats[env.YouSeat].Avatar)
	}
	if got := db.GetPokerAvatar("42"); got != 4 {
		t.Errorf("stored avatar = %d, want 4", got)
	}
}

// The pool is the client's business, but the BOUND is not: an index outside
// it would be stored and render as an empty circle for everyone.
func TestAvatarRejectsOutOfRangeIndex(t *testing.T) {
	h, db := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	for _, body := range []string{`{"idx":-1}`, `{"idx":10}`, `{"idx":900}`} {
		rec := httptest.NewRecorder()
		h.handleAvatar(rec, avatarReq(body), tbl, 42)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, rec.Code)
		}
	}
	if got := db.GetPokerAvatar("42"); got != 0 {
		t.Errorf("a rejected index was stored anyway: %d", got)
	}
}

// The choice follows the player rather than the seat, so standing up and
// sitting down again keeps it.
func TestAvatarSurvivesRebuy(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	rec := httptest.NewRecorder()
	h.handleAvatar(rec, avatarReq(`{"idx":7}`), tbl, 42)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d", rec.Code)
	}

	// Bust and buy back in.
	tbl.Lock()
	tbl.Seats[tbl.SeatIndexOf("42")].Stack = 0
	tbl.Stage = 0
	tbl.Unlock()
	join(h, tbl.ID, `{}`)
	join(h, tbl.ID, `{"buy_in":2000}`)

	tbl.Lock()
	defer tbl.Unlock()
	if got := tbl.Seats[tbl.SeatIndexOf("42")].Avatar; got != 7 {
		t.Errorf("avatar after re-buy = %d, want the 7 they chose", got)
	}
}
