package poker

import (
	pk "github.com/chehsunliu/poker"
)

type SeatView struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Stack  int      `json:"stack"`
	Bet    int      `json:"bet"`
	Folded bool     `json:"folded"`
	AllIn  bool     `json:"all_in"`
	Hole   []string `json:"hole,omitempty"` // populated only for the viewer
	ToAct  bool     `json:"to_act"`
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
	}
	for i, s := range t.Seats {
		sv := SeatView{
			UserID: s.UserID, Name: s.Name, Stack: s.Stack, Bet: s.Bet,
			Folded: s.Folded, AllIn: s.AllIn, ToAct: i == t.ToAct,
		}
		if s.UserID == userID {
			sv.Hole = strs(s.Hole)
			v.YouSeat = i
		} else if t.Stage == StageShowdown && !s.Folded && s.InHand {
			sv.Hole = strs(s.Hole) // cards are public at showdown
		}
		v.Seats = append(v.Seats, sv)
	}
	return v
}
