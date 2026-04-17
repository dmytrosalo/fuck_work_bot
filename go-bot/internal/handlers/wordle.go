package handlers

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tele "gopkg.in/telebot.v3"
)

// Ukrainian 5-letter words
var wordleWords = []string{
	"сонце", "місто", "книга", "земля", "море",
	"небо!", "вітер", "лісок", "річка", "поле",
	"хліба", "масло", "молок", "каша!", "борщу",
	"мамою", "батьк", "школу", "друзі", "серце",
	"зірка", "думка", "казка", "мрія!", "пісня",
	"танок", "вечір", "ранок", "обід!", "вікно",
	"двері", "столу", "крісл", "лампа", "годин",
	"місяц", "тижде", "хвили", "секун", "доба!",
	"кольо", "червo", "синій", "білий", "чорни",
	"зелен", "жовти", "сірий", "рожев", "карий",
	// Better 5-letter words
	"слово", "право", "місце", "робот", "кіно!",
	"класс", "група", "точка", "лінія", "форма",
	"сила!", "честь", "слава", "радіс", "щастя",
	"біда!", "горе!", "сміхи", "плач!", "крик!",
	"пташк", "котик", "песик", "рибка", "зайці",
	"олень", "ведмі", "лисиц", "вовки", "орел!",
	"яблук", "груша", "слива", "вишня", "диня!",
	"гарбу", "цибул", "морко", "перец", "томат",
	"кава!", "чайок", "сік!!", "водka", "пиво!",
	"хатка", "замок", "палац", "місто", "село!",
}

// Clean word list — only proper 5-letter Ukrainian words
var cleanWords = []string{
	"слово", "земля", "місто", "книга", "серце",
	"зірка", "думка", "казка", "пісня", "вечір",
	"ранок", "вікно", "двері", "столи", "лампа",
	"місяц", "тижні", "форма", "точка", "група",
	"честь", "слава", "котик", "песик", "рибка",
	"олень", "груша", "слива", "вишня", "морем",
	"перці", "томат", "хатка", "замок", "палац",
	"поле!", "вітер", "хліби", "масла", "школа",
	"друзі", "танок", "право", "місце", "сила!",
	"плачу", "сміхи", "яблук", "зайці", "лисий",
	"борщі", "каша!", "салат", "кефір", "сирок",
	"пиріг", "торти", "булка", "круас", "макар",
	"кнопк", "екран", "файли", "папка", "мишка",
	"клаві", "ноутб", "фотка", "відео", "мемас",
	"лайки", "репос", "стікр", "емодж", "бот!!",
	"спорт", "футбо", "баске", "теніс", "бігом",
	"музик", "гітар", "піано", "барбн", "скрип",
	"актор", "режис", "фільм", "серіа", "мульт",
	"герой", "лиход", "магія", "меч!!", "щит!!",
	"дракн", "ельфи", "гноми", "тролі", "орків",
}

// userWord returns a unique word per user per game (not shared)
func userWord(userID string, gameNum int) string {
	today := time.Now().Format("2006-01-02")
	h := fnv.New32a()
	h.Write([]byte(today + userID + fmt.Sprintf("%d", gameNum) + "wordle"))
	idx := int(h.Sum32()) % len(cleanWords)
	return cleanWords[idx]
}

type wordleState struct {
	Word     string
	Attempts []string
	MaxTries int
}

var (
	activeWordles = make(map[string]*wordleState) // userID -> state
	wordleMu     sync.Mutex
)

var wordleRewards = map[int]int{
	1: 50, 2: 40, 3: 30, 4: 20, 5: 15, 6: 10,
}

const maxWordlePerDay = 3

