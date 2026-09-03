package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
	tele "gopkg.in/telebot.v3"
)

func TestJoinRejectsMissingInitData(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	tbl := h.Create(999)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing initData", rec.Code)
	}
}

func TestUnknownTableReturns404(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/poker/nope/join", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for unknown table", rec.Code)
	}
}

// --- test helpers ---------------------------------------------------------

func setupTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.New(filepath.Join(t.TempDir(), "poker-test.db"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// userInitData builds a validly-signed initData blob for a single user.
func userInitData(t *testing.T, token string, userID int64, firstName, username string) string {
	t.Helper()
	return signInitData(token, map[string]string{
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
		"user":      fmt.Sprintf(`{"id":%d,"first_name":%q,"username":%q}`, userID, firstName, username),
	})
}

// newRoleBot returns a *tele.Bot that answers every Bot API call (including
// getChatMember) against a local fake server reporting the given chat
// member role. Offline:true skips the getMe handshake NewBot would
// otherwise make against the real Telegram API.
func newRoleBot(t *testing.T, role string) *tele.Bot {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"result":{"status":%q}}`, role)
	}))
	t.Cleanup(srv.Close)

	bot, err := tele.NewBot(tele.Settings{URL: srv.URL, Token: "test-token", Offline: true})
	if err != nil {
		t.Fatalf("tele.NewBot: %v", err)
	}
	return bot
}

// stubAllowMember installs a membership checker that always allows. Use it
// in tests that exercise something other than the membership check itself
// (join/action/stream mechanics) since NewPokerHub with a nil bot now fails
// closed by default instead of skipping the check.
func stubAllowMember(h *PokerHub) {
	h.isMember = func(chatID, userID int64) (bool, error) { return true, nil }
}

func decodeView(t *testing.T, rec *httptest.ResponseRecorder) poker.TableView {
	t.Helper()
	var v poker.TableView
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode view: %v, body=%s", err, rec.Body.String())
	}
	return v
}

// --- join ------------------------------------------------------------------

// TestJoinSucceedsWithBuyInClampedToMaxBuyIn drives a real join over HTTP
// end-to-end and checks the seated stack is clamped to MaxBuyIn. NOTE: this
// does NOT pin down handleJoin's own buyIn clamp — poker.Table.Sit clamps to
// MaxBuyIn itself, so this test still passes even with the handler-side
// clamp deleted. The handler clamp is kept anyway as defence in depth (so a
// future engine change can't silently remove the ceiling), but its coverage
// lives at the poker package level, not here.
func TestJoinSucceedsWithBuyInClampedToMaxBuyIn(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 50000-100) // GetBalance seeds new rows at 100

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 111, "Alice", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	v := decodeView(t, rec)
	// 3 seats, not 1: a solo human now brings bot company (see ensureBots),
	// so Alice's join also seats 2 bots and auto-starts a hand.
	if len(v.Seats) != 3 {
		t.Fatalf("seats = %d, want 3 (Alice + 2 bots)", len(v.Seats))
	}
	if v.YouSeat < 0 || v.YouSeat >= len(v.Seats) {
		t.Fatalf("you_seat = %d, want a real seat among %d", v.YouSeat, len(v.Seats))
	}
	if v.Seats[v.YouSeat].Stack != poker.MaxBuyIn {
		t.Errorf("stack = %d, want buy-in clamped to MaxBuyIn=%d", v.Seats[v.YouSeat].Stack, poker.MaxBuyIn)
	}
}

// TestJoinRejectsSecondSeatAtDifferentTable proves one bankroll can't back
// simultaneous buy-ins at several tables: a user seated at tableA must be
// rejected, with a Ukrainian 409, when trying to also sit at tableB — even
// though tableB has room and the user's balance alone would cover both
// buy-ins.
func TestJoinRejectsSecondSeatAtDifferentTable(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 20000-100) // enough to "afford" two buy-ins

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tableA := h.Create(1)
	tableB := h.Create(2)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 111, "Alice", "")

	reqA := httptest.NewRequest("POST", "/api/poker/"+tableA.ID+"/join", nil)
	reqA.Header.Set("X-Telegram-Init-Data", initData)
	recA := httptest.NewRecorder()
	mux.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("join tableA status = %d, want 200, body=%s", recA.Code, recA.Body.String())
	}

	reqB := httptest.NewRequest("POST", "/api/poker/"+tableB.ID+"/join", nil)
	reqB.Header.Set("X-Telegram-Init-Data", initData)
	recB := httptest.NewRecorder()
	mux.ServeHTTP(recB, reqB)

	if recB.Code != http.StatusConflict {
		t.Fatalf("join tableB status = %d, want 409 for a second simultaneous seat, body=%s", recB.Code, recB.Body.String())
	}
	if !strings.Contains(recB.Body.String(), "Ти вже за іншим столом") {
		t.Errorf("body = %q, want Ukrainian already-at-another-table message", recB.Body.String())
	}

	tableB.Lock()
	seatsB := len(tableB.Seats)
	tableB.Unlock()
	if seatsB != 0 {
		t.Errorf("tableB seats = %d, want 0 — the rejected join must not have seated the player", seatsB)
	}
}

func TestJoinRejectsBuyInBelowMinimum(t *testing.T) {
	// No db means balance defaults to 0, which is below poker.MinBuyIn.
	h := NewPokerHub(nil, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 222, "Bob", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for buy-in below minimum, body=%s", rec.Code, rec.Body.String())
	}
}

func TestJoinRejectsNonChatMember(t *testing.T) {
	bot := newRoleBot(t, "left")
	h := NewPokerHub(nil, bot, "test-token")
	tbl := h.Create(42)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 333, "Carl", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for non-member, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Ти не з цього чату") {
		t.Errorf("body = %q, want Ukrainian non-member message", rec.Body.String())
	}
}

func TestJoinAllowsChatMember(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("444", "Dana", 5000-100)
	bot := newRoleBot(t, "member")
	h := NewPokerHub(db, bot, "test-token")
	tbl := h.Create(42)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 444, "Dana", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for chat member, body=%s", rec.Code, rec.Body.String())
	}
}

// --- reconnect (FIX 1: CRITICAL — an already-seated player must not be
// locked out of their own seat on reload) ------------------------------------

// TestJoinReconnectsAlreadySeatedPlayer is the merge-blocker test for FIX 1:
// a second join by the SAME user at the SAME table — exactly what happens
// when the Mini App is closed and reopened, an iOS webview gets
// backgrounded, or the player switches device mid-hand — must return 200
// with their existing seat and UNCHANGED stack, not propagate Sit's own
// error (which, for an already-seated player, is always wrong: it reads as
// "table full" or "buy-in too low" depending on Sit's internal error
// precedence, neither of which describes their actual situation).
func TestJoinReconnectsAlreadySeatedPlayer(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 3000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 111, "Alice", "")
	join := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	first := join()
	if first.Code != http.StatusOK {
		t.Fatalf("first join status = %d, want 200, body=%s", first.Code, first.Body.String())
	}
	firstView := decodeView(t, first)
	if firstView.YouSeat < 0 {
		t.Fatalf("first join: you_seat = %d, want a real seat", firstView.YouSeat)
	}
	firstStack := firstView.Seats[firstView.YouSeat].Stack

	// Simulate a reload: the client re-runs join with the same, still-valid
	// initData, exactly what the page's own bootstrap script does on every
	// load. Sit() alone would reject this outright (ErrAlreadySat).
	second := join()
	if second.Code != http.StatusOK {
		t.Fatalf("reconnect join status = %d, want 200 (must not lock the player out of their own seat), body=%s", second.Code, second.Body.String())
	}
	secondView := decodeView(t, second)
	if secondView.YouSeat != firstView.YouSeat {
		t.Errorf("reconnect you_seat = %d, want unchanged %d", secondView.YouSeat, firstView.YouSeat)
	}
	// 3 seats, not 1: Alice's first join also seated 2 bots (see
	// ensureBots). The point of this test — reconnecting must not re-seat
	// or duplicate Alice's OWN seat — still holds; only the total table
	// population changed with this task.
	if len(secondView.Seats) != 3 {
		t.Errorf("reconnect seats = %d, want still 3 (Alice + 2 bots, must not re-seat or duplicate)", len(secondView.Seats))
	}
	if secondView.Seats[secondView.YouSeat].Stack != firstStack {
		t.Errorf("reconnect stack = %d, want unchanged %d (must not re-read balance or reset stack)", secondView.Seats[secondView.YouSeat].Stack, firstStack)
	}

	tbl.Lock()
	seatCount := len(tbl.Seats)
	tbl.Unlock()
	if seatCount != 3 {
		t.Errorf("table has %d seats after reconnect, want still 3", seatCount)
	}
}

// TestJoinReconnectsAlreadySeatedPlayerAtFullTable is the exact symptom from
// the bug report: at a FULL table, every seated player reloading would see
// "стіл заповнений" (table full) from Sit's own error precedence, even
// though they already have a seat and chips in play. This proves the
// reconnect fast-path is checked BEFORE the table-full rejection would ever
// apply.
func TestJoinReconnectsAlreadySeatedPlayerAtFullTable(t *testing.T) {
	db := setupTestDB(t)
	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	// Fill the table to capacity directly against the engine (equivalent to
	// MaxSeats players having already joined over HTTP).
	for i := 0; i < poker.MaxSeats; i++ {
		userID := fmt.Sprintf("%d", 1000+i)
		if err := tbl.Sit(userID, fmt.Sprintf("U%d", i), 2000); err != nil {
			t.Fatalf("Sit seat %d: %v", i, err)
		}
	}

	// Seat 0's player reloads the Mini App at their now-full table.
	initData := userInitData(t, "test-token", 1000, "U0", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("reconnect at a full table status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	v := decodeView(t, rec)
	if v.YouSeat != 0 {
		t.Errorf("you_seat = %d, want 0 (existing seat)", v.YouSeat)
	}
	if v.Seats[0].Stack != 2000 {
		t.Errorf("stack = %d, want unchanged 2000", v.Seats[0].Stack)
	}
}

// TestJoinEvictsOneBotToMakeRoomForNewHumanAtFullTable is the regression
// guard for Ruling 3: a full table (4 humans + 2 bots, exactly what
// ensureBots seats for 4 humans) must not silently reject a fifth,
// genuinely new human with "table full" just because bots are squatting on
// the remaining seats — that would defeat "humans get priority" without any
// visible error. handleJoin must evict one bot to make room BEFORE
// attempting Sit.
func TestJoinEvictsOneBotToMakeRoomForNewHumanAtFullTable(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("2000", "NewGuy", 5000-100) // GetBalance seeds new rows at 100

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(1)
	mux := http.NewServeMux()
	h.Register(mux)

	// Seat 4 humans directly against the engine, then let ensureBots fill
	// the remaining 2 seats — the exact full-table shape Ruling 3 describes.
	// The table is left in StageWaiting (no hand ever started), so this
	// exercises handleJoin's bot-eviction path without needing to drive a
	// full hand to reach a safe between-hands window.
	for i := 0; i < 4; i++ {
		userID := fmt.Sprintf("%d", 1000+i)
		if err := tbl.Sit(userID, fmt.Sprintf("U%d", i), 2000); err != nil {
			t.Fatalf("Sit seat %d: %v", i, err)
		}
	}
	tbl.Lock()
	h.ensureBots(tbl)
	seatCount := len(tbl.Seats)
	botsBefore := countBots(tbl)
	tbl.Unlock()
	if seatCount != poker.MaxSeats {
		t.Fatalf("setup: table has %d seats, want full %d", seatCount, poker.MaxSeats)
	}
	if botsBefore != 2 {
		t.Fatalf("setup: table has %d bots, want 2", botsBefore)
	}

	// A fifth, genuinely new human tries to join the full table.
	initData := userInitData(t, "test-token", 2000, "NewGuy", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("join at a full table with bots status = %d, want 200 (a bot must be evicted to make room), body=%s", rec.Code, rec.Body.String())
	}
	v := decodeView(t, rec)
	if v.YouSeat < 0 {
		t.Fatalf("you_seat = %d, want a real seat", v.YouSeat)
	}

	tbl.Lock()
	defer tbl.Unlock()
	if got := len(tbl.Seats); got != poker.MaxSeats {
		t.Errorf("seats = %d, want still %d (one bot evicted, one human added)", got, poker.MaxSeats)
	}
	if got := countBots(tbl); got != 1 {
		t.Errorf("bots = %d, want 1 (bot count must drop by one)", got)
	}
	if idx := tbl.SeatIndexOf("2000"); idx < 0 {
		t.Error("new human was not actually seated")
	}
}

// --- membership check: transient errors and caching (FIX 2) ----------------

// TestAuthReturns503OnTransientMembershipCheckError proves a checker ERROR
// (network blip, Telegram rate-limit) never reads as "you're not in this
// chat": only a DEFINITIVE ok=false gets 403. An error must get 503 with a
// Ukrainian retry message instead, so the client's own retry/reconnect
// logic can recover once Telegram answers again, rather than the request
// being ejected outright mid-hand.
func TestAuthReturns503OnTransientMembershipCheckError(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	h.isMember = func(chatID, userID int64) (bool, error) {
		return false, errors.New("telegram: rate limited")
	}
	tbl := h.Create(999)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 333, "Carl", "")
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 for a checker error (not 403 — that would read as \"not a member\"), body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Телеграм не відповідає") {
		t.Errorf("body = %q, want the Ukrainian retry message, not the non-member message", rec.Body.String())
	}
}

// TestAuthCachesPositiveMembershipAcrossRequests proves a successful
// membership check is cached: a fold/call/raise or SSE reconnect within
// membershipCacheTTL must not re-hit Telegram at all, so a later transient
// error from isMember cannot strand an already-verified player mid-hand.
func TestAuthCachesPositiveMembershipAcrossRequests(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("333", "Carl", 5000-100)
	h := NewPokerHub(db, nil, "test-token")
	var calls int32
	h.isMember = func(chatID, userID int64) (bool, error) {
		atomic.AddInt32(&calls, 1)
		return true, nil
	}
	tbl := h.Create(999)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 333, "Carl", "")
	req1 := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req1.Header.Set("X-Telegram-Init-Data", initData)
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first join status = %d, want 200, body=%s", rec1.Code, rec1.Body.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("isMember calls after first join = %d, want exactly 1", got)
	}

	// Now make isMember fail outright. If caching is working, this must not
	// matter: the cached positive result from the first join covers this
	// reconnect within membershipCacheTTL, so isMember is never called
	// again.
	h.isMember = func(chatID, userID int64) (bool, error) {
		atomic.AddInt32(&calls, 1)
		return false, errors.New("should never be called: cached")
	}
	req2 := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
	req2.Header.Set("X-Telegram-Init-Data", initData)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("cached reconnect join status = %d, want 200 (membership must be served from cache, not re-checked), body=%s", rec2.Code, rec2.Body.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("isMember calls after cached reconnect = %d, want still 1 (cache should have short-circuited the second check)", got)
	}
}

// TestAuthDoesNotCacheFailedMembershipCheck proves a NEGATIVE membership
// result is never cached — an unknown or currently-failing user must be
// re-checked every time, never accidentally admitted from a stale cache
// entry. This is the fail-closed property FIX 2 must preserve.
func TestAuthDoesNotCacheFailedMembershipCheck(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	var calls int32
	h.isMember = func(chatID, userID int64) (bool, error) {
		atomic.AddInt32(&calls, 1)
		return false, nil // definitive non-member
	}
	tbl := h.Create(999)
	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 333, "Carl", "")
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
		req.Header.Set("X-Telegram-Init-Data", initData)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d: status = %d, want 403", i, rec.Code)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("isMember calls after 3 rejected attempts = %d, want 3 (a negative result must never be cached)", got)
	}
}

// --- action ------------------------------------------------------------------

func TestActionRejectsStaleSeq(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	actor := tbl.Seats[tbl.ToAct].UserID
	initData := userInitData(t, "test-token", mustParseInt64(t, actor), "P", "")

	staleSeq := tbl.Seq - 1
	body := fmt.Sprintf(`{"action":"fold","seq":%d}`, staleSeq)
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/action", strings.NewReader(body))
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for stale seq, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Застаріла") {
		t.Errorf("body = %q, want stale-seq message, not the engine's own error", rec.Body.String())
	}
}

func TestActionRejectsWrongTurn(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	// Whichever seat is NOT to act tries to move out of turn.
	notActor := tbl.Seats[(tbl.ToAct+1)%len(tbl.Seats)].UserID
	initData := userInitData(t, "test-token", mustParseInt64(t, notActor), "P", "")

	body := fmt.Sprintf(`{"action":"fold","seq":%d}`, tbl.Seq)
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/action", strings.NewReader(body))
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for acting out of turn, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "не твій хід") {
		t.Errorf("body = %q, want the engine's own ErrNotYourTurn message passed through", rec.Body.String())
	}
}

// TestActionSettlesBalanceExactlyOnceAtShowdown drives a heads-up hand to
// showdown via a fold and checks that the winner/loser balances move by
// exactly the pot swing, then re-sends the same action to make sure a
// second attempt neither errors out silently into a double-settle nor
// mutates balances again. This is the money-correctness test for RULING 3.
func TestActionSettlesBalanceExactlyOnceAtShowdown(t *testing.T) {
	db := setupTestDB(t)
	// Seed both players to a known 1000 balance (GetBalance/UpdateBalance
	// start new rows at 100, so +900 lands exactly on 1000).
	db.UpdateBalance("111", "Alice", 900)
	db.UpdateBalance("222", "Bob", 900)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(7)
	if err := tbl.Sit("111", "Alice", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.Sit("222", "Bob", 2000); err != nil {
		t.Fatalf("Sit: %v", err)
	}
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)

	actor := tbl.Seats[tbl.ToAct].UserID // heads-up: small blind acts first preflop
	initData := userInitData(t, "test-token", mustParseInt64(t, actor), "P", "")

	body := fmt.Sprintf(`{"action":"fold","seq":%d}`, tbl.Seq)
	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/action", strings.NewReader(body))
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if tbl.Stage != poker.StageShowdown {
		t.Fatalf("stage = %v, want StageShowdown after the only live player folds", tbl.Stage)
	}

	aliceAfter := db.GetBalance("111", "")
	bobAfter := db.GetBalance("222", "")
	if aliceAfter+bobAfter != 2000 {
		t.Fatalf("balances not zero-sum: alice=%d bob=%d, want sum 2000", aliceAfter, bobAfter)
	}
	// Alice (small blind, first to act heads-up) folded, so she loses the
	// blind swing (50) and Bob (big blind) wins it.
	if aliceAfter != 950 {
		t.Errorf("alice balance = %d, want 950 (lost the 50 blind swing)", aliceAfter)
	}
	if bobAfter != 1050 {
		t.Errorf("bob balance = %d, want 1050 (won the 50 blind swing)", bobAfter)
	}

	// Re-send the identical request. The hand is over, so this must be
	// rejected (stale seq or hand-over) and must NOT touch balances again.
	req2 := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/action", strings.NewReader(body))
	req2.Header.Set("X-Telegram-Init-Data", initData)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for repeated post-showdown action, body=%s", rec2.Code, rec2.Body.String())
	}
	if got := db.GetBalance("111", ""); got != 950 {
		t.Errorf("alice balance after repeat = %d, want unchanged 950 (no double-settle)", got)
	}
	if got := db.GetBalance("222", ""); got != 1050 {
		t.Errorf("bob balance after repeat = %d, want unchanged 1050 (no double-settle)", got)
	}
}

// --- SSE stream --------------------------------------------------------------

func TestStreamSendsInitialSnapshotThenBroadcastsJoin(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("555", "Eve", 5000-100)

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(9)
	mux := http.NewServeMux()
	h.Register(mux)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	watcherInitData := userInitData(t, "test-token", 999, "Watcher", "")
	streamReq, err := http.NewRequest("GET", srv.URL+"/api/poker/"+tbl.ID+"/stream", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	streamReq.Header.Set("X-Telegram-Init-Data", watcherInitData)

	resp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d, want 200", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	initial, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read initial SSE event: %v", err)
	}
	var initialView poker.TableView
	if err := json.Unmarshal([]byte(initial), &initialView); err != nil {
		t.Fatalf("decode initial view: %v, raw=%s", err, initial)
	}
	if len(initialView.Seats) != 0 {
		t.Fatalf("initial seats = %d, want 0 before anyone joins", len(initialView.Seats))
	}

	// Trigger a broadcast via a real join, on a separate connection.
	joinInitData := userInitData(t, "test-token", 555, "Eve", "")
	joinReq, err := http.NewRequest("POST", srv.URL+"/api/poker/"+tbl.ID+"/join", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	joinReq.Header.Set("X-Telegram-Init-Data", joinInitData)
	joinResp, err := http.DefaultClient.Do(joinReq)
	if err != nil {
		t.Fatalf("join request: %v", err)
	}
	joinResp.Body.Close()
	if joinResp.StatusCode != http.StatusOK {
		t.Fatalf("join status = %d, want 200", joinResp.StatusCode)
	}

	updated, err := readSSEEvent(reader)
	if err != nil {
		t.Fatalf("read broadcast SSE event: %v", err)
	}
	var updatedView poker.TableView
	if err := json.Unmarshal([]byte(updated), &updatedView); err != nil {
		t.Fatalf("decode updated view: %v, raw=%s", err, updated)
	}
	// 3 seats, not 1: a solo human now brings bot company (see ensureBots),
	// so Eve's join also seats 2 bots and auto-starts a hand.
	if len(updatedView.Seats) != 3 {
		t.Fatalf("seats after join broadcast = %d, want 3 (Eve + 2 bots)", len(updatedView.Seats))
	}
}

// readSSEEvent reads one "data: ...\n\n" frame and returns its payload.
func readSSEEvent(r *bufio.Reader) (string, error) {
	var line string
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		if strings.HasPrefix(l, "data: ") {
			line = strings.TrimPrefix(l, "data: ")
			line = strings.TrimRight(line, "\n")
			// consume the trailing blank line that terminates the frame
			if _, err := r.ReadString('\n'); err != nil {
				return "", err
			}
			return line, nil
		}
	}
}

// --- auth on stream/action ------------------------------------------------
//
// The other stream/action tests above all install stubAllowMember, so none
// of them would catch a refactor that moved the auth() call into only the
// "join" branch of the action switch. These three pin auth on the other two
// endpoints directly.

// streamTestTimeout bounds the SSE tests below that call mux.ServeHTTP
// directly (not over a real network connection). If auth ever regressed to
// let an unauthenticated/non-member request reach handleStream, ServeHTTP
// would block forever inside its "for { select ... }" loop — httptest's
// ResponseRecorder has no reader to close the connection and unblock it, so
// the test would hang until the whole `go test` run times out rather than
// failing with a clear assertion. Giving the request a short-lived context
// means a regression instead makes handleStream exit almost immediately
// (r.Context().Done() fires), so the test fails fast on the wrong status
// code instead of hanging.
const streamTestTimeout = 2 * time.Second

func TestStreamRejectsMissingInitData(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(999)

	mux := http.NewServeMux()
	h.Register(mux)

	ctx, cancel := context.WithTimeout(context.Background(), streamTestTimeout)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/poker/"+tbl.ID+"/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing initData on stream", rec.Code)
	}
}

func TestActionRejectsMissingInitData(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(999)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/action", strings.NewReader(`{"action":"fold"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for missing initData on action", rec.Code)
	}
}

func TestStreamRejectsNonChatMember(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	h.isMember = func(chatID, userID int64) (bool, error) { return false, nil }
	tbl := h.Create(999)

	mux := http.NewServeMux()
	h.Register(mux)

	initData := userInitData(t, "test-token", 333, "Carl", "")
	ctx, cancel := context.WithTimeout(context.Background(), streamTestTimeout)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/poker/"+tbl.ID+"/stream", nil).WithContext(ctx)
	req.Header.Set("X-Telegram-Init-Data", initData)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for non-member on stream, body=%s", rec.Code, rec.Body.String())
	}
}

