package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

// lat/lng to tile coordinates
func latLngToTile(lat, lng float64, zoom int) (int, int) {
	n := math.Pow(2, float64(zoom))
	x := int((lng + 180.0) / 360.0 * n)
	latRad := lat * math.Pi / 180.0
	y := int((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n)
	return x, y
}

// fetchTile downloads a single map tile (no labels)
func fetchTile(z, x, y int) (image.Image, error) {
	// CartoDB no-labels tiles — no text, just terrain and borders
	url := fmt.Sprintf("https://basemaps.cartocdn.com/light_nolabels/%d/%d/%d.png", z, x, y)
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "FuckWorkBot/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return png.Decode(resp.Body)
}

// renderMap creates a 3x3 tile map image
func renderMap(lat, lng float64, zoom int) ([]byte, error) {
	cx, cy := latLngToTile(lat, lng, zoom)
	tileSize := 256
	img := image.NewRGBA(image.Rect(0, 0, tileSize*3, tileSize*3))

	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			tile, err := fetchTile(zoom, cx+dx, cy+dy)
			if err != nil {
				continue
			}
			offsetX := (dx + 1) * tileSize
			offsetY := (dy + 1) * tileSize
			draw.Draw(img, image.Rect(offsetX, offsetY, offsetX+tileSize, offsetY+tileSize), tile, image.Point{}, draw.Src)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type mapGame struct {
	Country   string
	ChatID    int64
	Aliases   []string
	Zoom      int
	Winner    string
	SentMsg   *tele.Message
	CmdMsg    *tele.Message
	CreatedAt time.Time
}

var (
	activeMapGame = make(map[int64]*mapGame)
	mapGameMu     sync.Mutex
)

type countryCoord struct {
	Name    string
	UkName  string
	Lat     float64
	Lng     float64
	Aliases []string
}

var mapCountries []countryCoord

func loadMapCountries() {
	if len(mapCountries) > 0 {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://restcountries.com/v3.1/all?fields=name,translations,latlng,cca2")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var countries []struct {
		Name struct {
			Common string `json:"common"`
		} `json:"name"`
		Translations map[string]struct {
			Common string `json:"common"`
		} `json:"translations"`
		LatLng []float64 `json:"latlng"`
		CCA2   string    `json:"cca2"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&countries); err != nil {
		return
	}

	for _, c := range countries {
		if len(c.LatLng) < 2 {
			continue
		}

		ukName := c.Name.Common
		if t, ok := c.Translations["ukr"]; ok && t.Common != "" {
			ukName = t.Common
		}

		aliases := []string{
			strings.ToLower(ukName),
			strings.ToLower(c.Name.Common),
			strings.ToLower(c.CCA2),
		}

		mapCountries = append(mapCountries, countryCoord{
			Name:    c.Name.Common,
			UkName:  ukName,
			Lat:     c.LatLng[0],
			Lng:     c.LatLng[1],
			Aliases: aliases,
		})
	}
}

var zoomLevels = []struct {
	Zoom int
	Name string
}{
	{4, "🔴 Hard"},
	{5, "🔴 Hard"},
	{6, "🟡 Medium"},
	{7, "🟡 Medium"},
	{8, "🟢 Easy"},
	{9, "🟢 Easy"},
}

func (b *Bot) handleMapGuess(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID
	hour := nowHourKyiv()

	// 10 per hour limit
	mapKey := "map:" + userID + ":" + hour
	countStr := b.db.GetMeta(mapKey)
	mapCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &mapCount)
	}
	if mapCount >= 10 {
		return c.Reply(fmt.Sprintf("🗺️ Ліміт 10 на годину. Через %s", timeUntilNextHour()))
	}

	mapGameMu.Lock()
	if g, ok := activeMapGame[chatID]; ok && time.Since(g.CreatedAt) < 20*time.Second {
		mapGameMu.Unlock()
		return c.Reply("🗺️ Гра вже йде! Вгадуй країну!")
	}
	mapGameMu.Unlock()

	loadMapCountries()
	if len(mapCountries) == 0 {
		return c.Reply("❌ Не вдалося завантажити країни")
	}

	// Pick random country and zoom
	country := mapCountries[rand.Intn(len(mapCountries))]
	zl := zoomLevels[rand.Intn(len(zoomLevels))]

	// Add some random offset to make it harder
	latOffset := (rand.Float64() - 0.5) * 2.0
	lngOffset := (rand.Float64() - 0.5) * 2.0

	// Render map from OSM tiles
	mapBytes, err := renderMap(country.Lat+latOffset, country.Lng+lngOffset, zl.Zoom)
	if err != nil {
		return c.Reply("❌ Не вдалося отримати карту")
	}

	b.db.SetMeta(mapKey, fmt.Sprintf("%d", mapCount+1))

	mapGameMu.Lock()
	activeMapGame[chatID] = &mapGame{
		Country:   country.UkName,
		ChatID:    chatID,
		Aliases:   country.Aliases,
		Zoom:      zl.Zoom,
		CmdMsg:    c.Message(),
		CreatedAt: time.Now(),
	}
	mapGameMu.Unlock()

	telePhoto := &tele.Photo{
		File:    tele.FromReader(bytes.NewReader(mapBytes)),
		Caption: fmt.Sprintf("🗺️ Де це? %s (20 сек)\nНагорода: +15 🪙", zl.Name),
	}
	sent, err := c.Bot().Send(c.Chat(), telePhoto)
	if err != nil {
		mapGameMu.Lock()
		delete(activeMapGame, chatID)
		mapGameMu.Unlock()
		return c.Reply("❌ Не вдалося відправити карту")
	}

	mapGameMu.Lock()
	if g, ok := activeMapGame[chatID]; ok {
		g.SentMsg = sent
	}
	mapGameMu.Unlock()

	bot := c.Bot()
	cmdMsg := c.Message()

	go func() {
		time.Sleep(20 * time.Second)
		mapGameMu.Lock()
		game, ok := activeMapGame[chatID]
		if ok && game.Winner == "" {
			sentMsg := game.SentMsg
			name := game.Country
			delete(activeMapGame, chatID)
			mapGameMu.Unlock()
			if sentMsg != nil {
				bot.Delete(sentMsg)
			}
			if cmdMsg != nil {
				bot.Delete(cmdMsg)
			}
			msg, _ := bot.Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🗺️ Час вийшов! Це було: %s", name))
			autoDelete(bot, 5*time.Second, msg)
		} else {
			mapGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkMapAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	mapGameMu.Lock()
	game, ok := activeMapGame[chatID]
	if !ok {
		mapGameMu.Unlock()
		return false
	}

	correct := false
	for _, alias := range game.Aliases {
		if strings.Contains(text, alias) {
			correct = true
			break
		}
	}

	if !correct {
		mapGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeMapGame, chatID)
	mapGameMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "map", reward)

	c.Reply(fmt.Sprintf("🗺️ %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Country, reward, newBal))
	return true
}
