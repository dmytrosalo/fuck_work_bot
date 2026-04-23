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

	prompt := fmt.Sprintf(`Згенеруй один короткий смішний жарт українською мовою на тему: %s.

Правила:
- Жарт має бути смішним і дотепним
- Максимум 3-4 речення
- Можеш використовувати мем-формат, діалоги, або класичний жарт
- Можна використовувати легкий сленг і сарказм
- Не згадуй війну чи політику
- Тільки текст жарту, без заголовків, зірочок та форматування
- Жарт має бути оригінальним`, topic)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{"temperature": 1.2, "maxOutputTokens": 200},
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
		"— Чому програміст пішов з роботи?\n— Бо не отримав масив.",
		"У продакшні все працює. Просто не так, як очікувалось.",
		"— Скільки програмістів потрібно, щоб вкрутити лампочку?\n— Жодного, це апаратна проблема.",
		"Мій код працює, і я не знаю чому. Мій код не працює, і я не знаю чому.",
		"— Дорогий, ти знову всю ніч дебажив?\n— Ні, я деплоїв. Дебажити буду зараз.",
		"Менеджер: \"Це ж просто кнопка!\"\nРозробник: *плаче на 47 файлах*",
		"Код без багів — це як борщ без буряка. Теоретично можливо, але навіщо?",
		"git commit -m \"final fix\" — найбільша брехня в історії IT.",
		"Кожен розробник знає: тимчасове рішення — це найпостійніше рішення.",
		"— Чому JavaScript розробники носять окуляри?\n— Бо вони не бачать TypeErrors.",
		"Стендап: \"Вчора працював над тікетом, сьогодні буду працювати над тікетом, блокерів немає\" — і так вже 3 тижні.",
		"Продакшн не впав. Продакшн прийняв горизонтальну позицію.",
		"Щоб стати сеньйором, потрібно зламати продакшн мінімум 3 рази.",
		"— Яка різниця між junior і senior розробником?\n— Senior знає які файли не можна чіпати.",
		"Рефакторинг — це коли ти переписуєш код, який працював, на код, який не працює, але виглядає красивіше.",
	}
	return jokes[rand.Intn(len(jokes))]
}
