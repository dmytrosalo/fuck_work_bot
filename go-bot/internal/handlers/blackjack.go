package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"

	tele "gopkg.in/telebot.v3"
)

var suits = []string{"♠", "♥", "♦", "♣"}
var ranks = []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}

type card struct {
	Rank string
	Suit string
}

func (c card) String() string {
	return c.Rank + c.Suit
}

func cardValue(c card) int {
	switch c.Rank {
	case "A":
		return 11
	case "K", "Q", "J":
		return 10
	default:
		v, _ := strconv.Atoi(c.Rank)
		return v
	}
}

func handValue(cards []card) int {
	total := 0
	aces := 0
	for _, c := range cards {
		total += cardValue(c)
		if c.Rank == "A" {
			aces++
		}
	}
	for aces > 0 && total > 21 {
		total -= 10
		aces--
	}
	return total
}

func handString(cards []card) string {
	var parts []string
	for _, c := range cards {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, " ")
}

type bjGame struct {
	PlayerCards []card
	DealerCards []card
	Deck        []card
	Bet         int
	UserID      string
	UserName    string
}

var (
	activeBlackjack = make(map[string]*bjGame) // userID -> game
	bjMu            sync.Mutex
)

func newDeck() []card {
	var deck []card
	for _, suit := range suits {
		for _, rank := range ranks {
			deck = append(deck, card{rank, suit})
		}
	}
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	return deck
}

func (g *bjGame) draw() card {
	c := g.Deck[0]
	g.Deck = g.Deck[1:]
	return c
}

