# Супер гра — Double-or-Nothing After a Poker Win

**Date:** 2026-09-08
**Status:** Design approved, ready for implementation planning
**Builds on:** `docs/superpowers/specs/2026-09-02-poker-mini-app-design.md`, `docs/superpowers/specs/2026-09-03-poker-bots-design.md`

## Context

Winning a poker hand currently ends in a number changing on a plaque. Супер гра turns some of those wins into a shared moment: the table stops, the winner picks one of two gambles for their winnings, and everyone at the table watches the result land.

This is the first feature in the Mini App that moves real богдудіки outside the poker settlement path, so most of this document is about not breaking the two things that are already documented as separately reviewed for money safety: `SettlePoker`'s zero-sum invariant and the table's stage machine.

## Decisions

| Decision | Choice | Consequence |
|---|---|---|
| Who can play | **Humans only** | `bot:` seats never trigger it; bots never stall the table |
| Flow | **Blocking — the table waits and everyone watches** | Needs a hard timeout and a Пас button, or one AFK player freezes the game |
| Decision window | **10 s**, then **4 s** to show the result | Worst-case table pause ~14 s |
| Frequency | **15% of qualifying wins**, per-player cooldown **10 min** | Roughly one Супер гра per player per 15–30 min of play |
| Minimum stake | **win ≥ 10 × big blind** | Keeps it an event rather than a coin-flip on pocket change |
| Split pots | **No Супер гра at all** | One winner or nothing — no tie-breaking rule to get wrong |
| RNG | **Server-side only** | Client RNG is editable from the console, and this is real currency |
| Money path | **Separate transaction against `bank:house`** | `SettlePoker`'s player-vs-player zero-sum stays untouched |
| Pending game at restart | **Dropped, treated as skipped** | A deploy mid-game cannot half-apply money |

## The two games

The winner picks one. Stake is `won` — their net result for the hand just finished.

### Кубики (2d6)

| Roll | Combinations | Chance | Payout |
|---|---|---|---|
| ≥ 9 | 10/36 | 27.8% | ×2 |
| = 8 | 5/36 | 13.9% | ×1 (keep) |
| ≤ 7 | 21/36 | 58.3% | ×½ |

**EV = (20 + 5 + 10.5) / 36 = 35.5/36 = 0.986** — a 1.4% edge to the bank.

### Червоне / чорне

One card off a fresh deck; the player calls the colour. Hit pays ×2, miss pays nothing. A standard deck is 26/26, so:

**EV = 0.5 × 2 = 1.000** — exactly fair.

### Known asymmetry

Червоне/чорне is EV-optimal, so a player optimising for expected value should always pick it. Кубики survives as the **lower-variance** option: it keeps half the stake 58% of the time instead of zeroing it 50% of the time. The choice is temperament, not arithmetic.

If the two should be exactly equal instead, the fix is one number: the Кубики low branch pays **11/21 ≈ 52%** rather than 50%, which makes its EV 1.000. Deliberately not done, because the user chose the ½ variant.

## Flow

1. Hand reaches showdown and settles normally through `SettlePoker`. Nothing about this step changes.
2. The hub looks for **exactly one** human winner. If the pot was split — more than one seat with `won > 0` — no Супер гра is offered at all. Otherwise, if `won ≥ 10 × big_blind`, the player is off cooldown, and a 15% roll passes, a Супер гра is opened on the table.
3. The table view now carries a `super` block. Every client renders it: the winner sees three buttons (Кубики / Червоне-чорне / Пас), everyone else sees "«Ім'я» грає Супер гру" and the same countdown.
4. The winner picks within **10 s**. Pressing Пас resolves immediately so nobody waits.
5. The server rolls, applies the money, and publishes the result. Clients play the process animation, then the outcome animation, for **4 s**.
6. The hold releases and the next hand starts as usual.

If the 10 s elapse with no choice, the game resolves as `skipped`: no money moves and the player keeps their winnings.

## How it holds the table

The next hand starts when `showdownReady(id)` reports that a full `sweepInterval` (5 s) has passed since settlement — `pokerweb.go:2312`. Супер гра adds one condition to that function: while a game on this table is unresolved and its deadline has not passed, `showdownReady` returns false.

This is deliberately a **hold on the existing showdown**, not a new `Stage`. The stage machine in `internal/poker` is the money-critical part; a hold in the handler layer cannot corrupt a hand.

