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

// cleanWords — proper 5-letter Ukrainian words only (350 words)
var cleanWords = []string{
	"абзац", "азарт", "амбар", "ангел", "архів",
	"атлет", "афіша", "балет", "банан", "барва",
	"батут", "бекон", "блиск", "бляха", "бомба",
	"брама", "букет", "бухта", "білка", "валет",
	"ванна", "вбити", "велет", "верес", "вести",
	"вибух", "видно", "вилка", "вихід", "вклад",
	"вміти", "волан", "вояка", "втіха", "вушко",
	"вікно", "вінок", "віскі", "віяло", "гаман",
	"гвинт", "голий", "голос", "горло", "горіх",
	"гриль", "груша", "гумор", "гірка", "гієна",
	"дартс", "декан", "джура", "дзюдо", "дивно",
	"динар", "добре", "догма", "донор", "дотик",
	"дошка", "дуель", "думка", "дірка", "ефект",
	"ждати", "жетон", "жилка", "життя", "жниця",
	"жолоб", "жінка", "завал", "задум", "закуп",
	"замах", "замша", "запис", "заріз", "заява",
	"звити", "зграя", "зерно", "знизу", "зошит",
	"зріст", "зілля", "калач", "карта", "катер",
	"каюта", "кивок", "кишка", "кобра", "козак",
	"комар", "копія", "котел", "криза", "крона",
	"круїз", "кубок", "куток", "кіоск", "лавра",
	"лазер", "ланка", "лафет", "лейка", "листя",
	"лицар", "лошак", "лівий", "лізти", "лікер",
	"лінза", "ліцей", "магія", "майор", "мамин",
	"маска", "матюк", "медик", "мерти", "метис",
	"метро", "мирне", "миття", "модно", "монах",
	"моток", "мрець", "музей", "мушля", "місія",
	"мітоз", "набат", "навіс", "нажив", "найти",
	"наліт", "напад", "нараз", "натяг", "начіс",
	"нести", "новий", "норка", "нотка", "нюанс",
	"німий", "нірка", "оазис", "облік", "обмір",
	"обряд", "оглав", "одежа", "оклад", "окунь",
	"опіум", "ореол", "осада", "осінь", "отець",
	"пагін", "пакет", "панна", "паста", "пачка",
	"пенат", "пилка", "писар", "плата", "плита",
	"побут", "повік", "поділ", "позов", "полин",
	"поляк", "помпа", "порив", "поруч", "посол",
	"потоп", "похід", "поява", "прима", "приют",
	"просо", "пузан", "пульт", "пучка", "пізно",
	"після", "пісок", "рабин", "радіо", "ранок",
	"ребус", "рейка", "рента", "рибка", "риска",
	"роман", "ртуть", "рулет", "русло", "ряска",
	"рідня", "різко", "ріоні", "річка", "салон",
	"самка", "сатин", "сачок", "свято", "седан",
	"серце", "сивий", "синій", "скала", "сквер",
	"склеп", "скоро", "слива", "слюда", "смола",
	"сміло", "снити", "сокіл", "сором", "сотня",
	"спати", "спирт", "сплін", "спрут", "старт",
	"створ", "стеля", "стиль", "стопа", "стрих",
	"ступа", "сукно", "сумка", "сурма", "сяйво",
	"сівба", "сірий", "сітка", "табло", "тайна",
	"талія", "таран", "тачка", "текти", "темно",
	"терен", "терти", "тиран", "титул", "ткати",
	"томик", "тонус", "торба", "точно", "транс",
	"траур", "трефа", "трипс", "тромб", "труна",
	"тріод", "туніс", "тупіт", "турок", "тягар",
	"тісто", "убити", "умить", "уміст", "упряж",
	"уступ", "утиск", "учити", "фагот", "фанат",
	"фасон", "фетиш", "фокус", "фотон", "фрукт",
	"фікус", "фінік", "фішка", "халат", "хамса",
	"хасид", "хвиля", "хитро", "ходак", "холод",
	"хорал", "хрест", "хурма", "хутір", "хінді",
	"цегла", "цикля", "цокіт", "цупко", "ціпок",
	"чайка", "чапля", "часто", "чемно", "число",
	"чобіт", "чорно", "чужий", "чутно", "шабля",
	"шалаш", "шахта", "шельф", "шинка", "шифон",
	"шквал", "шкура", "шнапс", "шофер", "шпара",
	"шпуля", "штамп", "штрих", "шубка", "щастя",
	"щиток", "щупак", "юдоль", "юрист", "ягуар",
	"ялиця", "ярлик", "яєчня", "ґвалт", "ґонта",
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
	1: 30, 2: 25, 3: 20, 4: 15, 5: 10, 6: 5,
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
		return c.Reply(fmt.Sprintf("📝 Ліміт %d wordle на день. Скидання через %s", maxWordlePerDay, timeUntilReset()))
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
	return c.Send(fmt.Sprintf("📝 *Wordle*\n\nВгадай слово з %d букв за 6 спроб!\nПиши слово в чат.\n\nНагорода: від 5 до 30 🪙\n🟩 = правильна буква і місце\n🟨 = правильна буква, інше місце\n⬛ = немає такої букви", wordLen),
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
		b.db.LogTransaction(userID, userName, "wordle", reward)

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
