package handlers

import (
	"fmt"
	"os"

	tele "gopkg.in/telebot.v3"
)

// publicBaseURL returns the base URL to build Mini App links against.
// PUBLIC_BASE_URL overrides the default production host.
func publicBaseURL() string {
	if v := os.Getenv("PUBLIC_BASE_URL"); v != "" {
		return v
	}
	return "https://fuck-work-bot.fly.dev"
}

// handlePoker creates a poker table bound to the current chat and sends a
// Mini App button to open it. The chat id always comes from the incoming
// message (c.Chat().ID) — it is the authorization anchor for the table.
func (b *Bot) handlePoker(c tele.Context) error {
	if b.poker == nil {
		return c.Send("♠️ Покер зараз недоступний")
	}
	tbl := b.poker.Create(c.Chat().ID)
	url := fmt.Sprintf("%s/poker/%s", publicBaseURL(), tbl.ID)

	markup := &tele.ReplyMarkup{}
	btn := markup.WebApp("♠️ Сісти за стіл", &tele.WebApp{URL: url})
	markup.Inline(markup.Row(btn))

	return c.Send(
		"♠️ *Покер!*\n\nМакс 6 гравців, бай-ін до 10000 🪙.\nНатисни кнопку, щоб сісти за стіл.",
		markup, tele.ModeMarkdown,
	)
}
