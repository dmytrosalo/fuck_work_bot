package handlers

import (
	"bufio"
	"encoding/json"
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

func decodeView(t *testing.T, rec *httptest.ResponseRecorder) poker.TableView {
	t.Helper()
	var v poker.TableView
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode view: %v, body=%s", err, rec.Body.String())
	}
	return v
}

// --- join ------------------------------------------------------------------

func TestJoinSucceedsAndClampsBuyInToMaxBuyIn(t *testing.T) {
	db := setupTestDB(t)
	db.UpdateBalance("111", "Alice", 50000-100) // GetBalance seeds new rows at 100

	h := NewPokerHub(db, nil, "test-token")
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
	if len(v.Seats) != 1 {
		t.Fatalf("seats = %d, want 1", len(v.Seats))
	}
	if v.Seats[0].Stack != poker.MaxBuyIn {
		t.Errorf("stack = %d, want buy-in clamped to MaxBuyIn=%d", v.Seats[0].Stack, poker.MaxBuyIn)
	}
}

func TestJoinRejectsBuyInBelowMinimum(t *testing.T) {
	// No db means balance defaults to 0, which is below poker.MinBuyIn.
	h := NewPokerHub(nil, nil, "test-token")
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

// --- action ------------------------------------------------------------------

func TestActionRejectsStaleSeq(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
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
	if len(updatedView.Seats) != 1 {
		t.Fatalf("seats after join broadcast = %d, want 1", len(updatedView.Seats))
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

// --- concurrency ---------------------------------------------------------

// TestConcurrentJoinsRespectCapacityAndDontRace hammers the join endpoint
// from many goroutines at once. Under the required lock discipline (every
// Sit/ViewFor call happens under tbl.Lock()), exactly poker.MaxSeats joins
// must succeed and the rest must be rejected with 409 — never more seats
// than capacity, and no corrupted/duplicate seats. Run with -race to prove
// there's no data race on the shared table.
func TestConcurrentJoinsRespectCapacityAndDontRace(t *testing.T) {
	db := setupTestDB(t)
	const attempts = 20
	for i := 0; i < attempts; i++ {
		userID := fmt.Sprintf("%d", 1000+i)
		db.UpdateBalance(userID, fmt.Sprintf("U%d", i), 5000-100)
	}

	h := NewPokerHub(db, nil, "test-token")
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

	if int(successes) != poker.MaxSeats {
		t.Errorf("successful joins = %d, want exactly poker.MaxSeats=%d", successes, poker.MaxSeats)
	}

	tbl.Lock()
	seatCount := len(tbl.Seats)
	seen := map[string]bool{}
	for _, s := range tbl.Seats {
		if seen[s.UserID] {
			t.Errorf("duplicate seat for user %s", s.UserID)
		}
		seen[s.UserID] = true
	}
	tbl.Unlock()

	if seatCount != poker.MaxSeats {
		t.Errorf("tbl.Seats has %d entries, want poker.MaxSeats=%d", seatCount, poker.MaxSeats)
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
