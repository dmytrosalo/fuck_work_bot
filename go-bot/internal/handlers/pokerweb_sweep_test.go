package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// --- auto-start on join (item 2) -------------------------------------------

// TestHandleJoinAutoStartsHandOnSecondPlayer proves a table that fills to
// two seats actually deals a hand, rather than sitting forever in
// StageWaiting with nothing to ever call StartHand.
func TestHandleJoinAutoStartsHandOnSecondPlayer(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 5000-100)
	db.UpdateBalance("222", "Bob", 5000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	join := func(uid int64, name string) *httptest.ResponseRecorder {
		initData := userInitData(t, "test-token", uid, name, "")
		req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec1 := join(111, "Alice")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first join status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}
	tbl.Lock()
	stage := tbl.Stage
	tbl.Unlock()
	if stage != poker.StageWaiting {
		t.Fatalf("stage after first join = %v, want StageWaiting (need 2 players)", stage)
	}

	rec2 := join(222, "Bob")
	if rec2.Code != http.StatusOK {
		t.Fatalf("second join status = %d, want 200, body=%s", rec2.Code, rec2.Body.String())
	}

	tbl.Lock()
	defer tbl.Unlock()
	if tbl.Stage != poker.StagePreflop {
		t.Fatalf("stage after second join = %v, want StagePreflop (hand must auto-start)", tbl.Stage)
	}
	if tbl.ToAct < 0 {
		t.Error("ToAct not set after auto-started hand")
	}
	for _, s := range tbl.Seats {
		if len(s.Hole) != 2 {
			t.Errorf("seat %s has %d hole cards, want 2 after auto-started hand", s.UserID, len(s.Hole))
		}
	}
}

// --- sweepOnce: forced timeout + settlement (item 1 wired to item 2/4) ----

// TestSweepOnceForcesExpiredTimeoutAndSettlesOnce drives a heads-up hand to
// showdown purely via an expired turn clock (no HTTP action at all), then
// checks the sweeper settled the balances exactly once using the same
// h.settle helper handleAction uses — never twice, and with an empty name so
// the player's stored display name isn't clobbered.
func TestSweepOnceForcesExpiredTimeoutAndSettlesOnce(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 900)
	db.UpdateBalance("222", "Bob", 900)

	h := NewPokerHub(db, nil, "test-token")
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Force the clock to have expired already, exactly what a player closing
	// Telegram mid-hand would look like from the sweeper's point of view.
	tbl.Lock()
	tbl.Deadline = time.Now().Add(-time.Second)
	tbl.Unlock()

	h.sweepOnce()

	tbl.Lock()
	stage := tbl.Stage
	tbl.Unlock()
	if stage != poker.StageShowdown {
		t.Fatalf("stage after sweep = %v, want StageShowdown (heads-up auto-fold ends the hand)", stage)
	}

	aliceAfter := db.GetBalance("111", "")
	bobAfter := db.GetBalance("222", "")
	if aliceAfter+bobAfter != 2000 {
		t.Fatalf("balances not zero-sum: alice=%d bob=%d, want sum 2000", aliceAfter, bobAfter)
	}
	if aliceAfter == 1000 && bobAfter == 1000 {
		t.Fatalf("balances unchanged (%d/%d) — sweeper did not settle the hand", aliceAfter, bobAfter)
	}

	// A second sweep pass must not settle again: the hand is over and
	// ForceTimeout is a no-op once in showdown (guarded by Stage), and the
	// table has 2 seated players with chips so this pass instead starts the
	// next hand — balances must stay exactly where the first settle left
	// them.
	h.sweepOnce()
	if got := db.GetBalance("111", ""); got != aliceAfter {
		t.Errorf("alice balance after second sweep = %d, want unchanged %d (no double-settle)", got, aliceAfter)
	}
	if got := db.GetBalance("222", ""); got != bobAfter {
		t.Errorf("bob balance after second sweep = %d, want unchanged %d (no double-settle)", got, bobAfter)
	}
}

// --- sweepOnce: next hand after showdown (item 2) --------------------------

// TestSweepOnceStartsNextHandAfterShowdownPause proves a table does not play
// exactly one hand and stop: once a hand has settled into showdown (from a
// PRIOR pass — see the "short pause" note below) and 2+ players still have
// chips, the next sweep pass deals a new hand.
func TestSweepOnceStartsNextHandAfterShowdownPause(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 900)
	db.UpdateBalance("222", "Bob", 900)

	h := NewPokerHub(db, nil, "test-token")
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Drive the hand to showdown and settle it exactly the way handleAction
	// would, WITHOUT going through the sweeper — this puts the table into
	// "showdown, already settled, from a previous pass" so the very first
	// sweepOnce call below is the "later pass" that must deal the next hand,
	// not the same tick that caused the transition.
	actor := tbl.Seats[tbl.ToAct].UserID
	tbl.Lock()
	prevStage := tbl.Stage
	if err := tbl.Act(actor, poker.ActFold, 0); err != nil {
		tbl.Unlock()
		t.Fatalf("Act: %v", err)
	}
	if tbl.Stage == poker.StageShowdown && prevStage != poker.StageShowdown {
		h.settle(tbl)
	}
	tbl.Unlock()

	tbl.Lock()
	stage := tbl.Stage
	tbl.Unlock()
	if stage != poker.StageShowdown {
		t.Fatalf("stage before sweep = %v, want StageShowdown", stage)
	}

	h.sweepOnce()

	tbl.Lock()
	defer tbl.Unlock()
	if tbl.Stage == poker.StageShowdown {
		t.Fatal("stage still StageShowdown after sweep — next hand never started, table is stuck")
	}
	if tbl.Stage != poker.StagePreflop {
		t.Errorf("stage after sweep = %v, want StagePreflop (fresh hand dealt)", tbl.Stage)
	}
}

