package poker

import (
	"testing"
	"time"
)

// A hand in progress is voided on restore, and the chips committed to its
// pot go back to the players who put them there. Nothing may be created or
// destroyed: settlement is per hand, so an unfinished hand never reached a
// balance and the table must come back chip-for-chip.
func TestSnapshotReturnsCommittedChips(t *testing.T) {
	tbl := NewTable("t", -1)
	_ = tbl.Sit("a", "Dmytro", 5000)
	_ = tbl.Sit("b", "Danya", 3000)
	before := 0
	for _, s := range tbl.Seats {
		before += s.Stack
	}
	_ = tbl.StartHand() // posts blinds, moving chips into the pot

	inPlay := 0
	for _, s := range tbl.Seats {
		inPlay += s.Stack
	}
	if inPlay >= before {
		t.Fatal("setup: blinds did not leave the stacks")
	}

	restored := RestoreTable(tbl.Snapshot())
	after := 0
	for _, s := range restored.Seats {
		after += s.Stack
	}
	if after != before {
		t.Errorf("restored with %d chips, want %d — the voided pot was lost or duplicated", after, before)
	}
	if restored.Stage != StageWaiting {
		t.Errorf("stage = %v, want StageWaiting", restored.Stage)
	}
	for _, s := range restored.Seats {
		if len(s.Hole) != 0 || s.Committed != 0 || s.Bet != 0 {
			t.Errorf("seat %s carried hand state across the restart: %+v", s.UserID, s)
		}
	}
}

func TestSnapshotKeepsIdentityAndClocks(t *testing.T) {
	tbl := NewTable("t7", -4242)
	tbl.CreatedAt = time.Now().Add(-12 * time.Minute)
	_ = tbl.Sit("a", "Dmytro", 5000)
	_ = tbl.Sit("b", "Danya", 5000)
	_ = tbl.StartHand()

	r := RestoreTable(tbl.Snapshot())
	if r.ID != "t7" || r.ChatID != -4242 {
		t.Errorf("identity lost: %s / %d", r.ID, r.ChatID)
	}
	if r.Hands != tbl.Hands {
		t.Errorf("hands = %d, want %d", r.Hands, tbl.Hands)
	}
	// The session clock drives the blind schedule; resetting it would drop
	// a long table back to the opening 50/100. Twelve minutes in is level
	// two, so a surviving clock reads 200/400.
	sb, bb := BlindsAt(r.Elapsed())
	if sb != 200 || bb != 400 {
		t.Errorf("restored blinds = %d/%d, want 200/400 — the session clock did not survive", sb, bb)
	}
	if fresh, _ := BlindsAt(0); sb == fresh {
		t.Error("restored table fell back to opening blinds")
	}
}

// Bots are reseated by the hub's rule from the current human population.
// Carrying stale bot seats over could leave a table of bots with no humans.
func TestRestoreDropsBotsAndBustedSeats(t *testing.T) {
	tbl := NewTable("t", -1)
	_ = tbl.Sit("a", "Dmytro", 5000)
	_ = tbl.SitBot("bot:1", "Director Bo", 5000)
	_ = tbl.Sit("b", "Broke", 1000)
	tbl.Seats[2].Stack = 0

	r := RestoreTable(tbl.Snapshot())
	if len(r.Seats) != 1 || r.Seats[0].UserID != "a" {
		t.Errorf("restored seats = %+v, want only the funded human", r.Seats)
	}
}

// Button must stay a valid index after seats are dropped, or the first hand
// back panics in nextOccupied.
func TestRestoreKeepsButtonInRange(t *testing.T) {
	tbl := NewTable("t", -1)
	_ = tbl.Sit("a", "Dmytro", 5000)
	_ = tbl.SitBot("bot:1", "Bo", 5000)
	_ = tbl.SitBot("bot:2", "God", 5000)
	tbl.Button = 2 // a bot, which restore drops

	r := RestoreTable(tbl.Snapshot())
	if r.Button >= len(r.Seats) {
		t.Fatalf("button = %d with %d seats", r.Button, len(r.Seats))
	}
	_ = tbl.Sit("b", "Danya", 5000)
	r2 := RestoreTable(tbl.Snapshot())
	if err := r2.StartHand(); err != nil {
		t.Errorf("restored table could not deal: %v", err)
	}
}

// A restart wakes everyone: sleep state does not survive a snapshot/restore
// round-trip. This pins the intended behaviour so a future change to carry
// the Asleep field cannot silently make sleep survive a deploy.
func TestRestoreWakesAsleepSeats(t *testing.T) {
	tbl := NewTable("t", -1)
	_ = tbl.Sit("a", "Dmytro", 5000)
	_ = tbl.Sit("b", "Danya", 3000)

	// Put one player to sleep and verify it stuck.
	idx := tbl.SeatIndexOf("b")
	tbl.Seats[idx].Asleep = true
	if !tbl.Seats[idx].Asleep {
		t.Fatal("setup: failed to put seat asleep")
	}

	// Round-trip through snapshot and restore.
	snap := tbl.Snapshot()
	restored := RestoreTable(snap)

	// The restored seat must be awake, while its stack is preserved.
	restoredIdx := restored.SeatIndexOf("b")
	if restoredIdx < 0 {
		t.Fatal("restored table missing the sleeping player")
	}
	if restored.Seats[restoredIdx].Asleep {
		t.Error("restored seat is asleep, want awake after restart")
	}
	if restored.Seats[restoredIdx].Stack != 3000 {
		t.Errorf("restored seat stack = %d, want 3000 — stack not preserved", restored.Seats[restoredIdx].Stack)
	}
}
