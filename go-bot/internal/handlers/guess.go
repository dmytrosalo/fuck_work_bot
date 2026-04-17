package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type guessGame struct {
	Number    int
	ChatID    int64
	Players   map[string]int // userID -> guess
	Names     map[string]string
	CreatedAt time.Time
}

var (
	activeGuesses = make(map[int64]*guessGame)
	guessMu       sync.Mutex
)

func (b *Bot) handleGuess(c tele.Context) error {
	chatID := c.Chat().ID

	guessMu.Lock()
	if g, ok := activeGuesses[chatID]; ok && time.Since(g.CreatedAt) < 60*time.Second {
		guessMu.Unlock()
		return c.Reply("❓ Гра вже йде! Напиши число від 1 до 100")
	}

	number := rand.Intn(100) + 1
	activeGuesses[chatID] = &guessGame{
		Number:    number,
		ChatID:    chatID,
		Players:   make(map[string]int),
		Names:     make(map[string]string),
		CreatedAt: time.Now(),
	}
	guessMu.Unlock()

	c.Send("🎯 *Вгадай число!*\n\nЯ загадав число від 1 до 100.\nВсі пишуть своє число (60 сек).\nХто ближче — той виграє 30 🪙!", &tele.SendOptions{ParseMode: tele.ModeMarkdown})

	// Auto-close after 60 seconds
	go func() {
		time.Sleep(60 * time.Second)
		b.closeGuess(c.Bot(), chatID)
	}()

	return nil
}

func (b *Bot) checkGuessAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.TrimSpace(c.Text())

	num, err := strconv.Atoi(text)
	if err != nil || num < 1 || num > 100 {
		return false
	}

	guessMu.Lock()
	game, ok := activeGuesses[chatID]
	if !ok {
		guessMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	if _, already := game.Players[userID]; already {
		guessMu.Unlock()
		return false // already guessed
	}

	game.Players[userID] = num
	game.Names[userID] = userName
	guessMu.Unlock()

	return true // consumed the message
}

func (b *Bot) closeGuess(bot *tele.Bot, chatID int64) {
	guessMu.Lock()
	game, ok := activeGuesses[chatID]
	if !ok {
		guessMu.Unlock()
		return
	}
	delete(activeGuesses, chatID)
	guessMu.Unlock()

	chat := &tele.Chat{ID: chatID}

	if len(game.Players) == 0 {
		bot.Send(chat, fmt.Sprintf("🎯 Ніхто не вгадував! Число було: %d", game.Number))
		return
	}

	// Find closest
	var winnerID, winnerName string
	minDiff := 101
	for uid, guess := range game.Players {
		diff := guess - game.Number
		if diff < 0 {
			diff = -diff
		}
		if diff < minDiff {
			minDiff = diff
			winnerID = uid
			winnerName = game.Names[uid]
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎯 Число було: *%d*\n\n", game.Number))

	for uid, guess := range game.Players {
		diff := guess - game.Number
		if diff < 0 {
			diff = -diff
		}
		marker := ""
		if uid == winnerID {
			marker = " 🏆"
		}
		sb.WriteString(fmt.Sprintf("%s: %d (різниця: %d)%s\n", game.Names[uid], guess, diff, marker))
	}

	reward := 30
	b.db.UpdateBalance(winnerID, winnerName, reward)
	sb.WriteString(fmt.Sprintf("\n🏆 *%s* виграє +%d 🪙!", winnerName, reward))

	bot.Send(chat, sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
