package handlers

import "time"

// Супер гра is a double-or-nothing on a poker win. See
// docs/superpowers/specs/2026-09-08-super-game-design.md for the maths
// behind every constant here.
const (
	// superChance is how often a qualifying win is offered a game, and
	// superCooldown is the minimum gap between two games for one player.
	// Same division of labour as tauntChance/tauntCooldown in pokerbots.go:
	// the probability is for variety, the cooldown is what stops a burst.
	superChance   = 0.15
	superCooldown = 10 * time.Minute
	// superMinBlinds keeps this an event. Without a floor it would fire on
	// pots of two blinds and stop meaning anything. Relative to the blind
	// rather than absolute because the blinds double on a schedule.
	superMinBlinds = 10

	// The table is held for the sum of these two, so they are the worst-case
	// pause everyone else sits through.
	superDecideWindow = 10 * time.Second
	superResultHold   = 4 * time.Second
)

// diceOutcome scores 2d6. Over 36 rolls this is 10 doubles, 5 keeps and 21
// halves, which is 35.5/36 of the stake returned -- a 1.4% edge to the bank.
func diceOutcome(a, b int) string {
	switch sum := a + b; {
	case sum >= 9:
		return "double"
	case sum == 8:
		return "keep"
	default:
		return "half"
	}
}

// colorOutcome scores a single card draw. A 26/26 deck makes this exactly
// fair, which is why it has no "keep" branch to soften it.
func colorOutcome(cardIsRed bool, pick string) string {
	if (pick == "red") == cardIsRed {
		return "double"
	}
	return "bust"
}

// superDelta is the signed balance change for an outcome. The half branch
// truncates, so the player keeps stake/2 and an odd stake loses the extra
// coin to the bank.
func superDelta(stake int, outcome string) int {
	switch outcome {
	case "double":
		return stake
	case "keep":
		return 0
	case "half":
		return stake/2 - stake
	case "bust":
		return -stake
	default:
		// An unrecognised outcome must never move money. Failing dangerous
		// here (e.g. treating it like "bust") would silently take the
		// player's entire win on a typo or a future outcome we forgot to
		// handle, so the safe default is a no-op.
		return 0
	}
}
