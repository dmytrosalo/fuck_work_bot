package poker

import (
	"strings"
	"time"
)

// SeatSnapshot is the part of a seat that must outlive a restart.
type SeatSnapshot struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Stack  int    `json:"stack"`
}

// TableSnapshot is a table reduced to what a restart has to preserve.
//
// Deliberately small. Everything to do with a hand IN PROGRESS — deck, hole
// cards, board, deadlines, per-street betting state — is left out, because
// a restored table starts a fresh hand rather than resuming the old one.
// Resuming would mean rebuilding all of that exactly right, and any mistake
// there mints or destroys chips; voiding costs nobody anything, since
// settlement is per hand and an unfinished hand has touched no balance.
//
// What must survive is the money and the seating: who is at the table, with
// how many chips, plus the identity and clocks that the blind schedule and
// the session display are computed from.
type TableSnapshot struct {
	ID        string         `json:"id"`
	ChatID    int64          `json:"chat_id"`
	CreatedAt time.Time      `json:"created_at"`
	Hands     int            `json:"hands"`
	Button    int            `json:"button"`
	Seats     []SeatSnapshot `json:"seats"`
}

// Snapshot captures the table for persistence. Callers must hold the lock.
//
// A seat's Committed chips are folded back into its Stack: they are sitting
// in a pot that will never be awarded, and they belong to the player who
// put them there. This is what makes voiding the hand chip-neutral.
func (t *Table) Snapshot() TableSnapshot {
	seats := make([]SeatSnapshot, 0, len(t.Seats))
	for _, s := range t.Seats {
		seats = append(seats, SeatSnapshot{
			UserID: s.UserID,
			Name:   s.Name,
			Stack:  s.Stack + s.Committed,
		})
	}
	return TableSnapshot{
		ID: t.ID, ChatID: t.ChatID, CreatedAt: t.CreatedAt,
		Hands: t.Hands, Button: t.Button, Seats: seats,
	}
}

// RestoreTable rebuilds a table from a snapshot, waiting for a fresh hand.
//
// Bots are dropped on purpose: they are reseated by the hub's own seating
// rule, which reads the CURRENT human population, and carrying over stale
// bot seats would fight that rule and could leave a table of bots with no
// humans left in it.
func RestoreTable(snap TableSnapshot) *Table {
	t := &Table{
		ID: snap.ID, ChatID: snap.ChatID,
		Stage:      StageWaiting,
		Button:     snap.Button,
		ToAct:      -1,
		CreatedAt:  snap.CreatedAt,
		Hands:      snap.Hands,
		SmallBlind: SmallBlind, BigBlind: BigBlind,
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	for _, s := range snap.Seats {
		if isBotSeat(s.UserID) || s.Stack <= 0 {
			continue
		}
		t.Seats = append(t.Seats, &Seat{UserID: s.UserID, Name: s.Name, Stack: s.Stack})
	}
	// Button must stay a valid index into the shortened slice, or the first
	// hand after a restart panics on nextOccupied.
	if t.Button >= len(t.Seats) {
		t.Button = len(t.Seats) - 1
	}
	return t
}

// isBotSeat mirrors the hub's bot check without importing it — the engine
// owns BotUserPrefix, so the test belongs here too.
func isBotSeat(userID string) bool {
	return strings.HasPrefix(userID, BotUserPrefix)
}
