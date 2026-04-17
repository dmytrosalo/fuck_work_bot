package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
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
	} `json:"sprites"`
	Abilities []struct {
		Ability struct {
			Name string `json:"name"`
		} `json:"ability"`
	} `json:"abilities"`
}

var typeEmoji = map[string]string{
	"normal":   "⚪", "fire": "🔥", "water": "💧", "electric": "⚡",
	"grass":    "🌿", "ice": "❄️", "fighting": "🥊", "poison": "☠️",
	"ground":   "🌍", "flying": "🦅", "psychic": "🔮", "bug": "🐛",
	"rock":     "🪨", "ghost": "👻", "dragon": "🐉", "dark": "🌑",
	"steel":    "⚙️", "fairy": "🧚",
}

func (b *Bot) handlePokemon(c tele.Context) error {
	// Random pokemon ID (1-1010)
	id := rand.Intn(1010) + 1

	// If user specified a name
	query := strings.ToLower(strings.TrimSpace(c.Message().Payload))
	url := fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%d", id)
	if query != "" {
		url = fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%s", query)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return c.Reply("❌ Не вдалося зʼєднатися з PokeAPI")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return c.Reply("❌ Покемон не знайдений")
	}
	if resp.StatusCode != 200 {
		return c.Reply("❌ Помилка API")
	}

	var pokemon pokemonAPI
	if err := json.NewDecoder(resp.Body).Decode(&pokemon); err != nil {
		return c.Reply("❌ Помилка парсингу")
	}

	// Build types string
	var types []string
	for _, t := range pokemon.Types {
		emoji := typeEmoji[t.Type.Name]
		if emoji == "" {
			emoji = "❓"
		}
		types = append(types, fmt.Sprintf("%s %s", emoji, t.Type.Name))
	}

	// Build stats
	statMap := make(map[string]int)
	for _, s := range pokemon.Stats {
		statMap[s.Stat.Name] = s.BaseStat
	}

	// Ability
	ability := ""
	if len(pokemon.Abilities) > 0 {
		ability = pokemon.Abilities[0].Ability.Name
	}

	// Name capitalized
	name := strings.ToUpper(pokemon.Name[:1]) + pokemon.Name[1:]

	userName := c.Sender().FirstName
	if userName == "" {
		userName = c.Sender().Username
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔴 *%s, це твій покемон!*\n\n", userName))
	sb.WriteString(fmt.Sprintf("*#%03d %s*\n", pokemon.ID, name))
	sb.WriteString(fmt.Sprintf("Type: %s\n\n", strings.Join(types, " / ")))
	sb.WriteString(fmt.Sprintf("❤️ HP: %d\n", statMap["hp"]))
	sb.WriteString(fmt.Sprintf("⚔️ ATK: %d  🛡 DEF: %d\n", statMap["attack"], statMap["defense"]))
	sb.WriteString(fmt.Sprintf("✨ SP.ATK: %d  🔰 SP.DEF: %d\n", statMap["special-attack"], statMap["special-defense"]))
	sb.WriteString(fmt.Sprintf("💨 SPEED: %d\n", statMap["speed"]))
	if ability != "" {
		sb.WriteString(fmt.Sprintf("\n🎯 Ability: %s", ability))
	}

	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
