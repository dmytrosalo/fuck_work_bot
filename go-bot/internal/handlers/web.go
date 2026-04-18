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
	GlowCSS    string
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

var webRarityGlow = map[int]string{
	1: "rgba(120,120,130,0.15)",
	2: "rgba(50,180,100,0.15)",
	3: "rgba(60,120,220,0.15)",
	4: "rgba(170,70,220,0.15)",
	5: "rgba(240,190,40,0.15)",
	6: "rgba(255,50,50,0.2)",
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
			GlowCSS:    webRarityGlow[rarity],
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
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap');
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: #0a0a1a;
  background-image: radial-gradient(ellipse at 20% 50%, rgba(60,40,120,0.15) 0%, transparent 50%),
                    radial-gradient(ellipse at 80% 20%, rgba(40,80,140,0.1) 0%, transparent 50%);
  color: #e8e8f0;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
  padding: 16px;
  max-width: 860px;
  margin: 0 auto;
  min-height: 100vh;
}
.glass {
  background: rgba(255,255,255,0.04);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 20px;
}
.header {
  text-align: center;
  padding: 28px 20px;
  margin-bottom: 28px;
}
.header h1 {
  font-size: 28px;
  font-weight: 800;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #fff 0%, #a0a0c0 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 6px;
}
.header .progress {
  font-size: 15px;
  color: rgba(255,255,255,0.5);
  font-weight: 500;
  margin-bottom: 4px;
}
.header .balance {
  font-size: 15px;
  font-weight: 600;
  color: #fbbf24;
}
.rarity-counts {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 14px;
}
.rarity-counts span {
  font-size: 12px;
  font-weight: 600;
  padding: 5px 12px;
  border-radius: 20px;
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.1);
}
.section {
  margin-bottom: 28px;
}
.section-header {
  font-size: 15px;
  font-weight: 700;
  padding: 10px 16px;
  margin-bottom: 14px;
  border-radius: 12px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}
@media (min-width: 500px) {
  .grid { grid-template-columns: repeat(3, 1fr); }
}
@media (min-width: 700px) {
  .grid { grid-template-columns: repeat(4, 1fr); }
}
.card {
  position: relative;
  border-radius: 16px;
  padding: 14px 10px 12px;
  text-align: center;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
  border: 1px solid;
  background: rgba(255,255,255,0.03);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  overflow: hidden;
}
.card::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 3px;
  border-radius: 16px 16px 0 0;
}
.card:hover {
  transform: translateY(-3px);
}
.card:active {
  transform: scale(0.98);
}
.card .emoji {
  font-size: 40px;
  line-height: 1.2;
  filter: drop-shadow(0 2px 4px rgba(0,0,0,0.3));
}
.card .name {
  font-size: 11px;
  font-weight: 700;
  margin-top: 6px;
  color: #fff;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.card .count {
  position: absolute;
  top: 6px;
  right: 8px;
  font-size: 10px;
  font-weight: 700;
  background: rgba(0,0,0,0.5);
  backdrop-filter: blur(8px);
  padding: 2px 7px;
  border-radius: 10px;
  color: rgba(255,255,255,0.8);
}
.card .stats {
  display: flex;
  justify-content: center;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.card .stats span {
  font-size: 9px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 6px;
  background: rgba(0,0,0,0.3);
  color: rgba(255,255,255,0.75);
  letter-spacing: 0.3px;
}
.card .stats .atk { color: #ff6b6b; }
.card .stats .def { color: #4ecdc4; }
.card .stats .spc { color: #fbbf24; }
.card .desc {
  display: none;
  font-size: 10px;
  color: rgba(255,255,255,0.5);
  margin-top: 6px;
  line-height: 1.4;
  font-style: italic;
}
.card.expanded .desc {
  display: block;
}
.card.expanded {
  grid-column: span 2;
}
</style>
</head>
<body>
<div class="header glass">
  <h1>{{.UserName}}</h1>
  <div class="progress">{{.Unique}} / {{.Total}} cards</div>
  <div class="balance">{{.Balance}} coins</div>
  <div class="rarity-counts">
    {{range $rarity := .Sections}}
    <span style="border-color:{{$rarity.AccentCSS}}40; color:{{$rarity.AccentCSS}}">{{$rarity.Stars}} {{index $.RarityCounts $rarity.Rarity}}</span>
    {{end}}
  </div>
</div>

{{range .Sections}}
{{$accent := .AccentCSS}}{{$glow := .GlowCSS}}
<div class="section">
  <div class="section-header" style="border-color:{{.AccentCSS}}20; color:{{.AccentCSS}}">
    {{.Stars}} {{.RarityName}} ({{len .Cards}})
  </div>
  <div class="grid">
    {{range .Cards}}
    <div class="card" onclick="toggle(this)"
         style="border-color:{{$accent}}30; box-shadow:0 0 12px {{$accent}}15, inset 0 1px 0 rgba(255,255,255,0.05);"
         onmouseover="this.style.boxShadow='0 4px 20px {{$accent}}30, inset 0 1px 0 rgba(255,255,255,0.08)'"
         onmouseout="this.style.boxShadow='0 0 12px {{$accent}}15, inset 0 1px 0 rgba(255,255,255,0.05)'">
      <div style="position:absolute;top:0;left:0;right:0;height:3px;background:linear-gradient(90deg,transparent,{{$accent}},transparent);border-radius:16px 16px 0 0;opacity:0.6"></div>
      {{if gt .Count 1}}<span class="count">x{{.Count}}</span>{{end}}
      <div class="emoji">{{.Emoji}}</div>
      <div class="name">{{.Name}}</div>
      <div class="stats">
        <span class="atk">ATK {{.ATK}}</span>
        <span class="def">DEF {{.DEF}}</span>
        <span class="spc">{{.SpecialName}} {{.Special}}</span>
      </div>
      <div class="desc">{{.Description}}</div>
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
