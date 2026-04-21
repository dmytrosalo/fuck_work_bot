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
	mux.HandleFunc("/help", func(w http.ResponseWriter, r *http.Request) {
		handleHelpPage(w, r)
	})
	mux.HandleFunc("/ideas", func(w http.ResponseWriter, r *http.Request) {
		handleIdeasPage(w, r, db)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "fuck-work-bot is running")
	})
}

func handleHelpPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	helpTmpl.Execute(w, nil)
}

func handleIdeasPage(w http.ResponseWriter, r *http.Request, db *storage.DB) {
	ideas := db.GetCardIdeas()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ideasTmpl.Execute(w, ideas)
}

var helpTmpl = template.Must(template.New("help").Parse(helpHTML))
var ideasTmpl = template.Must(template.New("ideas").Parse(ideasHTML))

const helpHTML = `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>FuckWorkBot — Правила</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700;800&display=swap');
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: #0a0a1a;
  color: #e0e0e8;
  font-family: 'Inter', -apple-system, sans-serif;
  padding: 16px;
  max-width: 720px;
  margin: 0 auto;
  line-height: 1.6;
}
h1 { font-size: 24px; margin-bottom: 20px; text-align: center; color: #fff; }
h2 {
  font-size: 16px; font-weight: 700; margin: 24px 0 12px;
  padding: 8px 12px; border-radius: 8px;
  background: rgba(255,255,255,0.04);
  border-left: 3px solid;
}
.eco { border-color: #fbbf24; color: #fbbf24; }
.casino { border-color: #ff6b6b; color: #ff6b6b; }
.cards { border-color: #4ecdc4; color: #4ecdc4; }
.pvp { border-color: #ff8a65; color: #ff8a65; }
.social { border-color: #ba68c8; color: #ba68c8; }
.games { border-color: #64b5f6; color: #64b5f6; }
.evolve { border-color: #81c784; color: #81c784; }
.ach { border-color: #ffd54f; color: #ffd54f; }
.cls { border-color: #90a4ae; color: #90a4ae; }
ul { list-style: none; padding: 0; }
li { padding: 4px 0 4px 12px; font-size: 14px; border-bottom: 1px solid rgba(255,255,255,0.04); }
li:last-child { border-bottom: none; }
code { background: rgba(255,255,255,0.08); padding: 1px 6px; border-radius: 4px; font-size: 13px; }
.cmd { color: #64b5f6; font-weight: 600; }
.note { color: rgba(255,255,255,0.5); font-size: 12px; }
table { width: 100%; border-collapse: collapse; margin: 8px 0; font-size: 13px; }
th { text-align: left; padding: 6px 8px; color: rgba(255,255,255,0.4); font-size: 11px; text-transform: uppercase; }
td { padding: 5px 8px; border-bottom: 1px solid rgba(255,255,255,0.04); }
</style>
</head>
<body>
<h1>📖 Правила та механіки</h1>

<h2 class="eco">💰 Економіка (богдудіки 🪙)</h2>
<ul>
<li>Стартовий баланс: <b>100 🪙</b></li>
<li><span class="cmd">/daily</span> — +75 🪙 на день (титул може давати бонус)</li>
<li><span class="cmd">/work</span> <span class="cmd">/notwork</span> — позначити повідомлення (+10 🪙, 1 раз на повідомлення)</li>
<li><span class="cmd">/balance</span> — перевірити баланс</li>
<li><span class="cmd">/top</span> — лідерборд</li>
<li><span class="cmd">/casino_stats</span> — твоя казино статистика</li>
<li><span class="cmd">/global_stats</span> — загальна статистика</li>
</ul>

<h2 class="casino">🎰 Казино</h2>
<ul>
<li><span class="cmd">/slots &lt;ставка&gt;</span> — слоти, 1-500 🪙, макс 20/день</li>
<li>Три однакових = x2-x50, 💎💎💎 = ДЖЕКПОТ x50</li>
<li><span class="cmd">/blackjack &lt;ставка&gt;</span> (<span class="cmd">/bj</span>) — блекджек, 1-500 🪙, Hit/Stand кнопки</li>
<li>Blackjack (21 з 2 карт) = виплата x2.5</li>
<li class="note">Титули Гемблер/Джекпот/Шулер збільшують макс ставку</li>
</ul>

<h2 class="cards">🃏 Картки (508 шт)</h2>
<table>
<tr><th>Рідкість</th><th>Шанс</th><th>Кількість</th></tr>
<tr><td>⭐ Common</td><td>41.5%</td><td>183</td></tr>
<tr><td>⭐⭐ Uncommon</td><td>30%</td><td>148</td></tr>
<tr><td>⭐⭐⭐ Rare</td><td>20%</td><td>107</td></tr>
<tr><td>⭐⭐⭐⭐ Epic</td><td>7%</td><td>47</td></tr>
<tr><td>⭐⭐⭐⭐⭐ Legendary</td><td>1.2%</td><td>20</td></tr>
<tr><td>💎 ULTRA LEGENDARY</td><td>0.3%</td><td>3</td></tr>
</table>
<ul>
<li><span class="cmd">/pack</span> — відкрити пак: 3 картки, 40 🪙, макс 7/день</li>
<li><span class="cmd">/gacha</span> — преміум пак: 1 картка Epic+, 300 🪙</li>
<li><span class="cmd">/collection</span> — твої картки + вебсторінка</li>
<li><span class="cmd">/card &lt;назва&gt;</span> — подивитись будь-яку картку</li>
<li><span class="cmd">/showcase</span> — показати найкрутішу картку</li>
<li><span class="cmd">/burn &lt;ID або назва&gt;</span> — спалити за монети (5-100 🪙)</li>
<li><span class="cmd">/gift @user &lt;ID або назва&gt;</span> — подарувати картку</li>
<li><span class="cmd">/sacrifice &lt;rarity&gt;</span> — 7 карток → 1 вищої рідкості</li>
<li><span class="cmd">/auction &lt;назва&gt;</span> — аукціон 60 сек, <span class="cmd">/bid &lt;сума&gt;</span></li>
</ul>

<h2 class="evolve">✨ Еволюція</h2>
<ul>
<li><span class="cmd">/evolve &lt;ID або назва&gt;</span> — еволюція картки до Stage 2</li>
<li>Потрібно: 2 копії картки + монети</li>
<li>Stage 2: <b>+20% всіх статів</b>, не можна вкрасти через /steal</li>
</ul>
<table>
<tr><th>Рідкість</th><th>Вартість</th></tr>
<tr><td>Common</td><td>50 🪙</td></tr>
<tr><td>Uncommon</td><td>100 🪙</td></tr>
<tr><td>Rare</td><td>200 🪙</td></tr>
<tr><td>Epic</td><td>500 🪙</td></tr>
<tr><td>Legendary</td><td>1,000 🪙</td></tr>
<tr><td>Ultra</td><td>2,000 🪙</td></tr>
</table>

<h2 class="pvp">⚔️ PvP</h2>
<ul>
<li><span class="cmd">/duel @user</span> → <span class="cmd">/accept</span> — обирай картку з 3, програвший віддає картку</li>
<li><span class="cmd">/war @user</span> → <span class="cmd">/accept</span> — 3 раунди, обирай порядок</li>
<li><span class="cmd">/steal @user</span> — 30% вкрасти картку, 70% штраф 20 🪙 (1/день, мін 5 карток у жертви)</li>
<li><span class="cmd">/rob @user</span> — 33% вкрасти 10-33% монет, 67% штраф 20 🪙 (1/год)</li>
<li><span class="cmd">/dart @user &lt;ставка&gt;</span> — дартс 5 раундів, банк переможцю (5/день)</li>
<li class="note">Stage 2 картки не можна вкрасти. Деякі титули дають захист від /rob та /steal</li>
</ul>

<h2 class="social">🔥 Соціальне</h2>
<ul>
<li><span class="cmd">/roast @user</span> — підколка (5 🪙 за іншого, безкоштовно з титулом Токсик)</li>
<li><span class="cmd">/compliment @user</span> — комплімент</li>
<li><span class="cmd">/quote</span> — випадкова цитата з чату</li>
<li><span class="cmd">/addquote</span> — зберегти цитату (відповідь на повідомлення)</li>
</ul>

<h2 class="games">🎮 Розваги</h2>
<ul>
<li><span class="cmd">/pokemon</span> — покемон дня 🔴</li>
<li><span class="cmd">/horoscope</span> — дев-гороскоп 🔮 (1/день)</li>
<li><span class="cmd">/8ball &lt;питання&gt;</span> — магічна куля 🎱</li>
<li><span class="cmd">/cat</span> 🐱 <span class="cmd">/dog</span> 🐕 — тваринки</li>
<li><span class="cmd">/quiz</span> — вікторина (+5-15 🪙, 10/день)</li>
<li><span class="cmd">/guess</span> — вгадай число 1-100 (2+ гравці, +30/+100 🪙)</li>
<li><span class="cmd">/wordle</span> — wordle (3/день, +5-30 🪙)</li>
</ul>

<h2 class="ach">🏆 Досягнення та титули</h2>
<ul>
<li><span class="cmd">/achievements</span> — прогрес досягнень (50 шт)</li>
<li><span class="cmd">/title &lt;назва&gt;</span> — встановити титул</li>
<li>Кожен титул дає пасивний бонус (більше daily, шанс steal/rob, захист, і т.д.)</li>
<li><span class="cmd">/card_idea &lt;опис&gt;</span> — запропонувати ідею для картки</li>
</ul>

<h2 class="cls">🤖 Класифікатор</h2>
<ul>
<li>Кожне повідомлення аналізується на "робочість"</li>
<li>Робоче (80%+) = 🤡 + підколка</li>
<li><span class="cmd">/check &lt;текст&gt;</span> — перевірити текст вручну</li>
<li><span class="cmd">/stats</span> — статистика класифікацій</li>
<li><span class="cmd">/mute</span> / <span class="cmd">/unmute</span> — вкл/викл трекінг</li>
</ul>

</body>
</html>`

