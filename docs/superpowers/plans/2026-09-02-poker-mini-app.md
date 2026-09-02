# Texas Hold'em Poker Mini App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a live 6-seat Texas Hold'em table to `fuck-work-bot`, played through a Telegram Mini App and restricted to members of the group chat, betting real богдудіки with a capped buy-in.

**Architecture:** A pure, dependency-light game engine in `internal/poker/` (no telebot, no net/http) owns all rules and returns per-viewer redacted views. A thin HTTP layer in `internal/handlers/pokerweb.go` authenticates via signed Telegram `initData` plus a `ChatMemberOf` membership check, streams state over SSE, and accepts actions. The browser is a dumb renderer that never receives another player's hole cards.

**Tech Stack:** Go 1.25.0, `gopkg.in/telebot.v3` v3.3.8, `github.com/chehsunliu/poker` (MIT, hand evaluation), `modernc.org/sqlite`, stdlib `net/http` + `html/template`, SSE.

**Spec:** `docs/superpowers/specs/2026-09-02-poker-mini-app-design.md`

## Global Constraints

- Module is `github.com/dmytrosalo/fuck-work-bot`; all Go code lives under `go-bot/`.
- Go 1.25.0. Build is **`CGO_ENABLED=0`** — never add a cgo dependency.
- **Import alias is mandatory:** our package is `package poker` and the library is also named `poker`. Every file that uses the library must import it as `pk "github.com/chehsunliu/poker"`. Using a bare `poker` import inside `internal/poker/` will not compile.
- Tests use **stdlib `testing` only**, table-driven, matching `internal/handlers/handlers_test.go`. Do **not** add testify or any assertion library.
- All user-facing text is **Ukrainian**.
- Do **not** add code to `internal/handlers/web.go` — it is already ~28K. Poker HTTP code goes in `internal/handlers/pokerweb.go`.
- Build: `cd go-bot && CGO_ENABLED=0 go build -o bot ./cmd/bot/`
- Test: `cd go-bot && go test ./...`
- Money constants (spec): `MaxSeats=6`, `SmallBlind=50`, `BigBlind=100`, `MinBuyIn=1000`, `MaxBuyIn=10000`, `TurnTimeout=90s`.
- Settlement calls `db.UpdateBalance(userID, "", delta)` — the empty name is **required** so the existing `CASE WHEN ? != ''` branch preserves the player's display name.

---

## File Structure

| File | Responsibility |
|---|---|
| `go-bot/internal/poker/cards.go` | Card helpers over `pk`, deck construction, hand ranking direction |
| `go-bot/internal/poker/pots.go` | Uncalled-bet return + side pot construction (money-critical) |
| `go-bot/internal/poker/table.go` | Table/Seat types, constants, sit, hand start, blinds, dealing |
| `go-bot/internal/poker/betting.go` | Action validation, street advancement |
| `go-bot/internal/poker/showdown.go` | Pot awarding, settlement deltas |
| `go-bot/internal/poker/view.go` | `TableView` redaction per viewer |
| `go-bot/internal/poker/*_test.go` | Engine tests (the money-safety proof) |
| `go-bot/internal/handlers/pokerauth.go` | Telegram `initData` HMAC verification |
| `go-bot/internal/handlers/pokerweb.go` | Routes, join, SSE stream, action endpoint, template |
| `go-bot/internal/handlers/poker.go` | `/poker` command, WebApp button |
| `go-bot/cmd/bot/main.go` | Wiring (modify) |

---

### Task 1: Engine package skeleton and hand ranking direction

**Files:**
- Create: `go-bot/internal/poker/cards.go`
- Test: `go-bot/internal/poker/cards_test.go`
- Modify: `go-bot/go.mod` (via `go get`)

**Interfaces:**
- Consumes: nothing
- Produces: `poker.NewShuffledDeck() []pk.Card`, `poker.Best(hole, board []pk.Card) int32`, constant `poker.MaxSeats`, `poker.SmallBlind`, `poker.BigBlind`, `poker.MinBuyIn`, `poker.MaxBuyIn`, `poker.TurnTimeout`

- [ ] **Step 1: Add the dependency**

```bash
cd go-bot && go get github.com/chehsunliu/poker@latest
```

- [ ] **Step 2: Write the failing test**

The library ports the `deuces` convention where a **lower** rank is a stronger hand. Everything downstream depends on this, so pin it explicitly rather than assuming.

```go
// go-bot/internal/poker/cards_test.go
package poker

import (
	"testing"

	pk "github.com/chehsunliu/poker"
)

func cards(ss ...string) []pk.Card {
	out := make([]pk.Card, 0, len(ss))
	for _, s := range ss {
		out = append(out, pk.NewCard(s))
	}
	return out
}

func TestLowerRankIsStronger(t *testing.T) {
	royal := Best(cards("Ah", "Kh"), cards("Qh", "Jh", "Th", "2c", "3d"))
	pair := Best(cards("As", "Ad"), cards("7h", "9c", "2s", "4d", "6h"))
	if royal >= pair {
		t.Fatalf("expected royal flush rank (%d) to be numerically lower than pair (%d)", royal, pair)
	}
}

func TestWheelStraight(t *testing.T) {
	wheel := Best(cards("Ah", "2d"), cards("3c", "4s", "5h", "Kd", "Qc"))
	high := Best(cards("Kh", "Qd"), cards("3c", "4s", "5h", "8d", "9c"))
	if wheel >= high {
		t.Fatalf("A-2-3-4-5 (%d) should beat king-high (%d)", wheel, high)
	}
}

func TestNewShuffledDeckIsComplete(t *testing.T) {
	d := NewShuffledDeck()
	if len(d) != 52 {
		t.Fatalf("deck size = %d, want 52", len(d))
	}
	seen := map[pk.Card]bool{}
	for _, c := range d {
		if seen[c] {
			t.Fatalf("duplicate card %v in deck", c)
		}
		seen[c] = true
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestLowerRankIsStronger -v`
Expected: FAIL — `undefined: Best`

- [ ] **Step 4: Write minimal implementation**

```go
// go-bot/internal/poker/cards.go
package poker

import (
	"math/rand"
	"time"

	pk "github.com/chehsunliu/poker"
)

const (
	MaxSeats    = 6
	SmallBlind  = 50
	BigBlind    = 100
	MinBuyIn    = 1000
	MaxBuyIn    = 10000
	TurnTimeout = 90 * time.Second
)

var allRanks = []string{"2", "3", "4", "5", "6", "7", "8", "9", "T", "J", "Q", "K", "A"}
var allSuits = []string{"s", "h", "d", "c"}

// NewShuffledDeck returns a full 52-card deck in random order.
func NewShuffledDeck() []pk.Card {
	deck := make([]pk.Card, 0, 52)
	for _, r := range allRanks {
		for _, s := range allSuits {
			deck = append(deck, pk.NewCard(r+s))
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

// Best returns the rank of the best five-card hand from hole+board.
// Lower is stronger (deuces convention, verified by TestLowerRankIsStronger).
func Best(hole, board []pk.Card) int32 {
	all := make([]pk.Card, 0, len(hole)+len(board))
	all = append(all, hole...)
	all = append(all, board...)
	return pk.Evaluate(all)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/poker/cards.go go-bot/internal/poker/cards_test.go go-bot/go.mod go-bot/go.sum
git commit -m "feat(poker): engine skeleton, deck and hand ranking"
```

---

### Task 2: Uncalled bet return and side pots

This is the money-critical task. Every later correctness guarantee rests on it.

**Files:**
- Create: `go-bot/internal/poker/pots.go`
- Create: `go-bot/internal/poker/table.go` (types only — the struct definitions used from here on)
- Test: `go-bot/internal/poker/pots_test.go`

