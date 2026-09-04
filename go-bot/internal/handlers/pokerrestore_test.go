package handlers

import (
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// A restored table has its players already seated, so handleJoin's
// auto-start never runs for them — they take the reconnect fast path and
// return first. The sweeper has to deal it, or the table sits at
// "Очікування" forever with everyone staring at an empty felt.
func TestSweeperDealsARestoredTable(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := poker.RestoreTable(poker.TableSnapshot{
		ID: "restored", ChatID: -1, Hands: 31,
		Seats: []poker.SeatSnapshot{
			{UserID: "42", Name: "Dmytro", Stack: 15040},
			{UserID: "43", Name: "Danya", Stack: 12799},
		},
	})
	h.mu.Lock()
	h.tables[tbl.ID] = tbl
	h.mu.Unlock()

	if tbl.Stage != poker.StageWaiting {
		t.Fatalf("setup: stage = %v", tbl.Stage)
	}

	h.sweepOnce()

	tbl.Lock()
	defer tbl.Unlock()
	if tbl.Stage == poker.StageWaiting {
		t.Fatal("restored table still waiting after a sweep — nothing deals it")
	}
	if tbl.Hands != 32 {
		t.Errorf("hands = %d, want the restored 31 plus the new one", tbl.Hands)
	}
	// Bots must be seated too, or a two-human table restored at a moment
	// when both are away never fills.
	bots := 0
	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) {
			bots++
		}
	}
	if bots == 0 {
		t.Error("no bots seated at the restored table")
	}
}

// A brand-new table nobody has joined must NOT seat bots to play against
// themselves.
func TestSweeperLeavesAnEmptyTableAlone(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := h.Create(-1)

	h.sweepOnce()

	tbl.Lock()
	defer tbl.Unlock()
	if len(tbl.Seats) != 0 {
		t.Errorf("empty table was seated with %d players", len(tbl.Seats))
	}
	if tbl.Stage != poker.StageWaiting {
		t.Errorf("empty table started a hand: stage = %v", tbl.Stage)
	}
}

// Clients drop any snapshot whose seq is not newer than the last they saw.
// A restored table whose seq restarted low is therefore invisible to every
// already-open page: it ignores all updates and every action is rejected as
// stale. The restored seq must land clear of anything a client holds.
func TestRestoredSeqOutrunsAnOpenClient(t *testing.T) {
	live := poker.NewTable("t", -1)
	_ = live.Sit("42", "Dmytro", 5000)
	_ = live.Sit("43", "Danya", 5000)
	for i := 0; i < 40; i++ {
		_ = live.StartHand()
	}
	clientSaw := live.Seq
	if clientSaw == 0 {
		t.Fatal("setup: seq never advanced")
	}

	restored := poker.RestoreTable(live.Snapshot())
	if restored.Seq <= clientSaw {
		t.Errorf("restored seq %d does not exceed the %d a client already saw — "+
			"every snapshot would be dropped and every action rejected",
			restored.Seq, clientSaw)
	}
}
