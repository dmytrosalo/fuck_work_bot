package handlers

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

// --- Steal ---

func (b *Bot) handleSteal(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	targetName, targetUsername := getTarget(c)
	if targetName == userName && c.Message().Payload == "" && c.Message().ReplyTo == nil {
		return c.Reply("Вкажи жертву: /steal @username або відповідай на повідомлення")
	}

	target := resolveTarget(targetName, targetUsername)
	targetID := ""
	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		targetID = fmt.Sprintf("%d", c.Message().ReplyTo.Sender.ID)
	} else {
		if id, found := b.db.FindUserByName(target); found {
			targetID = id
		} else if id, found := b.db.FindUserByName(targetName); found {
			targetID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", targetName))
		}
	}

	if userID == targetID {
		return c.Reply("Не можна красти у себе 🤦")
	}

	// Check cooldown
	today := time.Now().Format("2006-01-02")
	stealKey := "steal:" + userID + ":" + today
	if b.db.GetMeta(stealKey) != "" {
		return c.Reply("🕐 Ти вже крав сьогодні. Приходь завтра!")
	}

	// 30% success, 70% fail
	if rand.Intn(100) < 30 {
		// Success — steal random card
		card := b.db.GetRandomCollectionCard(targetID)
		if card.ID == 0 {
			return c.Reply(fmt.Sprintf("У %s немає карток!", targetName))
		}
		b.db.TransferCard(targetID, userID, card.ID)
		b.db.SetMeta(stealKey, "done")
		return c.Reply(fmt.Sprintf("🦹 Вкрав %s %s у %s!", card.Emoji, card.Name, targetName))
	}

	// Fail — lose 20 coins, victim gets them
	b.db.UpdateBalance(userID, userName, -20)
	b.db.UpdateBalance(targetID, targetName, 20)
	b.db.SetMeta(stealKey, "done")
	bal := b.db.GetBalance(userID, "")
	return c.Reply(fmt.Sprintf("🚨 Спіймали! -%d 🪙 (баланс: %d)\n%s отримує +20 🪙 як компенсацію", 20, bal, targetName))
}

// --- Gift ---

func (b *Bot) handleGift(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	args := strings.Fields(c.Message().Payload)
	if len(args) < 2 {
		return c.Reply("Формат: /gift @username назва картки")
	}

	recipientName := strings.TrimPrefix(args[0], "@")
	cardQuery := strings.Join(args[1:], " ")

	resolved := resolveTarget(recipientName, recipientName)
	recipientID := ""
	if id, found := b.db.FindUserByName(resolved); found {
		recipientID = id
		recipientName = resolved
	} else if id, found := b.db.FindUserByName(recipientName); found {
		recipientID = id
	} else {
		return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", recipientName))
	}

	// Find card by name
	card := b.db.FindCardByName(cardQuery)
	if card.ID == 0 {
		return c.Reply(fmt.Sprintf("❌ Картка '%s' не знайдена", cardQuery))
	}

	// Check if user has this card
	if !b.db.TransferCard(userID, recipientID, card.ID) {
		return c.Reply(fmt.Sprintf("❌ У тебе немає картки %s %s", card.Emoji, card.Name))
	}

	return c.Reply(fmt.Sprintf("🎁 %s подарував %s %s для %s!", userName, card.Emoji, card.Name, recipientName))
}

// --- Burn ---

var burnRewards = map[int]int{
	1: 5,    // Common
	2: 10,   // Uncommon
	3: 25,   // Rare
	4: 50,   // Epic
	5: 100,  // Legendary
	6: 500,  // Ultra Legendary
}

