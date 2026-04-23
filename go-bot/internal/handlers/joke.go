package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

const maxJokesPerDay = 17

func (b *Bot) handleJoke(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	today := todayKyiv()

	// Check daily limit
	jokeKey := "joke:" + userID + ":" + today
	countStr := b.db.GetMeta(jokeKey)
	count := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &count)
	}
	if count >= maxJokesPerDay {
		return c.Reply(fmt.Sprintf("😂 Ліміт %d жартів на день. Скидання через %s", maxJokesPerDay, timeUntilReset()))
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	var joke string

	if geminiKey != "" {
		joke = generateJoke(geminiKey, userName)
	}

	if joke == "" {
		joke = localJoke(userName)
	}

	b.db.SetMeta(jokeKey, fmt.Sprintf("%d", count+1))

	return c.Send(fmt.Sprintf("😂 *Жарт для %s*\n\n%s", userName, joke), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func generateJoke(apiKey, userName string) string {
	topics := []string{
		"програмування, баги, деплой, код-рев'ю, продакшн, стендапи",
		"автомобілі, Порш, Вольво, Мазда, техогляд, бензин, кредит на авто",
		"криптовалюта, біткоїн, трейдинг, інвестиції, 'to the moon'",
		"казино, слоти, блекджек, ставки, програш",
		"офісне життя, дедлайни, мітинги, зарплата, фріланс",
		"їжа, борщ, шаурма, кава, обід, доставка",
		"спортзал, тренування, біг, дієта",
		"стосунки, дейтинг, тіндер",
		"котики, собаки, тваринки",
		"ігри, PlayStation, Steam, мобільні ігри",
	}
	topic := topics[rand.Intn(len(topics))]

	prompt := fmt.Sprintf(`Напиши короткий їдкий жарт-підколку для %s на тему: %s.

ФОРМАТ: 1-2 речення максимум. Як SMS від токсичного друга.

ПРИКЛАДИ СТИЛЮ:
- "%s, ти обираєш машину довше ніж дівчину. І обидві потім ламаються."
- "Твоя дієта тримається рівно до першої шаурми. Тобто до обіду."
- "%s записався в зал, купив форму, зробив фотку — і на цьому все."
- "Ти граєш в казино як живеш — впевнено входиш і без грошей виходиш."
- "%s, ти кажеш 'я на дієті' з набитим ротом."

ПРАВИЛА:
- Коротко і різко, як удар
- Сарказм і іронія, не довгі пояснення
- НЕ тільки про роботу і код — жарти про життя, звички, лінь, їжу, гроші, стосунки, машини
- Українською, можна сленг
- Тільки текст жарту, нічого зайвого`, userName, topic, userName, userName, userName)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{"temperature": 1.3, "maxOutputTokens": 80},
	}

	jsonBody, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	}

	return ""
}

func localJoke(userName string) string {
	jokes := []string{
		"%s, твій код настільки поганий, що навіть Stack Overflow відмовився допомагати.",
		"%s пише код як п'яний хірург — впевнено, але результат жахливий.",
		"Щоразу коли %s каже 'я зараз швидко зафікшу' — всі знають, що продакшн впаде.",
		"%s, ти не senior розробник. Ти просто junior який занадто довго тут сидить.",
		"git blame показав що 90%% багів — від %s. Решта 10%% — теж від нього, просто під іншим акаунтом.",
		"%s знову 'працює з дому'. Тобто лежить, дивиться серіали і іноді ворушить мишкою.",
		"Код %s — як його особисте життя. Ніхто не розуміє, але всі роблять вигляд що все ок.",
		"%s каже 'мій код самодокументований'. Це означає що документації немає і не буде.",
		"В резюме %s написано '5+ років досвіду'. По коду — максимум 5 місяців.",
		"Якби %s програмував ракету — вона б летіла, але не туди і не тоді.",
		"%s, ти витрачаєш на вибір назви змінної більше часу, ніж на саму логіку.",
		"Коли %s каже 'це працює на моїй машині' — це означає що більше ніде не працює.",
		"%s, твій останній пул-реквест був настільки поганий, що рев'юер звільнився.",
		"Кожен раз коли %s деплоїть — DevOps команда відкриває шампанське. Бо треба заспокоїти нерви.",
		"Різниця між %s і ChatGPT: ChatGPT хоча б вибачається коли пише фігню.",
	}
	template := jokes[rand.Intn(len(jokes))]
	return fmt.Sprintf(template, userName)
}
