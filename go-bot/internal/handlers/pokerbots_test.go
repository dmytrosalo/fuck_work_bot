package handlers

import (
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

func TestSettleRoutesBotDeltasToBank(t *testing.T) {
	db := setupTestDB(t) // the handlers-package helper, pokerweb_test.go:55
	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)

	db.UpdateBalance("u1", "Danya", 9900) // 100 seed + 9900 = 10000
	tbl.Lock()
	_ = tbl.Sit("u1", "Danya", 10000)
	_ = tbl.SitBot("bot:1", "Вася", 10000)
	_ = tbl.SitBot("bot:2", "Вова", 10000)
	_ = tbl.StartHand()
	tbl.Unlock()

	// Drive to showdown. Seat.Bet is exported, so the test computes the high
	// bet itself rather than needing an engine accessor. Bound the loop to
	// prevent hangs on engine regressions.
	const maxIterations = 500
	for i := 0; i < maxIterations; i++ {
		tbl.Lock()
		if tbl.Stage == poker.StageShowdown {
			tbl.Unlock()
			break
		}
		high := 0
		for _, o := range tbl.Seats {
			if o.Bet > high {
				high = o.Bet
			}
		}
		s := tbl.Seats[tbl.ToAct]
		act := poker.ActCheck
		if s.Bet < high {
			act = poker.ActCall
		}
		if err := tbl.Act(s.UserID, act, 0); err != nil {
			tbl.Unlock()
			t.Fatalf("Act returned unexpected error: %v", err)
		}
		tbl.Unlock()
	}

	// Verify we reached showdown
	tbl.Lock()
	if tbl.Stage != poker.StageShowdown {
		tbl.Unlock()
		t.Fatal("did not reach showdown within max iterations")
	}
	tbl.Unlock()

	// Record balances before and after settle. GetBalance SEEDS an unknown
	// user at 100, so it cannot be used to assert a row's absence — reading
	// it would create it. Assert the real property instead: the human's change
	// and the bank's change cancel. Keep retrying hands until we get non-zero
	// deltas (avoiding ~5% chopped-pot failures).
	for attempt := 0; attempt < 20; attempt++ {
		humanBefore := db.GetBalance("u1", "")
		bankBefore := db.GetBalance(bankUserID, "")
		tbl.Lock()
		h.settle(tbl)
		tbl.Unlock()
		humanDelta := db.GetBalance("u1", "") - humanBefore
		bankDelta := db.GetBalance(bankUserID, "") - bankBefore

		if humanDelta+bankDelta != 0 {
			t.Errorf("human %+d and bank %+d do not cancel — zero-sum broken", humanDelta, bankDelta)
		}
		if humanDelta != 0 {
			// Successfully got a hand with movement; now verify the property
			pokerRowCount, err := db.CountPokerTransactions()
			if err != nil {
				t.Fatalf("failed to count poker rows: %v", err)
			}
			expectedRows := 2 // 1 human + 1 bank
			if pokerRowCount != expectedRows {
				t.Errorf("poker transaction rows = %d, want %d (1 human + 1 bank summing 2 bots)", pokerRowCount, expectedRows)
			}
			return // Test passed
		}

		// Chopped pot, start another hand
		tbl.Lock()
		_ = tbl.StartHand()
		tbl.Unlock()

		// Drive to showdown again
		for i := 0; i < maxIterations; i++ {
			tbl.Lock()
			if tbl.Stage == poker.StageShowdown {
				tbl.Unlock()
				break
			}
			high := 0
			for _, o := range tbl.Seats {
				if o.Bet > high {
					high = o.Bet
				}
			}
			s := tbl.Seats[tbl.ToAct]
			act := poker.ActCheck
			if s.Bet < high {
				act = poker.ActCall
			}
			if err := tbl.Act(s.UserID, act, 0); err != nil {
				tbl.Unlock()
				t.Fatalf("Act returned unexpected error: %v", err)
			}
			tbl.Unlock()
		}
	}

	t.Error("could not produce a non-zero hand delta after 20 attempts (1 in ~5^20 chance)")
}
