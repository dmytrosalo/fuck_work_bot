package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

type achievement struct {
	ID          string
	Name        string
	Emoji       string
	Description string
	Category    string
	Reward      int
	Title       string
	Hidden      bool
	Check       func(s storage.UserStats, unique int, rarityCounts map[int]int, maxCopies int) bool
}

var allAchievements = []achievement{
	// === Collection (10) ===
	{"col_5", "Початківець", "🗃", "Зібрав 5 карток", "collection", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 5 }},
	{"col_10", "Колекціонер", "🎴", "Зібрав 10 карток", "collection", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 10 }},
	{"col_25", "Знавець", "📚", "Зібрав 25 карток", "collection", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 25 }},
	{"col_50", "Справжній збирач", "🏆", "Зібрав 50 карток", "collection", 200, "Збирач", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 50 }},
	{"col_100", "Картковий магнат", "👑", "Зібрав 100 карток", "collection", 500, "Магнат", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 100 }},
	{"col_200", "Легенда колекцій", "🌟", "Зібрав 200 карток", "collection", 1000, "Легенда", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return u >= 200 }},
	{"col_all_rarities", "Райдуга", "🌈", "Маєш картку кожної рідкості", "collection", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool {
			for i := 1; i <= 6; i++ {
				if r[i] == 0 {
					return false
				}
			}
			return true
		}},
	{"col_10_legendary", "Золота жила", "✨", "Маєш 10+ легендарних карток", "collection", 500, "Золотий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return r[5]+r[6] >= 10 }},
	{"col_full_common", "Сірий кардинал", "⚫", "Зібрав всі Common картки", "collection", 300, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
	{"col_full_rare", "Синій барон", "💎", "Зібрав всі Rare картки", "collection", 500, "Барон", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},

	// === Economy (8) ===
	{"eco_1k", "Перша тисяча", "💰", "Заробив 1,000 монет загалом", "economy", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 1000 }},
	{"eco_5k", "Бізнесмен", "📈", "Заробив 5,000 монет загалом", "economy", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 5000 }},
	{"eco_25k", "Олігарх", "💎", "Заробив 25,000 монет загалом", "economy", 500, "Олігарх", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalEarned >= 25000 }},
	{"eco_spend_10k", "Транжира", "💸", "Витратив 10,000 монет загалом", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.TotalSpent >= 10000 }},
	{"eco_rich", "Багатій", "💵", "Мав 1,000+ монет одночасно", "economy", 100, "Багатій", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxBalance >= 1000 }},
	{"eco_daily_30", "Стабільність", "📅", "Забрав /daily 30 разів", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DailyClaimed >= 30 }},
	{"eco_generous", "Щедра душа", "🎁", "Подарував 10+ карток", "economy", 150, "Щедрий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 10 }},
	{"eco_packs_50", "Шопоголік", "📦", "Відкрив 50 паків", "economy", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.PacksOpened >= 50 }},

	// === PvP (8) ===
	{"pvp_win_1", "Перша перемога", "⚔️", "Виграв першу дуель", "pvp", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 1 }},
	{"pvp_win_5", "Боєць", "🥊", "Виграв 5 дуелей", "pvp", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 5 }},
	{"pvp_win_15", "Воїн", "🛡", "Виграв 15 дуелей", "pvp", 150, "Воїн", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 15 }},
	{"pvp_win_30", "Чемпіон", "🏅", "Виграв 30 дуелей", "pvp", 500, "Чемпіон", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.DuelsWon >= 30 }},
	{"pvp_streak_3", "Серія", "🔥", "Виграв 3 дуелі поспіль", "pvp", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxDuelStreak >= 3 }},
	{"pvp_steal_5", "Злодій", "🥷", "Вкрав 5 карток", "pvp", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsStolen >= 5 }},
	{"pvp_steal_15", "Майстер крадій", "🦹", "Вкрав 15 карток", "pvp", 300, "Злодій", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsStolen >= 15 }},
	{"pvp_rob_1k", "Грабіжник", "💣", "Пограбував 1,000 монет загалом", "pvp", 200, "Грабіжник", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CoinsRobbed >= 1000 }},

	// === Gambling (9) ===
	{"gam_slots_10", "Новачок казино", "🎰", "Зіграв 10 разів у слоти", "gambling", 25, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 10 }},
	{"gam_slots_50", "Завсідник", "🎲", "Зіграв 50 разів у слоти", "gambling", 100, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 50 }},
	{"gam_slots_200", "Залежний", "🌀", "Зіграв 200 разів у слоти", "gambling", 300, "Гемблер", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsPlayed >= 200 }},
	{"gam_big_win", "Великий куш", "💥", "Виграв 500+ монет за один спін", "gambling", 200, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxWin >= 500 }},
	{"gam_jackpot", "ДЖЕКПОТ", "💎", "Вибив джекпот", "gambling", 500, "Джекпот", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxWin >= 2500 }},
	{"gam_bj_10", "Картяр", "♠️", "Зіграв 10 разів у блекджек", "gambling", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJPlayed >= 10 }},
	{"gam_bj_50", "Шулер", "♣️", "Зіграв 50 разів у блекджек", "gambling", 200, "Шулер", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJPlayed >= 50 }},
	{"gam_bj_blackjack_5", "Натурал", "🃏", "Отримав Blackjack 5 разів", "gambling", 300, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.BJBlackjacks >= 5 }},
	{"gam_slots_streak_5", "Фартовий", "🍀", "Виграв 5 слотів поспіль", "gambling", 200, "Фартовий", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.SlotsMaxStreak >= 5 }},

	// === Social (8) ===
	{"soc_roast_10", "Тролер", "😈", "Підколов 10 людей", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsGiven >= 10 }},
	{"soc_roast_50", "Токсик", "💀", "Підколов 50 людей", "social", 200, "Токсик", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsGiven >= 50 }},
	{"soc_roasted_25", "Жертва", "🤡", "Був підколотий 25 разів", "social", 100, "Жертва", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.RoastsReceived >= 25 }},
	{"soc_quote_5", "Цитатник", "💬", "Додав 5 цитат", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.QuotesAdded >= 5 }},
	{"soc_quote_20", "Архіваріус", "📜", "Додав 20 цитат", "social", 200, "Архіваріус", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.QuotesAdded >= 20 }},
	{"soc_gift_3", "Дарувальник", "💝", "Подарував 3 картки", "social", 50, "", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 3 }},
	{"soc_gift_10", "Санта", "🎅", "Подарував 10 карток", "social", 200, "Санта", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.CardsGifted >= 10 }},
	{"soc_compliment_20", "Душа компанії", "💞", "Зробив 20 компліментів", "social", 100, "Душка", false,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.ComplimentsGiven >= 20 }},

	// === Secret (7) — Hidden ===
	{"sec_broke", "Банкрут", "📉", "Мав 0 монет", "secret", 50, "Банкрут", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
	{"sec_robbed_10", "Магніт для злодіїв", "🧲", "Був пограбований 10 разів", "secret", 100, "", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
	{"sec_burn_legendary", "Божевілля", "🧠", "Спалив легендарну картку", "secret", 200, "Божевільний", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
	{"sec_5_copies", "Дублікатор", "🔁", "Мав 5 копій однієї картки", "secret", 100, "", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return m >= 5 }},
	{"sec_lose_5", "Невдаха", "👎", "Програв 5 дуелей поспіль", "secret", 100, "Невдаха", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return s.MaxLoseStreak >= 5 }},
	{"sec_3am", "Нічна зміна", "🦉", "Грав о 3 ночі за Києвом", "secret", 100, "Сова", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
	{"sec_wordle_1", "Геній", "🧠", "Вгадав wordle з першої спроби", "secret", 200, "Геній", true,
		func(s storage.UserStats, u int, r map[int]int, m int) bool { return false }},
}

// Title passive bonuses
type titleBonus struct {
	Description      string
	DailyBonus       int  // extra coins on /daily
	PackCards        int  // extra cards per pack
	RobChanceAdd     int  // added to rob success % (base 33)
	StealChanceAdd   int  // added to steal success % (base 30)
	SlotMaxBetAdd    int  // added to max slot/bj bet (base 500)
	FreeRoasts       bool // roasts cost 0
	WordleExtraGames int  // extra wordle games per day
	NightMultiplier  bool // x2 rewards 00:00-06:00 Kyiv
	RobProtect       int  // % chance to block incoming rob
	BurnBonusPct     int  // extra % coins from /burn
}

var titleBonuses = map[string]titleBonus{
	// Collection
	"Збирач":    {Description: "+1 картка в паку", PackCards: 1},
	"Магнат":    {Description: "+1 картка в паку, +25 /daily", PackCards: 1, DailyBonus: 25},
	"Легенда":   {Description: "+2 картки в паку, +50 /daily", PackCards: 2, DailyBonus: 50},
	"Золотий":   {Description: "+1 картка в паку", PackCards: 1},
	"Барон":     {Description: "+1 картка в паку, +25 /daily", PackCards: 1, DailyBonus: 25},
	// Economy
	"Олігарх":   {Description: "+50 /daily", DailyBonus: 50},
	"Багатій":   {Description: "+25 /daily", DailyBonus: 25},
	"Щедрий":    {Description: "+25 /daily, +25% burn", DailyBonus: 25, BurnBonusPct: 25},
	// PvP
	"Воїн":      {Description: "+5% steal", StealChanceAdd: 5},
	"Чемпіон":   {Description: "+10% steal, +5% rob", StealChanceAdd: 10, RobChanceAdd: 5},
	"Злодій":    {Description: "+10% steal", StealChanceAdd: 10},
	"Грабіжник": {Description: "+10% rob", RobChanceAdd: 10},
	// Gambling
	"Гемблер":   {Description: "+100 макс ставка", SlotMaxBetAdd: 100},
	"Джекпот":   {Description: "+200 макс ставка", SlotMaxBetAdd: 200},
	"Шулер":     {Description: "+100 макс ставка", SlotMaxBetAdd: 100},
	"Фартовий":  {Description: "+100 макс ставка, +25 /daily", SlotMaxBetAdd: 100, DailyBonus: 25},
	// Social
	"Токсик":     {Description: "безкоштовні роасти", FreeRoasts: true},
	"Жертва":     {Description: "30% захист від /rob", RobProtect: 30},
	"Архіваріус": {Description: "+1 wordle на день", WordleExtraGames: 1},
	"Санта":      {Description: "+25 /daily, +25% burn", DailyBonus: 25, BurnBonusPct: 25},
	"Душка":      {Description: "+25 /daily", DailyBonus: 25},
	// Secret
	"Банкрут":     {Description: "+10 /daily, 20% захист від /rob", DailyBonus: 10, RobProtect: 20},
	"Божевільний": {Description: "+50% burn монети", BurnBonusPct: 50},
	"Невдаха":     {Description: "30% захист від /steal", RobProtect: 30},
	"Сова":        {Description: "x2 нагорода 00:00-06:00", NightMultiplier: true},
	"Геній":       {Description: "+50 /daily, +1 wordle", DailyBonus: 50, WordleExtraGames: 1},
}

// getTitleBonus returns the active title's bonus for a user (zero value if none)
func (b *Bot) getTitleBonus(userID string) titleBonus {
	title := b.db.GetActiveTitle(userID)
	if title == "" {
		return titleBonus{}
	}
	return titleBonuses[title]
}

var achievementByID = func() map[string]*achievement {
	m := make(map[string]*achievement)
	for i := range allAchievements {
		m[allAchievements[i].ID] = &allAchievements[i]
	}
	return m
}()

func (b *Bot) checkAchievements(c tele.Context, userID, userName string) {
	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	stats := b.db.GetUserStats(userID)
	unique, _ := b.db.GetCollectionStats(userID)
	rarityCounts := b.db.GetRarityCardCounts(userID)
	maxCopies := b.db.GetMaxCardCopies(userID)

	for i := range allAchievements {
		a := &allAchievements[i]
		if unlockedSet[a.ID] {
			continue
		}

		passed := false
		switch a.ID {
		case "col_full_common":
			total := b.db.GetTotalCardsByRarity(1)
			passed = total > 0 && rarityCounts[1] >= total
		case "col_full_rare":
			total := b.db.GetTotalCardsByRarity(3)
			passed = total > 0 && rarityCounts[3] >= total
		default:
			passed = a.Check(stats, unique, rarityCounts, maxCopies)
		}

		if passed {
			if b.db.UnlockAchievement(userID, a.ID) {
				b.db.UpdateBalance(userID, userName, a.Reward)
				msg := fmt.Sprintf("🏆 Досягнення розблоковано!\n%s *%s*\n_%s_\n+%d 🪙", a.Emoji, a.Name, a.Description, a.Reward)
				if a.Title != "" {
					msg += fmt.Sprintf("\n\nНовий титул: *%s* — /title %s", a.Title, a.Title)
				}
				c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
			}
		}
	}
}

func (b *Bot) unlockSpecial(c tele.Context, userID, userName, achievementID string) {
	a, ok := achievementByID[achievementID]
	if !ok {
		return
	}
	if b.db.UnlockAchievement(userID, a.ID) {
		b.db.UpdateBalance(userID, userName, a.Reward)
		msg := fmt.Sprintf("🏆 Досягнення розблоковано!\n%s *%s*\n_%s_\n+%d 🪙", a.Emoji, a.Name, a.Description, a.Reward)
		if a.Title != "" {
			msg += fmt.Sprintf("\n\nНовий титул: *%s* — /title %s", a.Title, a.Title)
		}
		c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}
}

func (b *Bot) handleAchievements(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)

	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏆 *Досягнення* (%d/%d)\n\n", len(unlocked), len(allAchievements)))

	count := 0
	for _, a := range allAchievements {
		if unlockedSet[a.ID] {
			sb.WriteString(fmt.Sprintf("✅ %s %s\n", a.Emoji, a.Name))
			count++
		}
	}

	if count == 0 {
		sb.WriteString("_Поки немає досягнень_\n")
	}

	sb.WriteString("\n🔒 *Наступні:*\n")
	shown := 0
	for _, a := range allAchievements {
		if unlockedSet[a.ID] || a.Hidden {
			continue
		}
		if shown >= 5 {
			break
		}
		sb.WriteString(fmt.Sprintf("%s %s — _%s_\n", a.Emoji, a.Name, a.Description))
		shown++
	}

	secretCount := 0
	for _, a := range allAchievements {
		if a.Hidden && unlockedSet[a.ID] {
			secretCount++
		}
	}
	sb.WriteString(fmt.Sprintf("\n🔮 Секретні: %d/7\n", secretCount))
	sb.WriteString(fmt.Sprintf("\n🌐 https://fuck-work-bot.fly.dev/collection/%s", userID))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (b *Bot) handleTitle(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	payload := strings.TrimSpace(c.Message().Payload)

	if payload == "" {
		unlocked := b.db.GetUnlockedAchievements(userID)
		unlockedSet := make(map[string]bool)
		for _, id := range unlocked {
			unlockedSet[id] = true
		}

		var titles []string
		for _, a := range allAchievements {
			if a.Title != "" && unlockedSet[a.ID] {
				titles = append(titles, a.Title)
			}
		}

		current := b.db.GetActiveTitle(userID)
		if len(titles) == 0 {
			return c.Reply("У тебе ще немає титулів. Заробляй досягнення!")
		}

		var sb strings.Builder
		sb.WriteString("🏷 *Доступні титули:*\n\n")
		for _, t := range titles {
			bonusDesc := ""
			if b, ok := titleBonuses[t]; ok && b.Description != "" {
				bonusDesc = " — _" + b.Description + "_"
			}
			if t == current {
				sb.WriteString(fmt.Sprintf("  ▸ *%s* (активний)%s\n", t, bonusDesc))
			} else {
				sb.WriteString(fmt.Sprintf("  ▸ %s%s\n", t, bonusDesc))
			}
		}
		sb.WriteString("\n/title <назва> — встановити\n/title off — прибрати")
		return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	if payload == "off" {
		b.db.SetActiveTitle(userID, "")
		return c.Reply("🏷 Титул прибрано")
	}

	unlocked := b.db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlocked {
		unlockedSet[id] = true
	}

	for _, a := range allAchievements {
		if a.Title != "" && unlockedSet[a.ID] && strings.EqualFold(a.Title, payload) {
			b.db.SetActiveTitle(userID, a.Title)
			return c.Reply(fmt.Sprintf("🏷 Титул встановлено: *%s*", a.Title), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
		}
	}

	return c.Reply("❌ Титул не знайдено або не розблоковано")
}

func is3AMKyiv() bool {
	kyiv := kyivLocation()
	hour := time.Now().In(kyiv).Hour()
	return hour == 3
}

func isNightKyiv() bool {
	kyiv := kyivLocation()
	hour := time.Now().In(kyiv).Hour()
	return hour >= 0 && hour < 6
}
