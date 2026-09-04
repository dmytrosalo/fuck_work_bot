package handlers

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"
)

// buildTime is the modtime reported for embedded photos. embed.FS reports a
// zero modtime, which makes ServeContent skip Last-Modified entirely; a
// fixed non-zero stamp lets conditional requests work without pretending
// the bytes ever change.
var buildTime = time.Unix(0, 0)

// bgFS holds the table background photos. Embedding the whole directory
// (rather than a *.jpg pattern) means the build never breaks when it is
// empty — the README alone satisfies the pattern — and new photos need no
// code change: drop a file in and it appears in the rotation.
//
//go:embed assets/bg
var bgFS embed.FS

var bgImageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

// bgNames lists the embedded background images, sorted so the order is
// stable across builds and machines.
func bgNames() []string {
	entries, err := fs.ReadDir(bgFS, "assets/bg")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if bgImageExts[strings.ToLower(path.Ext(e.Name()))] {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// serveBackground serves one embedded photo. The requested name is matched
// against the embedded list rather than joined onto a path, so a crafted
// name cannot reach outside the directory — there is no traversal to
// sanitise because the filename is never used to build a path.
func (h *PokerHub) serveBackground(w http.ResponseWriter, r *http.Request) {
	want := strings.TrimPrefix(r.URL.Path, "/poker/bg/")
	for _, name := range bgNames() {
		if name != want {
			continue
		}
		data, err := bgFS.ReadFile("assets/bg/" + name)
		if err != nil {
			break
		}
		// Immutable: a given filename always holds the same bytes for the
		// life of a build, and a new photo arrives under a new name.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, name, buildTime, strings.NewReader(string(data)))
		return
	}
	http.NotFound(w, r)
}