// TestSweepOnceDoesNotStartNextHandWithoutTwoFundedPlayers proves a busted
// player (0 chips) correctly stops the auto-deal loop instead of trying to
// start a hand that StartHand itself would reject.
func TestSweepOnceDoesNotStartNextHandWithoutTwoFundedPlayers(t *testing.T) {
	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "test-token")
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Manually park the table in showdown with one seat already busted, as
	// if Bob lost his whole stack in the settled hand.
	tbl.Lock()
	tbl.Stage = poker.StageShowdown
	tbl.Seats[0].Stack = 4000
	tbl.Seats[1].Stack = 0
	tbl.Unlock()

	h.sweepOnce()

	tbl.Lock()
	defer tbl.Unlock()
	if tbl.Stage != poker.StageShowdown {
		t.Errorf("stage = %v, want unchanged StageShowdown — only one player has chips left", tbl.Stage)
	}
}

// --- sweepOnce: idle table reclamation and seat-claim release (items 3+4) -

// TestSweepOnceReclaimsIdleTableReleasesClaimsAndDropsSubscribers is the
// merge-blocker test for items 3 and 4: an idle table (no activity for
// longer than idleTableTimeout) must be deleted from the hub, every
// hub-wide seat claim pointing at it released so its players can join
// elsewhere, and its subscriber list dropped so any still-connected SSE
// goroutine is signalled to exit via sub.done.
func TestSweepOnceReclaimsIdleTableReleasesClaimsAndDropsSubscribers(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 5000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	// Seat one real player via the actual HTTP path so seatedAt carries a
	// real claim, exactly as production traffic would leave it.
	initData := userInitData(t, "test-token", 111, "Alice", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("join status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	// A still-connected SSE subscriber watching this table, added directly
	// (same package) rather than opening a real streaming connection —
	// sweepOnce must close its done channel so the (real, in production)
	// goroutine blocked in handleStream's select exits.
	sub := &subscriber{userID: "111", ch: make(chan poker.TableView, 1), done: make(chan struct{})}
	h.mu.Lock()
	h.subs[tbl.ID] = append(h.subs[tbl.ID], sub)
	h.lastActivity[tbl.ID] = time.Now().Add(-(idleTableTimeout + time.Minute))
	h.mu.Unlock()

	h.sweepOnce()

	if h.table(tbl.ID) != nil {
		t.Error("table still present in the hub after it should have been reclaimed as idle")
	}

	h.mu.Lock()
	_, stillClaimed := h.seatedAt["111"]
	_, hasSubs := h.subs[tbl.ID]
	_, hasActivity := h.lastActivity[tbl.ID]
	h.mu.Unlock()
	if stillClaimed {
		t.Error("seat claim for user 111 not released after table reclaim — user is now locked out of every table")
	}
	if hasSubs {
		t.Error("subscriber list for reclaimed table not dropped")
	}
	if hasActivity {
		t.Error("lastActivity entry for reclaimed table not cleaned up")
	}

	select {
	case <-sub.done:
		// expected: closed, so a blocked handleStream goroutine can exit
	default:
		t.Error("subscriber's done channel not closed — a real SSE goroutine would idle forever")
	}

	// The freed claim must actually be usable: the same user joining a
	// DIFFERENT table must now succeed, proving item 3 end-to-end rather
	// than just inspecting the map.
	tbl2 := h.Create(2)
	req2 := httptest.NewRequest("POST", "/api/poker/"+tbl2.ID+"/join", nil)
	req2.Header.Set("X-Telegram-Init-Data", initData)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("join to a new table after reclaim status = %d, want 200, body=%s", rec2.Code, rec2.Body.String())
	}
}

// TestSweepOnceReclaimsIdleTableRegardlessOfSeatedPlayers proves reclamation
// does not wait for a table to be empty: an abandoned table with players
// still seated is exactly the case that would otherwise strand their seat
// claims forever, so idleness alone must be enough to reclaim it.
func TestSweepOnceReclaimsIdleTableRegardlessOfSeatedPlayers(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(1)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	h.mu.Lock()
	h.lastActivity[tbl.ID] = time.Now().Add(-(idleTableTimeout + time.Minute))
	h.mu.Unlock()

	h.sweepOnce()

	if h.table(tbl.ID) != nil {
		t.Error("table with seated players not reclaimed despite being idle beyond idleTableTimeout")
	}
}

// TestSweepOnceDoesNotReclaimActiveTable is the negative case: a table with
// recent activity (as any table freshly created by h.Create is) must never
// be swept away, however many passes run.
func TestSweepOnceDoesNotReclaimActiveTable(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(1)

	h.sweepOnce()
	h.sweepOnce()
	h.sweepOnce()

	if h.table(tbl.ID) == nil {
		t.Fatal("freshly created table was reclaimed despite being active")
	}
}
