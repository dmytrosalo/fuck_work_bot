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
	Title        string
	Unique       int
	Total        int
	Balance      int
	RarityCounts map[int]int
	Sections     []raritySection
	Stats        storage.UserStats
	Achievements []achievementDisplay
	AchCount     int
	AchTotal     int
}

type achievementDisplay struct {
	Emoji       string
	Name        string
	Description string
	Unlocked    bool
	Hidden      bool
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
	1: "rgb(60,60,70)",
	2: "rgb(25,75,45)",
	3: "rgb(25,50,110)",
	4: "rgb(85,25,120)",
	5: "rgb(110,85,15)",
	6: "rgb(130,20,20)",
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

	title := db.GetActiveTitle(userID)
	userStats := db.GetUserStats(userID)
	unlockedIDs := db.GetUnlockedAchievements(userID)
	unlockedSet := make(map[string]bool)
	for _, id := range unlockedIDs {
		unlockedSet[id] = true
	}
	var achDisplays []achievementDisplay
	for _, a := range allAchievements {
		achDisplays = append(achDisplays, achievementDisplay{
			Emoji:       a.Emoji,
			Name:        a.Name,
			Description: a.Description,
			Unlocked:    unlockedSet[a.ID],
			Hidden:      a.Hidden,
		})
	}

	data := collectionPageData{
		UserName:     userName,
		Title:        title,
		Unique:       unique,
		Total:        total,
		Balance:      balance,
		RarityCounts: rarityCounts,
		Sections:     sections,
		Stats:        userStats,
		Achievements: achDisplays,
		AchCount:     len(unlockedIDs),
		AchTotal:     len(allAchievements),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := collectionTmpl.Execute(w, data); err != nil {
		http.Error(w, "render error", 500)
	}
}

var collectionFuncs = template.FuncMap{
	"power": func(a, d, s, stage int) int {
		total := a + d + s
		if stage >= 2 {
			total = total * 120 / 100
		}
		return total
	},
	"boost": func(val, stage int) int {
		if stage >= 2 {
			return val + val*20/100
		}
		return val
	},
	"cardName": func(name string, stage int) string {
		if stage >= 2 {
			return name + "+"
		}
		return name
	},
}

var collectionTmpl = template.Must(template.New("collection").Funcs(collectionFuncs).Parse(collectionHTML))

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
  background-image: radial-gradient(ellipse at 20% 50%, rgba(60,40,120,0.12) 0%, transparent 50%),
                    radial-gradient(ellipse at 80% 20%, rgba(40,80,140,0.08) 0%, transparent 50%);
  color: #e8e8f0;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
  padding: 12px;
  max-width: 860px;
  margin: 0 auto;
  min-height: 100vh;
}
.header {
  text-align: center;
  padding: 24px 16px;
  margin-bottom: 24px;
  background: rgba(255,255,255,0.04);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 20px;
}
.header h1 {
  font-size: 26px;
  font-weight: 800;
  background: linear-gradient(135deg, #fff, #a0a0c0);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  margin-bottom: 4px;
}
.header .progress { font-size: 14px; color: rgba(255,255,255,0.5); margin-bottom: 2px; }
.header .balance { font-size: 14px; font-weight: 600; color: #fbbf24; }
.rarity-counts {
  display: flex; flex-wrap: wrap; justify-content: center;
  gap: 6px; margin-top: 12px;
}
.rarity-counts span {
  font-size: 11px; font-weight: 600; padding: 4px 10px;
  border-radius: 16px; background: rgba(255,255,255,0.05);
}
.section { margin-bottom: 24px; }
.section-header {
  font-size: 14px; font-weight: 700; padding: 8px 14px;
  margin-bottom: 12px; border-radius: 10px;
  background: rgba(255,255,255,0.03);
  border-left: 3px solid;
}
.grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
}
@media (min-width: 480px) { .grid { grid-template-columns: repeat(3, 1fr); } }
@media (min-width: 720px) { .grid { grid-template-columns: repeat(4, 1fr); } }

/* TCG Card */
.card {
  position: relative;
  border-radius: 12px;
  padding: 0;
  text-align: center;
  cursor: pointer;
  transition: transform 0.2s, box-shadow 0.3s;
  border: 2px solid;
  overflow: hidden;
  aspect-ratio: 3/4;
  display: flex;
  flex-direction: column;
}
.card:hover { transform: translateY(-4px) scale(1.02); }
.card:active { transform: scale(0.97); }

/* Card frame top — rarity color gradient */
.card-top {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  min-height: 0;
}
.card-top .emoji {
  font-size: 44px;
  filter: drop-shadow(0 3px 6px rgba(0,0,0,0.4));
}
.card-top .card-id {
  position: absolute;
  top: 6px; left: 6px;
  font-size: 9px; font-weight: 700;
  color: rgba(255,255,255,0.35);
}
.card-top .badge {
  position: absolute;
  top: 6px; right: 6px;
  font-size: 9px; font-weight: 800;
  background: rgba(0,0,0,0.5);
  padding: 2px 6px; border-radius: 8px;
  color: rgba(255,255,255,0.85);
}
.card-top .power {
  position: absolute;
  bottom: 6px; right: 6px;
  font-size: 14px; font-weight: 800;
  text-shadow: 0 1px 4px rgba(0,0,0,0.6);
}

/* Card info bottom panel */
.card-info {
  background: rgba(0,0,0,0.55);
  padding: 8px 8px 10px;
  border-top: 1px solid rgba(255,255,255,0.08);
}
.card-info .name {
  font-size: 11px;
  font-weight: 700;
  color: #fff;
  line-height: 1.2;
  margin-bottom: 6px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.card-info .stats {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  justify-content: center;
}
.card-info .stats span {
  font-size: 8px;
  font-weight: 700;
  padding: 2px 5px;
  border-radius: 4px;
  background: rgba(255,255,255,0.1);
  text-transform: uppercase;
  letter-spacing: 0.3px;
}
.stat-atk { color: #ff6b6b !important; }
.stat-def { color: #4ecdc4 !important; }
.stat-spc { color: #fbbf24 !important; }

/* Card description on expand */
.card .desc {
  display: none;
  font-size: 9px;
  color: rgba(255,255,255,0.6);
  margin-top: 4px;
  line-height: 1.3;
  padding: 0 2px;
}
.card.expanded .desc { display: block; }
.card.expanded { aspect-ratio: auto; }
.card.evolved { border-width: 2px; }
.card.evolved .name { text-shadow: 0 0 8px currentColor; }
.title {
  font-size: 13px;
  font-weight: 600;
  color: #fbbf24;
  margin-bottom: 4px;
}
.stats-panel {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 10px;
  margin-bottom: 24px;
}
@media (min-width: 600px) {
  .stats-panel { grid-template-columns: repeat(4, 1fr); }
}
.stat-group {
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 12px;
  padding: 12px;
}
.stat-title {
  font-size: 11px;
  font-weight: 700;
  text-transform: uppercase;
  color: rgba(255,255,255,0.4);
  margin-bottom: 8px;
  letter-spacing: 0.5px;
}
.stat-row {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  padding: 2px 0;
}
.stat-row span:first-child { color: rgba(255,255,255,0.5); }
.stat-row span:last-child { font-weight: 600; }
.ach-section { margin-bottom: 24px; }
.ach-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}
@media (min-width: 500px) {
  .ach-grid { grid-template-columns: repeat(5, 1fr); }
}
.ach-badge {
  text-align: center;
  padding: 10px 4px;
  border-radius: 10px;
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
}
.ach-badge.unlocked {
  background: rgba(251,191,36,0.08);
  border-color: rgba(251,191,36,0.2);
}
.ach-badge.locked { opacity: 0.4; }
.ach-emoji { font-size: 24px; display: block; }
.ach-name { font-size: 9px; font-weight: 600; display: block; margin-top: 4px; }
</style>
</head>
<body>
<div class="header">
  <h1>{{.UserName}}</h1>
  {{if .Title}}<div class="title">{{.Title}}</div>{{end}}
  <div class="progress">{{.Unique}} / {{.Total}} cards</div>
  <div class="balance">{{.Balance}} coins</div>
  <div class="rarity-counts">
    {{range $s := .Sections}}
    <span style="border:1px solid {{$s.AccentCSS}}50; color:{{$s.AccentCSS}}">{{$s.Stars}} {{index $.RarityCounts $s.Rarity}}</span>
    {{end}}
  </div>
</div>

<div class="stats-panel">
  <div class="stat-group">
    <div class="stat-title">Економіка</div>
    <div class="stat-row"><span>Заробив</span><span>{{.Stats.TotalEarned}} 🪙</span></div>
    <div class="stat-row"><span>Витратив</span><span>{{.Stats.TotalSpent}} 🪙</span></div>
    <div class="stat-row"><span>Макс баланс</span><span>{{.Stats.MaxBalance}} 🪙</span></div>
    <div class="stat-row"><span>Паки</span><span>{{.Stats.PacksOpened}}</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">PvP</div>
    <div class="stat-row"><span>Дуелі</span><span>{{.Stats.DuelsWon}}W / {{.Stats.DuelsLost}}L</span></div>
    <div class="stat-row"><span>Серія</span><span>{{.Stats.MaxDuelStreak}}</span></div>
    <div class="stat-row"><span>Вкрадено карт</span><span>{{.Stats.CardsStolen}}</span></div>
    <div class="stat-row"><span>Пограбовано</span><span>{{.Stats.CoinsRobbed}} 🪙</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">Казино</div>
    <div class="stat-row"><span>Слоти</span><span>{{.Stats.SlotsPlayed}}</span></div>
    <div class="stat-row"><span>Блекджек</span><span>{{.Stats.BJPlayed}}</span></div>
    <div class="stat-row"><span>Макс виграш</span><span>{{.Stats.SlotsMaxWin}} 🪙</span></div>
    <div class="stat-row"><span>BJ натурал</span><span>{{.Stats.BJBlackjacks}}</span></div>
  </div>
  <div class="stat-group">
    <div class="stat-title">Соціальне</div>
    <div class="stat-row"><span>Роасти</span><span>{{.Stats.RoastsGiven}}</span></div>
    <div class="stat-row"><span>Цитати</span><span>{{.Stats.QuotesAdded}}</span></div>
    <div class="stat-row"><span>Подаровано</span><span>{{.Stats.CardsGifted}}</span></div>
    <div class="stat-row"><span>Wordle</span><span>{{.Stats.WordlePlayed}}</span></div>
  </div>
</div>

<div class="ach-section">
  <div class="section-header" style="border-color:#fbbf24; color:#fbbf24">
    🏆 Досягнення ({{.AchCount}}/{{.AchTotal}})
  </div>
  <div class="ach-grid">
    {{range .Achievements}}
    {{if .Unlocked}}
    <div class="ach-badge unlocked">
      <span class="ach-emoji">{{.Emoji}}</span>
      <span class="ach-name">{{.Name}}</span>
    </div>
    {{else if .Hidden}}
    <div class="ach-badge locked">
      <span class="ach-emoji">🔒</span>
      <span class="ach-name">???</span>
    </div>
    {{else}}
    <div class="ach-badge locked">
      <span class="ach-emoji">{{.Emoji}}</span>
      <span class="ach-name">{{.Name}}</span>
    </div>
    {{end}}
    {{end}}
  </div>
</div>

{{range .Sections}}
{{$accent := .AccentCSS}}{{$bg := .BgCSS}}
<div class="section">
  <div class="section-header" style="border-color:{{.AccentCSS}}; color:{{.AccentCSS}}">
    {{.Stars}} {{.RarityName}} ({{len .Cards}})
  </div>
  <div class="grid">
    {{range .Cards}}
    <div class="card{{if ge .Stage 2}} evolved{{end}}" onclick="toggle(this)"
         style="border-color:{{$accent}}; background:{{$bg}}; box-shadow:0 0 10px {{$accent}}25{{if ge .Stage 2}}, 0 0 20px {{$accent}}50, inset 0 0 15px {{$accent}}15{{end}};">
      <div class="card-top" style="background:linear-gradient(180deg, {{$bg}} 0%, rgba(0,0,0,0.3) 100%);">
        <span class="card-id">#{{.ID}}{{if ge .Stage 2}} ✨{{end}}</span>
        <div class="emoji">{{.Emoji}}</div>
        <span class="power" style="color:{{$accent}}">{{power .ATK .DEF .Special .Stage}}</span>
        {{if gt .Count 1}}<span class="badge">x{{.Count}}</span>{{end}}
      </div>
      <div class="card-info">
        <div class="name" style="color:{{$accent}}">{{cardName .Name .Stage}}</div>
        <div class="stats">
          <span class="stat-atk">ATK {{boost .ATK .Stage}}</span>
          <span class="stat-def">DEF {{boost .DEF .Stage}}</span>
          <span class="stat-spc">{{.SpecialName}} {{boost .Special .Stage}}</span>
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
  document.querySelectorAll('.card.expanded').forEach(function(c) {
    if (c !== el) c.classList.remove('expanded');
  });
  el.classList.toggle('expanded');
}
</script>
</body>
</html>`