func (b *Bot) handleWordle(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	today := time.Now().Format("2006-01-02")

	// Check how many games played today
	countKey := "wordle_count:" + userID + ":" + today
	countStr := b.db.GetMeta(countKey)
	gamesPlayed := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &gamesPlayed)
	}

	if gamesPlayed >= maxWordlePerDay {
		return c.Reply(fmt.Sprintf("📝 Ліміт %d wordle на день. Приходь завтра!", maxWordlePerDay))
	}

	wordleMu.Lock()
	if _, ok := activeWordles[userID]; ok {
		wordleMu.Unlock()
		return c.Reply("📝 У тебе вже є активна гра! Напиши слово")
	}

	word := userWord(userID, gamesPlayed+1)
	// Clean word (remove non-letter chars)
	cleaned := ""
	for _, r := range word {
		if r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'і' || r == 'ї' || r == 'є' || r == 'ґ' {
			cleaned += string(r)
		}
	}
	if utf8.RuneCountInString(cleaned) < 4 {
		cleaned = "слово" // fallback
	}

	activeWordles[userID] = &wordleState{
		Word:     cleaned,
		Attempts: nil,
		MaxTries: 6,
	}
	wordleMu.Unlock()

	wordLen := utf8.RuneCountInString(cleaned)
	return c.Send(fmt.Sprintf("📝 *Wordle*\n\nВгадай слово з %d букв за 6 спроб!\nПиши слово в чат.\n\nНагорода: від 10 до 50 🪙\n🟩 = правильна буква і місце\n🟨 = правильна буква, інше місце\n⬛ = немає такої букви", wordLen),
		&tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) checkWordleAnswer(c tele.Context) bool {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	wordleMu.Lock()
	game, ok := activeWordles[userID]
	if !ok {
		wordleMu.Unlock()
		return false
	}

	wordRunes := []rune(game.Word)
	guessRunes := []rune(text)

	// Must be same length
	if len(guessRunes) != len(wordRunes) {
		wordleMu.Unlock()
		return false
	}

	// Check only Ukrainian letters
	for _, r := range guessRunes {
		if !((r >= 'а' && r <= 'я') || r == 'і' || r == 'ї' || r == 'є' || r == 'ґ') {
			wordleMu.Unlock()
			return false
		}
	}

	// Build result
	result := make([]rune, len(wordRunes))
	wordCopy := make([]rune, len(wordRunes))
	copy(wordCopy, wordRunes)

	// First pass: exact matches
	for i := range guessRunes {
		if guessRunes[i] == wordRunes[i] {
			result[i] = '🟩'
			wordCopy[i] = 0
		}
	}

	// Second pass: wrong position
	for i := range guessRunes {
		if result[i] == '🟩' {
			continue
		}
		found := false
		for j := range wordCopy {
			if wordCopy[j] == guessRunes[i] {
				result[i] = '🟨'
				wordCopy[j] = 0
				found = true
				break
			}
		}
		if !found {
			result[i] = '⬛'
		}
	}

	resultStr := string(result)
	game.Attempts = append(game.Attempts, fmt.Sprintf("%s %s", resultStr, text))
	attempt := len(game.Attempts)

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	// Check win
	allGreen := true
	for _, r := range result {
		if r != '🟩' {
			allGreen = false
			break
		}
	}

	if allGreen {
		delete(activeWordles, userID)
		wordleMu.Unlock()

		today := time.Now().Format("2006-01-02")
		countKey := "wordle_count:" + userID + ":" + today
		countStr := b.db.GetMeta(countKey)
		played := 0
		if countStr != "" {
			fmt.Sscanf(countStr, "%d", &played)
		}
		b.db.SetMeta(countKey, fmt.Sprintf("%d", played+1))
		remaining := maxWordlePerDay - played - 1

		reward := wordleRewards[attempt]
		newBal := b.db.UpdateBalance(userID, userName, reward)

		var sb strings.Builder
		sb.WriteString("📝 *Wordle*\n\n")
		for _, a := range game.Attempts {
			sb.WriteString(a + "\n")
		}
		sb.WriteString(fmt.Sprintf("\n✅ Вгадав за %d/6! +%d 🪙 (баланс: %d)", attempt, reward, newBal))
		if remaining > 0 {
			sb.WriteString(fmt.Sprintf("\n\n📝 Залишилось ігор: %d. /wordle", remaining))
		}
		c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		return true
	}

	// Check max tries
	if attempt >= game.MaxTries {
		word := game.Word
		delete(activeWordles, userID)
		wordleMu.Unlock()

		today := time.Now().Format("2006-01-02")
		countKey := "wordle_count:" + userID + ":" + today
		countStr2 := b.db.GetMeta(countKey)
		played2 := 0
		if countStr2 != "" {
			fmt.Sscanf(countStr2, "%d", &played2)
		}
		b.db.SetMeta(countKey, fmt.Sprintf("%d", played2+1))
		remaining2 := maxWordlePerDay - played2 - 1

		var sb strings.Builder
		sb.WriteString("📝 *Wordle*\n\n")
		for _, a := range game.Attempts {
			sb.WriteString(a + "\n")
		}
		sb.WriteString(fmt.Sprintf("\n❌ Не вгадав! Слово було: %s", word))
		if remaining2 > 0 {
			sb.WriteString(fmt.Sprintf("\n\n📝 Залишилось ігор: %d. /wordle", remaining2))
		}
		c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		return true
	}

	wordleMu.Unlock()

	// Show progress
	var sb strings.Builder
	for _, a := range game.Attempts {
		sb.WriteString(a + "\n")
	}
	sb.WriteString(fmt.Sprintf("\nСпроба %d/%d", attempt, game.MaxTries))
	c.Reply(sb.String())
	return true
}
