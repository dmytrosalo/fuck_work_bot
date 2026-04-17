package main

import (
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
