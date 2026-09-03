package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

	// A second sweep pass, run immediately with no time gap, must not
	// settle again: the hand is over and ForceTimeout is a no-op once in
	// showdown (guarded by Stage), and the showdown-pause gate (FIX 5)
	// means the table also does NOT auto-start the next hand on this
	// immediate second pass — either way, balances must stay exactly where
	// the first settle left them.
	h.sweepOnce()
	if got := db.GetBalance("111", ""); got != aliceAfter {
		t.Errorf("alice balance after second sweep = %d, want unchanged %d (no double-settle)", got, aliceAfter)
	}
	if got := db.GetBalance("222", ""); got != bobAfter {
		t.Errorf("bob balance after second sweep = %d, want unchanged %d (no double-settle)", got, bobAfter)
	}
}

// --- sweepOnce: next hand after showdown (item 2 / FIX 5) ------------------

// TestSweepOnceDoesNotStartNextHandBeforeShowdownPauseElapses is the
// regression guard for FIX 5: the showdown reveal must actually be visible
// for at least one sweep interval. A hand settled a moment ago (showdownAt
// only microseconds old) must NOT have its next hand auto-started by the
// very next sweep pass — checking merely "the table happens to be in
// StageShowdown right now" is true equally a millisecond after settling and
// a full sweepInterval after settling, so that alone cannot gate the pause;
// only an actual elapsed-time check (showdownAt/showdownReady) can.
func TestSweepOnceDoesNotStartNextHandBeforeShowdownPauseElapses(t *testing.T) {
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

	// Settle the hand exactly as handleAction would, WITHOUT going through
	// the sweeper, moments before the sweep pass below — showdownAt is
	// therefore only microseconds old when sweepOnce runs, exactly the case
	// that must NOT auto-deal the next hand.
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
	if tbl.Stage != poker.StageShowdown {
		t.Fatalf("stage after sweep = %v, want still StageShowdown — the next hand must not deal before players get at least one sweep interval to see the reveal", tbl.Stage)
	}
}

