package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type triviaResponse struct {
	Results []struct {
		Category         string   `json:"category"`
		Difficulty       string   `json:"difficulty"`
		Question         string   `json:"question"`
		CorrectAnswer    string   `json:"correct_answer"`
		IncorrectAnswers []string `json:"incorrect_answers"`
	} `json:"results"`
}

var difficultyReward = map[string]int{
	"easy":   5,
	"medium": 10,
	"hard":   15,
}

var difficultyEmoji = map[string]string{
	"easy":   "🟢",
	"medium": "🟡",
	"hard":   "🔴",
}

const maxQuizPerDay = 10

// Active quizzes per user
var (
	activeQuizzes = make(map[string]*quizState)
	quizMu        sync.Mutex
)

type quizState struct {
	Correct   string
	Reward    int
	ExpiresAt time.Time
}

func (b *Bot) handleQuiz(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	today := todayKyiv()

	// Check daily limit
	key := "quiz:" + userID + ":" + today
	countStr := b.db.GetMeta(key)
	count := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &count)
	}
	if count >= maxQuizPerDay {
		return c.Reply(fmt.Sprintf("🧠 Ліміт %d квізів на день. Скидання через %s", maxQuizPerDay, timeUntilReset()))
	}

	// Check if user already has active quiz
	quizMu.Lock()
	if q, ok := activeQuizzes[userID]; ok && time.Now().Before(q.ExpiresAt) {
		quizMu.Unlock()
		return c.Reply("❓ У тебе вже є активне питання! Відповідай A, B, C або D")
	}
	quizMu.Unlock()

	// Fetch question from Open Trivia DB
	// Categories: 9=General, 11=Film, 12=Music, 14=TV, 15=Games, 32=Cartoons
	categories := []int{9, 11, 12, 14, 15, 32}
	cat := categories[rand.Intn(len(categories))]

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://opentdb.com/api.php?amount=1&category=%d&type=multiple", cat))
	if err != nil {
		log.Printf("[quiz] API error: %v", err)
		return c.Reply("❌ Quiz API недоступний")
	}
	defer resp.Body.Close()

	var trivia triviaResponse
	if err := json.NewDecoder(resp.Body).Decode(&trivia); err != nil || len(trivia.Results) == 0 {
		return c.Reply("❌ Не вдалося отримати питання")
	}

	q := trivia.Results[0]

	// Build answers (shuffle correct into incorrect)
	answers := make([]string, len(q.IncorrectAnswers)+1)
	copy(answers, q.IncorrectAnswers)
	answers[len(answers)-1] = q.CorrectAnswer
	rand.Shuffle(len(answers), func(i, j int) { answers[i], answers[j] = answers[j], answers[i] })

	// Find correct letter
	correctLetter := ""
	letters := []string{"A", "B", "C", "D"}
	for i, a := range answers {
		if a == q.CorrectAnswer {
			correctLetter = letters[i]
			break
		}
	}

	reward := difficultyReward[q.Difficulty]
	emoji := difficultyEmoji[q.Difficulty]

	// Store active quiz
	quizMu.Lock()
	activeQuizzes[userID] = &quizState{
		Correct:   correctLetter,
		Reward:    reward,
		ExpiresAt: time.Now().Add(60 * time.Second),
	}
	quizMu.Unlock()

	// Increment count
	b.db.SetMeta(key, fmt.Sprintf("%d", count+1))

	// Build message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🧠 *Quiz* %s %s\n", emoji, strings.Title(q.Difficulty)))
	sb.WriteString(fmt.Sprintf("📁 %s\n\n", html.UnescapeString(q.Category)))
	sb.WriteString(fmt.Sprintf("❓ %s\n\n", html.UnescapeString(q.Question)))
	for i, a := range answers {
		sb.WriteString(fmt.Sprintf("*%s)* %s\n", letters[i], html.UnescapeString(a)))
	}
	sb.WriteString(fmt.Sprintf("\nReply A, B, C or D (60 sec)\nReward: +%d 🪙", reward))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// checkQuizAnswer is called from handleText for single-letter answers
func (b *Bot) checkQuizAnswer(c tele.Context) bool {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	text := strings.ToUpper(strings.TrimSpace(c.Text()))

	if len(text) != 1 || (text != "A" && text != "B" && text != "C" && text != "D") {
		return false
	}

	quizMu.Lock()
	q, ok := activeQuizzes[userID]
	if !ok || time.Now().After(q.ExpiresAt) {
		if ok {
			delete(activeQuizzes, userID)
		}
		quizMu.Unlock()
		return false
	}
	delete(activeQuizzes, userID)
	quizMu.Unlock()

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	if text == q.Correct {
		newBalance := b.db.UpdateBalance(userID, userName, q.Reward)
		b.db.LogTransaction(userID, userName, "quiz", q.Reward)
		c.Reply(fmt.Sprintf("✅ Правильно! +%d 🪙 (баланс: %d)", q.Reward, newBalance))
	} else {
		c.Reply(fmt.Sprintf("❌ Неправильно! Правильна відповідь: %s", q.Correct))
	}
	return true
}
