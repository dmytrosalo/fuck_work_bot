package poker

import "sort"

// ReturnUncalled gives back the portion of the largest commitment that no
// other player matched. Without this, an uncalled bet would be split into a
// pot that only its own bettor is eligible for, and the zero-sum invariant
// would still hold but the player would be paid from their own chips.
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
}

// BuildPots splits commitments into a main pot and any side pots.
// Folded players' chips stay in the pot but grant no eligibility.
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
		if amount > 0 && len(eligible) > 0 {
			pots = append(pots, Pot{Amount: amount, Eligible: eligible})
		}
		prev = lvl
	}
	return pots
}
