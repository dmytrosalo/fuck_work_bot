package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

const maxDartsPerDay = 5

type dartChallenge struct {
	ChallengerID   string
	ChallengerName string
	OpponentID     string
	OpponentName   string
	Bet            int
	ChatID         int64
	CreatedAt      time.Time
}

var (
	activeDarts = make(map[int64]*dartChallenge)
	dartMu      sync.Mutex
)

func (b *Bot) handleDart(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}
	chatID := c.Chat().ID

	// Daily limit
	today := time.Now().Format("2006-01-02")
	dartKey := "dart:" + userID + ":" + today
	countStr := b.db.GetMeta(dartKey)
	dartCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &dartCount)
	}
	if dartCount >= maxDartsPerDay {
		return c.Reply(fmt.Sprintf("🎯 Ліміт %d дартс на день. Скидання через %s", maxDartsPerDay, timeUntilReset()))
	}

	// Parse opponent and bet
	var opponentName, opponentID string
	bet := 20

	if c.Message().ReplyTo != nil && c.Message().ReplyTo.Sender != nil {
		opponent := c.Message().ReplyTo.Sender
		opponentID = fmt.Sprintf("%d", opponent.ID)
		opponentName = opponent.FirstName
		if opponentName == "" {
			opponentName = opponent.Username
		}
		if c.Message().Payload != "" {
			parsed, _ := strconv.Atoi(strings.TrimSpace(c.Message().Payload))
			if parsed > 0 {
				bet = parsed
			}
		}
	} else {
		args := strings.Fields(c.Message().Payload)
		if len(args) < 1 {
			return c.Reply("Формат: /dart @username <ставка> або відповідай на повідомлення")
		}
		opponentName = strings.TrimPrefix(args[0], "@")
		resolved := resolveTarget(opponentName, opponentName)
		if id, found := b.db.FindUserByName(resolved); found {
			opponentID = id
			opponentName = resolved
		} else if id, found := b.db.FindUserByName(opponentName); found {
			opponentID = id
		} else {
			return c.Reply(fmt.Sprintf("❌ Гравець %s не знайдений", opponentName))
		}
		if len(args) > 1 {
			parsed, _ := strconv.Atoi(args[1])
			if parsed > 0 {
				bet = parsed
			}
		}
	}

	if userID == opponentID {
		return c.Reply("Не можна грати з самим собою 🤦")
	}
	if bet > 500 {
		bet = 500
	}

	// Check challenger balance
	bal := b.db.GetBalance(userID, userName)
	if bal < bet {
		return c.Reply(fmt.Sprintf("💸 У тебе %d 🪙, ставка %d", bal, bet))
	}

	dartMu.Lock()
	activeDarts[chatID] = &dartChallenge{
		ChallengerID:   userID,
		ChallengerName: userName,
		OpponentID:     opponentID,
		OpponentName:   opponentName,
		Bet:            bet,
		ChatID:         chatID,
		CreatedAt:      time.Now(),
	}
	dartMu.Unlock()

	return c.Send(fmt.Sprintf("🎯 %s викликає %s на дартс!\nСтавка: %d 🪙\nЦіль: 50\n\n%s, напиши /accept (60 сек)",
		userName, opponentName, bet, opponentName))
}

func (b *Bot) handleDartAccept(c tele.Context) {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID

	dartMu.Lock()
	dart, ok := activeDarts[chatID]
	if !ok || time.Since(dart.CreatedAt) > 60*time.Second {
		if ok {
			delete(activeDarts, chatID)
		}
		dartMu.Unlock()
		return
	}

	if userID != dart.OpponentID {
		dartMu.Unlock()
		return
	}

	delete(activeDarts, chatID)
	dartMu.Unlock()

	// Check both balances
	bal1 := b.db.GetBalance(dart.ChallengerID, dart.ChallengerName)
	bal2 := b.db.GetBalance(dart.OpponentID, dart.OpponentName)
	if bal1 < dart.Bet {
		c.Send(fmt.Sprintf("💸 У %s недостатньо монет!", dart.ChallengerName))
		return
	}
	if bal2 < dart.Bet {
		c.Send(fmt.Sprintf("💸 У %s недостатньо монет!", dart.OpponentName))
		return
	}

	// Both pay into pot
	b.db.UpdateBalance(dart.ChallengerID, dart.ChallengerName, -dart.Bet)
	b.db.UpdateBalance(dart.OpponentID, dart.OpponentName, -dart.Bet)
	pot := dart.Bet * 2

	// Increment daily count for both
	today := time.Now().Format("2006-01-02")
	for _, uid := range []string{dart.ChallengerID, dart.OpponentID} {
		key := "dart:" + uid + ":" + today
		countStr := b.db.GetMeta(key)
		count := 0
		if countStr != "" {
			fmt.Sscanf(countStr, "%d", &count)
		}
		b.db.SetMeta(key, fmt.Sprintf("%d", count+1))
	}

	// 5 rounds of throws
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🎯 ДАРТС — 5 раундів\nСтавка: %d 🪙 кожен | Банк: %d 🪙\nЦіль: 50\n\n", dart.Bet, pot))

	wins1, wins2 := 0, 0

	for round := 1; round <= 5; round++ {
		throw1 := rand.Intn(100) + 1
		throw2 := rand.Intn(100) + 1

		diff1 := throw1 - 50
		if diff1 < 0 {
			diff1 = -diff1
		}
		diff2 := throw2 - 50
		if diff2 < 0 {
			diff2 = -diff2
		}

		sb.WriteString(fmt.Sprintf("Раунд %d: ", round))

		if diff1 < diff2 {
			wins1++
			sb.WriteString(fmt.Sprintf("%s %d(%d) vs %d(%d) %s ← %s\n", dart.ChallengerName, throw1, diff1, throw2, diff2, dart.OpponentName, dart.ChallengerName))
		} else if diff2 < diff1 {
			wins2++
			sb.WriteString(fmt.Sprintf("%s %d(%d) vs %d(%d) %s ← %s\n", dart.ChallengerName, throw1, diff1, throw2, diff2, dart.OpponentName, dart.OpponentName))
		} else {
			sb.WriteString(fmt.Sprintf("%s %d(%d) vs %d(%d) %s — нічия\n", dart.ChallengerName, throw1, diff1, throw2, diff2, dart.OpponentName))
		}
	}

	sb.WriteString(fmt.Sprintf("\nРахунок: %s %d — %d %s\n\n", dart.ChallengerName, wins1, wins2, dart.OpponentName))

	if wins1 > wins2 {
		b.db.UpdateBalance(dart.ChallengerID, dart.ChallengerName, pot)
		sb.WriteString(fmt.Sprintf("🏆 %s забирає банк %d 🪙!", dart.ChallengerName, pot))
	} else if wins2 > wins1 {
		b.db.UpdateBalance(dart.OpponentID, dart.OpponentName, pot)
		sb.WriteString(fmt.Sprintf("🏆 %s забирає банк %d 🪙!", dart.OpponentName, pot))
	} else {
		b.db.UpdateBalance(dart.ChallengerID, dart.ChallengerName, dart.Bet)
		b.db.UpdateBalance(dart.OpponentID, dart.OpponentName, dart.Bet)
		sb.WriteString("🤝 Нічия! Ставки повернуто")
	}

	c.Send(sb.String())
}
