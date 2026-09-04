package handlers

import (
	"bufio"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Lofi/downtempo stations from SomaFM, which is listener-supported and
// publishes these playlists openly. The station name and a SomaFM credit are
// shown in the UI — the least we can do for a free stream.
//
// Each entry points at a .pls PLAYLIST, not a stream: SomaFM rotates which
// icecast host serves a channel, so hardcoding today's stream URL would
// break silently the next time they move it. The playlist is resolved at
// runtime and cached.
var radioStations = []struct{ ID, Title, PLS string }{
	{"fluid", "Fluid — instrumental hiphop", "https://api.somafm.com/fluid.pls"},
	{"groovesalad", "Groove Salad — chilled beats", "https://api.somafm.com/groovesalad256.pls"},
	{"lush", "Lush — vocal chillout", "https://api.somafm.com/lush.pls"},
	{"dronezone", "Drone Zone — ambient", "https://api.somafm.com/dronezone256.pls"},
}

const radioCacheTTL = time.Hour

type radioStation struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

var (
	radioMu       sync.Mutex
	radioCache    []radioStation
	radioCachedAt time.Time
)

// resolvePLS returns the first stream URL in a .pls playlist. Failures are
// reported as an empty string: a station that cannot be resolved is simply
// left out of the list rather than being offered as a button that plays
// nothing.
func resolvePLS(url string) string {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	// Icecast hangs up on a request with no User-Agent, which is what a bare
	// client sends; a browser always has one.
	req.Header.Set("User-Agent", "fuck-work-bot/1.0 (+https://fuck-work-bot.fly.dev)")
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "File1=") {
			return strings.TrimPrefix(line, "File1=")
		}
	}
	return ""
}

// radioList returns the playable stations, refreshing the resolved URLs at
// most once an hour. Cached across every player and every table: this is
// public data with no per-user component, and resolving four playlists on
// each page load would add four outbound requests to every open.
func radioList() []radioStation {
	radioMu.Lock()
	defer radioMu.Unlock()
	if radioCache != nil && time.Since(radioCachedAt) < radioCacheTTL {
		return radioCache
	}
	out := make([]radioStation, 0, len(radioStations))
	for _, s := range radioStations {
		if u := resolvePLS(s.PLS); u != "" {
			out = append(out, radioStation{ID: s.ID, Title: s.Title, URL: u})
		}
	}
	// Only cache a usable answer, so a transient network failure does not
	// pin an empty list for an hour.
	if len(out) > 0 {
		radioCache, radioCachedAt = out, time.Now()
	}
	return out
}

func (h *PokerHub) handleRadio(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, radioList())
}
