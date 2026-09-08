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

// TestOfferSuperDrawsTheGame pins "which game you get is not your choice":
// the server draws dice-or-colour at offer time, not the player, and the
// draw is published on the offer itself (State "offered") rather than held
// back until resolution. superGameRoll is forced both ways so both branches
// of the 50/50 actually run.
func TestOfferSuperDrawsTheGame(t *testing.T) {
	for _, tc := range []struct {
		name string
		roll float64
		want string
	}{
		{"low roll draws colour", 0.0, "color"},
		{"high roll draws dice", 0.999, "dice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &PokerHub{
				super:         map[string]*superGame{},
				superLast:     map[string]time.Time{},
				superRoll:     func() float64 { return 0 }, // always inside the chance
				superGameRoll: func() float64 { return tc.roll },
			}
			tbl := seatedTable(t, "u1", "u2")
			h.offerSuper(tbl, map[string]int{"u1": 5000, "u2": -5000})

			g := h.superFor(tbl.ID)
			if g == nil {
				t.Fatal("expected an offered game")
			}
			if g.State != "offered" {
				t.Errorf("state = %q, want offered", g.State)
			}
			if g.Game != tc.want {
				t.Errorf("drawn game = %q, want %q -- must be published while still offered", g.Game, tc.want)
			}
		})
	}
}

// TestResolveSuperRejectsAChoiceFromTheOtherGame proves that a choice which
// does not belong to the game the server already drew is refused as a bad
// request AND, critically, does not consume the offer: the game must still
// be "offered" and still resolvable with a valid choice afterwards. This is
// what closes the door on the request influencing which rules apply -- the
// player's only real decision is within the game the server fixed.
func TestResolveSuperRejectsAChoiceFromTheOtherGame(t *testing.T) {
	db := setupTestDB(t)

	// A dice game must reject "red".
	h := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	h.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered", Game: "dice",
		Deadline: time.Now().Add(superDecideWindow),
	})
	if _, err := h.resolveSuper("t1", "u1", "red"); !errors.Is(err, errSuperBadChoice) {
		t.Fatalf("dice game accepted %q: err = %v, want errSuperBadChoice", "red", err)
	}
	if g := h.superFor("t1"); g == nil || g.State != "offered" {
		t.Fatalf("game after a rejected choice = %+v, want still offered", g)
	}
	if _, err := h.resolveSuper("t1", "u1", "roll"); err != nil {
		t.Fatalf("resolving with a valid choice after the rejection: %v", err)
	}

	// A colour game must reject "roll".
	h2 := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	h2.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered", Game: "color",
		Deadline: time.Now().Add(superDecideWindow),
	})
	if _, err := h2.resolveSuper("t1", "u1", "roll"); !errors.Is(err, errSuperBadChoice) {
		t.Fatalf("colour game accepted %q: err = %v, want errSuperBadChoice", "roll", err)
	}
	if g := h2.superFor("t1"); g == nil || g.State != "offered" {
		t.Fatalf("game after a rejected choice = %+v, want still offered", g)
	}
	if _, err := h2.resolveSuper("t1", "u1", "red"); err != nil {
		t.Fatalf("resolving with a valid choice after the rejection: %v", err)
	}
}

func TestResolveSuperPaysExactlyOnce(t *testing.T) {
	db := setupTestDB(t)
	h := &PokerHub{db: db, super: map[string]*superGame{}, superLast: map[string]time.Time{}}
	// "color" is used instead of "dice": dice can roll a "keep" (Delta ==
	// 0), which would short-circuit the cp.Delta != 0 guard below and skip
	// SettlePoker entirely, collapsing this test's zero-sum assertion to a
	// vacuous 0 == 0 on roughly one run in seven. Colour's outcome is
	// always "double" or "bust", so Delta is always non-zero and a payout
	// is guaranteed regardless of the random draw.
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		Name:     "Danya",
		Stake:    1000,
		State:    "offered",
		Game:     "color",
		Deadline: time.Now().Add(superDecideWindow),
	})

	before := db.GetBalance("u1", "Danya")
	bankBefore := db.GetBalance(bankUserID, "Банк")

	g, err := h.resolveSuper("t1", "u1", "red")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	// A second resolve must be refused, not applied again.
	if _, err := h.resolveSuper("t1", "u1", "roll"); err == nil {
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
		UserID: "u1", Stake: 1000, State: "offered", Game: "dice",
		Deadline: time.Now().Add(superDecideWindow),
	})
	if _, err := h.resolveSuper("t1", "u2", "roll"); err == nil {
		t.Errorf("a player who did not win must not be able to resolve the game")
	}
}