func (b *Bot) handleBurn(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	cardQuery := strings.TrimSpace(c.Message().Payload)
	if cardQuery == "" {
		return c.Reply("Формат: /burn назва картки\nCommon=5🪙, Uncommon=10, Rare=25, Epic=50, Legendary=100")
	}

	card := b.db.FindCardByName(cardQuery)
	if card.ID == 0 {
		return c.Reply(fmt.Sprintf("❌ Картка '%s' не знайдена", cardQuery))
	}

	// Check if user has it
	if !b.db.RemoveFromCollection(userID, card.ID) {
		return c.Reply(fmt.Sprintf("❌ У тебе немає картки %s %s", card.Emoji, card.Name))
	}

	reward := burnRewards[card.Rarity]
	newBal := b.db.UpdateBalance(userID, userName, reward)
	return c.Reply(fmt.Sprintf("🔥 Спалив %s %s!\n+%d 🪙 (баланс: %d)", card.Emoji, card.Name, reward, newBal))
}

// --- Sacrifice ---

var sacrificeCost = map[int]int{
	1: 7, // 7 common → 1 uncommon
	2: 7, // 7 uncommon → 1 rare
	3: 7, // 7 rare → 1 epic
	4: 7, // 7 epic → 1 legendary
}

var sacrificeUpgrade = map[int]int{
	1: 2,
	2: 3,
	3: 4,
	4: 5,
}

func (b *Bot) handleSacrifice(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)

	args := strings.TrimSpace(c.Message().Payload)
	if args == "" {
		return c.Reply("Формат: /sacrifice <rarity>\nНаприклад: /sacrifice common\nПотрібно 3 картки → отримаєш 1 вищої рідкості")
	}

	rarityMap := map[string]int{
		"common": 1, "uncommon": 2, "rare": 3, "epic": 4,
		"1": 1, "2": 2, "3": 3, "4": 4,
	}

	rarity, ok := rarityMap[strings.ToLower(args)]
	if !ok {
		return c.Reply("Варіанти: common, uncommon, rare, epic")
	}

	targetRarity, ok := sacrificeUpgrade[rarity]
	if !ok || rarity >= 5 {
		return c.Reply("❌ Legendary та Ultra Legendary не можна апгрейдити!")
	}

	cost := sacrificeCost[rarity]

	// Get cards of this rarity from collection
	cards := b.db.GetCollectionByRarity(userID, rarity)
	if len(cards) < cost {
		return c.Reply(fmt.Sprintf("❌ Потрібно %d %s карток, у тебе %d", cost, rarityNames[rarity], len(cards)))
	}

	// Remove cards
	removed := b.db.RemoveCardsByRarity(userID, rarity, cost)
	if removed < cost {
		return c.Reply("❌ Не вдалося забрати картки")
	}

	// Give 1 card of higher rarity
	newCard := b.db.GetRandomCard(targetRarity)
	if newCard.ID == 0 {
		return c.Reply("❌ Немає карток вищої рідкості")
	}
	b.db.AddToCollection(userID, newCard.ID)

	return c.Reply(fmt.Sprintf("✨ Жертвоприношення!\n%dx %s → 1x %s\n\nОтримано: %s %s %s",
		cost, rarityNames[rarity], rarityNames[targetRarity],
		rarityStars[targetRarity], newCard.Emoji, newCard.Name))
}

// --- Showcase ---

