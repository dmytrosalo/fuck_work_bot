package handlers

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type flagGame struct {
	ID        int64
	Country   string
	ChatID    int64
	Aliases   []string
	Winner    string
	SentMsg   *tele.Message
	CreatedAt time.Time
}

var (
	activeFlagGame = make(map[int64]*flagGame)
	flagGameMu     sync.Mutex
)

func (b *Bot) handleFlagGuess(c tele.Context) error {
	chatID := c.Chat().ID

	flagGameMu.Lock()
	if g, ok := activeFlagGame[chatID]; ok && time.Since(g.CreatedAt) < 30*time.Second {
		flagGameMu.Unlock()
		return c.Reply("🏳️ Гра вже йде! Вгадуй країну!")
	}
	flagGameMu.Unlock()

	loadMapCountries()
	if len(mapCountries) == 0 {
		return c.Reply("❌ Не вдалося завантажити країни")
	}

	country := mapCountries[rand.Intn(len(mapCountries))]

	// Get 2-letter code from aliases (last one is cca2)
	code := ""
	for _, a := range country.Aliases {
		if len(a) == 2 {
			code = a
			break
		}
	}
	if code == "" {
		return c.Reply("❌ Код країни не знайдений")
	}

	flagURL := fmt.Sprintf("https://flagcdn.com/w640/%s.png", strings.ToLower(code))

	gameID := time.Now().UnixNano()

	flagGameMu.Lock()
	activeFlagGame[chatID] = &flagGame{
		ID:        gameID,
		Country:   country.UkName,
		ChatID:    chatID,
		Aliases:   country.Aliases,
		CreatedAt: time.Now(),
	}
	flagGameMu.Unlock()

	photo := &tele.Photo{
		File:    tele.FromURL(flagURL),
		Caption: "🏳️ Чий це прапор? (30 сек)\nНагорода: +15 🪙",
	}
	sent, err := c.Bot().Send(c.Chat(), photo)
	if err != nil {
		flagGameMu.Lock()
		delete(activeFlagGame, chatID)
		flagGameMu.Unlock()
		return c.Reply("❌ Не вдалося відправити прапор")
	}

	flagGameMu.Lock()
	if g, ok := activeFlagGame[chatID]; ok {
		g.SentMsg = sent
	}
	flagGameMu.Unlock()

	bot := c.Bot()
	cmdMsg := c.Message()

	go func() {
		time.Sleep(30 * time.Second)
		flagGameMu.Lock()
		game, ok := activeFlagGame[chatID]
		if ok && game.Winner == "" && game.ID == gameID {
			sentMsg := game.SentMsg
			name := game.Country
			delete(activeFlagGame, chatID)
			flagGameMu.Unlock()
			if sentMsg != nil {
				bot.Delete(sentMsg)
			}
			if cmdMsg != nil {
				bot.Delete(cmdMsg)
			}
			msg, _ := bot.Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🏳️ Час вийшов! Це було: %s", name))
			autoDelete(bot, 5*time.Second, msg)
		} else {
			flagGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkFlagAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	flagGameMu.Lock()
	game, ok := activeFlagGame[chatID]
	if !ok {
		flagGameMu.Unlock()
		return false
	}

	correct := false
	for _, alias := range game.Aliases {
		if strings.Contains(text, alias) {
			correct = true
			break
		}
	}

	if !correct {
		flagGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeFlagGame, chatID)
	flagGameMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "flag", reward)

	c.Reply(fmt.Sprintf("🏳️ %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Country, reward, newBal))
	return true
}
