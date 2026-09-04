package handlers

import (
	"net/http"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// historyDepth is how many finished hands a table remembers. Short on
// purpose: this is a "what just happened" list, not a database, and it is
// served on demand rather than riding every state broadcast.
const historyDepth = 10

// handResult is one finished hand, as it looked at the end.
type handResult struct {
	Hand    int      `json:"hand"`
	Board   []string `json:"board"`
	Winners []string `json:"winners"`
	// Combo is empty when the pot was taken without a showdown — nobody
	// revealed anything, so naming a hand would expose a holding nobody
	// paid to see, the same rule the showdown line follows.
	Combo string `json:"combo"`
	Pot   int    `json:"pot"`
	At    int64  `json:"at"`
}

// recordHistory appends the hand that just settled. Caller must hold the
// table lock and not h.mu.
func (h *PokerHub) recordHistory(tbl *poker.Table) {
	if len(tbl.LastWinners) == 0 {
		return // nothing was decided: not a hand worth listing
	}
	entry := handResult{
		Hand:    tbl.Hands,
		Board:   append([]string(nil), tbl.BoardStrings()...),
		Winners: append([]string(nil), tbl.LastWinners...),
		Combo:   tbl.LastHandName,
		Pot:     tbl.LastPot,
		At:      time.Now().Unix(),
	}

	h.mu.Lock()
	list := append(h.history[tbl.ID], entry)
	if len(list) > historyDepth {
		list = list[len(list)-historyDepth:]
	}
	h.history[tbl.ID] = list
	h.mu.Unlock()
}

// handleHistory serves the table's recent hands, newest first. Behind the
// same table authorization as every other /api/poker route, so it cannot be
// used to read a table you are not a member of.
func (h *PokerHub) handleHistory(w http.ResponseWriter, tbl *poker.Table) {
	h.mu.Lock()
	src := h.history[tbl.ID]
	out := make([]handResult, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		out = append(out, src[i])
	}
	h.mu.Unlock()
	writeJSON(w, out)
}
