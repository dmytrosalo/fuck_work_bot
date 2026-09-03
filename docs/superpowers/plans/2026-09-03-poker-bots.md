# Poker Bot Opponents Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add up to two break-even bot opponents to the poker Mini App, funded by a reserved bank account, so a single player can start a game.

**Architecture:** A pure decision function in `internal/poker` (no HTTP, no locking) decides a bot's action from table state. The existing sweeper drives it under the table lock, so no new goroutines and no new locking are introduced. Bot settlement deltas are summed into one reserved bank account, which keeps the engine's zero-sum invariant unchanged.

**Tech Stack:** Go 1.25.0, `github.com/chehsunliu/poker` (aliased `pk`), `modernc.org/sqlite`, stdlib `testing`.

**Spec:** `docs/superpowers/specs/2026-09-03-poker-bots-design.md`

## Global Constraints

- Module `github.com/dmytrosalo/fuck-work-bot`; Go code under `go-bot/`. Go 1.25.0, `CGO_ENABLED=0`.
- **Import alias:** inside `internal/poker` the library MUST be imported as `pk "github.com/chehsunliu/poker"` — our own package is also named `poker`.
- Tests use stdlib `testing` ONLY. No testify.
- All user-facing text is Ukrainian.
- **LOCK ORDERING (inviolable):** table lock OUTER, hub mutex `h.mu` INNER. `broadcast` is called with the table lock held. NEVER acquire a table lock while holding `h.mu`.
- **Settlement semantics are unchanged:** `UpdateBalance` with an EMPTY name (preserves display names), zero deltas skipped, one transaction per hand via `db.SettlePoker`.
- Existing constants: `MaxSeats = 6`, `SmallBlind = 50`, `BigBlind = 100`, `MinBuyIn = 1000`, `MaxBuyIn = 10000`, `TurnTimeout = 90s`.
- Bot seating rule: `bots = min(2, MaxSeats − humans)`.
- `gofmt -l` clean for files you touch. Seven files in `internal/handlers` (achievements.go, cardgames.go, cardimage.go, handlers.go, quiz.go, slots.go, wordle.go) are ALREADY unformatted — PRE-EXISTING, do not reformat, never run a package-wide `go fmt`.
- Do NOT modify `internal/poker/pots.go` or `internal/poker/showdown.go` — money code with three review rounds each.
- Stage only files you intentionally changed, explicit paths. NEVER `git add -A` (two stray files, `To` and a `python` symlink, must stay untracked).
- Verify: `cd go-bot && go test ./... -race -count=1` and `CGO_ENABLED=0 go build -o /tmp/bot ./cmd/bot/`.

---

## File Structure

| File | Responsibility |
|---|---|
| `go-bot/internal/poker/bot.go` | Pure bot decision: preflop tiers, postflop strength, action choice |
| `go-bot/internal/poker/bot_test.go` | Decision tests + self-play simulation |
| `go-bot/internal/poker/table.go` | (modify) `SitBot` — seat a bot without buy-in clamps |
| `go-bot/internal/handlers/pokerbots.go` | Bot/bank identity, `ensureBots` seating policy, sweeper driving |
| `go-bot/internal/handlers/pokerbots_test.go` | Seating-policy and driving tests |
| `go-bot/internal/handlers/pokerweb.go` | (modify) route bot deltas to the bank; call `ensureBots` |
| `go-bot/internal/storage/sqlite.go` | (modify) exclude bot/bank ids from `GetTopBalances` |

---

### Task 1: Identity constants and leaderboard exclusion

**Files:**
- Create: `go-bot/internal/handlers/pokerbots.go`
- Modify: `go-bot/internal/storage/sqlite.go:693-710` (`GetTopBalances`)
- Test: `go-bot/internal/storage/sqlite_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `handlers.botUserPrefix = "bot:"`, `handlers.bankUserID = "bank:house"`, `handlers.isBotUser(userID string) bool`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/storage/sqlite_test.go — add to the existing file
func TestGetTopBalancesExcludesBotsAndBank(t *testing.T) {
	db := newTestDB(t)

	db.UpdateBalance("460670583", "Danya", 5000)
	db.UpdateBalance("bank:house", "Bank", 99999)
	db.UpdateBalance("bot:1", "Вася", 88888)

	for _, e := range db.GetTopBalances(10) {
		if e.UserID == "bank:house" || e.UserID == "bot:1" {
			t.Errorf("leaderboard leaked a non-player row: %s (%s)", e.UserID, e.Name)
		}
	}
	found := false
	for _, e := range db.GetTopBalances(10) {
		if e.UserID == "460670583" {
			found = true
		}
	}
	if !found {
		t.Error("real player missing from leaderboard")
	}
}
```

