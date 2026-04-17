package main

import (
	"encoding/json"
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

	// Seed DB if empty
	if !db.HasContent() {
		seedPath := os.Getenv("SEED_PATH")
		if seedPath == "" {
			seedPath = "./seed_data.json"
		}
		if seedData, err := os.ReadFile(seedPath); err == nil {
			seedDB(db, seedData)
			log.Println("Database seeded with initial data")
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

func seedDB(db *storage.DB, data []byte) {
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
	}
	if err := json.Unmarshal(data, &seed); err != nil {
		log.Printf("Failed to parse seed data: %v", err)
		return
	}
	for _, r := range seed.Roasts {
		db.AddRoast(r.Category, r.Target, r.Text)
	}
	for _, c := range seed.Compliments {
		db.AddCompliment(c.Target, c.Text)
	}
	for _, q := range seed.Quotes {
		db.AddQuote(q.Author, q.Text)
	}
	log.Printf("Seeded %d roasts, %d compliments, %d quotes", len(seed.Roasts), len(seed.Compliments), len(seed.Quotes))
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
