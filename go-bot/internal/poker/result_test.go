package poker

import "testing"

func TestShowdownReportsWinnerAndCombination(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()

	tbl.Seats[0].Hole = cards("Ah", "Ad")
	tbl.Seats[1].Hole = cards("2c", "7d")
	tbl.Board = cards("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = StageRiver
	tbl.Showdown()

	if len(tbl.LastWinners) != 1 || tbl.LastWinners[0] != "Dmytro" {
		t.Errorf("winners = %v, want [Dmytro]", tbl.LastWinners)
	}
	if tbl.LastHandName != "трійка" {
		t.Errorf("hand = %q, want трійка", tbl.LastHandName)
	}

	v := tbl.ViewFor("b")
	if len(v.Winners) != 1 || v.Winners[0] != "Dmytro" || v.WinHand != "трійка" {
		t.Errorf("view reports winners=%v hand=%q", v.Winners, v.WinHand)
	}
}

// Winning because everyone folded reveals nothing. Naming the combination
// would expose a holding nobody paid to see.
func TestUncontestedPotNamesNoHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()

	tbl.Seats[0].Hole = cards("Ah", "Ad")
	tbl.Seats[1].Hole = cards("2c", "7d")
	tbl.Board = cards("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = StageRiver
	tbl.Seats[1].Folded = true
	tbl.Showdown()

	if len(tbl.LastWinners) != 1 || tbl.LastWinners[0] != "Dmytro" {
		t.Errorf("winners = %v, want [Dmytro]", tbl.LastWinners)
	}
	if tbl.LastHandName != "" {
		t.Errorf("named %q for an uncontested pot — that leaks a hand nobody saw", tbl.LastHandName)
	}
}

func TestSplitPotReportsBothWinners(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()

	// Identical hands: the board plays.
	tbl.Seats[0].Hole = cards("2c", "3d")
	tbl.Seats[1].Hole = cards("2h", "3s")
	tbl.Board = cards("As", "Kh", "Qd", "Jc", "Ts")
	tbl.Stage = StageRiver
	tbl.Showdown()

	if len(tbl.LastWinners) != 2 {
		t.Errorf("winners = %v, want both players", tbl.LastWinners)
	}
}

func TestResultClearedOnNextHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()
	tbl.Board = cards("As", "Kh", "Qd", "3c", "9s")
	tbl.Stage = StageRiver
	tbl.Showdown()
	if len(tbl.LastWinners) == 0 {
		t.Fatal("setup: no winner recorded")
	}
	_ = tbl.StartHand()
	if len(tbl.LastWinners) != 0 || tbl.LastHandName != "" {
		t.Errorf("last hand's result survived into the new hand: %v %q", tbl.LastWinners, tbl.LastHandName)
	}
}

// hand_name is the viewer's own read. It must never appear in a view built
// for someone else, or it would hand them a read on a live opponent.
func TestHandNameIsViewerOnly(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()
	tbl.Seats[0].Hole = cards("Ah", "Ad")
	tbl.Seats[1].Hole = cards("2c", "7d")
	tbl.Board = cards("As", "Kh", "Qd")
	tbl.Stage = StageFlop

	if got := tbl.ViewFor("a").HandName; got != "трійка" {
		t.Errorf("own view hand_name = %q, want трійка", got)
	}
	// Danya holds 2-7 on A-K-Q: high card. If aces-trips ever showed up in
	// their view, the viewer isolation is broken.
	if got := tbl.ViewFor("b").HandName; got == "трійка" {
		t.Error("opponent's hand leaked into another player's view")
	}
	// And the highlighted cards must all be ones that viewer can see.
	v := tbl.ViewFor("b")
	visible := map[string]bool{}
	for _, c := range v.Board {
		visible[c] = true
	}
	for _, c := range v.Seats[v.YouSeat].Hole {
		visible[c] = true
	}
	for _, c := range v.HandCards {
		if !visible[c] {
			t.Errorf("hand_cards leaked %s, which this viewer cannot see", c)
		}
	}
}

// A folded player is out; giving them a running read is noise at best.
func TestNoHandNameOnceFolded(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "Dmytro", 10000)
	_ = tbl.Sit("b", "Danya", 10000)
	_ = tbl.StartHand()
	tbl.Seats[0].Hole = cards("Ah", "Ad")
	tbl.Board = cards("As", "Kh", "Qd")
	tbl.Stage = StageFlop
	tbl.Seats[0].Folded = true

	if got := tbl.ViewFor("a").HandName; got != "" {
		t.Errorf("folded player still shown %q", got)
	}
}