Use the same `newTestDB(t)` helper the existing tests in that file use. If it is named differently there, follow the existing convention rather than inventing one.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/storage/ -run TestGetTopBalancesExcludes -v`
Expected: FAIL — the bank and bot rows appear in the leaderboard.

- [ ] **Step 3: Filter at the query**

In `GetTopBalances` (sqlite.go:694), change the query so every present and future caller is covered rather than filtering at one call site:

```go
	rows, err := d.db.Query(`SELECT user_id, name, coins FROM balances
		WHERE user_id NOT LIKE 'bot:%' AND user_id NOT LIKE 'bank:%'
		ORDER BY coins DESC LIMIT ?`, limit)
```

- [ ] **Step 4: Create the identity file**

```go
// go-bot/internal/handlers/pokerbots.go
package handlers

import "strings"

const (
	// botUserPrefix marks a seat occupied by a bot rather than a Telegram
	// user. It is the single discriminator used for settlement routing and
	// for keeping bots out of leaderboards and stats.
	botUserPrefix = "bot:"

	// bankUserID is the house account that funds bots. Bot winnings and
	// losses are netted into this one row per hand, so humans + bank still
	// sum to zero and no per-bot rows pollute activity stats.
	bankUserID = "bank:house"

	// maxBots is the ceiling on bots at one table, regardless of free seats.
	maxBots = 2
)

// isBotUser reports whether a seat belongs to a bot.
func isBotUser(userID string) bool {
	return strings.HasPrefix(userID, botUserPrefix)
}
```

- [ ] **Step 5: Run tests**

Run: `cd go-bot && go test ./internal/storage/ ./internal/handlers/ -race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/pokerbots.go go-bot/internal/storage/sqlite.go go-bot/internal/storage/sqlite_test.go
git commit -m "feat(poker): bot/bank identity and leaderboard exclusion"
```

---

### Task 2: `SitBot` — seat a bot without buy-in clamps

**Files:**
- Modify: `go-bot/internal/poker/table.go` (add after `Sit`, around line 92)
- Test: `go-bot/internal/poker/table_test.go`

**Interfaces:**
- Consumes: `Table`, `Seat`, `MaxSeats`, `ErrTableFull`, `ErrAlreadySat`
- Produces: `func (t *Table) SitBot(userID, name string, stack int) error`

**Why this exists:** `Sit` rejects buy-ins below `MinBuyIn` and clamps them to `MaxBuyIn = 10000`. A bot must match the top human's stack *exactly*, and a human who has been winning can hold more than 10000, so `Sit` cannot be reused.

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/table_test.go — append
func TestSitBotIgnoresBuyInClamps(t *testing.T) {
	tbl := NewTable("t1", 1)
	if err := tbl.SitBot("bot:1", "Вася", 25000); err != nil {
		t.Fatalf("SitBot: %v", err)
	}
	if got := tbl.Seats[0].Stack; got != 25000 {
		t.Errorf("stack = %d, want 25000 (SitBot must not clamp to MaxBuyIn)", got)
	}
	if err := tbl.SitBot("bot:2", "Петро", 100); err != nil {
		t.Fatalf("SitBot below MinBuyIn: %v", err)
	}
	if got := tbl.Seats[1].Stack; got != 100 {
		t.Errorf("stack = %d, want 100 (SitBot must not reject below MinBuyIn)", got)
	}
}

func TestSitBotStillEnforcesSeatsAndDuplicates(t *testing.T) {
	tbl := NewTable("t1", 1)
	for i := 0; i < MaxSeats; i++ {
		if err := tbl.SitBot("bot:"+string(rune('a'+i)), "B", 1000); err != nil {
			t.Fatalf("seat %d: %v", i, err)
		}
	}
	if err := tbl.SitBot("bot:overflow", "B", 1000); err == nil {
		t.Error("expected ErrTableFull at a full table")
	}

	tbl2 := NewTable("t2", 1)
	_ = tbl2.SitBot("bot:1", "Вася", 1000)
	if err := tbl2.SitBot("bot:1", "Вася", 1000); err == nil {
		t.Error("expected ErrAlreadySat for a duplicate bot")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestSitBot -v`
