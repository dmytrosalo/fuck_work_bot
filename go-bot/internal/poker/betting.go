package poker

import (
	"errors"
	"time"
)

type Action string

const (
	ActFold  Action = "fold"
	ActCheck Action = "check"
	ActCall  Action = "call"
	ActRaise Action = "raise"
)

var (
	ErrNotYourTurn = errors.New("не твій хід")
	ErrCannotCheck = errors.New("не можна чекати, є ставка")
	ErrRaiseTooLow = errors.New("замала ставка")
	ErrNotEnough   = errors.New("недостатньо фішок")
	ErrHandOver    = errors.New("роздача завершена")
)

// highBet returns the largest bet on the current street.
func (t *Table) highBet() int {
	high := 0
	for _, s := range t.Seats {
		if s.Bet > high {
			high = s.Bet
		}
	}
	return high
}

func (t *Table) Act(userID string, a Action, amount int) error {
	if t.Stage == StageWaiting || t.Stage == StageShowdown {
		return ErrHandOver
	}
	if t.ToAct < 0 || t.Seats[t.ToAct].UserID != userID {
		return ErrNotYourTurn
	}
	s := t.Seats[t.ToAct]
	s.actedThisStreet = true
	high := t.highBet()

	switch a {
	case ActFold:
		s.Folded = true
	case ActCheck:
		if s.Bet < high {
			return ErrCannotCheck
		}
	case ActCall:
		t.post(s, high-s.Bet)
	case ActRaise:
		if amount > s.Stack+s.Bet {
			return ErrNotEnough
		}
		if amount < high+t.MinRaise && amount < s.Stack+s.Bet {
			return ErrRaiseTooLow
		}
		t.MinRaise = amount - high
		t.post(s, amount-s.Bet)
	default:
		return ErrHandOver
	}

	t.Seq++

	if t.liveCount() <= 1 {
		t.Stage = StageShowdown
		return nil
	}
	if t.bettingClosed() {
		t.advance()
	} else {
		t.ToAct = t.nextActive(t.ToAct)
		t.Deadline = time.Now().Add(TurnTimeout)
	}
	return nil
}

// liveCount counts seats that have not folded.
func (t *Table) liveCount() int {
	n := 0
	for _, s := range t.Seats {
		if s.InHand && !s.Folded {
			n++
		}
	}
	return n
}

// bettingClosed reports whether every live, non-all-in seat has matched the
// high bet and has acted at least once this street.
func (t *Table) bettingClosed() bool {
	high := t.highBet()
	for _, s := range t.Seats {
		if !s.InHand || s.Folded || s.AllIn {
			continue
		}
	if s.Bet != high || !s.actedThisStreet {
			return false
		}
	}
	return true
}

// advance deals the next street, or moves to showdown after the river.
// When all remaining players are all-in, it loops through remaining streets
// to reach showdown without hanging.
func (t *Table) advance() {
	for {
		for _, s := range t.Seats {
			s.Bet = 0
			s.actedThisStreet = false
		}
		t.MinRaise = BigBlind

		switch t.Stage {
		case StagePreflop:
			t.Stage = StageFlop
			t.Board = append(t.Board, t.draw(), t.draw(), t.draw())
		case StageFlop:
			t.Stage = StageTurn
			t.Board = append(t.Board, t.draw())
		case StageTurn:
			t.Stage = StageRiver
			t.Board = append(t.Board, t.draw())
		case StageRiver:
			t.Stage = StageShowdown
			return
		}

		t.ToAct = t.nextActive(t.Button)
		if t.ToAct >= 0 {
			t.Deadline = time.Now().Add(TurnTimeout)
			return
		}
		// nobody can act — run the remaining board out rather than hang
	}
}
