package handlers

import (
	"fmt"
	"strings"

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

// botNames are the display names bots use, in order.
var botNames = []string{"Вася 🤖", "Петро 🤖"}

func botName(i int) string {
	if i-1 < len(botNames) && i >= 1 {
		return botNames[i-1]
	}
	return fmt.Sprintf("Бот %d 🤖", i)
}
