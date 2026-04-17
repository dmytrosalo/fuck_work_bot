package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	tele "gopkg.in/telebot.v3"
)

func decodeJSON(resp *http.Response, v interface{}) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

// /dog — random dog photo "який ти песик сьогодні"
func (b *Bot) handleDog(c tele.Context) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://dog.ceo/api/breeds/image/random")
	if err != nil {
		return c.Reply("❌ Песики сплять, спробуй пізніше")
	}
	defer resp.Body.Close()

	var result struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(resp, &result); err != nil || result.Message == "" {
		return c.Reply("❌ Песик втік")
	}

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	photo := &tele.Photo{
		File:    tele.FromURL(result.Message),
		Caption: fmt.Sprintf("🐕 %s, це твій песик сьогодні!", userName),
	}
	return c.Send(photo)
}

// /cat or /cat @username — random cat photo for user
func (b *Bot) handleCat(c tele.Context) error {
	targetName, _ := getTarget(c)

	// Use thecatapi.com (free, no auth needed)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.thecatapi.com/v1/images/search")
	if err != nil {
		return c.Reply("❌ Котики сплять")
	}
	defer resp.Body.Close()

	var results []struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(resp, &results); err != nil || len(results) == 0 {
		return c.Reply("❌ Кіт втік")
	}

	photo := &tele.Photo{
		File:    tele.FromURL(results[0].URL),
		Caption: fmt.Sprintf("🐱 %s, це твій кіт", targetName),
	}
	return c.Send(photo)
}
