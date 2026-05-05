package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type carGame struct {
	Brand     string
	ChatID    int64
	Aliases   []string
	Winner    string
	SentMsg   *tele.Message
	CmdMsg    *tele.Message
	CreatedAt time.Time
}

var (
	activeCarGame = make(map[int64]*carGame)
	carGameMu     sync.Mutex
)

type carBrand struct {
	Query   string
	Name    string
	Domain  string // for Clearbit logo
	Aliases []string
}

var carBrands = []carBrand{
	// German
	{"BMW car", "BMW", "bmw.com", []string{"bmw", "бмв"}},
	{"Mercedes Benz car", "Mercedes-Benz", "mercedes-benz.com", []string{"mercedes", "мерседес", "мерс"}},
	{"Audi car", "Audi", "audi.com", []string{"audi", "ауді"}},
	{"Porsche car", "Porsche", "porsche.com", []string{"porsche", "порше"}},
	{"Volkswagen car", "Volkswagen", "volkswagen.com", []string{"volkswagen", "vw", "фольксваген"}},
	{"Opel car", "Opel", "opel.com", []string{"opel", "опель"}},
	{"Smart car", "Smart", "smart.com", []string{"smart", "смарт"}},
	{"Maybach car", "Maybach", "mercedes-benz.com", []string{"maybach", "майбах"}},
	// Japanese
	{"Toyota car", "Toyota", "toyota.com", []string{"toyota", "тойота"}},
	{"Honda car", "Honda", "honda.com", []string{"honda", "хонда"}},
	{"Nissan car", "Nissan", "nissan.com", []string{"nissan", "ніссан", "нісан"}},
	{"Mazda car", "Mazda", "mazda.com", []string{"mazda", "мазда"}},
	{"Subaru car", "Subaru", "subaru.com", []string{"subaru", "субару"}},
	{"Mitsubishi car", "Mitsubishi", "mitsubishi-motors.com", []string{"mitsubishi", "мітсубіші"}},
	{"Lexus car", "Lexus", "lexus.com", []string{"lexus", "лексус"}},
	{"Infiniti car", "Infiniti", "infiniti.com", []string{"infiniti", "інфініті"}},
	{"Acura car", "Acura", "acura.com", []string{"acura", "акура"}},
	{"Suzuki car", "Suzuki", "suzuki.com", []string{"suzuki", "сузукі"}},
	{"Daihatsu car", "Daihatsu", "daihatsu.com", []string{"daihatsu", "дайхатсу"}},
	{"Isuzu car", "Isuzu", "isuzu.com", []string{"isuzu", "ісузу"}},
	// American
	{"Ford car", "Ford", "ford.com", []string{"ford", "форд"}},
	{"Chevrolet car", "Chevrolet", "chevrolet.com", []string{"chevrolet", "chevy", "шевроле"}},
	{"Tesla car", "Tesla", "tesla.com", []string{"tesla", "тесла"}},
	{"Dodge car", "Dodge", "dodge.com", []string{"dodge", "додж"}},
	{"Jeep car", "Jeep", "jeep.com", []string{"jeep", "джип"}},
	{"Cadillac car", "Cadillac", "cadillac.com", []string{"cadillac", "кадилак"}},
	{"Lincoln car", "Lincoln", "lincoln.com", []string{"lincoln", "лінкольн"}},
	{"GMC car", "GMC", "gmc.com", []string{"gmc", "джіемсі"}},
	{"Buick car", "Buick", "buick.com", []string{"buick", "бʼюік"}},
	{"Chrysler car", "Chrysler", "chrysler.com", []string{"chrysler", "крайслер"}},
	{"Ram truck", "Ram", "ramtrucks.com", []string{"ram", "рем"}},
	{"Corvette car", "Corvette", "chevrolet.com", []string{"corvette", "корвет"}},
	{"Mustang car", "Mustang", "ford.com", []string{"mustang", "мустанг"}},
	{"Hummer car", "Hummer", "hummer.com", []string{"hummer", "хаммер"}},
	// Italian
	{"Ferrari car", "Ferrari", "ferrari.com", []string{"ferrari", "феррарі", "ферарі"}},
	{"Lamborghini car", "Lamborghini", "lamborghini.com", []string{"lamborghini", "ламборгіні", "ламбо"}},
	{"Maserati car", "Maserati", "maserati.com", []string{"maserati", "мазераті"}},
	{"Alfa Romeo car", "Alfa Romeo", "alfaromeo.com", []string{"alfa romeo", "альфа ромео", "альфа"}},
	{"Fiat car", "Fiat", "fiat.com", []string{"fiat", "фіат"}},
	{"Pagani car", "Pagani", "pagani.com", []string{"pagani", "пагані"}},
	{"Lancia car", "Lancia", "lancia.com", []string{"lancia", "лянча"}},
	// British
	{"Rolls Royce car", "Rolls-Royce", "rolls-royce.com", []string{"rolls-royce", "rolls royce", "ролс-ройс"}},
	{"Bentley car", "Bentley", "bentley.com", []string{"bentley", "бентлі"}},
	{"Aston Martin car", "Aston Martin", "astonmartin.com", []string{"aston martin", "астон мартін", "астон"}},
	{"McLaren car", "McLaren", "mclaren.com", []string{"mclaren", "макларен"}},
	{"Jaguar car", "Jaguar", "jaguar.com", []string{"jaguar", "ягуар"}},
	{"Land Rover car", "Land Rover", "landrover.com", []string{"land rover", "ленд ровер"}},
	{"Range Rover car", "Range Rover", "landrover.com", []string{"range rover", "рендж ровер"}},
	{"Mini Cooper car", "Mini", "mini.com", []string{"mini", "міні", "mini cooper"}},
	{"Lotus car", "Lotus", "lotuscars.com", []string{"lotus", "лотус"}},
	{"Morgan car", "Morgan", "morgan-motor.com", []string{"morgan", "морган"}},
	// French
	{"Peugeot car", "Peugeot", "peugeot.com", []string{"peugeot", "пежо"}},
	{"Renault car", "Renault", "renault.com", []string{"renault", "рено"}},
	{"Citroen car", "Citroen", "citroen.com", []string{"citroen", "сітроен"}},
	{"DS car", "DS", "dsautomobiles.com", []string{"ds", "дс"}},
	{"Bugatti car", "Bugatti", "bugatti.com", []string{"bugatti", "бугатті"}},
	{"Alpine car", "Alpine", "alpinecars.com", []string{"alpine", "альпін"}},
	// Korean
	{"Hyundai car", "Hyundai", "hyundai.com", []string{"hyundai", "хюндай", "хундай"}},
	{"Kia car", "Kia", "kia.com", []string{"kia", "кіа"}},
	{"Genesis car", "Genesis", "genesis.com", []string{"genesis", "генезіс"}},
	{"SsangYong car", "SsangYong", "ssangyong.com", []string{"ssangyong", "санг йонг"}},
	// Chinese
	{"BYD car", "BYD", "byd.com", []string{"byd", "бід"}},
	{"Geely car", "Geely", "geely.com", []string{"geely", "джилі"}},
	{"Chery car", "Chery", "cheryinternational.com", []string{"chery", "чері"}},
	{"Haval car", "Haval", "haval.com", []string{"haval", "хавал"}},
	{"Great Wall car", "Great Wall", "gwm.com", []string{"great wall", "грейт вол"}},
	{"NIO car", "NIO", "nio.com", []string{"nio", "ніо"}},
	{"XPeng car", "XPeng", "xpeng.com", []string{"xpeng", "іксменг"}},
	{"Li Auto car", "Li Auto", "lixiang.com", []string{"li auto", "лі авто"}},
	{"Changan car", "Changan", "globalchangan.com", []string{"changan", "чанган"}},
	{"Dongfeng car", "Dongfeng", "dongfeng.com", []string{"dongfeng", "донгфенг"}},
	{"JAC car", "JAC", "jacmotors.com", []string{"jac", "джак"}},
	{"MG car", "MG", "mgmotor.com", []string{"mg", "ем джи"}},
	{"Zeekr car", "Zeekr", "zeekr.com", []string{"zeekr", "зікр"}},
	// Swedish
	{"Volvo car", "Volvo", "volvocars.com", []string{"volvo", "вольво"}},
	{"Koenigsegg car", "Koenigsegg", "koenigsegg.com", []string{"koenigsegg", "коенігсегг"}},
	{"Polestar car", "Polestar", "polestar.com", []string{"polestar", "полстар"}},
	// Czech
	{"Skoda car", "Skoda", "skoda-auto.com", []string{"skoda", "шкода"}},
	// Romanian
	{"Dacia car", "Dacia", "dacia.com", []string{"dacia", "дачія"}},
	// Spanish
	{"SEAT car", "SEAT", "seat.com", []string{"seat", "сеат"}},
	{"Cupra car", "Cupra", "cupraofficial.com", []string{"cupra", "купра"}},
	// Croatian
	{"Rimac car", "Rimac", "rimac-automobili.com", []string{"rimac", "рімак"}},
	// Indian
	{"Tata car", "Tata", "tatamotors.com", []string{"tata", "тата"}},
	{"Mahindra car", "Mahindra", "mahindra.com", []string{"mahindra", "махіндра"}},
	// Malaysian
	{"Proton car", "Proton", "proton.com", []string{"proton", "протон"}},
	// Russian
	{"Lada car", "Lada", "lada.ru", []string{"lada", "лада", "ваз"}},
	{"UAZ car", "UAZ", "uaz.ru", []string{"uaz", "уаз"}},
	// American luxury/sport
	{"Rivian car", "Rivian", "rivian.com", []string{"rivian", "рівіан"}},
	{"Lucid car", "Lucid", "lucidmotors.com", []string{"lucid", "люсід"}},
	// Supercar/Hypercar
	{"SSC car", "SSC", "sscnorthamerica.com", []string{"ssc", "ессесі"}},
	{"Hennessey car", "Hennessey", "hennesseyperformance.com", []string{"hennessey", "хенессі"}},
	{"W Motors car", "W Motors", "wmotors.ae", []string{"w motors", "в моторс"}},
	// Electric
	{"Fisker car", "Fisker", "fiskerinc.com", []string{"fisker", "фіскер"}},
	{"Vinfast car", "VinFast", "vinfast.com", []string{"vinfast", "вінфаст"}},
	// Classic
	{"Porsche 911", "Porsche", "porsche.com", []string{"porsche", "порше", "911"}},
	{"Toyota Supra car", "Toyota", "toyota.com", []string{"toyota", "тойота", "supra", "супра"}},
}

