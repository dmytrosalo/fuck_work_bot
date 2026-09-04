package poker

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	pk "github.com/chehsunliu/poker"
)

type Stage int

const (
	StageWaiting Stage = iota
	StagePreflop
	StageFlop
	StageTurn
	StageRiver
	StageShowdown
)

type Seat struct {
	UserID          string
	Name            string
	Stack           int // chips at the table
	Hole            []pk.Card
	Folded          bool
	AllIn           bool
	Committed       int // total committed THIS HAND — basis for side pots
	Bet             int // committed on the CURRENT street — basis for min-raise
	InHand          bool
	startStack      int // stack when the hand began, for settlement deltas
	actedThisStreet bool
}

type Pot struct {
	Amount   int
	Eligible []int // seat indices
}

type Table struct {
	ID       string
	ChatID   int64
	Seats    []*Seat
	Button   int
	Stage    Stage
	Board    []pk.Card
	Deck     []pk.Card
	ToAct    int
	MinRaise int
	Deadline time.Time
	Seq      uint64
	// CreatedAt anchors both the session clock and the blind schedule, so
	// what the player sees counting up is the same clock the blinds move
	// on. Set by NewTable; a zero value degrades to the base blind level
	// rather than to the cap (see BlindsAt).
	CreatedAt time.Time
	// Hands counts dealt hands, incremented by StartHand.
	Hands int
	// SmallBlind/BigBlind are the blinds for the hand in progress. Fixed at
	// StartHand from the elapsed time so a level change can never move the
	// stakes underneath a hand that is already being played.
	SmallBlind int
	BigBlind   int
	// LastWinners / LastHandName describe the hand just finished, for the
	// showdown line. LastHandName is empty when the pot was won without a
	// showdown — everyone else folded, so no hand was ever revealed and
	// naming one would expose a holding nobody paid to see.
	LastWinners  []string
	LastHandName string
	settled      bool // true after Showdown() to prevent re-settlement
	mu           sync.Mutex
}

var (
	ErrTableFull  = errors.New("стіл заповнений")
	ErrAlreadySat = errors.New("ти вже за столом")
	// ErrBuyInTooLow reaches the player verbatim as the join error, so it
	// names the threshold rather than making them guess it.
	ErrBuyInTooLow = fmt.Errorf("замало богдудіків — треба щонайменше %d 🪙", MinBuyIn)
	ErrNeedPlayers = errors.New("потрібно щонайменше 2 гравці")
	ErrNotABot     = errors.New("userID must start with " + BotUserPrefix)
)

func NewTable(id string, chatID int64) *Table {
	return &Table{
		ID: id, ChatID: chatID, Stage: StageWaiting, Button: -1,
		CreatedAt:  time.Now(),
		SmallBlind: SmallBlind, BigBlind: BigBlind,
	}
}

// Elapsed is how long this table has been running — the session clock, and
// the input to the blind schedule.
func (t *Table) Elapsed() time.Duration {
	if t.CreatedAt.IsZero() {
		return 0
	}
	return time.Since(t.CreatedAt)
}

func (t *Table) Sit(userID, name string, buyIn int) error {
	if buyIn < MinBuyIn {
		return ErrBuyInTooLow
	}
	if buyIn > MaxBuyIn {
		buyIn = MaxBuyIn
	}
	if len(t.Seats) >= MaxSeats {
		return ErrTableFull
	}
	for _, s := range t.Seats {
		if s.UserID == userID {
			return ErrAlreadySat
		}
	}
	t.Seats = append(t.Seats, &Seat{UserID: userID, Name: name, Stack: buyIn})
	t.Seq++
	return nil
}

// SitBot seats a bot with an exact stack, bypassing the human buy-in rules.
// Bots are funded by the house rather than a balance, so MinBuyIn/MaxBuyIn
// do not apply — a bot matches the top human's stack, which may exceed
// MaxBuyIn once that player has been winning. The userID must start with
// BotUserPrefix ("bot:") to prevent accidental seating of human players with
// bot privileges. Seat limits and the duplicate-user check still apply.
func (t *Table) SitBot(userID, name string, stack int) error {
	if !strings.HasPrefix(userID, BotUserPrefix) {
		return ErrNotABot
	}
	if len(t.Seats) >= MaxSeats {
		return ErrTableFull
	}
	for _, s := range t.Seats {
		if s.UserID == userID {
			return ErrAlreadySat
		}
	}
	t.Seats = append(t.Seats, &Seat{UserID: userID, Name: name, Stack: stack})
	t.Seq++
	return nil
}

// SeatIndexOf returns the seat index currently held by userID at this
// table, or -1 if userID has no seat here. Exposed so callers outside this
// package can distinguish "already seated" from "not seated" BEFORE
// deciding whether to call Sit at all, rather than depending on Sit's own
// error precedence (buy-in-too-low vs. table-full vs. already-sat) to infer
// it after the fact — see handleJoin's reconnect fast path.
func (t *Table) SeatIndexOf(userID string) int {
	for i, s := range t.Seats {
		if s.UserID == userID {
			return i
		}
	}
	return -1
}

func (t *Table) SeatedCount() int {
	n := 0
	for _, s := range t.Seats {
		if s.Stack > 0 {
			n++
		}
	}
	return n
}

