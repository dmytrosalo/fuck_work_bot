package handlers

import (
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"
)

const warReward = 20

func (b *Bot) handleWar(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	var opponentName, opponentID string
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		opponent := c.Message().ReplyTo.Sender
		opponentID = fmt.Sprintf("%d", opponent.ID)
		opponentName = opponent.FirstName
		if opponentName == "" {
			opponentName = opponent.Username
		}
	} else if c.Message().Payload != "" {
		opponentName = strings.TrimPrefix(c.Message().Payload, "@")
		resolved := resolveTarget(opponentName, opponentName)
		if id, found := b.db.FindUserByName(resolved); found {
			opponentID = id
			opponentName = resolved
		} else if id, found := b.db.FindUserByName(opponentName); found {
			opponentID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", opponentName))
		}
	} else {
		return c.Reply("Формат: /war @username або відповідай на повідомлення")
	}

	if userID == opponentID {
		return c.Reply("Не можна воювати з самим собою 🤦")
	}

	// Both need 3+ cards
	u1, _ := b.db.GetCollectionStats(userID)
	u2, _ := b.db.GetCollectionStats(opponentID)
	if u1 < 3 {
		return c.Reply("У тебе менше 3 карток! /pack")
	}
	if u2 < 3 {
		return c.Reply(fmt.Sprintf("У %s менше 3 карток!", opponentName))
	}

	// Draw 3 cards each
	cards1 := b.getRandomCards(userID, 3)
	cards2 := b.getRandomCards(opponentID, 3)

	if len(cards1) < 3 || len(cards2) < 3 {
		return c.Reply("❌ Недостатньо карток для війни!")
	}

	var sb strings.Builder
	sb.WriteString("⚔️ WAR — Найкращий з 3 раундів!\n\n")

	wins1, wins2 := 0, 0

	for round := 0; round < 3; round++ {
		c1 := cards1[round]
		c2 := cards2[round]
		pwr1 := c1.ATK + c1.DEF + c1.Special
		pwr2 := c2.ATK + c2.DEF + c2.Special

		sb.WriteString(fmt.Sprintf("Раунд %d:\n", round+1))
		sb.WriteString(fmt.Sprintf("🔵 %s %s (PWR:%d)\n", c1.Emoji, c1.Name, pwr1))
		sb.WriteString(fmt.Sprintf("🔴 %s %s (PWR:%d)\n", c2.Emoji, c2.Name, pwr2))

		if pwr1 > pwr2 {
			wins1++
			sb.WriteString(fmt.Sprintf("→ %s виграє раунд!\n\n", userName))
		} else if pwr2 > pwr1 {
			wins2++
			sb.WriteString(fmt.Sprintf("→ %s виграє раунд!\n\n", opponentName))
		} else {
			sb.WriteString("→ Нічия в раунді!\n\n")
		}
	}

	sb.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━\nРахунок: %s %d — %d %s\n\n", userName, wins1, wins2, opponentName))

	if wins1 > wins2 {
		b.db.TransferCoins(opponentID, userID, warReward)
		// Steal one of opponent's losing cards
		loserCard := cards2[0]
		b.db.TransferCard(opponentID, userID, loserCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає!\n+%d 🪙 і забирає %s %s", userName, warReward, loserCard.Emoji, loserCard.Name))
	} else if wins2 > wins1 {
		b.db.TransferCoins(userID, opponentID, warReward)
		loserCard := cards1[0]
		b.db.TransferCard(userID, opponentID, loserCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає!\n+%d 🪙 і забирає %s %s", opponentName, warReward, loserCard.Emoji, loserCard.Name))
	} else {
		sb.WriteString("🤝 Нічия! Ніхто не втрачає")
	}

	return c.Send(sb.String())
}
