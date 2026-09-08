package handlers

import (
	"math/rand"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// Супер гра is a double-or-nothing on a poker win. See
// docs/superpowers/specs/2026-09-08-super-game-design.md for the maths
// behind every constant here.
const (
	// superChance is how often a qualifying win is offered a game, and
	// superCooldown is the minimum gap between two games for one player.
	// Same division of labour as tauntChance/tauntCooldown in pokerbots.go:
	// the probability is for variety, the cooldown is what stops a burst.
	superChance   = 0.15
	superCooldown = 10 * time.Minute
	// superMinBlinds keeps this an event. Without a floor it would fire on
	// pots of two blinds and stop meaning anything. Relative to the blind
	// rather than absolute because the blinds double on a schedule.
	superMinBlinds = 10

	// The table is held for the sum of these two, so they are the worst-case
	// pause everyone else sits through.
	superDecideWindow = 10 * time.Second
	superResultHold   = 4 * time.Second
)

// diceOutcome scores 2d6. Over 36 rolls this is 10 doubles, 5 keeps and 21
// halves, which is 35.5/36 of the stake returned -- a 1.4% edge to the bank.
func diceOutcome(a, b int) string {
	switch sum := a + b; {
	case sum >= 9:
		return "double"
	case sum == 8:
		return "keep"
	default:
		return "half"
	}
}

// colorOutcome scores a single card draw. A 26/26 deck makes this exactly
// fair, which is why it has no "keep" branch to soften it.
func colorOutcome(cardIsRed bool, pick string) string {
	if (pick == "red") == cardIsRed {
		return "double"
	}
	return "bust"
}

// superDelta is the signed balance change for an outcome. The half branch
// truncates, so the player keeps stake/2 and an odd stake loses the extra
// coin to the bank.
func superDelta(stake int, outcome string) int {
	switch outcome {
	case "double":
		return stake
	case "keep":
		return 0
	case "half":
		return stake/2 - stake
	case "bust":
		return -stake
	default:
		// An unrecognised outcome must never move money. Failing dangerous
		// here (e.g. treating it like "bust") would silently take the
		// player's entire win on a typo or a future outcome we forgot to
		// handle, so the safe default is a no-op.
		return 0
	}
}

// superCandidate finds the single human who won this hand, if there is
// exactly one and their win clears superMinBlinds.
//
// A split pot -- more than one seat with a positive delta -- offers no game
// at all, bots included. That is stricter than picking the largest winner,
// and deliberately so: it means there is no tie-breaking rule to get wrong
// and the table can never be held twice for one hand.
func superCandidate(deltas map[string]int, bigBlind int) (string, int, bool) {
	winners := 0
	user, stake := "", 0
	for id, d := range deltas {
		if d <= 0 {
			continue
		}
		winners++
		user, stake = id, d
	}
	if winners != 1 || isBotUser(user) {
		return "", 0, false
	}
	if stake < superMinBlinds*bigBlind {
		return "", 0, false
	}
	return user, stake, true
}

// superGame is one pending Супер гра. It lives on the hub rather than on
// poker.Table: the stage machine is the money-critical part, and a hold in
// the handler layer cannot corrupt a hand.
type superGame struct {
	UserID   string
	Name     string
	Stake    int
	State    string // "offered" | "resolved"
	Game     string // "dice" | "color"
	Pick     string // "red" | "black", colour game only
	Outcome  string // "double" | "keep" | "half" | "bust"
	Card     string
	Dice     [2]int
	Delta    int
	Deadline time.Time
}

func (h *PokerHub) superFor(tableID string) *superGame {
	h.mu.Lock()
	defer h.mu.Unlock()
	g, ok := h.super[tableID]
	if !ok {
		return nil
	}
	cp := *g
	return &cp
}

func (h *PokerHub) setSuper(tableID string, g *superGame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g == nil {
		delete(h.super, tableID)
		return
	}
	h.super[tableID] = g
}

// superHolds reports whether tableID must not deal the next hand yet. An
// expired deadline releases the hold whatever the state, so a crashed or
// abandoned game can never wedge a table -- the same defence showdownReady
// already applies to a missing showdown timestamp.
func (h *PokerHub) superHolds(tableID string) bool {
	g := h.superFor(tableID)
	return g != nil && time.Now().Before(g.Deadline)
}

// offerSuper opens a Супер гра on tbl if this hand qualifies. Caller must
// hold tbl.Lock(), same as settle() itself.
func (h *PokerHub) offerSuper(tbl *poker.Table, deltas map[string]int) {
	user, stake, ok := superCandidate(deltas, tbl.BigBlind)
	if !ok {
		return
	}

	h.mu.Lock()
	last := h.superLast[user]
	h.mu.Unlock()
	if time.Since(last) < superCooldown {
		return
	}

	roll := h.superRoll
	if roll == nil {
		roll = rand.Float64
	}
	if roll() >= superChance {
		return
	}

	name := user
	for _, s := range tbl.Seats {
		if s.UserID == user {
			name = s.Name
			break
		}
	}

	h.mu.Lock()
	h.superLast[user] = time.Now()
	h.super[tbl.ID] = &superGame{
		UserID:   user,
		Name:     name,
		Stake:    stake,
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	}
	h.mu.Unlock()
}