func TestResolveSuperSkipMovesNoMoney(t *testing.T) {
	db := setupTestDB(t)
	h := &PokerHub{db: db, super: map[string]*superGame{}}
	h.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered", Game: "dice",
		Deadline: time.Now().Add(superDecideWindow),
	})
	before := db.GetBalance("u1", "Danya")

	g, err := h.resolveSuper("t1", "u1", "skip")
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
	// "color" is used instead of "dice": dice can roll a "keep" (Delta ==
	// 0), which would short-circuit the cp.Delta != 0 guard in resolveSuper
	// and skip SettlePoker entirely, collapsing this test's zero-sum
	// assertion to a vacuous 0 == 0 on roughly one run in seven. Colour's
	// outcome is always "double" or "bust", so Delta is always non-zero and
	// a payout is guaranteed regardless of the random draw.
	h.setSuper("t1", &superGame{
		UserID:   "u1",
		Name:     "Danya",
		Stake:    1000,
		State:    "offered",
		Game:     "color",
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
			_, err := h.resolveSuper("t1", "u1", "red")
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
		Game:     "color",
		Deadline: time.Now().Add(superDecideWindow),
	})

	g, err := h.resolveSuper("t1", "u1", "red")
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

// superSeatedHub builds a real hub, wired with a real db and a real table,
// with u1 seated at buyIn chips and holding a pending colour-game offer for
// stake. "color" is used throughout this file's chip-mirroring tests for the
// same reason the existing money tests use it: its outcome is always
// "double" or "bust" (never dice's zero-delta "keep"), so Delta is
// guaranteed non-zero and every call here actually attempts a payout.
func superSeatedHub(t *testing.T, buyIn, stake int) (*PokerHub, *poker.Table, *storage.DB) {
	t.Helper()
	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	if err := tbl.Sit("u1", "Danya", buyIn); err != nil {
		tbl.Unlock()
		t.Fatalf("Sit: %v", err)
	}
	tbl.Unlock()
	h.mu.Lock()
	h.seatedAt["u1"] = tbl.ID
	h.mu.Unlock()
	h.setSuper(tbl.ID, &superGame{
		UserID: "u1", Name: "Danya", Stake: stake, State: "offered", Game: "color",
		Deadline: time.Now().Add(superDecideWindow),
	})
	return h, tbl, db
}

func seatStack(tbl *poker.Table, userID string) int {
	tbl.Lock()
	defer tbl.Unlock()
	for _, s := range tbl.Seats {
		if s.UserID == userID {
			return s.Stack
		}
	}
	return -1
}

// TestResolveSuperWinningPayoutReachesTheChips proves the fix: a double must
// raise BOTH the balance and the seated player's stack, by the same amount.
// Before this fix the balance moved and the felt did not.
func TestResolveSuperWinningPayoutReachesTheChips(t *testing.T) {
	const buyIn, stake = 5000, 1000
	for attempt := 0; attempt < 50; attempt++ {
		h, tbl, db := superSeatedHub(t, buyIn, stake)
		balBefore := db.GetBalance("u1", "Danya")
		stackBefore := seatStack(tbl, "u1")

		g, err := h.resolveSuper(tbl.ID, "u1", "red")
		if err != nil {
			t.Fatalf("resolveSuper: %v", err)
		}
		if g.Outcome != "double" {
			continue // wrong branch of the 50/50 this attempt; try again
		}

		balAfter := db.GetBalance("u1", "Danya")
		stackAfter := seatStack(tbl, "u1")

		if g.Delta <= 0 {
			t.Fatalf("double outcome with Delta = %d, want > 0", g.Delta)
		}
		if balAfter-balBefore != g.Delta {
			t.Errorf("balance moved %d, want %d", balAfter-balBefore, g.Delta)
		}
		if stackAfter-stackBefore != g.Delta {
			t.Errorf("stack moved %d, want %d — the win never reached the felt", stackAfter-stackBefore, g.Delta)
		}
		return
	}
	t.Fatal("never rolled a double in 50 attempts")
}

// TestResolveSuperLosingPayoutReachesTheChips is the mirror of the winning
// case: a bust must lower BOTH the balance and the stack, by the same
// amount.
func TestResolveSuperLosingPayoutReachesTheChips(t *testing.T) {
	const buyIn, stake = 5000, 1000
	for attempt := 0; attempt < 50; attempt++ {
		h, tbl, db := superSeatedHub(t, buyIn, stake)
		balBefore := db.GetBalance("u1", "Danya")
		stackBefore := seatStack(tbl, "u1")

		g, err := h.resolveSuper(tbl.ID, "u1", "red")
		if err != nil {
			t.Fatalf("resolveSuper: %v", err)
		}
		if g.Outcome != "bust" {
			continue // wrong branch of the 50/50 this attempt; try again
		}

		balAfter := db.GetBalance("u1", "Danya")
		stackAfter := seatStack(tbl, "u1")

		if g.Delta >= 0 {
			t.Fatalf("bust outcome with Delta = %d, want < 0", g.Delta)
		}
		if balAfter-balBefore != g.Delta {
			t.Errorf("balance moved %d, want %d", balAfter-balBefore, g.Delta)
		}
		if stackAfter-stackBefore != g.Delta {
			t.Errorf("stack moved %d, want %d — a loss must also leave the felt", stackAfter-stackBefore, g.Delta)
		}
		return
	}
	t.Fatal("never rolled a bust in 50 attempts")
}

