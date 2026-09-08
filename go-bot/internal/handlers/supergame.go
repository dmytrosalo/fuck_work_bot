package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
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
//
// It also draws which game the table gets -- dice or colour, 50/50 -- and
// stores that on the offer itself. The design is deliberate: "which game you
// get is not your choice" (see the spec), so the draw happens here, at offer
// time, using the same injectable-RNG pattern as the chance gate below, and
// is published as part of the offer rather than withheld until resolution.
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

	draw := h.superGameRoll
	if draw == nil {
		draw = rand.Float64
	}
	game := "dice"
	if draw() < 0.5 {
		game = "color"
	}

	h.mu.Lock()
	h.superLast[user] = time.Now()
	h.super[tbl.ID] = &superGame{
		UserID:   user,
		Name:     name,
		Stake:    stake,
		State:    "offered",
		Game:     game,
		Deadline: time.Now().Add(superDecideWindow),
	}
	h.mu.Unlock()
}

// errSuperUnavailable is returned by resolveSuper whenever there is nothing
// left to resolve: no pending game, the wrong player, an expired offer, or
// a game that has already been resolved. It doubles as the idempotency
// guard's refusal and as the HTTP handler's "already gone" response, so its
// Ukrainian text is what a double-tap or a retried request sees.
var errSuperUnavailable = errors.New("Супер гра недоступна")

// errSuperBadChoice is returned by resolveSuper when choice does not belong
// to the game the server already drew at offer time (e.g. "red" against a
// dice game, or "roll" against a colour game). Unlike errSuperUnavailable,
// this must NOT consume the game: the offer is left exactly as it was, still
// "offered" and still resolvable, so a client bug or a stray request cannot
// burn the player's one shot at it.
var errSuperBadChoice = errors.New("Такого вибору немає в цій грі")

// resolveSuper plays out tableID's pending game and applies the money. The
// game itself is never an input here -- it was fixed by offerSuper -- only
// the player's choice within it: "roll" or "skip" for dice, "red"/"black"/
// "skip" for colour. A choice that does not belong to the drawn game is
// refused via errSuperBadChoice without touching g at all, so the offer
// stays live for a corrected retry.
//
// The transition out of "offered" happens under h.mu BEFORE anything is
// rolled or written, so a double-tap or a retried request finds a state
// that is no longer offered and is refused. Unlike a design that releases
// the lock before rolling, every mutation of g -- state, the roll itself,
// the outcome, and the delta -- happens while h.mu is still held. rand is
// mutex-protected internally and does no I/O, so holding the lock across it
// is cheap, and it keeps the *superGame that other goroutines can reach
// through superFor always in a self-consistent state: nobody can ever
// observe a game whose State is "resolved" but whose Outcome or Delta is
// still zero-valued from a half-finished write. Only the database call --
// the one thing that can genuinely be slow -- happens after the lock is
// released, using values copied out under the lock.
func (h *PokerHub) resolveSuper(tableID, userID, choice string) (*superGame, error) {
	h.mu.Lock()
	g, ok := h.super[tableID]
	if !ok || g.State != "offered" || g.UserID != userID || time.Now().After(g.Deadline) {
		h.mu.Unlock()
		return nil, errSuperUnavailable
	}

	var pick string
	switch g.Game {
	case "dice":
		if choice != "roll" && choice != "skip" {
			h.mu.Unlock()
			return nil, errSuperBadChoice
		}
	case "color":
		switch choice {
		case "red", "black":
			pick = choice
		case "skip":
		default:
			h.mu.Unlock()
			return nil, errSuperBadChoice
		}
	default:
		// offerSuper always sets Game to "dice" or "color"; anything else
		// means the offer is malformed and there is nothing safe to resolve.
		h.mu.Unlock()
		return nil, errSuperBadChoice
	}

	g.State = "resolved"
	g.Pick = pick
	g.Deadline = time.Now().Add(superResultHold)

	if choice == "skip" {
		// A deliberate pass, not a keep. superDelta has no "skip" branch so
		// this moves no money, but it stays distinguishable from a rolled
		// 8 in the audit log and in the UI -- there is no "skipped" State,
		// only a resolved game whose Outcome records that the player
		// declined to play.
		g.Outcome = "skip"
	} else if g.Game == "dice" {
		a, b := rand.Intn(6)+1, rand.Intn(6)+1
		g.Dice = [2]int{a, b}
		g.Outcome = diceOutcome(a, b)
	} else {
		red := rand.Intn(2) == 0
		suits := map[bool][]string{true: {"♥", "♦"}, false: {"♠", "♣"}}[red]
		ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
		g.Card = ranks[rand.Intn(len(ranks))] + suits[rand.Intn(len(suits))]
		g.Outcome = colorOutcome(red, pick)
	}
	g.Delta = superDelta(g.Stake, g.Outcome)

	cp := *g
	h.mu.Unlock()

	// paidOut tracks whether cp.Delta was actually written to the ledger,
	// as opposed to merely computed. It gates the AdjustStack mirror below:
	// a Delta that was never persisted (h.db == nil, test-only) or that
	// SettlePoker rolled back must move no chips either, same as it moved
	// no balance.
	paidOut := false
	if cp.Delta != 0 && h.db != nil {
		// Two parties, one transaction, exactly like the bot-chip entries
		// in settle(): the player's gain is the bank's loss and the pair
		// sums to zero.
		if err := h.db.SettlePoker([]storage.PokerDelta{
			{UserID: cp.UserID, Name: cp.Name, Amount: cp.Delta},
			{UserID: bankUserID, Name: "Банк", Amount: -cp.Delta},
		}, "supergame"); err != nil {
			// SettlePoker's transaction rolled back, so no money moved --
			// but g is already State "resolved" with the winning Outcome
			// and Delta, and that is what superFor and the broadcast are
			// about to hand every client at the table. Left alone, the
			// table would render "doubled!" for a payout that never
			// happened. Downgrade the stored game to a no-gain result
			// under the same lock discipline as the rest of this function
			// so what gets published matches what actually moved in the
			// ledger: nothing. Re-opening State to "offered" is not an
			// option -- that would reopen the double-resolve window the
			// idempotency guard above exists to close.
			log.Printf("[poker] SUPER GAME PAYOUT FAILED table=%s user=%s amount=%d NOT credited (outcome %q downgraded to skip): %v",
				tableID, cp.UserID, cp.Delta, cp.Outcome, err)

			h.mu.Lock()
			g.Outcome = "skip"
			g.Delta = 0
			cp = *g
			h.mu.Unlock()

			return &cp, fmt.Errorf("супер гра payout failed: %w", err)
		}
		paidOut = true
	}

	if paidOut {
		// SettlePoker only ever moves богдудіки, not chips: the win it
		// just wrote for the player was created against bank:house, not
		// inside the poker engine, so nothing about a normal hand's
		// settlement touched this seat's stack. OnBalanceChange does not
		// fire here either -- it is UpdateBalance's hook, and SettlePoker
		// deliberately bypasses it (see the doc comment on
		// OnBalanceChange/AdjustStack) because for an ordinary poker hand
		// the chips already moved inside the engine and firing it would
		// double-count. Супер гра has no such double-count risk -- its
		// winnings never touched the engine -- so it is the one caller
		// that must mirror the delta onto the felt itself, explicitly,
		// exactly the way the hook would have if it fired. Called with
		// neither h.mu nor the table lock held (h.mu was released above,
		// and the payout-failure branch above always re-locks/unlocks and
		// returns before reaching here), matching AdjustStack's own
		// locking contract. A no-longer-seated player (they left between
		// the offer and the resolve) is a no-op inside AdjustStack: only
		// the balance change above applies to them.
		h.AdjustStack(cp.UserID, cp.Delta)
	}

	return &cp, nil
}

