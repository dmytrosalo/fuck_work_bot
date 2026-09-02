package poker

import "testing"

func seat(committed int, folded bool) *Seat {
	return &Seat{Committed: committed, Folded: folded}
}

func TestReturnUncalledGivesBackExcess(t *testing.T) {
	// A bets 500, B calls all-in for 200. A's extra 300 is uncalled.
	a := seat(500, false)
	b := seat(200, false)
	ReturnUncalled([]*Seat{a, b})
	if a.Committed != 200 {
		t.Errorf("A committed = %d, want 200", a.Committed)
	}
	if a.Stack != 300 {
		t.Errorf("A stack = %d, want 300 returned", a.Stack)
	}
}

func TestReturnUncalledNoopWhenMatched(t *testing.T) {
	a := seat(200, false)
	b := seat(200, false)
	ReturnUncalled([]*Seat{a, b})
	if a.Committed != 200 || a.Stack != 0 {
		t.Errorf("nothing should be returned, got committed=%d stack=%d", a.Committed, a.Stack)
	}
}

func TestBuildPotsSimpleSingle(t *testing.T) {
	pots := BuildPots([]*Seat{seat(100, false), seat(100, false)})
	if len(pots) != 1 {
		t.Fatalf("pots = %d, want 1", len(pots))
	}
	if pots[0].Amount != 200 {
		t.Errorf("amount = %d, want 200", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 2 {
		t.Errorf("eligible = %v, want 2 seats", pots[0].Eligible)
	}
}

func TestBuildPotsShortStackSidePot(t *testing.T) {
	// seat0 all-in 100, seat1 and seat2 continue to 300.
	pots := BuildPots([]*Seat{seat(100, false), seat(300, false), seat(300, false)})
	if len(pots) != 2 {
		t.Fatalf("pots = %d, want 2", len(pots))
	}
	if pots[0].Amount != 300 || len(pots[0].Eligible) != 3 {
		t.Errorf("main pot = %d/%v, want 300 with 3 eligible", pots[0].Amount, pots[0].Eligible)
	}
	if pots[1].Amount != 400 || len(pots[1].Eligible) != 2 {
		t.Errorf("side pot = %d/%v, want 400 with 2 eligible", pots[1].Amount, pots[1].Eligible)
	}
	for _, i := range pots[1].Eligible {
		if i == 0 {
			t.Error("short stack must not be eligible for the side pot")
		}
	}
}

func TestBuildPotsFoldedChipsCountButConferNoEligibility(t *testing.T) {
	// seat0 folded after committing 100; seats 1,2 at 100.
	pots := BuildPots([]*Seat{seat(100, true), seat(100, false), seat(100, false)})
	if len(pots) != 1 {
		t.Fatalf("pots = %d, want 1", len(pots))
	}
	if pots[0].Amount != 300 {
		t.Errorf("amount = %d, want 300 (folded chips still in the pot)", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 2 {
		t.Errorf("eligible = %v, want only the two live seats", pots[0].Eligible)
	}
}

func TestBuildPotsConservesChips(t *testing.T) {
	seats := []*Seat{seat(50, true), seat(275, false), seat(275, false), seat(120, false)}
	total := 0
	for _, s := range seats {
		total += s.Committed
	}
	sum := 0
	for _, p := range BuildPots(seats) {
		sum += p.Amount
	}
	if sum != total {
		t.Fatalf("pots total %d != committed total %d — chips created or destroyed", sum, total)
	}
}
