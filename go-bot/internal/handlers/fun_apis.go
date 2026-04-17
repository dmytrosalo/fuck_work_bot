package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// /cat or /cat @username — cat meme with user name
func (b *Bot) handleCat(c tele.Context) error {
	targetName, _ := getTarget(c)

	encoded := url.PathEscape(targetName)
	imageURL := fmt.Sprintf("https://cataas.com/cat/says/%s?fontSize=40&fontColor=white&type=square", encoded)

	photo := &tele.Photo{
		File:    tele.FromURL(imageURL),
		Caption: fmt.Sprintf("🐱 %s", targetName),
	}

	err := c.Send(photo)
	if err != nil {
		photo2 := &tele.Photo{
			File:    tele.FromURL("https://cataas.com/cat"),
			Caption: fmt.Sprintf("🐱 %s", targetName),
		}
		return c.Send(photo2)
	}
	return nil
}
