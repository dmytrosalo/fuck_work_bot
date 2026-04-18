# Collection Web Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a web page at `/collection/:userID` that renders a user's card collection as styled HTML/CSS cards, linked from the Telegram `/collection` command.

**Architecture:** HTTP server runs alongside the Telegram bot in a goroutine on `:8080`. Server-side rendered HTML via Go `html/template`. Dark theme, mobile-first, compact card grid with click-to-expand details.

**Tech Stack:** Go stdlib (`net/http`, `html/template`), pure CSS, minimal vanilla JS.

---

### Task 1: Add DB methods for collection web page

**Files:**
- Modify: `go-bot/internal/storage/sqlite.go:291-313`

- [ ] **Step 1: Add `FullCard` type and `GetFullCollection` method**

Add after the existing `CollectionCard` type (line 295) in `sqlite.go`:

```go
type FullCard struct {
	ID          int
	Name        string
	Rarity      int
	Category    string
	Emoji       string
	Description string
	ATK         int
	DEF         int
	SpecialName string
	Special     int
	Count       int
}

func (d *DB) GetFullCollection(userID string) []FullCard {
	rows, err := d.db.Query(`SELECT c.id, c.name, c.rarity, c.category, c.emoji, c.description, c.atk, c.def, c.special_name, c.special, col.count
		FROM collection col JOIN cards c ON col.card_id = c.id
		WHERE col.user_id = ? ORDER BY c.rarity DESC, c.name`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cards []FullCard
	for rows.Next() {
		var card FullCard
		if err := rows.Scan(&card.ID, &card.Name, &card.Rarity, &card.Category, &card.Emoji, &card.Description, &card.ATK, &card.DEF, &card.SpecialName, &card.Special, &card.Count); err != nil {
			continue
		}
		cards = append(cards, card)
	}
	return cards
}
```

- [ ] **Step 2: Add `GetUserName` method**

Add after `GetFullCollection`:

```go
func (d *DB) GetUserName(userID string) string {
	var name string
	d.db.QueryRow(`SELECT name FROM balances WHERE user_id = ? LIMIT 1`, userID).Scan(&name)
	return name
}
```

- [ ] **Step 3: Add `GetRarityCounts` method**

Add after `GetUserName`:

```go
func (d *DB) GetRarityCounts(userID string) map[int]int {
	counts := make(map[int]int)
	rows, err := d.db.Query(`SELECT c.rarity, SUM(col.count) FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? GROUP BY c.rarity`, userID)
	if err != nil {
		return counts
	}
	defer rows.Close()
	for rows.Next() {
		var rarity, count int
		if err := rows.Scan(&rarity, &count); err != nil {
			continue
		}
		counts[rarity] = count
	}
	return counts
}
```

- [ ] **Step 4: Build and verify compilation**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./internal/storage/`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
cd /Users/dmytrosalo/Projects/own/fuck_work_bot
git add go-bot/internal/storage/sqlite.go
git commit -m "feat: add DB methods for collection web page"
```

---

### Task 2: Create web handler with HTML template

**Files:**
- Create: `go-bot/internal/handlers/web.go`

- [ ] **Step 1: Create `web.go` with HTTP handler and HTML template**

Create `go-bot/internal/handlers/web.go`:

