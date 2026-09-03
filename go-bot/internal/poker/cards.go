package poker

import (
	"math/rand"
	"time"

	pk "github.com/chehsunliu/poker"
)

const (
	BotUserPrefix = "bot:"
	MaxSeats      = 6
	SmallBlind    = 50
	BigBlind      = 100
	MinBuyIn      = 1000
	MaxBuyIn      = 10000
	TurnTimeout   = 90 * time.Second

	// BlindInterval is how long each blind level lasts. Levels double the
	// blinds, so a level is a real jump, not a nudge.
	BlindInterval = 5 * time.Minute
	// MaxBlindLevel caps the escalation at 2^4 = 16x the base, i.e.
	// 800/1600. Deliberately capped: this is a CASH game funded from real
	// balances, and uncapped doubling would reach a big blind larger than
	// any stack within the hour, turning every hand into a forced all-in of
	// someone's whole bankroll decided by a timer rather than by cards.
	MaxBlindLevel = 4
)

var allRanks = []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}
var allSuits = []string{"s", "h", "d", "c"}

// NewShuffledDeck returns a full 52-card deck in random order.
func NewShuffledDeck() []pk.Card {
	deck := make([]pk.Card, 0, 52)
	for _, r := range allRanks {
		for _, s := range allSuits {
			deck = append(deck, pk.NewCard(r+s))
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

// Best returns the rank of the best five-card hand from hole+board.
// Lower is stronger (deuces convention, verified by TestLowerRankIsStronger).
func Best(hole, board []pk.Card) int32 {
	all := make([]pk.Card, 0, len(hole)+len(board))
	all = append(all, hole...)
	all = append(all, board...)
	return pk.Evaluate(all)
}

// BlindsAt returns the blinds for a table that has been running for d,
// doubling once per BlindInterval and stopping at MaxBlindLevel. A negative
// or zero d yields the base level, so a table with an unset creation time
// starts at 50/100 rather than jumping straight to the cap.
func BlindsAt(d time.Duration) (small, big int) {
	level := 0
	if d > 0 {
		level = int(d / BlindInterval)
	}
	if level > MaxBlindLevel {
		level = MaxBlindLevel
	}
	mult := 1 << level
	return SmallBlind * mult, BigBlind * mult
}

// NextBlindRaise returns how long until the next level for a table running
// for d, and false once the cap is reached and blinds never move again.
func NextBlindRaise(d time.Duration) (time.Duration, bool) {
	if d < 0 {
		d = 0
	}
	level := int(d / BlindInterval)
	if level >= MaxBlindLevel {
		return 0, false
	}
	return BlindInterval - (d % BlindInterval), true
}
