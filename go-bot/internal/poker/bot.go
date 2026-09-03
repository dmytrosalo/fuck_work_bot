package poker

import (
	"math/rand"

	pk "github.com/chehsunliu/poker"
)

// rankOrder maps a card's rank character to 0 (deuce) through 12 (ace).
const rankOrder = "23456789TJQKA"

func rankIndex(c pk.Card) int {
	s := c.String()
	if len(s) == 0 {
		return 0
	}
	for i := 0; i < len(rankOrder); i++ {
		if rankOrder[i] == s[0] {
			return i
		}
	}
	return 0
}

func suitOf(c pk.Card) byte {
	s := c.String()
	if len(s) < 2 {
		return '?'
	}
	return s[1]
}

// worstRank is the numeric rank pk.Evaluate assigns to the weakest possible
// five-card hand (7-5-4-3-2 offsuit). Lower is stronger, so it is the
// denominator when normalising a rank to a 0..1 strength.
const worstRank = 7462

// handStrength scores a holding from 0.0 (hopeless) to 1.0 (nuts).
//
// pk.Evaluate panics unless the card count is exactly 5, 6, or 7. A preflop
// hand (two cards, empty board) cannot go through the evaluator, nor can any
// holding with fewer than two hole cards. Preflop and incomplete holdings
// therefore use a tier heuristic; once there are at least three board cards
// AND at least two hole cards the real evaluator is used and its rank is
// normalised.
func handStrength(hole, board []pk.Card) float64 {
	if len(board) < 3 || len(hole) < 2 {
		return preflopStrength(hole)
	}
	rank := Best(hole, board)
	s := 1.0 - float64(rank)/float64(worstRank)
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// preflopStrength scores a two-card holding using the usual shape heuristics:
// pairs are strong and scale with rank, suited cards gain, and connected
// cards gain. The absolute numbers matter less than the ordering, which the
// decision thresholds are tuned against.
func preflopStrength(hole []pk.Card) float64 {
	if len(hole) < 2 {
		return 0
	}
	a, b := rankIndex(hole[0]), rankIndex(hole[1])
	hi, lo := a, b
	if lo > hi {
		hi, lo = lo, hi
	}

	if hi == lo { // pocket pair: 22 -> 0.50, AA -> 0.95
		return 0.50 + 0.45*float64(hi)/12.0
	}

	s := 0.30 * float64(hi) / 12.0 // high card carries most of it
	s += 0.15 * float64(lo) / 12.0
	if suitOf(hole[0]) == suitOf(hole[1]) {
		s += 0.08
	}
	if gap := hi - lo; gap == 1 {
		s += 0.06
	} else if gap == 2 {
		s += 0.03
	}
	if s > 1 {
		return 1
	}
	return s
}

// BotInput is everything a bot needs to choose an action. It is a plain
// value: the decision function reads no table state and takes no locks, so
// it can be tested in isolation and called safely under the table lock.
type BotInput struct {
	Hole     []pk.Card
	Board    []pk.Card
	ToCall   int // chips needed to call; 0 means checking is free
	Pot      int // chips already in the pot this hand
	Stack    int // the bot's remaining chips
	MinRaise int // the smallest legal raise increment
	Bet      int // the bot's own contribution on this street
}

// bluffFrequency is how often a bot bets a weak hand. It keeps bots from
// being trivially readable without meaningfully changing their expected
// value, which the self-play simulation guards.
const bluffFrequency = 0.10

// raiseThreshold is the minimum hand strength to raise facing a bet.
// Approximately trips-or-better on the 0..1 normalization (1 - rank/7462).
const raiseThreshold = 0.75

// Decide chooses an action for a bot. The returned amount is meaningful only
// for ActRaise, where it is the total this seat should have committed on the
// current street. The engine validates every action regardless, so a bad
// return here can only produce a rejected action, never an illegal one.
// Decide chooses an action for a bot. The returned amount is meaningful only
// for ActRaise, where it is the total this seat should have committed on the
// current street. The engine validates every action regardless, so a bad
// return here can only produce a rejected action, never an illegal one.
func Decide(in BotInput, rng *rand.Rand) (Action, int) {
	strength := handStrength(in.Hole, in.Board)

	// Free to see the next card: never fold.
	if in.ToCall <= 0 {
		if strength > raiseThreshold || rng.Float64() < bluffFrequency {
			return raiseOrAllIn(in, strength, rng)
		}
		return ActCheck, 0
	}

	// Facing a bet: compare hand strength against the price being offered.
	potOdds := float64(in.ToCall) / float64(in.Pot+in.ToCall)

	switch {
	case strength > raiseThreshold:
		return raiseOrAllIn(in, strength, rng)
	case strength > potOdds+0.10:
		return ActCall, 0
	case rng.Float64() < bluffFrequency && in.ToCall < in.Stack/4:
		return ActCall, 0
	default:
		return ActFold, 0
	}
}

// raiseOrAllIn sizes a raise as a fraction of the pot, clamped to what the
// bot can actually put in. If the raise would not clear the minimum but
// equals the full stack, it is a legal all-in and returned as ActRaise.
// Otherwise, it calls or checks rather than returning an action the engine
// will reject.
func raiseOrAllIn(in BotInput, strength float64, rng *rand.Rand) (Action, int) {
	target := in.Bet + in.ToCall + int(float64(in.Pot)*(0.4+0.4*strength))
	max := in.Bet + in.Stack
	if target > max {
		target = max
	}
	if target < in.Bet+in.ToCall+in.MinRaise {
		// A shove (target == max) is always legal even if it doesn't clear the min-raise.
		if target == max {
			return ActRaise, target
		}
		if in.ToCall <= 0 {
			return ActCheck, 0
		}
		return ActCall, 0
	}
	return ActRaise, target
}
