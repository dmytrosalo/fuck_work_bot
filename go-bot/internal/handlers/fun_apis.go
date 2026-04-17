package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
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

// /cat — cat meme with optional text overlay
func (b *Bot) handleCat(c tele.Context) error {
	text := strings.TrimSpace(c.Message().Payload)

	var imageURL string
	if text != "" {
		// Cat with text overlay
		encoded := url.PathEscape(text)
		imageURL = fmt.Sprintf("https://cataas.com/cat/says/%s?fontSize=40&fontColor=white&type=square", encoded)
	} else {
		// Random cat meme with random funny text
		funnyTexts := []string{
			"я на стендапі",
			"деплой в п'ятницю",
			"код рев'ю",
			"баг на проді",
			"мітинг що міг бути емейлом",
			"дедлайн вчора",
			"force push",
			"merge conflict",
			"ще 5 хвилинок",
			"працюю з дому",
			"я не сплю я думаю",
			"ще один спринт",
			"все зламалось",
			"хто це написав? а це я",
			"production is fine",
			"я на мʼюті",
			"тест не пройшов",
			"знову рефакторинг",
			"не чіпай продакшн",
			"hotfix о 3 ночі",
		}
		randomText := funnyTexts[rand.Intn(len(funnyTexts))]
		encoded := url.PathEscape(randomText)
		imageURL = fmt.Sprintf("https://cataas.com/cat/says/%s?fontSize=40&fontColor=white&type=square", encoded)
	}

	photo := &tele.Photo{
		File: tele.FromURL(imageURL),
	}
	if text != "" {
		photo.Caption = fmt.Sprintf("🐱 %s", text)
	}

	err := c.Send(photo)
	if err != nil {
		// Fallback — just send a cat without text
		photo2 := &tele.Photo{
			File:    tele.FromURL("https://cataas.com/cat"),
			Caption: "🐱 Кіт прийшов без тексту, але з настроєм",
		}
		return c.Send(photo2)
	}
	return nil
}
