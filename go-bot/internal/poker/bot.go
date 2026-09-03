package poker

import (
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
// pk.Evaluate panics on fewer than five cards, so a preflop hand (two cards,
// empty board) cannot go through the evaluator. Preflop therefore uses a
// tier heuristic; once there are at least three board cards the real
// evaluator is used and its rank is normalised.
func handStrength(hole, board []pk.Card) float64 {
	if len(board) < 3 {
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
