package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) handleDart(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	// Get opponent and bet
	var opponentName, opponentID string
	bet := 20

	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		opponent := c.Message().ReplyTo.Sender
		opponentID = fmt.Sprintf("%d", opponent.ID)
		opponentName = opponent.FirstName
		if opponentName == "" {
			opponentName = opponent.Username
		}
		if c.Message().Payload != "" {
			parsed, _ := strconv.Atoi(strings.TrimSpace(c.Message().Payload))
			if parsed > 0 {
				bet = parsed
			}
		}
	} else {
		args := strings.Fields(c.Message().Payload)
		if len(args) < 1 {
			return c.Reply("Формат: /dart @username <ставка> або відповідай на повідомлення")
		}
		opponentName = strings.TrimPrefix(args[0], "@")
		resolved := resolveTarget(opponentName, opponentName)
		if id, found := b.db.FindUserByName(resolved); found {
			opponentID = id
			opponentName = resolved
		} else if id, found := b.db.FindUserByName(opponentName); found {
			opponentID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", opponentName))
		}
		if len(args) > 1 {
			parsed, _ := strconv.Atoi(args[1])
			if parsed > 0 {
				bet = parsed
			}
		}
	}

	if userID == opponentID {
		return c.Reply("Не можна грати з самим собою 🤦")
	}

	if bet > 500 {
		bet = 500
	}

	// Check balances
	bal1 := b.db.GetBalance(userID, userName)
	bal2 := b.db.GetBalance(opponentID, opponentName)
	if bal1 < bet {
		return c.Reply(fmt.Sprintf("💸 У тебе %d 🪙, ставка %d", bal1, bet))
	}
	if bal2 < bet {
		return c.Reply(fmt.Sprintf("💸 У %s недостатньо монет!", opponentName))
	}

	// Both throw — random 1-100, closest to 50 wins
	throw1 := rand.Intn(100) + 1
	throw2 := rand.Intn(100) + 1

	diff1 := throw1 - 50
	if diff1 < 0 {
		diff1 = -diff1
	}
	diff2 := throw2 - 50
	if diff2 < 0 {
		diff2 = -diff2
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎯 ДАРТС (ставка: %d 🪙)\nЦіль: 50\n\n", bet))
	sb.WriteString(fmt.Sprintf("🔵 %s: %d (різниця: %d)\n", userName, throw1, diff1))
	sb.WriteString(fmt.Sprintf("🔴 %s: %d (різниця: %d)\n\n", opponentName, throw2, diff2))

	if diff1 < diff2 {
		b.db.TransferCoins(opponentID, userID, bet)
		sb.WriteString(fmt.Sprintf("🏆 %s виграє +%d 🪙!", userName, bet))
	} else if diff2 < diff1 {
		b.db.TransferCoins(userID, opponentID, bet)
		sb.WriteString(fmt.Sprintf("🏆 %s виграє +%d 🪙!", opponentName, bet))
	} else {
		sb.WriteString("🤝 Нічия! Ніхто не втрачає")
	}

	if throw1 == 50 {
		sb.WriteString(fmt.Sprintf("\n🎯 %s влучив точно в 50! Бонус +50 🪙", userName))
		b.db.UpdateBalance(userID, userName, 50)
	}
	if throw2 == 50 {
		sb.WriteString(fmt.Sprintf("\n🎯 %s влучив точно в 50! Бонус +50 🪙", opponentName))
		b.db.UpdateBalance(opponentID, opponentName, 50)
	}

	return c.Send(sb.String())
}
