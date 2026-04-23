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
	// /battle removed — use /duel or /war instead
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
	bot.Handle("/dart", b.handleDart)
	bot.Handle("/war", b.handleWar)
	bot.Handle("/casino_stats", b.handleCasinoStats)
	bot.Handle("/global_stats", b.handleGlobalStats)
	bot.Handle(&tele.Btn{Unique: "war_pick"}, b.handleWarPick)
	bot.Handle("/blackjack", b.handleBlackjack)
	bot.Handle("/bj", b.handleBlackjack)
	bot.Handle(&tele.Btn{Unique: "bj_hit"}, b.handleBJHit)
	bot.Handle(&tele.Btn{Unique: "bj_stand"}, b.handleBJStand)
	bot.Handle("/bj_cancel", b.handleBJCancel)
	bot.Handle("/addquote", b.handleAddQuote)
	bot.Handle("/work", b.handleMarkWork)
	bot.Handle("/notwork", b.handleMarkNotWork)
	bot.Handle("/achievements", b.handleAchievements)
	bot.Handle("/title", b.handleTitle)
	bot.Handle("/evolve", b.handleEvolve)
	bot.Handle("/card_idea", b.handleCardIdea)
	bot.Handle("/joke", b.handleJoke)
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

🃏 *Картки (505 шт):*
/pack — пак (40 🪙, макс 7/день)
/collection — колекція
/duel — дуель з вибором картки
/steal — вкрасти картку (30%)
/rob — пограбувати монети (33%)
/auction — аукціон картки
/sacrifice — 7 карток → 1 вищої
/gacha — преміум пак (300 🪙)
/burn — спалити за монети
/gift — подарувати картку
/showcase — найкраща картка
/card — подивитись картку

🎰 *Економіка:*
/slots — слоти (1-500 🪙, макс 20/день)
/blackjack — блекджек (1-500 🪙)
/daily — бонус +75 🪙
/balance — баланс
/top — лідерборд
/casino_stats — твоя статистика 📊
/global_stats — загальна статистика

🎮 *Розваги:*
/pokemon — покемон 🔴
/horoscope — дев-гороскоп 🔮
/8ball — магічна куля 🎱
/cat 🐱 /dog 🐕
/quiz — вікторина (+5-15 🪙) 🧠
/guess — вгадай число (мультиплеєр, +30/+100) 🎯
/wordle — wordle (3/день, +5-30 🪙) 📝
/dart — дартс PvP 🎯
/war — війна карток (3 раунди) ⚔️

✨ /evolve — еволюція картки (Stage 2)
🏆 /achievements — досягнення
🏷 /title — титули з бонусами
💡 /card\_idea — запропонувати картку

⚙️ /mute /unmute — трекінг
📖 /help — правила`
	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send("📖 *Правила та механіки*\n\nhttps://fuck-work-bot.fly.dev/help", &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleCardIdea(c tele.Context) error {
	idea := strings.TrimSpace(c.Message().Payload)
	if idea == "" {
		return c.Reply("Формат: /card\\_idea назва картки — опис\n\nНаприклад: /card\\_idea Кава о 8 ранку — коли без кави нічого не працює")
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	b.db.SaveCardIdea(userID, userName, idea)
	return c.Reply("💡 Ідею збережено! Переглянути всі: https://fuck-work-bot.fly.dev/ideas")
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

type pendingGift struct {
	Key      string
	Username string
	CardID   int
	CardName string
	Rarity   int
}

var pendingGifts = []pendingGift{
	{"gift_data_emerald", "kondzhariia_data", 604, "Смарагдове небо", 5},
	{"gift_data_emerald", "kondzhariia", 604, "Смарагдове небо", 5},
	{"gift_data_terpila", "kondzhariia_data", 608, "Тєрпіла", 2},
	{"gift_data_terpila", "kondzhariia", 608, "Тєрпіла", 2},
	{"gift_bo_terpila", "facethestrange", 608, "Тєрпіла", 2},
}

func (b *Bot) checkPendingGifts(c tele.Context, userID, userName string) {
	username := c.Sender().Username
	for _, g := range pendingGifts {
		if username != g.Username {
			continue
		}
		if b.db.GetMeta(g.Key) != "" {
			continue
		}
		b.db.EnsureUser(userID, userName)
		b.db.AddToCollection(userID, g.CardID)
		b.db.SetMeta(g.Key, "done")
		stars := rarityStars[g.Rarity]
		rName := rarityNames[g.Rarity]
		c.Send(fmt.Sprintf("%s %s отримує %s картку: *%s*!", stars, userName, rName, g.CardName), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}
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

	// One-time pending card gifts
	b.checkPendingGifts(c, userID, userName)

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
	userID := strconv.FormatInt(c.Sender().ID, 10)
	msgID := fmt.Sprintf("%d", c.Message().ReplyTo.ID)
	feedbackKey := "fb:" + userID + ":" + msgID
	if b.db.GetMeta(feedbackKey) != "" {
		return c.Reply("⚠️ Ти вже оцінив це повідомлення")
	}
	text := c.Message().ReplyTo.Text
	b.db.SaveFeedback(text, "work")
	b.db.SetMeta(feedbackKey, "work")
	userName := c.Sender().FirstName
	b.db.UpdateBalance(userID, userName, 10)
	b.db.LogTransaction(userID, userName, "feedback", 10)
	log.Printf("[feedback] /work: %q", text)
	return c.Reply("✅ Позначено як робота (+10 🪙)")
}

func (b *Bot) handleMarkNotWork(c tele.Context) error {
	if c.Message().ReplyTo == nil || c.Message().ReplyTo.Text == "" {
		return c.Reply("Відповідай на повідомлення командою /notwork щоб позначити його як не робочe")
	}
	userID := strconv.FormatInt(c.Sender().ID, 10)
	msgID := fmt.Sprintf("%d", c.Message().ReplyTo.ID)
	feedbackKey := "fb:" + userID + ":" + msgID
	if b.db.GetMeta(feedbackKey) != "" {
		return c.Reply("⚠️ Ти вже оцінив це повідомлення")
	}
	text := c.Message().ReplyTo.Text
	b.db.SaveFeedback(text, "personal")
	b.db.SetMeta(feedbackKey, "notwork")
	userName := c.Sender().FirstName
	b.db.UpdateBalance(userID, userName, 10)
	b.db.LogTransaction(userID, userName, "feedback", 10)
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
