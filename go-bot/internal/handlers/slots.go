package handlers

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

var slotSymbols = []string{"🍒", "🍋", "🍊", "🥟", "🌭", "👟", "🏎️", "💎"}
var slotWeights = []int{25, 20, 18, 15, 10, 7, 4, 1}     // normal
var riggedWeights = []int{1, 1, 1, 1, 10, 20, 30, 36}     // rigged (favor high symbols)

var slotPayouts = map[string]int{
	"💎💎💎": 50,  // Jackpot
	"🏎️🏎️🏎️": 25,  // Порше
	"👟👟👟": 15,  // Узкачі
	"🌭🌭🌭": 10,  // Обухівська
	"🥟🥟🥟": 8,   // Хінкалі
	"🍊🍊🍊": 5,
	"🍋🍋🍋": 3,
	"🍒🍒🍒": 2,
}

const (
	maxSpinsPerDay = 20
	maxBet         = 100
	defaultBet     = 10
	dailyBonus     = 50
)

func weightedChoice(symbols []string, weights []int) string {
	total := 0
	for _, w := range weights {
		total += w
	}
	r := rand.Intn(total)
	for i, w := range weights {
		r -= w
		if r < 0 {
			return symbols[i]
		}
	}
	return symbols[0]
}

func isRigged(name, username string) bool {
	riggedUsers := os.Getenv("RIGGED_CASINO_USERS")
	if riggedUsers == "" || os.Getenv("RIGGED_CASINO_ENABLED") == "false" {
		return false
	}
	for _, u := range strings.Split(riggedUsers, ",") {
		u = strings.TrimSpace(u)
		if u != "" && (strings.Contains(name, u) || username == u) {
			return true
		}
	}
	return false
}

func (b *Bot) handleSlots(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	today := time.Now().Format("2006-01-02")

	// Check daily limit
	spins := b.db.GetSlotSpinsToday(userID, today)
	if spins >= maxSpinsPerDay {
		return c.Reply(fmt.Sprintf("🎰 Ліміт %d спінів на день вичерпано. Приходь завтра!", maxSpinsPerDay))
	}

	// Parse bet
	bet := defaultBet
	if c.Message().Payload != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(c.Message().Payload))
		if err != nil {
			return c.Reply("❌ Введи число: /slots 50")
		}
		bet = parsed
	}
	if bet < 1 {
		return c.Reply("❌ Мінімальна ставка: 1 🪙")
	}
	if bet > maxBet {
		return c.Reply(fmt.Sprintf("❌ Максимальна ставка: %d 🪙", maxBet))
	}

	// Check balance
	balance := b.db.GetBalance(userID, userName)
	if balance < bet {
		return c.Reply(fmt.Sprintf("💸 Недостатньо богдудіків!\nБаланс: %d 🪙\nСтавка: %d 🪙", balance, bet))
	}

	// Spin
	weights := slotWeights
	if isRigged(userName, c.Sender().Username) {
		weights = riggedWeights
	}

	r1 := weightedChoice(slotSymbols, weights)
	r2 := weightedChoice(slotSymbols, weights)
	r3 := weightedChoice(slotSymbols, weights)

	b.db.IncrementSlotSpins(userID, today)

	// Calculate winnings
	key := r1 + r2 + r3
	multiplier, isTriple := slotPayouts[key]

	// Check for two of a kind
	isTwoOfAKind := !isTriple && (r1 == r2 || r2 == r3 || r1 == r3)

	var winnings int
	if isTriple {
		winnings = bet * multiplier
	} else if isTwoOfAKind {
		winnings = bet // return bet
	}

	profit := winnings - bet
	newBalance := b.db.UpdateBalance(userID, userName, profit)

	// Build message
	display := fmt.Sprintf("╔══════════╗\n║ %s │ %s │ %s ║\n╚══════════╝", r1, r2, r3)

	var msg string
	if isTriple && multiplier >= 25 {
		msg = fmt.Sprintf("🎰 *ДЖЕКПОТ!!!* 🎰\n\n%s\n\n💎💎💎 x%d!\n\nСтавка: %d 🪙\nВиграш: +%d 🪙\nБаланс: %d 🪙",
			display, multiplier, bet, winnings, newBalance)
	} else if isTriple {
		msg = fmt.Sprintf("🎰 *ВИГРАШ!* 🎰\n\n%s\n\nx%d!\n\nСтавка: %d 🪙\nВиграш: +%d 🪙\nБаланс: %d 🪙",
			display, multiplier, bet, winnings, newBalance)
	} else if isTwoOfAKind {
		msg = fmt.Sprintf("🎰 Майже!\n\n%s\n\nСтавка повернута\nБаланс: %d 🪙", display, newBalance)
	} else {
		msg = fmt.Sprintf("🎰 Не пощастило\n\n%s\n\n-%d 🪙\nБаланс: %d 🪙", display, bet, newBalance)
	}

	remaining := maxSpinsPerDay - spins - 1
	msg += fmt.Sprintf("\n\n_Спінів залишилось: %d_", remaining)

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleBalance(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	balance := b.db.GetBalance(userID, userName)
	return c.Reply(fmt.Sprintf("💰 *%s*\n\n🪙 %d богдудіків", userName, balance), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleDaily(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	today := time.Now().Format("2006-01-02")

	key := "daily:" + userID
	lastClaim := b.db.GetMeta(key)
	if lastClaim == today {
		return c.Reply("🎁 Ти вже забрав бонус сьогодні. Приходь завтра!")
	}

	newBalance := b.db.UpdateBalance(userID, userName, dailyBonus)
	b.db.SetMeta(key, today)

	return c.Reply(fmt.Sprintf("🎁 *Щоденний бонус!*\n\n+%d 🪙\nБаланс: %d 🪙", dailyBonus, newBalance), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleTop(c tele.Context) error {
	entries := b.db.GetTopBalances(10)
	if len(entries) == 0 {
		return c.Reply("🏆 Ще немає гравців!")
	}

	var sb strings.Builder
	sb.WriteString("🏆 *ЛІДЕРБОРД* 🏆\n\n")

	medals := []string{"🥇", "🥈", "🥉"}
	for i, e := range entries {
		medal := fmt.Sprintf("%d.", i+1)
		if i < 3 {
			medal = medals[i]
		}
		sb.WriteString(fmt.Sprintf("%s %s: %d 🪙\n", medal, e.Name, e.Coins))
	}

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