## State and protocol

State lives on `PokerHub` keyed by table id, not in `internal/poker` — it is presentation and economy, not game rules.

```go
type superGame struct {
    UserID   string
    Name     string
    Stake    int
    Deadline time.Time
    State    string // "offered" | "resolved" | "skipped"
    Game     string // "dice" | "color"
    Pick     string // "red" | "black", colour game only
    Dice     [2]int
    Card     string // e.g. "K♦"
    Outcome  string // "double" | "keep" | "half" | "bust"
    Delta    int    // signed balance change actually applied
}
```

The wire format gains an optional `super` object carrying the same fields. It is shared, not per-client: everyone must see one result.

It hangs off `tableEnvelope` (`pokerchat.go:46`), which already embeds `poker.TableView` and adds its own `chat` field — not off `TableView` itself. Супер гра is economy and presentation, so `internal/poker` stays untouched, same reasoning as keeping the hold out of the stage machine.

New endpoint: `POST /api/poker/{id}/super` with `{"game":"dice"}`, `{"game":"color","pick":"red"}`, or `{"game":"skip"}`. Authenticated exactly like `/act`. Rejected unless the caller is the named winner and the state is still `offered`.

**Idempotency:** the transition out of `offered` happens once, under the table lock, before any money moves. A double-tap or a retried request finds a state that is no longer `offered` and is refused. This is the single most important guard in the feature.

## Money

The payout is a two-party transfer: the player gains (or loses) `Delta`, and `bank:house` takes the exact inverse. That is the same shape `pokerweb.go:2283` already uses to fund bot chips, and the same invariant `pokerbots_test.go:194` and `pokerweb_sweep_test.go:978` already assert.

`storage.SettlePoker` already applies a list of deltas atomically in one transaction, which is exactly what is needed — but it hardcodes the audit activity as `"poker"`. It gains an `activity string` parameter; the existing caller passes `"poker"`, Супер гра passes `"supergame"`, so the two show up separately in `/stats`.

Money moves **once**, after the roll, inside that single transaction. Nothing is written on offer, and nothing is written on skip or timeout.

## Animations

Two beats inside the 4 s result window, both server-driven off the published result — the client animates a known outcome, it never decides one.

- **Process (~1.2 s).** Кубики: both dice tumble through faces before settling on the rolled values. Червоне/чорне: the card flips face-down to face-up.
- **Outcome (~2.8 s).** ×2 — gold burst and the stake counting up to its doubled value. ×1 — a quiet pulse. ×½ — the number counting down. Bust — the card or dice desaturate and the stake drops to zero.

Both respect `prefers-reduced-motion`: the result appears without the tumble or the count.

## Edge cases

| Case | Behaviour |
|---|---|
| Winner disconnects after the offer | Timeout fires, resolves as `skipped`, table advances |
| Winner busts to 0 on the same hand | Cannot happen — the offer requires `won > 0` |
| Split pot | No game is offered; the hand ends normally |
| Winner is a bot, split with a human | Still a split — more than one seat won, so no game |
| Deploy / SIGTERM mid-game | Snapshot does not persist `super`; on restore there is no pending game and no money moved |
| Table reclaimed by the idle sweeper | Pending game discarded with the table; no money moved |
| Double-tap / replayed request | Refused: state is no longer `offered` |
| Player picks a colour game with no pick | Refused as a bad request |

## Testing

- EV of both games asserted directly from the payout table, so a silent edit to a threshold or multiplier fails a test rather than the economy.
- Zero-sum: player delta and bank delta cancel exactly, in the style of `pokerbots_test.go:194`.
- Idempotency: two concurrent resolves apply money exactly once.
- Timeout: an unanswered offer releases the hold and moves no money.
- Hold: `showdownReady` stays false while a game is pending and true once it resolves.
- Bots are never offered a game.
- A split pot offers no game, including when the split is between a human and a bot.
- A win under 10 × big blind offers no game.
- The rendered page pins the button labels and the two game ids, in the style of the existing `TestBlameLinesAreKeyedToBotUserIDs`.

## Out of scope

- Duplicating the Супер гра into the Telegram chat — the Mini App is the only surface.
- Супер гра on slots, blackjack, or duels. This is a poker-table feature; "humans only" only means anything where bots exist.
- Any second chance, re-roll, or insurance.
