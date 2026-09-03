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
