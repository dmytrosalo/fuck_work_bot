package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/dmytrosalo/fuck-work-bot/internal/classifier"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

// roasts and compliments are in roasts.go

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
	bot.Handle("/help", b.handleHelp)
	bot.Handle("/check", b.handleCheck)
	bot.Handle("/stats", b.handleStats)
	bot.Handle("/mute", b.handleMute)
	bot.Handle("/unmute", b.handleUnmute)
	bot.Handle("/roast", b.handleRoast)
	bot.Handle("/compliment", b.handleCompliment)
	bot.Handle("/quote", b.handleQuote)
	bot.Handle("/pack", b.handlePack)
	bot.Handle("/collection", b.handleCollection)
	bot.Handle("/battle", b.handleBattle)
	bot.Handle("/pokemon", b.handlePokemon)
	bot.Handle("/horoscope", b.handleHoroscope)
	bot.Handle("/8ball", b.handleEightBall)
	bot.Handle("/slots", b.handleSlots)
	bot.Handle("/slot", b.handleSlots)
	bot.Handle("/balance", b.handleBalance)
	bot.Handle("/bal", b.handleBalance)
	bot.Handle("/daily", b.handleDaily)
	bot.Handle("/top", b.handleTop)
	bot.Handle("/dog", b.handleDog)
	bot.Handle("/cat", b.handleCat)
	bot.Handle("/quiz", b.handleQuiz)
	bot.Handle("/duel", b.handleDuel)
	bot.Handle("/accept", b.handleAccept)
	bot.Handle(&tele.Btn{Unique: "duel_pick"}, b.handleDuelPick)
	bot.Handle("/steal", b.handleSteal)
	bot.Handle("/gift", b.handleGift)
	bot.Handle("/burn", b.handleBurn)
	bot.Handle("/sacrifice", b.handleSacrifice)
	bot.Handle("/showcase", b.handleShowcase)
	bot.Handle("/gacha", b.handleGacha)
	bot.Handle("/auction", b.handleAuction)
	bot.Handle("/bid", b.handleBid)
	bot.Handle("/guess", b.handleGuess)
	bot.Handle("/rob", b.handleRob)
	bot.Handle("/wordle", b.handleWordle)
	bot.Handle("/card", b.handleCardInfo)
	bot.Handle("/addquote", b.handleAddQuote)
	bot.Handle("/work", b.handleMarkWork)
	bot.Handle("/notwork", b.handleMarkNotWork)
bot.Handle(tele.OnText, b.handleText)
}