// handleSuper is the HTTP action for resolving a pending Супер гра: the
// player's choice arrives here, resolveSuper does the roll and the payout,
// and the result reaches every client -- including the player who just
// acted -- through the normal broadcast rather than the response body.
//
// The game itself is never in the request: it was fixed by the server at
// offer time (see offerSuper), so the wire only ever carries the player's
// choice within that game.
func (h *PokerHub) handleSuper(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	var body struct {
		Choice string `json:"choice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Некоректний запит", http.StatusBadRequest)
		return
	}
	switch body.Choice {
	case "roll", "red", "black", "skip":
	default:
		http.Error(w, "Невідомий вибір", http.StatusBadRequest)
		return
	}
	_, err := h.resolveSuper(tbl.ID, fmt.Sprintf("%d", uid), body.Choice)
	switch {
	case errors.Is(err, errSuperBadChoice):
		http.Error(w, "Такого вибору немає в цій грі", http.StatusBadRequest)
		return
	case errors.Is(err, errSuperUnavailable):
		http.Error(w, "Супер гра вже завершена", http.StatusConflict)
		return
	case err != nil:
		// The game was resolved and downgraded to a no-gain result (see
		// resolveSuper), so the table still needs the broadcast -- without
		// it every other client is stuck looking at a stale "offered"
		// panel. Only the HTTP response tells the acting player their
		// payout did not go through.
		h.broadcast(tbl)
		http.Error(w, "Не вдалося зарахувати виграш", http.StatusInternalServerError)
		return
	}
	h.broadcast(tbl)
	w.WriteHeader(http.StatusNoContent)
}

// superView is the wire form of a pending game. It hangs off tableEnvelope
// rather than poker.TableView so internal/poker stays untouched, and it is
// identical for every viewer -- the whole point is that the table watches
// one result together.
type superView struct {
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Stake   int    `json:"stake"`
	State   string `json:"state"`
	Game    string `json:"game,omitempty"`
	Pick    string `json:"pick,omitempty"`
	Dice    []int  `json:"dice,omitempty"`
	Card    string `json:"card,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Delta   int    `json:"delta,omitempty"`
	Left    int    `json:"left"` // seconds remaining on the current phase
}

// superView builds tableID's published game state. Takes h.mu itself, so
// callers must NOT already hold it -- inside broadcast(), which does, use
// superViewLocked instead. Matches the chatSnapshot/chatLocked split in
// pokerchat.go for the same reason: sync.Mutex is not reentrant.
func (h *PokerHub) superView(tableID string) *superView {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.superViewLocked(tableID)
}

// superViewLocked builds tableID's published game state. Caller must hold
// h.mu.
//
// Returns nil once g.Deadline has passed, whatever State says. Nothing else
// removes a finished or timed-out game from h.super, so without this check
// a resolved game (or an offer nobody acted on) would keep being published
// forever and the client's panel would never disappear. The deadline is
// exactly the moment superHolds releases the table, so this makes the panel
// vanish in lockstep with the hold -- one rule, two consumers.
func (h *PokerHub) superViewLocked(tableID string) *superView {
	g, ok := h.super[tableID]
	if !ok || !time.Now().Before(g.Deadline) {
		return nil
	}
	left := int(time.Until(g.Deadline).Seconds())
	if left < 0 {
		left = 0
	}
	v := &superView{
		UserID: g.UserID, Name: g.Name, Stake: g.Stake, State: g.State,
		Game: g.Game, Pick: g.Pick, Card: g.Card,
		Outcome: g.Outcome, Delta: g.Delta, Left: left,
	}
	if g.Dice[0] != 0 {
		v.Dice = []int{g.Dice[0], g.Dice[1]}
	}
	return v
}
