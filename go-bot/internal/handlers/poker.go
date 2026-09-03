package handlers

import (
	"fmt"
	"os"

	tele "gopkg.in/telebot.v3"
)

// defaultMiniAppName is the @BotFather Mini App short name assumed when
// POKER_MINIAPP is unset. It MUST match the short name registered with
// /newapp for @fuuck_work_bot — BotFather assigns the name, it is not
// chosen here, and a mismatch makes the group button open a dead t.me link
// with no server-side error to notice.
const defaultMiniAppName = "finikultramatcha"

// miniAppName returns the Mini App short name registered with @BotFather.
func miniAppName() string {
	if v := os.Getenv("POKER_MINIAPP"); v != "" {
		return v
	}
	return defaultMiniAppName
}

// miniAppLink builds the direct Mini App link for a table.
//
// Telegram refuses an inline web_app button anywhere but a private chat with
// the bot — a group message carrying one fails with
// "Bad Request: BUTTON_TYPE_INVALID" and never arrives. A direct Mini App
// link is an ordinary URL button, legal in every chat type, and still opens
// the app with signed initData.
//
// The table id travels in startapp rather than the path, because Telegram
// opens the Mini App at the fixed URL registered with @BotFather; the page
// reads it back as initDataUnsafe.start_param. Table ids are hex, so they
// need no escaping to satisfy startapp's A-Za-z0-9_- alphabet.
func miniAppLink(username, tableID string) string {
	return fmt.Sprintf("https://t.me/%s/%s?startapp=%s", username, miniAppName(), tableID)
}

// handlePoker creates a poker table bound to the current chat and sends a
// Mini App link to open it. The chat id always comes from the incoming
// message (c.Chat().ID) — it is the authorization anchor for the table.
func (b *Bot) handlePoker(c tele.Context) error {
	if b.poker == nil {
		return c.Send("♠️ Покер зараз недоступний")
	}
	tbl := b.poker.Create(c.Chat().ID)

	markup := &tele.ReplyMarkup{}
	btn := markup.URL("♠️ Сісти за стіл", miniAppLink(c.Bot().Me.Username, tbl.ID))
	markup.Inline(markup.Row(btn))

	return c.Send(
		"♠️ *Покер!*\n\nМакс 6 гравців, бай-ін до 10000 🪙.\nНатисни кнопку, щоб сісти за стіл.",
		markup, tele.ModeMarkdown,
	)
}