func (b *Bot) handleShowcase(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	// Find user's rarest card
	card := b.db.GetRarestCard(userID)
	if card.ID == 0 {
		return c.Reply("У тебе ще немає карток! /pack")
	}

	power := card.ATK + card.DEF + card.Special
	unique, total := b.db.GetCollectionStats(userID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🏅 *%s показує свою найкрутішу картку!*\n\n", userName))
	sb.WriteString(fmt.Sprintf("%s %s\n", rarityStars[card.Rarity], rarityNames[card.Rarity]))
	sb.WriteString(fmt.Sprintf("%s *%s*\n\n", card.Emoji, card.Name))
	sb.WriteString(fmt.Sprintf("⚔️ ATK: %d  🛡 DEF: %d\n", card.ATK, card.DEF))
	sb.WriteString(fmt.Sprintf("%s: %d\n", card.SpecialName, card.Special))
	sb.WriteString(fmt.Sprintf("💪 PWR: *%d*\n\n", power))
	sb.WriteString(fmt.Sprintf("🃏 Колекція: %d/%d", unique, total))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// --- Gacha (premium pack) ---

func (b *Bot) handleGacha(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	cost := 300
	balance := b.db.GetBalance(userID, userName)
	if balance < cost {
		return c.Reply(fmt.Sprintf("💸 Преміум пак коштує %d 🪙, у тебе %d", cost, balance))
	}

	b.db.UpdateBalance(userID, userName, -cost)

	// 1 card: guaranteed epic+
	var cards []CardData
	for i := 0; i < 1; i++ {
		rarity := rollGachaRarity()
		fc := b.db.GetRandomCard(rarity)
		if fc.ID == 0 {
			continue
		}
		cards = append(cards, CardData{
			Name: fc.Name, Rarity: fc.Rarity, Emoji: fc.Emoji,
			Description: fc.Description, ATK: fc.ATK, DEF: fc.DEF,
			SpecialName: fc.SpecialName, Special: fc.Special,
		})
		b.db.AddToCollection(userID, fc.ID)
	}

	unique, total := b.db.GetCollectionStats(userID)
	newBal := b.db.GetBalance(userID, "")

	var sb strings.Builder
	sb.WriteString("💎 *Преміум пак!* (100 🪙)\n━━━━━━━━━━━━━━━━\n\n")
	for _, card := range cards {
		sb.WriteString(fmt.Sprintf("%s %s\n", rarityStars[card.Rarity], rarityNames[card.Rarity]))
		sb.WriteString(fmt.Sprintf("%s %s\n", card.Emoji, card.Name))
		sb.WriteString(fmt.Sprintf("⚔️%d  🛡%d  %s: %d\n\n", card.ATK, card.DEF, card.SpecialName, card.Special))
	}
	sb.WriteString(fmt.Sprintf("━━━━━━━━━━━━━━━━\n🃏 %d/%d | 🪙 %d", unique, total, newBal))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// rollGachaRarity: guaranteed epic+
// 60% epic, 30% legendary, 10% ultra
func rollGachaRarity() int {
	r := rand.Intn(100)
	switch {
	case r < 60:
		return 4
	case r < 90:
		return 5
	default:
		return 6
	}
}

// --- Card Info ---

func (b *Bot) handleCardInfo(c tele.Context) error {
	query := strings.TrimSpace(c.Message().Payload)
	if query == "" {
		return c.Reply("Формат: /card назва картки")
	}

	card := b.db.FindCardByName(query)
	if card.ID == 0 {
		return c.Reply(fmt.Sprintf("❌ Картка '%s' не знайдена", query))
	}

	power := card.ATK + card.DEF + card.Special
	cardData := CardData{
		Name: card.Name, Rarity: card.Rarity, Emoji: card.Emoji,
		Description: "", ATK: card.ATK, DEF: card.DEF,
		SpecialName: card.SpecialName, Special: card.Special,
	}

	// Try render image
	imgBytes, err := renderCard(cardData)
	if err == nil {
		photo := &tele.Photo{
			File:    tele.FromReader(bytes.NewReader(imgBytes)),
			Caption: fmt.Sprintf("%s %s — PWR: %d", card.Emoji, card.Name, power),
		}
		return c.Send(photo)
	}

	// Fallback text
	return c.Reply(fmt.Sprintf("%s %s\n%s %s\n⚔️%d 🛡%d %s: %d\nPWR: %d",
		rarityStars[card.Rarity], rarityNames[card.Rarity],
		card.Emoji, card.Name,
		card.ATK, card.DEF, card.SpecialName, card.Special, power))
}

// --- Auction ---

type auctionState struct {
	SellerID   string
	SellerName string
	Card       storage.BattleCard
	ChatID     int64
	HighBid    int
	HighBidder string
	BidderName string
	EndsAt     time.Time
}

var (
	activeAuctions = make(map[int64]*auctionState)
	auctionMu      sync.Mutex
)

func (b *Bot) handleAuction(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	chatID := c.Chat().ID

	cardQuery := strings.TrimSpace(c.Message().Payload)
	if cardQuery == "" {
		return c.Reply("Формат: /auction назва картки")
	}

	auctionMu.Lock()
	if _, ok := activeAuctions[chatID]; ok {
		auctionMu.Unlock()
		return c.Reply("❌ В цьому чаті вже є активний аукціон! Чекай завершення")
	}
	auctionMu.Unlock()

	card := b.db.FindCardByName(cardQuery)
	if card.ID == 0 {
		return c.Reply(fmt.Sprintf("❌ Картка '%s' не знайдена", cardQuery))
	}

	// Check ownership
	battleCard := b.db.GetSpecificCollectionCard(userID, card.ID)
	if battleCard.ID == 0 {
		return c.Reply(fmt.Sprintf("❌ У тебе немає %s %s", card.Emoji, card.Name))
	}

	startBid := burnRewards[card.Rarity] // Minimum = burn value

	auctionMu.Lock()
	activeAuctions[chatID] = &auctionState{
		SellerID:   userID,
		SellerName: userName,
		Card:       battleCard,
		ChatID:     chatID,
		HighBid:    startBid,
		EndsAt:     time.Now().Add(60 * time.Second),
	}
	auctionMu.Unlock()

	msg := fmt.Sprintf("🔨 *АУКЦІОН!*\n\n%s продає:\n%s %s %s\n⚔️%d 🛡%d %s: %d\n\nСтартова ціна: %d 🪙\nНапиши /bid <сума> (60 сек)",
		userName, rarityStars[battleCard.Rarity], battleCard.Emoji, battleCard.Name,
		battleCard.ATK, battleCard.DEF, battleCard.SpecialName, battleCard.Special,
		startBid)

	c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})

	// Auto-close after 60 seconds
	go func() {
		time.Sleep(61 * time.Second)
		b.closeAuction(c.Bot(), chatID)
	}()

	return nil
}

func (b *Bot) handleBid(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	chatID := c.Chat().ID

	bidStr := strings.TrimSpace(c.Message().Payload)
	bid, err := strconv.Atoi(bidStr)
	if err != nil || bid < 1 {
		return c.Reply("Формат: /bid <сума>")
	}

	auctionMu.Lock()
	auction, ok := activeAuctions[chatID]
	if !ok {
		auctionMu.Unlock()
		return c.Reply("❌ Немає активного аукціону")
	}

	if userID == auction.SellerID {
		auctionMu.Unlock()
		return c.Reply("❌ Не можна ставити на свій аукціон")
	}

	if bid <= auction.HighBid {
		auctionMu.Unlock()
		return c.Reply(fmt.Sprintf("❌ Ставка має бути більше %d 🪙", auction.HighBid))
	}

	balance := b.db.GetBalance(userID, userName)
	if balance < bid {
		auctionMu.Unlock()
		return c.Reply(fmt.Sprintf("❌ Недостатньо! Баланс: %d 🪙", balance))
	}

	auction.HighBid = bid
	auction.HighBidder = userID
	auction.BidderName = userName
	auctionMu.Unlock()

	return c.Reply(fmt.Sprintf("💰 %s ставить %d 🪙!", userName, bid))
}

func (b *Bot) closeAuction(bot *tele.Bot, chatID int64) {
	auctionMu.Lock()
	auction, ok := activeAuctions[chatID]
	if !ok {
		auctionMu.Unlock()
		return
	}
	delete(activeAuctions, chatID)
	auctionMu.Unlock()

	chat := &tele.Chat{ID: chatID}

	if auction.HighBidder == "" {
		bot.Send(chat, fmt.Sprintf("🔨 Аукціон завершено! Ніхто не зробив ставку. %s %s залишається у %s",
			auction.Card.Emoji, auction.Card.Name, auction.SellerName))
		return
	}

	// Transfer card and coins
	b.db.TransferCard(auction.SellerID, auction.HighBidder, auction.Card.ID)
	b.db.TransferCoins(auction.HighBidder, auction.SellerID, auction.HighBid)

	msg := fmt.Sprintf("🔨 *Аукціон завершено!*\n\n%s %s продано!\n\n%s → %s\nЦіна: %d 🪙",
		auction.Card.Emoji, auction.Card.Name,
		auction.SellerName, auction.BidderName, auction.HighBid)
	bot.Send(chat, msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
