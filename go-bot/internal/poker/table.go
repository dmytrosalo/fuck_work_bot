package poker

import (
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
	UserID     string
	Name       string
	Stack      int       // chips at the table
	Hole       []pk.Card
	Folded     bool
	AllIn      bool
	Committed  int // total committed THIS HAND — basis for side pots
	Bet        int // committed on the CURRENT street — basis for min-raise
	InHand     bool
	startStack int // stack when the hand began, for settlement deltas
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
	mu       sync.Mutex
}