func (b *Bot) handleStart(c tele.Context) error {
	msg := `*Привіт!* Я бот що слідкує за робочими повідомленнями і розважає.

📊 *Класифікатор:*
/check — перевірити текст
/stats — статистика
/work /notwork — мітки (+10 🪙)

🔥 *Соціальне:*
/roast — підколка (5 🪙 за іншого)
/compliment — комплімент
/quote — цитата з чату
/addquote — зберегти цитату

🃏 *Картки (301 шт):*
/pack — пак (20 🪙, макс 10/день)
/collection — колекція
/battle — швидкий батл (±10 🪙)
/duel — дуель з вибором (±15 🪙)
/steal — вкрасти картку (30%)
/rob — пограбувати монети (40%)
/auction — аукціон картки
/sacrifice — 3 картки → 1 вищої
/gacha — преміум пак (100 🪙)
/burn — спалити за монети
/gift — подарувати картку
/showcase — найкраща картка
/card — подивитись картку

🎰 *Економіка:*
/slots — слоти (1-100 🪙, макс 20/день)
/daily — бонус +50 🪙
/balance — баланс
/top — лідерборд

🎮 *Розваги:*
/pokemon — покемон 🔴
/horoscope — дев-гороскоп 🔮
/8ball — магічна куля 🎱
/cat 🐱 /dog 🐕
/quiz — вікторина (+10-25 🪙) 🧠
/guess — вгадай число (мультиплеєр) 🎯
/wordle — українське wordle 📝

⚙️ /mute /unmute — трекінг
📖 /help — правила`
	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleHelp(c tele.Context) error {
	msg := `📖 *Правила та механіки*

💰 *Економіка (богдудіки 🪙)*
• Старт: 100 🪙
• /daily: +50 🪙 на день
• /work і /notwork: +10 🪙 за мітку
• Пак карток: 20 🪙
• /roast @user: 5 🪙

🎰 *Слоти*
• Ставка: 1-100 🪙, макс 20/день
• Три однакових = множник x2-x50
• Два однакових = ставка повернута
• 💎💎💎 = ДЖЕКПОТ x50

🃏 *Картки (301 шт)*
⭐ Common (40%) — 100 шт
⭐⭐ Uncommon (25%) — 96 шт
⭐⭐⭐ Rare (25%) — 67 шт
⭐⭐⭐⭐ Epic (7%) — 27 шт
⭐⭐⭐⭐⭐ Legendary (3%) — 11 шт
• Пак = 3 картки, макс 10/день

⚔️ *Бої*
• /battle — швидкий (випадкова картка, ±10 🪙)
• /duel @user → /accept — обирай картку з 3 (±15 🪙)
• Переможець забирає картку програвшого

🦹 *Картки — дії*
• /steal @user — 30% вкрасти, 70% втратити 20 🪙 (1/день)
• /auction назва — аукціон 60 сек, /bid сума
• /sacrifice common — 3 картки → 1 вищої рідкості
• /gacha — преміум пак, гарантовано Rare+ (100 🪙)
• /burn назва — спалити за монети (5-100 🪙)
• /gift @user назва — подарувати картку
• /showcase — показати найкрутішу картку

🎮 *Розваги*
• /pokemon — покемон сьогодні (1/день)
• /horoscope — дев-гороскоп (1/день)
• /quiz — вікторина (+10-25 🪙, 10/день)
• /guess — вгадай число 1-100 (мультиплеєр, +30 🪙)
• /wordle — українське wordle (1/день, +10-50 🪙)
• /8ball — магічна куля
• /cat /dog — тваринки
• /roast /compliment — підколка/комплімент
• /quote /addquote — цитати з чату

🦹 *PvP*
• /rob @user — 40% вкрасти 10-50% монет, 60% втратити 20 (1/год)
• /steal @user — 30% вкрасти картку, 70% втратити 20 🪙 (1/день)

🤖 *Класифікатор*
• Кожне повідомлення аналізується
• Робоче (80%+) = 🤡 + підколка
• /work /notwork = тренування моделі (+10 🪙)`

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
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

	// Check active game answers
	if b.checkQuizAnswer(c) {
		return nil
	}
	if b.checkGuessAnswer(c) {
		return nil
	}
	if b.checkWordleAnswer(c) {
		return nil
	}

	if b.db.IsMuted(userID) {
		return nil
	}

	res, err := b.clf.Classify(text)
	if err != nil {
		log.Printf("[%s] classify error: %v", userName, err)
		return nil
	}

	log.Printf("[%s] %s (%.0f%%) %q", userName, res.Label, res.Confidence*100, text)

	b.db.UpdateStats(userID, userName, res.IsWork)
	b.db.UpdateDailyStats(userID, userName, res.IsWork)

	if res.IsWork && res.Confidence >= 0.80 {
		_ = c.Bot().React(c.Chat(), c.Message(), tele.ReactionOptions{
			Reactions: []tele.Reaction{{Type: "emoji", Emoji: "\U0001f921"}},
		})

		target := resolveTarget(userName, c.Sender().Username)
		roast := b.db.GetRandomRoast(target)
		if roast == "" {
			roast = "Знову про роботу? Серйозно?"
		}
		roast = strings.ReplaceAll(roast, "{name}", userName)

		reply := fmt.Sprintf("%s (%.0f%%)", roast, res.Confidence*100)
		return c.Reply(reply)
	}

	return nil
}

func (b *Bot) handleMarkWork(c tele.Context) error {
	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Text == "" {
		return c.Reply("Відповідай на повідомлення командою /work щоб позначити його як робочe")
	}
	text := c.Message().ReplyTo.Text
	b.db.SaveFeedback(text, "work")
	userID := strconv.FormatInt(c.Sender().ID, 10)
	userName := c.Sender().FirstName
	b.db.UpdateBalance(userID, userName, 10)
	log.Printf("[feedback] /work: %q", text)
	return c.Reply("✅ Позначено як робота (+10 🪙)")
}

func (b *Bot) handleMarkNotWork(c tele.Context) error {
	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Text == "" {
		return c.Reply("Відповідай на повідомлення командою /notwork щоб позначити його як не робочe")
	}
	text := c.Message().ReplyTo.Text
	b.db.SaveFeedback(text, "personal")
	userID := strconv.FormatInt(c.Sender().ID, 10)
	userName := c.Sender().FirstName
	b.db.UpdateBalance(userID, userName, 10)
	log.Printf("[feedback] /notwork: %q", text)
	return c.Reply("❌ Позначено як не робота (+10 🪙)")
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
