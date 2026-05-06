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

type movieGame struct {
	ID        int64
	Title     string
	ChatID    int64
	Aliases   []string
	Winner    string
	SentMsg   *tele.Message
	CreatedAt time.Time
}

var (
	activeMovieGame = make(map[int64]*movieGame)
	movieGameMu     sync.Mutex
)

type tmdbMovie struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	OrigTitle   string `json:"original_title"`
	PosterPath  string `json:"poster_path"`
	ReleaseDate string `json:"release_date"`
}

func (b *Bot) handleMovieGuess(c tele.Context) error {
	chatID := c.Chat().ID

	movieGameMu.Lock()
	if g, ok := activeMovieGame[chatID]; ok && time.Since(g.CreatedAt) < 30*time.Second {
		movieGameMu.Unlock()
		return c.Reply("🎬 Гра вже йде! Вгадуй фільм!")
	}
	movieGameMu.Unlock()

	tmdbKey := os.Getenv("TMDB_API_KEY")
	if tmdbKey == "" {
		return c.Reply("❌ TMDB API не налаштований")
	}

	// Fetch popular movies (random page 1-20)
	page := rand.Intn(20) + 1
	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.themoviedb.org/3/movie/popular?api_key=%s&language=uk-UA&page=%d", tmdbKey, page)

	resp, err := client.Get(url)
	if err != nil {
		return c.Reply("❌ TMDB API помилка")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return c.Reply(fmt.Sprintf("❌ TMDB API помилка (код: %d)", resp.StatusCode))
	}

	var result struct {
		Results []tmdbMovie `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Results) == 0 {
		return c.Reply("❌ Фільми не знайдені")
	}

	// Pick random movie with poster
	var movie tmdbMovie
	for attempts := 0; attempts < 10; attempts++ {
		m := result.Results[rand.Intn(len(result.Results))]
		if m.PosterPath != "" {
			movie = m
			break
		}
	}
	if movie.PosterPath == "" {
		return c.Reply("❌ Фільм без постера")
	}

	posterURL := fmt.Sprintf("https://image.tmdb.org/t/p/w500%s", movie.PosterPath)

	// Build aliases
	aliases := []string{strings.ToLower(movie.Title)}
	if movie.OrigTitle != "" && movie.OrigTitle != movie.Title {
		aliases = append(aliases, strings.ToLower(movie.OrigTitle))
	}
	// Add without year
	for _, a := range aliases {
		// Remove common suffixes
		cleaned := strings.TrimSpace(a)
		if len(cleaned) > 3 {
			aliases = append(aliases, cleaned)
		}
	}

	displayTitle := movie.Title
	if movie.OrigTitle != "" && movie.OrigTitle != movie.Title {
		displayTitle = fmt.Sprintf("%s (%s)", movie.Title, movie.OrigTitle)
	}

	gameID := time.Now().UnixNano()

	movieGameMu.Lock()
	activeMovieGame[chatID] = &movieGame{
		ID:        gameID,
		Title:     displayTitle,
		ChatID:    chatID,
		Aliases:   aliases,
		CreatedAt: time.Now(),
	}
	movieGameMu.Unlock()

	year := ""
	if len(movie.ReleaseDate) >= 4 {
		year = movie.ReleaseDate[:4]
	}

	caption := fmt.Sprintf("🎬 Що це за фільм? (30 сек)\nРік: %s\nНагорода: +15 🪙", year)

	photo := &tele.Photo{
		File:    tele.FromURL(posterURL),
		Caption: caption,
	}
	sent, err := c.Bot().Send(c.Chat(), photo)
	if err != nil {
		movieGameMu.Lock()
		delete(activeMovieGame, chatID)
		movieGameMu.Unlock()
		return c.Reply("❌ Не вдалося відправити постер")
	}

	movieGameMu.Lock()
	if g, ok := activeMovieGame[chatID]; ok {
		g.SentMsg = sent
	}
	movieGameMu.Unlock()

	bot := c.Bot()
	cmdMsg := c.Message()

	go func() {
		time.Sleep(30 * time.Second)
		movieGameMu.Lock()
		game, ok := activeMovieGame[chatID]
		if ok && game.Winner == "" && game.ID == gameID {
			sentMsg := game.SentMsg
			title := game.Title
			delete(activeMovieGame, chatID)
			movieGameMu.Unlock()
			if sentMsg != nil {
				bot.Delete(sentMsg)
			}
			if cmdMsg != nil {
				bot.Delete(cmdMsg)
			}
			msg, _ := bot.Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🎬 Час вийшов! Це було: %s", title))
			autoDelete(bot, 5*time.Second, msg)
		} else {
			movieGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkMovieAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	movieGameMu.Lock()
	game, ok := activeMovieGame[chatID]
	if !ok {
		movieGameMu.Unlock()
		return false
	}

	correct := false
	for _, alias := range game.Aliases {
		if len(alias) > 2 && strings.Contains(text, alias) {
			correct = true
			break
		}
	}

	if !correct {
		movieGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeMovieGame, chatID)
	movieGameMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "movie", reward)

	c.Reply(fmt.Sprintf("🎬 %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Title, reward, newBal))
	return true
}
