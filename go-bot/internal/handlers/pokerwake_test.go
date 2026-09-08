package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

func wake(h *PokerHub, tbl *poker.Table, uid int64) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.handleWake(rec, tbl, uid)
	return rec
}

// The wake endpoint clears the Asleep flag on the caller's own seat and
// broadcasts the change, so the client's button actually does something.
func TestWakeEndpointClearsAsleepFlag(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	tbl.Lock()
	idx := tbl.SeatIndexOf("42")
	if idx < 0 {
		tbl.Unlock()
		t.Fatal("setup: player not seated")
	}
	tbl.Seats[idx].Asleep = true
	tbl.Unlock()

	rec := wake(h, tbl, 42)
	if rec.Code != http.StatusOK {
		t.Fatalf("wake = %d (%s)", rec.Code, rec.Body.String())
	}

	tbl.Lock()
	asleep := tbl.Seats[idx].Asleep
	tbl.Unlock()
	if asleep {
		t.Error("seat still asleep after calling wake")
	}

	var env tableEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Seats[env.YouSeat].Asleep {
		t.Error("response view still reports the seat as asleep")
	}
}

// Waking a seat that was never asleep, or a user with no seat at all, is a
// harmless no-op rather than an error — matching handleLeave's idempotence.
func TestWakeEndpointIsIdempotent(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	if rec := wake(h, tbl, 42); rec.Code != http.StatusOK {
		t.Errorf("wake on an awake seat = %d, want 200", rec.Code)
	}
	if rec := wake(h, tbl, 999); rec.Code != http.StatusOK {
		t.Errorf("wake by a never-seated user = %d, want 200", rec.Code)
	}
}
