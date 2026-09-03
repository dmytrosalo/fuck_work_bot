package poker

import "testing"

func TestSitRejectsBelowMinBuyIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	if err := tbl.Sit("u1", "Danya", MinBuyIn-1); err == nil {
		t.Fatal("expected error for buy-in below minimum")
	}
}

func TestSitRejectsWhenFull(t *testing.T) {
	tbl := NewTable("t1", 1)
	for i := 0; i < MaxSeats; i++ {
		if err := tbl.Sit(string(rune('a'+i)), "P", MinBuyIn); err != nil {
			t.Fatalf("seat %d: %v", i, err)
		}
	}
	if err := tbl.Sit("overflow", "P", MinBuyIn); err == nil {
		t.Fatal("expected error when table is full")
	}
}

func TestSitRejectsDuplicateUser(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", MinBuyIn)
	if err := tbl.Sit("u1", "Danya", MinBuyIn); err == nil {
		t.Fatal("expected error when the same user sits twice")
	}
}

func TestStartHandNeedsTwoPlayers(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	if err := tbl.StartHand(); err == nil {
		t.Fatal("expected error starting a hand with one player")
	}
}

func TestStartHandPostsBlindsAndDeals(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if tbl.Stage != StagePreflop {
		t.Errorf("stage = %v, want preflop", tbl.Stage)
	}
	posted := 0
	for _, s := range tbl.Seats {
		if len(s.Hole) != 2 {
			t.Errorf("seat %s has %d hole cards, want 2", s.UserID, len(s.Hole))
		}
		posted += s.Committed
	}
	if posted != SmallBlind+BigBlind {
		t.Errorf("blinds posted = %d, want %d", posted, SmallBlind+BigBlind)
	}
	if tbl.MinRaise != BigBlind {
		t.Errorf("MinRaise = %d, want %d", tbl.MinRaise, BigBlind)
	}
}

func TestShortStackPostsBlindAllInNotNegative(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", MinBuyIn)
	_ = tbl.Sit("u2", "Data", 5000)
	tbl.Seats[0].Stack = 20 // less than the small blind
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for _, s := range tbl.Seats {
		if s.Stack < 0 {
			t.Fatalf("seat %s has negative stack %d", s.UserID, s.Stack)
		}
	}
	if !tbl.Seats[0].AllIn {
		t.Error("short stack should be marked all-in after posting a partial blind")
	}
}

func TestHeadsUpButtonIsSmallBlind(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	buttonSeat := tbl.Seats[tbl.Button]
	otherIdx := (tbl.Button + 1) % 2
	otherSeat := tbl.Seats[otherIdx]

	// In heads-up, button is SB, other seat is BB
	if buttonSeat.Committed != SmallBlind {
		t.Errorf("button seat committed %d, want SmallBlind (%d)", buttonSeat.Committed, SmallBlind)
	}
	if otherSeat.Committed != BigBlind {
		t.Errorf("other seat committed %d, want BigBlind (%d)", otherSeat.Committed, BigBlind)
	}

	// ToAct preflop should be the small blind (button)
	if tbl.ToAct != tbl.Button {
		t.Errorf("ToAct = %d, want button seat %d (small blind acts first preflop)", tbl.ToAct, tbl.Button)
	}
}

func TestThreeSeatsButtonSBBBRotation(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.Sit("u3", "Bo", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// With 3 seats, Button, SB, and BB should be on three DISTINCT seats
	button := tbl.Button
	sb := (button + 1) % 3
	bb := (button + 2) % 3

	if button == sb || button == bb || sb == bb {
		t.Error("Button, SB, and BB should be on distinct seats")
	}

	// Verify the committed amounts match
	if tbl.Seats[button].Committed != 0 {
		t.Errorf("button (seat %d) committed %d, want 0", button, tbl.Seats[button].Committed)
	}
	if tbl.Seats[sb].Committed != SmallBlind {
		t.Errorf("SB (seat %d) committed %d, want %d", sb, tbl.Seats[sb].Committed, SmallBlind)
	}
	if tbl.Seats[bb].Committed != BigBlind {
		t.Errorf("BB (seat %d) committed %d, want %d", bb, tbl.Seats[bb].Committed, BigBlind)
	}

	// ToAct preflop should be the seat after big blind (UTG)
	utg := (bb + 1) % 3
	if tbl.ToAct != utg {
		t.Errorf("ToAct = %d, want UTG (seat after BB) %d", tbl.ToAct, utg)
	}
}
