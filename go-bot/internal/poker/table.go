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
	settled  bool // true after Showdown() to prevent re-settlement
	mu       sync.Mutex
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
	return &Table{ID: id, ChatID: chatID, Stage: StageWaiting, Button: -1}
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
	t.MinRaise = BigBlind
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
	t.post(t.Seats[sb], SmallBlind)
	t.post(t.Seats[bb], BigBlind)

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
