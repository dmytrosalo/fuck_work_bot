package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The README exists only so the embed pattern always matches; it must never
// be served as a background.
func TestBgNamesIgnoresNonImages(t *testing.T) {
	for _, n := range bgNames() {
		low := strings.ToLower(n)
		if !strings.HasSuffix(low, ".jpg") && !strings.HasSuffix(low, ".jpeg") &&
			!strings.HasSuffix(low, ".png") && !strings.HasSuffix(low, ".webp") {
			t.Errorf("%q listed as a background image", n)
		}
	}
}

// The filename is matched against the embedded list rather than joined onto
// a path, so traversal has nothing to work with. This pins that.
func TestBackgroundRejectsUnknownAndTraversal(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	mux := http.NewServeMux()
	h.Register(mux)

	for _, p := range []string{
		"/poker/bg/README.txt",
		"/poker/bg/nope.jpg",
		"/poker/bg/../../etc/passwd",
		"/poker/bg/",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code == http.StatusOK {
			t.Errorf("GET %s = 200, want a refusal (body %d bytes)", p, rec.Body.Len())
		}
	}
}

// Whatever IS embedded must actually be servable — otherwise the page would
// reference photos that 404 and every table would fall back to plain green.
func TestEmbeddedBackgroundsAreServable(t *testing.T) {
	h := NewPokerHub(nil, nil, "tok")
	mux := http.NewServeMux()
	h.Register(mux)

	names := bgNames()
	if len(names) == 0 {
		t.Skip("no background photos embedded yet")
	}
	for _, n := range names {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/poker/bg/"+n, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET /poker/bg/%s = %d, want 200", n, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s served empty", n)
		}
	}
}
