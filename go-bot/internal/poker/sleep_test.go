package poker

import (
	"testing"
	"time"
)

// A player who times out is asleep afterwards.
func TestForceTimeoutPutsActorToSleep(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if !actor.Asleep {
		t.Error("player who timed out should be asleep afterwards")
	}
}

// Falling asleep itself must not move any chips: the auto-action (a free
// check here) is the only thing allowed to touch the stack.
func TestForceTimeoutToSleepMovesNoExtraChips(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	_ = tbl.Act(first, ActCall, 0) // BB can check now
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	stackBefore := actor.Stack
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if actor.Folded {
		t.Fatal("setup: expected a free check, not a fold")
	}
	if actor.Stack != stackBefore {
		t.Errorf("stack moved from %d to %d just from falling asleep", stackBefore, actor.Stack)
	}
	if !actor.Asleep {
		t.Error("player who timed out should be asleep afterwards")
	}
}

// A sleeping player is NOT dealt in on the next hand: no hole cards, InHand
// false, and folded so betting logic (liveCount, bettingClosed) treats them
// exactly like a busted seat.
func TestSleepingPlayerNotDealtInNextHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.Sit("c", "C", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Run hand 1 down to showdown so the next StartHand is a clean deal.
	for tbl.Stage != StageShowdown {
		actor := tbl.Seats[tbl.ToAct]
		if err := tbl.Act(actor.UserID, ActFold, 0); err != nil {
			t.Fatalf("Act fold: %v", err)
		}
	}
	tbl.Showdown()

	idx := tbl.SeatIndexOf("c")
	tbl.Seats[idx].Asleep = true

	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	s := tbl.Seats[idx]
	if s.InHand {
		t.Error("sleeping seat should not be dealt into the hand")
	}
	if len(s.Hole) != 0 {
		t.Errorf("sleeping seat has %d hole cards, want 0", len(s.Hole))
	}
	if !s.Folded {
		t.Error("sleeping seat should be marked folded, like a busted seat")
	}
}

// The regression that matters most: a sleeping player posts NO blind. Deal
// several hands with the button rotating and their stack must never move.
func TestSleepingPlayerPostsNoBlind(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.Sit("c", "C", 5000)
	idx := tbl.SeatIndexOf("c")
	tbl.Seats[idx].Asleep = true

	for i := 0; i < 6; i++ {
		if err := tbl.StartHand(); err != nil {
			t.Fatalf("hand %d: StartHand: %v", i, err)
		}
		if tbl.Seats[idx].Stack != 5000 {
			t.Fatalf("hand %d: sleeping seat's stack = %d, want untouched at 5000", i, tbl.Seats[idx].Stack)
		}
		if tbl.Seats[idx].Committed != 0 {
			t.Fatalf("hand %d: sleeping seat committed %d, want 0 — it posted a blind while asleep", i, tbl.Seats[idx].Committed)
		}
		if tbl.Button == idx {
			t.Fatalf("hand %d: button landed on the sleeping seat", i)
		}
	}
}

// Three seats, one asleep: the hand must use the heads-up blind rule, since
// only two seats are genuinely in play.
func TestThreeSeatsOneAsleepUsesHeadsUpBlindRule(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	_ = tbl.Sit("c", "C", 5000)
	tbl.Seats[tbl.SeatIndexOf("c")].Asleep = true

	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if tbl.SeatedCount() != 2 {
		t.Fatalf("SeatedCount = %d, want 2 with one seat asleep", tbl.SeatedCount())
	}
	// Heads-up rule: the button itself posts the small blind.
	button := tbl.Seats[tbl.Button]
	if button.Committed != tbl.SmallBlind {
		t.Errorf("button seat committed %d, want the small blind %d (heads-up rule)", button.Committed, tbl.SmallBlind)
	}
	sleeper := tbl.Seats[tbl.SeatIndexOf("c")]
	if sleeper.Committed != 0 {
		t.Errorf("sleeping seat committed %d, want 0", sleeper.Committed)
	}
	if sleeper.InHand {
		t.Error("sleeping seat should not be in hand")
	}
	posted := 0
	for _, s := range tbl.Seats {
		posted += s.Committed
	}
	if posted != tbl.SmallBlind+tbl.BigBlind {
		t.Errorf("total posted = %d, want %d (sb+bb only)", posted, tbl.SmallBlind+tbl.BigBlind)
	}
}

// Wake puts them back in the next hand.
func TestWakePutsBackInNextHand(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	idx := tbl.SeatIndexOf("b")
	tbl.Seats[idx].Asleep = true

	if !tbl.Wake("b") {
		t.Fatal("Wake reported no change for a sleeping seat")
	}
	if tbl.Seats[idx].Asleep {
		t.Error("Wake did not clear Asleep")
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if !tbl.Seats[idx].InHand {
		t.Error("woken seat should be dealt into the next hand")
	}
	if len(tbl.Seats[idx].Hole) != 2 {
		t.Errorf("woken seat has %d hole cards, want 2", len(tbl.Seats[idx].Hole))
	}
}

// Wake is a no-op (and does not bump Seq) when the seat was never asleep, or
// when userID has no seat at all.
func TestWakeNoopWhenNotAsleep(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	seq := tbl.Seq
	if tbl.Wake("a") {
		t.Error("Wake reported a change for a seat that was never asleep")
	}
	if tbl.Seq != seq {
		t.Error("Wake bumped Seq despite no change")
	}
	if tbl.Wake("nobody") {
		t.Error("Wake reported a change for a user with no seat")
	}
}

// A table where everyone is asleep does not deal.
func TestStartHandFailsWhenEveryoneAsleep(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	_ = tbl.Sit("b", "B", 5000)
	for _, s := range tbl.Seats {
		s.Asleep = true
	}
	if err := tbl.StartHand(); err != ErrNeedPlayers {
		t.Fatalf("StartHand with everyone asleep = %v, want ErrNeedPlayers", err)
	}
}

// A fresh seat (join) and a re-bought seat must never start asleep.
func TestFreshSeatIsNotAsleep(t *testing.T) {
	tbl := NewTable("t", 1)
	_ = tbl.Sit("a", "A", 5000)
	if tbl.Seats[0].Asleep {
		t.Error("a freshly seated player must not start asleep")
	}
}
