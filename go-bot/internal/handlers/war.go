package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

const warReward = 20

type warState struct {
	Player1ID   string
	Player1Name string
	Player2ID   string
	Player2Name string
	ChatID      int64
	Cards1      []storage.BattleCard
	Cards2      []storage.BattleCard
	Order1      []int // indices into Cards1, chosen by player 1
	Order2      []int // indices into Cards2, chosen by player 2
	CreatedAt   time.Time
}

var (
	activeWars = make(map[int64]*warState)
	warMu      sync.Mutex
)

func (b *Bot) handleWar(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	chatID := c.Chat().ID

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

	warMu.Lock()
	if _, ok := activeWars[chatID]; ok {
		warMu.Unlock()
		return c.Reply("❌ В цьому чаті вже йде війна!")
	}

	u1, _ := b.db.GetCollectionStats(userID)
	u2, _ := b.db.GetCollectionStats(opponentID)
	if u1 < 3 {
		warMu.Unlock()
		return c.Reply("У тебе менше 3 карток! /pack")
	}
	if u2 < 3 {
		warMu.Unlock()
		return c.Reply(fmt.Sprintf("У %s менше 3 карток!", opponentName))
	}

	cards1 := b.getRandomCards(userID, 3)
	cards2 := b.getRandomCards(opponentID, 3)

	activeWars[chatID] = &warState{
		Player1ID:   userID,
		Player1Name: userName,
		Player2ID:   opponentID,
		Player2Name: opponentName,
		ChatID:      chatID,
		Cards1:      cards1,
		Cards2:      cards2,
		CreatedAt:   time.Now(),
	}
	warMu.Unlock()

	return c.Send(fmt.Sprintf("⚔️ %s викликає %s на ВІЙНУ!\n\n3 раунди, обирай порядок карток!\n%s, напиши /accept (60 сек)", userName, opponentName, opponentName))
}

func (b *Bot) handleWarAccept(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID

	warMu.Lock()
	war, ok := activeWars[chatID]
	if !ok || time.Since(war.CreatedAt) > 60*time.Second {
		if ok {
			delete(activeWars, chatID)
		}
		warMu.Unlock()
		return nil // silent — /accept also used by duel
	}

	if userID != war.Player2ID {
		warMu.Unlock()
		return nil // not for this player, might be duel accept
	}
	warMu.Unlock()

	c.Send("⚔️ Війна прийнята! Обирайте порядок карток:")

	// Send card ordering to both
	b.sendWarPicker(c.Bot(), chatID, war.Player1ID, war.Player1Name, war.Cards1, "w1", 1)
	b.sendWarPicker(c.Bot(), chatID, war.Player2ID, war.Player2Name, war.Cards2, "w2", 1)

	return nil
}

func (b *Bot) sendWarPicker(bot *tele.Bot, chatID int64, userID, userName string, cards []storage.BattleCard, prefix string, round int) {
	markup := &tele.ReplyMarkup{}
	var buttons []tele.Btn

	for i, card := range cards {
		pwr := card.ATK + card.DEF + card.Special
		label := fmt.Sprintf("%s %s (PWR:%d)", card.Emoji, card.Name, pwr)
		btn := markup.Data(label, "war_pick", fmt.Sprintf("%s:%d:%d:%d", prefix, chatID, i, round))
		buttons = append(buttons, btn)
	}

	var rows []tele.Row
	for _, btn := range buttons {
		rows = append(rows, markup.Row(btn))
	}
	markup.Inline(rows...)

	msg := fmt.Sprintf("⚔️ %s, обери картку для раунду %d:", userName, round)
	bot.Send(&tele.Chat{ID: chatID}, msg, markup)
}

