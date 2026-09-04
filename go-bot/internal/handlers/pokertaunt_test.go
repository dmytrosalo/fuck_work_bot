package handlers

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
)

func tauntHub(t *testing.T) (*PokerHub, *storage.DB, *poker.Table) {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "taunt.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewPokerHub(db, nil, "tok")
	tbl := h.Create(-1)
	tbl.Lock()
	_ = tbl.Sit("42", "Dmytro", 5000)
	_ = tbl.SitBot("bot:1", "Director Bo", 5000)
	tbl.Unlock()
	return h, db, tbl
}

// A bot that wins should sometimes speak, and when it does the line must be
// attributed to the bot and drawn from the group's own roast table.
func TestBotTauntUsesAPersonalRoast(t *testing.T) {
	h, db, tbl := tauntHub(t)
	db.AddRoast("personal", "Dmytro", "Dmytro знову programує без тестів")

	deltas := map[string]int{"bot:1": 500, "42": -500}
	// tauntChance is probabilistic; drive it until it speaks rather than
	// asserting on one roll.
	var got []chatMsg
	for i := 0; i < 200 && len(got) == 0; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
		got = h.chatSnapshot(tbl.ID)
	}
	if len(got) == 0 {
		t.Fatal("bot never said anything in 200 wins")
	}
	if len(got) != 1 {
		t.Errorf("cooldown let %d taunts through in one burst, want at most 1", len(got))
	}
	if got[0].Name != "Director Bo" {
		t.Errorf("line attributed to %q, want the winning bot", got[0].Name)
	}
	if !strings.Contains(got[0].Text, "Dmytro") {
		t.Errorf("roast %q was not the one targeted at the loser", got[0].Text)
	}
}

// No bot won the hand: there is nothing to crow about.
func TestBotStaysQuietWhenAHumanWins(t *testing.T) {
	h, db, tbl := tauntHub(t)
	db.AddRoast("personal", "Dmytro", "щось образливе")

	deltas := map[string]int{"bot:1": -500, "42": 500}
	for i := 0; i < 200; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
	}
	if msgs := h.chatSnapshot(tbl.ID); len(msgs) != 0 {
		t.Errorf("bot taunted after LOSING: %+v", msgs)
	}
}

// Empty roast and quote tables must produce silence, not blank messages.
func TestBotSaysNothingWithNoContent(t *testing.T) {
	h, _, tbl := tauntHub(t)
	deltas := map[string]int{"bot:1": 500, "42": -500}
	for i := 0; i < 200; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
	}
	for _, m := range h.chatSnapshot(tbl.ID) {
		if strings.TrimSpace(m.Text) == "" {
			t.Error("posted a blank line when the content tables were empty")
		}
	}
}

// Bot chatter shares the players' log and cap, so it can never grow the
// payload that rides every state broadcast.
func TestBotChatterRespectsTheHistoryCap(t *testing.T) {
	h, db, tbl := tauntHub(t)
	db.AddRoast("personal", "Dmytro", "коротко")
	for i := 0; i < chatHistory*3; i++ {
		tbl.Lock()
		h.appendChatFrom(tbl.ID, "Director Bo", "line")
		tbl.Unlock()
	}
	if n := len(h.chatSnapshot(tbl.ID)); n != chatHistory {
		t.Errorf("log holds %d messages, want it capped at %d", n, chatHistory)
	}
}

func TestBotChatterTruncatesLongLines(t *testing.T) {
	h, _, tbl := tauntHub(t)
	tbl.Lock()
	h.appendChatFrom(tbl.ID, "Director Bo", strings.Repeat("я", chatMaxLen+80))
	tbl.Unlock()
	msgs := h.chatSnapshot(tbl.ID)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages", len(msgs))
	}
	if n := len([]rune(msgs[0].Text)); n != chatMaxLen {
		t.Errorf("line is %d runes, want it capped at %d", n, chatMaxLen)
	}
}

// The cooldown is what actually stops spam: probability alone allows two
// taunts on consecutive hands. Even winning every hand in a tight loop must
// produce a single line until the gap has passed.
func TestBotTauntCooldownPreventsBursts(t *testing.T) {
	h, db, tbl := tauntHub(t)
	db.AddRoast("personal", "Dmytro", "щось образливе")
	deltas := map[string]int{"bot:1": 500, "42": -500}

	for i := 0; i < 500; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
	}
	if n := len(h.chatSnapshot(tbl.ID)); n > 1 {
		t.Errorf("500 consecutive bot wins produced %d taunts, want at most 1", n)
	}

	// Once the gap has passed, they may speak again.
	h.mu.Lock()
	h.lastTauntAt[tbl.ID] = time.Now().Add(-2 * tauntCooldown)
	h.mu.Unlock()
	for i := 0; i < 200 && len(h.chatSnapshot(tbl.ID)) < 2; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
	}
	if n := len(h.chatSnapshot(tbl.ID)); n < 2 {
		t.Errorf("bots stayed silent after the cooldown expired (%d taunts)", n)
	}
}

// A hand where the content tables yield nothing must not burn the cooldown,
// or empty tables would silence the bots for minutes at a time.
func TestEmptyContentDoesNotBurnTheCooldown(t *testing.T) {
	h, _, tbl := tauntHub(t) // no roasts, no quotes
	deltas := map[string]int{"bot:1": 500, "42": -500}
	for i := 0; i < 50; i++ {
		tbl.Lock()
		h.botTaunt(tbl, deltas)
		tbl.Unlock()
	}
	h.mu.Lock()
	_, stamped := h.lastTauntAt[tbl.ID]
	h.mu.Unlock()
	if stamped {
		t.Error("cooldown was stamped despite nothing being said")
	}
}
