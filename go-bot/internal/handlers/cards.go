package handlers

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

const (
	maxPacksPerDay = 10
	packCost       = 20
	battleReward   = 10
)

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
// 1: 40%, 2: 25%, 3: 25%, 4: 7%, 5: 3%
func rollRarity() int {
	r := rand.Intn(100)
	switch {
	case r < 40:
		return 1
	case r < 65:
		return 2
	case r < 90:
		return 3
	case r < 97:
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

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	opens := b.db.GetPackOpensToday(userID, today)
	if opens >= maxPacksPerDay {
		return c.Reply(fmt.Sprintf("📦 Ліміт %d паків на день. Приходь завтра!", maxPacksPerDay))
	}

	balance := b.db.GetBalance(userID, userName)
	if balance < packCost {
		return c.Reply(fmt.Sprintf("💸 Недостатньо богдудіків!\nПак: %d 🪙\nБаланс: %d 🪙\n\n_/daily — щоденний бонус_", packCost, balance), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	b.db.UpdateBalance(userID, userName, -packCost)
	b.db.IncrementPackOpens(userID, today)

	// Roll 3 cards: 2 random + 1 guaranteed uncommon+
	var cards []CardData

	rarities := []int{rollRarity(), rollRarity(), rollGuaranteedRarity()}
	for _, rarity := range rarities {
		fc := b.db.GetRandomCard(rarity)
		if fc.ID == 0 {
			continue
		}
		cards = append(cards, CardData{
			Name: fc.Name, Rarity: fc.Rarity, Emoji: fc.Emoji,
			Description: fc.Description, ATK: fc.ATK, DEF: fc.DEF,
			SpecialName: fc.SpecialName, Special: fc.Special,
		})
		b.db.AddToCollection(userID, fc.ID)
	}

	if len(cards) == 0 {
		return c.Reply("📦 Карток поки немає. Зверніться до адміна.")
	}

	unique, total := b.db.GetCollectionStats(userID)
	newBalance := b.db.GetBalance(userID, "")

	// Try to render card images and send as album
	var album tele.Album
	allRendered := true

	for i, card := range cards {
		imgBytes, err := renderCard(card)
		if err != nil {
			allRendered = false
			break
		}
		caption := ""
		if i == len(cards)-1 {
			caption = fmt.Sprintf("📦 Пак відкрито!\n🃏 %d/%d | 🪙 %d", unique, total, newBalance)
		}
		photo := &tele.Photo{
			File:    tele.FromReader(bytes.NewReader(imgBytes)),
			Caption: caption,
		}
		album = append(album, photo)
	}

	if allRendered && len(album) > 0 {
		err := c.SendAlbum(album)
		if err == nil {
			return nil
		}
		log.Printf("[pack] Album send failed: %v, falling back to text", err)
	}

	// Fallback to text (no Markdown to avoid formatting issues)
	var sb strings.Builder
	sb.WriteString("📦 Пак відкрито!\n━━━━━━━━━━━━━━━━\n\n")
	for i, card := range cards {
		sb.WriteString(fmt.Sprintf("%s %s\n", rarityStars[card.Rarity], rarityNames[card.Rarity]))
		sb.WriteString(fmt.Sprintf("%s %s\n", card.Emoji, card.Name))
		sb.WriteString(fmt.Sprintf("⚔️%d  🛡%d  %s: %d\n", card.ATK, card.DEF, card.SpecialName, card.Special))
		sb.WriteString(fmt.Sprintf("%s\n", card.Description))
		if i < len(cards)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString(fmt.Sprintf("\n━━━━━━━━━━━━━━━━\n🃏 %d/%d | 🪙 %d", unique, total, newBalance))
	return c.Send(sb.String())
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
		// Try resolving username to name first
		resolved := resolveTarget(opponentName, opponentName)
		// Search by resolved name, then original
		if id, found := b.db.FindUserByName(resolved); found {
			opponentID = id
			opponentName = resolved
		} else if id, found := b.db.FindUserByName(opponentName); found {
			opponentID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений. Нехай спочатку напише /daily", opponentName))
		}
	} else {
		return c.Reply("Відповідай на повідомлення або напиши /battle @username")
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
		b.db.TransferCoins(opponentID, userID, battleReward)
		b.db.TransferCard(opponentID, userID, theirCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 *%s* перемагає!\n+%d 🪙 і забирає %s %s!", userName, battleReward, theirCard.Emoji, theirCard.Name))
	} else if theirPower > myPower {
		b.db.TransferCoins(userID, opponentID, battleReward)
		b.db.TransferCard(userID, opponentID, myCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 *%s* перемагає!\n%s втрачає %d 🪙 і %s %s", opponentName, userName, battleReward, myCard.Emoji, myCard.Name))
	} else {
		sb.WriteString("🤝 *Нічия!* Ніхто нічого не втрачає")
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
