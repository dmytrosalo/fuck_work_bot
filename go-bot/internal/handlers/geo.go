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
	Players   map[string]bool
	Winner    string
	CreatedAt time.Time
}

var (
	activeGeo = make(map[int64]*geoGame)
	geoMu     sync.Mutex
)

// Countries with Ukrainian names
var geoCountries = []struct {
	Query string // search term for Unsplash
	Name  string // correct answer in Ukrainian
	Aliases []string // alternative accepted answers
}{
	{"Paris France", "Франція", []string{"франція", "france"}},
	{"Tokyo Japan", "Японія", []string{"японія", "japan"}},
	{"New York USA", "США", []string{"сша", "usa", "америка", "united states"}},
	{"London England", "Великобританія", []string{"великобританія", "англія", "uk", "england", "britain"}},
	{"Rome Italy", "Італія", []string{"італія", "italy"}},
	{"Barcelona Spain", "Іспанія", []string{"іспанія", "spain"}},
	{"Berlin Germany", "Німеччина", []string{"німеччина", "germany"}},
	{"Amsterdam Netherlands", "Нідерланди", []string{"нідерланди", "голландія", "netherlands", "holland"}},
	{"Sydney Australia", "Австралія", []string{"австралія", "australia"}},
	{"Rio de Janeiro Brazil", "Бразилія", []string{"бразилія", "brazil"}},
	{"Cairo Egypt", "Єгипет", []string{"єгипет", "egypt"}},
	{"Istanbul Turkey", "Туреччина", []string{"туреччина", "turkey", "türkiye"}},
	{"Bangkok Thailand", "Таїланд", []string{"таїланд", "thailand"}},
	{"Dubai UAE", "ОАЕ", []string{"оае", "uae", "дубай", "dubai"}},
	{"Moscow Russia", "Росія", []string{"росія", "russia"}},
	{"Beijing China", "Китай", []string{"китай", "china"}},
	{"Seoul South Korea", "Південна Корея", []string{"корея", "південна корея", "south korea", "korea"}},
	{"Mumbai India", "Індія", []string{"індія", "india"}},
	{"Mexico City Mexico", "Мексика", []string{"мексика", "mexico"}},
	{"Buenos Aires Argentina", "Аргентина", []string{"аргентина", "argentina"}},
	{"Lisbon Portugal", "Португалія", []string{"португалія", "portugal"}},
	{"Athens Greece", "Греція", []string{"греція", "greece"}},
	{"Prague Czech Republic", "Чехія", []string{"чехія", "czech", "czechia"}},
	{"Vienna Austria", "Австрія", []string{"австрія", "austria"}},
	{"Warsaw Poland", "Польща", []string{"польща", "poland"}},
	{"Stockholm Sweden", "Швеція", []string{"швеція", "sweden"}},
	{"Oslo Norway", "Норвегія", []string{"норвегія", "norway"}},
	{"Helsinki Finland", "Фінляндія", []string{"фінляндія", "finland"}},
	{"Copenhagen Denmark", "Данія", []string{"данія", "denmark"}},
	{"Zurich Switzerland", "Швейцарія", []string{"швейцарія", "switzerland"}},
	{"Kyiv Ukraine", "Україна", []string{"україна", "ukraine"}},
	{"Havana Cuba", "Куба", []string{"куба", "cuba"}},
	{"Marrakech Morocco", "Марокко", []string{"марокко", "morocco"}},
	{"Nairobi Kenya", "Кенія", []string{"кенія", "kenya"}},
	{"Cape Town South Africa", "Південна Африка", []string{"пар", "південна африка", "south africa"}},
	{"Singapore", "Сінгапур", []string{"сінгапур", "singapore"}},
	{"Bali Indonesia", "Індонезія", []string{"індонезія", "indonesia", "балі"}},
	{"Santorini Greece", "Греція", []string{"греція", "greece", "санторіні"}},
	{"Machu Picchu Peru", "Перу", []string{"перу", "peru"}},
	{"Reykjavik Iceland", "Ісландія", []string{"ісландія", "iceland"}},
}

const maxGeoPerHour = 20

func (b *Bot) handleGeo(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID
	hour := nowHourKyiv()

	// Hourly limit
	geoKey := "geo:" + userID + ":" + hour
	countStr := b.db.GetMeta(geoKey)
	geoCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &geoCount)
	}
	if geoCount >= maxGeoPerHour {
		return c.Reply(fmt.Sprintf("🌍 Ліміт %d гео на годину. Через %s", maxGeoPerHour, timeUntilNextHour()))
	}

	geoMu.Lock()
	if g, ok := activeGeo[chatID]; ok && time.Since(g.CreatedAt) < 60*time.Second {
		geoMu.Unlock()
		return c.Reply("🌍 Гра вже йде! Вгадуй країну!")
	}
	geoMu.Unlock()

	// Pick random country
	country := geoCountries[rand.Intn(len(geoCountries))]

	// Fetch photo from Unsplash
	unsplashKey := os.Getenv("UNSPLASH_ACCESS_KEY")
	if unsplashKey == "" {
		return c.Reply("❌ Unsplash API не налаштований")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.unsplash.com/photos/random?query=%s&orientation=landscape&client_id=%s",
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

	// Increment daily count
	b.db.SetMeta(geoKey, fmt.Sprintf("%d", geoCount+1))

	// Set active game
	geoMu.Lock()
	activeGeo[chatID] = &geoGame{
		Country:   country.Name,
		ChatID:    chatID,
		Players:   make(map[string]bool),
		CreatedAt: time.Now(),
	}
	// Store aliases for checking
	activeGeo[chatID].Players["__aliases__"] = false // hack: store aliases in country field
	geoMu.Unlock()

	// Send photo
	telePhoto := &tele.Photo{
		File:    tele.FromURL(photo.URLs.Regular),
		Caption: "🌍 Де це? Напиши назву країни! (20 сек)\nНагорода: +30 🪙",
	}
	sent, _ := c.Bot().Send(c.Chat(), telePhoto)

	// Store sent message for deletion
	geoMu.Lock()
	if g, ok := activeGeo[chatID]; ok {
		g.Players["__msg_id__"] = false // placeholder
	}
	geoMu.Unlock()

	// Auto-close after 60 seconds
	go func() {
		time.Sleep(20 * time.Second)
		geoMu.Lock()
		game, ok := activeGeo[chatID]
		if ok && game.Winner == "" {
			delete(activeGeo, chatID)
			geoMu.Unlock()
			// Delete photo and command
			if sent != nil {
				c.Bot().Delete(sent)
			}
			c.Bot().Delete(c.Message())
			c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🌍 Час вийшов! Це було: %s", country.Name))
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

	// Find the country in our list to check aliases
	var correct bool
	for _, country := range geoCountries {
		if country.Name == game.Country {
			for _, alias := range country.Aliases {
				if strings.Contains(text, alias) {
					correct = true
					break
				}
			}
			break
		}
	}

	if !correct {
		geoMu.Unlock()
		return false
	}

	// Winner!
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeGeo, chatID)
	geoMu.Unlock()

	reward := 30
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "geo", reward)

	c.Reply(fmt.Sprintf("🎉 %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Country, reward, newBal))
	return true
}
