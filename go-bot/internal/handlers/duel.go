package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

const duelReward = 15

type duelState struct {
	ChallengerID   string
	ChallengerName string
	OpponentID     string
	OpponentName   string
	ChatID         int64
	Cards1         []storage.BattleCard // challenger's options
	Cards2         []storage.BattleCard // opponent's options
	Pick1          *storage.BattleCard  // challenger's pick
	Pick2          *storage.BattleCard  // opponent's pick
	CreatedAt      time.Time
}

var (
	activeDuels = make(map[int64]*duelState) // chatID -> duel
	duelMu      sync.Mutex
)

func (b *Bot) handleDuel(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	chatID := c.Chat().ID

	// Get opponent
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
		return c.Reply("Вкажи суперника: /duel @username або відповідай на повідомлення")
	}

	if userID == opponentID {
		return c.Reply("Не можна дуелити з самим собою 🤦")
	}

	// Check both have cards
	u1, _ := b.db.GetCollectionStats(userID)
	u2, _ := b.db.GetCollectionStats(opponentID)
	if u1 < 3 {
		return c.Reply("У тебе менше 3 карток! Спочатку /pack")
	}
	if u2 < 3 {
		return c.Reply(fmt.Sprintf("У %s менше 3 карток!", opponentName))
	}

	// Get 3 random cards for each
	cards1 := b.getRandomCards(userID, 3)
	cards2 := b.getRandomCards(opponentID, 3)

	duelMu.Lock()
	activeDuels[chatID] = &duelState{
		ChallengerID:   userID,
		ChallengerName: userName,
		OpponentID:     opponentID,
		OpponentName:   opponentName,
		ChatID:         chatID,
		Cards1:         cards1,
		Cards2:         cards2,
		CreatedAt:      time.Now(),
	}
	duelMu.Unlock()

	return c.Send(fmt.Sprintf("⚔️ *%s* викликає *%s* на дуель!\n\n%s, напиши /accept щоб прийняти (60 сек)",
		userName, opponentName, opponentName),
		&tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleAccept(c tele.Context) error {
	// Try war and dart first
	b.handleWarAccept(c)
	b.handleDartAccept(c)

	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID

	duelMu.Lock()
	duel, ok := activeDuels[chatID]
	if !ok || time.Since(duel.CreatedAt) > 60*time.Second {
		if ok {
			delete(activeDuels, chatID)
		}
		duelMu.Unlock()
		return nil // silent — war might have handled it
	}

	if userID != duel.OpponentID {
		duelMu.Unlock()
		return nil
	}
	duelMu.Unlock()

	// Send card selection to both players
	c.Send("⚔️ Дуель прийнята! Обирайте картки:")

	// Challenger picks
	b.sendCardPicker(c.Bot(), chatID, duel.ChallengerID, duel.ChallengerName, duel.Cards1, "ch")
	// Opponent picks
	b.sendCardPicker(c.Bot(), chatID, duel.OpponentID, duel.OpponentName, duel.Cards2, "op")

	return nil
}

func (b *Bot) sendCardPicker(bot *tele.Bot, chatID int64, userID, userName string, cards []storage.BattleCard, prefix string) {
	markup := &tele.ReplyMarkup{}
	var buttons []tele.Btn
	for _, card := range cards {
		label := fmt.Sprintf("%s %s", card.Emoji, card.Name)
		btn := markup.Data(label, "duel_pick", fmt.Sprintf("%s:%d:%d", prefix, chatID, card.ID))
		buttons = append(buttons, btn)
	}

	var rows []tele.Row
	for _, btn := range buttons {
		rows = append(rows, markup.Row(btn))
	}
	markup.Inline(rows...)

	msg := fmt.Sprintf("🃏 %s, обери картку для дуелі:", userName)
	bot.Send(&tele.Chat{ID: chatID}, msg, markup)
}

func (b *Bot) handleDuelPick(c tele.Context) error {
	data := c.Callback().Data
	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Помилка"})
	}

	prefix := parts[0]
	chatID, _ := strconv.ParseInt(parts[1], 10, 64)
	cardID, _ := strconv.Atoi(parts[2])
	userID := fmt.Sprintf("%d", c.Sender().ID)

	duelMu.Lock()
	duel, ok := activeDuels[chatID]
	if !ok {
		duelMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Дуель не знайдена"})
	}

	// Verify it's the right player
	if prefix == "ch" && userID != duel.ChallengerID {
		duelMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Це не твій вибір!"})
	}
	if prefix == "op" && userID != duel.OpponentID {
		duelMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Це не твій вибір!"})
	}

	// Find the card
	var picked *storage.BattleCard
	cards := duel.Cards1
	if prefix == "op" {
		cards = duel.Cards2
	}
	for i := range cards {
		if cards[i].ID == cardID {
			picked = &cards[i]
			break
		}
	}
	if picked == nil {
		duelMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Картка не знайдена"})
	}

	if prefix == "ch" {
		duel.Pick1 = picked
	} else {
		duel.Pick2 = picked
	}

	// Remove buttons
	c.Edit(fmt.Sprintf("✅ Обрано: %s %s", picked.Emoji, picked.Name))

	// Check if both picked
	if duel.Pick1 != nil && duel.Pick2 != nil {
		// Resolve duel
		delete(activeDuels, chatID)
		duelMu.Unlock()
		b.resolveDuel(c.Bot(), duel)
		return c.Respond(&tele.CallbackResponse{Text: "⚔️ Дуель!"})
	}

	duelMu.Unlock()
	return c.Respond(&tele.CallbackResponse{Text: fmt.Sprintf("✅ %s %s обрано! Чекаємо суперника...", picked.Emoji, picked.Name)})
}