// TestSweepOnceStartsNextHandAfterShowdownPauseElapses proves a table does
// not play exactly one hand and stop: once a hand has settled into showdown
// AND at least one full sweepInterval has genuinely elapsed since (not
// merely "the table happens to still be in showdown when this pass
// begins" — see the sibling test above for why that alone is insufficient)
// and 2+ players still have chips, the next sweep pass deals a new hand.
func TestSweepOnceStartsNextHandAfterShowdownPauseElapses(t *testing.T) {
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

	// Simulate real time having passed since the hand settled, well beyond
	// one sweepInterval — exactly what a real deployment reaches one tick
	// (5s) after showdown.
	h.mu.Lock()
	h.showdownAt[tbl.ID] = time.Now().Add(-(sweepInterval + time.Second))
	h.mu.Unlock()

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

// --- regression guard: a busted player must not be locked out hub-wide
// until their table goes idle (FIX 4) ---------------------------------------

// TestSettleReleasesSeatClaimForBustedPlayer is the merge-blocker test for
// FIX 4: a player whose seat reaches 0 chips at settlement must be free to
// join a DIFFERENT table immediately, not remain locked out hub-wide until
// THIS table goes a full 30 idle minutes without activity — which never
// happens while the winner keeps playing. A still-funded player at the same
// table must keep their own claim untouched.
func TestSettleReleasesSeatClaimForBustedPlayer(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 5000-100)
	// Bob's real balance is well above poker.MaxBuyIn, so his buy-in at
	// table 1 is clamped to MaxBuyIn and losing that ENTIRE at-table stack
	// still leaves real balance behind for a fresh buy-in elsewhere —
	// exactly the realistic case FIX 4 is about. (A player who genuinely
	// brings their whole bankroll to one table and loses all of it has, in
	// fact, nothing left to rebuy with anywhere — that is not a bug.)
	db.UpdateBalance("222", "Bob", 20000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	join := func(uid int64, name string) {
		t.Helper()
		initData := userInitData(t, "test-token", uid, name, "")
		req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("join uid=%d status = %d, want 200, body=%s", uid, rec.Code, rec.Body.String())
		}
	}
	join(111, "Alice")
	join(222, "Bob") // second join auto-starts the hand

	// Simulate Bob busting his entire stack in this hand: park the table in
	// showdown with Bob at 0 chips and folded (same manual-override pattern
	// as TestSweepOnceDoesNotStartNextHandWithoutTwoFundedPlayers above).
	// Committed is zeroed on both seats so Showdown() has no pot left to
	// award — otherwise it would happily hand Bob's real (randomly dealt)
	// winning hand a share of the blinds pot and put his stack back above
	// 0, making the very thing this test checks nondeterministic.
	tbl.Lock()
	tbl.Stage = poker.StageShowdown
	tbl.Seats[0].Stack = 4000
	tbl.Seats[0].Committed = 0
	tbl.Seats[1].Stack = 0
	tbl.Seats[1].Committed = 0
	tbl.Seats[1].Folded = true
	h.settle(tbl)
	tbl.Unlock()

	h.mu.Lock()
	_, bobStillClaimed := h.seatedAt["222"]
	aliceTable, aliceStillClaimed := h.seatedAt["111"]
	h.mu.Unlock()
	if bobStillClaimed {
		t.Error("Bob's seatedAt claim not released after busting to 0 chips — he is now locked out of every other table")
	}
	if !aliceStillClaimed || aliceTable != tbl.ID {
		t.Errorf("Alice's seatedAt claim changed after settlement even though she still has chips: claimed=%v table=%q, want tbl.ID=%q", aliceStillClaimed, aliceTable, tbl.ID)
	}

	// End-to-end, not just an inspected map: Bob must actually be able to
	// join a DIFFERENT table now.
	tbl2 := h.Create(2)
	initData := userInitData(t, "test-token", 222, "Bob", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl2.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("busted player join to a different table status = %d, want 200, body=%s", rec.Code, rec.Body.String())
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

// --- regression guard: sweeper-driven events must NOT refresh idleness ----

// TestSweepOnceReclaimsTableWhoseOnlyEventsAreForcedTimeouts is the
// regression guard for the corrected touch policy: only PLAYER-INITIATED
// events (a real join, a real action) may refresh a table's activity.
// Sweeper-fired forced timeouts and sweeper-started hands must NOT — they
// are evidence of absence, not activity. Without this, two players who go
// permanently AFK while still funded keep the sweeper auto-folding them
// forever, which (with the old, wrong touch-on-sweeper-event behaviour)
// would refresh lastActivity every pass, so the table would never go idle,
// would never be reclaimed, and both players' seatedAt claims would be
// stuck forever — exactly the failure item 3 exists to prevent.
func TestSweepOnceReclaimsTableWhoseOnlyEventsAreForcedTimeouts(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 5000-100)
	db.UpdateBalance("222", "Bob", 5000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	join := func(uid int64, name string) {
		t.Helper()
		initData := userInitData(t, "test-token", uid, name, "")
		req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("join uid=%d status = %d, want 200, body=%s", uid, rec.Code, rec.Body.String())
		}
	}
	join(111, "Alice")
	join(222, "Bob") // second join auto-starts the hand — this is the LAST real activity this table ever sees

	forceExpiredTurn := func() {
		tbl.Lock()
		if tbl.Stage != poker.StageWaiting && tbl.Stage != poker.StageShowdown {
			tbl.Deadline = time.Now().Add(-time.Second)
		}
		tbl.Unlock()
	}

	// Several sweep passes, each forcing an expired turn (and, once a hand
	// ends, auto-starting the next one) — simulating the sweeper being the
	// ONLY thing that ever touches this table again, i.e. both players are
	// permanently AFK but still funded. This must not, by itself, prevent
	// the table from eventually going idle.
	for i := 0; i < 3; i++ {
		forceExpiredTurn()
		h.sweepOnce()
	}
	if h.table(tbl.ID) == nil {
		t.Fatal("table reclaimed too early — only milliseconds of wall-clock time have actually elapsed")
	}

	// Simulate real time having passed with no further real activity: push
	// lastActivity itself into the past, beyond idleTableTimeout. A real
	// deployment reaches this state 30 minutes after the second join, with
	// the sweeper ticking every 5s across all of it exactly like the loop
	// above.
	h.mu.Lock()
	h.lastActivity[tbl.ID] = time.Now().Add(-(idleTableTimeout + time.Minute))
	h.mu.Unlock()

	forceExpiredTurn()
	h.sweepOnce() // this pass both forces a timeout AND finds the table idle

	if h.table(tbl.ID) != nil {
		t.Fatal("table whose only events were sweeper-fired timeouts was never reclaimed — a sweeper-fired timeout must not refresh activity")
	}

	h.mu.Lock()
	_, aliceStillClaimed := h.seatedAt["111"]
	_, bobStillClaimed := h.seatedAt["222"]
	h.mu.Unlock()
	if aliceStillClaimed {
		t.Error("Alice's seatedAt claim not released after reclaiming an AFK-only-touched table")
	}
	if bobStillClaimed {
		t.Error("Bob's seatedAt claim not released after reclaiming an AFK-only-touched table")
	}

	// End-to-end: both formerly-AFK players must now be able to join a
	// different table, proving the claim release is real, not just an
	// empty map.
	tbl2 := h.Create(2)
	for _, u := range []struct {
		id   int64
		name string
	}{{111, "Alice"}, {222, "Bob"}} {
		initData := userInitData(t, "test-token", u.id, u.name, "")
		req := httptest.NewRequest("POST", "/api/poker/"+tbl2.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s join to a new table after reclaim status = %d, want 200, body=%s", u.name, rec.Code, rec.Body.String())
		}
	}
}

// --- regression guard: sweeper reclaim racing an in-flight request (C1) ---

// checkNoOrphanedHubState asserts the invariant C1 exists to protect: no
// entry in h.seatedAt, h.lastActivity, or h.subs may reference a tableID
// absent from h.tables. Such an entry is permanently unreachable — the
// sweeper only ever iterates the current h.tables, so it can never clean up
// bookkeeping for a table id that has already been removed from it. For
// seatedAt specifically, an orphaned entry means the claimed user can never
// join any table again (item 3's exact failure mode); for lastActivity it
// is the unbounded growth item 4 exists to bound; for subs it is a
// subscriber list nothing will ever flush.
func checkNoOrphanedHubState(t *testing.T, h *PokerHub) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	for uid, tid := range h.seatedAt {
		if _, live := h.tables[tid]; !live {
			t.Errorf("orphaned seatedAt claim: user %s -> dead table %s (user is now locked out of every table)", uid, tid)
		}
	}
	for tid := range h.lastActivity {
		if _, live := h.tables[tid]; !live {
			t.Errorf("orphaned lastActivity entry for dead table %s (unbounded growth)", tid)
		}
	}
	for tid := range h.subs {
		if _, live := h.tables[tid]; !live {
			t.Errorf("orphaned subs entry for dead table %s (subscriber list nothing will ever flush)", tid)
		}
	}
}

