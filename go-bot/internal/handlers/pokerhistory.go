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

// handPlayer is one seat's part in a finished hand.
//
// Hole cards are listed for EVERY player who was dealt in, including those
// who folded and never showed. Real sites never reveal a mucked hand; this
// deliberately does, because seeing whether someone was bluffing is the
// point of a history among four friends. It is only ever the FINISHED hand
// — nothing here can inform a hand still being played.
type handPlayer struct {
	Name  string   `json:"name"`
	Hole  []string `json:"hole"`
	Delta int      `json:"delta"`
	// Combo is that player's best hand, present only when there was a
	// board to make one from.
	Combo  string `json:"combo,omitempty"`
	Folded bool   `json:"folded"`
	Won    bool   `json:"won"`
}

// handResult is one finished hand, as it looked at the end.
type handResult struct {
	Hand    int      `json:"hand"`
	Board   []string `json:"board"`
	Winners []string `json:"winners"`
	// Combo is empty when the pot was taken without a showdown — nobody
	// revealed anything, so naming a hand would expose a holding nobody
	// paid to see, the same rule the showdown line follows.
	Combo   string       `json:"combo"`
	Pot     int          `json:"pot"`
	At      int64        `json:"at"`
	Players []handPlayer `json:"players"`
}

// recordHistory appends the hand that just settled. Caller must hold the
// table lock and not h.mu.
func (h *PokerHub) recordHistory(tbl *poker.Table, deltas map[string]int) {
	if len(tbl.LastWinners) == 0 {
		return // nothing was decided: not a hand worth listing
	}

	won := map[string]bool{}
	for _, name := range tbl.LastWinners {
		won[name] = true
	}
	players := make([]handPlayer, 0, len(tbl.Seats))
	for _, s := range tbl.Seats {
		if !s.InHand {
			continue // sat down mid-hand: not part of this one
		}
		p := handPlayer{
			Name:   s.Name,
			Hole:   s.HoleStrings(),
			Delta:  deltas[s.UserID],
			Folded: s.Folded,
			Won:    won[s.Name],
		}
		// Naming a combination needs a board; a hand that ended preflop
		// has none, and "старша карта" there would be meaningless.
		if len(tbl.Board) >= 3 {
			p.Combo, _ = poker.HandName(s.Hole, tbl.Board)
		}
		players = append(players, p)
	}

	entry := handResult{
		Hand:    tbl.Hands,
		Board:   append([]string(nil), tbl.BoardStrings()...),
		Winners: append([]string(nil), tbl.LastWinners...),
		Combo:   tbl.LastHandName,
		Pot:     tbl.LastPot,
		At:      time.Now().Unix(),
		Players: players,
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
