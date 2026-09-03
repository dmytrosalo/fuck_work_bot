package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

func chatReq(text string) *http.Request {
	body, _ := json.Marshal(map[string]string{"text": text})
	return httptest.NewRequest(http.MethodPost, "/api/poker/t/chat", strings.NewReader(string(body)))
}

// post sends one chat message, clearing the cooldown first so a test can
// send several in a row without sleeping.
func post(h *PokerHub, tbl *poker.Table, uid int64, name, text string) *httptest.ResponseRecorder {
	h.mu.Lock()
	delete(h.lastChatAt, strconv.FormatInt(uid, 10))
	h.mu.Unlock()
	rec := httptest.NewRecorder()
	h.handleChat(rec, chatReq(text), tbl, uid, name, "")
	return rec
}

func newChatTable(h *PokerHub) *poker.Table {
	return h.Create(-1001)
}

func TestChatStoresAndReturnsMessage(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)

	rec := post(h, tbl, 1, "Dmytro", "  ти блефуєш  ")
	if rec.Code != http.StatusOK {
		t.Fatalf("chat POST = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var env tableEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if len(env.Chat) != 1 {
		t.Fatalf("chat length = %d, want 1", len(env.Chat))
	}
	if env.Chat[0].Text != "ти блефуєш" {
		t.Errorf("text = %q, want trimmed %q", env.Chat[0].Text, "ти блефуєш")
	}
	if env.Chat[0].Name != "Dmytro" {
		t.Errorf("name = %q, want Dmytro", env.Chat[0].Name)
	}
	// The envelope must still carry the table view — chat is additive, and
	// flattening it away would break every existing client field.
	if env.ID != tbl.ID {
		t.Errorf("envelope lost the table view: id = %q, want %q", env.ID, tbl.ID)
	}
}

func TestChatRejectsEmptyAndLimitsLength(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)

	for _, empty := range []string{"", "   ", "\n\t "} {
		if rec := post(h, tbl, 1, "A", empty); rec.Code != http.StatusBadRequest {
			t.Errorf("empty %q = %d, want 400", empty, rec.Code)
		}
	}

	// Cyrillic is 2 bytes per rune in UTF-8: a byte-based cap would cut this
	// to half the advertised limit and could split a rune mid-sequence.
	long := strings.Repeat("я", chatMaxLen+50)
	rec := post(h, tbl, 1, "A", long)
	if rec.Code != http.StatusOK {
		t.Fatalf("long message = %d, want 200", rec.Code)
	}
	var env tableEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	got := env.Chat[len(env.Chat)-1].Text
	if n := len([]rune(got)); n != chatMaxLen {
		t.Errorf("truncated to %d runes, want %d", n, chatMaxLen)
	}
	if !isValidUTF8(got) {
		t.Errorf("truncation split a rune: %q", got)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestChatCooldownIsServerSide(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)

	rec := httptest.NewRecorder()
	h.handleChat(rec, chatReq("раз"), tbl, 7, "A", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("first message = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.handleChat(rec, chatReq("два"), tbl, 7, "A", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("immediate second message = %d, want 429", rec.Code)
	}
	// A different user is unaffected by someone else's cooldown.
	rec = httptest.NewRecorder()
	h.handleChat(rec, chatReq("три"), tbl, 8, "B", "")
	if rec.Code != http.StatusOK {
		t.Errorf("other user = %d, want 200", rec.Code)
	}
}

func TestChatHistoryIsCapped(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)
	for i := 0; i < chatHistory+15; i++ {
		post(h, tbl, 1, "A", "msg"+strconv.Itoa(i))
	}
	msgs := h.chatSnapshot(tbl.ID)
	if len(msgs) != chatHistory {
		t.Fatalf("history = %d, want capped at %d", len(msgs), chatHistory)
	}
	// The cap must drop the OLDEST, keeping the newest visible.
	if want := "msg" + strconv.Itoa(chatHistory+14); msgs[len(msgs)-1].Text != want {
		t.Errorf("newest kept = %q, want %q", msgs[len(msgs)-1].Text, want)
	}
}

// TestChatBroadcastDoesNotDeadlock is the point of chatLocked existing.
// broadcast() runs holding h.mu; if it reached for the chat log through
// chatSnapshot (which takes h.mu) the hub would deadlock on a non-reentrant
// mutex and every table would freeze.
func TestChatBroadcastDoesNotDeadlock(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)
	post(h, tbl, 1, "A", "привіт")

	sub := &subscriber{userID: "1", ch: make(chan tableEnvelope, 4), done: make(chan struct{})}
	h.mu.Lock()
	h.subs[tbl.ID] = append(h.subs[tbl.ID], sub)
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		tbl.Lock()
		h.broadcast(tbl)
		tbl.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("broadcast deadlocked with chat present")
	}

	select {
	case env := <-sub.ch:
		if len(env.Chat) != 1 || env.Chat[0].Text != "привіт" {
			t.Errorf("broadcast chat = %+v, want the one message", env.Chat)
		}
	default:
		t.Error("broadcast delivered nothing")
	}
}

// TestChatDroppedWithTable stops the log outliving the table it belongs to.
func TestChatDroppedWithTable(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)
	post(h, tbl, 1, "A", "hi")

	h.mu.Lock()
	h.dropChat(tbl.ID)
	h.mu.Unlock()

	if msgs := h.chatSnapshot(tbl.ID); len(msgs) != 0 {
		t.Errorf("chat survived table reclaim: %+v", msgs)
	}
}

// TestChatSnapshotIsACopy guards the marshal-outside-the-lock path: handing
// out the live slice would race the next append.
func TestChatSnapshotIsACopy(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	tbl := newChatTable(h)
	post(h, tbl, 1, "A", "original")

	snap := h.chatSnapshot(tbl.ID)
	snap[0].Text = "mutated"

	if again := h.chatSnapshot(tbl.ID); again[0].Text != "original" {
		t.Errorf("snapshot aliases hub state: %q", again[0].Text)
	}
}
