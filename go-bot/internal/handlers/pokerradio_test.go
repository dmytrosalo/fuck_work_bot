package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// resolvePLS must return the FIRST stream entry and nothing else — the file
// also contains titles and a second/third mirror.
func TestResolvePLSPicksTheFirstStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			// Icecast hangs up on an empty UA; assert we always send one.
			t.Error("request sent with no User-Agent")
		}
		_, _ = w.Write([]byte("[playlist]\nnumberofentries=2\n" +
			"File1=https://ice6.example.com/fluid-128-mp3\n" +
			"Title1=SomaFM: Fluid\nLength1=-1\n" +
			"File2=https://ice2.example.com/fluid-128-mp3\n"))
	}))
	defer srv.Close()

	if got, want := resolvePLS(srv.URL), "https://ice6.example.com/fluid-128-mp3"; got != want {
		t.Errorf("resolvePLS = %q, want %q", got, want)
	}
}

// A station that will not resolve must be left out rather than offered as a
// button that plays nothing.
func TestResolvePLSFailsSoftly(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()

	for _, url := range []string{bad.URL, "http://127.0.0.1:1/nothing", "not-a-url"} {
		if got := resolvePLS(url); got != "" {
			t.Errorf("resolvePLS(%q) = %q, want empty", url, got)
		}
	}
}

// Playlists with no File1 line yield nothing rather than garbage.
func TestResolvePLSIgnoresPlaylistWithoutStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[playlist]\nnumberofentries=0\nVersion=2\n"))
	}))
	defer srv.Close()
	if got := resolvePLS(srv.URL); got != "" {
		t.Errorf("got %q from a playlist with no entries", got)
	}
}

// The endpoint must always answer with a JSON array the client can iterate,
// even when every station is unreachable — never null, which would throw on
// .length in the page.
func TestRadioEndpointAlwaysReturnsAnArray(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/radio", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := strings.TrimSpace(rec.Body.String())
	if !strings.HasPrefix(body, "[") {
		t.Fatalf("body is not a JSON array: %s", body)
	}
	var got []radioStation
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, s := range got {
		if s.URL == "" || s.Title == "" {
			t.Errorf("station %+v offered with a missing field", s)
		}
	}
}

// /poker/radio must win over the /poker/ page subtree, or the radio request
// would be served an HTML page and the client would fail to parse it.
func TestRadioRouteBeatsThePageRoute(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/radio", nil))
	if strings.Contains(rec.Body.String(), "<html") {
		t.Error("/poker/radio was served the Mini App page instead of JSON")
	}
}
