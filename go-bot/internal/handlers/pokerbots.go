package handlers

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

const (
	// bankUserID is the house account that funds bots. Bot winnings and
	// losses are netted into this one row per hand, so humans + bank still
	// sum to zero and no per-bot rows pollute activity stats.
	bankUserID = "bank:house"

	// maxBots is the ceiling on bots at one table, regardless of free seats.
	maxBots = 2
)

// isBotUser reports whether a seat belongs to a bot.
func isBotUser(userID string) bool {
	return strings.HasPrefix(userID, poker.BotUserPrefix)
}

// hasActiveHuman reports whether tbl has at least one non-bot seat with
// chips to play, i.e. someone bots could actually play against. The caller
// MUST hold the table lock.
func hasActiveHuman(tbl *poker.Table) bool {
	for _, s := range tbl.Seats {
		if !isBotUser(s.UserID) && s.Stack > 0 {
			return true
		}
	}
	return false
}

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

	// topStack is only ever raised inside the loop above, and only by a
	// human seat — so topStack == 0 covers both "no humans at all" (nobody
	// to play against; let the time-based idle sweeper reclaim the table)
	// and "every human present is busted" (no bots until someone rebuys) in
	// one guard. No input can tell these two cases apart from inside this
	// function, so there is no separate branch left to test for either one.
	if topStack == 0 {
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
		id := fmt.Sprintf("%s%d", poker.BotUserPrefix, i)
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

// actBots plays one action for the seat to act, if that seat is a bot.
// Reports whether it acted. The caller MUST hold the table lock; like
// ensureBots, this touches Seats/ToAct directly and takes no locks of its
// own.
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
		// the always-legal action so a bot can never stall the table: check
		// when nothing is owed, otherwise fold. Report whether THAT action
		// actually succeeded rather than unconditionally returning true —
		// callers (the sweeper) key their settle-and-broadcast logic off
		// this return value, so it must be structurally tied to whether a
		// bot really acted, not just assumed true because a bot was found.
		fallback := poker.ActCheck
		if high > s.Bet {
			fallback = poker.ActFold
		}
		return tbl.Act(s.UserID, fallback, 0) == nil
	}
	return true
}

// botNames are the display names bots use, in order. The client keys each
// bot's card back off its seat user_id (bot:1, bot:2), not off these
// strings, so renaming a bot never silently changes its artwork.
var botNames = []string{"Director Bo", "Data Android God"}

func botName(i int) string {
	if i-1 < len(botNames) && i >= 1 {
		return botNames[i-1]
	}
	return fmt.Sprintf("Бот %d 🤖", i)
}

// tauntChance is how often a bot says something after taking a pot, and
// tauntCooldown is the minimum gap between two taunts at the same table.
//
// The cooldown is the part that actually prevents spam. A probability alone
// still allows two taunts on consecutive hands — at 1-in-3 that happens
// roughly every ninth win, which is exactly the burst that reads as noise.
// The gate makes bursts impossible rather than merely unlikely, so the
// chance can stay high enough that they still feel present.
const (
	tauntChance   = 0.35
	tauntCooldown = 4 * time.Minute
)

// botTaunt drops a line into table chat when a bot wins a hand — a roast
// aimed at whoever lost the most that hand, or one of the group's quotes.
//
// Reuses the same roast and quote tables the chat commands draw from, so
// the bots speak in the group's own voice rather than in canned strings,
// and a roast targeted at that player is preferred over a generic one
// (GetRandomRoast falls back on its own when there is no personal line).
//
// Caller must hold the table lock and not h.mu: appendSystem takes h.mu,
// the inner lock. The message is appended before settle's own broadcast, so
// it reaches everyone with the same snapshot that shows the result.
func (h *PokerHub) botTaunt(tbl *poker.Table, deltas map[string]int) {
	if h.db == nil || rand.Float64() > tauntChance {
		return
	}
	// Hard floor between taunts at this table, checked before any work.
	h.mu.Lock()
	if last, ok := h.lastTauntAt[tbl.ID]; ok && time.Since(last) < tauntCooldown {
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	// The bot that actually won this hand, and the human it hurt most.
	var speaker string
	victim, worst := "", 0
	for _, s := range tbl.Seats {
		d := deltas[s.UserID]
		if isBotUser(s.UserID) {
			if d > 0 && speaker == "" {
				speaker = s.Name
			}
			continue
		}
		if d < worst {
			victim, worst = s.Name, d
		}
	}
	if speaker == "" {
		return // no bot won: nothing to crow about
	}

	var line string
	if victim != "" && rand.Float64() < 0.7 {
		line = h.db.GetRandomRoast(victim)
	}
	if line == "" {
		if author, text := h.db.GetRandomQuote(); text != "" {
			line = text
			if author != "" {
				line += " © " + author
			}
		}
	}
	if line == "" {
		return // empty content tables: say nothing rather than something blank
	}

	// Roast rows are templates carrying a {name} placeholder, which every
	// other caller fills in (see handlers.go and quotes.go). Without this
	// the bots posted the raw row and players saw a literal "{name}".
	line = strings.ReplaceAll(line, "{name}", victim)
	// A line still holding a placeholder had no one to name — a generic
	// roast surfaced when nobody lost anything. Say nothing rather than
	// something obviously broken.
	if strings.Contains(line, "{name}") {
		return
	}
	// Stamped only once a line is actually produced, so a hand where the
	// content tables came back empty does not silence the bots for the next
	// four minutes.
	h.mu.Lock()
	h.lastTauntAt[tbl.ID] = time.Now()
	h.mu.Unlock()

	h.appendChatFrom(tbl.ID, speaker, line)
}
