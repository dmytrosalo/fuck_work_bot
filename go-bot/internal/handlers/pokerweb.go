package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

// pokerTmpl is a placeholder page. A later task replaces this with the real
// mini-app UI; this stub only exists so the tree builds and /poker/{id}
// resolves to something.
var pokerTmpl = template.Must(template.New("poker").Parse(`<!doctype html><meta charset="utf-8"><title>Покер</title>`))

// subscriber is one open SSE connection watching a table.
type subscriber struct {
	userID string
	ch     chan poker.TableView
}

// PokerHub owns every live poker table and the SSE subscribers watching
// them. It is the only thing allowed to mutate a poker.Table from outside
// the poker package: the engine itself is lock-free (so it stays trivially
// testable in isolation), so this hub is responsible for serializing access
// to each table via table.Lock()/Unlock() around every Sit/Act/Showdown/
// ViewFor call.
//
// Lock ordering: a table's own lock is always the OUTER lock, h.mu is
// always the INNER lock. broadcast() takes h.mu while the caller already
// holds the table lock — never the reverse.
type PokerHub struct {
	db    *storage.DB
	bot   *tele.Bot
	token string

	// isMember checks whether userID is a member of the Telegram chat
	// chatID. It defaults to a bot-backed check (see defaultIsMember) but
	// is a field, not a hardcoded call, so tests can stub it instead of
	// relying on a nil bot to skip the check.
	isMember func(chatID, userID int64) (bool, error)

	mu     sync.Mutex
	tables map[string]*poker.Table
	subs   map[string][]*subscriber
}

func NewPokerHub(db *storage.DB, bot *tele.Bot, token string) *PokerHub {
	return &PokerHub{
		db:       db,
		bot:      bot,
		token:    token,
		isMember: defaultIsMember(bot),
		tables:   map[string]*poker.Table{},
		subs:     map[string][]*subscriber{},
	}
}

// defaultIsMember returns the production chat-membership checker backed by
// bot. If bot is nil, membership can never be verified, so the checker
// fails CLOSED: it always reports "not a member" rather than silently
// granting access, unlike the old behaviour of skipping the check entirely.
func defaultIsMember(bot *tele.Bot) func(chatID, userID int64) (bool, error) {
	if bot == nil {
		return func(chatID, userID int64) (bool, error) {
			return false, nil
		}
	}
	return func(chatID, userID int64) (bool, error) {
		m, err := bot.ChatMemberOf(&tele.Chat{ID: chatID}, &tele.User{ID: userID})
		if err != nil {
			return false, err
		}
		switch m.Role {
		case tele.Creator, tele.Administrator, tele.Member, tele.Restricted:
			return true, nil
		default:
			return false, nil
		}
	}
}

// Create allocates a new table for the given chat and registers it in the
// hub under a fresh random id.
func (h *PokerHub) Create(chatID int64) *poker.Table {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	tbl := poker.NewTable(id, chatID)
	h.mu.Lock()
	h.tables[id] = tbl
	h.mu.Unlock()
	return tbl
}

func (h *PokerHub) table(id string) *poker.Table {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tables[id]
}

