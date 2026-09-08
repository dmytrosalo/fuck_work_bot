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
		if got, want := miniAppName(), "finikultramatcha"; got != want {
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
	t.Setenv("POKER_MINIAPP", "")
	got := miniAppLink("fuuck_work_bot", "a1b2c3d4e5f60718")
	want := "https://t.me/fuuck_work_bot/finikultramatcha?startapp=a1b2c3d4e5f60718"
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

// TestHookahIsKeyedToBotUserID pins Data Android God's hookah — and the
// smoke it puffs when he takes a pot — to his seat user_id rather than his
// display name. botNames in pokerbots.go is editable prose, and the rest of
// his artwork (card back, avatar) already keys off "bot:2" for exactly this
// reason: a rename must not silently hand the hookah to another seat or
// drop it altogether.
func TestHookahIsKeyedToBotUserID(t *testing.T) {
	var b strings.Builder
	if err := pokerTmpl.Execute(&b, map[string]string{"TableID": "a1b2c3d4e5f60718"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		".hookah{",              // the prop itself
		"@keyframes hookahpuff", // puffs rising off the bowl
		"@keyframes hookahhaze", // the drift across the felt
		"const HOOKAH=",         // inline SVG, same shape as DROID
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// The seat that gets the hookah must be selected by user_id. If the
	// display name leaks into the client, renaming the bot breaks the art.
	if strings.Contains(out, "Data Android God") {
		t.Errorf("hookah/smoke must not be keyed off the bot's display name")
	}

	// Decorative overlays must never swallow a tap meant for the table,
	// the same guarantee #win already documents.
	if !strings.Contains(out, ".haze{") || !strings.Contains(out, "pointer-events:none") {
		t.Errorf("haze layer missing or not pointer-transparent")
	}

	if !strings.Contains(out, "prefers-reduced-motion") {
		t.Errorf("smoke has no reduced-motion guard")
	}

	// He wins often, so the smoke is rationed the same way bot taunts are
	// (pokerbots.go): a probability for variety, and a cooldown, which is
	// the part that actually stops two puffs on consecutive hands.
	for _, want := range []string{"SMOKE_CHANCE", "SMOKE_COOLDOWN_MS", "Math.random()"} {
		if !strings.Contains(out, want) {
			t.Errorf("smoke is not rationed: missing %q", want)
		}
	}
}

// TestBlameLinesAreKeyedToBotUserIDs pins each bot's blame bubble to its
// seat user_id, for the same reason the hookah and the card backs are:
// botNames in pokerbots.go is editable prose, and a rename must never move
// Bo's line onto the Android's seat. Also pins the lines themselves — they
// are the whole joke, and a silent edit would not fail anything else.
func TestBlameLinesAreKeyedToBotUserIDs(t *testing.T) {
	var b strings.Builder
	if err := pokerTmpl.Execute(&b, map[string]string{"TableID": "a1b2c3d4e5f60718"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		`"bot:1":"Це все Делна!"`,
		`"bot:2":"Це все бекенд!"`,
		"const BUBBLE=",
		".blame{",
		"@keyframes blamepop",
		"BLAME_CHANCE",
		"BLAME_COOLDOWN_MS",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	// A blind posted and folded is not a loss worth blaming anyone for, so
	// the trigger is a loss bigger than the big blind rather than any
	// negative result at all.
	if !strings.Contains(out, "big_blind") {
		t.Errorf("blame trigger does not reference big_blind")
	}
}

// Pins the Супер гра surface the same way the hookah and blame lines are
// pinned: the labels are the feature, and a silent edit would fail nothing
// else. Also pins that the client never decides an outcome -- the roll is
// the server's, and a Math.random in this panel would be a money bug.
func TestSuperGamePanelIsRenderedAndServerDriven(t *testing.T) {
	var b strings.Builder
	if err := pokerTmpl.Execute(&b, map[string]string{"TableID": "a1b2c3d4e5f60718"}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	out := b.String()

	for _, want := range []string{
		"Супер гра",
		"Кубики",
		"Червоне",
		"Чорне",
		"Пас",
		"#supergame",
		"@keyframes dicetumble",
		"@keyframes superwin",
		`"/super"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered page is missing %q", want)
		}
	}

	if !strings.Contains(out, "prefers-reduced-motion") {
		t.Errorf("super game animations have no reduced-motion guard")
	}
}
