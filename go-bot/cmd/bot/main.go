package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/classifier"
	"github.com/dmytrosalo/fuck-work-bot/internal/handlers"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	modelPath := os.Getenv("MODEL_PATH")
	if modelPath == "" {
		modelPath = "./model/tfidf_model.json"
	}

	// Init storage
	db, err := storage.New(dataDir + "/bot.db")
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	defer db.Close()
	log.Println("Storage initialized")

	// Seed DB — version-based, re-seeds when seed data changes
	seedPath := os.Getenv("SEED_PATH")
	if seedPath == "" {
		seedPath = "./seed_data.json"
	}
	if seedData, err := os.ReadFile(seedPath); err == nil {
		// Compute simple version from data length
		seedVersion := fmt.Sprintf("v%d", len(seedData))
		currentVersion := db.GetMeta("seed_version")

		if currentVersion != seedVersion {
			log.Printf("Seed version changed (%s -> %s), re-seeding...", currentVersion, seedVersion)
			seedAll(db, seedData)
			db.SetMeta("seed_version", seedVersion)
			log.Printf("Seeded: %d cards, %d quotes", db.CardCount(), db.QuoteCount())
		} else {
			log.Printf("Seed up to date (%s)", seedVersion)
		}
	}

	// One-time bonuses
	bonusKey1 := "bonus_danyro_1000"
	if db.GetMeta(bonusKey1) == "" {
		if danyaID, found := db.FindUserByName("Danya"); found {
			db.UpdateBalance(danyaID, "Danya", 1000)
			db.SetMeta(bonusKey1, "done")
		}
	}
	bonusKey2 := "bonus_danyro_1234"
	if db.GetMeta(bonusKey2) == "" {
		if danyaID, found := db.FindUserByName("Danya"); found {
			db.UpdateBalance(danyaID, "Danya", 1234)
			db.SetMeta(bonusKey2, "done")
		}
	}
	bonusKey3 := "bonus_danyro_666"
	if db.GetMeta(bonusKey3) == "" {
		if danyaID, found := db.FindUserByName("Danya"); found {
			db.UpdateBalance(danyaID, "Danya", 666)
			db.SetMeta(bonusKey3, "done")
			log.Printf("Gave Danya +666 coins")
		}
	}

	// Init classifier
	clf, err := classifier.New(modelPath)
	if err != nil {
		log.Fatalf("Failed to init classifier: %v", err)
	}
	defer clf.Close()
	log.Println("Classifier loaded")

	// Init bot
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Register handlers
	h := handlers.New(clf, db)
	h.Register(bot)

	// Schedule daily report at 23:00 Kyiv time
	go scheduleDailyReport(bot, h)

	log.Println("Bot starting...")
	bot.Start()
}

func seedAll(db *storage.DB, data []byte) {
	var seed struct {
		Roasts []struct {
			Category string `json:"category"`
			Target   string `json:"target"`
			Text     string `json:"text"`
		} `json:"roasts"`
		Compliments []struct {
			Target string `json:"target"`
			Text   string `json:"text"`
		} `json:"compliments"`
		Quotes []struct {
			Author string `json:"author"`
			Text   string `json:"text"`
		} `json:"quotes"`
		Cards []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Rarity      int    `json:"rarity"`
			Category    string `json:"category"`
			Emoji       string `json:"emoji"`
			Description string `json:"description"`
			Stats       struct {
				ATK         int    `json:"atk"`
				DEF         int    `json:"def"`
				SpecialName string `json:"special_name"`
				Special     int    `json:"special"`
			} `json:"stats"`
		} `json:"cards"`
	}
	if err := json.Unmarshal(data, &seed); err != nil {
		log.Printf("Failed to parse seed data: %v", err)
		return
	}

	// Clear and re-seed everything
	db.ClearQuotes()
	db.ClearCards()

	for _, r := range seed.Roasts {
		db.AddRoast(r.Category, r.Target, r.Text)
	}
	for _, c := range seed.Compliments {
		db.AddCompliment(c.Target, c.Text)
	}
	for _, q := range seed.Quotes {
		db.AddQuote(q.Author, q.Text)
	}
	for _, card := range seed.Cards {
		db.AddCard(card.ID, card.Name, card.Rarity, card.Category, card.Emoji, card.Description,
			card.Stats.ATK, card.Stats.DEF, card.Stats.SpecialName, card.Stats.Special)
	}
}

func scheduleDailyReport(bot *tele.Bot, h *handlers.Bot) {
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	if err != nil {
		log.Printf("Failed to load Kyiv timezone: %v, using UTC+2", err)
		kyiv = time.FixedZone("Kyiv", 2*60*60)
	}

	for {
		now := time.Now().In(kyiv)
		next := time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, kyiv)
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		sleepDuration := time.Until(next)
		log.Printf("Next daily report in %v (at %s)", sleepDuration.Round(time.Minute), next.Format("15:04 MST"))

		time.Sleep(sleepDuration)

		log.Println("Sending daily report...")
		h.DailyReport(bot)
	}
}
