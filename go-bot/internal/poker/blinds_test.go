package poker

import (
	"testing"
	"time"
)

func TestBlindsAtEachLevel(t *testing.T) {
	cases := []struct {
		after  time.Duration
		sb, bb int
		note   string
	}{
		{0, 50, 100, "fresh table"},
		{4 * time.Minute, 50, 100, "still level 0 just before the boundary"},
		{5 * time.Minute, 100, 200, "level 1 exactly on the boundary"},
		{9*time.Minute + 59*time.Second, 100, 200, "still level 1"},
		{10 * time.Minute, 200, 400, "level 2"},
		{15 * time.Minute, 400, 800, "level 3"},
		{20 * time.Minute, 800, 1600, "level 4 — the cap"},
		{25 * time.Minute, 800, 1600, "capped, does not keep doubling"},
		{3 * time.Hour, 800, 1600, "still capped hours later"},
		{-time.Minute, 50, 100, "negative elapsed degrades to base, never the cap"},
	}
	for _, c := range cases {
		sb, bb := BlindsAt(c.after)
		if sb != c.sb || bb != c.bb {
			t.Errorf("BlindsAt(%v) = %d/%d, want %d/%d (%s)", c.after, sb, bb, c.sb, c.bb, c.note)
		}
	}
}

// The cap is the whole reason this is safe to run on real balances: without
// it the big blind passes MaxBuyIn and every hand becomes a forced all-in.
func TestBlindCapStaysBelowMaxBuyIn(t *testing.T) {
	_, bb := BlindsAt(24 * time.Hour)
	if bb >= MaxBuyIn {
		t.Errorf("capped big blind %d >= MaxBuyIn %d: a full stack could not cover one blind", bb, MaxBuyIn)
	}
}

func TestNextBlindRaise(t *testing.T) {
	if d, more := NextBlindRaise(0); !more || d != BlindInterval {
		t.Errorf("fresh table: got %v more=%v, want %v true", d, more, BlindInterval)
	}
	if d, more := NextBlindRaise(4 * time.Minute); !more || d != time.Minute {
		t.Errorf("4min in: got %v more=%v, want 1m true", d, more)
	}
	if _, more := NextBlindRaise(20 * time.Minute); more {
		t.Error("at the cap NextBlindRaise still reports another level coming")
	}
	if _, more := NextBlindRaise(5 * time.Hour); more {
		t.Error("long past the cap NextBlindRaise still reports another level coming")
	}
}

func TestStartHandPostsCurrentLevelBlinds(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 10000)
	_ = tbl.Sit("b", "B", 10000)
	_ = tbl.Sit("c", "C", 10000)
	// Backdate the table into level 2.
	tbl.CreatedAt = time.Now().Add(-11 * time.Minute)
	if err := tbl.StartHand(); err != nil {
		t.Fatal(err)
	}
	if tbl.SmallBlind != 200 || tbl.BigBlind != 400 {
		t.Fatalf("blinds = %d/%d, want 200/400", tbl.SmallBlind, tbl.BigBlind)
	}
	posted := 0
	for _, s := range tbl.Seats {
		posted += s.Committed
	}
	if posted != 600 {
		t.Errorf("chips posted = %d, want 600 (200+400)", posted)
	}
	if tbl.MinRaise != 400 {
		t.Errorf("MinRaise = %d, want the current big blind 400", tbl.MinRaise)
	}
}

// A level boundary crossing mid-hand must NOT move the stakes underneath a
// hand in progress: blinds are read once, at StartHand.
func TestBlindsFixedForTheHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 10000)
	_ = tbl.Sit("b", "B", 10000)
	// One second short of level 1.
	tbl.CreatedAt = time.Now().Add(-(BlindInterval - time.Second))
	_ = tbl.StartHand()
	if tbl.BigBlind != 100 {
		t.Fatalf("setup: big blind = %d, want 100", tbl.BigBlind)
	}

	// Cross the boundary while the hand is live.
	tbl.CreatedAt = time.Now().Add(-(BlindInterval + time.Minute))
	if got, _ := BlindsAt(tbl.Elapsed()); got == 50 {
		t.Fatal("setup failed: clock did not cross the level boundary")
	}

	// Streets advance; MinRaise must reset to the HAND's blind, not the
	// clock's current level.
	tbl.advance()
	if tbl.BigBlind != 100 {
		t.Errorf("big blind changed mid-hand to %d, want 100", tbl.BigBlind)
	}
	if tbl.MinRaise != 100 {
		t.Errorf("post-street MinRaise = %d, want the hand's big blind 100", tbl.MinRaise)
	}

	// The NEXT hand picks up the new level.
	_ = tbl.StartHand()
	if tbl.BigBlind != 200 {
		t.Errorf("next hand big blind = %d, want 200", tbl.BigBlind)
	}
}

func TestHandsCounted(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 100000)
	_ = tbl.Sit("b", "B", 100000)
	if tbl.Hands != 0 {
		t.Fatalf("fresh table Hands = %d, want 0", tbl.Hands)
	}
	for i := 1; i <= 3; i++ {
		if err := tbl.StartHand(); err != nil {
			t.Fatal(err)
		}
		if tbl.Hands != i {
			t.Errorf("after %d hands Hands = %d", i, tbl.Hands)
		}
		if v := tbl.ViewFor("a"); v.Hands != i {
			t.Errorf("view reports %d hands, want %d", v.Hands, i)
		}
	}
}

func TestViewReportsSessionAndBlinds(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 10000)
	_ = tbl.Sit("b", "B", 10000)
	tbl.CreatedAt = time.Now().Add(-6 * time.Minute)
	_ = tbl.StartHand()

	v := tbl.ViewFor("a")
	if v.Elapsed < 355 || v.Elapsed > 365 {
		t.Errorf("elapsed = %d, want about 360", v.Elapsed)
	}
	if v.SmallBlind != 100 || v.BigBlind != 200 {
		t.Errorf("view blinds = %d/%d, want 100/200", v.SmallBlind, v.BigBlind)
	}
	if v.NextBlindIn <= 0 || v.NextBlindIn > 4*60+5 {
		t.Errorf("next_blind_in = %d, want about 240", v.NextBlindIn)
	}

	tbl.CreatedAt = time.Now().Add(-2 * time.Hour)
	if v := tbl.ViewFor("a"); v.NextBlindIn != -1 {
		t.Errorf("capped table next_blind_in = %d, want -1", v.NextBlindIn)
	}
}
