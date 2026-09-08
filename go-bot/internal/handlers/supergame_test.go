package handlers

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
)

// seatedTable builds a fresh two-seat table for offerSuper tests. It uses
// its own throwaway hub only to reach Create; the hub each test exercises
// is the *PokerHub literal built in the test itself, not this one. Mirrors
// historyTable in pokerhistory_test.go but parameterised on user ids since
// offerSuper needs specific ones to line up with the deltas map.
func seatedTable(t *testing.T, u1, u2 string) *poker.Table {
	t.Helper()
	h := NewPokerHub(nil, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	_ = tbl.Sit(u1, u1, 100000)
	_ = tbl.Sit(u2, u2, 100000)
	tbl.BigBlind = 100
	tbl.Unlock()
	return tbl
}

func TestDiceOutcomeThresholds(t *testing.T) {
	for _, tc := range []struct {
		a, b int
		want string
	}{
		{6, 6, "double"}, {5, 4, "double"}, {4, 5, "double"},
		{4, 4, "keep"}, {6, 2, "keep"}, {2, 6, "keep"},
		{4, 3, "half"}, {1, 1, "half"}, {3, 3, "half"},
	} {
		if got := diceOutcome(tc.a, tc.b); got != tc.want {
			t.Errorf("diceOutcome(%d,%d) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// The whole point of the feature is that it is close to fair. A silent edit
// to a threshold or a multiplier must fail here rather than in the economy.
func TestDiceExpectedValueIsCloseToFair(t *testing.T) {
	const stake = 3600
	total := 0
	for a := 1; a <= 6; a++ {
		for b := 1; b <= 6; b++ {
			total += stake + superDelta(stake, diceOutcome(a, b))
		}
	}
	// 36 rolls: 10 double, 5 keep, 21 half -> 35.5/36 of the stake.
	want := 35*stake + stake/2
	if total != want {
		t.Errorf("dice EV over all 36 rolls = %d, want %d", total, want)
	}
}

func TestColorOutcomeAndItsExpectedValue(t *testing.T) {
	if got := colorOutcome(true, "red"); got != "double" {
		t.Errorf("red card, red pick = %q, want double", got)
	}
	if got := colorOutcome(true, "black"); got != "bust" {
		t.Errorf("red card, black pick = %q, want bust", got)
	}
	// A 26/26 deck: half the draws double the stake, half take it all.
	const stake = 1000
	total := (stake + superDelta(stake, "double")) + (stake + superDelta(stake, "bust"))
	if total != 2*stake {
		t.Errorf("colour EV over one hit and one miss = %d, want %d", total, 2*stake)
	}
}

func TestSuperDeltaRoundsHalfInTheHousesFavour(t *testing.T) {
	if got := superDelta(101, "half"); got != -51 {
		t.Errorf("superDelta(101, half) = %d, want -51 (player keeps 50)", got)
	}
	if got := superDelta(100, "double"); got != 100 {
		t.Errorf("superDelta(100, double) = %d, want 100", got)
	}
	if got := superDelta(100, "keep"); got != 0 {
		t.Errorf("superDelta(100, keep) = %d, want 0", got)
	}
	if got := superDelta(100, "bust"); got != -100 {
		t.Errorf("superDelta(100, bust) = %d, want -100", got)
	}
}

func TestSuperDeltaIgnoresAnUnknownOutcome(t *testing.T) {
	if got := superDelta(1000, "nonsense"); got != 0 {
		t.Errorf("superDelta(1000, unknown) = %d, want 0 — an unknown outcome must move no money", got)
	}
}

func TestSuperCandidate(t *testing.T) {
	const bb = 100
	for _, tc := range []struct {
		name     string
		deltas   map[string]int
		wantUser string
		wantOK   bool
	}{
		{"one big human winner", map[string]int{"u1": 1500, "u2": -1500}, "u1", true},
		{"win under ten blinds", map[string]int{"u1": 900, "u2": -900}, "", false},
		{"exactly ten blinds qualifies", map[string]int{"u1": 1000, "u2": -1000}, "u1", true},
		{"split pot offers nothing", map[string]int{"u1": 1200, "u2": 1200, "u3": -2400}, "", false},
		{"bot winner is skipped", map[string]int{"bot:2": 5000, "u1": -5000}, "", false},
		{"split between human and bot", map[string]int{"u1": 1200, "bot:1": 1200, "u2": -2400}, "", false},
		{"nobody won", map[string]int{}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user, stake, ok := superCandidate(tc.deltas, bb)
			if ok != tc.wantOK || user != tc.wantUser {
				t.Fatalf("superCandidate = (%q, %d, %v), want (%q, _, %v)",
					user, stake, ok, tc.wantUser, tc.wantOK)
			}
			if ok && stake != tc.deltas[tc.wantUser] {
				t.Errorf("stake = %d, want %d", stake, tc.deltas[tc.wantUser])
			}
		})
	}
}

func TestSuperHoldsBlocksTheNextHandUntilResolved(t *testing.T) {
	h := &PokerHub{super: map[string]*superGame{}}

	if h.superHolds("t1") {
		t.Errorf("no game pending, must not hold")
	}

	h.setSuper("t1", &superGame{
		UserID:   "u1",
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})
	if !h.superHolds("t1") {
		t.Errorf("an offered game inside its window must hold the table")
	}

	// Resolved games still hold, so everyone gets to watch the result.
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		State:    "resolved",
		Deadline: time.Now().Add(superResultHold),
	})
	if !h.superHolds("t1") {
		t.Errorf("a resolved game must hold for the result window")
	}

	// An expired deadline must never wedge a table, whatever the state.
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		State:    "offered",
		Deadline: time.Now().Add(-time.Second),
	})
	if h.superHolds("t1") {
		t.Errorf("an expired game must release the table")
	}
}

