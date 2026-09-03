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

	// No humans means nobody to play against; make no changes and let the
	// time-based idle sweeper reclaim the table.
	if humans == 0 {
		return
	}

	// All humans are busted; no bots until someone rebuys.
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
		// when nothing is owed, otherwise fold.
		fallback := poker.ActCheck
		if high > s.Bet {
			fallback = poker.ActFold
		}
		_ = tbl.Act(s.UserID, fallback, 0)
	}
	return true
}

// botNames are the display names bots use, in order.
var botNames = []string{"Вася 🤖", "Петро 🤖"}

func botName(i int) string {
	if i-1 < len(botNames) && i >= 1 {
		return botNames[i-1]
	}
	return fmt.Sprintf("Бот %d 🤖", i)
}
