# Texas Hold'em Poker as a Telegram Mini App

**Date:** 2026-09-02
**Status:** Design approved, ready for implementation planning

## Context

`fuck-work-bot` is a Go Telegram bot for a Ukrainian dev friend group, running on a single 256MB Fly.io machine in Frankfurt. It already has an economy (богдудіки), a card-battle system, and several chat games (`/duel`, `/war`, `/blackjack`, `/slots`). It also already serves HTTP on `:8080` for collection pages.

The ask: real poker, played through a web UI, restricted to members of the group chat.

## Decisions

Four decisions were settled before design, and they constrain everything below:

| Decision | Choice | Consequence |
|---|---|---|
| Pacing | **Live table** — one sitting, all players present | Game state stays in memory; no game persistence needed |
| Rules depth | **Full Texas Hold'em**, including side pots | Betting engine is the bulk of the work; needs real tests |
| Stakes | **Real богдудіки, capped buy-in** | Bugs move real money; settlement must be provably correct |
| Layout | **Oval felt table** | Prettier but tighter; needs explicit legibility mitigations |

## Non-goals

Deliberately out of scope:

- Persistence of in-flight games across restarts (a deploy ends the table)
- Async / multi-hour play
- Tournaments, blind escalation, rake
- AI/bot opponents
- Spectators, hand history, replay
- More than 6 seats at a table
- Retrofitting auth onto the existing public `/collection/<id>` page

## Architecture

### Trust boundary

**The browser is a dumb renderer.** It never receives another player's hole cards, and it never decides anything. Every action is re-validated server-side against whose turn it actually is. Opening devtools in the Telegram webview reveals, at most, your own hand.

This drives the package split: the engine returns *per-viewer views*, never raw table state.

### New package: `internal/poker/`

Pure game engine. Imports neither `telebot` nor `net/http`.

- `engine.go` — table, seats, streets, betting, side pots
- `view.go` — redaction: `Table` → `TableView` for a given `user_id`
- `engine_test.go` — TDD; this is where money bugs get caught