func (b *Bot) handleBlackjack(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	bjMu.Lock()
	if _, ok := activeBlackjack[userID]; ok {
		bjMu.Unlock()
		return c.Reply("🃏 У тебе вже є активна гра! Натисни Hit або Stand")
	}
	bjMu.Unlock()

	bet := 10
	if c.Message().Payload != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(c.Message().Payload))
		if err != nil {
			return c.Reply("Формат: /blackjack <ставка>")
		}
		bet = parsed
	}
	if bet < 1 {
		return c.Reply("❌ Мінімальна ставка: 1 🪙")
	}
	if bet > 500 {
		return c.Reply("❌ Максимальна ставка: 500 🪙")
	}

	balance := b.db.GetBalance(userID, userName)
	if balance < bet {
		return c.Reply(fmt.Sprintf("💸 Недостатньо! Баланс: %d 🪙", balance))
	}

	b.db.UpdateBalance(userID, userName, -bet)

	game := &bjGame{
		Deck:     newDeck(),
		Bet:      bet,
		UserID:   userID,
		UserName: userName,
	}

	game.PlayerCards = append(game.PlayerCards, game.draw(), game.draw())
	game.DealerCards = append(game.DealerCards, game.draw(), game.draw())

	playerVal := handValue(game.PlayerCards)

	// Check for natural blackjack
	if playerVal == 21 {
		dealerVal := handValue(game.DealerCards)
		winnings := bet * 5 / 2 // 2.5x
		if dealerVal == 21 {
			// Push
			b.db.UpdateBalance(userID, userName, bet)
			return c.Send(fmt.Sprintf("🃏 *Blackjack!*\n\nТвої: %s = 21\nДилер: %s = 21\n\n🤝 Нічия! Ставка повернута",
				handString(game.PlayerCards), handString(game.DealerCards)),
				&tele.SendOptions{ParseMode: tele.ModeMarkdown})
		}
		b.db.UpdateBalance(userID, userName, winnings)
		newBal := b.db.GetBalance(userID, "")
		return c.Send(fmt.Sprintf("🃏 *BLACKJACK!* 🎉\n\nТвої: %s = 21\nДилер: %s 🂠\n\n+%d 🪙 (баланс: %d)",
			handString(game.PlayerCards), game.DealerCards[0].String(), winnings, newBal),
			&tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	bjMu.Lock()
	activeBlackjack[userID] = game
	bjMu.Unlock()

	markup := &tele.ReplyMarkup{}
	btnHit := markup.Data("🃏 Hit", "bj_hit", userID)
	btnStand := markup.Data("✋ Stand", "bj_stand", userID)
	markup.Inline(markup.Row(btnHit, btnStand))

	msg := fmt.Sprintf("🃏 Blackjack (ставка: %d 🪙)\n\nТвої: %s = %d\nДилер: %s 🂠\n\nHit або Stand?",
		bet, handString(game.PlayerCards), playerVal, game.DealerCards[0].String())

	return c.Send(msg, markup)
}

func (b *Bot) handleBJHit(c tele.Context) error {
	targetUserID := c.Callback().Data
	userID := fmt.Sprintf("%d", c.Sender().ID)

	if userID != targetUserID {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Це не твоя гра!"})
	}

	bjMu.Lock()
	game, ok := activeBlackjack[userID]
	if !ok {
		bjMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Гра не знайдена"})
	}

	game.PlayerCards = append(game.PlayerCards, game.draw())
	playerVal := handValue(game.PlayerCards)

	if playerVal > 21 {
		delete(activeBlackjack, userID)
		bjMu.Unlock()

		bal := b.db.GetBalance(userID, "")
		c.Edit(fmt.Sprintf("🃏 Blackjack\n\nТвої: %s = %d 💥 BUST!\nДилер: %s\n\n❌ Програв -%d 🪙 (баланс: %d)",
			handString(game.PlayerCards), playerVal, handString(game.DealerCards), game.Bet, bal))
		return c.Respond(&tele.CallbackResponse{Text: "💥 Bust!"})
	}

	if playerVal == 21 {
		delete(activeBlackjack, userID)
		bjMu.Unlock()
		return b.bjStand(c, game)
	}

	bjMu.Unlock()

	markup := &tele.ReplyMarkup{}
	btnHit := markup.Data("🃏 Hit", "bj_hit", userID)
	btnStand := markup.Data("✋ Stand", "bj_stand", userID)
	markup.Inline(markup.Row(btnHit, btnStand))

	c.Edit(fmt.Sprintf("🃏 Blackjack (ставка: %d 🪙)\n\nТвої: %s = %d\nДилер: %s 🂠\n\nHit або Stand?",
		game.Bet, handString(game.PlayerCards), playerVal, game.DealerCards[0].String()),
		markup)

	return c.Respond()
}

func (b *Bot) handleBJStand(c tele.Context) error {
	targetUserID := c.Callback().Data
	userID := fmt.Sprintf("%d", c.Sender().ID)

	if userID != targetUserID {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Це не твоя гра!"})
	}

	bjMu.Lock()
	game, ok := activeBlackjack[userID]
	if !ok {
		bjMu.Unlock()
		return c.Respond(&tele.CallbackResponse{Text: "❌ Гра не знайдена"})
	}
	delete(activeBlackjack, userID)
	bjMu.Unlock()

	return b.bjStand(c, game)
}

func (b *Bot) bjStand(c tele.Context, game *bjGame) error {
	// Dealer draws until 17
	for handValue(game.DealerCards) < 17 {
		game.DealerCards = append(game.DealerCards, game.draw())
	}

	playerVal := handValue(game.PlayerCards)
	dealerVal := handValue(game.DealerCards)

	var result string
	var winnings int

	if dealerVal > 21 {
		winnings = game.Bet * 2
		result = fmt.Sprintf("🏆 Дилер bust! +%d 🪙", winnings)
	} else if playerVal > dealerVal {
		winnings = game.Bet * 2
		result = fmt.Sprintf("🏆 Ти виграв! +%d 🪙", winnings)
	} else if dealerVal > playerVal {
		result = fmt.Sprintf("❌ Дилер виграв. -%d 🪙", game.Bet)
	} else {
		winnings = game.Bet
		result = "🤝 Нічия! Ставка повернута"
	}

	if winnings > 0 {
		b.db.UpdateBalance(game.UserID, game.UserName, winnings)
		b.db.LogTransaction(game.UserID, game.UserName, "blackjack", winnings-game.Bet)
	} else {
		b.db.LogTransaction(game.UserID, game.UserName, "blackjack", -game.Bet)
	}
	bal := b.db.GetBalance(game.UserID, "")

	msg := fmt.Sprintf("🃏 Blackjack — Результат\n\nТвої: %s = %d\nДилер: %s = %d\n\n%s\nБаланс: %d 🪙",
		handString(game.PlayerCards), playerVal,
		handString(game.DealerCards), dealerVal,
		result, bal)

	c.Edit(msg)
	return c.Respond()
}
