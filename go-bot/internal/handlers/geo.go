package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type geoGame struct {
	Country   string
	ChatID    int64
	Aliases   []string
	Winner    string
	SentMsg   *tele.Message
	CmdMsg    *tele.Message
	CreatedAt time.Time
}

var (
	activeGeo = make(map[int64]*geoGame)
	geoMu     sync.Mutex
)

// Fetched dynamically from RestCountries API on first use
var allCountries []struct {
	Query   string
	Name    string
	Aliases []string
}

func loadCountries() {
	if len(allCountries) > 0 {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://restcountries.com/v3.1/all?fields=name,translations,cca2")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var countries []struct {
		Name struct {
			Common string `json:"common"`
		} `json:"name"`
		Translations map[string]struct {
			Common string `json:"common"`
		} `json:"translations"`
		CCA2 string `json:"cca2"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&countries); err != nil {
		return
	}

	for _, c := range countries {
		ukName := c.Name.Common
		if t, ok := c.Translations["ukr"]; ok && t.Common != "" {
			ukName = t.Common
		}

		aliases := []string{
			strings.ToLower(ukName),
			strings.ToLower(c.Name.Common),
			strings.ToLower(c.CCA2),
		}
		// Add without diacritics for common misspellings
		simplified := strings.ToLower(c.Name.Common)
		if simplified != strings.ToLower(ukName) {
			aliases = append(aliases, simplified)
		}

		allCountries = append(allCountries, struct {
			Query   string
			Name    string
			Aliases []string
		}{
			Query:   c.Name.Common,
			Name:    ukName,
			Aliases: aliases,
		})
	}
}

const maxGeoPerHour = 10

func (b *Bot) handleGeo(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID
	hour := nowHourKyiv()

	geoKey := "geo:" + userID + ":" + hour
	countStr := b.db.GetMeta(geoKey)
	geoCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &geoCount)
	}
	if geoCount >= maxGeoPerHour {
		return c.Reply(fmt.Sprintf("🌍 Ліміт %d на годину. Через %s", maxGeoPerHour, timeUntilNextHour()))
	}

	geoMu.Lock()
	if g, ok := activeGeo[chatID]; ok && time.Since(g.CreatedAt) < 20*time.Second {
		geoMu.Unlock()
		return c.Reply("🌍 Гра вже йде! Вгадуй країну!")
	}
	geoMu.Unlock()

	// Load countries on first use
	loadCountries()
	if len(allCountries) == 0 {
		return c.Reply("❌ Не вдалося завантажити країни")
	}

	// Pick random country
	country := allCountries[rand.Intn(len(allCountries))]

	// Fetch photo from Unsplash
	unsplashKey := os.Getenv("UNSPLASH_ACCESS_KEY")
	if unsplashKey == "" {
		return c.Reply("❌ Unsplash API не налаштований")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.unsplash.com/photos/random?query=%s+landscape+city&orientation=landscape&client_id=%s",
		strings.ReplaceAll(country.Query, " ", "+"), unsplashKey)

	resp, err := client.Get(url)
	if err != nil {
		return c.Reply("❌ Не вдалося отримати фото")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return c.Reply("❌ Unsplash API помилка")
	}

	var photo struct {
		URLs struct {
			Regular string `json:"regular"`
		} `json:"urls"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&photo); err != nil || photo.URLs.Regular == "" {
		return c.Reply("❌ Фото не знайдено")
	}

	// Increment hourly count
	b.db.SetMeta(geoKey, fmt.Sprintf("%d", geoCount+1))

	// Set active game
	geoMu.Lock()
	activeGeo[chatID] = &geoGame{
		Country:   country.Name,
		ChatID:    chatID,
		Aliases:   country.Aliases,
		CmdMsg:    c.Message(),
		CreatedAt: time.Now(),
	}
	geoMu.Unlock()

	// Send photo
	telePhoto := &tele.Photo{
		File:    tele.FromURL(photo.URLs.Regular),
		Caption: "🌍 Де це? Напиши назву країни! (20 сек)\nНагорода: +15 🪙",
	}
	sent, _ := c.Bot().Send(c.Chat(), telePhoto)

	geoMu.Lock()
	if g, ok := activeGeo[chatID]; ok {
		g.SentMsg = sent
	}
	geoMu.Unlock()

	// Auto-close after 20 seconds
	go func() {
		time.Sleep(20 * time.Second)
		geoMu.Lock()
		game, ok := activeGeo[chatID]
		if ok && game.Winner == "" {
			sentMsg := game.SentMsg
			cmdMsg := game.CmdMsg
			name := game.Country
			delete(activeGeo, chatID)
			geoMu.Unlock()
			if sentMsg != nil {
				c.Bot().Delete(sentMsg)
			}
			if cmdMsg != nil {
				c.Bot().Delete(cmdMsg)
			}
			msg, _ := c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🌍 Час вийшов! Це було: %s", name))
			autoDelete(c.Bot(), 5*time.Second, msg)
		} else {
			geoMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkGeoAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	geoMu.Lock()
	game, ok := activeGeo[chatID]
	if !ok {
		geoMu.Unlock()
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
		geoMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeGeo, chatID)
	geoMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "geo", reward)

	c.Reply(fmt.Sprintf("🎉 %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Country, reward, newBal))
	return true
}