Only external dependency: [`github.com/chehsunliu/poker`](https://github.com/chehsunliu/poker) (MIT) for hand evaluation.

### New handler files

Kept out of `web.go`, which is already ~28K and would only get worse:

- `internal/handlers/poker.go` — `/poker` command, WebApp button, chat-side messages
- `internal/handlers/pokerweb.go` — auth, SSE, action endpoint, embedded UI template

`RegisterWeb(mux, db)` already receives everything needed; wiring is a few lines.

### Dependency rationale

[`notnil/joker`](https://github.com/notnil/joker) has a `pkg/holdem` that would supply the betting engine and pots outright — tempting, since side pots are the hard part. It is **rejected**: [pkg.go.dev](https://pkg.go.dev/github.com/notnil/joker) reports no detected license (all rights reserved by default) and no tagged release, only a pseudo-version. An unlicensed, pre-v1, untagged dependency handling real balances is not a trade worth making for ~150 lines.

`chehsunliu/poker` is MIT, pure Go (important: the build is `CGO_ENABLED=0`), and covers the genuinely fiddly part — 7-card best-five selection, kickers, wheel straights.

## Authentication

This is what makes the table actually chat-only.

1. `/poker` in the group creates a table bound to that `chatID` with a random `tableID`
2. Bot posts an inline `WebApp` button → `https://fuck-work-bot.fly.dev/poker/{tableID}`
3. Telegram opens the webview and injects signed `initData`
4. Server verifies HMAC-SHA256 per Telegram's spec — `secret_key = HMAC("WebAppData", bot_token)`, checked against the sorted data-check-string — and rejects a stale `auth_date` (max age: 24h)
5. Verified `user_id` → `bot.ChatMemberOf(table.ChatID, userID)` (telebot `bot.go:1100`) → seat only if role is `creator`, `administrator`, `member`, or `restricted`

Step 4 proves *who* someone is. Step 5 proves they belong in *this chat*. Both are required: a leaked URL alone gets a stranger nothing.

The bot token doubles as the HMAC key. It is already present as `TELEGRAM_BOT_TOKEN`, so no new secret is introduced; token rotation invalidates live sessions, which is acceptable.

## Game model

State follows the existing `war.go` pattern (`map[int64]*warState` + mutex), keyed by table:

```go
type Table struct {
    ID       string
    ChatID   int64          // auth anchor
    Seats    []*Seat        // fixed order, max 6
    Button   int            // dealer position, rotates each hand
    Stage    Stage          // Waiting|Preflop|Flop|Turn|River|Showdown
    Board    []poker.Card
    Deck     []poker.Card
    ToAct    int
    MinRaise int
    Deadline time.Time
    Seq      uint64         // monotonic, bumped on every state change
    mu       sync.Mutex
}

type Seat struct {
    UserID    string
    Name      string
    Stack     int   // table chips
    Hole      []poker.Card
    Folded    bool
    AllIn     bool
    Committed int   // total committed THIS HAND — basis for side pots
}
```

One mutex per table, so concurrent tables never block each other.

### Constants

- Max seats: **6**
- Buy-in: `min(balance, 10000)` — matches the existing max-bet rule
- Minimum buy-in: **1000**; a player whose balance is below this cannot sit (prevents 5-богдудік seats that are all-in on the blind every hand)
- Blinds: small 50 / big 100
- Turn clock: **90s**, then auto-check if free, else auto-fold

### Hand lifecycle

Post blinds → deal → preflop → flop → turn → river → showdown → **settle** → rotate button → next hand.

Players join between hands only, never mid-hand.

### Side pots

At showdown, build pots from the distinct `Committed` levels:

```
levels := sorted distinct Committed values > 0
prev := 0
for each level L:
    contributors := seats with Committed >= L
    pot.Amount   := (L - prev) * len(contributors)
    pot.Eligible := contributors that have not folded
    prev = L
```

Folded players' chips still enter the pot but confer no eligibility. Each pot is awarded to the best hand among its eligible seats; ties split, with any odd chip going to the first eligible seat left of the button.

`chehsunliu/poker`'s `Evaluate` returns an integer rank following the deuces convention it ports, where **lower is stronger** (1 = royal flush). The first engine test pins this down explicitly rather than assuming it.

## Settlement and money safety

One write per player per hand, at showdown:

```go
delta := seat.Stack - stackAtHandStart   // signed
db.UpdateBalance(userID, "", delta)
db.LogTransaction(userID, name, "poker", delta)
```

Three properties, all falling out of existing code:

- **Nothing is debited at sit-down.** Losses land hand by hand, so a redeploy mid-hand moves zero coins. Players lose the hand in progress, never their buy-in.
- **`UpdateBalance` is additive** (`coins = coins + ?`, `sqlite.go:596`), so a poker settlement composes safely with the same user playing `/slots` in the same second. No read-modify-write race.
- **Passing `""` as the name preserves the display name** via the `CASE WHEN ? != ''` branch. This matters: the production DB holds both `Danya` and `Dany_ro` for user `460670583`, and settlement must not flip it.

## HTTP protocol

| Endpoint | Purpose |
|---|---|
| `GET /poker/{tableID}` | HTML shell (the Mini App) |
| `POST /api/poker/{tableID}/join` | verify `initData` → `ChatMemberOf` → seat |
| `GET /api/poker/{tableID}/stream` | SSE, per-viewer redacted state |
| `POST /api/poker/{tableID}/action` | fold / check / call / raise |

**Redaction is per-connection and server-side.** Each SSE subscriber receives a `TableView` built for that `user_id`; other seats' `Hole` fields are absent from the JSON, not sent-and-hidden in CSS.

**Full snapshots, not deltas.** Every SSE message carries the complete current view plus the monotonic `Seq`. A view is ~1KB for ≤6 players, and snapshots are self-healing: `EventSource` reconnects on its own after a dropped connection or a locked phone, and the next snapshot repairs any missed state. Delta streams would need replay buffers and gap detection for no benefit at this scale.

**Actions carry the `Seq` they were made against.** A stale `Seq` is rejected with `409` rather than applied to a situation that has moved on — which also eliminates double-taps.

Server-side validation on every action regardless: correct seat, correct turn, amount ≥ min-raise and ≤ stack.

## UI

Oval felt table. Seats ring the table, board and pot at centre, and a fixed bottom third holding your two hole cards and the action row (`Пас / Чек / Колл / Рейз`), thumb-reachable.

The chosen layout is the tightest of the three considered, so three mitigations are required rather than optional:

1. Hard cap of 6 seats
2. Display names clipped to 10 characters
3. An explicit active-turn indicator — ring plus visible countdown — not merely a glow

All user-facing text is Ukrainian, per project convention.

## Error handling

| Code | Cause | User sees |
|---|---|---|
| `401` | bad or expired `initData` | "Відкрий через кнопку в чаті" |
| `403` | not a member of the chat | "Ти не з цього чату" |
| `404` | table gone (post-redeploy) | "Стіл закрито" + link back to chat |
| `409` | not your turn / illegal action / stale seq | silent re-sync from next snapshot |

`404` is the common one, since every push to `main` wipes in-memory tables. Because settlement is per-hand, it is a cosmetic annoyance rather than a money event.

## Testing strategy

The engine is pure specifically so it can be tested hard, without a bot or a server.

**The central invariant:** for every hand, settlement deltas across all players **sum to exactly zero**. Money is never created or destroyed. This single property test catches nearly every side-pot error — the class of bug that would otherwise silently mint богдудіки.

Alongside it:

- Short-stack all-in produces a correctly sized side pot; the short stack cannot win more than they paid in
- Split pot with an odd chip awards it left of the button
- Wheel straight (A-2-3-4-5) ranks correctly; board-plays-the-hand ties split
- Fold-to-one ends the hand immediately without reaching showdown
- Min-raise and over-stack raises are rejected
- Blind posting with a stack smaller than the blind results in all-in, not a negative stack
- `initData` HMAC verification accepts a known-good vector and rejects a tampered one and a stale `auth_date`
- Redaction: a `TableView` built for player X never contains player Y's `Hole`

## Risks

- **Side-pot correctness** is the main one, mitigated by the zero-sum invariant test above.
- **SSE through Telegram's WebView / Fly's proxy** is unproven here. Mitigation: the polling fallback (`GET` the same view every ~1s) shares the entire server implementation, so switching is a template change, not a rewrite.
- **A deploy mid-game** ends the table. Accepted, and made harmless by per-hand settlement.

## Rough effort

| Piece | Est. |
|---|---|
| Engine + side pots (TDD) | ~300 lines |
| HTTP / SSE / auth | ~200 lines |
| Mini App frontend | ~300 lines |
| Chat command + wiring | ~100 lines |