Expected: FAIL — `undefined: SitBot`

- [ ] **Step 3: Implement**

```go
// SitBot seats a bot with an exact stack, bypassing the human buy-in rules.
// Bots are funded by the house rather than a balance, so MinBuyIn/MaxBuyIn
// do not apply — a bot matches the top human's stack, which may exceed
// MaxBuyIn once that player has been winning. Seat limits and the
// duplicate-user check still apply.
func (t *Table) SitBot(userID, name string, stack int) error {
	if len(t.Seats) >= MaxSeats {
		return ErrTableFull
	}
	for _, s := range t.Seats {
		if s.UserID == userID {
			return ErrAlreadySat
		}
	}
	t.Seats = append(t.Seats, &Seat{UserID: userID, Name: name, Stack: stack})
	t.Seq++
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd go-bot && go test ./internal/poker/ -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/table.go go-bot/internal/poker/table_test.go
git commit -m "feat(poker): SitBot seats a bot with an exact stack"
```

---

### Task 3: Hand strength

**Files:**
- Create: `go-bot/internal/poker/bot.go`
- Test: `go-bot/internal/poker/bot_test.go`

**Interfaces:**
- Consumes: `Best(hole, board []pk.Card) int32`, test helper `cards(ss ...string) []pk.Card`
- Produces: `func handStrength(hole, board []pk.Card) float64` returning 0.0–1.0

**Why two paths:** `pk.Evaluate` panics on fewer than five cards, so a two-card preflop hand cannot be evaluated. Preflop uses a tier heuristic; postflop uses the real evaluator.

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/bot_test.go
package poker

import "testing"

func TestHandStrengthPreflopOrdersHands(t *testing.T) {
	aces := handStrength(cards("Ah", "As"), nil)
	suitedConnector := handStrength(cards("8h", "9h"), nil)
	trash := handStrength(cards("2c", "7d"), nil)

	if !(aces > suitedConnector && suitedConnector > trash) {
		t.Fatalf("preflop ordering wrong: AA=%.2f 89s=%.2f 72o=%.2f", aces, suitedConnector, trash)
	}
	if aces > 1.0 || trash < 0.0 {
		t.Errorf("strength out of range: AA=%.2f 72o=%.2f", aces, trash)
	}
}

