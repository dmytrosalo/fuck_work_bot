package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/dmytrosalo/fuck-work-bot/internal/classifier"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

var roasts = []string{
	"О, хтось знову не може відпустити роботу навіть у чаті",
	"Так, ми всі вражені твоєю зайнятістю. Ні, насправді ні.",
	"Чат для відпочинку, а не для твоїх робочих драм",
	"Ти взагалі вмієш говорити про щось крім роботи?",
	"Вау, робота. Як оригінально. Всім дуже цікаво.",
	"Хтось явно не вміє відділяти роботу від життя",
	"Знову ця корпоративна нудьга в чаті...",
	"Ми зрозуміли, ти працюєш. Можна далі жити?",
	"Робота-робота... А особистість у тебе є?",
	"Чергова робоча тема? Як несподівано від тебе.",
	"Ти на годиннику чи просто не можеш зупинитись?",
	"Слухай, є інші теми для розмов. Google допоможе.",
	"О ні, знову хтось важливий зі своєю важливою роботою",
	"Так, так, дедлайни, мітинги, ми в захваті. Далі що?",
	"Може краще в робочий чат? Або в щоденник?",
	"Друже, це чат, а не твій LinkedIn",
	"Знову робочі проблеми? Психотерапевт дешевший",
	"Цікаво, ти й уві сні про роботу говориш?",
	"Нагадую: тут люди відпочивають від роботи. Ну, крім тебе.",
	"Ого, ще одне повідомлення про роботу! Який сюрприз!",
	"Може хоч раз поговоримо про щось людське?",
	"Твій роботодавець не платить за рекламу в цьому чаті",
	"Роботоголізм — це діагноз, до речі",
	"Дивно, що ти ще не створив окремий чат для своїх тікетів",
	"О, знову ти зі своїми важливими справами. Фанфари!",
	"Тут є правило: хто пише про роботу — той лох",
	"Знаєш що крутіше за роботу? Буквально все.",
	"А ти точно не бот? Бо тільки боти так багато про роботу",
	"Ми не твої колеги, можеш розслабитись",
	"Хтось забув вимкнути робочий режим",
}

func randomRoast() string {
	return roasts[rand.Intn(len(roasts))]
}

// Bot wraps the classifier and storage for Telegram handlers.
type Bot struct {
	clf *classifier.Classifier
	db  *storage.DB
}

// New creates a new Bot handler.
func New(clf *classifier.Classifier, db *storage.DB) *Bot {
	return &Bot{clf: clf, db: db}
}

// Register attaches all command and message handlers to the telebot instance.
func (b *Bot) Register(bot *tele.Bot) {
	bot.Handle("/start", b.handleStart)
	bot.Handle("/check", b.handleCheck)
	bot.Handle("/stats", b.handleStats)
	bot.Handle("/mute", b.handleMute)
	bot.Handle("/unmute", b.handleUnmute)
	bot.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleStart(c tele.Context) error {
	msg := `*Привіт\\!* Я бот, який класифікує повідомлення\\.

*Команди:*
/check <текст> — перевірити текст
/stats — статистика користувачів
/mute — замутити себе
/unmute — розмутити себе`
	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdownV2})
}

func (b *Bot) handleCheck(c tele.Context) error {
	text := c.Message().Payload
	if text == "" {
		return c.Reply("Напиши текст після команди: /check <текст>")
	}

	res, err := b.clf.Classify(text)
	if err != nil {
		return c.Reply("Помилка класифікації")
	}

	emoji := "💬"
	if res.IsWork {
		emoji = "💼"
	}

	reply := fmt.Sprintf("%s %s (%.0f%%)", emoji, res.Label, res.Confidence*100)
	return c.Reply(reply)
}

func (b *Bot) handleStats(c tele.Context) error {
	stats, err := b.db.GetAllStats()
	if err != nil {
		return c.Reply("Помилка отримання статистики")
	}

	if len(stats) == 0 {
		return c.Reply("Статистика поки порожня")
	}

	var sb strings.Builder
	sb.WriteString("*Статистика:*\n\n")

	var totalWork, totalPersonal int
	for _, s := range stats {
		total := s.Work + s.Personal
		workPct := 0.0
		if total > 0 {
			workPct = float64(s.Work) / float64(total) * 100
		}
		sb.WriteString(fmt.Sprintf("*%s* — робота: %.0f%% (%d/%d)\n", s.Name, workPct, s.Work, total))
		totalWork += s.Work
		totalPersonal += s.Personal
	}

	totalAll := totalWork + totalPersonal
	totalPct := 0.0
	if totalAll > 0 {
		totalPct = float64(totalWork) / float64(totalAll) * 100
	}
	sb.WriteString(fmt.Sprintf("\n*Всього:* робота %.0f%% (%d/%d)", totalPct, totalWork, totalAll))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleMute(c tele.Context) error {
	userID := strconv.FormatInt(c.Sender().ID, 10)
	b.db.Mute(userID)
	return c.Reply("Ти замучений. Бот більше не буде реагувати на твої повідомлення.")
}

func (b *Bot) handleUnmute(c tele.Context) error {
	userID := strconv.FormatInt(c.Sender().ID, 10)
	b.db.Unmute(userID)
	return c.Reply("Ти розмучений. Бот знову стежить за тобою.")
}

func (b *Bot) handleText(c tele.Context) error {
	text := c.Text()
	if text == "" {
		return nil
	}

	userID := strconv.FormatInt(c.Sender().ID, 10)
	chatID := strconv.FormatInt(c.Chat().ID, 10)

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	if userName == "" {
		userName = userID
	}

	b.db.TrackChat(chatID)

	if b.db.IsMuted(userID) {
		return nil
	}

	res, err := b.clf.Classify(text)
	if err != nil {
		return nil
	}

	b.db.UpdateStats(userID, userName, res.IsWork)
	b.db.UpdateDailyStats(userID, userName, res.IsWork)

	if res.IsWork && res.Confidence >= 0.95 {
		_ = c.Bot().React(c.Chat(), c.Message(), tele.ReactionOptions{
			Reactions: []tele.Reaction{{Type: "emoji", Emoji: "\U0001f921"}},
		})
		reply := fmt.Sprintf("%s (%.0f%%)", randomRoast(), res.Confidence*100)
		return c.Reply(reply)
	}

	return nil
}

// DailyReport sends a daily stats report to all active chats and resets daily stats.
func (b *Bot) DailyReport(bot *tele.Bot) {
	stats, err := b.db.GetDailyStats()
	if err != nil || len(stats) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("*Денний звіт:*\n\n")

	for _, s := range stats {
		total := s.Work + s.Personal
		workPct := 0.0
		if total > 0 {
			workPct = float64(s.Work) / float64(total) * 100
		}
		sb.WriteString(fmt.Sprintf("*%s* — %d повідомлень, робота: %.0f%%\n", s.Name, total, workPct))
	}

	msg := sb.String()

	chats, err := b.db.GetActiveChats()
	if err != nil {
		return
	}

	for _, chatID := range chats {
		id, err := strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			continue
		}
		chat := &tele.Chat{ID: id}
		_, _ = bot.Send(chat, msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	b.db.ResetDailyStats()
}
