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

var webRarityAccent = map[int]string{
	1: "rgb(120,120,130)",
	2: "rgb(50,180,100)",
	3: "rgb(60,120,220)",
	4: "rgb(170,70,220)",
	5: "rgb(240,190,40)",
	6: "rgb(255,50,50)",
}

var webRarityBg = map[int]string{
	1: "rgb(55,55,65)",
	2: "rgb(35,58,45)",
	3: "rgb(35,45,70)",
	4: "rgb(55,35,70)",
	5: "rgb(65,55,30)",
	6: "rgb(75,25,25)",
}

func handleCollectionPage(w http.ResponseWriter, r *http.Request, db *storage.DB) {
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
{{$bg := .BgCSS}}{{$accent := .AccentCSS}}
<div class="section">
  <div class="section-header" style="border-color:{{.AccentCSS}}; color:{{.AccentCSS}}">
    {{.Stars}} {{.RarityName}} ({{len .Cards}})
  </div>
  <div class="grid">
    {{range .Cards}}
    <div class="card" onclick="toggle(this)"
         style="background:{{$bg}}; border-color:{{$accent}}; box-shadow:0 0 8px {{$accent}}33">
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