func (b *Bot) handleWarPick(c tele.Context) error {
	data := c.Callback().Data
	parts := strings.Split(data, ":")
	if len(parts) != 4 {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Помилка"})
	}

	prefix := parts[0]
	chatID, _ := strconv.ParseInt(parts[1], 10, 64)
	cardIdx, _ := strconv.Atoi(parts[2])
	// round not used directly but validated

	userID := fmt.Sprintf("%d", c.Sender().ID)

	warMu.Lock()
	war, ok := activeWars[chatID]
	if !ok {
		warMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Війна не знайдена"})
	}

	// Verify player
	isP1 := prefix == "w1" && userID == war.Player1ID
	isP2 := prefix == "w2" && userID == war.Player2ID
	if !isP1 && !isP2 {
		warMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Це не твій вибір!"})
	}

	// Check if already picked this card
	order := &war.Order1
	cards := war.Cards1
	if isP2 {
		order = &war.Order2
		cards = war.Cards2
	}

	for _, idx := range *order {
		if idx == cardIdx {
			warMu.Unlock()
			return c.Respond(&tele.CallbackResponse{Text: "❌ Ця картка вже обрана!"})
		}
	}

	*order = append(*order, cardIdx)
	round := len(*order)

	card := cards[cardIdx]
	c.Edit(fmt.Sprintf("✅ Раунд %d: %s %s", round, card.Emoji, card.Name))

	// Need more picks?
	if round < 3 {
		warMu.Unlock()
		// Send next picker with remaining cards
		remaining := make([]storage.BattleCard, 0)
		remainingIdx := make([]int, 0)
		for i, c := range cards {
			picked := false
			for _, idx := range *order {
				if i == idx {
					picked = true
					break
				}
			}
			if !picked {
				remaining = append(remaining, c)
				remainingIdx = append(remainingIdx, i)
			}
		}

		markup := &tele.ReplyMarkup{}
		var buttons []tele.Btn
		for j, card := range remaining {
			pwr := card.ATK + card.DEF + card.Special
			label := fmt.Sprintf("%s %s (PWR:%d)", card.Emoji, card.Name, pwr)
			btn := markup.Data(label, "war_pick", fmt.Sprintf("%s:%d:%d:%d", prefix, chatID, remainingIdx[j], round+1))
			buttons = append(buttons, btn)
		}
		var rows []tele.Row
		for _, btn := range buttons {
			rows = append(rows, markup.Row(btn))
		}
		markup.Inline(rows...)

		playerName := war.Player1Name
		if isP2 {
			playerName = war.Player2Name
		}
		c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("⚔️ %s, обери картку для раунду %d:", playerName, round+1), markup)

		return c.Respond()
	}

	// Check if both have all 3 picks
	if len(war.Order1) == 3 && len(war.Order2) == 3 {
		delete(activeWars, chatID)
		warMu.Unlock()
		b.resolveWar(c.Bot(), war)
		return c.Respond(&tele.CallbackResponse{Text: "⚔️ Війна!"})
	}

	warMu.Unlock()
	return c.Respond(&tele.CallbackResponse{Text: fmt.Sprintf("✅ Обрано! Чекаємо суперника...")})
}

func (b *Bot) resolveWar(bot *tele.Bot, war *warState) {
	var sb strings.Builder
	sb.WriteString("⚔️ ВІЙНА — РЕЗУЛЬТАТ!\n\n")

	wins1, wins2 := 0, 0

	for round := 0; round < 3; round++ {
		c1 := war.Cards1[war.Order1[round]]
		c2 := war.Cards2[war.Order2[round]]
		pwr1 := c1.ATK + c1.DEF + c1.Special
		pwr2 := c2.ATK + c2.DEF + c2.Special

		sb.WriteString(fmt.Sprintf("Раунд %d:\n", round+1))
		sb.WriteString(fmt.Sprintf("🔵 %s %s (PWR:%d)\n", c1.Emoji, c1.Name, pwr1))
		sb.WriteString(fmt.Sprintf("🔴 %s %s (PWR:%d)\n", c2.Emoji, c2.Name, pwr2))

		if pwr1 > pwr2 {
			wins1++
			sb.WriteString(fmt.Sprintf("→ %s!\n\n", war.Player1Name))
		} else if pwr2 > pwr1 {
			wins2++
			sb.WriteString(fmt.Sprintf("→ %s!\n\n", war.Player2Name))
		} else {
			sb.WriteString("→ Нічия!\n\n")
		}
	}

	sb.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━\n%s %d — %d %s\n\n", war.Player1Name, wins1, wins2, war.Player2Name))

	if wins1 > wins2 {
		loserCard := war.Cards2[war.Order2[rand.Intn(3)]]
		b.db.TransferCard(war.Player2ID, war.Player1ID, loserCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає і забирає %s %s!", war.Player1Name, loserCard.Emoji, loserCard.Name))
		b.db.IncrementStat(war.Player1ID, "duels_won", 1)
		b.db.IncrementStat(war.Player2ID, "duels_lost", 1)
	} else if wins2 > wins1 {
		loserCard := war.Cards1[war.Order1[rand.Intn(3)]]
		b.db.TransferCard(war.Player1ID, war.Player2ID, loserCard.ID)
		sb.WriteString(fmt.Sprintf("🏆 %s перемагає і забирає %s %s!", war.Player2Name, loserCard.Emoji, loserCard.Name))
		b.db.IncrementStat(war.Player2ID, "duels_won", 1)
		b.db.IncrementStat(war.Player1ID, "duels_lost", 1)
	} else {
		sb.WriteString("🤝 Нічия! Ніхто не втрачає")
	}

	bot.Send(&tele.Chat{ID: war.ChatID}, sb.String())
}

func init() {
	go func() {
		for {
			time.Sleep(60 * time.Second)
			warMu.Lock()
			for chatID, war := range activeWars {
				if time.Since(war.CreatedAt) > 3*time.Minute {
					delete(activeWars, chatID)
				}
			}
			warMu.Unlock()
		}
	}()
}
