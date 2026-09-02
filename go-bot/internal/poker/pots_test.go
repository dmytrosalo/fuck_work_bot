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
	if a.AllIn {
		t.Error("A should not be marked AllIn after refund")
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

func TestReturnUncalledNil(t *testing.T) {
	// Should not panic on nil slice
	ReturnUncalled(nil)
}

func TestReturnUncalledEmptySlice(t *testing.T) {
	// Should not panic on empty slice
	ReturnUncalled([]*Seat{})
}

func TestReturnUncalledCount1Path(t *testing.T) {
	// [500, 500, 200] - two seats at max level should not return
	a := seat(500, false)
	b := seat(500, false)
	c := seat(200, false)
	ReturnUncalled([]*Seat{a, b, c})
	if a.Committed != 500 || a.Stack != 0 {
		t.Errorf("A should not be refunded when count > 1, got committed=%d stack=%d", a.Committed, a.Stack)
	}
	if b.Committed != 500 || b.Stack != 0 {
		t.Errorf("B should not be refunded when count > 1, got committed=%d stack=%d", b.Committed, b.Stack)
	}
}

func TestReturnUncalledOrderingAscending(t *testing.T) {
	// [200, 500] - ascending order should still return
	a := seat(200, false)
	b := seat(500, false)
	ReturnUncalled([]*Seat{a, b})
	if b.Committed != 200 {
		t.Errorf("B committed = %d, want 200", b.Committed)
	}
	if b.Stack != 300 {
		t.Errorf("B stack = %d, want 300", b.Stack)
	}
}

func TestReturnUncalledOrderingMiddle(t *testing.T) {
	// [100, 500, 300] - max in middle should return
	a := seat(100, false)
	b := seat(500, false)
	c := seat(300, false)
	ReturnUncalled([]*Seat{a, b, c})
	if b.Committed != 300 {
		t.Errorf("B committed = %d, want 300", b.Committed)
	}
	if b.Stack != 200 {
		t.Errorf("B stack = %d, want 200", b.Stack)
	}
	// A and C should not be touched
	if a.Committed != 100 || a.Stack != 0 {
		t.Errorf("A should not be touched, got committed=%d stack=%d", a.Committed, a.Stack)
	}
	if c.Committed != 300 || c.Stack != 0 {
		t.Errorf("C should not be touched, got committed=%d stack=%d", c.Committed, c.Stack)
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
		t.Errorf("eligible length = %d, want 2", len(pots[0].Eligible))
	}
	// Check exact indices
	if pots[0].Eligible[0] != 0 || pots[0].Eligible[1] != 1 {
		t.Errorf("eligible = %v, want [0, 1]", pots[0].Eligible)
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
		t.Errorf("eligible length = %d, want 2", len(pots[0].Eligible))
	}
	// Check exact indices - must be [1, 2], not [0, 1] or any other combination
	if len(pots[0].Eligible) != 2 || pots[0].Eligible[0] != 1 || pots[0].Eligible[1] != 2 {
		t.Errorf("eligible = %v, want [1, 2]", pots[0].Eligible)
	}
}

func TestBuildPotsConservesChips(t *testing.T) {
	seats := []*Seat{seat(50, true), seat(275, false), seat(275, false), seat(120, false)}
	total := 0
	for _, s := range seats {
		total += s.Committed
	}
	pots := BuildPots(seats)
	sum := 0
	for _, p := range pots {
		sum += p.Amount
	}
	if sum != total {
		t.Fatalf("pots total %d != committed total %d — chips created or destroyed", sum, total)
	}
	// Assert full expected shape: [50 folded, 275, 275, 120]
	// Levels: [50, 120, 275]
	// Level 50: all 4 contribute (50*4=200), eligible [1,2,3] (seat 0 folded)
	// Level 120: seats 1,2,3 contribute ((120-50)*3=210), eligible [1,2,3]
	// Level 275: seats 1,2 contribute ((275-120)*2=310), eligible [1,2]
	// Expected: [{200,[1,2,3]}, {210,[1,2,3]}, {310,[1,2]}]
	if len(pots) != 3 {
		t.Fatalf("pots length = %d, want 3", len(pots))
	}
	expectedPots := []struct {
		amount   int
		eligible []int
	}{
		{200, []int{1, 2, 3}},
		{210, []int{1, 2, 3}},
		{310, []int{1, 2}},
	}
	for i, expected := range expectedPots {
		if pots[i].Amount != expected.amount {
			t.Errorf("pot %d amount = %d, want %d", i, pots[i].Amount, expected.amount)
		}
		if len(pots[i].Eligible) != len(expected.eligible) {
			t.Errorf("pot %d eligible length = %d, want %d", i, len(pots[i].Eligible), len(expected.eligible))
		}
		for j, idx := range pots[i].Eligible {
			if j >= len(expected.eligible) || idx != expected.eligible[j] {
				t.Errorf("pot %d eligible[%d] = %d, want %d", i, j, idx, expected.eligible[j])
			}
		}
	}
}

func TestBuildPotsZeroEligibleAllFolded(t *testing.T) {
	// Both players commit, one folds. Folded player's chips stay.
	// But actually this test needs a case where ALL contributors at a level are folded.
	// Example: seat0 bets 100 (not folded), seat1 raises to 200 (folded).
	// Level 100: seat0 not folded, seat1 not folded → eligible=[0,1], amount=200
	// Level 200: only seat1 → folded → no eligible → chips should go to previous level
	// Wait, this doesn't work with current test setup because once you bet, you're committed.
	// Let me use the case from Finding 1: BuildPots([seat(100,false), seat(200,true)])
	pots := BuildPots([]*Seat{seat(100, false), seat(200, true)})
	// Levels: [100, 200]
	// Level 100: both contribute (100*2=200), seat0 not folded, seat1 folded → eligible=[0]
	// Level 200: only seat1 contributes ((200-100)*1=100), folded → eligible=[]
	// The 100 from level 200 should be orphaned and folded into level 100
	// Result: should be 1 pot with 300 chips and eligible=[0]
	if len(pots) != 1 {
		t.Fatalf("pots length = %d, want 1", len(pots))
	}
	if pots[0].Amount != 300 {
		t.Errorf("pot amount = %d, want 300 (orphaned chips should be conserved)", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 1 || pots[0].Eligible[0] != 0 {
		t.Errorf("pot eligible = %v, want [0]", pots[0].Eligible)
	}
}

func TestBuildPotsConservationProperty(t *testing.T) {
	// Table-driven conservation test across varied scenarios
	testCases := []struct {
		name  string
		seats []*Seat
	}{
		{
			name:  "all folded",
			seats: []*Seat{seat(100, true), seat(100, true), seat(100, true)},
		},
		{
			name:  "mixed folded at different levels",
			seats: []*Seat{seat(50, true), seat(150, false), seat(150, true), seat(300, false)},
		},
		{
			name:  "single player",
			seats: []*Seat{seat(500, false)},
		},
		{
			name:  "no one committed",
			seats: []*Seat{seat(0, false), seat(0, false)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			committed := 0
			for _, s := range tc.seats {
				committed += s.Committed
			}
			pots := BuildPots(tc.seats)
			potTotal := 0
			for _, p := range pots {
				potTotal += p.Amount
			}
			if potTotal != committed {
				t.Errorf("%s: pots total %d != committed %d", tc.name, potTotal, committed)
			}
		})
	}
}

func TestBuildPotsAllFoldedDegenerate(t *testing.T) {
	// When everyone folds, the degenerate case should make all contributors eligible
	pots := BuildPots([]*Seat{seat(100, true), seat(200, true), seat(150, true)})
	if len(pots) != 1 {
		t.Fatalf("pots length = %d, want 1 degenerate pot", len(pots))
	}
	if pots[0].Amount != 450 {
		t.Errorf("pot amount = %d, want 450", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 3 {
		t.Errorf("eligible length = %d, want 3 (all contributors)", len(pots[0].Eligible))
	}
	// All contributors should be eligible
	expectedEligible := []int{0, 1, 2}
	for i, idx := range pots[0].Eligible {
		if i >= len(expectedEligible) || idx != expectedEligible[i] {
			t.Errorf("eligible[%d] = %d, want %d", i, idx, expectedEligible[i])
		}
	}
}