func TestShowdownReadyIsFalseWhileASuperGameHolds(t *testing.T) {
	h := &PokerHub{
		showdownAt: map[string]time.Time{"t1": time.Now().Add(-time.Minute)},
		super:      map[string]*superGame{},
	}
	if !h.showdownReady("t1") {
		t.Fatalf("without a super game an old showdown is ready")
	}
	h.setSuper("t1", &superGame{State: "offered", Deadline: time.Now().Add(superDecideWindow)})
	if h.showdownReady("t1") {
		t.Errorf("showdownReady must be false while a super game holds")
	}
}

func TestOfferSuperRespectsCooldown(t *testing.T) {
	h := &PokerHub{
		super:     map[string]*superGame{},
		superLast: map[string]time.Time{"u1": time.Now()},
		superRoll: func() float64 { return 0 }, // always inside the chance
	}
	tbl := seatedTable(t, "u1", "u2")
	h.offerSuper(tbl, map[string]int{"u1": 5000, "u2": -5000})
	if h.superFor(tbl.ID) != nil {
		t.Errorf("a player on cooldown must not be offered a game")
	}
}

func TestOfferSuperCreatesAnOfferedGame(t *testing.T) {
	h := &PokerHub{
		super:     map[string]*superGame{},
		superLast: map[string]time.Time{},
		superRoll: func() float64 { return 0 },
	}
	tbl := seatedTable(t, "u1", "u2")
	h.offerSuper(tbl, map[string]int{"u1": 5000, "u2": -5000})

	g := h.superFor(tbl.ID)
	if g == nil {
		t.Fatal("expected an offered game")
	}
	if g.UserID != "u1" || g.Stake != 5000 || g.State != "offered" {
		t.Errorf("game = %+v, want u1 / 5000 / offered", g)
	}
	if !h.superHolds(tbl.ID) {
		t.Errorf("a fresh offer must hold the table")
	}
}

func TestOfferSuperSkipsWhenTheRollMisses(t *testing.T) {
	h := &PokerHub{
		super:     map[string]*superGame{},
		superLast: map[string]time.Time{},
		superRoll: func() float64 { return 0.99 }, // outside the chance
	}
	tbl := seatedTable(t, "u1", "u2")
	h.offerSuper(tbl, map[string]int{"u1": 5000, "u2": -5000})
	if h.superFor(tbl.ID) != nil {
		t.Errorf("a missed roll must offer nothing")
	}
}

func TestResolveSuperPaysExactlyOnce(t *testing.T) {
	db := setupTestDB(t)
	h := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		Name:     "Danya",
		Stake:    1000,
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})

	before := db.GetBalance("u1", "Danya")
	bankBefore := db.GetBalance(bankUserID, "Банк")

	g, err := h.resolveSuper("t1", "u1", "dice", "")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	// A second resolve must be refused, not applied again.
	if _, err := h.resolveSuper("t1", "u1", "dice", ""); err == nil {
		t.Fatalf("second resolve was accepted; money would be applied twice")
	}

	after := db.GetBalance("u1", "Danya")
	bankAfter := db.GetBalance(bankUserID, "Банк")

	if after-before != g.Delta {
		t.Errorf("player moved %d, want %d", after-before, g.Delta)
	}
	if (after-before)+(bankAfter-bankBefore) != 0 {
		t.Errorf("player %+d and bank %+d do not cancel — zero-sum broken",
			after-before, bankAfter-bankBefore)
	}
}

