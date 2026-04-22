package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	// One-time reset all daily limits
	resetKey := "reset_all_limits_v1"
	if db.GetMeta(resetKey) == "" {
		db.ClearDailyLimits()
		db.SetMeta(resetKey, "done")
		log.Println("Reset all daily limits")
	}

	bonusKey3 := "bonus_danyro_666"
	if db.GetMeta(bonusKey3) == "" {
		if danyaID, found := db.FindUserByName("Danya"); found {
			db.UpdateBalance(danyaID, "Danya", 666)
			db.SetMeta(bonusKey3, "done")
			log.Printf("Gave Danya +666 coins")
		}
	}
	bonusKey4 := "bonus_danyro_46"
	if db.GetMeta(bonusKey4) == "" {
		if danyaID, found := db.FindUserByName("Danya"); found {
			db.UpdateBalance(danyaID, "Danya", 46)
			db.SetMeta(bonusKey4, "done")
			log.Printf("Gave Danya +46 coins")
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

	// One-time legendary card gifts
	rarityStarsMap := map[int]string{
		1: "⭐", 2: "⭐⭐", 3: "⭐⭐⭐", 4: "⭐⭐⭐⭐", 5: "⭐⭐⭐⭐⭐", 6: "💎💎💎💎💎💎",
	}
	rarityNamesMap := map[int]string{
		1: "Common", 2: "Uncommon", 3: "Rare", 4: "Epic", 5: "Legendary", 6: "Ultra Legendary",
	}
	type cardGift struct {
		Key      string
		UserName string
		CardID   int
		CardName string
		Rarity   int
	}
	cardGifts := []cardGift{
		{"gift_bo_kercher", "Bo", 602, "Фотка з Керхер", 5},
		{"gift_danya_rain", "Danya", 603, "Дощ після 3-ох фазної мийки", 5},
		{"gift_data_emerald", "Data", 604, "Смарагдове небо", 5},
		{"gift_bo_zhmykh", "Bo", 605, "Жмих", 1},
		{"gift_bo_melisa", "Bo", 606, "Меліса", 2},
		{"gift_danya_46", "Danya", 607, "Ті самі 46 баксів", 2},
		{"gift_bo_terpila", "Bo", 608, "Тєрпіла", 2},
		{"gift_danya_terpila", "Danya", 608, "Тєрпіла", 2},
		{"gift_data_terpila", "Data", 608, "Тєрпіла", 2},
		{"gift_dmytro_terpila", "Dmytro", 608, "Тєрпіла", 2},
		{"gift_danya_chikuha", "Danya", 609, "Чікуха бояри", 3},
		{"gift_danya_zlyden", "Danya", 610, "Злидень", 1},
		{"gift_danya_usman", "Danya", 611, "Фанат Усмана", 1},
	}
	var giftMessages []string
	for _, g := range cardGifts {
		if db.GetMeta(g.Key) == "" {
			if uid, found := db.FindUserByName(g.UserName); found {
				db.AddToCollection(uid, g.CardID)
				db.SetMeta(g.Key, "done")
				stars := rarityStarsMap[g.Rarity]
				rName := rarityNamesMap[g.Rarity]
				giftMessages = append(giftMessages, fmt.Sprintf("%s %s отримує %s картку: *%s*!", stars, g.UserName, rName, g.CardName))
				log.Printf("Gifted %s card #%d (%s)", g.UserName, g.CardID, g.CardName)
			}
		}
	}

	// Register handlers
	h := handlers.New(clf, db)
	h.Register(bot)

	// Start web server
	mux := http.NewServeMux()
	handlers.RegisterWeb(mux, db)
	go func() {
		log.Println("Web server starting on :8080...")
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()

	// Send card gift announcements to all chats
	if len(giftMessages) > 0 {
		go func() {
			time.Sleep(3 * time.Second) // wait for bot to connect
			chats, err := db.GetActiveChats()
			if err != nil {
				return
			}
			msg := "🎉 *Нові легендарні картки!*\n\n" + strings.Join(giftMessages, "\n")
			for _, chatID := range chats {
				id, err := strconv.ParseInt(chatID, 10, 64)
				if err != nil {
					continue
				}
				bot.Send(&tele.Chat{ID: id}, msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
			}
		}()
	}

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