const maxCarPerHour = 10

func (b *Bot) handleCarGuess(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	chatID := c.Chat().ID
	hour := nowHourKyiv()

	carKey := "carguess:" + userID + ":" + hour
	countStr := b.db.GetMeta(carKey)
	carCount := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &carCount)
	}
	if carCount >= maxCarPerHour {
		return c.Reply(fmt.Sprintf("🚗 Ліміт %d на годину. Через %s", maxCarPerHour, timeUntilNextHour()))
	}

	carGameMu.Lock()
	if g, ok := activeCarGame[chatID]; ok && time.Since(g.CreatedAt) < 20*time.Second {
		carGameMu.Unlock()
		return c.Reply("🚗 Гра вже йде! Вгадуй марку!")
	}
	carGameMu.Unlock()

	brand := carBrands[rand.Intn(len(carBrands))]

	unsplashKey := os.Getenv("UNSPLASH_ACCESS_KEY")
	if unsplashKey == "" {
		return c.Reply("❌ Unsplash API не налаштований")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("https://api.unsplash.com/photos/random?query=%s&orientation=landscape&client_id=%s",
		strings.ReplaceAll(brand.Query, " ", "+"), unsplashKey)

	resp, err := client.Get(url)
	if err != nil {
		return c.Reply("❌ Не вдалося отримати фото")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return c.Reply("❌ Unsplash API помилка")
	}

	var photo struct {
		URLs struct {
			Regular string `json:"regular"`
		} `json:"urls"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&photo); err != nil || photo.URLs.Regular == "" {
		return c.Reply("❌ Фото не знайдено")
	}

	b.db.SetMeta(carKey, fmt.Sprintf("%d", carCount+1))

	carGameMu.Lock()
	activeCarGame[chatID] = &carGame{
		Brand:     brand.Name,
		ChatID:    chatID,
		Aliases:   brand.Aliases,
		CmdMsg:    c.Message(),
		CreatedAt: time.Now(),
	}
	carGameMu.Unlock()

	telePhoto := &tele.Photo{
		File:    tele.FromURL(photo.URLs.Regular),
		Caption: "🚗 Що за марка? (20 сек)\nНагорода: +15 🪙",
	}
	sent, _ := c.Bot().Send(c.Chat(), telePhoto)

	carGameMu.Lock()
	if g, ok := activeCarGame[chatID]; ok {
		g.SentMsg = sent
	}
	carGameMu.Unlock()

	go func() {
		time.Sleep(20 * time.Second)
		carGameMu.Lock()
		game, ok := activeCarGame[chatID]
		if ok && game.Winner == "" {
			sentMsg := game.SentMsg
			cmdMsg := game.CmdMsg
			name := game.Brand
			delete(activeCarGame, chatID)
			carGameMu.Unlock()
			if sentMsg != nil {
				c.Bot().Delete(sentMsg)
			}
			if cmdMsg != nil {
				c.Bot().Delete(cmdMsg)
			}
			msg, _ := c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🚗 Час вийшов! Це було: %s", name))
			autoDelete(c.Bot(), 5*time.Second, msg)
		} else {
			carGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkCarAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	carGameMu.Lock()
	game, ok := activeCarGame[chatID]
	if !ok {
		carGameMu.Unlock()
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
		carGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeCarGame, chatID)
	carGameMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "carguess", reward)

	c.Reply(fmt.Sprintf("🏎️ %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Brand, reward, newBal))
	return true
}

// --- Logo Guess ---

type logoGame struct {
	Brand     string
	ChatID    int64
	Aliases   []string
	Winner    string
	SentMsg   *tele.Message
	CmdMsg    *tele.Message
	CreatedAt time.Time
}

var (
	activeLogoGame = make(map[int64]*logoGame)
	logoGameMu     sync.Mutex
)

func (b *Bot) handleLogoGuess(c tele.Context) error {
	chatID := c.Chat().ID

	logoGameMu.Lock()
	if g, ok := activeLogoGame[chatID]; ok && time.Since(g.CreatedAt) < 20*time.Second {
		logoGameMu.Unlock()
		return c.Reply("🏷️ Гра вже йде! Вгадуй бренд!")
	}
	logoGameMu.Unlock()

	// Try up to 5 brands until we find one with a working logo
	var brand carBrand
	var sent *tele.Message
	for attempt := 0; attempt < 5; attempt++ {
		brand = carBrands[rand.Intn(len(carBrands))]
		logoURL := fmt.Sprintf("https://logo.clearbit.com/%s?size=200", brand.Domain)

		// Check if logo exists
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Head(logoURL)
		if err != nil || resp.StatusCode != 200 {
			continue
		}

		telePhoto := &tele.Photo{
			File:    tele.FromURL(logoURL),
			Caption: "🏷️ Чий це логотип? (20 сек)\nНагорода: +15 🪙",
		}
		sent, err = c.Bot().Send(c.Chat(), telePhoto)
		if err == nil {
			break
		}
	}

	if sent == nil {
		return c.Reply("❌ Логотип не знайдено, спробуй ще раз")
	}

	logoGameMu.Lock()
	activeLogoGame[chatID] = &logoGame{
		Brand:     brand.Name,
		ChatID:    chatID,
		Aliases:   brand.Aliases,
		CmdMsg:    c.Message(),
		SentMsg:   sent,
		CreatedAt: time.Now(),
	}
	logoGameMu.Unlock()

	go func() {
		time.Sleep(20 * time.Second)
		logoGameMu.Lock()
		game, ok := activeLogoGame[chatID]
		if ok && game.Winner == "" {
			sentMsg := game.SentMsg
			cmdMsg := game.CmdMsg
			name := game.Brand
			delete(activeLogoGame, chatID)
			logoGameMu.Unlock()
			if sentMsg != nil {
				c.Bot().Delete(sentMsg)
			}
			if cmdMsg != nil {
				c.Bot().Delete(cmdMsg)
			}
			msg, _ := c.Bot().Send(&tele.Chat{ID: chatID}, fmt.Sprintf("🏷️ Час вийшов! Це було: %s", name))
			autoDelete(c.Bot(), 5*time.Second, msg)
		} else {
			logoGameMu.Unlock()
		}
	}()

	return nil
}

func (b *Bot) checkLogoAnswer(c tele.Context) bool {
	chatID := c.Chat().ID
	text := strings.ToLower(strings.TrimSpace(c.Text()))

	logoGameMu.Lock()
	game, ok := activeLogoGame[chatID]
	if !ok {
		logoGameMu.Unlock()
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
		logoGameMu.Unlock()
		return false
	}

	userID := fmt.Sprintf("%d", c.Sender().ID)
	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	game.Winner = userID
	delete(activeLogoGame, chatID)
	logoGameMu.Unlock()

	reward := 15
	newBal := b.db.UpdateBalance(userID, userName, reward)
	b.db.LogTransaction(userID, userName, "logo", reward)

	c.Reply(fmt.Sprintf("🏷️ %s вгадав! Це %s!\n+%d 🪙 (баланс: %d)", userName, game.Brand, reward, newBal))
	return true
}