// TestResolveSuperSkipMovesNoChips is TestResolveSuperSkipMovesNoMoney's
// counterpart for the felt: a deliberate pass (Delta == 0) must move no
// chips, exactly as it moves no balance.
func TestResolveSuperSkipMovesNoChips(t *testing.T) {
	h, tbl, db := superSeatedHub(t, 5000, 1000)
	balBefore := db.GetBalance("u1", "Danya")
	stackBefore := seatStack(tbl, "u1")

	g, err := h.resolveSuper(tbl.ID, "u1", "skip")
	if err != nil {
		t.Fatalf("resolveSuper: %v", err)
	}
	if g.Delta != 0 {
		t.Fatalf("skip Delta = %d, want 0", g.Delta)
	}
	if got := db.GetBalance("u1", "Danya"); got != balBefore {
		t.Errorf("balance moved on skip: %d -> %d", balBefore, got)
	}
	if got := seatStack(tbl, "u1"); got != stackBefore {
		t.Errorf("stack moved on skip: %d -> %d", stackBefore, got)
	}
}

// TestResolveSuperPayoutFailureMovesNoChips extends
// TestResolveSuperPayoutFailureDowngradesToNoGain to the felt: when
// SettlePoker fails (here, a closed db, same technique as that test) neither
// the balance nor the seated player's chips may move, whatever Outcome was
// rolled before the downgrade.
func TestResolveSuperPayoutFailureMovesNoChips(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "supergame-payout-fail-chips.db")
	db, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	db.UpdateBalance("u1", "Danya", 400) // 100 starting + 400 = 500, matches the buy-in seed below
	balBefore := db.GetBalance("u1", "Danya")

	h := NewPokerHub(db, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	if err := tbl.Sit("u1", "Danya", 5000); err != nil {
		tbl.Unlock()
		t.Fatalf("Sit: %v", err)
	}
	tbl.Unlock()
	h.mu.Lock()
	h.seatedAt["u1"] = tbl.ID
	h.mu.Unlock()
	stackBefore := seatStack(tbl, "u1")

	h.setSuper(tbl.ID, &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered", Game: "color",
		Deadline: time.Now().Add(superDecideWindow),
	})

	// Close the db so SettlePoker's db.Begin() fails for real, exactly as
	// TestResolveSuperPayoutFailureDowngradesToNoGain does.
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	g, err := h.resolveSuper(tbl.ID, "u1", "red")
	if err == nil {
		t.Fatalf("resolveSuper succeeded despite a closed database")
	}
	if g.Delta != 0 {
		t.Errorf("downgraded game Delta = %d, want 0", g.Delta)
	}
	if got := seatStack(tbl, "u1"); got != stackBefore {
		t.Errorf("stack moved despite the payout failing: %d -> %d", stackBefore, got)
	}

	db2, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("storage.New (reopen): %v", err)
	}
	defer db2.Close()
	if after := db2.GetBalance("u1", "Danya"); after != balBefore {
		t.Errorf("persisted balance moved despite the payout failing: %d -> %d", balBefore, after)
	}
}

// TestResolveSuperUnseatedPlayerStillGetsBalance covers a player who left
// the table between the offer and the resolve (or was never seated through
// this hub at all, e.g. h.seatedAt has no entry for them): the balance
// change must still land, with no panic, and AdjustStack's own not-seated
// branch must simply no-op rather than move chips at some other table.
func TestResolveSuperUnseatedPlayerStillGetsBalance(t *testing.T) {
	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "tok")
	// Deliberately no h.seatedAt entry and no table for u1 -- they stood up
	// (or never sat) after the offer was made.
	h.setSuper("t1", &superGame{
		UserID: "u1", Name: "Danya", Stake: 1000, State: "offered", Game: "color",
		Deadline: time.Now().Add(superDecideWindow),
	})
	balBefore := db.GetBalance("u1", "Danya")

	g, err := h.resolveSuper("t1", "u1", "red")
	if err != nil {
		t.Fatalf("resolveSuper: %v", err)
	}
	if g.Delta == 0 {
		t.Fatalf("colour outcome must be double or bust, got Delta 0 (Outcome %q)", g.Outcome)
	}
	if got := db.GetBalance("u1", "Danya"); got-balBefore != g.Delta {
		t.Errorf("balance moved %d, want %d — an unseated player must still be paid", got-balBefore, g.Delta)
	}
	// Reaching here at all is the proof against a panic: AdjustStack must
	// have found no table for u1 and returned without touching anything.
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
