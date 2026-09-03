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