```go
package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/dmytrosalo/fuck-work-bot/internal/storage"
)

// RegisterWeb sets up HTTP routes for the web UI.
func RegisterWeb(mux *http.ServeMux, db *storage.DB) {
	mux.HandleFunc("/collection/", func(w http.ResponseWriter, r *http.Request) {
		handleCollectionPage(w, r, db)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "fuck-work-bot is running")
	})
}

type collectionPageData struct {
	UserName     string
	Unique       int
	Total        int
	Balance      int
	RarityCounts map[int]int
	Sections     []raritySection
}

type raritySection struct {
	Rarity     int
	RarityName string
	Stars      string
	AccentCSS  string
	BgCSS      string
	Cards      []storage.FullCard
}

var webRarityNames = map[int]string{
	1: "Common",
	2: "Uncommon",
	3: "Rare",
	4: "Epic",
	5: "Legendary",
	6: "ULTRA LEGENDARY",
}

var webRarityStars = map[int]string{
	1: "\u2b50",
	2: "\u2b50\u2b50",
	3: "\u2b50\u2b50\u2b50",
	4: "\u2b50\u2b50\u2b50\u2b50",
	5: "\u2b50\u2b50\u2b50\u2b50\u2b50",
	6: "\U0001f48e\U0001f48e\U0001f48e\U0001f48e\U0001f48e\U0001f48e",
}

// CSS colors matching cardimage.go rarityAccent/rarityBg
var webRarityAccent = map[int]string{
	1: "rgb(120,120,130)",
	2: "rgb(50,180,100)",
	3: "rgb(60,120,220)",
	4: "rgb(170,70,220)",
	5: "rgb(240,190,40)",
	6: "rgb(255,50,50)",
}

var webRarityBg = map[int]string{
	1: "rgb(35,35,40)",
	2: "rgb(25,38,32)",
	3: "rgb(25,30,45)",
	4: "rgb(38,25,45)",
	5: "rgb(45,38,22)",
	6: "rgb(50,15,15)",
}

func handleCollectionPage(w http.ResponseWriter, r *http.Request, db *storage.DB) {
	// Extract userID from /collection/{userID}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/collection/"), "/")
	userID := parts[0]
	if userID == "" {
		http.NotFound(w, r)
		return
	}

	userName := db.GetUserName(userID)
	if userName == "" {
		http.NotFound(w, r)
		return
	}

	cards := db.GetFullCollection(userID)
	unique, total := db.GetCollectionStats(userID)
	balance := db.GetBalance(userID, "")
	rarityCounts := db.GetRarityCounts(userID)

	// Group cards by rarity
	grouped := make(map[int][]storage.FullCard)
	for _, card := range cards {
		grouped[card.Rarity] = append(grouped[card.Rarity], card)
	}

	var sections []raritySection
	for _, rarity := range []int{6, 5, 4, 3, 2, 1} {
		if len(grouped[rarity]) == 0 {
			continue
		}
		sections = append(sections, raritySection{
			Rarity:     rarity,
			RarityName: webRarityNames[rarity],
			Stars:      webRarityStars[rarity],
			AccentCSS:  webRarityAccent[rarity],
			BgCSS:      webRarityBg[rarity],
			Cards:      grouped[rarity],
		})
	}

	data := collectionPageData{
		UserName:     userName,
		Unique:       unique,
		Total:        total,
		Balance:      balance,
		RarityCounts: rarityCounts,
		Sections:     sections,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := collectionTmpl.Execute(w, data); err != nil {
		http.Error(w, "render error", 500)
	}
}

var collectionTmpl = template.Must(template.New("collection").Parse(collectionHTML))

const collectionHTML = `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.UserName}} — Collection</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: #1a1a2e;
  color: #e0e0e0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  padding: 16px;
  max-width: 800px;
  margin: 0 auto;
}
.header {
  text-align: center;
  padding: 24px 16px;
  margin-bottom: 24px;
  background: #16213e;
  border-radius: 16px;
  border: 1px solid #0f3460;
}
.header h1 {
  font-size: 24px;
  margin-bottom: 8px;
}
.header .progress {
  font-size: 18px;
  color: #a0a0a0;
  margin-bottom: 8px;
}
.header .balance {
  font-size: 16px;
  color: #f0c040;
}
.rarity-counts {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 12px;
}
.rarity-counts span {
  font-size: 13px;
  padding: 4px 10px;
  border-radius: 12px;
  background: rgba(255,255,255,0.05);
}
.section {
  margin-bottom: 24px;
}
.section-header {
  font-size: 16px;
  font-weight: 700;
  padding: 8px 0;
  margin-bottom: 12px;
  border-bottom: 2px solid;
}
.grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
}
@media (min-width: 600px) {
  .grid { grid-template-columns: repeat(5, 1fr); }
}
.card {
  position: relative;
  border-radius: 12px;
  padding: 12px 8px;
  text-align: center;
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s;
  border: 2px solid;
}
.card:hover {
  transform: translateY(-2px);
}
.card .emoji {
  font-size: 36px;
  line-height: 1.2;
}
.card .name {
  font-size: 12px;
  font-weight: 600;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.card .count {
  position: absolute;
  top: 4px;
  right: 6px;
  font-size: 11px;
  font-weight: 700;
  background: rgba(0,0,0,0.6);
  padding: 1px 6px;
  border-radius: 8px;
}
.card .details {
  display: none;
  margin-top: 8px;
  font-size: 11px;
  text-align: left;
  line-height: 1.5;
}
.card.expanded .details {
  display: block;
}
.card.expanded {
  grid-column: span 1;
}
@media (min-width: 600px) {
  .card.expanded { grid-column: span 2; }
}
.details .stats {
  display: flex;
  gap: 8px;
  margin: 4px 0;
  font-weight: 600;
}
.details .desc {
  color: #b0b0b0;
  font-style: italic;
  margin-top: 4px;
}
.details .stars {
  margin-top: 4px;
}
</style>
</head>
<body>
<div class="header">
  <h1>{{.UserName}}</h1>
  <div class="progress">{{.Unique}} / {{.Total}} cards</div>
  <div class="balance">{{.Balance}} coins</div>
  <div class="rarity-counts">
    {{range $rarity := .Sections}}
    <span style="border:1px solid {{$rarity.AccentCSS}}">{{$rarity.Stars}} {{index $.RarityCounts $rarity.Rarity}}</span>
    {{end}}
  </div>
</div>

{{range .Sections}}
<div class="section">
  <div class="section-header" style="border-color:{{.AccentCSS}}; color:{{.AccentCSS}}">
    {{.Stars}} {{.RarityName}} ({{len .Cards}})
  </div>
  <div class="grid">
    {{range .Cards}}
    <div class="card" onclick="toggle(this)"
         style="background:{{$.BgCSS}}; border-color:{{$.AccentCSS}}; box-shadow:0 0 8px {{$.AccentCSS}}33">
      {{if gt .Count 1}}<span class="count">x{{.Count}}</span>{{end}}
      <div class="emoji">{{.Emoji}}</div>
      <div class="name">{{.Name}}</div>
      <div class="details">
        <div class="stats">
          <span>ATK {{.ATK}}</span>
          <span>DEF {{.DEF}}</span>
          <span>{{.SpecialName}} {{.Special}}</span>
        </div>
        <div class="desc">{{.Description}}</div>
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}

