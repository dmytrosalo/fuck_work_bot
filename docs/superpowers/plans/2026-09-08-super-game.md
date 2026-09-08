# Супер гра Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a big solo poker win, the table stops and the winner gambles the winnings on dice or red/black while everyone watches.

**Architecture:** The hand settles normally through `SettlePoker` — untouched. The hub then holds the showdown open (one extra condition in `showdownReady`, not a new `Stage`), publishes a shared `super` block on `tableEnvelope`, and on resolution moves money as a separate two-party transaction against `bank:house`. `internal/poker` is not modified by any task in this plan.

**Tech Stack:** Go 1.25, `net/http` + `http.ServeMux`, `math/rand` (matches the rest of the package), `modernc.org/sqlite`, vanilla JS in the `pokerTmpl` template.

**Spec:** `docs/superpowers/specs/2026-09-08-super-game-design.md`

## Global Constraints

- All user-facing text is Ukrainian.
- `CGO_ENABLED=0`; pure-Go SQLite driver only.
- The dice roll and the card draw happen **server-side only**. No `Math.random()` may decide an outcome.
- `internal/poker` must not be modified. The hold and the state live in `internal/handlers`.
- `SettlePoker`'s per-hand zero-sum behaviour must not change; the only edit to it is adding an `activity` parameter.
- Money moves exactly once per super game, inside one DB transaction, after the outcome is known.
- Bots (`user_id` prefix `bot:`) are never offered a super game.
- Rounding rule for ×½: the player **keeps `stake/2` truncated**, so a stake of 101 returns 50 and the delta is −51.
- Payout constants, verbatim: `superChance = 0.15`, `superCooldown = 10 * time.Minute`, `superMinBlinds = 10`, `superDecideWindow = 10 * time.Second`, `superResultHold = 4 * time.Second`.

---

### Task 1: Give `SettlePoker` an activity label

The super game must appear in `/stats` as its own activity rather than as more poker. `SettlePoker` already applies a delta list atomically, which is exactly what the payout needs — it just hardcodes `"poker"` in the audit row.

