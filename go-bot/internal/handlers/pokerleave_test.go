package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

func leave(h *PokerHub, tbl *poker.Table, uid int64) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.handleLeave(rec, tbl, uid)
	return rec
}

// Leaving frees the seat AND the hub-wide claim. Without the claim going
// too, the player would be told "Ти вже за іншим столом" everywhere else
// until the table went idle.
func TestLeaveFreesSeatAndClaim(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)
	endHand(t, h, tbl)

	if rec := leave(h, tbl, 42); rec.Code != http.StatusOK {
		t.Fatalf("leave = %d (%s)", rec.Code, rec.Body.String())
	}

	tbl.Lock()
	seated := tbl.SeatIndexOf("42") >= 0
	tbl.Unlock()
	if seated {
		t.Error("seat survived the leave — it still blocks everyone else")
	}
	h.mu.Lock()
	claim, held := h.seatedAt["42"]
	h.mu.Unlock()
	if held {
		t.Errorf("claim on %s survived — the player is locked out elsewhere", claim)
	}
}

// After leaving, the next open must offer a fresh buy-in rather than
// dropping them back into the seat they left.
func TestLeaveThenBuyInFromScratch(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)
	endHand(t, h, tbl)
	leave(h, tbl, 42)

	rec := join(h, tbl.ID, `{}`)
	if body := rec.Body.String(); !contains(body, "choose_buy_in") {
		t.Fatalf("re-open did not offer a buy-in: %s", body)
	}
	if rec := join(h, tbl.ID, `{"buy_in":1000}`); rec.Code != http.StatusOK {
		t.Fatalf("re-buy = %d (%s)", rec.Code, rec.Body.String())
	}
	tbl.Lock()
	defer tbl.Unlock()
	if got := tbl.Seats[tbl.SeatIndexOf("42")].Stack; got != 1000 {
		t.Errorf("stack = %d, want the freshly chosen 1000", got)
	}
}

// Asking to leave mid-hand is QUEUED, not refused: the next hand starts
// seconds after the last settles, so a player forced to catch the gap could
// effectively never leave. They stay seated until the hand ends.
func TestLeaveMidHandIsQueued(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)

	tbl.Lock()
	live := tbl.HasLiveStake("42")
	tbl.Unlock()
	if !live {
		t.Fatal("setup: joining did not put the player in a live hand")
	}

	rec := leave(h, tbl, 42)
	if rec.Code != http.StatusOK {
		t.Fatalf("leave mid-hand = %d, want 200 with a pending flag", rec.Code)
	}
	if !contains(rec.Body.String(), `"pending":true`) {
		t.Errorf("body = %s, want pending", rec.Body.String())
	}
	tbl.Lock()
	stillSeated := tbl.SeatIndexOf("42") >= 0
	tbl.Unlock()
	if !stillSeated {
		t.Error("player was stood up mid-hand, deleting chips from a live pot")
	}

	// Settling the hand must carry the request out.
	tbl.Lock()
	tbl.Board = cardsFor("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = poker.StageRiver
	h.settle(tbl)
	tbl.Unlock()

	tbl.Lock()
	gone := tbl.SeatIndexOf("42") < 0
	tbl.Unlock()
	if !gone {
		t.Error("queued leave was not carried out when the hand ended")
	}
	h.mu.Lock()
	_, held := h.seatedAt["42"]
	h.mu.Unlock()
	if held {
		t.Error("claim survived the queued leave")
	}
}

// Leaving twice, or leaving when never seated, is a no-op rather than an
// error — the button should not punish a double tap.
func TestLeaveIsIdempotent(t *testing.T) {
	h, _ := buyInHub(t, 20000)
	tbl := h.Create(-1)
	join(h, tbl.ID, `{"buy_in":5000}`)
	endHand(t, h, tbl)

	leave(h, tbl, 42)
	if rec := leave(h, tbl, 42); rec.Code != http.StatusOK {
		t.Errorf("second leave = %d, want 200", rec.Code)
	}
	if rec := leave(h, tbl, 999); rec.Code != http.StatusOK {
		t.Errorf("leave by a never-seated user = %d, want 200", rec.Code)
	}
}

// endHand settles whatever hand joining started, so a test can exercise the
// between-hands path.
func endHand(t *testing.T, h *PokerHub, tbl *poker.Table) {
	t.Helper()
	tbl.Lock()
	defer tbl.Unlock()
	if tbl.Stage == poker.StageWaiting || tbl.Stage == poker.StageShowdown {
		return
	}
	tbl.Board = cardsFor("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = poker.StageRiver
	h.settle(tbl)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