// --- concurrency ---------------------------------------------------------

// TestConcurrentJoinsRespectCapacityAndDontRace hammers the join endpoint
// from many goroutines at once. Under the required lock discipline (every
// Sit/ViewFor call happens under tbl.Lock()), the table must never exceed
// poker.MaxSeats and must never contain a corrupted/duplicate seat. Run with
// -race to prove there's no data race on the shared table.
//
// Before bots existed, exactly poker.MaxSeats of these humans succeeded and
// the rest were rejected with 409. That is no longer guaranteed: whichever
// goroutine's join happens to land first (a race with no fixed winner) seats
// 2 bots and auto-starts a hand (Ruling 1 — ensureBots now runs for a solo
// human too), permanently claiming up to 2 of the MaxSeats slots for the
// rest of that hand. So this test now checks the invariant that actually
// matters under concurrency — capacity respected, no duplicates, and the
// HTTP success count matches the humans really seated — rather than a fixed
// human count that bots make nondeterministic.
func TestConcurrentJoinsRespectCapacityAndDontRace(t *testing.T) {
	db := setupTestDB(t)
	const attempts = 20
	for i := 0; i < attempts; i++ {
		userID := fmt.Sprintf("%d", 1000+i)
		db.UpdateBalance(userID, fmt.Sprintf("U%d", i), 5000-100)
	}

	h := NewPokerHub(db, nil, "test-token")
	stubAllowMember(h)
	tbl := h.Create(123)
	mux := http.NewServeMux()
	h.Register(mux)

	var wg sync.WaitGroup
	var successes int64
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := int64(1000 + i)
			initData := userInitData(t, "test-token", uid, fmt.Sprintf("U%d", i), "")
			req := httptest.NewRequest("POST", "/api/poker/"+tbl.ID+"/join", nil)
			req.Header.Set("X-Telegram-Init-Data", initData)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code == http.StatusOK {
				atomic.AddInt64(&successes, 1)
			}
		}(i)
	}
	wg.Wait()

	tbl.Lock()
	seatCount := len(tbl.Seats)
	humanSeats, botSeats := 0, 0
	seen := map[string]bool{}
	for _, s := range tbl.Seats {
		if seen[s.UserID] {
			t.Errorf("duplicate seat for user %s", s.UserID)
		}
		seen[s.UserID] = true
		if isBotUser(s.UserID) {
			botSeats++
		} else {
			humanSeats++
		}
	}
	tbl.Unlock()

	if seatCount > poker.MaxSeats {
		t.Errorf("tbl.Seats has %d entries, want at most poker.MaxSeats=%d", seatCount, poker.MaxSeats)
	}
	if humanSeats+botSeats != seatCount {
		t.Errorf("bookkeeping mismatch: %d human + %d bot seats != %d total", humanSeats, botSeats, seatCount)
	}
	if int(successes) != humanSeats {
		t.Errorf("successful joins = %d, want to match the %d human seats actually seated", successes, humanSeats)
	}
}

func mustParseInt64(t *testing.T, s string) int64 {
	t.Helper()
	var v int64
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		t.Fatalf("parse int64 from %q: %v", s, err)
	}
	return v
}