**Files:**
- Modify: `go-bot/internal/storage/sqlite.go:701-722`
- Modify: `go-bot/internal/handlers/pokerweb.go:2290`
- Test: `go-bot/internal/storage/sqlite_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `func (d *DB) SettlePoker(deltas []PokerDelta, activity string) error`

- [ ] **Step 1: Write the failing test**

```go
func TestSettlePokerRecordsTheGivenActivity(t *testing.T) {
	db := testDB(t)
	if err := db.SettlePoker([]PokerDelta{
		{UserID: "u1", Name: "Danya", Amount: 500},
		{UserID: "bank:house", Name: "Банк", Amount: -500},
	}, "supergame"); err != nil {
		t.Fatalf("SettlePoker: %v", err)
	}

	var activity string
	err := db.db.QueryRow(`SELECT activity FROM transactions WHERE user_id = 'u1'`).Scan(&activity)
	if err != nil {
		t.Fatalf("read transaction: %v", err)
	}
	if activity != "supergame" {
		t.Errorf("activity = %q, want %q", activity, "supergame")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/storage/ -run TestSettlePokerRecordsTheGivenActivity`
Expected: FAIL to compile — "too many arguments in call to db.SettlePoker".

- [ ] **Step 3: Add the parameter**

In `sqlite.go`, change the signature and the insert:

```go
func (d *DB) SettlePoker(deltas []PokerDelta, activity string) error {
```

```go
		if _, err := tx.Exec(`INSERT INTO transactions (user_id, name, activity, amount) VALUES (?, ?, ?, ?)`,
			delta.UserID, delta.Name, activity, delta.Amount); err != nil {
```

Update the one existing caller in `pokerweb.go:2290`:

```go
		if err := h.db.SettlePoker(entries, "poker"); err != nil {
```

- [ ] **Step 4: Run the full suite**

Run: `cd go-bot && go test ./...`
Expected: PASS. The poker settlement tests still pass because they assert balances, not the activity string.

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/storage/sqlite.go go-bot/internal/storage/sqlite_test.go go-bot/internal/handlers/pokerweb.go
git commit -m "SettlePoker records a caller-supplied activity"
```

---

### Task 2: Payout rules as pure functions

The maths is the part that must never drift silently, so it lands first with no HTTP, no state and no money attached.

**Files:**
- Create: `go-bot/internal/handlers/supergame.go`
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `func diceOutcome(a, b int) string` — `"double"` for a+b ≥ 9, `"keep"` for 8, `"half"` for ≤ 7
  - `func colorOutcome(cardIsRed bool, pick string) string` — `"double"` on a match, `"bust"` otherwise
  - `func superDelta(stake int, outcome string) int` — signed balance change
  - `const superChance`, `superCooldown`, `superMinBlinds`, `superDecideWindow`, `superResultHold`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run 'TestDice|TestColor|TestSuperDelta'`
Expected: FAIL to compile — "undefined: diceOutcome".

- [ ] **Step 3: Write the implementation**

Create `go-bot/internal/handlers/supergame.go`:

```go
package handlers

import "time"

// Супер гра is a double-or-nothing on a poker win. See
// docs/superpowers/specs/2026-09-08-super-game-design.md for the maths
// behind every constant here.
const (
	// superChance is how often a qualifying win is offered a game, and
	// superCooldown is the minimum gap between two games for one player.
	// Same division of labour as tauntChance/tauntCooldown in pokerbots.go:
	// the probability is for variety, the cooldown is what stops a burst.
	superChance    = 0.15
	superCooldown  = 10 * time.Minute
	// superMinBlinds keeps this an event. Without a floor it would fire on
	// pots of two blinds and stop meaning anything. Relative to the blind
	// rather than absolute because the blinds double on a schedule.
	superMinBlinds = 10

	// The table is held for the sum of these two, so they are the worst-case
	// pause everyone else sits through.
	superDecideWindow = 10 * time.Second
	superResultHold   = 4 * time.Second
)

// diceOutcome scores 2d6. Over 36 rolls this is 10 doubles, 5 keeps and 21
// halves, which is 35.5/36 of the stake returned -- a 1.4% edge to the bank.
func diceOutcome(a, b int) string {
	switch sum := a + b; {
	case sum >= 9:
		return "double"
	case sum == 8:
		return "keep"
	default:
		return "half"
	}
}

// colorOutcome scores a single card draw. A 26/26 deck makes this exactly
// fair, which is why it has no "keep" branch to soften it.
func colorOutcome(cardIsRed bool, pick string) string {
	if (pick == "red") == cardIsRed {
		return "double"
	}
	return "bust"
}

// superDelta is the signed balance change for an outcome. The half branch
// truncates, so the player keeps stake/2 and an odd stake loses the extra
// coin to the bank.
func superDelta(stake int, outcome string) int {
	switch outcome {
	case "double":
		return stake
	case "keep":
		return 0
	case "half":
		return stake/2 - stake
	default: // "bust"
		return -stake
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run 'TestDice|TestColor|TestSuperDelta' -v`
Expected: PASS, all four.

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/supergame.go go-bot/internal/handlers/supergame_test.go
git commit -m "Супер гра payout rules"
```

---

### Task 3: Deciding whether to offer a game

Pure selection logic, still with no state and no money: given a settled hand, is there exactly one human winner big enough to qualify?

**Files:**
- Modify: `go-bot/internal/handlers/supergame.go`
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: `superMinBlinds` from Task 2.
- Produces: `func superCandidate(deltas map[string]int, bigBlind int) (userID string, stake int, ok bool)`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestSuperCandidate`
Expected: FAIL to compile — "undefined: superCandidate".

- [ ] **Step 3: Write the implementation**

Append to `supergame.go`:

```go
// superCandidate finds the single human who won this hand, if there is
// exactly one and their win clears superMinBlinds.
//
// A split pot -- more than one seat with a positive delta -- offers no game
// at all, bots included. That is stricter than picking the largest winner,
// and deliberately so: it means there is no tie-breaking rule to get wrong
// and the table can never be held twice for one hand.
func superCandidate(deltas map[string]int, bigBlind int) (string, int, bool) {
	winners := 0
	user, stake := "", 0
	for id, d := range deltas {
		if d <= 0 {
			continue
		}
		winners++
		user, stake = id, d
	}
	if winners != 1 || isBotUser(user) {
		return "", 0, false
	}
	if stake < superMinBlinds*bigBlind {
		return "", 0, false
	}
	return user, stake, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run TestSuperCandidate -v`
Expected: PASS, all seven subtests.

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/supergame.go go-bot/internal/handlers/supergame_test.go
git commit -m "Супер гра offer conditions"
```

---

### Task 4: Hub state and the showdown hold

The state and the thing that actually stops the next hand from being dealt.

**Files:**
- Modify: `go-bot/internal/handlers/supergame.go`
- Modify: `go-bot/internal/handlers/pokerweb.go:1611-1633` (hub fields), `:2318-2326` (`showdownReady`)
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: `superDecideWindow`, `superResultHold` from Task 2.
- Produces:
  - `type superGame struct` with fields `UserID, Name string; Stake int; State, Game, Pick, Outcome, Card string; Dice [2]int; Delta int; Deadline time.Time`
  - `func (h *PokerHub) superFor(tableID string) *superGame` — a copy, or nil
  - `func (h *PokerHub) setSuper(tableID string, g *superGame)`
  - `func (h *PokerHub) superHolds(tableID string) bool`
  - Hub field: `super map[string]*superGame`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run 'TestSuperHolds|TestShowdownReadyIsFalse'`
Expected: FAIL to compile — "unknown field super in struct literal".

- [ ] **Step 3: Write the implementation**

Add the hub field in `pokerweb.go`, next to `showdownAt`:

```go
	// super holds the pending Супер гра per table id, if any. Guarded by
	// h.mu like the other per-table maps. Deliberately NOT part of the
	// table snapshot: a deploy mid-game drops it, which is the only
	// outcome that cannot half-apply money.
	super map[string]*superGame
```

Initialise it wherever `showdownAt` is initialised in the hub constructor.

Append to `supergame.go`:

```go
// superGame is one pending Супер гра. It lives on the hub rather than on
// poker.Table: the stage machine is the money-critical part, and a hold in
// the handler layer cannot corrupt a hand.
type superGame struct {
	UserID   string
	Name     string
	Stake    int
	State    string // "offered" | "resolved"
	Game     string // "dice" | "color"
	Pick     string // "red" | "black", colour game only
	Dice     [2]int
	Card     string
	Outcome  string // "double" | "keep" | "half" | "bust"
	Delta    int
	Deadline time.Time
}

func (h *PokerHub) superFor(tableID string) *superGame {
	h.mu.Lock()
	defer h.mu.Unlock()
	g, ok := h.super[tableID]
	if !ok {
		return nil
	}
	cp := *g
	return &cp
}

func (h *PokerHub) setSuper(tableID string, g *superGame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if g == nil {
		delete(h.super, tableID)
		return
	}
	h.super[tableID] = g
}

// superHolds reports whether tableID must not deal the next hand yet. An
// expired deadline releases the hold whatever the state, so a crashed or
// abandoned game can never wedge a table -- the same defence showdownReady
// already applies to a missing showdown timestamp.
func (h *PokerHub) superHolds(tableID string) bool {
	g := h.superFor(tableID)
	return g != nil && time.Now().Before(g.Deadline)
}
```

Extend `showdownReady` in `pokerweb.go`:

```go
func (h *PokerHub) showdownReady(tableID string) bool {
	// A pending Супер гра holds the showdown open past the usual interval
	// so the winner can decide and everyone can watch the result.
	if h.superHolds(tableID) {
		return false
	}
	h.mu.Lock()
	at, ok := h.showdownAt[tableID]
	h.mu.Unlock()
	if !ok {
		return true
	}
	return time.Since(at) >= sweepInterval
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run 'TestSuperHolds|TestShowdownReadyIsFalse' -v`
Expected: PASS both.

- [ ] **Step 5: Run the full suite**

Run: `cd go-bot && go test ./...`
Expected: PASS. If a poker test constructs `PokerHub` literally and now panics on a nil `super` map, initialise the map in that test's helper rather than making `setSuper` lazily allocate — the production constructor must own it.

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/supergame.go go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/supergame_test.go
git commit -m "Супер гра holds the showdown open"
```

---

### Task 5: Offering a game from settle()

Wire Task 3's decision into the real settlement path, behind the chance roll and the cooldown.

**Files:**
- Modify: `go-bot/internal/handlers/supergame.go`
- Modify: `go-bot/internal/handlers/pokerweb.go:2296` (end of `settle`)
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: `superCandidate` (Task 3), `setSuper`/`superFor` (Task 4).
- Produces: `func (h *PokerHub) offerSuper(tbl *poker.Table, deltas map[string]int)`; hub field `superLast map[string]time.Time`

- [ ] **Step 1: Write the failing test**

```go
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
```

`seatedTable` is an existing helper in the poker handler tests; if the one in scope does not take user ids, reuse whatever the neighbouring `pokerweb_test.go` tests use to build a two-seat table and set its big blind to 100.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestOfferSuper`
Expected: FAIL to compile — "unknown field superRoll".

- [ ] **Step 3: Write the implementation**

Add two hub fields next to `super`:

```go
	// superLast is the last time each user actually played a Супер гра,
	// keyed by user id. Only a game that was offered updates it, so a
	// missed chance roll does not start the cooldown.
	superLast map[string]time.Time
	// superRoll returns a number in [0,1) for the chance gate. A field so
	// tests can make the roll deterministic; production leaves it nil and
	// falls back to rand.Float64.
	superRoll func() float64
```

Append to `supergame.go`:

```go
// offerSuper opens a Супер гра on tbl if this hand qualifies. Caller must
// hold tbl.Lock(), same as settle() itself.
func (h *PokerHub) offerSuper(tbl *poker.Table, deltas map[string]int) {
	user, stake, ok := superCandidate(deltas, tbl.BigBlind)
	if !ok {
		return
	}

	h.mu.Lock()
	last := h.superLast[user]
	h.mu.Unlock()
	if time.Since(last) < superCooldown {
		return
	}

	roll := h.superRoll
	if roll == nil {
		roll = rand.Float64
	}
	if roll() >= superChance {
		return
	}

	name := user
	for _, s := range tbl.Seats {
		if s.UserID == user {
			name = s.Name
			break
		}
	}

	h.mu.Lock()
	h.superLast[user] = time.Now()
	h.super[tbl.ID] = &superGame{
		UserID:   user,
		Name:     name,
		Stake:    stake,
		State:    "offered",
		Deadline: time.Now().Add(superDecideWindow),
	}
	h.mu.Unlock()
}
```

Add `"math/rand"` to the imports of `supergame.go`.

Call it at the end of `settle()` in `pokerweb.go`, after `h.botTaunt(tbl, deltas)`:

```go
	// Offered only after the hand has fully settled, so the stake is a
	// balance the player already holds and the payout is a clean second
	// transaction rather than an edit to the settlement.
	h.offerSuper(tbl, deltas)
```

If `tbl.BigBlind` is not an exported field, read the blind through whatever accessor `ViewFor` uses to populate `big_blind` and use that instead.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run TestOfferSuper -v`
Expected: PASS, all three.

- [ ] **Step 5: Run the full suite**

Run: `cd go-bot && go test ./...`
Expected: PASS. Existing poker tests may now occasionally hold a showdown; if any test asserts the next hand starts immediately after a settle, set `superRoll` to always miss in that test's hub.

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/supergame.go go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/supergame_test.go
git commit -m "Offer a Супер гра on a qualifying win"
```

---

### Task 6: Resolving the game and moving the money

The endpoint, the server-side roll, the idempotency guard, and the only place money changes.

**Files:**
- Modify: `go-bot/internal/handlers/supergame.go`
- Modify: `go-bot/internal/handlers/pokerweb.go:1953-1969` (action switch)
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–5.
- Produces: `func (h *PokerHub) handleSuper(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64)`; `func (h *PokerHub) resolveSuper(tableID, userID, game, pick string) (*superGame, error)`

- [ ] **Step 1: Write the failing test**

```go
func TestResolveSuperPaysExactlyOnce(t *testing.T) {
	db := testStorage(t)
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
	db := testStorage(t)
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
	if got := db.GetBalance("u1", "Danya"); got != before {
		t.Errorf("balance moved on skip: %d -> %d", before, got)
	}
}
```

`testStorage` is the existing storage fixture used by the poker handler tests; reuse whatever `pokerweb_test.go` already calls to get a `*storage.DB`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestResolveSuper`
Expected: FAIL to compile — "undefined: resolveSuper".

- [ ] **Step 3: Write the implementation**

Append to `supergame.go`:

```go
var errSuperUnavailable = errors.New("Супер гра недоступна")

// resolveSuper plays out tableID's pending game and applies the money.
//
// The transition out of "offered" happens under h.mu BEFORE anything is
// rolled or written, so a double-tap or a retried request finds a state
// that is no longer offered and is refused. This is the guard that stops a
// double payout; everything after it is safe to be slow.
func (h *PokerHub) resolveSuper(tableID, userID, game, pick string) (*superGame, error) {
	h.mu.Lock()
	g, ok := h.super[tableID]
	if !ok || g.State != "offered" || g.UserID != userID || time.Now().After(g.Deadline) {
		h.mu.Unlock()
		return nil, errSuperUnavailable
	}
	if game == "color" && pick != "red" && pick != "black" {
		h.mu.Unlock()
		return nil, errSuperUnavailable
	}
	g.State = "resolved"
	g.Game = game
	g.Pick = pick
	g.Deadline = time.Now().Add(superResultHold)
	h.mu.Unlock()

	switch game {
	case "dice":
		a, b := rand.Intn(6)+1, rand.Intn(6)+1
		g.Dice = [2]int{a, b}
		g.Outcome = diceOutcome(a, b)
	case "color":
		red := rand.Intn(2) == 0
		suits := map[bool][]string{true: {"♥", "♦"}, false: {"♠", "♣"}}[red]
		ranks := []string{"2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A"}
		g.Card = ranks[rand.Intn(len(ranks))] + suits[rand.Intn(len(suits))]
		g.Outcome = colorOutcome(red, pick)
	default: // "skip"
		g.Outcome = "keep"
	}
	g.Delta = superDelta(g.Stake, g.Outcome)

	if g.Delta != 0 && h.db != nil {
		// Two parties, one transaction, exactly like the bot-chip entries
		// in settle(): the player's gain is the bank's loss and the pair
		// sums to zero.
		if err := h.db.SettlePoker([]storage.PokerDelta{
			{UserID: g.UserID, Name: g.Name, Amount: g.Delta},
			{UserID: bankUserID, Name: "Банк", Amount: -g.Delta},
		}, "supergame"); err != nil {
			log.Printf("[poker] super game payout failed for table %s: %v", tableID, err)
		}
	}

	cp := *g
	return &cp, nil
}
```

Add `errors`, `log`, `net/http`, `encoding/json` and the `storage` import as needed.

Then the HTTP handler:

```go
func (h *PokerHub) handleSuper(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	var body struct {
		Game string `json:"game"`
		Pick string `json:"pick"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Некоректний запит", http.StatusBadRequest)
		return
	}
	switch body.Game {
	case "dice", "color", "skip":
	default:
		http.Error(w, "Невідома гра", http.StatusBadRequest)
		return
	}
	if _, err := h.resolveSuper(tbl.ID, fmt.Sprintf("%d", uid), body.Game, body.Pick); err != nil {
		http.Error(w, "Супер гра вже завершена", http.StatusConflict)
		return
	}
	h.broadcast(tbl)
	w.WriteHeader(http.StatusNoContent)
}
```

Register it in the action switch in `pokerweb.go`, next to `"leave"`:

```go
		case "super":
			h.handleSuper(w, r, tbl, uid)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run TestResolveSuper -v`
Expected: PASS, all three.

- [ ] **Step 5: Run the full suite**

Run: `cd go-bot && go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/supergame.go go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/supergame_test.go
git commit -m "Resolve a Супер гра and pay it out against the house"
```

---

### Task 7: Publishing the game to every client

The shared `super` block on the envelope, so all seats see the same offer and the same result.

**Files:**
- Modify: `go-bot/internal/handlers/pokerchat.go:46-49` (`tableEnvelope`), `:76-78` (`envelope`)
- Modify: `go-bot/internal/handlers/pokerweb.go:2424` (the broadcast envelope)
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: `superFor` (Task 4).
- Produces: `type superView struct` serialised as `super` on the envelope.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestEnvelopeCarriesTheSuperGame`
Expected: FAIL to compile — "env.Super undefined".

- [ ] **Step 3: Write the implementation**

In `pokerchat.go`, extend the envelope:

```go
type tableEnvelope struct {
	poker.TableView
	Chat  []chatMsg  `json:"chat"`
	Super *superView `json:"super,omitempty"`
}
```

```go
func (h *PokerHub) envelope(tbl *poker.Table, userID string) tableEnvelope {
	return tableEnvelope{
		TableView: tbl.ViewFor(userID),
		Chat:      h.chatSnapshot(tbl.ID),
		Super:     h.superView(tbl.ID),
	}
}
```

Append to `supergame.go`:

```go
// superView is the wire form of a pending game. It hangs off tableEnvelope
// rather than poker.TableView so internal/poker stays untouched, and it is
// identical for every viewer -- the whole point is that the table watches
// one result together.
type superView struct {
	UserID  string `json:"user_id"`
	Name    string `json:"name"`
	Stake   int    `json:"stake"`
	State   string `json:"state"`
	Game    string `json:"game,omitempty"`
	Pick    string `json:"pick,omitempty"`
	Dice    []int  `json:"dice,omitempty"`
	Card    string `json:"card,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Delta   int    `json:"delta,omitempty"`
	Left    int    `json:"left"` // seconds remaining on the current phase
}

func (h *PokerHub) superView(tableID string) *superView {
	g := h.superFor(tableID)
	if g == nil {
		return nil
	}
	left := int(time.Until(g.Deadline).Seconds())
	if left < 0 {
		left = 0
	}
	v := &superView{
		UserID: g.UserID, Name: g.Name, Stake: g.Stake, State: g.State,
		Game: g.Game, Pick: g.Pick, Card: g.Card,
		Outcome: g.Outcome, Delta: g.Delta, Left: left,
	}
	if g.Dice[0] != 0 {
		v.Dice = []int{g.Dice[0], g.Dice[1]}
	}
	return v
}
```

Change the broadcast at `pokerweb.go:2424` to build its envelope the same way, so a broadcast and a fresh snapshot never disagree:

```go
		case s.ch <- tableEnvelope{TableView: tbl.ViewFor(s.userID), Chat: msgs, Super: h.superViewLocked(tbl.ID)}:
```

`broadcast` already holds `h.mu`, so add a `superViewLocked` that does the same work without re-taking the lock, and have `superView` take the lock and call it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run TestEnvelopeCarriesTheSuperGame -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `cd go-bot && go test ./...`
Expected: PASS. Watch specifically for a deadlock in `broadcast` — if the suite hangs, `superView` is being called while `h.mu` is already held.

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/pokerchat.go go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/supergame.go go-bot/internal/handlers/supergame_test.go
git commit -m "Publish the Супер гра to every seat"
```

---

### Task 8: The client — panel, buttons, animations

**Files:**
- Modify: `go-bot/internal/handlers/pokerweb.go` (the `pokerTmpl` CSS block, the markup near `<div id="win">`, and `render()`)
- Test: `go-bot/internal/handlers/poker_test.go`

**Interfaces:**
- Consumes: the `super` block from Task 7 and the `POST /api/poker/{id}/super` endpoint from Task 6.
- Produces: no Go interface; a rendered panel.

- [ ] **Step 1: Write the failing test**

```go
// Pins the Супер гра surface the same way the hookah and blame lines are
// pinned: the labels are the feature, and a silent edit would fail nothing
// else. Also pins that the client never decides an outcome -- the roll is
// the server's, and a Math.random in this panel would be a money bug.
func TestSuperGamePanelIsRenderedAndServerDriven(t *testing.T) {
	var b strings.Builder
	if err := pokerTmpl.Execute(&b, map[string]string{"TableID": "a1b2c3d4e5f60718"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"Супер гра",
		"Кубики",
		"Червоне",
		"Чорне",
		"Пас",
		"#supergame",
		"@keyframes dicetumble",
		"@keyframes superwin",
		`"/super"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	if !strings.Contains(out, "prefers-reduced-motion") {
		t.Errorf("super game animations have no reduced-motion guard")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestSuperGamePanel`
Expected: FAIL — missing "Супер гра" and the rest.

- [ ] **Step 3: Write the implementation**

Add to the template, inside `#felt` after `<div id="win">`:

```html
<div id="supergame"><div class="sgbox">
  <div class="sgtitle">Супер гра</div>
  <div class="sgstake"></div>
  <div class="sgbody"></div>
  <div class="sgacts">
    <button data-sg="dice">Кубики</button>
    <button data-sg="red">Червоне</button>
    <button data-sg="black">Чорне</button>
    <button data-sg="skip" class="ghost">Пас</button>
  </div>
  <div class="sgwait"></div>
</div></div>
```

CSS: `#supergame` is a centred overlay over the felt, hidden unless a game is live. `.sgdie` tumbles through faces via `@keyframes dicetumble` (~1.2 s), `.sgcard` flips via `@keyframes cardflip`. Outcome animations: `@keyframes superwin` (gold burst, stake counting up), `@keyframes superlose` (desaturate, count down). All four keyframes get an `animation:none` entry in the existing `prefers-reduced-motion` block.

In `render()`, drive the panel entirely off `v.super`:

```js
  // The client only renders what the server already decided. It must never
  // roll anything itself: this panel moves real богдудіки.
  const sg=document.getElementById("supergame");
  const g=v.super;
  if(!g){sg.classList.remove("on");sgShown=null}
  else{
    sg.classList.add("on");
    const mine=g.user_id===myUserID;
    sg.querySelector(".sgacts").style.display=(mine&&g.state==="offered")?"":"none";
    sg.querySelector(".sgwait").textContent=
      g.state==="offered"
        ? (mine?("Обирай — "+g.left+" с"):(g.name+" грає Супер гру — "+g.left+" с"))
        : "";
    sg.querySelector(".sgstake").textContent=g.stake+" 🪙";
    if(g.state==="resolved"&&sgShown!==g.outcome){sgShown=g.outcome;playSuperResult(g)}
  }
```

Buttons post to the endpoint and let the broadcast update everyone:

```js
  sg.querySelectorAll("[data-sg]").forEach(btn=>btn.addEventListener("click",()=>{
    const k=btn.dataset.sg;
    const body=k==="dice"?{game:"dice"}:k==="skip"?{game:"skip"}:{game:"color",pick:k};
    sg.querySelector(".sgacts").style.display="none";   // no double-tap
    fetch("/api/poker/"+TABLE+"/super",{method:"POST",
      headers:{"X-Telegram-Init-Data":INIT},body:JSON.stringify(body)});
  }));
```

`playSuperResult(g)` runs the process animation for the game it was (`g.dice` or `g.card`), then the outcome animation for `g.outcome`, then leaves the final numbers on screen until the panel disappears.

- [ ] **Step 4: Run the test**

Run: `cd go-bot && go test ./internal/handlers/ -run TestSuperGamePanel -v`
Expected: PASS.

- [ ] **Step 5: Check the JS actually parses**

The whole client dies on a syntax error, and no Go test would catch it.

```bash
cd /Users/dmytrosalo/Projects/own/fuck_work_bot && python3 - <<'PY'
import io, re
src = io.open("go-bot/internal/handlers/pokerweb.go", encoding="utf-8").read()
blocks = [b for b in re.findall(r"<script>(.*?)</script>", src, re.S) if len(b) > 1000]
js = "\n;\n".join(blocks).replace("{{.TableID}}", '"x"')
io.open("/tmp/client.js", "w", encoding="utf-8").write(re.sub(r"\{\{[^}]*\}\}", "[]", js))
PY
node --check /tmp/client.js && echo "JS SYNTAX OK"
```

Expected: `JS SYNTAX OK`.

- [ ] **Step 6: Run the full suite and commit**

```bash
cd go-bot && gofmt -l internal/handlers/pokerweb.go && go test ./...
git add go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/poker_test.go
git commit -m "Супер гра panel with dice and card animations"
```

---

### Task 9: Drop pending games on restart

Every push to main SIGTERMs the process. A game pending at that moment must vanish rather than half-apply.

**Files:**
- Modify: `go-bot/internal/handlers/pokerpersist.go`
- Test: `go-bot/internal/handlers/supergame_test.go`

**Interfaces:**
- Consumes: the hub `super` map (Task 4).
- Produces: nothing new — an assertion that the snapshot has no super field.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run the test**

Run: `cd go-bot && go test ./internal/handlers/ -run TestPendingSuperGameDoesNotSurvive -v`
Expected: PASS immediately — the hub map was deliberately kept off `poker.Table`, so this test documents and locks in that decision rather than driving new code.

- [ ] **Step 3: Add the note that makes it deliberate**

In `pokerpersist.go`, above `persistTable`:

```go
// A pending Супер гра is deliberately NOT part of the snapshot. It lives on
// PokerHub and dies with the process, so a redeploy mid-game leaves the
// player holding the winnings they already had and moves no money. Resuming
// one would mean deciding after the fact whether a payout that was never
// written should be, and there is no safe answer to that.
```

- [ ] **Step 4: Run the full suite and commit**

```bash
cd go-bot && go test ./...
git add go-bot/internal/handlers/pokerpersist.go go-bot/internal/handlers/supergame_test.go
git commit -m "Pin that a pending Супер гра dies with the process"
```

---

### Task 10: Manual verification before deploy

**Files:** none — this is a check, not a change.

- [ ] **Step 1: Build and run locally**

```bash
cd go-bot && CGO_ENABLED=0 go build -o bot ./cmd/bot/ && \
  TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

- [ ] **Step 2: Force an offer**

Temporarily set `superChance = 1.0`, `superMinBlinds = 0` and `superCooldown = 0`, rebuild, play a hand against the bots and win it.

- [ ] **Step 3: Check each of these by hand**

- The panel appears for the winner with four buttons, and for a second browser as a spectator line with the same countdown.
- Кубики: two dice tumble, land on the values the server sent, and the outcome animation matches.
- Червоне/чорне: the card flips and the outcome matches the pick.
- Пас resolves instantly and the next hand starts.
- Doing nothing for 10 s resolves as skipped and the next hand starts.
- Double-tapping a button pays exactly once: check with `SELECT * FROM transactions WHERE activity='supergame'`.
- The next hand does not start while the panel is up.

- [ ] **Step 4: Restore the real constants**

Set `superChance`, `superMinBlinds` and `superCooldown` back to `0.15`, `10` and `10 * time.Minute`. Rebuild and confirm `go test ./...` passes.

- [ ] **Step 5: Commit nothing**

This task must end with a clean `git status` for `go-bot/`. If the tuned constants are still in the diff, the deploy would ship a game that fires on every win.

---

## Self-Review

**Spec coverage:** humans only (T3), blocking hold (T4), 10 s + 4 s (T2, T4, T6), 15% + cooldown (T5), 10 blinds (T3), split pots (T3), server RNG (T6), bank payout (T1, T6), idempotency (T6), shared view (T7), animations (T8), restart drop (T9), reduced motion (T8). The spec's EV table is asserted in T2. Every row of the spec's edge-case table has a test in T3, T6 or T9 except "winner busts to 0", which the spec itself notes cannot happen because the offer requires `won > 0`.

**Placeholders:** none — every code step carries real code. Two steps name existing test helpers (`seatedTable`, `testStorage`) rather than inventing them, and say what to do if the signature differs.

**Type consistency:** `superGame` fields are written in T4 and read unchanged in T5, T6, T7, T9. `superDelta`/`diceOutcome`/`colorOutcome` signatures from T2 are used verbatim in T6. `SettlePoker`'s new `activity` parameter from T1 is used in T6. `superView` (T7) is consumed by the client in T8 under the same JSON names.

**Known risk carried forward:** T7 changes `broadcast`, which already holds `h.mu`, and `superView` takes `h.mu`. The split into `superView`/`superViewLocked` is what prevents a deadlock; T7 Step 5 says to watch for a hang if that split is skipped.