func TestResolveSuperRejectsTheWrongPlayer(t *testing.T) {
	h := &PokerHub{super: map[string]*superGame{}}
	h.setSuper("t1", &superGame{
		UserID: "u1", Stake: 1000, State: "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})
	if _, err := h.resolveSuper("t1", "u2", "dice", ""); err == nil {
		t.Errorf("a player who did not win must not be able to resolve the game")
	}
}

func TestResolveSuperSkipMovesNoMoney(t *testing.T) {
	db := setupTestDB(t)
	h := &PokerHub{db: db, super: map[string]*superGame{}}
	h.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})
	before := db.GetBalance("u1", "Danya")

	g, err := h.resolveSuper("t1", "u1", "skip", "")
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if g.Delta != 0 {
		t.Errorf("skip delta = %d, want 0", g.Delta)
	}
	if g.Outcome != "skip" {
		t.Errorf("skip outcome = %q, want %q", g.Outcome, "skip")
	}
	if g.State != "resolved" {
		t.Errorf("skip state = %q, want %q — there is no \"skipped\" state", g.State, "resolved")
	}
	if got := db.GetBalance("u1", "Danya"); got != before {
		t.Errorf("balance moved on skip: %d -> %d", before, got)
	}
}

// TestResolveSuperConcurrentCallsPayExactlyOnce is stronger than the
// sequential double-resolve check above: it fires two resolveSuper calls at
// the same table/user/game truly concurrently (both goroutines released by
// the same channel close, no ordering between them) and requires that
// exactly one wins the idempotency guard and the balance moves by exactly
// one payout, never zero and never two. Run with -race: correction (1) in
// the brief exists specifically so this cannot torn-write the shared
// *superGame that superFor lets other goroutines read concurrently.
func TestResolveSuperConcurrentCallsPayExactlyOnce(t *testing.T) {
	db := setupTestDB(t)
	h := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		Name:     "Danya",
		Stake:    1000,
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})

	before := db.GetBalance("u1", "Danya")
	bankBefore := db.GetBalance(bankUserID, "Банк")

	const n = 8
	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := h.resolveSuper("t1", "u1", "dice", "")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	oks, fails := 0, 0
	for err := range results {
		if err == nil {
			oks++
		} else {
			fails++
		}
	}
	if oks != 1 {
		t.Fatalf("resolveSuper succeeded %d times concurrently, want exactly 1 (failed %d)", oks, fails)
	}

	g := h.superFor("t1")
	if g == nil {
		t.Fatal("expected a resolved game to remain")
	}

	after := db.GetBalance("u1", "Danya")
	bankAfter := db.GetBalance(bankUserID, "Банк")

	if after-before != g.Delta {
		t.Errorf("player moved %d, want %d (the single winning resolve's delta)", after-before, g.Delta)
	}
	if (after-before)+(bankAfter-bankBefore) != 0 {
		t.Errorf("player %+d and bank %+d do not cancel — money moved more than once",
			after-before, bankAfter-bankBefore)
	}
}

