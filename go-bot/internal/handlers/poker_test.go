package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiniAppName(t *testing.T) {
	t.Run("defaults when unset", func(t *testing.T) {
		t.Setenv("POKER_MINIAPP", "")
		if got, want := miniAppName(), "poker"; got != want {
			t.Errorf("miniAppName() = %q, want %q", got, want)
		}
	})
	t.Run("env overrides", func(t *testing.T) {
		t.Setenv("POKER_MINIAPP", "holdem")
		if got, want := miniAppName(), "holdem"; got != want {
			t.Errorf("miniAppName() = %q, want %q", got, want)
		}
	})
}

// TestMiniAppLink pins the exact t.me shape. A web_app button is rejected
// outside private chats (BUTTON_TYPE_INVALID), so this link is the only
// thing that makes /poker work in a group — a regression here silences the
// command with no error visible to players.
func TestMiniAppLink(t *testing.T) {
	t.Setenv("POKER_MINIAPP", "poker")
	got := miniAppLink("fuuck_work_bot", "a1b2c3d4e5f60718")
	want := "https://t.me/fuuck_work_bot/poker?startapp=a1b2c3d4e5f60718"
	if got != want {
		t.Errorf("miniAppLink() = %q, want %q", got, want)
	}
}

func TestMiniAppLinkUsesConfiguredShortName(t *testing.T) {
	t.Setenv("POKER_MINIAPP", "holdem")
	got := miniAppLink("somebot", "ff00")
	want := "https://t.me/somebot/holdem?startapp=ff00"
	if got != want {
		t.Errorf("miniAppLink() = %q, want %q", got, want)
	}
}

// TestPokerTemplateResolvesTableID pins how the page learns which table it
// is. Opened from a group the path carries no id — Telegram serves the
// fixed @BotFather URL and passes the table in startapp — so the rendered
// script MUST fall back to start_param. A regression here renders a page
// that authenticates fine and then talks to table "", which 404s with
// "Стіл закрито" and looks like an expired table rather than a bug.
func TestPokerTemplateResolvesTableID(t *testing.T) {
	render := func(id string) string {
		var b strings.Builder
		if err := pokerTmpl.Execute(&b, map[string]string{"TableID": id}); err != nil {
			t.Fatalf("Execute(%q) error: %v", id, err)
		}
		return b.String()
	}

	t.Run("empty id falls back to start_param", func(t *testing.T) {
		out := render("")
		if !strings.Contains(out, `const TABLE=""||`) {
			t.Errorf("empty TableID did not render a falsy literal with a fallback")
		}
		if !strings.Contains(out, "initDataUnsafe.start_param") {
			t.Errorf("rendered page has no start_param fallback")
		}
	})

	t.Run("path id wins over start_param", func(t *testing.T) {
		out := render("a1b2c3d4e5f60718")
		if !strings.Contains(out, `const TABLE="a1b2c3d4e5f60718"||`) {
			t.Errorf("templated TableID not rendered as the leading operand")
		}
	})

	// tg is read by the fallback expression, so it must already be assigned
	// where TABLE is initialised — otherwise the page dies on a TDZ
	// ReferenceError before it ever reaches join.
	t.Run("tg is declared before TABLE uses it", func(t *testing.T) {
		out := render("")
		tg, table := strings.Index(out, "const tg="), strings.Index(out, "const TABLE=")
		if tg < 0 || table < 0 {
			t.Fatalf("missing declarations: tg=%d table=%d", tg, table)
		}
		if tg > table {
			t.Errorf("const tg declared after const TABLE (%d > %d)", tg, table)
		}
	})
}

// TestPokerPageServesShellWithoutPathID covers the server half of the same
// fix. Telegram requests the @BotFather URL with no table in the path; if
// that 404s, the Mini App shows "Стіл закрито" and never runs, which is
// exactly the silent failure this change exists to remove.
func TestPokerPageServesShellWithoutPathID(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	t.Run("no path id serves the page", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /poker/ = %d, want 200 (body %q)", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "start_param") {
			t.Errorf("shell served without the start_param fallback")
		}
	})

	// An id that IS present must still be validated, or a stale link would
	// silently open a live-looking table that no longer exists.
	t.Run("unknown path id still 404s", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/deadbeefdeadbeef", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET /poker/deadbeefdeadbeef = %d, want 404", rec.Code)
		}
	})

	t.Run("known path id serves the page", func(t *testing.T) {
		tbl := h.Create(-100)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/"+tbl.ID, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /poker/%s = %d, want 200", tbl.ID, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `const TABLE="`+tbl.ID+`"`) {
			t.Errorf("page did not embed the requested table id")
		}
	})
}

// TestPokerPageNoRedirectWithoutSlash guards the exact "/poker" route. If
// only the "/poker/" subtree pattern were registered, the mux would answer
// "/poker" with a 301 — and a redirect can drop the "#tgWebAppData=..."
// fragment carrying initData, so the app would load unauthenticated with
// nothing in the logs to say why.
func TestPokerPageNoRedirectWithoutSlash(t *testing.T) {
	h := NewPokerHub(nil, nil, "test-token")
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /poker = %d, want 200 (a 3xx here drops the initData fragment)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "start_param") {
		t.Errorf("shell served without the start_param fallback")
	}
}
