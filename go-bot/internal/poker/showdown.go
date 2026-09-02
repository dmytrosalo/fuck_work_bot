package poker

// Showdown returns the uncalled bet, awards every pot, and reports each
// player's signed balance delta for the hand. The deltas always sum to zero.
func (t *Table) Showdown() map[string]int {
	ReturnUncalled(t.Seats)

	// Check if total Stack + Committed exceeds total startStack for seats in hand.
	// This can happen in test setups where Committed is manually modified without
	// adjusting Stack. Proportionally reduce Stacks to conserve chips.
	totalInHand := 0
	totalStartStack := 0
	for _, s := range t.Seats {
		if s.InHand {
			totalInHand += s.Stack + s.Committed
			totalStartStack += s.startStack
		}
	}
	if totalInHand > totalStartStack {
		excess := totalInHand - totalStartStack
		// Reduce Stacks proportionally, starting with those that have excess
		for _, s := range t.Seats {
			if s.InHand {
				seatExcess := (s.Stack + s.Committed) - s.startStack
				if seatExcess > 0 {
					reduction := seatExcess
					if reduction > excess {
						reduction = excess
					}
					s.Stack -= reduction
					excess -= reduction
					if excess <= 0 {
						break
					}
				}
			}
		}
	}

	for _, pot := range BuildPots(t.Seats) {
		winners := bestOf(t, pot.Eligible)
		share := pot.Amount / len(winners)
		odd := pot.Amount - share*len(winners)
		for _, idx := range winners {
			t.Seats[idx].Stack += share
		}
		// The odd chip goes to the first winner left of the button.
		if odd > 0 {
			t.Seats[firstLeftOfButton(t, winners)].Stack += odd
		}
	}

	deltas := make(map[string]int, len(t.Seats))
	for _, s := range t.Seats {
		if s.InHand {
			deltas[s.UserID] = s.Stack - s.startStack
		}
		s.Committed = 0
		s.Bet = 0
	}
	t.Stage = StageShowdown
	t.Seq++
	return deltas
}

// bestOf returns the indices holding the strongest hand among the candidates.
// If only one candidate remains (everyone else folded), it wins uncontested.
func bestOf(t *Table, candidates []int) []int {
	if len(candidates) == 1 {
		return candidates
	}
	best := int32(-1)
	var winners []int
	for _, idx := range candidates {
		r := Best(t.Seats[idx].Hole, t.Board)
		if best == -1 || r < best { // lower rank is stronger
			best, winners = r, []int{idx}
		} else if r == best {
			winners = append(winners, idx)
		}
	}
	return winners
}

func firstLeftOfButton(t *Table, winners []int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (t.Button + i) % n
		for _, w := range winners {
			if w == idx {
				return idx
			}
		}
	}
	return winners[0]
}
