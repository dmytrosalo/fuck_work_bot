package handlers

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

const maxPacksPerDay = 5

var rarityStars = map[int]string{
	1: "⭐",
	2: "⭐⭐",
	3: "⭐⭐⭐",
	4: "⭐⭐⭐⭐",
	5: "⭐⭐⭐⭐⭐",
}

var rarityNames = map[int]string{
	1: "Common",
	2: "Uncommon",
	3: "Rare",
	4: "Epic",
	5: "Legendary",
}

// rollRarity returns a rarity based on weighted random.
// 1: 40%, 2: 25%, 3: 20%, 4: 10%, 5: 5%
func rollRarity() int {
	r := rand.Intn(100)
	switch {
	case r < 40:
		return 1
	case r < 65:
		return 2
	case r < 85:
		return 3
	case r < 95:
		return 4
	default:
		return 5
	}
}

// rollGuaranteedRarity returns at least uncommon.
func rollGuaranteedRarity() int {
	r := rollRarity()
	if r < 2 {
		return 2
	}
	return r
}

func (b *Bot) handlePack(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	today := time.Now().Format("2006-01-02")

	opens := b.db.GetPackOpensToday(userID, today)
	if opens >= maxPacksPerDay {
		return c.Reply(fmt.Sprintf("📦 Ти вже відкрив %d паків сьогодні. Приходь завтра!", maxPacksPerDay))
	}
	b.db.IncrementPackOpens(userID, today)

	// Roll 3 cards: 2 random + 1 guaranteed uncommon+
	type packCard struct {
		ID          int
		Name        string
		Rarity      int
		Emoji       string
		Description string
		ATK         int
		DEF         int
		SpecialName string
		Special     int
	}

	var cards []packCard

	rarities := []int{rollRarity(), rollRarity(), rollGuaranteedRarity()}
	for _, rarity := range rarities {
		id, name, emoji, desc, atk, def, specialName, special := b.db.GetRandomCard(rarity)
		if id == 0 {
			continue
		}
		cards = append(cards, packCard{id, name, rarity, emoji, desc, atk, def, specialName, special})

		// Add to collection
		b.db.AddToCollection(userID, id)
	}

	if len(cards) == 0 {
		return c.Reply("📦 Карток поки немає. Зверніться до адміна.")
	}

	// Build message
	var sb strings.Builder
	sb.WriteString("📦 *Відкриваємо пак...*\n\n")

	for _, card := range cards {
		stars := rarityStars[card.Rarity]
		sb.WriteString(fmt.Sprintf("%s %s\n", stars, rarityNames[card.Rarity]))
		sb.WriteString(fmt.Sprintf("%s *%s*\n", card.Emoji, card.Name))
		sb.WriteString(fmt.Sprintf("_%s_\n", card.Description))
		sb.WriteString(fmt.Sprintf("⚔️ %d  🛡 %d  %s: %d\n\n", card.ATK, card.DEF, card.SpecialName, card.Special))
	}

	// Show collection progress
	unique, total := b.db.GetCollectionStats(userID)
	sb.WriteString(fmt.Sprintf("🃏 Колекція: %d/%d", unique, total))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleBattle(c tele.Context) error {
	// Get opponent
	var opponentName string
	var opponentID string

	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		opponent := c.Message().ReplyTo.Sender
		opponentID = fmt.Sprintf("%d", opponent.ID)
		opponentName = opponent.FirstName
		if opponentName == "" {
			opponentName = opponent.Username
		}
	} else if c.Message().Payload != "" {
		opponentName = strings.TrimPrefix(c.Message().Payload, "@")
		return c.Reply("Відповідай на повідомлення суперника командою /battle")
	} else {
		return c.Reply("Відповідай на повідомлення суперника командою /battle")
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	// Don't battle yourself
	if userID == opponentID {
		return c.Reply("Не можна битися з самим собою. Хоча спробуй 🤔")
	}

	// Draw random card from each player's collection
	myCard := b.db.GetRandomCollectionCard(userID)
	theirCard := b.db.GetRandomCollectionCard(opponentID)

	if myCard.ID == 0 {
		return c.Reply("У тебе немає карток! Напиши /pack")
	}
	if theirCard.ID == 0 {
		return c.Reply(fmt.Sprintf("У %s немає карток!", opponentName))
	}

	myPower := myCard.ATK + myCard.DEF + myCard.Special
	theirPower := theirCard.ATK + theirCard.DEF + theirCard.Special

	var sb strings.Builder
	sb.WriteString("⚔️ *БАТЛ!*\n\n")

	// My card
	sb.WriteString(fmt.Sprintf("🔵 *%s*\n", userName))
	sb.WriteString(fmt.Sprintf("%s %s %s\n", rarityStars[myCard.Rarity], myCard.Emoji, myCard.Name))
	sb.WriteString(fmt.Sprintf("ATK: %d  DEF: %d  %s: %d\n", myCard.ATK, myCard.DEF, myCard.SpecialName, myCard.Special))
	sb.WriteString(fmt.Sprintf("💪 Сила: *%d*\n\n", myPower))

	sb.WriteString("vs\n\n")

	// Their card
	sb.WriteString(fmt.Sprintf("🔴 *%s*\n", opponentName))
	sb.WriteString(fmt.Sprintf("%s %s %s\n", rarityStars[theirCard.Rarity], theirCard.Emoji, theirCard.Name))
	sb.WriteString(fmt.Sprintf("ATK: %d  DEF: %d  %s: %d\n", theirCard.ATK, theirCard.DEF, theirCard.SpecialName, theirCard.Special))
	sb.WriteString(fmt.Sprintf("💪 Сила: *%d*\n\n", theirPower))

	// Result
	if myPower > theirPower {
		sb.WriteString(fmt.Sprintf("🏆 *%s* перемагає!", userName))
	} else if theirPower > myPower {
		sb.WriteString(fmt.Sprintf("🏆 *%s* перемагає!", opponentName))
	} else {
		sb.WriteString("🤝 *Нічия!*")
	}

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleCollection(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	unique, total := b.db.GetCollectionStats(userID)
	if unique == 0 {
		return c.Reply("🃏 У тебе ще немає карток. Напиши /pack!")
	}

	// Get cards grouped by rarity
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🃏 *Колекція %s* (%d/%d)\n\n", userName, unique, total))

	for rarity := 5; rarity >= 1; rarity-- {
		cards := b.db.GetCollectionByRarity(userID, rarity)
		if len(cards) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s *%s* (%d)\n", rarityStars[rarity], rarityNames[rarity], len(cards)))
		for _, card := range cards {
			sb.WriteString(fmt.Sprintf("  %s %s (x%d)\n", card.Emoji, card.Name, card.Count))
		}
		sb.WriteString("\n")
	}

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