const ideasHTML = `<!DOCTYPE html>
<html lang="uk">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Card Ideas</title>
<style>
@import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700&display=swap');
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  background: #0a0a1a;
  color: #e0e0e8;
  font-family: 'Inter', -apple-system, sans-serif;
  padding: 16px;
  max-width: 720px;
  margin: 0 auto;
}
h1 { font-size: 24px; margin-bottom: 20px; text-align: center; color: #fff; }
.idea {
  background: rgba(255,255,255,0.04);
  border: 1px solid rgba(255,255,255,0.08);
  border-radius: 12px;
  padding: 14px;
  margin-bottom: 10px;
}
.idea .author { font-size: 12px; color: rgba(255,255,255,0.4); margin-bottom: 4px; }
.idea .text { font-size: 14px; line-height: 1.5; }
.empty { text-align: center; color: rgba(255,255,255,0.4); padding: 40px; }
.note { text-align: center; font-size: 13px; color: rgba(255,255,255,0.3); margin-bottom: 20px; }
</style>
</head>
<body>
<h1>💡 Ідеї для карток</h1>
<p class="note">Напиши в чаті: /card_idea назва — опис</p>
{{if .}}
{{range .}}
<div class="idea">
  <div class="author">{{.Name}} · {{.CreatedAt}}</div>
  <div class="text">{{.Idea}}</div>
</div>
{{end}}
{{else}}
<div class="empty">Поки немає ідей. Будь першим!</div>
{{end}}
</body>
</html>`

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