// post moves up to amount from the seat's stack into the pot, marking the
// seat all-in when it cannot cover the full amount.
func (t *Table) post(s *Seat, amount int) {
	if amount <= 0 {
		return
	}
	if amount >= s.Stack {
		amount = s.Stack
		s.AllIn = true
	}
	s.Stack -= amount
	s.Bet += amount
	s.Committed += amount
}

func (t *Table) StartHand() error {
	if t.SeatedCount() < 2 {
		return ErrNeedPlayers
	}
	t.Deck = NewShuffledDeck()
	t.Board = nil
	t.Stage = StagePreflop
	// Blinds are locked in for the whole hand here, at the level the clock
	// says right now — never re-read mid-hand.
	t.LastWinners, t.LastHandName = nil, ""
	t.SmallBlind, t.BigBlind = BlindsAt(t.Elapsed())
	t.MinRaise = t.BigBlind
	t.Hands++
	t.settled = false

	for _, s := range t.Seats {
		s.Hole = nil
		s.Folded = s.Stack <= 0
		s.AllIn = false
		s.Committed = 0
		s.Bet = 0
		s.actedThisStreet = false
		s.InHand = s.Stack > 0
		s.startStack = s.Stack
	}

	t.Button = t.nextOccupied(t.Button)
	var sb, bb int
	if t.SeatedCount() == 2 {
		sb = t.Button
		bb = t.nextOccupied(t.Button)
	} else {
		sb = t.nextOccupied(t.Button)
		bb = t.nextOccupied(sb)
	}
	t.post(t.Seats[sb], t.SmallBlind)
	t.post(t.Seats[bb], t.BigBlind)

	for i := 0; i < 2; i++ {
		for _, s := range t.Seats {
			if s.InHand {
				s.Hole = append(s.Hole, t.draw())
			}
		}
	}

	t.ToAct = t.nextActive(bb)
	t.Deadline = time.Now().Add(TurnTimeout)
	t.Seq++
	return nil
}

func (t *Table) draw() pk.Card {
	c := t.Deck[0]
	t.Deck = t.Deck[1:]
	return c
}

// nextOccupied returns the next seat index with chips, wrapping around.
func (t *Table) nextOccupied(from int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (from + i%n + n) % n
		if t.Seats[idx].Stack > 0 || t.Seats[idx].InHand {
			return idx
		}
	}
	return 0
}

// nextActive returns the next seat still able to act.
func (t *Table) nextActive(from int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (from + i) % n
		s := t.Seats[idx]
		if s.InHand && !s.Folded && !s.AllIn {
			return idx
		}
	}
	return -1
}

// Lock guards the table for callers outside this package. The engine itself
// is lock-free so it stays testable; HTTP handlers must hold this lock
// around Sit, Act, Showdown and ViewFor.
func (t *Table) Lock() { t.mu.Lock() }

func (t *Table) Unlock() { t.mu.Unlock() }

// AdjustSeat moves a seated player's chips by delta to mirror a change made
// to their balance outside the game — a rob, a slots spin, a gift.
//
// startStack moves by the SAME delta, and that is the whole point. Showdown
// computes each seat's settlement result as Stack - startStack and hands
// that to SettlePoker, which applies it to the balance. Moving Stack alone
// would make these chips look like poker winnings and credit them to the
// balance a second time, minting money out of a /rob. Moving both leaves
// the hand's result untouched while the chips still appear on the table.
//
// A withdrawal is clamped at the chips actually sitting in the stack:
// anything already committed to the pot this hand belongs to the pot, not
// to the player, and taking it would break the pot arithmetic. The returned
// value is the delta actually applied, which can therefore be smaller in
// magnitude than the one requested.
func (t *Table) AdjustSeat(userID string, delta int) int {
	if delta == 0 {
		return 0
	}
	for _, s := range t.Seats {
		if s.UserID != userID {
			continue
		}
		if delta < 0 && -delta > s.Stack {
			delta = -s.Stack
		}
		if delta == 0 {
			return 0
		}
		s.Stack += delta
		s.startStack += delta
		t.Seq++
		return delta
	}
	return 0
}

// HasLiveStake reports whether userID currently has chips at risk in a hand
// in progress at this table. It is the test for whether abandoning their
// seat would strand real money: between hands, or once they have folded or
// busted, there is nothing left to protect.
func (t *Table) HasLiveStake(userID string) bool {
	if t.Stage == StageWaiting || t.Stage == StageShowdown {
		return false
	}
	for _, s := range t.Seats {
		if s.UserID == userID {
			return s.InHand && !s.Folded
		}
	}
	return false
}

// StandUp removes a player's seat, repairing Button and ToAct so they stay
// valid indices into the shortened slice — the same repair evictOneBot does
// for bots. It refuses while the seat has a live stake, since removing it
// mid-hand would delete chips that are already part of a pot other players
// are contesting.
//
// Returns whether a seat was actually removed.
func (t *Table) StandUp(userID string) bool {
	if t.HasLiveStake(userID) {
		return false
	}
	for i, s := range t.Seats {
		if s.UserID != userID {
			continue
		}
		t.Seats = append(t.Seats[:i], t.Seats[i+1:]...)
		if t.Button >= i {
			t.Button--
		}
		if t.Button < 0 {
			t.Button = len(t.Seats) - 1
		}
		switch {
		case t.ToAct == i:
			// -1 is the engine's "nobody to act" sentinel; StartHand
			// recomputes it, and no money path reads ToAct between hands.
			t.ToAct = -1
		case t.ToAct > i:
			t.ToAct--
		}
		t.Seq++
		return true
	}
	return false
}
