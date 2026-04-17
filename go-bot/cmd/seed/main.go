package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
)

type seedData struct {
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

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "./testdata"
	}

	seedFile := "seed_data.json"
	if len(os.Args) > 1 {
		seedFile = os.Args[1]
	}

	// Open DB
	db, err := storage.New(dataDir + "/bot.db")
	if err != nil {
		log.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Read seed data
	raw, err := os.ReadFile(seedFile)
	if err != nil {
		log.Fatalf("Failed to read %s: %v", seedFile, err)
	}

	var data seedData
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Fatalf("Failed to parse seed data: %v", err)
	}

	// Seed roasts
	for _, r := range data.Roasts {
		db.AddRoast(r.Category, r.Target, r.Text)
	}
	fmt.Printf("Seeded %d roasts\n", len(data.Roasts))

	// Seed compliments
	for _, c := range data.Compliments {
		db.AddCompliment(c.Target, c.Text)
	}
	fmt.Printf("Seeded %d compliments\n", len(data.Compliments))

	// Seed quotes
	for _, q := range data.Quotes {
		db.AddQuote(q.Author, q.Text)
	}
	fmt.Printf("Seeded %d quotes\n", len(data.Quotes))

	fmt.Println("Done!")
}
