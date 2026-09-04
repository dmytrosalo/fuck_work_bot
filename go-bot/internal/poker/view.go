package poker

import (
	pk "github.com/chehsunliu/poker"
)

type SeatView struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Stack  int    `json:"stack"`
	Bet    int    `json:"bet"`
	Folded bool   `json:"folded"`
	AllIn  bool   `json:"all_in"`
	// InHand is false for a seat that is NOT part of the hand in progress —
	// most often someone who sat down after the cards were dealt, who has
	// no hole cards and will be dealt in next hand. Without it the client
	// cannot tell them apart from an active player and renders them
	// holding face-down cards they do not have.
	InHand bool `json:"in_hand"`
	// Avatar indexes the client's avatar pool. Public by design: everyone
	// at the table sees which one you picked.
	Avatar int      `json:"avatar"`
	Hole   []string `json:"hole,omitempty"` // populated only for the viewer
	ToAct  bool     `json:"to_act"`
	// Won is this seat's NET result for the hand just finished — winnings
	// minus everything it put in — and is meaningful only at showdown. It
	// is derived from startStack, which Showdown leaves untouched, so no
	// extra per-hand state has to be carried to report it.
	Won int `json:"won,omitempty"`
}

type TableView struct {
	ID       string     `json:"id"`
	Seq      uint64     `json:"seq"`
	Stage    string     `json:"stage"`
	Board    []string   `json:"board"`
	Pot      int        `json:"pot"`
	Seats    []SeatView `json:"seats"`
	YouSeat  int        `json:"you_seat"`
	Deadline int64      `json:"deadline"`
	// MinRaise is the smallest legal RAISE INCREMENT over the current high
	// bet. The client needs it to offer a legal raise: guessing BigBlind
	// is wrong on any street where a previous raise widened it, and the
	// engine rejects the guess with ErrRaiseTooLow.
	MinRaise int `json:"min_raise"`
	// HighBet is the largest bet committed on the current street, so the
	// client does not have to re-derive it from Seats to size a raise.
	HighBet int `json:"high_bet"`
	// Hands is how many hands this table has dealt.
	Hands int `json:"hands"`
	// Elapsed is the session clock in seconds. Sent as a duration rather
	// than a start timestamp so a client whose device clock is wrong still
	// shows the right elapsed time.
	Elapsed int `json:"elapsed"`
	// SmallBlind/BigBlind are the blinds in force for the current hand.
	SmallBlind int `json:"small_blind"`
	BigBlind   int `json:"big_blind"`
	// NextBlindIn is seconds until the blinds next double, or -1 once the
	// schedule has reached its cap and they never move again.
	NextBlindIn int `json:"next_blind_in"`
	// HandName and HandCards describe THE VIEWER'S OWN best hand and the
	// five cards forming it. They are computed only from the seat matching
	// the requesting user, so this view can never carry a read on someone
	// else's holding — the same isolation rule Hole follows.
	HandName  string   `json:"hand_name,omitempty"`
	HandCards []string `json:"hand_cards,omitempty"`
	// Winners and WinHand describe the hand just finished. WinHand is empty
	// when the pot was taken without a showdown.
	Winners []string `json:"winners,omitempty"`
	WinHand string   `json:"win_hand,omitempty"`
}

var stageNames = map[Stage]string{
	StageWaiting: "waiting", StagePreflop: "preflop", StageFlop: "flop",
	StageTurn: "turn", StageRiver: "river", StageShowdown: "showdown",
}

func strs(cs []pk.Card) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.String())
	}
	return out
}

// ViewFor builds the state visible to one player. Other players' hole cards
// are never included — this is the trust boundary, enforced server-side.
func (t *Table) ViewFor(userID string) TableView {
	pot := 0
	for _, s := range t.Seats {
		pot += s.Committed
	}
	v := TableView{
		ID: t.ID, Seq: t.Seq, Stage: stageNames[t.Stage],
		Board: strs(t.Board), Pot: pot, YouSeat: -1,
		Deadline: t.Deadline.Unix(),
		MinRaise: t.MinRaise, HighBet: t.highBet(),
		Winners: t.LastWinners, WinHand: t.LastHandName,
		Hands:      t.Hands,
		Elapsed:    int(t.Elapsed().Seconds()),
		SmallBlind: t.SmallBlind, BigBlind: t.BigBlind,
		NextBlindIn: nextBlindSeconds(t),
	}
	for i, s := range t.Seats {
		sv := SeatView{
			UserID: s.UserID, Name: s.Name, Stack: s.Stack, Bet: s.Bet,
			Folded: s.Folded, AllIn: s.AllIn, InHand: s.InHand, ToAct: i == t.ToAct,
			Avatar: s.Avatar,
		}
		if t.Stage == StageShowdown && s.InHand {
			sv.Won = s.Stack - s.startStack
		}
		if s.UserID == userID {
			sv.Hole = strs(s.Hole)
			v.YouSeat = i
			// Derived from THIS seat only — never from any other seat's
			// Hole — so it cannot leak an opponent's hand.
			if s.InHand && !s.Folded {
				if name, used := HandName(s.Hole, t.Board); name != "" {
					v.HandName, v.HandCards = name, strs(used)
				}
			}
		} else if t.Stage == StageShowdown && !s.Folded && s.InHand {
			sv.Hole = strs(s.Hole) // cards are public at showdown
		}
		v.Seats = append(v.Seats, sv)
	}
	return v
}

// nextBlindSeconds is NextBlindRaise rendered for the wire: seconds until
// the next level, or -1 once capped.
func nextBlindSeconds(t *Table) int {
	d, more := NextBlindRaise(t.Elapsed())
	if !more {
		return -1
	}
	return int(d.Seconds())
}