// tableIDFrom extracts the table id and sub-action from /api/poker/{id}/{action}.
func tableIDFrom(path string) (id, action string) {
	rest := strings.TrimPrefix(path, "/api/poker/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// auth verifies initData and chat membership, returning the authenticated
// Telegram user id and profile fields. On failure it returns a non-zero
// HTTP status code the caller should respond with.
//
// The raw initData string is handed to verifyInitData untouched — it is
// never parsed here first, and never re-parsed afterwards. A second parser
// with different duplicate-key semantics is the only way a known,
// currently-unexploitable duplicate-key issue in initData parsing becomes
// an actual auth bypass.
func (h *PokerHub) auth(r *http.Request, tbl *poker.Table) (uid int64, firstName, username string, status int) {
	initData := r.Header.Get("X-Telegram-Init-Data")
	if initData == "" {
		initData = r.URL.Query().Get("init_data")
	}
	if initData == "" {
		return 0, "", "", http.StatusUnauthorized
	}
	uid, firstName, username, err := verifyInitData(initData, h.token, 24*time.Hour)
	if err != nil {
		return 0, "", "", http.StatusUnauthorized
	}
	ok, err := h.isMember(tbl.ChatID, uid)
	if err != nil || !ok {
		return 0, "", "", http.StatusForbidden
	}
	return uid, firstName, username, 0
}

// Register wires the poker HTTP surface into mux: the join/stream/action
// API under /api/poker/{id}/{action} and the mini-app page under
// /poker/{id}.
func (h *PokerHub) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/poker/", func(w http.ResponseWriter, r *http.Request) {
		id, action := tableIDFrom(r.URL.Path)
		tbl := h.table(id)
		if tbl == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		uid, firstName, username, status := h.auth(r, tbl)
		if status != 0 {
			msg := "Відкрий через кнопку в чаті"
			if status == http.StatusForbidden {
				msg = "Ти не з цього чату"
			}
			http.Error(w, msg, status)
			return
		}
		switch action {
		case "join":
			h.handleJoin(w, tbl, uid, firstName, username)
		case "stream":
			h.handleStream(w, r, tbl, uid)
		case "action":
			h.handleAction(w, r, tbl, uid)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/poker/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/poker/")
		if h.table(id) == nil {
			http.Error(w, "Стіл закрито", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pokerTmpl.Execute(w, map[string]string{"TableID": id})
	})
}

// handleJoin seats the authenticated user with min(balance, MaxBuyIn) chips.
// The engine itself rejects a buy-in below MinBuyIn.
func (h *PokerHub) handleJoin(w http.ResponseWriter, tbl *poker.Table, uid int64, firstName, username string) {
	userID := fmt.Sprintf("%d", uid)
	name := resolveTarget(firstName, username)

	balance := 0
	if h.db != nil {
		balance = h.db.GetBalance(userID, name)
	}
	buyIn := balance
	if buyIn > poker.MaxBuyIn {
		buyIn = poker.MaxBuyIn
	}

	tbl.Lock()
	if err := tbl.Sit(userID, name, buyIn); err != nil {
		tbl.Unlock()
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	view := tbl.ViewFor(userID)
	h.broadcast(tbl) // called with the table lock held, per lock ordering
	tbl.Unlock()

	writeJSON(w, view)
}

// handleAction applies one player action, settles the hand exactly once if
// it just reached showdown, and returns the actor's fresh view.
//
// amount is untrusted client input. It is passed straight through to
// Act, which is the sole authority on whether it is legal — this handler
// neither clamps nor otherwise "sanitizes" it, since doing so could turn an
// invalid action into a valid one.
func (h *PokerHub) handleAction(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	var body struct {
		Action string `json:"action"`
		Amount int    `json:"amount"`
		Seq    uint64 `json:"seq"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Некоректний запит", http.StatusBadRequest)
		return
	}
	userID := fmt.Sprintf("%d", uid)

	tbl.Lock()
	defer tbl.Unlock()

	if body.Seq != tbl.Seq {
		http.Error(w, "Застаріла дія, онови стан", http.StatusConflict)
		return
	}

	prevStage := tbl.Stage
	if err := tbl.Act(userID, poker.Action(body.Action), body.Amount); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	// Settle exactly once: only on the single transition into showdown, not
	// merely "whenever we currently observe StageShowdown". Act() itself
	// refuses any further action once the hand is over (StageWaiting or
	// StageShowdown), so this branch cannot be entered twice for the same
	// hand even without the engine's own internal settled guard.
	if tbl.Stage == poker.StageShowdown && prevStage != poker.StageShowdown {
		h.settle(tbl)
	}

	view := tbl.ViewFor(userID)
	h.broadcast(tbl)
	writeJSON(w, view)
}

// settle writes each player's showdown delta to the currency database. The
// caller must already hold tbl.Lock().
func (h *PokerHub) settle(tbl *poker.Table) {
	deltas := tbl.Showdown()
	if h.db == nil {
		return
	}
	for _, s := range tbl.Seats {
		d, ok := deltas[s.UserID]
		if !ok || d == 0 {
			continue
		}
		// The empty name is required: it preserves the player's existing
		// display name rather than overwriting it with a stale one.
		h.db.UpdateBalance(s.UserID, "", d)
		h.db.LogTransaction(s.UserID, s.Name, "poker", d)
	}
}

func (h *PokerHub) handleStream(w http.ResponseWriter, r *http.Request, tbl *poker.Table, uid int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Стрімінг не підтримується", http.StatusInternalServerError)
		return
	}
	userID := fmt.Sprintf("%d", uid)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sub := &subscriber{userID: userID, ch: make(chan poker.TableView, 4)}
	h.mu.Lock()
	h.subs[tbl.ID] = append(h.subs[tbl.ID], sub)
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		list := h.subs[tbl.ID]
		for i, s := range list {
			if s == sub {
				h.subs[tbl.ID] = append(list[:i], list[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
	}()

	// The hub mutex is released above before we ever touch the table lock:
	// register-then-release, then take the table lock separately for the
	// initial snapshot. Nesting them the other way risks deadlock against
	// broadcast(), which takes h.mu while holding the table lock.
	tbl.Lock()
	initial := tbl.ViewFor(userID)
	tbl.Unlock()
	sendView(w, flusher, initial)

	for {
		select {
		case <-r.Context().Done():
			return
		case v := <-sub.ch:
			sendView(w, flusher, v)
		}
	}
}

func sendView(w http.ResponseWriter, f http.Flusher, v poker.TableView) {
	raw, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", raw)
	f.Flush()
}

// broadcast pushes a fresh, individually-redacted snapshot to every viewer
// of tbl. The caller must already hold tbl.Lock() — broadcast takes h.mu
// as the inner lock, never the other way around.
//
// A subscriber's channel send never blocks: a full channel means a slow
// consumer, so that update is dropped and the next snapshot repairs it.
func (h *PokerHub) broadcast(tbl *poker.Table) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.subs[tbl.ID] {
		select {
		case s.ch <- tbl.ViewFor(s.userID):
		default: // slow consumer: drop, the next snapshot repairs it
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