// TestConcurrentJoinAndSweepNeverOrphanHubState is the regression guard for
// C1: a request handler resolves tbl := h.table(id) and releases h.mu
// before doing any further work (Register's existence check, at the top of
// the /api/poker/ mux handler). If the sweeper reclaims that same table in
// the window between that check and the handler's own h.mu-touching calls
// (claimSeat, touch, the subs append in handleStream), those calls must
// refuse to resurrect hub-map entries for a table id no longer in
// h.tables — otherwise the claim/activity-entry/subscriber-list becomes
// permanently orphaned, since the sweeper only ever iterates h.tables.
//
// This races many concurrent joins against a concurrently-running sweeper
// over many tables that are ALL already idle at the moment the race starts,
// maximizing the chance that at least some joins land in the exact TOCTOU
// window: the outer existence check (h.table(id) via Register) succeeds,
// then the sweeper's reclaim runs before the join's own claimSeat/touch.
func TestConcurrentJoinAndSweepNeverOrphanHubState(t *testing.T) {
	const numTables = 200

	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	mux := http.NewServeMux()
	h.Register(mux)

	type joiner struct {
		tbl *poker.Table
		uid int64
	}
	joiners := make([]joiner, numTables)
	for i := 0; i < numTables; i++ {
		tbl := h.Create(int64(i))
		uid := int64(200000 + i)
		userID := fmt.Sprintf("%d", uid)
		db.UpdateBalance(userID, fmt.Sprintf("U%d", i), 5000-100)
		joiners[i] = joiner{tbl: tbl, uid: uid}

		// Idle from the moment it's created: the very first sweep pass is
		// eligible to reclaim it, so it can race a join that starts at any
		// point during the fan-out below.
		h.mu.Lock()
		h.lastActivity[tbl.ID] = time.Now().Add(-(idleTableTimeout + time.Minute))
		h.mu.Unlock()
	}

	var wg sync.WaitGroup

	// One goroutine plays the sweeper, running many passes concurrently
	// with the joins below — exactly like StartSweeper's real ticker loop,
	// just driven directly (no goroutine leak: it's bounded and this test
	// waits for it via wg).
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			h.sweepOnce()
		}
	}()

	// Many goroutines join concurrently, each racing the sweeper's reclaim
	// of its own table. Either outcome (200 joined before reclaim, or 404
	// table closed) is a legitimate result — this test only cares about the
	// invariant checked below, not which side "wins" any individual race.
	for _, j := range joiners {
		wg.Add(1)
		go func(j joiner) {
			defer wg.Done()
			initData := userInitData(t, "test-token", j.uid, fmt.Sprintf("U%d", j.uid), "")
			req := httptest.NewRequest("POST", "/api/poker/"+j.tbl.ID+"/join", nil)
			req.Header.Set("X-Telegram-Init-Data", initData)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound {
				t.Errorf("join uid=%d status = %d, want 200 (joined before reclaim) or 404 (table closed), body=%s", j.uid, rec.Code, rec.Body.String())
			}
		}(j)
	}
	wg.Wait()

	// Drain whatever is left idle (some sweeps above may have run before
	// every join even started).
	for i := 0; i < 5; i++ {
		h.sweepOnce()
	}

	checkNoOrphanedHubState(t, h)
}