**Interfaces:**
- Consumes: nothing from Task 1 except the package
- Produces: `type Seat struct{...}`, `type Pot struct{ Amount int; Eligible []int }`, `func ReturnUncalled(seats []*Seat)`, `func BuildPots(seats []*Seat) []Pot`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/pots_test.go
package poker

import "testing"

func seat(committed int, folded bool) *Seat {
	return &Seat{Committed: committed, Folded: folded}
}

func TestReturnUncalledGivesBackExcess(t *testing.T) {
	// A bets 500, B calls all-in for 200. A's extra 300 is uncalled.
	a := seat(500, false)
	b := seat(200, false)
	ReturnUncalled([]*Seat{a, b})
	if a.Committed != 200 {
		t.Errorf("A committed = %d, want 200", a.Committed)
	}
	if a.Stack != 300 {
		t.Errorf("A stack = %d, want 300 returned", a.Stack)
	}
}

func TestReturnUncalledNoopWhenMatched(t *testing.T) {
	a := seat(200, false)
	b := seat(200, false)
	ReturnUncalled([]*Seat{a, b})
	if a.Committed != 200 || a.Stack != 0 {
		t.Errorf("nothing should be returned, got committed=%d stack=%d", a.Committed, a.Stack)
	}
}

func TestBuildPotsSimpleSingle(t *testing.T) {
	pots := BuildPots([]*Seat{seat(100, false), seat(100, false)})
	if len(pots) != 1 {
		t.Fatalf("pots = %d, want 1", len(pots))
	}
	if pots[0].Amount != 200 {
		t.Errorf("amount = %d, want 200", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 2 {
		t.Errorf("eligible = %v, want 2 seats", pots[0].Eligible)
	}
}

func TestBuildPotsShortStackSidePot(t *testing.T) {
	// seat0 all-in 100, seat1 and seat2 continue to 300.
	pots := BuildPots([]*Seat{seat(100, false), seat(300, false), seat(300, false)})
	if len(pots) != 2 {
		t.Fatalf("pots = %d, want 2", len(pots))
	}
	if pots[0].Amount != 300 || len(pots[0].Eligible) != 3 {
		t.Errorf("main pot = %d/%v, want 300 with 3 eligible", pots[0].Amount, pots[0].Eligible)
	}
	if pots[1].Amount != 400 || len(pots[1].Eligible) != 2 {
		t.Errorf("side pot = %d/%v, want 400 with 2 eligible", pots[1].Amount, pots[1].Eligible)
	}
	for _, i := range pots[1].Eligible {
		if i == 0 {
			t.Error("short stack must not be eligible for the side pot")
		}
	}
}

func TestBuildPotsFoldedChipsCountButConferNoEligibility(t *testing.T) {
	// seat0 folded after committing 100; seats 1,2 at 100.
	pots := BuildPots([]*Seat{seat(100, true), seat(100, false), seat(100, false)})
	if len(pots) != 1 {
		t.Fatalf("pots = %d, want 1", len(pots))
	}
	if pots[0].Amount != 300 {
		t.Errorf("amount = %d, want 300 (folded chips still in the pot)", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 2 {
		t.Errorf("eligible = %v, want only the two live seats", pots[0].Eligible)
	}
}

func TestBuildPotsConservesChips(t *testing.T) {
	seats := []*Seat{seat(50, true), seat(275, false), seat(275, false), seat(120, false)}
	total := 0
	for _, s := range seats {
		total += s.Committed
	}
	sum := 0
	for _, p := range BuildPots(seats) {
		sum += p.Amount
	}
	if sum != total {
		t.Fatalf("pots total %d != committed total %d — chips created or destroyed", sum, total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestBuildPots -v`
Expected: FAIL — `undefined: Seat`, `undefined: BuildPots`

- [ ] **Step 3: Write the types**

```go
// go-bot/internal/poker/table.go
package poker

import (
	"sync"
	"time"

	pk "github.com/chehsunliu/poker"
)

type Stage int

const (
	StageWaiting Stage = iota
	StagePreflop
	StageFlop
	StageTurn
	StageRiver
	StageShowdown
)

type Seat struct {
	UserID     string
	Name       string
	Stack      int       // chips at the table
	Hole       []pk.Card
	Folded     bool
	AllIn      bool
	Committed  int // total committed THIS HAND — basis for side pots
	Bet        int // committed on the CURRENT street — basis for min-raise
	InHand     bool
	startStack int // stack when the hand began, for settlement deltas
}

type Pot struct {
	Amount   int
	Eligible []int // seat indices
}

type Table struct {
	ID       string
	ChatID   int64
	Seats    []*Seat
	Button   int
	Stage    Stage
	Board    []pk.Card
	Deck     []pk.Card
	ToAct    int
	MinRaise int
	Deadline time.Time
	Seq      uint64
	mu       sync.Mutex
}
```

- [ ] **Step 4: Write the pot implementation**

```go
// go-bot/internal/poker/pots.go
package poker

import "sort"

// ReturnUncalled gives back the portion of the largest commitment that no
// other player matched. Without this, an uncalled bet would be split into a
// pot that only its own bettor is eligible for, and the zero-sum invariant
// would still hold but the player would be paid from their own chips.
func ReturnUncalled(seats []*Seat) {
	max, second := 0, 0
	var top *Seat
	count := 0
	for _, s := range seats {
		if s.Committed > max {
			second = max
			max = s.Committed
			top = s
			count = 1
		} else if s.Committed == max {
			count++
		} else if s.Committed > second {
			second = s.Committed
		}
	}
	if top == nil || count > 1 || max == second {
		return
	}
	excess := max - second
	top.Committed -= excess
	top.Stack += excess
}

// BuildPots splits commitments into a main pot and any side pots.
// Folded players' chips stay in the pot but grant no eligibility.
func BuildPots(seats []*Seat) []Pot {
	levelSet := map[int]bool{}
	for _, s := range seats {
		if s.Committed > 0 {
			levelSet[s.Committed] = true
		}
	}
	levels := make([]int, 0, len(levelSet))
	for l := range levelSet {
		levels = append(levels, l)
	}
	sort.Ints(levels)

	pots := make([]Pot, 0, len(levels))
	prev := 0
	for _, lvl := range levels {
		amount := 0
		eligible := []int{}
		for i, s := range seats {
			if s.Committed >= lvl {
				amount += lvl - prev
				if !s.Folded {
					eligible = append(eligible, i)
				}
			}
		}
		if amount > 0 && len(eligible) > 0 {
			pots = append(pots, Pot{Amount: amount, Eligible: eligible})
		}
		prev = lvl
	}
	return pots
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS (all pot tests plus Task 1's)

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/poker/
git commit -m "feat(poker): side pots and uncalled bet return"
```

---

### Task 3: Table lifecycle — sit, start hand, blinds, dealing

**Files:**
- Modify: `go-bot/internal/poker/table.go`
- Test: `go-bot/internal/poker/table_test.go`

**Interfaces:**
- Consumes: `Seat`, `Table`, `Stage`, constants from Tasks 1-2
- Produces: `func NewTable(id string, chatID int64) *Table`, `func (t *Table) Sit(userID, name string, buyIn int) error`, `func (t *Table) StartHand() error`, `func (t *Table) seatedCount() int`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/table_test.go
package poker

import "testing"

func TestSitRejectsBelowMinBuyIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	if err := tbl.Sit("u1", "Danya", MinBuyIn-1); err == nil {
		t.Fatal("expected error for buy-in below minimum")
	}
}

func TestSitRejectsWhenFull(t *testing.T) {
	tbl := NewTable("t1", 1)
	for i := 0; i < MaxSeats; i++ {
		if err := tbl.Sit(string(rune('a'+i)), "P", MinBuyIn); err != nil {
			t.Fatalf("seat %d: %v", i, err)
		}
	}
	if err := tbl.Sit("overflow", "P", MinBuyIn); err == nil {
		t.Fatal("expected error when table is full")
	}
}

func TestSitRejectsDuplicateUser(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", MinBuyIn)
	if err := tbl.Sit("u1", "Danya", MinBuyIn); err == nil {
		t.Fatal("expected error when the same user sits twice")
	}
}

func TestStartHandNeedsTwoPlayers(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	if err := tbl.StartHand(); err == nil {
		t.Fatal("expected error starting a hand with one player")
	}
}

func TestStartHandPostsBlindsAndDeals(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	if tbl.Stage != StagePreflop {
		t.Errorf("stage = %v, want preflop", tbl.Stage)
	}
	posted := 0
	for _, s := range tbl.Seats {
		if len(s.Hole) != 2 {
			t.Errorf("seat %s has %d hole cards, want 2", s.UserID, len(s.Hole))
		}
		posted += s.Committed
	}
	if posted != SmallBlind+BigBlind {
		t.Errorf("blinds posted = %d, want %d", posted, SmallBlind+BigBlind)
	}
	if tbl.MinRaise != BigBlind {
		t.Errorf("MinRaise = %d, want %d", tbl.MinRaise, BigBlind)
	}
}

func TestShortStackPostsBlindAllInNotNegative(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", MinBuyIn)
	_ = tbl.Sit("u2", "Data", 5000)
	tbl.Seats[0].Stack = 20 // less than the small blind
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	for _, s := range tbl.Seats {
		if s.Stack < 0 {
			t.Fatalf("seat %s has negative stack %d", s.UserID, s.Stack)
		}
	}
	if !tbl.Seats[0].AllIn {
		t.Error("short stack should be marked all-in after posting a partial blind")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestStartHand -v`
Expected: FAIL — `undefined: NewTable`

- [ ] **Step 3: Write the implementation (append to `table.go`)**

```go
import "errors" // add to the existing import block in table.go

var (
	ErrTableFull   = errors.New("стіл заповнений")
	ErrAlreadySat  = errors.New("ти вже за столом")
	ErrBuyInTooLow = errors.New("замало богдудіків")
	ErrNeedPlayers = errors.New("потрібно щонайменше 2 гравці")
)

func NewTable(id string, chatID int64) *Table {
	return &Table{ID: id, ChatID: chatID, Stage: StageWaiting, Button: -1}
}

func (t *Table) Sit(userID, name string, buyIn int) error {
	if buyIn < MinBuyIn {
		return ErrBuyInTooLow
	}
	if buyIn > MaxBuyIn {
		buyIn = MaxBuyIn
	}
	if len(t.Seats) >= MaxSeats {
		return ErrTableFull
	}
	for _, s := range t.Seats {
		if s.UserID == userID {
			return ErrAlreadySat
		}
	}
	t.Seats = append(t.Seats, &Seat{UserID: userID, Name: name, Stack: buyIn})
	t.Seq++
	return nil
}

func (t *Table) seatedCount() int {
	n := 0
	for _, s := range t.Seats {
		if s.Stack > 0 {
			n++
		}
	}
	return n
}

// post moves up to amount from the seat's stack into the pot, marking the
// seat all-in when it cannot cover the full amount.
func (t *Table) post(s *Seat, amount int) {
	if amount >= s.Stack {
		amount = s.Stack
		s.AllIn = true
	}
	s.Stack -= amount
	s.Bet += amount
	s.Committed += amount
}

func (t *Table) StartHand() error {
	if t.seatedCount() < 2 {
		return ErrNeedPlayers
	}
	t.Deck = NewShuffledDeck()
	t.Board = nil
	t.Stage = StagePreflop
	t.MinRaise = BigBlind

	for _, s := range t.Seats {
		s.Hole = nil
		s.Folded = s.Stack <= 0
		s.AllIn = false
		s.Committed = 0
		s.Bet = 0
		s.InHand = s.Stack > 0
		s.startStack = s.Stack
	}

	t.Button = t.nextOccupied(t.Button)
	sb := t.nextOccupied(t.Button)
	bb := t.nextOccupied(sb)
	t.post(t.Seats[sb], SmallBlind)
	t.post(t.Seats[bb], BigBlind)

	for i := 0; i < 2; i++ {
		for _, s := range t.Seats {
			if s.InHand {
				s.Hole = append(s.Hole, t.draw())
			}
		}
	}

	t.ToAct = t.nextActive(bb)
	t.Deadline = time.Now().Add(TurnTimeout)
	t.Seq++
	return nil
}

func (t *Table) draw() pk.Card {
	c := t.Deck[0]
	t.Deck = t.Deck[1:]
	return c
}

// nextOccupied returns the next seat index with chips, wrapping around.
func (t *Table) nextOccupied(from int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (from + i%n + n) % n
		if t.Seats[idx].Stack > 0 || t.Seats[idx].InHand {
			return idx
		}
	}
	return 0
}

// nextActive returns the next seat still able to act.
func (t *Table) nextActive(from int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (from + i) % n
		s := t.Seats[idx]
		if s.InHand && !s.Folded && !s.AllIn {
			return idx
		}
	}
	return -1
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/
git commit -m "feat(poker): table lifecycle, blinds and dealing"
```

---

### Task 4: Betting actions and street advancement

**Files:**
- Create: `go-bot/internal/poker/betting.go`
- Test: `go-bot/internal/poker/betting_test.go`

**Interfaces:**
- Consumes: `Table`, `Seat`, `nextActive`, `post`, `draw` from Task 3
- Produces: `type Action string` with `ActFold/ActCheck/ActCall/ActRaise`, `func (t *Table) Act(userID string, a Action, amount int) error`, `func (t *Table) bettingClosed() bool`, `func (t *Table) advance()`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/betting_test.go
package poker

import "testing"

func headsUp(t *testing.T) *Table {
	t.Helper()
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	return tbl
}

func TestActRejectsOutOfTurn(t *testing.T) {
	tbl := headsUp(t)
	wrong := tbl.Seats[(tbl.ToAct+1)%len(tbl.Seats)].UserID
	if err := tbl.Act(wrong, ActCall, 0); err == nil {
		t.Fatal("expected error acting out of turn")
	}
}

func TestActRejectsCheckWhenBetOutstanding(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActCheck, 0); err == nil {
		t.Fatal("expected error checking with a bet outstanding")
	}
}

func TestActRejectsRaiseBelowMin(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActRaise, BigBlind+1); err == nil {
		t.Fatal("expected error raising below the minimum")
	}
}

func TestActRejectsRaiseAboveStack(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct]
	if err := tbl.Act(actor.UserID, ActRaise, actor.Stack+9999); err == nil {
		t.Fatal("expected error raising more than the stack")
	}
}

func TestFoldToOneEndsHandImmediately(t *testing.T) {
	tbl := headsUp(t)
	actor := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(actor, ActFold, 0); err != nil {
		t.Fatalf("fold: %v", err)
	}
	if tbl.Stage != StageShowdown {
		t.Errorf("stage = %v, want showdown after everyone folded", tbl.Stage)
	}
}

func TestCallThenCheckAdvancesToFlop(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(first, ActCall, 0); err != nil {
		t.Fatalf("call: %v", err)
	}
	second := tbl.Seats[tbl.ToAct].UserID
	if err := tbl.Act(second, ActCheck, 0); err != nil {
		t.Fatalf("check: %v", err)
	}
	if tbl.Stage != StageFlop {
		t.Fatalf("stage = %v, want flop", tbl.Stage)
	}
	if len(tbl.Board) != 3 {
		t.Errorf("board = %d cards, want 3", len(tbl.Board))
	}
	for _, s := range tbl.Seats {
		if s.Bet != 0 {
			t.Errorf("seat %s street bet = %d, want reset to 0", s.UserID, s.Bet)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestAct -v`
Expected: FAIL — `undefined: ActCall`

- [ ] **Step 3: Add the street-tracking field, then write the implementation**

First add this field to `Seat` in `table.go`. The betting code below references it, so it must exist first or the package will not compile:

```go
	actedThisStreet bool
```

Also reset it in `StartHand`'s per-seat loop, on the line after `s.Bet = 0`:

```go
		s.actedThisStreet = false
```

Then create the betting file:

```go
// go-bot/internal/poker/betting.go
package poker

import "errors"

type Action string

const (
	ActFold  Action = "fold"
	ActCheck Action = "check"
	ActCall  Action = "call"
	ActRaise Action = "raise"
)

var (
	ErrNotYourTurn = errors.New("не твій хід")
	ErrCannotCheck = errors.New("не можна чекати, є ставка")
	ErrRaiseTooLow = errors.New("замала ставка")
	ErrNotEnough   = errors.New("недостатньо фішок")
	ErrHandOver    = errors.New("роздача завершена")
)

// highBet returns the largest bet on the current street.
func (t *Table) highBet() int {
	high := 0
	for _, s := range t.Seats {
		if s.Bet > high {
			high = s.Bet
		}
	}
	return high
}

func (t *Table) Act(userID string, a Action, amount int) error {
	if t.Stage == StageWaiting || t.Stage == StageShowdown {
		return ErrHandOver
	}
	if t.ToAct < 0 || t.Seats[t.ToAct].UserID != userID {
		return ErrNotYourTurn
	}
	s := t.Seats[t.ToAct]
	s.actedThisStreet = true
	high := t.highBet()

	switch a {
	case ActFold:
		s.Folded = true
	case ActCheck:
		if s.Bet < high {
			return ErrCannotCheck
		}
	case ActCall:
		t.post(s, high-s.Bet)
	case ActRaise:
		if amount > s.Stack+s.Bet {
			return ErrNotEnough
		}
		if amount < high+t.MinRaise && amount < s.Stack+s.Bet {
			return ErrRaiseTooLow
		}
		t.MinRaise = amount - high
		t.post(s, amount-s.Bet)
	default:
		return ErrHandOver
	}

	t.Seq++

	if t.liveCount() <= 1 {
		t.Stage = StageShowdown
		return nil
	}
	if t.bettingClosed() {
		t.advance()
	} else {
		t.ToAct = t.nextActive(t.ToAct)
		t.Deadline = time.Now().Add(TurnTimeout)
	}
	return nil
}

// liveCount counts seats that have not folded.
func (t *Table) liveCount() int {
	n := 0
	for _, s := range t.Seats {
		if s.InHand && !s.Folded {
			n++
		}
	}
	return n
}

// bettingClosed reports whether every live, non-all-in seat has matched the
// high bet and has acted at least once this street.
func (t *Table) bettingClosed() bool {
	high := t.highBet()
	for _, s := range t.Seats {
		if !s.InHand || s.Folded || s.AllIn {
			continue
		}
		if s.Bet != high || !s.actedThisStreet {
			return false
		}
	}
	return true
}

// advance deals the next street, or moves to showdown after the river.
func (t *Table) advance() {
	for _, s := range t.Seats {
		s.Bet = 0
		s.actedThisStreet = false
	}
	t.MinRaise = BigBlind

	switch t.Stage {
	case StagePreflop:
		t.Stage = StageFlop
		t.Board = append(t.Board, t.draw(), t.draw(), t.draw())
	case StageFlop:
		t.Stage = StageTurn
		t.Board = append(t.Board, t.draw())
	case StageTurn:
		t.Stage = StageRiver
		t.Board = append(t.Board, t.draw())
	case StageRiver:
		t.Stage = StageShowdown
		return
	}
	t.ToAct = t.nextActive(t.Button)
	t.Deadline = time.Now().Add(TurnTimeout)
}
```

- [ ] **Step 4: Confirm the package compiles**

Run: `cd go-bot && go build ./internal/poker/`
Expected: success. `betting.go` needs `"time"` in its import block for the `Deadline` updates — add it if the build complains.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/poker/
git commit -m "feat(poker): betting actions and street advancement"
```

---

### Task 5: Showdown, pot awarding, and the zero-sum settlement invariant

**Files:**
- Create: `go-bot/internal/poker/showdown.go`
- Test: `go-bot/internal/poker/showdown_test.go`

**Interfaces:**
- Consumes: `BuildPots`, `ReturnUncalled`, `Best`, `Table`, `Seat`
- Produces: `func (t *Table) Showdown() map[string]int` returning `userID -> signed delta`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/showdown_test.go
package poker

import "testing"

func TestShowdownDeltasSumToZero(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.Sit("u3", "Bo", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Drive to showdown: everyone calls, then checks down.
	for tbl.Stage != StageShowdown {
		s := tbl.Seats[tbl.ToAct]
		if s.Bet < tbl.highBet() {
			_ = tbl.Act(s.UserID, ActCall, 0)
		} else {
			_ = tbl.Act(s.UserID, ActCheck, 0)
		}
	}
	deltas := tbl.Showdown()
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("settlement deltas sum to %d, want 0 — money was created or destroyed", sum)
	}
}

func TestShowdownShortStackCannotWinMoreThanPaidIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Short", MinBuyIn)
	_ = tbl.Sit("u2", "Big", 5000)
	_ = tbl.Sit("u3", "Also", 5000)
	_ = tbl.StartHand()

	tbl.Seats[0].Committed, tbl.Seats[0].Folded, tbl.Seats[0].AllIn = 100, false, true
	tbl.Seats[1].Committed, tbl.Seats[1].Folded = 300, false
	tbl.Seats[2].Committed, tbl.Seats[2].Folded = 300, false
	// Give the short stack the winning hand.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Ah", "As")
	tbl.Seats[1].Hole = cards("Kd", "Qd")
	tbl.Seats[2].Hole = cards("3c", "5h")

	deltas := tbl.Showdown()
	// Short stack paid 100, so may win at most 100 from each of the other two.
	if deltas["u1"] > 200 {
		t.Fatalf("short stack won %d, cannot exceed 200", deltas["u1"])
	}
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("deltas sum to %d, want 0", sum)
	}
}

func TestShowdownSplitPotOddChip(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	_ = tbl.StartHand()
	tbl.Button = 0
	tbl.Seats[0].Committed, tbl.Seats[0].Folded, tbl.Seats[0].AllIn = 75, false, false
	tbl.Seats[1].Committed, tbl.Seats[1].Folded, tbl.Seats[1].AllIn = 75, false, false
	// Identical hands: the board plays.
	tbl.Board = cards("Ah", "Kh", "Qh", "Jh", "Th")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")

	deltas := tbl.Showdown()
	sum := deltas["u1"] + deltas["u2"]
	if sum != 0 {
		t.Fatalf("split pot deltas sum to %d, want 0 (odd chip must not vanish)", sum)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestShowdown -v`
Expected: FAIL — `undefined: Showdown`

- [ ] **Step 3: Write the implementation**

```go
// go-bot/internal/poker/showdown.go
package poker

// Showdown returns the uncalled bet, awards every pot, and reports each
// player's signed balance delta for the hand. The deltas always sum to zero.
func (t *Table) Showdown() map[string]int {
	ReturnUncalled(t.Seats)

	for _, pot := range BuildPots(t.Seats) {
		winners := bestOf(t, pot.Eligible)
		share := pot.Amount / len(winners)
		odd := pot.Amount - share*len(winners)
		for _, idx := range winners {
			t.Seats[idx].Stack += share
		}
		// The odd chip goes to the first winner left of the button.
		if odd > 0 {
			t.Seats[firstLeftOfButton(t, winners)].Stack += odd
		}
	}

	deltas := make(map[string]int, len(t.Seats))
	for _, s := range t.Seats {
		if s.InHand {
			deltas[s.UserID] = s.Stack - s.startStack
		}
		s.Committed = 0
		s.Bet = 0
	}
	t.Stage = StageShowdown
	t.Seq++
	return deltas
}

// bestOf returns the indices holding the strongest hand among the candidates.
// If only one candidate remains (everyone else folded), it wins uncontested.
func bestOf(t *Table, candidates []int) []int {
	if len(candidates) == 1 {
		return candidates
	}
	best := int32(-1)
	var winners []int
	for _, idx := range candidates {
		r := Best(t.Seats[idx].Hole, t.Board)
		if best == -1 || r < best { // lower rank is stronger
			best, winners = r, []int{idx}
		} else if r == best {
			winners = append(winners, idx)
		}
	}
	return winners
}

func firstLeftOfButton(t *Table, winners []int) int {
	n := len(t.Seats)
	for i := 1; i <= n; i++ {
		idx := (t.Button + i) % n
		for _, w := range winners {
			if w == idx {
				return idx
			}
		}
	}
	return winners[0]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/
git commit -m "feat(poker): showdown, pot awarding, zero-sum settlement"
```

---

### Task 6: Per-viewer redaction

**Files:**
- Create: `go-bot/internal/poker/view.go`
- Test: `go-bot/internal/poker/view_test.go`

**Interfaces:**
- Consumes: `Table`, `Seat`, `BuildPots`
- Produces: `type TableView struct{...}`, `type SeatView struct{...}`, `func (t *Table) ViewFor(userID string) TableView`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/view_test.go
package poker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestViewHidesOtherPlayersHoleCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()
	tbl.Seats[1].Hole = cards("Ah", "Ks")

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u2" && len(s.Hole) != 0 {
			t.Fatalf("viewer u1 can see u2's hole cards: %v", s.Hole)
		}
	}

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "Ah") || strings.Contains(string(raw), "Ks") {
		t.Fatalf("serialized view leaks another player's cards: %s", raw)
	}
}

func TestViewShowsOwnHoleCards(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.StartHand()

	v := tbl.ViewFor("u1")
	for _, s := range v.Seats {
		if s.UserID == "u1" && len(s.Hole) != 2 {
			t.Fatalf("viewer cannot see own cards: %v", s.Hole)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestView -v`
Expected: FAIL — `undefined: ViewFor`

- [ ] **Step 3: Write the implementation**

```go
// go-bot/internal/poker/view.go
package poker

type SeatView struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Stack  int      `json:"stack"`
	Bet    int      `json:"bet"`
	Folded bool     `json:"folded"`
	AllIn  bool     `json:"all_in"`
	Hole   []string `json:"hole,omitempty"` // populated only for the viewer
	ToAct  bool     `json:"to_act"`
}

type TableView struct {
	ID       string     `json:"id"`
	Seq      uint64     `json:"seq"`
	Stage    string     `json:"stage"`
	Board    []string   `json:"board"`
	Pot      int        `json:"pot"`
	Seats    []SeatView `json:"seats"`
	YouSeat  int        `json:"you_seat"`
	Deadline int64      `json:"deadline"`
}

var stageNames = map[Stage]string{
	StageWaiting: "waiting", StagePreflop: "preflop", StageFlop: "flop",
	StageTurn: "turn", StageRiver: "river", StageShowdown: "showdown",
}

func strs(cs []pk.Card) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.String())
	}
	return out
}

// ViewFor builds the state visible to one player. Other players' hole cards
// are never included — this is the trust boundary, enforced server-side.
func (t *Table) ViewFor(userID string) TableView {
	pot := 0
	for _, s := range t.Seats {
		pot += s.Committed
	}
	v := TableView{
		ID: t.ID, Seq: t.Seq, Stage: stageNames[t.Stage],
		Board: strs(t.Board), Pot: pot, YouSeat: -1,
		Deadline: t.Deadline.Unix(),
	}
	for i, s := range t.Seats {
		sv := SeatView{
			UserID: s.UserID, Name: s.Name, Stack: s.Stack, Bet: s.Bet,
			Folded: s.Folded, AllIn: s.AllIn, ToAct: i == t.ToAct,
		}
		if s.UserID == userID {
			sv.Hole = strs(s.Hole)
			v.YouSeat = i
		} else if t.Stage == StageShowdown && !s.Folded && s.InHand {
			sv.Hole = strs(s.Hole) // cards are public at showdown
		}
		v.Seats = append(v.Seats, sv)
	}
	return v
}
```

Add `pk "github.com/chehsunliu/poker"` to `view.go`'s imports.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/poker/
git commit -m "feat(poker): per-viewer redaction"
```

---

### Task 7: Telegram initData verification

**Files:**
- Create: `go-bot/internal/handlers/pokerauth.go`
- Test: `go-bot/internal/handlers/pokerauth_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `func verifyInitData(initData, botToken string, maxAge time.Duration) (userID int64, firstName, username string, err error)`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/handlers/pokerauth_test.go
package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

// signInitData builds a validly signed initData payload for testing.
func signInitData(token string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+fields[k])
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(parts, "\n")))

	q := url.Values{}
	for k, v := range fields {
		q.Set(k, v)
	}
	q.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return q.Encode()
}

func validFields() map[string]string {
	return map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      `{"id":460670583,"first_name":"Danya","username":"Dany_ro"}`,
	}
}

func TestVerifyInitDataAcceptsValidSignature(t *testing.T) {
	data := signInitData("test-token", validFields())
	id, firstName, username, err := verifyInitData(data, "test-token", 24*time.Hour)
	if err != nil {
		t.Fatalf("expected valid signature, got %v", err)
	}
	if id != 460670583 {
		t.Errorf("user id = %d, want 460670583", id)
	}
	if firstName != "Danya" {
		t.Errorf("first name = %q, want Danya", firstName)
	}
	if username != "Dany_ro" {
		t.Errorf("username = %q, want Dany_ro", username)
	}
}

func TestVerifyInitDataRejectsTamperedPayload(t *testing.T) {
	data := signInitData("test-token", validFields())
	tampered := strings.Replace(data, "460670583", "111111111", 1)
	if _, _, _, err := verifyInitData(tampered, "test-token", 24*time.Hour); err == nil {
		t.Fatal("expected tampered payload to be rejected")
	}
}

func TestVerifyInitDataRejectsWrongToken(t *testing.T) {
	data := signInitData("test-token", validFields())
	if _, _, _, err := verifyInitData(data, "different-token", 24*time.Hour); err == nil {
		t.Fatal("expected signature from another token to be rejected")
	}
}

func TestVerifyInitDataRejectsStaleAuthDate(t *testing.T) {
	f := validFields()
	f["auth_date"] = fmt.Sprintf("%d", time.Now().Add(-48*time.Hour).Unix())
	data := signInitData("test-token", f)
	if _, _, _, err := verifyInitData(data, "test-token", 24*time.Hour); err == nil {
		t.Fatal("expected stale auth_date to be rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestVerifyInitData -v`
Expected: FAIL — `undefined: verifyInitData`

- [ ] **Step 3: Write the implementation**

```go
// go-bot/internal/handlers/pokerauth.go
package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errBadSignature = errors.New("invalid initData signature")
	errStaleAuth    = errors.New("initData is too old")
)

// verifyInitData validates a Telegram Mini App initData payload and returns
// the authenticated Telegram user id. Per Telegram's spec:
//
//	secret_key = HMAC_SHA256(key="WebAppData", data=bot_token)
//	hash       = HMAC_SHA256(key=secret_key,  data=data_check_string)
//
// where data_check_string is "key=value" pairs sorted by key, joined by "\n",
// excluding the hash field itself.
func verifyInitData(initData, botToken string, maxAge time.Duration) (int64, string, string, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, "", "", err
	}
	given := values.Get("hash")
	if given == "" {
		return 0, "", "", errBadSignature
	}
	values.Del("hash")

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(parts, "\n")))
	if !hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(given)) {
		return 0, "", "", errBadSignature
	}

	ts, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return 0, "", "", errStaleAuth
	}
	if time.Since(time.Unix(ts, 0)) > maxAge {
		return 0, "", "", errStaleAuth
	}

	var user struct {
		ID        int64  `json:"id"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return 0, "", "", err
	}
	return user.ID, user.FirstName, user.Username, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -run TestVerifyInitData -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add go-bot/internal/handlers/pokerauth.go go-bot/internal/handlers/pokerauth_test.go
git commit -m "feat(poker): verify Telegram initData signatures"
```

---

### Task 8: Table registry and HTTP layer

**Files:**
- Create: `go-bot/internal/handlers/pokerweb.go`
- Test: `go-bot/internal/handlers/pokerweb_test.go`

**Interfaces:**
- Consumes: `poker.NewTable`, `poker.Table`, `verifyInitData`, `storage.DB`
- Produces: `type PokerHub struct{...}`, `func NewPokerHub(db *storage.DB, bot *tele.Bot, token string) *PokerHub`, `func (h *PokerHub) Register(mux *http.ServeMux)`, `func (h *PokerHub) Create(chatID int64) *poker.Table`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/handlers/pokerweb_test.go
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJoinRejectsMissingInitData(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(999)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing initData", rec.Code)
	}
}

func TestUnknownTableReturns404(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/poker/nope/join", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown table", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/handlers/ -run TestJoinRejects -v`
Expected: FAIL — `undefined: NewPokerHub`

- [ ] **Step 3: Write the implementation**

```go
// go-bot/internal/handlers/pokerweb.go
package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

type subscriber struct {
	userID string
	ch     chan poker.TableView
}

type PokerHub struct {
	db     *storage.DB
	bot    *tele.Bot
	token  string
	mu     sync.Mutex
	tables map[string]*poker.Table
	subs   map[string][]*subscriber
}

func NewPokerHub(db *storage.DB, bot *tele.Bot, token string) *PokerHub {
	return &PokerHub{
		db: db, bot: bot, token: token,
		tables: map[string]*poker.Table{},
		subs:   map[string][]*subscriber{},
	}
}

func (h *PokerHub) Create(chatID int64) *poker.Table {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	tbl := poker.NewTable(id, chatID)
	h.mu.Lock()
	h.tables[id] = tbl
	h.mu.Unlock()
	return tbl
}

func (h *PokerHub) table(id string) *poker.Table {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tables[id]
}

// tableIDFrom extracts the table id from /api/poker/{id}/{action}.
func tableIDFrom(path string) (id, action string) {
	rest := strings.TrimPrefix(path, "/api/poker/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// auth verifies initData and chat membership, returning the Telegram user id.
func (h *PokerHub) auth(r *http.Request, tbl *poker.Table) (int64, string, string, int) {
	initData := r.Header.Get("X-Telegram-Init-Data")
	if initData == "" {
		initData = r.URL.Query().Get("init_data")
	}
	if initData == "" {
		return 0, "", "", http.StatusUnauthorized
	}
	uid, firstName, username, err := verifyInitData(initData, h.token, 24*time.Hour)
	if err != nil {
		return 0, "", "", http.StatusUnauthorized
	}
	if h.bot != nil {
		m, err := h.bot.ChatMemberOf(&tele.Chat{ID: tbl.ChatID}, &tele.User{ID: uid})
		if err != nil {
			return 0, "", "", http.StatusForbidden
		}
		switch m.Role {
		case tele.Creator, tele.Administrator, tele.Member, tele.Restricted:
		default:
			return 0, "", "", http.StatusForbidden
		}
	}
	return uid, firstName, username, 0
}

func (h *PokerHub) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/poker/", func(w http.ResponseWriter, r *http.Request) {
		id, action := tableIDFrom(r.URL.Path)
		tbl := h.table(id)
		if tbl == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		uid, firstName, username, code := h.auth(r, tbl)
		if code != 0 {
			msg := "Відкрий через кнопку в чаті"
			if code == http.StatusForbidden {
				msg = "Ти не з цього чату"
			}
			http.Error(w, msg, code)
			return
		}
		switch action {
		case "join":
			h.handleJoin(w, tbl, uid, firstName, username)
		case "stream":
			h.handleStream(w, r, tbl, uid)
		case "action":
			h.handleAction(w, r, tbl, uid)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/poker/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/poker/")
		if h.table(id) == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pokerTmpl.Execute(w, map[string]string{"TableID": id})
	})
}

func (h *PokerHub) handleJoin(w http.ResponseWriter, tbl *poker.Table, uid int64, firstName, username string) {
	userID := fmt.Sprintf("%d", uid)
	name := resolveTarget(firstName, username)
	balance := 0
	if h.db != nil {
		balance = h.db.GetBalance(userID, name)
	}
	buyIn := balance
	if buyIn > poker.MaxBuyIn {
		buyIn = poker.MaxBuyIn
	}
	if err := tbl.Sit(userID, name, buyIn); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	h.broadcast(tbl)
	writeJSON(w, tbl.ViewFor(userID))
}

func (h *PokerHub) handleAction(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	var body struct {
		Action string `json:"action"`
		Amount int    `json:"amount"`
		Seq    uint64 `json:"seq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	userID := fmt.Sprintf("%d", uid)
	if body.Seq != tbl.Seq {
		http.Error(w, "stale", http.StatusConflict)
		return
	}
	if err := tbl.Act(userID, poker.Action(body.Action), body.Amount); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	h.settleIfDone(tbl)
	h.broadcast(tbl)
	writeJSON(w, tbl.ViewFor(userID))
}

// settleIfDone writes balance deltas once the hand reaches showdown.
func (h *PokerHub) settleIfDone(tbl *poker.Table) {
	if tbl.Stage != poker.StageShowdown {
		return
	}
	deltas := tbl.Showdown()
	if h.db == nil {
		return
	}
	for _, s := range tbl.Seats {
		d, ok := deltas[s.UserID]
		if !ok || d == 0 {
			continue
		}
		// Empty name preserves the player's existing display name.
		h.db.UpdateBalance(s.UserID, "", d)
		h.db.LogTransaction(s.UserID, s.Name, "poker", d)
	}
}

func (h *PokerHub) handleStream(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	userID := fmt.Sprintf("%d", uid)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := &subscriber{userID: userID, ch: make(chan poker.TableView, 4)}
	h.mu.Lock()
	h.subs[tbl.ID] = append(h.subs[tbl.ID], sub)
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		list := h.subs[tbl.ID]
		for i, s := range list {
			if s == sub {
				h.subs[tbl.ID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
	}()

	sendView(w, flusher, tbl.ViewFor(userID))
	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-sub.ch:
			sendView(w, flusher, v)
		}
	}
}

func sendView(w http.ResponseWriter, f http.Flusher, v poker.TableView) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", raw)
	f.Flush()
}

// broadcast pushes a fresh, individually redacted snapshot to every viewer.
func (h *PokerHub) broadcast(tbl *poker.Table) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.subs[tbl.ID] {
		select {
		case s.ch <- tbl.ViewFor(s.userID):
		default: // slow consumer: drop, the next snapshot repairs it
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Export `Stage` and `Seq` access used above**

`poker.Table` already exposes `Stage`, `Seq`, and `Seats` as public fields, and `poker.StageShowdown` is exported. No change needed — verify by compiling.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/handlers/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/pokerweb.go go-bot/internal/handlers/pokerweb_test.go
git commit -m "feat(poker): HTTP layer with SSE and membership auth"
```

---

### Task 9: The `/poker` chat command and wiring

**Files:**
- Create: `go-bot/internal/handlers/poker.go`
- Modify: `go-bot/internal/handlers/handlers.go` (add to `Register`, add hub field)
- Modify: `go-bot/cmd/bot/main.go:215-219` (construct hub, register routes)

**Interfaces:**
- Consumes: `PokerHub`
- Produces: `func (b *Bot) handlePoker(c tele.Context) error`, `func (b *Bot) SetPokerHub(h *PokerHub)`

- [ ] **Step 1: Write the command handler**

```go
// go-bot/internal/handlers/poker.go
package handlers

import (
	"fmt"
	"os"

	tele "gopkg.in/telebot.v3"
)

func publicBaseURL() string {
	if v := os.Getenv("PUBLIC_BASE_URL"); v != "" {
		return v
	}
	return "https://fuck-work-bot.fly.dev"
}

func (b *Bot) handlePoker(c tele.Context) error {
	if b.poker == nil {
		return c.Send("♠️ Покер зараз недоступний")
	}
	tbl := b.poker.Create(c.Chat().ID)
	url := fmt.Sprintf("%s/poker/%s", publicBaseURL(), tbl.ID)

	markup := &tele.ReplyMarkup{}
	btn := markup.WebApp("♠️ Сісти за стіл", &tele.WebApp{URL: url})
	markup.Inline(markup.Row(btn))

	return c.Send(
		"♠️ *Покер!*\n\nМакс 6 гравців, бай-ін до 10000 🪙.\nНатисни кнопку, щоб сісти за стіл.",
		markup, tele.ModeMarkdown,
	)
}
```

- [ ] **Step 2: Add the hub to the Bot struct**

In `internal/handlers/handlers.go`, change the struct at line 17 and add a setter:

```go
type Bot struct {
	clf   *classifier.Classifier
	db    *storage.DB
	poker *PokerHub
}

// SetPokerHub wires the poker hub after construction, since the hub needs the
// *tele.Bot which is created after the handlers.
func (b *Bot) SetPokerHub(h *PokerHub) { b.poker = h }
```

- [ ] **Step 3: Register the command**

In `Register` (handlers.go:39), alongside the other muted-checked commands:

```go
	bot.Handle("/poker", b.muteCheck(b.handlePoker))
```

- [ ] **Step 4: Wire it in main.go**

Replace lines 215-216 of `cmd/bot/main.go`:

```go
	mux := http.NewServeMux()
	handlers.RegisterWeb(mux, db)

	pokerHub := handlers.NewPokerHub(db, bot, os.Getenv("TELEGRAM_BOT_TOKEN"))
	pokerHub.Register(mux)
	botHandlers.SetPokerHub(pokerHub)
```

Adjust `botHandlers` to whatever the local variable holding `*handlers.Bot` is called in `main.go` — check the call to `handlers.New(...)` and reuse that name.

- [ ] **Step 5: Build and test**

Run: `cd go-bot && CGO_ENABLED=0 go build -o /tmp/bot ./cmd/bot/ && go test ./...`
Expected: build succeeds, all tests pass

- [ ] **Step 6: Commit**

```bash
git add go-bot/internal/handlers/poker.go go-bot/internal/handlers/handlers.go go-bot/cmd/bot/main.go
git commit -m "feat(poker): /poker command and wiring"
```

---

### Task 10: Mini App frontend (oval table)

**Files:**
- Modify: `go-bot/internal/handlers/pokerweb.go` (add `pokerTmpl`)

**Interfaces:**
- Consumes: `TableView` JSON from Task 6, the SSE and action endpoints from Task 8
- Produces: `var pokerTmpl *template.Template`

Per the spec, the oval layout needs three mitigations for phone width: hard cap of 6 seats (already enforced in the engine), names clipped to 10 characters, and an explicit turn indicator with a countdown rather than only a glow.

- [ ] **Step 1: Add the template**

Append to `pokerweb.go`, and add `"html/template"` to its imports:

```go
var pokerTmpl = template.Must(template.New("poker").Parse(`<!doctype html>
<html lang="uk"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<title>Покер</title>
<script src="https://telegram.org/js/telegram-web-app.js"></script>
<style>
:root{color-scheme:dark}
body{margin:0;background:#0f1420;color:#e6edf7;font:14px -apple-system,"Segoe UI",sans-serif}
#felt{position:relative;height:58vh;background:radial-gradient(ellipse at 50% 45%,#1e7350,#124b35 70%,#0d3626)}
.seat{position:absolute;width:80px;text-align:center;font-size:11px;transition:opacity .2s}
.seat .nm{font-weight:700;color:#fff;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.seat .st{color:#7ddba5}
.seat.folded{opacity:.35}
.seat.act{outline:2px solid #ffd166;border-radius:8px;padding:2px}
.seat.act .nm{color:#ffd166}
.chip{display:inline-block;background:#e8a33d;color:#3a2708;border-radius:8px;padding:0 5px;font-size:10px;font-weight:700}
#centre{position:absolute;top:44%;left:0;right:0;text-align:center}
.card{display:inline-block;background:#fff;border-radius:4px;width:26px;height:36px;line-height:36px;
 text-align:center;font-size:15px;font-weight:700;margin:0 2px;color:#111}
.card.red{color:#d62828}
#pot{color:#ffd166;font-weight:700;margin-top:8px}
#mine{display:flex;justify-content:space-between;align-items:center;padding:10px 14px;background:#0d1220}
#acts{display:flex;gap:6px;padding:10px}
button{flex:1;padding:12px 0;border:0;border-radius:8px;font-weight:700;font-size:13px;
 background:#243147;color:#c9d5e8}
button.pri{background:#e8a33d;color:#2b1d05}
button.dng{background:#3a2029;color:#e08a9a}
button:disabled{opacity:.35}
#msg{text-align:center;padding:8px;color:#9fb0c9;font-size:12px;min-height:18px}
</style></head><body>
<div id="felt"><div id="centre"><div id="board"></div><div id="pot"></div></div></div>
<div id="mine"><span id="me"></span><span id="hole"></span></div>
<div id="acts">
  <button class="dng" data-a="fold">Пас</button>
  <button data-a="check">Чек</button>
  <button data-a="call">Колл</button>
  <button class="pri" data-a="raise">Рейз</button>
</div>
<div id="msg"></div>
<script>
const TABLE={{.TableID}};
const tg=window.Telegram?.WebApp; tg?.ready(); tg?.expand();
const INIT=tg?.initData||"";
let state=null;

function card(s){
  const red=s.includes("h")||s.includes("d");
  return '<span class="card'+(red?' red':'')+'">'+s.replace("T","10")+'</span>';
}
function clip(n){return n.length>10?n.slice(0,10)+"…":n}

function render(v){
  state=v;
  document.getElementById("board").innerHTML=v.board.map(card).join("");
  document.getElementById("pot").textContent="🪙 Банк "+v.pot;
  const felt=document.getElementById("felt");
  felt.querySelectorAll(".seat").forEach(e=>e.remove());
  const n=v.seats.length, cx=50, cy=42, rx=38, ry=30;
  v.seats.forEach((s,i)=>{
    const ang=(-Math.PI/2)+(2*Math.PI*i/n);
    const d=document.createElement("div");
    d.className="seat"+(s.folded?" folded":"")+(s.to_act?" act":"");
    d.style.left=(cx+rx*Math.cos(ang))+"%";
    d.style.top=(cy+ry*Math.sin(ang))+"%";
    d.innerHTML='<div class="nm">'+clip(s.name)+'</div><div class="st">'+s.stack+'</div>'+
      (s.bet?'<span class="chip">'+s.bet+'</span>':'');
    felt.appendChild(d);
  });
  const me=v.seats[v.you_seat];
  document.getElementById("me").textContent=me?clip(me.name)+" · "+me.stack:"";
  document.getElementById("hole").innerHTML=me&&me.hole?me.hole.map(card).join(""):"";
  const mine=me&&me.to_act;
  document.querySelectorAll("#acts button").forEach(b=>b.disabled=!mine);
  tick();
}

function tick(){
  if(!state||!state.deadline)return;
  const left=Math.max(0,state.deadline-Math.floor(Date.now()/1000));
  const me=state.seats[state.you_seat];
  document.getElementById("msg").textContent=
    me&&me.to_act?("Твій хід · "+left+"с"):"";
}
setInterval(tick,1000);

async function act(a){
  const r=await fetch("/api/poker/"+TABLE+"/action",{
    method:"POST",
    headers:{"Content-Type":"application/json","X-Telegram-Init-Data":INIT},
    body:JSON.stringify({action:a,amount:0,seq:state?state.seq:0})
  });
  if(r.status===409){document.getElementById("msg").textContent="Хід уже пройшов";return}
  if(!r.ok){document.getElementById("msg").textContent=await r.text()}
}
document.querySelectorAll("#acts button").forEach(b=>
  b.onclick=()=>act(b.dataset.a));

(async()=>{
  const j=await fetch("/api/poker/"+TABLE+"/join",{
    method:"POST",headers:{"X-Telegram-Init-Data":INIT}});
  if(!j.ok){document.getElementById("msg").textContent=await j.text();return}
  render(await j.json());
  const es=new EventSource("/api/poker/"+TABLE+"/stream?init_data="+encodeURIComponent(INIT));
  es.onmessage=e=>render(JSON.parse(e.data));
  es.onerror=()=>{document.getElementById("msg").textContent="Зʼєднання втрачено…"};
})();
</script></body></html>`))
```

Note: `{{.TableID}}` inside a `<script>` block is escaped by `html/template` as a JS string literal automatically — this is why the template uses `html/template` rather than `text/template`.

- [ ] **Step 2: Build and test**

Run: `cd go-bot && CGO_ENABLED=0 go build -o /tmp/bot ./cmd/bot/ && go test ./...`
Expected: build succeeds, all tests pass

- [ ] **Step 3: Manual smoke test**

```bash
cd go-bot && TELEGRAM_BOT_TOKEN=xxx MODEL_PATH=./model/tfidf_model.json DATA_DIR=./testdata ./bot
```

Then `curl -i localhost:8080/poker/doesnotexist` → expect `404` with "Стіл закрито".

- [ ] **Step 4: Commit**

```bash
git add go-bot/internal/handlers/pokerweb.go
git commit -m "feat(poker): Mini App oval table frontend"
```

---

### Task 11: Turn clock enforcement and auto-start

The spec requires a 90s deadlock breaker and hands that actually begin. Task 10 only *displays* the countdown; this task enforces it.

**Files:**
- Modify: `go-bot/internal/poker/betting.go`
- Modify: `go-bot/internal/handlers/pokerweb.go`
- Test: `go-bot/internal/poker/timeout_test.go`

**Interfaces:**
- Consumes: `Table`, `Act`, `ActCheck`, `ActFold`, `TurnTimeout`
- Produces: `func (t *Table) ForceTimeout() bool`, `func (h *PokerHub) StartSweeper()`

- [ ] **Step 1: Write the failing test**

```go
// go-bot/internal/poker/timeout_test.go
package poker

import (
	"testing"
	"time"
)

func TestForceTimeoutFoldsWhenBetOutstanding(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if !actor.Folded {
		t.Error("player facing a bet should be folded on timeout")
	}
}

func TestForceTimeoutChecksWhenFree(t *testing.T) {
	tbl := headsUp(t)
	first := tbl.Seats[tbl.ToAct].UserID
	_ = tbl.Act(first, ActCall, 0) // now the big blind can check
	tbl.Deadline = time.Now().Add(-time.Second)
	actor := tbl.Seats[tbl.ToAct]
	if !tbl.ForceTimeout() {
		t.Fatal("expected timeout to fire")
	}
	if actor.Folded {
		t.Error("player with no bet to call should be checked, not folded")
	}
}

func TestForceTimeoutNoopBeforeDeadline(t *testing.T) {
	tbl := headsUp(t)
	tbl.Deadline = time.Now().Add(time.Minute)
	if tbl.ForceTimeout() {
		t.Fatal("timeout must not fire before the deadline")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go-bot && go test ./internal/poker/ -run TestForceTimeout -v`
Expected: FAIL — `undefined: ForceTimeout`

- [ ] **Step 3: Write the implementation (append to `betting.go`)**

```go
// ForceTimeout applies the deadlock-breaking auto-action when the turn clock
// has expired: check if it is free, otherwise fold. Reports whether it acted.
func (t *Table) ForceTimeout() bool {
	if t.Stage == StageWaiting || t.Stage == StageShowdown {
		return false
	}
	if t.ToAct < 0 || time.Now().Before(t.Deadline) {
		return false
	}
	s := t.Seats[t.ToAct]
	a := ActFold
	if s.Bet >= t.highBet() {
		a = ActCheck
	}
	return t.Act(s.UserID, a, 0) == nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd go-bot && go test ./internal/poker/ -v`
Expected: PASS

- [ ] **Step 5: Add the sweeper and auto-start to the hub**

In `pokerweb.go`, append:

```go
// StartSweeper enforces turn deadlines across all tables. Without it, a player
// who closes Telegram stalls the table indefinitely with chips committed.
func (h *PokerHub) StartSweeper() {
	go func() {
		for range time.Tick(5 * time.Second) {
			h.mu.Lock()
			tables := make([]*poker.Table, 0, len(h.tables))
			for _, t := range h.tables {
				tables = append(tables, t)
			}
			h.mu.Unlock()
			for _, tbl := range tables {
				if tbl.ForceTimeout() {
					h.settleIfDone(tbl)
					h.broadcast(tbl)
				}
			}
		}
	}()
}
```

And at the end of `handleJoin`, before the response is written, auto-start once a second player sits:

```go
	if tbl.Stage == poker.StageWaiting && tbl.SeatedCount() >= 2 {
		_ = tbl.StartHand()
	}
```

- [ ] **Step 6: Export the seat count**

`seatedCount` in `table.go` is unexported but is now needed by the handlers package. Rename it to `SeatedCount` and update its two call sites in `table.go` (`StartHand` and any test).

- [ ] **Step 7: Start the sweeper in main.go**

Immediately after `pokerHub.Register(mux)` from Task 9:

```go
	pokerHub.StartSweeper()
```

- [ ] **Step 8: Build and test**

Run: `cd go-bot && CGO_ENABLED=0 go build -o /tmp/bot ./cmd/bot/ && go test ./...`
Expected: build succeeds, all tests pass

- [ ] **Step 9: Commit**

```bash
git add go-bot/internal/poker/ go-bot/internal/handlers/pokerweb.go go-bot/cmd/bot/main.go
git commit -m "feat(poker): turn clock enforcement and auto-start"
```

---

## Self-Review Notes

**Spec coverage:** trust boundary → Task 6; auth (initData + membership) → Tasks 7-8; game model → Tasks 2-5; side pots → Task 2; settlement/zero-sum → Task 5 + Task 8 `settleIfDone`; HTTP protocol incl. seq/409 → Task 8; oval UI + 3 mitigations → Task 10; error codes → Task 8; testing strategy → Tasks 1-7.

**Fixed during self-review:**
- `verifyInitData` originally returned only the username, but `resolveTarget(name, username)` takes the display name first — it now also returns `first_name`, so players who are not in `usernameToName` get a real name instead of a raw handle.
- Task 4 referenced `Seat.actedThisStreet` in Step 3 but only added the field in Step 4, so the package would not have compiled mid-task. The field now lands first.
- The 90s turn clock and hand auto-start were initially left unimplemented. Both are now Task 11 — without the sweeper, one player closing Telegram stalls the table with real chips committed.

**Remaining known limitation:** a player cannot leave the table or top up mid-session; they stay seated until the process restarts. Not in the spec, not implemented, worth a follow-up if the game gets used.
