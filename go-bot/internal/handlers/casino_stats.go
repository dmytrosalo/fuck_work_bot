package handlers

import (
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) handleCasinoStats(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	// Determine period
	period := strings.TrimSpace(c.Message().Payload)
	periodName := "весь час"
	switch period {
	case "today", "сьогодні":
		period = "today"
		periodName = "сьогодні"
	case "week", "тиждень":
		period = "week"
		periodName = "тиждень"
	default:
		period = ""
		periodName = "весь час"
	}

	stats := b.db.GetUserActivityStats(userID, period)
	if len(stats) == 0 {
		return c.Reply("📊 Немає активності за цей період")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Статистика %s (%s)\n━━━━━━━━━━━━━━━━\n\n", userName, periodName))

	totalEarned := 0
	totalSpent := 0

	for _, s := range stats {
		emoji := "📈"
		if s.Total < 0 {
			emoji = "📉"
			totalSpent += -s.Total
		} else {
			totalEarned += s.Total
		}

		sign := "+"
		if s.Total < 0 {
			sign = ""
		}

		sb.WriteString(fmt.Sprintf("%s %s: %s%d 🪙 (%d разів)\n", emoji, s.Activity, sign, s.Total, s.Count))
	}

	balance := b.db.GetBalance(userID, "")
	sb.WriteString(fmt.Sprintf("\n━━━━━━━━━━━━━━━━\n📈 Зароблено: +%d 🪙\n📉 Витрачено: -%d 🪙\n💰 Баланс: %d 🪙", totalEarned, totalSpent, balance))

	return c.Send(sb.String())
}

func (b *Bot) handleGlobalStats(c tele.Context) error {
	period := strings.TrimSpace(c.Message().Payload)
	periodName := "весь час"
	switch period {
	case "today", "сьогодні":
		period = "today"
		periodName = "сьогодні"
	case "week", "тиждень":
		period = "week"
		periodName = "тиждень"
	default:
		period = ""
		periodName = "весь час"
	}

	stats := b.db.GetAllActivityStats(period)
	if len(stats) == 0 {
		return c.Reply("📊 Немає активності")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 Загальна статистика (%s)\n━━━━━━━━━━━━━━━━\n\n", periodName))

	for _, s := range stats {
		sign := "+"
		if s.Total < 0 {
			sign = ""
		}
		sb.WriteString(fmt.Sprintf("%s: %s%d 🪙 (%d разів)\n", s.Activity, sign, s.Total, s.Count))
	}

	return c.Send(sb.String())
}