// TestConcurrentActionAndSweepNeverOrphanHubState is the action-path
// counterpart: each table already has a hand in progress (two seated
// players), and a real player action races the sweeper's reclaim of that
// same idle-since-creation table. handleAction's only h.mu-touching call is
// h.touch — this pins that it, too, must not resurrect a hub-map entry for
// a table the sweeper has already reclaimed.
func TestConcurrentActionAndSweepNeverOrphanHubState(t *testing.T) {
	const numTables = 100

	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	mux := http.NewServeMux()
	h.Register(mux)

	type actor struct {
		tbl *poker.Table
		uid int64
	}
	actors := make([]actor, numTables)
	for i := 0; i < numTables; i++ {
		tbl := h.Create(int64(i))
		if err := tbl.Sit("111", "Alice", 2000); err != nil {
			t.Fatalf("Sit: %v", err)
		}
		if err := tbl.Sit("222", "Bob", 2000); err != nil {
			t.Fatalf("Sit: %v", err)
		}
		if err := tbl.StartHand(); err != nil {
			t.Fatalf("StartHand: %v", err)
		}
		toActUserID := tbl.Seats[tbl.ToAct].UserID
		uid := mustParseInt64(t, toActUserID)
		actors[i] = actor{tbl: tbl, uid: uid}

		h.mu.Lock()
		h.lastActivity[tbl.ID] = time.Now().Add(-(idleTableTimeout + time.Minute))
		h.mu.Unlock()
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			h.sweepOnce()
		}
	}()

	for _, a := range actors {
		wg.Add(1)
		go func(a actor) {
			defer wg.Done()
			initData := userInitData(t, "test-token", a.uid, "P", "")
			body := fmt.Sprintf(`{"action":"fold","seq":%d}`, a.tbl.Seq)
			req := httptest.NewRequest("POST", "/api/poker/"+a.tbl.ID+"/action", strings.NewReader(body))
			req.Header.Set("X-Telegram-Init-Data", initData)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			// 200 (acted before reclaim), 404 (table closed by the
			// sweeper first), or 409 (a sweeper-fired ForceTimeout already
			// consumed this seq first) are all legitimate outcomes here.
			if rec.Code != http.StatusOK && rec.Code != http.StatusNotFound && rec.Code != http.StatusConflict {
				t.Errorf("action uid=%d status = %d, want 200/404/409, body=%s", a.uid, rec.Code, rec.Body.String())
			}
		}(a)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		h.sweepOnce()
	}

	checkNoOrphanedHubState(t, h)
}
