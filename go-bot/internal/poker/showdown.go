package poker

// Showdown returns the uncalled bet, awards every pot, and reports each
// player's signed balance delta for the hand. The deltas always sum to zero.
// It is idempotent: calling twice returns nil on the second call.
func (t *Table) Showdown() map[string]int {
	if t.settled {
		return nil
	}
	t.settled = true

	ReturnUncalled(t.Seats)

	for _, pot := range BuildPots(t.Seats) {
		winners := bestOf(t, pot.Eligible)
		if len(winners) == 0 {
			continue
		}
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

	t.recordResult()

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
// If the board is too short to evaluate hands, all candidates split.
func bestOf(t *Table, candidates []int) []int {
	if len(candidates) == 0 {
		return candidates
	}
	if len(candidates) == 1 {
		return candidates
	}
	// Guard against short boards: if hole+board < 5 cards, can't evaluate hands.
	// All candidates split the pot equally.
	if len(t.Board) < 3 {
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

// recordResult captures who won and with what, for the showdown line. Called
// from Showdown after the pots have been awarded but before Committed is
// cleared, so contested() still sees the hand as it was played.
//
// A hand is only NAMED when at least two players reached showdown. If
// everyone else folded, the last player standing never revealed anything,
// and naming their holding would leak a hand nobody paid to see.
func (t *Table) recordResult() {
	shown := 0
	for _, s := range t.Seats {
		if s.InHand && !s.Folded {
			shown++
		}
	}

	seen := map[string]bool{}
	t.LastWinners = nil
	t.LastHandName = ""
	best := int32(-1)

	for _, pot := range BuildPots(t.Seats) {
		for _, idx := range bestOf(t, pot.Eligible) {
			w := t.Seats[idx]
			if !seen[w.UserID] {
				seen[w.UserID] = true
				t.LastWinners = append(t.LastWinners, w.Name)
			}
			// Name the strongest winning hand across pots: with a single
			// pot — the ordinary case — that is simply the winner's hand.
			if shown >= 2 && len(t.Board) >= 3 && len(w.Hole) == 2 {
				if r := Best(w.Hole, t.Board); best == -1 || r < best {
					best = r
					if name, _ := HandName(w.Hole, t.Board); name != "" {
						t.LastHandName = name
					}
				}
			}
		}
	}
}
