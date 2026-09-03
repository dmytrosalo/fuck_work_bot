package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

const (
	// chatHistory is how many messages a table keeps. The whole log rides
	// every state broadcast (see tableEnvelope), so this is a bandwidth
	// knob as much as a scrollback one.
	chatHistory = 20
	// chatMaxLen caps a single message in RUNES, not bytes — Ukrainian text
	// is two bytes per letter in UTF-8, so a byte cap would silently halve
	// the usable length.
	chatMaxLen = 200
	// chatCooldown is the minimum gap between two messages from the same
	// user. Enforced server-side: a modified client is exactly the thing a
	// client-side limit would not stop.
	chatCooldown = 1500 * time.Millisecond
)

// chatMsg is one table-chat line. Name is player-controlled (it comes from
// the sender's Telegram profile) and is rendered with textContent on the
// client, never as markup.
type chatMsg struct {
	Name string `json:"name"`
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// tableEnvelope is what the client actually receives: the engine's view plus
// the table chat. TableView is embedded, so its fields stay at the top level
// of the JSON and the client's existing render() is untouched.
//
// Chat rides the state payload rather than a separate SSE event type on
// purpose. broadcast() drops updates for slow consumers and relies on the
// next snapshot to repair them; a standalone chat event would have no such
// repair, so a single dropped frame would lose a message permanently.
type tableEnvelope struct {
	poker.TableView
	Chat []chatMsg `json:"chat"`
}

// chatSnapshot returns a copy of a table's chat log. Takes h.mu itself, so
// callers must NOT already hold it — inside broadcast(), which does, use
// chatLocked instead.
func (h *PokerHub) chatSnapshot(tableID string) []chatMsg {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.chatLocked(tableID)
}

// chatLocked returns a copy of a table's chat log. Caller must hold h.mu.
// The copy matters: the returned slice is marshalled outside the lock, and
// handing out the live backing array would race with the next append.
func (h *PokerHub) chatLocked(tableID string) []chatMsg {
	src := h.chat[tableID]
	if len(src) == 0 {
		return nil
	}
	out := make([]chatMsg, len(src))
	copy(out, src)
	return out
}

// envelope pairs a viewer's redacted table view with the chat log. Must be
// called with the TABLE lock held and h.mu NOT held, matching the ordering
// rule the rest of the hub follows: table lock outer, hub mutex inner.
func (h *PokerHub) envelope(tbl *poker.Table, userID string) tableEnvelope {
	return tableEnvelope{TableView: tbl.ViewFor(userID), Chat: h.chatSnapshot(tbl.ID)}
}

// handleChat records one message and broadcasts the updated log.
func (h *PokerHub) handleChat(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64, firstName, username string) {
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Некоректний запит", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(body.Text)
	// Collapse newlines: the client renders each message as a single line,
	// and without this one message could occupy the whole visible log.
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		http.Error(w, "Порожнє повідомлення", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(text) > chatMaxLen {
		text = string([]rune(text)[:chatMaxLen])
	}

	userID := fmt.Sprintf("%d", uid)
	now := time.Now()

	h.mu.Lock()
	if last, ok := h.lastChatAt[userID]; ok && now.Sub(last) < chatCooldown {
		h.mu.Unlock()
		http.Error(w, "Не так швидко", http.StatusTooManyRequests)
		return
	}
	h.lastChatAt[userID] = now
	msgs := append(h.chat[tbl.ID], chatMsg{
		Name: resolveTarget(firstName, username),
		Text: text,
		At:   now.Unix(),
	})
	if len(msgs) > chatHistory {
		msgs = msgs[len(msgs)-chatHistory:]
	}
	h.chat[tbl.ID] = msgs
	h.mu.Unlock()

	// Same ordering as every other write path: take the table lock, then
	// broadcast while holding it.
	tbl.Lock()
	view := h.envelope(tbl, userID)
	h.broadcast(tbl)
	tbl.Unlock()

	writeJSON(w, view)
}

// dropChat forgets a reclaimed table's log so the map does not grow without
// bound as tables come and go. Caller must hold h.mu.
func (h *PokerHub) dropChat(tableID string) {
	delete(h.chat, tableID)
}
