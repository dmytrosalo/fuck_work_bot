package handlers

import (
	"testing"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

func countBots(tbl *poker.Table) int {
	n := 0
	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) {
			n++
		}
	}
	return n
}

func TestEnsureBotsSeatingTable(t *testing.T) {
	cases := []struct{ humans, wantBots int }{
		{1, 2}, {2, 2}, {3, 2}, {4, 2}, {5, 1}, {6, 0},
	}
	for _, tc := range cases {
		h := NewPokerHub(nil, nil, "test-token")
		tbl := h.Create(1)
		tbl.Lock()
		for i := 0; i < tc.humans; i++ {
			if err := tbl.Sit("u"+string(rune('a'+i)), "H", 5000); err != nil {
				t.Fatalf("%d humans: seat %d: %v", tc.humans, i, err)
			}
		}
		h.ensureBots(tbl)
		got := countBots(tbl)
		tbl.Unlock()
		if got != tc.wantBots {
			t.Errorf("%d humans: got %d bots, want %d", tc.humans, got, tc.wantBots)
		}
		if total := len(tbl.Seats); total > poker.MaxSeats {
			t.Errorf("%d humans: table has %d seats, over the %d cap", tc.humans, total, poker.MaxSeats)
		}
	}
}

func TestEnsureBotsMatchesTopHumanStack(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(1)
	tbl.Lock()
	_ = tbl.Sit("u1", "Danya", 4000)
	_ = tbl.Sit("u2", "Data", 9000)
	h.ensureBots(tbl)
	tbl.Unlock()

	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) && s.Stack != 9000 {
			t.Errorf("bot %s stack = %d, want 9000 (the top human stack)", s.UserID, s.Stack)
		}
	}
}

func TestEnsureBotsRebuysBustedBot(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(1)
	tbl.Lock()
	_ = tbl.Sit("u1", "Danya", 7000)
	h.ensureBots(tbl)
	// Bust one bot.
	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) {
			s.Stack = 0
			break
		}
	}
	h.ensureBots(tbl)
	tbl.Unlock()

	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) && s.Stack != 7000 {
			t.Errorf("bot %s stack = %d, want 7000 after rebuy", s.UserID, s.Stack)
		}
	}
}

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