func TestHandStrengthPostflopOrdersHands(t *testing.T) {
	board := cards("Ah", "Kh", "7d", "2c", "9s")
	twoPair := handStrength(cards("Ad", "Kd"), board)
	highCard := handStrength(cards("3c", "4s"), board)

	if twoPair <= highCard {
		t.Fatalf("postflop ordering wrong: two pair=%.2f high card=%.2f", twoPair, highCard)
	}
	if twoPair > 1.0 || highCard < 0.0 {
		t.Errorf("strength out of range: %.2f %.2f", twoPair, highCard)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestHandStrength -v`
Expected: FAIL — `undefined: handStrength`

- [ ] **Step 3: Implement**

```go
// go-bot/internal/poker/bot.go
package poker

import (
	pk "github.com/chehsunliu/poker"
)

// rankOrder maps a card's rank character to 0 (deuce) through 12 (ace).
const rankOrder = "23456789TJQKA"

func rankIndex(c pk.Card) int {
	s := c.String()
	if len(s) == 0 {
		return 0
	}
	for i := 0; i < len(rankOrder); i++ {
		if rankOrder[i] == s[0] {
			return i
		}
	}
	return 0
}

func suitOf(c pk.Card) byte {
	s := c.String()
	if len(s) < 2 {
		return '?'
	}
	return s[1]
}

// worstRank is the numeric rank pk.Evaluate assigns to the weakest possible
// five-card hand (7-5-4-3-2 offsuit). Lower is stronger, so it is the
// denominator when normalising a rank to a 0..1 strength.
const worstRank = 7462

// handStrength scores a holding from 0.0 (hopeless) to 1.0 (nuts).
//
// pk.Evaluate panics on fewer than five cards, so a preflop hand (two cards,
// empty board) cannot go through the evaluator. Preflop therefore uses a
// tier heuristic; once there are at least three board cards the real
// evaluator is used and its rank is normalised.
func handStrength(hole, board []pk.Card) float64 {
	if len(board) < 3 {
		return preflopStrength(hole)
	}
	rank := Best(hole, board)
	s := 1.0 - float64(rank)/float64(worstRank)
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// preflopStrength scores a two-card holding using the usual shape heuristics:
// pairs are strong and scale with rank, suited cards gain, and connected
// cards gain. The absolute numbers matter less than the ordering, which the
// decision thresholds are tuned against.
func preflopStrength(hole []pk.Card) float64 {
	if len(hole) < 2 {
		return 0
	}
	a, b := rankIndex(hole[0]), rankIndex(hole[1])
	hi, lo := a, b
	if lo > hi {
		hi, lo = lo, hi
	}

	if hi == lo { // pocket pair: 22 -> 0.50, AA -> 0.95
		return 0.50 + 0.45*float64(hi)/12.0
	}

	s := 0.30 * float64(hi) / 12.0 // high card carries most of it
	s += 0.15 * float64(lo) / 12.0
	if suitOf(hole[0]) == suitOf(hole[1]) {
		s += 0.08
	}
	if gap := hi - lo; gap == 1 {
		s += 0.06
	} else if gap == 2 {
		s += 0.03
	}
	if s > 1 {
		return 1
	}
	return s
}
```

- [ ] **Step 4: Run tests**

Run: `cd go-bot && go test ./internal/poker/ -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/bot.go go-bot/internal/poker/bot_test.go
git commit -m "feat(poker): bot hand-strength scoring"
```

---

### Task 4: The decision function

**Files:**
- Modify: `go-bot/internal/poker/bot.go`
- Test: `go-bot/internal/poker/bot_test.go`

**Interfaces:**
- Consumes: `handStrength`, `Action` (`ActFold`/`ActCheck`/`ActCall`/`ActRaise`), `BigBlind`
- Produces: `type BotInput struct{...}`, `func Decide(in BotInput, rng *rand.Rand) (Action, int)`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/bot_test.go — append
import "math/rand" // add to the import block

func fixedRNG() *rand.Rand { return rand.New(rand.NewSource(1)) }

func TestDecideChecksWhenFree(t *testing.T) {
	in := BotInput{Hole: cards("2c", "7d"), Board: cards("Ah", "Kd", "9s"),
		ToCall: 0, Pot: 300, Stack: 5000, MinRaise: BigBlind}
	a, _ := Decide(in, fixedRNG())
	if a == ActFold {
		t.Error("must never fold when checking is free")
	}
}

func TestDecideFoldsTrashFacingBigBet(t *testing.T) {
	in := BotInput{Hole: cards("2c", "7d"), Board: cards("Ah", "Kd", "9s"),
		ToCall: 4000, Pot: 300, Stack: 5000, MinRaise: BigBlind}
	a, _ := Decide(in, fixedRNG())
	if a != ActFold {
		t.Errorf("got %v, want fold with trash facing a huge bet", a)
	}
}

func TestDecideNeverExceedsStack(t *testing.T) {
	in := BotInput{Hole: cards("Ah", "As"), Board: cards("Ad", "Kd", "9s"),
		ToCall: 100, Pot: 5000, Stack: 250, MinRaise: BigBlind, Bet: 0}
	_, amount := Decide(in, fixedRNG())
	if amount > 250 {
		t.Errorf("amount %d exceeds stack 250", amount)
	}
}

func TestDecideRaisesWithAStrongHand(t *testing.T) {
	in := BotInput{Hole: cards("Ah", "Ad"), Board: cards("As", "Kd", "9s"),
		ToCall: 100, Pot: 600, Stack: 5000, MinRaise: BigBlind, Bet: 0}
	sawRaise := false
	for seed := int64(0); seed < 20 && !sawRaise; seed++ {
		if a, _ := Decide(in, rand.New(rand.NewSource(seed))); a == ActRaise {
			sawRaise = true
		}
	}
	if !sawRaise {
		t.Error("trips should raise at least sometimes across 20 seeds")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestDecide -v`
Expected: FAIL — `undefined: BotInput`

- [ ] **Step 3: Implement**

```go
// BotInput is everything a bot needs to choose an action. It is a plain
// value: the decision function reads no table state and takes no locks, so
// it can be tested in isolation and called safely under the table lock.
type BotInput struct {
	Hole     []pk.Card
	Board    []pk.Card
	ToCall   int // chips needed to call; 0 means checking is free
	Pot      int // chips already in the pot this hand
	Stack    int // the bot's remaining chips
	MinRaise int // the smallest legal raise increment
	Bet      int // the bot's own contribution on this street
}

// bluffFrequency is how often a bot bets a weak hand. It keeps bots from
// being trivially readable without meaningfully changing their expected
// value, which the self-play simulation guards.
const bluffFrequency = 0.10

// Decide chooses an action for a bot. The returned amount is meaningful only
// for ActRaise, where it is the total this seat should have committed on the
// current street. The engine validates every action regardless, so a bad
// return here can only produce a rejected action, never an illegal one.
func Decide(in BotInput, rng *rand.Rand) (Action, int) {
	strength := handStrength(in.Hole, in.Board)

	// Free to see the next card: never fold.
	if in.ToCall <= 0 {
		if strength > 0.75 || rng.Float64() < bluffFrequency {
			return raiseOrAllIn(in, strength, rng)
		}
		return ActCheck, 0
	}

	// Facing a bet: compare hand strength against the price being offered.
	potOdds := float64(in.ToCall) / float64(in.Pot+in.ToCall)

	switch {
	case strength > 0.80:
		return raiseOrAllIn(in, strength, rng)
	case strength > potOdds+0.10:
		return ActCall, 0
	case rng.Float64() < bluffFrequency && in.ToCall < in.Stack/4:
		return ActCall, 0
	default:
		return ActFold, 0
	}
}

// raiseOrAllIn sizes a raise as a fraction of the pot, clamped to what the
// bot can actually put in. If the raise would not clear the minimum, it
// calls instead rather than returning an action the engine will reject.
func raiseOrAllIn(in BotInput, strength float64, rng *rand.Rand) (Action, int) {
	target := in.Bet + in.ToCall + int(float64(in.Pot)*(0.4+0.4*strength))
	max := in.Bet + in.Stack
	if target > max {
		target = max
	}
	if target < in.Bet+in.ToCall+in.MinRaise {
		if in.ToCall <= 0 {
			return ActCheck, 0
		}
		return ActCall, 0
	}
	return ActRaise, target
}
```

Add `"math/rand"` to `bot.go`'s import block.

- [ ] **Step 4: Run tests**

Run: `cd go-bot && go test ./internal/poker/ -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/bot.go go-bot/internal/poker/bot_test.go
git commit -m "feat(poker): bot decision function"
```

---

### Task 5: Route bot deltas to the bank

**Files:**
- Modify: `go-bot/internal/handlers/pokerweb.go:791-820` (`settle`)
- Test: `go-bot/internal/handlers/pokerbots_test.go`

**Interfaces:**
- Consumes: `isBotUser`, `bankUserID`, `storage.PokerDelta{UserID, Name, Amount}`, `db.SettlePoker([]PokerDelta) error`
- Produces: no new exported surface; `settle`'s behaviour changes

**The property this preserves:** human deltas plus the bank delta sum to exactly zero, so the engine's invariant is untouched. Bot deltas are *summed into one* bank entry rather than written per bot, which keeps activity stats free of bot rows.

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/handlers/pokerbots_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestSettleRoutesBotDeltas -v`
Expected: FAIL — the bot's delta is written under `bot:1` instead of the bank.

- [ ] **Step 3: Implement**

In `settle`, replace the entry-building loop:

```go
	if h.db != nil {
		entries := make([]storage.PokerDelta, 0, len(tbl.Seats))
		bankDelta := 0
		for _, s := range tbl.Seats {
			d, ok := deltas[s.UserID]
			if !ok || d == 0 {
				continue
			}
			// A bot has no balance of its own: its win or loss belongs to
			// the house. Netting every bot into one bank entry keeps humans
			// + bank summing to zero while leaving activity stats free of
			// bot rows.
			if isBotUser(s.UserID) {
				bankDelta += d
				continue
			}
			entries = append(entries, storage.PokerDelta{UserID: s.UserID, Name: s.Name, Amount: d})
		}
		if bankDelta != 0 {
			entries = append(entries, storage.PokerDelta{UserID: bankUserID, Name: "Банк", Amount: bankDelta})
		}
		...unchanged SettlePoker call and error log...
	}
```

- [ ] **Step 4: Run tests**

Run: `cd go-bot && go test ./internal/handlers/ -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/pokerbots_test.go
git commit -m "feat(poker): settle bot deltas against the house bank"
```

---

### Task 6: `ensureBots` — the seating policy

**Files:**
- Modify: `go-bot/internal/handlers/pokerbots.go`
- Test: `go-bot/internal/handlers/pokerbots_test.go`

**Interfaces:**
- Consumes: `poker.Table`, `SitBot`, `isBotUser`, `botUserPrefix`, `maxBots`, `poker.MaxSeats`
- Produces: `func (h *PokerHub) ensureBots(tbl *poker.Table)` — **caller must hold the table lock**

**Rule:** `targetBots = min(maxBots, poker.MaxSeats − humans)`; humans always keep their seats. Busted bots (stack 0) rebuy to the top human stack. Called between hands only.

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/handlers/pokerbots_test.go — append
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestEnsureBots -v`
Expected: FAIL — `h.ensureBots undefined`

- [ ] **Step 3: Implement**

```go
// go-bot/internal/handlers/pokerbots.go — append

// ensureBots brings a table's bot population in line with the seating rule
// and reloads any busted bot. The caller MUST hold the table lock; this
// touches Seats directly and takes no locks of its own.
//
// It must only run BETWEEN hands. Adding or removing a seat mid-hand would
// disturb the button and to-act indices the engine derives from seat order.
func (h *PokerHub) ensureBots(tbl *poker.Table) {
	humans, topStack := 0, 0
	for _, s := range tbl.Seats {
		if isBotUser(s.UserID) {
			continue
		}
		humans++
		if s.Stack > topStack {
			topStack = s.Stack
		}
	}

	// No humans means nobody to play against; leave the table empty so the
	// idle sweeper can reclaim it.
	if humans == 0 {
		return
	}

	target := poker.MaxSeats - humans
	if target > maxBots {
		target = maxBots
	}
	if target < 0 {
		target = 0
	}

	// Drop surplus bots (humans took their seats), preferring to remove from
	// the end so remaining seat order is stable.
	seats := tbl.Seats[:0]
	bots := 0
	for _, s := range tbl.Seats {
		if !isBotUser(s.UserID) {
			seats = append(seats, s)
			continue
		}
		if bots < target {
			// Reload a busted bot to match the current top human.
			if s.Stack <= 0 {
				s.Stack = topStack
			}
			seats = append(seats, s)
			bots++
		}
	}
	tbl.Seats = seats

	// Add bots up to the target.
	for i := 1; bots < target; i++ {
		id := fmt.Sprintf("%s%d", botUserPrefix, i)
		if tbl.SeatIndexOf(id) >= 0 {
			continue // that slot is taken; try the next number
		}
		if err := tbl.SitBot(id, botName(i), topStack); err != nil {
			break
		}
		bots++
	}
	tbl.Seq++
}

// botNames are the display names bots use, in order.
var botNames = []string{"Вася 🤖", "Петро 🤖"}

func botName(i int) string {
	if i-1 < len(botNames) && i >= 1 {
		return botNames[i-1]
	}
	return fmt.Sprintf("Бот %d 🤖", i)
}
```

Add `"fmt"` and the `poker` import to `pokerbots.go`.

- [ ] **Step 4: Run tests**

Run: `cd go-bot && go test ./internal/handlers/ -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/pokerbots.go go-bot/internal/handlers/pokerbots_test.go
git commit -m "feat(poker): bot seating policy"
```

---

### Task 7: Drive bots from the sweeper, and seat them between hands

**Files:**
- Modify: `go-bot/internal/handlers/pokerbots.go` (add `actBots`)
- Modify: `go-bot/internal/handlers/pokerweb.go:720` (join auto-start) and `:1092` (sweeper next hand)
- Test: `go-bot/internal/handlers/pokerbots_test.go`

**Interfaces:**
- Consumes: `poker.Decide`, `poker.BotInput`, `tbl.Act`, `ensureBots`
- Produces: `func (h *PokerHub) actBots(tbl *poker.Table) bool` — **caller must hold the table lock**; reports whether any bot acted

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/handlers/pokerbots_test.go — append
func TestActBotsAdvancesAHandWithoutHumans(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(1)
	tbl.Lock()
	_ = tbl.Sit("u1", "Danya", 5000)
	h.ensureBots(tbl)
	_ = tbl.StartHand()

	// The human folds immediately; the bots must be able to finish the hand
	// among themselves rather than leaving the table stuck.
	if tbl.Seats[tbl.ToAct].UserID == "u1" {
		_ = tbl.Act("u1", poker.ActFold, 0)
	}
	for i := 0; i < 200 && tbl.Stage != poker.StageShowdown; i++ {
		if s := tbl.Seats[tbl.ToAct]; !isBotUser(s.UserID) {
			_ = tbl.Act(s.UserID, poker.ActFold, 0)
			continue
		}
		if !h.actBots(tbl) {
			break
		}
	}
	stage := tbl.Stage
	tbl.Unlock()

	if stage != poker.StageShowdown {
		t.Fatalf("stage = %v, want showdown — bots did not drive the hand to a conclusion", stage)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestActBots -v`
Expected: FAIL — `h.actBots undefined`

- [ ] **Step 3: Implement `actBots`**

```go
// actBots plays one action for the seat to act, if that seat is a bot.
// Reports whether it acted. The caller MUST hold the table lock.
//
// One action per call, not a loop to completion: the sweeper calls this on
// its regular tick, so a bot's turn resolves within one interval. That pause
// reads as deliberation and, more importantly, keeps bots on machinery whose
// locking has already been reviewed rather than introducing new goroutines.
func (h *PokerHub) actBots(tbl *poker.Table) bool {
	if tbl.ToAct < 0 || tbl.ToAct >= len(tbl.Seats) {
		return false
	}
	s := tbl.Seats[tbl.ToAct]
	if !isBotUser(s.UserID) {
		return false
	}

	high := 0
	for _, o := range tbl.Seats {
		if o.Bet > high {
			high = o.Bet
		}
	}
	pot := 0
	for _, o := range tbl.Seats {
		pot += o.Committed
	}

	action, amount := poker.Decide(poker.BotInput{
		Hole:     s.Hole,
		Board:    tbl.Board,
		ToCall:   high - s.Bet,
		Pot:      pot,
		Stack:    s.Stack,
		MinRaise: tbl.MinRaise,
		Bet:      s.Bet,
	}, rand.New(rand.NewSource(time.Now().UnixNano())))

	if err := tbl.Act(s.UserID, action, amount); err != nil {
		// The engine rejected it (an illegal raise size, say). Fall back to
		// the always-legal action so a bot can never stall the table.
		fallback := poker.ActCheck
		if high > s.Bet {
			fallback = poker.ActFold
		}
		_ = tbl.Act(s.UserID, fallback, 0)
	}
	return true
}
```

Add `"math/rand"` and `"time"` to `pokerbots.go`'s imports.

- [ ] **Step 4: Wire it into the two between-hands points**

In `pokerweb.go` at the join auto-start (line ~720), seat bots before starting:

```go
		h.ensureBots(tbl)
		_ = tbl.StartHand()
```

In `pokerweb.go` at the sweeper's next-hand block (line ~1092), likewise:

```go
			h.ensureBots(tbl)
			if err := tbl.StartHand(); err == nil {
				h.broadcast(tbl)
			}
```

And in `sweepOnce`, after the `ForceTimeout` handling and while still holding the table lock, let a bot take its turn:

```go
		if h.actBots(tbl) {
			h.settleIfShowdown(tbl) // use whatever the existing transition guard is
			h.broadcast(tbl)
		}
```

Match the existing settle-on-transition pattern exactly — capture `prevStage` before the action and settle only when the stage has just become `StageShowdown`, the same condition `handleAction` uses. Do not invent a second settlement path.

- [ ] **Step 5: Run the full suite**

Run: `cd go-bot && go test ./... -race -count=1` and `CGO_ENABLED=0 go build -o /tmp/bot ./cmd/bot/`
Expected: PASS, build succeeds

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/pokerbots.go go-bot/internal/handlers/pokerbots_test.go go-bot/internal/handlers/pokerweb.go
git commit -m "feat(poker): drive bots from the sweeper"
```

---

### Task 8: Self-play simulation — is "break-even" true?

**Files:**
- Modify: `go-bot/internal/poker/bot_test.go`

**Interfaces:**
- Consumes: `NewTable`, `SitBot`, `StartHand`, `Decide`, `Showdown`, `Act`
- Produces: no production surface — this is the test that turns a tuning claim into a checked property

**Why:** "Roughly break-even" is a goal, not something thresholds prove. Without this, the spec would ship a claim nobody verifies — the same failure mode that produced three unfalsifiable tests during the poker build.

- [ ] **Step 1: Write the test**

```go
// go-bot/internal/poker/bot_test.go — append
func TestBotSelfPlayIsRoughlyBreakEven(t *testing.T) {
	const (
		hands     = 2000
		startingS = 100000 // large enough that nobody busts and skews the sample
	)
	rng := rand.New(rand.NewSource(42))

	tbl := NewTable("sim", 1)
	_ = tbl.SitBot("bot:1", "A", startingS)
	_ = tbl.SitBot("bot:2", "B", startingS)
	_ = tbl.SitBot("bot:3", "C", startingS)

	net := map[string]int{}
	for i := 0; i < hands; i++ {
		if err := tbl.StartHand(); err != nil {
			t.Fatalf("hand %d: %v", i, err)
		}
		for guard := 0; tbl.Stage != StageShowdown; guard++ {
			if guard > 500 {
				t.Fatalf("hand %d did not terminate", i)
			}
			s := tbl.Seats[tbl.ToAct]
			high := 0
			for _, o := range tbl.Seats {
				if o.Bet > high {
					high = o.Bet
				}
			}
			pot := 0
			for _, o := range tbl.Seats {
				pot += o.Committed
			}
			action, amount := Decide(BotInput{Hole: s.Hole, Board: tbl.Board,
				ToCall: high - s.Bet, Pot: pot, Stack: s.Stack,
				MinRaise: tbl.MinRaise, Bet: s.Bet}, rng)
			if err := tbl.Act(s.UserID, action, amount); err != nil {
				fallback := ActCheck
				if high > s.Bet {
					fallback = ActFold
				}
				if err := tbl.Act(s.UserID, fallback, 0); err != nil {
					t.Fatalf("hand %d: fallback rejected: %v", i, err)
				}
			}
		}
		for id, d := range tbl.Showdown() {
			net[id] += d
		}
		// Reload so a bust cannot end the simulation early.
		for _, s := range tbl.Seats {
			s.Stack = startingS
		}
	}

	// Every hand is zero-sum, so the totals must cancel exactly.
	sum := 0
	for _, d := range net {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("simulated deltas sum to %d, want 0 — chips created or destroyed", sum)
	}

	// No bot should be systematically printing or bleeding money. The band is
	// per-hand average, so it scales with the sample rather than being a
	// magic absolute number.
	const maxDriftPerHand = 60 // chips; big blind is 100
	for id, d := range net {
		if avg := float64(d) / hands; avg > maxDriftPerHand || avg < -maxDriftPerHand {
			t.Errorf("%s drifts %.1f chips/hand (band ±%d) — bots are not break-even",
				id, avg, maxDriftPerHand)
		}
	}
}
```

- [ ] **Step 2: Run it**

Run: `cd go-bot && go test ./internal/poker/ -run TestBotSelfPlay -v`
Expected: PASS. If the drift band fails, the thresholds in `Decide` need tuning — that is the expected iteration, not a plan defect. Report the observed drift values before changing anything, and tune `Decide`'s thresholds rather than widening the band.

- [ ] **Step 3: Commit**

```bash
git add go-bot/internal/poker/bot_test.go
git commit -m "test(poker): self-play simulation checks bots are break-even"
```

---

## Self-Review Notes

**Spec coverage:** bank routing → Task 5; bot identity and exclusion → Task 1; `min(2, 6−humans)` seating → Task 6; rebuy to top human → Task 6; decision function (preflop tiers + postflop evaluator + pot odds + bluffing) → Tasks 3-4; sweeper driving with no new goroutines → Task 7; self-play simulation → Task 8; no auth surface change → Tasks 6-7 seat bots internally, never through `handleJoin`.

**Known risks carried into implementation:**
- Task 6 mutates `tbl.Seats` directly and must only run between hands; adding or removing a seat mid-hand would disturb the button and to-act indices.
- Task 7 must reuse the existing settle-on-transition guard rather than adding a second settlement path — a second path is how double-settlement bugs appear.
- Task 8's drift band may need tuning on first run. Tune `Decide`, not the band.
