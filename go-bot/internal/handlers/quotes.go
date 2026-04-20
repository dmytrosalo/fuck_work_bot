package handlers

import (
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) handleQuote(c tele.Context) error {
	author, text := b.db.GetRandomQuote()
	if text == "" {
		return c.Reply("Цитат поки немає")
	}
	return c.Reply(fmt.Sprintf("💬 \"%s\"\n\n— %s", text, author))
}

func (b *Bot) handleAddQuote(c tele.Context) error {
	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Text == "" {
		return c.Reply("Відповідай на повідомлення командою /addquote")
	}

	text := c.Message().ReplyTo.Text
	author := c.Message().ReplyTo.Sender.FirstName
	if author == "" {
		author = c.Message().ReplyTo.Sender.Username
	}

	b.db.AddQuote(author, text)

	senderID := fmt.Sprintf("%d", c.Sender().ID)
	b.db.IncrementStat(senderID, "quotes_added", 1)
	b.checkAchievements(c, senderID, c.Sender().FirstName)

	return c.Reply(fmt.Sprintf("💬 Цитату від %s збережено!", author))
}

const roastCost = 5

func (b *Bot) handleRoast(c tele.Context) error {
	targetName, targetUsername := getTarget(c)
	target := resolveTarget(targetName, targetUsername)

	// Roasting others costs coins, self-roast is free
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	isSelf := targetName == userName || (c.Message().Payload == "" && c.Message().ReplyTo == nil)

	if !isSelf {
		roastBonus := b.getTitleBonus(userID)
		if !roastBonus.FreeRoasts {
			balance := b.db.GetBalance(userID, userName)
			if balance < roastCost {
				return c.Reply(fmt.Sprintf("💸 Роаст коштує %d 🪙, у тебе %d 🪙", roastCost, balance))
			}
			b.db.UpdateBalance(userID, userName, -roastCost)
		}
	}

	roast := b.db.GetRandomRoast(target)
	if roast == "" {
		roast = "Навіть роастів на тебе не вистачило"
	}
	roast = strings.ReplaceAll(roast, "{name}", targetName)

	roasterID := fmt.Sprintf("%d", c.Sender().ID)
	b.db.IncrementStat(roasterID, "roasts_given", 1)
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		targetRoastID := fmt.Sprintf("%d", c.Message().ReplyTo.Sender.ID)
		b.db.IncrementStat(targetRoastID, "roasts_received", 1)
	}
	b.checkAchievements(c, roasterID, c.Sender().FirstName)

	return c.Reply(fmt.Sprintf("🔥 %s", roast))
}

func (b *Bot) handleCompliment(c tele.Context) error {
	targetName, targetUsername := getTarget(c)
	target := resolveTarget(targetName, targetUsername)

	compliment := b.db.GetRandomCompliment(target)
	if compliment == "" {
		compliment = "Ти топ, не слухай нікого"
	}
	compliment = strings.ReplaceAll(compliment, "{name}", targetName)

	senderID := fmt.Sprintf("%d", c.Sender().ID)
	b.db.IncrementStat(senderID, "compliments_given", 1)
	b.checkAchievements(c, senderID, c.Sender().FirstName)

	return c.Reply(fmt.Sprintf("💖 %s", compliment))
}

func getTarget(c tele.Context) (name, username string) {
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		target := c.Message().ReplyTo.Sender
		name = target.FirstName
		if name == "" {
			name = target.Username
		}
		username = target.Username
		return
	}

	if c.Message().Payload != "" {
		name = strings.TrimPrefix(c.Message().Payload, "@")
		username = name
		return
	}

	name = c.Sender().FirstName
	if name == "" {
		name = c.Sender().Username
	}
	username = c.Sender().Username
	return
}
