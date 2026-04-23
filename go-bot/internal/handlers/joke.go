package handlers

import (
	"encoding/json"
	"fmt"
	"log"
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

	// Optional custom topic + detect target name
	customTopic := strings.TrimSpace(c.Message().Payload)

	// Check if topic mentions a known member — target joke at them
	targetName := userName
	knownNames := map[string]string{
		"danya": "Danya", "данька": "Danya", "данік": "Danya", "дані": "Danya",
		"bo": "Bo", "бо": "Bo", "богдан": "Bo", "бодька": "Bo",
		"data": "Data", "дата": "Data",
		"dmytro": "Dmytro", "дмитро": "Dmytro", "діма": "Dmytro",
	}
	for key, name := range knownNames {
		if strings.Contains(strings.ToLower(customTopic), key) {
			targetName = name
			break
		}
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	var joke string

	if geminiKey != "" {
		joke = generateJoke(geminiKey, targetName, customTopic)
	}

	if joke == "" {
		joke = localJoke(targetName)
	}

	b.db.SetMeta(jokeKey, fmt.Sprintf("%d", count+1))

	return c.Send(fmt.Sprintf("😂 *Жарт для %s*\n\n%s", targetName, joke), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func generateJoke(apiKey, userName, customTopic string) string {
	topics := []string{
		"програмування, баги, деплой, код-рев'ю",
		"дизайн, Figma, пікселі, макети, UI/UX, шрифти",
		"автомобілі, Порш, Вольво, Мазда, кредит на авто",
		"криптовалюта, біткоїн, трейдинг",
		"казино, слоти, блекджек, програш",
		"дедлайни, мітинги, зарплата",
		"їжа, борщ, шаурма, кава, доставка",
		"спортзал, дієта, тренування",
		"стосунки, дейтинг, тіндер, самотність",
		"гроші, кредити, борги, економія, жадібність",
		"лінь, прокрастинація, серіали, сон до обіду",
		"зовнішність, одяг, стиль, секонд-хенд",
		"алкоголь, пиво, п'ятниця, похмілля",
		"інтелект, тупість, забудькуватість",
	}
	topic := customTopic
	if topic == "" {
		topic = topics[rand.Intn(len(topics))]
	}

	// Member context — the more details, the better jokes
	memberContext := ""
	switch userName {
	case "Danya":
		memberContext = ` Danya (Данька) — дизайнер, працює в Figma/UI/UX, НЕ програміст.
Лудоман — грає в слоти і блекджек постійно, завжди програє. Взяв позику у "Швидкогроші".
Фанат серіалу Усман. Любить шаурму. Має дівчину Лізу.
Вічно без грошей. Каже "ще один спін і все". Дизайнить макети які ніхто не апрувить.`
	case "Bo":
		memberContext = ` Bo (Богдан/Бодька) — розробник. Фанат Керхера — миє все підряд (машину, тротуар, сусідів).
Продав Мазду і досі жалкує. Любить чай з мелісою (якщо ви розумієте).
Має кота. Живе на секонд-хенді. Вічно щось ремонтує.
Каже що на дієті але їсть шаурму о 2 ночі.`
	case "Data":
		memberContext = ` Data (Кондж) — iOS розробник, пише на Swift.
Мрія — купити Порш. Їздить 40км заради хінкалі.
Фанат даних і аналітики. Архітектор. Серйозний але дивний.
Вірить в крипту. Дивиться на графіки більше ніж на людей.`
	case "Dmytro":
		memberContext = ` Dmytro (Діма) — розробник, створив цього бота.
Грає в казино в своєму ж боті. Патріот. Любить автоматизувати все.
Пише код вночі. Каже "я швидко зафікшу" і фіксить 3 дні.
Має більше комітів ніж друзів.`
	}

	prompt := fmt.Sprintf(`Ти — токсичний друг який жорстко підколює. Напиши підколку для %s на тему: %s.%s

ФОРМАТ: 1-2 речення. Коротко, боляче, смішно.

ПРИКЛАДИ (такий рівень жорсткості):
- "%s, ти як Wi-Fi в метро — ніби є, але нічого не працює."
- "Твоє обличчя — найкраща реклама роботи з дому."
- "%s, ти не товстий, просто занадто ambitious для свого метаболізму."
- "Ти настільки лінивий, що навіть твоя прокрастинація прокрастинує."
- "%s, твій смак в одязі — як твій код. Всі бачать проблему, крім тебе."

ПРАВИЛА:
- Жорстко але смішно, як панч від друга
- Можна чіпати: зовнішність, інтелект, лінь, невезіння, бідність, самотність
- Українською, сленг ок
- ТІЛЬКИ текст жарту`, userName, topic, memberContext, userName, userName, userName)

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent?key=%s", apiKey)

	body := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]string{{"text": prompt}}},
		},
		"generationConfig": map[string]interface{}{"temperature": 1.3, "maxOutputTokens": 100},
		"safetySettings": []map[string]string{
			{"category": "HARM_CATEGORY_HARASSMENT", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_HATE_SPEECH", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_SEXUALLY_EXPLICIT", "threshold": "BLOCK_NONE"},
			{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_NONE"},
		},
	}

	jsonBody, _ := json.Marshal(body)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", strings.NewReader(string(jsonBody)))
	if err != nil {
		log.Printf("[joke] Gemini request error: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[joke] Gemini status: %d", resp.StatusCode)
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