<script>
function toggle(el) {
  var wasExpanded = el.classList.contains('expanded');
  document.querySelectorAll('.card.expanded').forEach(function(c) {
    c.classList.remove('expanded');
  });
  if (!wasExpanded) {
    el.classList.add('expanded');
  }
}
</script>
</body>
</html>`
```

- [ ] **Step 2: Build and verify compilation**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./internal/handlers/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /Users/dmytrosalo/Projects/own/fuck_work_bot
git add go-bot/internal/handlers/web.go
git commit -m "feat: add collection web page handler with HTML template"
```

---

### Task 3: Start HTTP server in main.go

**Files:**
- Modify: `go-bot/cmd/bot/main.go:100-119`

- [ ] **Step 1: Add HTTP server goroutine**

In `cmd/bot/main.go`, add the import `"net/http"` to the import block and insert the HTTP server startup between handler registration (line 113) and the daily report scheduler (line 116):

```go
	// Start web server
	mux := http.NewServeMux()
	handlers.RegisterWeb(mux, db)
	go func() {
		log.Println("Web server starting on :8080...")
		if err := http.ListenAndServe(":8080", mux); err != nil {
			log.Printf("Web server error: %v", err)
		}
	}()
```

- [ ] **Step 2: Build and verify compilation**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./cmd/bot/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /Users/dmytrosalo/Projects/own/fuck_work_bot
git add go-bot/cmd/bot/main.go
git commit -m "feat: start HTTP server alongside Telegram bot"
```

---

### Task 4: Simplify /collection command to counts + link

**Files:**
- Modify: `go-bot/internal/handlers/cards.go:158-187`

- [ ] **Step 1: Replace `handleCollection` with compact version**

Replace the entire `handleCollection` method in `cards.go` (lines 158-187) with:

```go
func (b *Bot) handleCollection(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	unique, total := b.db.GetCollectionStats(userID)
	if unique == 0 {
		return c.Reply("У тебе ще немає карток. Напиши /pack!")
	}

	rarityCounts := b.db.GetRarityCounts(userID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Колекція %s* (%d/%d)\n\n", userName, unique, total))

	// Rarity counts on one line
	first := true
	for _, rarity := range []int{1, 2, 3, 4, 5, 6} {
		count, ok := rarityCounts[rarity]
		if !ok || count == 0 {
			continue
		}
		if !first {
			sb.WriteString(" | ")
		}
		sb.WriteString(fmt.Sprintf("%s %d", rarityStars[rarity], count))
		first = false
	}

	sb.WriteString(fmt.Sprintf("\n\nhttps://fuck-work-bot.fly.dev/collection/%s", userID))

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
```

- [ ] **Step 2: Build and verify compilation**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build ./cmd/bot/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
cd /Users/dmytrosalo/Projects/own/fuck_work_bot
git add go-bot/internal/handlers/cards.go
git commit -m "feat: simplify /collection to rarity counts + web link"
```

---

### Task 5: Verify full build and test

**Files:** none (verification only)

- [ ] **Step 1: Run full build**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go build -o bot ./cmd/bot/`
Expected: binary built with no errors

- [ ] **Step 2: Run tests**

Run: `cd /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot && CGO_ENABLED=0 go test ./...`
Expected: all tests pass

- [ ] **Step 3: Clean up binary**

Run: `rm /Users/dmytrosalo/Projects/own/fuck_work_bot/go-bot/bot`
