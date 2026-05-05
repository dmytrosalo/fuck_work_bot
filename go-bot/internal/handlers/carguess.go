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

type carGame struct {
	Brand     string
	ChatID    int64
	Winner    string
	CreatedAt time.Time
}

var (
	activeCarGame = make(map[int64]*carGame)
	carGameMu     sync.Mutex
)

var carBrands = []struct {
	Query   string
	Name    string
	Aliases []string
}{
	{"BMW car", "BMW", []string{"bmw", "бмв", "бемведе"}},
	{"Mercedes Benz car", "Mercedes", []string{"mercedes", "мерседес", "мерс"}},
	{"Audi car", "Audi", []string{"audi", "ауді", "ауди"}},
	{"Porsche car", "Porsche", []string{"porsche", "порше", "порш"}},
	{"Tesla car", "Tesla", []string{"tesla", "тесла"}},
	{"Toyota car", "Toyota", []string{"toyota", "тойота"}},
	{"Honda car", "Honda", []string{"honda", "хонда"}},
	{"Volkswagen car", "Volkswagen", []string{"volkswagen", "vw", "фольксваген", "фольц"}},
	{"Ford car", "Ford", []string{"ford", "форд"}},
	{"Chevrolet car", "Chevrolet", []string{"chevrolet", "chevy", "шевроле"}},
	{"Lamborghini car", "Lamborghini", []string{"lamborghini", "ламборгіні", "ламборджіні", "ламбо"}},
	{"Ferrari car", "Ferrari", []string{"ferrari", "феррарі", "ферарі"}},
	{"Bugatti car", "Bugatti", []string{"bugatti", "бугатті", "бугаті"}},
	{"Rolls Royce car", "Rolls-Royce", []string{"rolls-royce", "rolls royce", "ролс-ройс", "ролс ройс"}},
	{"Nissan car", "Nissan", []string{"nissan", "ніссан", "нісан"}},
	{"Mazda car", "Mazda", []string{"mazda", "мазда"}},
	{"Subaru car", "Subaru", []string{"subaru", "субару"}},
	{"Volvo car", "Volvo", []string{"volvo", "вольво"}},
	{"Jeep car", "Jeep", []string{"jeep", "джип"}},
	{"Land Rover car", "Land Rover", []string{"land rover", "ленд ровер", "range rover", "рендж ровер"}},
	{"Hyundai car", "Hyundai", []string{"hyundai", "хюндай", "хундай", "хьюндай"}},
	{"Kia car", "Kia", []string{"kia", "кіа", "кіа"}},
	{"Dodge car", "Dodge", []string{"dodge", "додж"}},
	{"Jaguar car", "Jaguar", []string{"jaguar", "ягуар"}},
	{"Maserati car", "Maserati", []string{"maserati", "мазераті"}},
	{"Bentley car", "Bentley", []string{"bentley", "бентлі"}},
	{"Aston Martin car", "Aston Martin", []string{"aston martin", "астон мартін", "астон"}},
	{"McLaren car", "McLaren", []string{"mclaren", "макларен"}},
	{"Fiat car", "Fiat", []string{"fiat", "фіат"}},
	{"Peugeot car", "Peugeot", []string{"peugeot", "пежо"}},
	{"Renault car", "Renault", []string{"renault", "рено"}},
	{"Citroen car", "Citroen", []string{"citroen", "сітроен"}},
	{"Skoda car", "Skoda", []string{"skoda", "шкода"}},
	{"Mitsubishi car", "Mitsubishi", []string{"mitsubishi", "мітсубіші", "мітсубісі"}},
	{"Lexus car", "Lexus", []string{"lexus", "лексус"}},
	{"Infiniti car", "Infiniti", []string{"infiniti", "інфініті"}},
	{"BYD car", "BYD", []string{"byd", "бід"}},
	{"Mini Cooper car", "Mini", []string{"mini", "міні", "mini cooper"}},
	{"Alfa Romeo car", "Alfa Romeo", []string{"alfa romeo", "альфа ромео", "альфа"}},
	{"Suzuki car", "Suzuki", []string{"suzuki", "сузукі"}},
}

const maxCarGuessPerHour = 20

func (b *Bot) handleCarGuess(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID
	hour := nowHourKyiv()

	// Hourly limit
	carKey := "carguess:" + userID + ":" + hour
	countStr := b.db.GetMeta(carKey)
	carCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &carCount)
	}
	if carCount >= maxCarGuessPerHour {
		return c.Reply(fmt.Sprintf("🚗 Ліміт %d на годину. Через %s", maxCarGuessPerHour, timeUntilNextHour()))
	}

	carGameMu.Lock()
	if g, ok := activeCarGame[chatID]; ok && time.Since(g.CreatedAt) < 60*time.Second {
		carGameMu.Unlock()
		return c.Reply("🚗 Гра вже йде! Вгадуй марку!")
	}
	carGameMu.Unlock()

	// Pick random car brand
	brand := carBrands[rand.Intn(len(carBrands))]

	// Fetch photo from Unsplash
	unsplashKey := os.Getenv("UNSPLASH_ACCESS_KEY")
	if unsplashKey == "" {
		return c.Reply("❌ Unsplash API не налаштований")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.unsplash.com/photos/random?query=%s&orientation=landscape&client_id=%s",
		strings.ReplaceAll(brand.Query, " ", "+"), unsplashKey)

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
	b.db.SetMeta(carKey, fmt.Sprintf("%d", carCount+1))

	// Set active game
	carGameMu.Lock()
	activeCarGame[chatID] = &carGame{
		Brand:     brand.Name,
		ChatID:    chatID,
		CreatedAt: time.Now(),
	}
	carGameMu.Unlock()

	// Send photo
	telePhoto := &tele.Photo{
		File:    tele.FromURL(photo.URLs.Regular),
		Caption: "🚗 Що за марка? Напиши! (20 сек)\nНагорода: +25 🪙",
	}
	sent, _ := c.Bot().Send(c.Chat(), telePhoto)

	// Auto-close after 60 seconds
	go func() {
		time.Sleep(20 * time.Second)
		carGameMu.Lock()
		game, ok := activeCarGame[chatID]
		if ok && game.Winner == "" {
			delete(activeCarGame, chatID)
			carGameMu.Unlock()
			if sent != nil {
				c.Bot().Delete(sent)
			}
			c.Bot().Delete(c.Message())
			c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🚗 Час вийшов! Це було: %s", brand.Name))
		} else {
			carGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkCarAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	carGameMu.Lock()
	game, ok := activeCarGame[chatID]
	if !ok {
		carGameMu.Unlock()
		return false
	}

	var correct bool
	for _, brand := range carBrands {
		if brand.Name == game.Brand {
			for _, alias := range brand.Aliases {
				if strings.Contains(text, alias) {
					correct = true
					break
				}
			}
			break
		}
	}

	if !correct {
		carGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeCarGame, chatID)
	carGameMu.Unlock()

	reward := 25
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "carguess", reward)

	c.Reply(fmt.Sprintf("🏎️ %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Brand, reward, newBal))
	return true
}
