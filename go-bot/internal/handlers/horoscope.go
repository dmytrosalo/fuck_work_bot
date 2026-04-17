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

var devSigns = []string{
	"♈ Баг-Овен", "♉ Тест-Телець", "♊ Мердж-Близнюки", "♋ Код-Рак",
	"♌ Деплой-Лев", "♍ Рефактор-Діва", "♎ Пулл-Реквест-Терези", "♏ Дебаг-Скорпіон",
	"♐ Фічер-Стрілець", "♑ Дедлайн-Козеріг", "♒ Аджайл-Водолій", "♓ Бекенд-Риби",
}

func getDailySign(userID string) string {
	today := time.Now().Format("2006-01-02")
	h := fnvHash(userID + today + "sign")
	return devSigns[h%uint32(len(devSigns))]
}

func fnvHash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func (b *Bot) handleHoroscope(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	today := time.Now().Format("2006-01-02")

	// Check daily limit
	key := "horoscope:" + userID
	lastDate := b.db.GetMeta(key)
	if lastDate == today {
		return c.Reply("🔮 Ти вже отримав гороскоп сьогодні. Зірки кажуть — приходь завтра!")
	}

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	sign := getDailySign(userID)

	// Try Gemini for personalized horoscope
	geminiKey := os.Getenv("GEMINI_API_KEY")
	var horoscope string

	if geminiKey != "" {
		horoscope = generateHoroscope(geminiKey, userName, sign)
	}

	if horoscope == "" {
		// Fallback to local generation
		horoscope = localHoroscope(userName)
	}

	b.db.SetMeta(key, today)

	msg := fmt.Sprintf("🔮 *Дев-гороскоп для %s*\n\n%s %s\n\n%s", userName, sign, "", horoscope)
	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func generateHoroscope(apiKey, userName, sign string) string {
	prompt := fmt.Sprintf(`Ти — смішний астролог для IT-розробників. Згенеруй короткий (3-4 речення) гороскоп українською для розробника на ім'я %s, знак — %s.

Правила:
- Використовуй IT терміни (деплой, баг, код-рев'ю, мердж, стендап, продакшн)
- Будь смішним і саркастичним
- Додай одну конкретну "пораду" (наприклад: "уникай force push після обіду")
- Не згадуй війну чи політику
- Тільки текст, без зірочок та форматування`, userName, sign)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{"temperature": 1.0, "maxOutputTokens": 200},
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

func localHoroscope(userName string) string {
	predictions := []string{
		"Сьогодні зірки радять не чіпати продакшн після 16:00. Серйозно, %s, не чіпай.",
		"Меркурій у ретрограді — ідеальний день для рефакторингу. Жартую, ніколи не рефактори в ретроград.",
		"%s, твій код сьогодні буде працювати з першого разу. Це не жарт, це пророцтво. Насолоджуйся, бо завтра буде 47 багів.",
		"Венера входить у зону pull request — очікуй 23 коментарі від код-рев'юера на один рядок.",
		"Сатурн кажє: %s, сьогодні тебе чекає мітинг який міг бути емейлом. Тримайся.",
		"Юпітер обіцяє %s підвищення. Юпітер також обіцяв це минулого місяця. Юпітер бреше.",
		"Марс у Близнюках — %s, уникай merge conflicts і Делну. Обидва однаково небезпечні.",
		"Нептун нашіптує: сьогодні ідеальний день щоб нарешті закрити той тікет який висить з минулого спринту.",
		"%s, зірки кажуть що force push сьогодні — це доля. Але доля буває помилковою.",
		"Місяць у Рибах — %s, твій стендап сьогодні буде коротким. Ти скажеш 'без блокерів' і збрешеш.",
		"Плутон рекомендує %s сьогодні працювати з дому. Або з пляжу. Або взагалі не працювати.",
		"Уран входить у фазу деплою — %s, тримай руку на кнопці rollback і пий ромашковий чай.",
	}
	template := predictions[rand.Intn(len(predictions))]
	return fmt.Sprintf(template, userName)
}
