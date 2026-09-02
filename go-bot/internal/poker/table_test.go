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