func (b *Bot) resolveDuel(bot *tele.Bot, duel *duelState) {
	p1 := duel.Pick1
	p2 := duel.Pick2
	power1 := p1.ATK + p1.DEF + p1.Special
	power2 := p2.ATK + p2.DEF + p2.Special

	var sb strings.Builder
	sb.WriteString("⚔️ ДУЕЛЬ — РЕЗУЛЬТАТ!\n\n")

	sb.WriteString(fmt.Sprintf("🔵 %s\n", duel.ChallengerName))
	sb.WriteString(fmt.Sprintf("%s %s %s\n", rarityStars[p1.Rarity], p1.Emoji, p1.Name))
	sb.WriteString(fmt.Sprintf("ATK: %d  DEF: %d  %s: %d\n", p1.ATK, p1.DEF, p1.SpecialName, p1.Special))
	sb.WriteString(fmt.Sprintf("💪 PWR: %d\n\n", power1))

	sb.WriteString("⚡ vs ⚡\n\n")

	sb.WriteString(fmt.Sprintf("🔴 %s\n", duel.OpponentName))
	sb.WriteString(fmt.Sprintf("%s %s %s\n", rarityStars[p2.Rarity], p2.Emoji, p2.Name))
	sb.WriteString(fmt.Sprintf("ATK: %d  DEF: %d  %s: %d\n", p2.ATK, p2.DEF, p2.SpecialName, p2.Special))
	sb.WriteString(fmt.Sprintf("💪 PWR: %d\n\n", power2))

	if power1 > power2 {
		b.db.TransferCoins(duel.OpponentID, duel.ChallengerID, duelReward)
		b.db.TransferCard(duel.OpponentID, duel.ChallengerID, p2.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає!\n+%d 🪙 і забирає %s %s!",
			duel.ChallengerName, duelReward, p2.Emoji, p2.Name))
	} else if power2 > power1 {
		b.db.TransferCoins(duel.ChallengerID, duel.OpponentID, duelReward)
		b.db.TransferCard(duel.ChallengerID, duel.OpponentID, p1.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає!\n+%d 🪙 і забирає %s %s!",
			duel.OpponentName, duelReward, p1.Emoji, p1.Name))
	} else {
		sb.WriteString("🤝 Нічия! Обидві картки залишаються")
	}

	chat := &tele.Chat{ID: duel.ChatID}
	bot.Send(chat, sb.String())
}

func (b *Bot) getRandomCards(userID string, count int) []storage.BattleCard {
	var cards []storage.BattleCard
	seen := make(map[int]bool)
	for i := 0; i < count*3 && len(cards) < count; i++ {
		card := b.db.GetRandomCollectionCard(userID)
		if card.ID == 0 || seen[card.ID] {
			continue
		}
		seen[card.ID] = true
		cards = append(cards, card)
	}
	return cards
}

func init() {
	// Cleanup expired duels every minute
	go func() {
		for {
			time.Sleep(60 * time.Second)
			duelMu.Lock()
			for chatID, duel := range activeDuels {
				if time.Since(duel.CreatedAt) > 2*time.Minute {
					delete(activeDuels, chatID)
					log.Printf("[duel] Expired duel in chat %d", chatID)
				}
			}
			duelMu.Unlock()
		}
	}()
}
