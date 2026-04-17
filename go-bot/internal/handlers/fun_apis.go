package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
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

var catPhrases = []string{
	"на стендапі",
	"деплоїть в п'ятницю",
	"на код рев'ю",
	"зламав прод",
	"на мітингу що міг бути емейлом",
	"дедлайн був вчора",
	"зробив force push",
	"в merge conflict",
	"ще 5 хвилинок і все",
	"працює з дому",
	"не спить а думає",
	"в ще одному спринті",
	"все зламав",
	"написав цей код",
	"каже production is fine",
	"на мʼюті",
	"тест не пройшов",
	"знову рефакторить",
	"чіпає продакшн",
	"робить hotfix о 3 ночі",
	"їсть хінкалі",
	"грає в слоти",
	"мріє про Порше",
	"ігнорує повідомлення",
	"каже харош!",
	"шукає баг",
	"дивиться на Jira",
	"пише в Slack о суботі",
	"каже ще трохи і все",
	"оновлює резюме",
}

// /cat or /cat @username — cat meme with user name + random phrase
func (b *Bot) handleCat(c tele.Context) error {
	// Get target name
	targetName, _ := getTarget(c)
	phrase := catPhrases[rand.Intn(len(catPhrases))]
	text := fmt.Sprintf("%s %s", targetName, phrase)

	encoded := url.PathEscape(text)
	imageURL := fmt.Sprintf("https://cataas.com/cat/says/%s?fontSize=35&fontColor=white&type=square", encoded)

	photo := &tele.Photo{
		File:    tele.FromURL(imageURL),
		Caption: fmt.Sprintf("🐱 %s", text),
	}

	err := c.Send(photo)
	if err != nil {
		photo2 := &tele.Photo{
			File:    tele.FromURL("https://cataas.com/cat"),
			Caption: fmt.Sprintf("🐱 %s %s", targetName, phrase),
		}
		return c.Send(photo2)
	}
	return nil
}
