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
	_ = tbl.StartHand()
	tbl.Unlock()

	// Drive to showdown. Seat.Bet is exported, so the test computes the high
	// bet itself rather than needing an engine accessor.
	for {
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
		_ = tbl.Act(s.UserID, act, 0)
		tbl.Unlock()
	}

	// NOTE: GetBalance SEEDS an unknown user at 100, so it cannot be used to
	// assert a row's absence — reading it would create it. Assert the real
	// property instead: the human's change and the bank's change cancel.
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
	if humanDelta == 0 {
		t.Error("nothing settled; the hand did not move chips, so this proves nothing")
	}
}
