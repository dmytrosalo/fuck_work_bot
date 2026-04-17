package handlers

import (
	"fmt"
	"math/rand"
	"time"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) handleRob(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	targetName, targetUsername := getTarget(c)
	if targetName == userName && c.Message().Payload == "" && c.Message().ReplyTo == nil {
		return c.Reply("Вкажи жертву: /rob @username або відповідай на повідомлення")
	}

	target := resolveTarget(targetName, targetUsername)
	targetID := ""
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		targetID = fmt.Sprintf("%d", c.Message().ReplyTo.Sender.ID)
	} else {
		if id, found := b.db.FindUserByName(target); found {
			targetID = id
		} else if id, found := b.db.FindUserByName(targetName); found {
			targetID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", targetName))
		}
	}

	if userID == targetID {
		return c.Reply("Не можна грабувати себе 🤦")
	}

	// Cooldown: 1 per hour
	today := time.Now().Format("2006-01-02-15")
	robKey := "rob:" + userID + ":" + today
	if b.db.GetMeta(robKey) != "" {
		return c.Reply(fmt.Sprintf("🕐 Можна грабувати раз на годину! Через %s", timeUntilNextHour()))
	}
	b.db.SetMeta(robKey, "done")

	targetBalance := b.db.GetBalance(targetID, "")
	if targetBalance <= 0 {
		return c.Reply(fmt.Sprintf("💸 У %s немає грошей!", targetName))
	}

	// 40% success, 60% fail
	if rand.Intn(100) < 40 {
		// Steal 10-50% of their balance
		pct := rand.Intn(41) + 10 // 10-50%
		stolen := targetBalance * pct / 100
		if stolen < 1 {
			stolen = 1
		}

		b.db.TransferCoins(targetID, userID, stolen)
		b.db.LogTransaction(userID, userName, "rob", stolen)
		b.db.LogTransaction(targetID, targetName, "robbed", -stolen)
		newBal := b.db.GetBalance(userID, "")
		return c.Reply(fmt.Sprintf("💰 Пограбував %s на %d 🪙 (%d%%)!\nБаланс: %d 🪙", targetName, stolen, pct, newBal))
	}

	// Fail — lose 20 coins, victim gets them
	penalty := 20
	b.db.UpdateBalance(userID, userName, -penalty)
	b.db.UpdateBalance(targetID, targetName, penalty)
	b.db.LogTransaction(userID, userName, "rob_fail", -penalty)
	b.db.LogTransaction(targetID, targetName, "rob_comp", penalty)
	bal := b.db.GetBalance(userID, "")
	return c.Reply(fmt.Sprintf("🚔 Спіймали при спробі пограбувати %s!\n-%d 🪙 (баланс: %d)\n%s отримує +%d 🪙 як компенсацію", targetName, penalty, bal, targetName, penalty))
}
