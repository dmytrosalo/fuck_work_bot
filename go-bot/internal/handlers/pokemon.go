package handlers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

type pokemonAPI struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Stats []struct {
		BaseStat int `json:"base_stat"`
		Stat     struct {
			Name string `json:"name"`
		} `json:"stat"`
	} `json:"stats"`
	Types []struct {
		Type struct {
			Name string `json:"name"`
		} `json:"type"`
	} `json:"types"`
	Sprites struct {
		Front string `json:"front_default"`
		Other struct {
			OfficialArtwork struct {
				Front string `json:"front_default"`
			} `json:"official-artwork"`
		} `json:"other"`
	} `json:"sprites"`
	Abilities []struct {
		Ability struct {
			Name string `json:"name"`
		} `json:"ability"`
	} `json:"abilities"`
}

var typeEmoji = map[string]string{
	"normal": "⚪", "fire": "🔥", "water": "💧", "electric": "⚡",
	"grass": "🌿", "ice": "❄️", "fighting": "🥊", "poison": "☠️",
	"ground": "🌍", "flying": "🦅", "psychic": "🔮", "bug": "🐛",
	"rock": "🪨", "ghost": "👻", "dragon": "🐉", "dark": "🌑",
	"steel": "⚙️", "fairy": "🧚",
}

// dailyPokemonID returns a deterministic Pokemon ID for a user on a given day.
func dailyPokemonID(userID string) int {
	today := todayKyiv()
	h := fnv.New32a()
	h.Write([]byte(userID + today))
	return int(h.Sum32()%1010) + 1
}

func (b *Bot) handlePokemon(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)
	query := strings.ToLower(strings.TrimSpace(c.Message().Payload))

	// Send loading message
	loading, _ := c.Bot().Send(c.Chat(), "🔴 Шукаємо покемона...")

	// Fetch async — but we need the result, so just run inline with loading message shown
	go func() {
		b.fetchAndSendPokemon(c, userID, query, loading)
	}()

	return nil
}

func (b *Bot) fetchAndSendPokemon(c tele.Context, userID, query string, loading *tele.Message) {

	var url string
	if query != "" {
		url = fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%s", query)
	} else {
		// Daily Pokemon — same for the same user each day
		id := dailyPokemonID(userID)
		url = fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%d", id)
	}

	bot := c.Bot()
	chat := c.Chat()

	sendErr := func(text string) {
		if loading != nil {
			bot.Delete(loading)
		}
		bot.Send(chat, text)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[pokemon] API error: %v", err)
		sendErr("❌ Не вдалося зʼєднатися з PokeAPI")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		sendErr("❌ Покемон не знайдений")
		return
	}
	if resp.StatusCode != 200 {
		log.Printf("[pokemon] API status: %d", resp.StatusCode)
		sendErr("❌ Помилка API")
		return
	}

	var pokemon pokemonAPI
	if err := json.NewDecoder(resp.Body).Decode(&pokemon); err != nil {
		log.Printf("[pokemon] Parse error: %v", err)
		sendErr("❌ Помилка парсингу")
		return
	}

	// Delete loading message
	if loading != nil {
		bot.Delete(loading)
	}

	// Build types
	var types []string
	for _, t := range pokemon.Types {
		emoji := typeEmoji[t.Type.Name]
		if emoji == "" {
			emoji = "❓"
		}
		types = append(types, fmt.Sprintf("%s %s", emoji, t.Type.Name))
	}

	statMap := make(map[string]int)
	for _, s := range pokemon.Stats {
		statMap[s.Stat.Name] = s.BaseStat
	}

	total := 0
	for _, s := range pokemon.Stats {
		total += s.BaseStat
	}

	ability := ""
	if len(pokemon.Abilities) > 0 {
		ability = pokemon.Abilities[0].Ability.Name
	}

	name := strings.ToUpper(pokemon.Name[:1]) + pokemon.Name[1:]

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	var sb strings.Builder
	if query == "" {
		sb.WriteString(fmt.Sprintf("🔴 %s, сьогодні ти — %s!\n\n", userName, name))
	} else {
		sb.WriteString(fmt.Sprintf("🔴 %s\n\n", name))
	}
	sb.WriteString(fmt.Sprintf("#%03d | %s\n\n", pokemon.ID, strings.Join(types, " / ")))
	sb.WriteString(fmt.Sprintf("HP: %d\n", statMap["hp"]))
	sb.WriteString(fmt.Sprintf("ATK: %d  DEF: %d\n", statMap["attack"], statMap["defense"]))
	sb.WriteString(fmt.Sprintf("SP.ATK: %d  SP.DEF: %d\n", statMap["special-attack"], statMap["special-defense"]))
	sb.WriteString(fmt.Sprintf("SPEED: %d\n", statMap["speed"]))
	sb.WriteString(fmt.Sprintf("\nTotal: %d", total))
	if ability != "" {
		sb.WriteString(fmt.Sprintf("\nAbility: %s", ability))
	}

	spriteURL := pokemon.Sprites.Other.OfficialArtwork.Front
	if spriteURL == "" {
		spriteURL = pokemon.Sprites.Front
	}

	if spriteURL != "" {
		photo := &tele.Photo{
			File:    tele.FromURL(spriteURL),
			Caption: sb.String(),
		}
		_, err := bot.Send(chat, photo)
		if err != nil {
			log.Printf("[pokemon] Photo send error (url=%s): %v", spriteURL, err)
			bot.Send(chat, sb.String())
		}
	} else {
		log.Printf("[pokemon] No sprite URL for %s", pokemon.Name)
		bot.Send(chat, sb.String())
	}
}
