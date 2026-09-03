package handlers

import (
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