// TestResolveSuperPayoutFailureDowngradesToNoGain proves the review fix: if
// SettlePoker fails after the game is already State "resolved" with a
// winning Outcome and Delta, resolveSuper must not let that winning result
// reach superFor/broadcast. It induces a *real* SettlePoker failure -- not a
// simulated one -- by closing the underlying *storage.DB before calling
// resolveSuper, so db.Begin() itself returns an error and the transaction
// never runs. "color" is used instead of "dice" because dice can roll a
// "keep" (Delta == 0), which would skip the SettlePoker call entirely and
// make the test flaky; color's outcome is always "double" or "bust", so
// Delta is always non-zero and a payout attempt (and failure) is guaranteed
// regardless of the random draw.
func TestResolveSuperPayoutFailureDowngradesToNoGain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "supergame-payout-fail.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	// Seed a known balance while the db is still open, so we have a real
	// figure to check against after the failed payout -- not just the
	// closed-db fallback value.
	db.UpdateBalance("u1", "Danya", 400) // 100 starting + 400 = 500
	before := db.GetBalance("u1", "Danya")
	if before != 500 {
		t.Fatalf("seed balance = %d, want 500", before)
	}

	// Close the db so SettlePoker's db.Begin() fails for real -- this is
	// not a simulated error, it is the actual storage layer refusing the
	// write.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	h := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		Name:     "Danya",
		Stake:    1000,
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})

	g, err := h.resolveSuper("t1", "u1", "color", "red")
	if err == nil {
		t.Fatalf("resolveSuper succeeded despite a closed database -- payout failure was not detected")
	}
	if errors.Is(err, errSuperUnavailable) {
		t.Fatalf("resolveSuper returned the idempotency-guard error, not a payout-failure error: %v", err)
	}
	if g == nil {
		t.Fatal("resolveSuper returned a nil game alongside the payout error; the caller has nothing to broadcast")
	}
	if g.Delta != 0 {
		t.Errorf("downgraded game Delta = %d, want 0 -- a failed payout must publish no gain", g.Delta)
	}
	if g.Outcome == "double" {
		t.Errorf("downgraded game Outcome = %q, must not still claim a win", g.Outcome)
	}
	if g.State != "resolved" {
		t.Errorf("downgraded game State = %q, want %q -- must not reopen the double-resolve window", g.State, "resolved")
	}

	// The game stored on the hub (what superFor/broadcast would hand every
	// other client) must show the same downgrade, not just the copy
	// returned to the caller.
	stored := h.superFor("t1")
	if stored == nil {
		t.Fatal("expected the resolved game to remain on the hub")
	}
	if stored.Delta != 0 || stored.Outcome == "double" {
		t.Errorf("stored game = %+v, want Delta 0 and Outcome != double", stored)
	}

	// Reopen the same database file and check the real, persisted balance
	// -- not the closed-db fallback -- to confirm SettlePoker's rolled-back
	// transaction moved nothing.
	db2, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New (reopen): %v", err)
	}
	defer db2.Close()
	if after := db2.GetBalance("u1", "Danya"); after != before {
		t.Errorf("persisted balance moved despite the payout failing: %d -> %d", before, after)
	}
}

// TestEnvelopeCarriesTheSuperGameToEveryone proves the block is published
// on the envelope, identically to the winner and to a bystander, and
// disappears once nothing is pending.
func TestEnvelopeCarriesTheSuperGameToEveryone(t *testing.T) {
	h := &PokerHub{super: map[string]*superGame{}, chat: map[string][]chatMsg{}}
	tbl := seatedTable(t, "u1", "u2")
	h.setSuper(tbl.ID, &superGame{
		UserID: "u1", Name: "Danya", Stake: 1500, State: "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})

	// The winner and a bystander must both receive it, identically.
	for _, viewer := range []string{"u1", "u2"} {
		env := h.envelope(tbl, viewer)
		if env.Super == nil {
			t.Fatalf("viewer %s got no super block", viewer)
		}
		if env.Super.UserID != "u1" || env.Super.Stake != 1500 || env.Super.State != "offered" {
			t.Errorf("viewer %s got %+v, want u1 / 1500 / offered", viewer, env.Super)
		}
	}

	h.setSuper(tbl.ID, nil)
	if env := h.envelope(tbl, "u1"); env.Super != nil {
		t.Errorf("no pending game must serialise no super block")
	}
}

// TestSuperViewHidesAnExpiredGame pins the carried-over fix: nothing else
// deletes a finished or timed-out game from h.super, so superView(Locked)
// itself must stop publishing one once its Deadline has passed -- whatever
// State says -- or the client's panel would never disappear.
func TestSuperViewHidesAnExpiredGame(t *testing.T) {
	h := &PokerHub{super: map[string]*superGame{}}
	h.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "resolved",
		Outcome: "double", Deadline: time.Now().Add(-time.Second),
	})

	if v := h.superView("t1"); v != nil {
		t.Errorf("superView of an expired game = %+v, want nil", v)
	}
	if v := h.superViewLocked("t1"); v != nil {
		t.Errorf("superViewLocked of an expired game = %+v, want nil", v)
	}
}

// A deploy mid-game must lose the offer, not resume it. Resuming would mean
// deciding, after the fact, whether money that was never written should be —
// and there is no safe answer. Dropping it leaves the player with the
// winnings they already have.
func TestPendingSuperGameDoesNotSurviveASnapshot(t *testing.T) {
	h := &PokerHub{super: map[string]*superGame{}}
	tbl := seatedTable(t, "u1", "u2")
	h.setSuper(tbl.ID, &superGame{
		UserID: "u1", Stake: 1000, State: "offered",
		Deadline: time.Now().Add(superDecideWindow),
	})

	raw, err := json.Marshal(tbl.Snapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if strings.Contains(string(raw), "super") {
		t.Errorf("table snapshot carries super game state: %s", raw)
	}
}
