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
	maxPacksPerDay = 7
	packCost       = 40
)

var rarityStars = map[int]string{
	1: "⭐",
	2: "⭐⭐",
	3: "⭐⭐⭐",
	4: "⭐⭐⭐⭐",
	5: "⭐⭐⭐⭐⭐",
	6: "💎💎💎💎💎💎",
}

var rarityNames = map[int]string{
	1: "Common",
	2: "Uncommon",
	3: "Rare",
	4: "Epic",
	5: "Legendary",
	6: "ULTRA LEGENDARY MAX PRO",
}

// rollRarity returns a rarity based on weighted random.
// 1: 35%, 2: 30%, 3: 20%, 4: 10%, 5: 4%, 6: 1%
func rollRarity() int {
	r := rand.Intn(100)
	switch {
	case r < 35:
		return 1
	case r < 65:
		return 2
	case r < 85:
		return 3
	case r < 95:
		return 4
	case r < 99:
		return 5
	default:
		return 6
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
	b.db.LogTransaction(userID, userName, "pack", -packCost)
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
