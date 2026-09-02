package poker

import "sort"

// ReturnUncalled gives back the portion of the largest commitment that no
// other player matched. Without this, an uncalled bet would be split into a
// pot that only its own bettor is eligible for, and the zero-sum invariant
// would still hold but the player would be paid from their own chips.
//
// CONTRACT: ReturnUncalled runs exactly once per hand, at hand end,
// immediately before BuildPots.
func ReturnUncalled(seats []*Seat) {
	max, second := 0, 0
	var top *Seat
	count := 0
	for _, s := range seats {
		if s.Committed > max {
			second = max
			max = s.Committed
			top = s
			count = 1
		} else if s.Committed == max {
			count++
		} else if s.Committed > second {
			second = s.Committed
		}
	}
	if top == nil || count > 1 || max == second {
		return
	}
	excess := max - second
	top.Committed -= excess
	top.Stack += excess
	if top.Stack > 0 {
		top.AllIn = false
	}
}

// BuildPots splits commitments into a main pot and any side pots.
// Folded players' chips stay in the pot but grant no eligibility.
//
// Eligibility is monotonically non-increasing across levels. When a level
// has no eligible players (all contributors folded), its chips are accumulated
// and folded into the last pot that does have eligible players. If NO level
// has any eligible player (every contributor folded), a degenerate pot is
// returned containing all committed chips with all contributors eligible,
// conserving chips rather than deleting them.
func BuildPots(seats []*Seat) []Pot {
	levelSet := map[int]bool{}
	for _, s := range seats {
		if s.Committed > 0 {
			levelSet[s.Committed] = true
		}
	}
	levels := make([]int, 0, len(levelSet))
	for l := range levelSet {
		levels = append(levels, l)
	}
	sort.Ints(levels)

	pots := make([]Pot, 0, len(levels))
	prev := 0
	orphanedAmount := 0
	for _, lvl := range levels {
		amount := 0
		eligible := []int{}
		for i, s := range seats {
			if s.Committed >= lvl {
				amount += lvl - prev
				if !s.Folded {
					eligible = append(eligible, i)
				}
			}
		}
		if amount > 0 {
			if len(eligible) > 0 {
				// This level has eligible players; add any orphaned chips
				amount += orphanedAmount
				pots = append(pots, Pot{Amount: amount, Eligible: eligible})
				orphanedAmount = 0
			} else {
				// No eligible players at this level; accumulate chips
				orphanedAmount += amount
			}
		}
		prev = lvl
	}

	// Fold orphaned chips into the last pot that has eligible players,
	// or create a degenerate pot if no pot has any eligible players
	if orphanedAmount > 0 {
		if len(pots) > 0 {
			pots[len(pots)-1].Amount += orphanedAmount
		} else {
			// Degenerate case: all contributors folded at all levels
			eligible := []int{}
			for i, s := range seats {
				if s.Committed > 0 {
					eligible = append(eligible, i)
				}
			}
			pots = append(pots, Pot{Amount: orphanedAmount, Eligible: eligible})
		}
	}

	return pots
}
