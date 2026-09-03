# Bot Opponents for the Poker Mini App

**Date:** 2026-09-03
**Status:** Design approved, ready for implementation planning
**Builds on:** `docs/superpowers/specs/2026-09-02-poker-mini-app-design.md`

## Context

The poker Mini App shipped on 2026-09-03. It needs at least two seated humans before a hand can start, so a single player who opens `/poker` sits alone and nothing happens. Bots make solo play possible and pad out thin tables.

## Decisions

| Decision | Choice | Consequence |
|---|---|---|
| Bot skill | **Decent — roughly break-even** | Bank hovers near zero; currency stays stable |
| Seating | **`bots = min(2, 6 − humans)`**, humans priority | Max 2 bots; table caps at the existing 6 |
| Rebuy | **Busted bot reloads to the top human stack, between hands** | Solo play always works; farming is uncapped |
| Funding | **A reserved bank account** | Zero-sum extends to humans + bank |

## Non-goals

- A daily cap on bank losses (considered and declined; add later if the bank trends badly)
- Bots on tables with no humans
- Difficulty settings, bot personalities, or per-bot tuning
- Bots visible in `/top`, casino stats, or achievements
- Any change to the authentication or authorization surface

## The money model

**Bot settlement deltas route to a reserved bank account** rather than creating a `balances` row per bot:

```
human +2000, bot −2000  →  human +2000, bank −2000
human −2000, bot +2000  →  human −2000, bank +2000
```

Humans + bank always sum to zero, so the engine's zero-sum invariant is **unchanged**, and every existing guard keeps working: the single settlement transaction, the empty-name rule that preserves display names, and the zero-delta skip.

**Rebuys move no money.** Setting a bot's stack does not touch the database; chips become real only at settlement. The bank balance therefore reflects only realised wins and losses.

Properties that follow:

- **The bank may go negative.** That is correct for a house account. It is excluded from `/top` and casino stats by its user-id prefix so it never appears as a player at −40000.
- **Farming is uncapped.** A hot streak has no ceiling: bust a bot, it reloads, bust it again. Break-even bots should wash out over months, but variance will not. The bank balance is the metric to watch.
- **Bots never touch the auth path.** They are seated internally by the hub, never through `handleJoin`, so the authorization surface is exactly as it is today.
- **Bots take no `seatedAt` claim.** Those exist to stop one bankroll backing several tables; a bot has no bankroll.

## Bot identity

- User id: `bot:1`, `bot:2` — the `bot:` prefix is the single discriminator used for settlement routing, leaderboard exclusion, and stats exclusion.
- Bank user id: `bank:house`, same exclusion mechanism.
- Display names are Ukrainian, consistent with the rest of the bot's text.

## Decision function

A pure function in `internal/poker`, with no HTTP or locking knowledge, mirroring how the engine already separates rules from transport.

**Preflop** — `Best()` requires five cards, so it cannot evaluate a two-card hand. A tier heuristic is used instead: pocket pairs, suited cards, connectors, high cards.

**Postflop** — `Best(hole, board)` yields a rank, converted to a rough strength percentile.

**Choosing** — strength is compared against pot odds (`toCall / (pot + toCall)`), with fold/call/raise thresholds and a bluff frequency of roughly 10%. The function always returns a legal action; the engine validates every action regardless, so a bot cannot violate betting rules any more than a crafted HTTP request can.

## Driving

The sweeper already ticks every ~5 seconds holding the table lock. When it is a bot's turn, the bot acts on the next tick.

This is deliberate: the 0–5 second pause reads as deliberation, and it introduces **no new goroutines and no new locking**, reusing machinery that has already been through concurrency review. The lock ordering rule is untouched — table lock outer, hub mutex inner.

## Seating

A hub method runs between hands, after settlement and before the next hand auto-starts:

1. Count seated humans.
2. Target bots = `min(2, 6 − humans)`.
3. Add or remove bots to match the target.
4. Rebuy any bot whose stack is 0 to the current highest human stack.

Bots are added and removed between hands only, never mid-hand — the same rule that already governs human seating.

## Testing

- **Decision tests** under a seeded RNG: folds trash, calls or raises strong holdings, never bets more than its stack, never acts out of turn.
- **Seating tests**: the `min(2, 6 − humans)` table is honoured as humans join and leave; rebuy fires only on a busted bot and only between hands.
- **Zero-sum extended**: over a played hand containing bots, human deltas plus the bank delta sum to exactly zero.
- **Self-play simulation**: bots against bots over several thousand hands, asserting the bank drifts within a band. "Break-even" is a tuning goal rather than something thresholds can prove, so this converts the claim into something the suite checks. The band should be wide enough not to be flaky and narrow enough to catch a bot that is systematically losing or printing money.

## Risks

- **"Break-even" is unproven until the simulation runs.** If the drift band fails, the thresholds need tuning; that is expected iteration, not a design failure.
- **Uncapped farming** is accepted by decision. Mitigation if needed is the daily cap that was declined.
- **Bank exposure scales with the top human stack.** Two bots matching a 10000 stack put 20000 of bank money on the table.
