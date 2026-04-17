package handlers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	today := time.Now().Format("2006-01-02")
	h := fnv.New32a()
	h.Write([]byte(userID + today))
	return int(h.Sum32()%1010) + 1
}

func (b *Bot) handlePokemon(c tele.Context) error {
	userID := fmt.Sprintf("%d", c.Sender().ID)

	// If user specified a name, fetch that
	query := strings.ToLower(strings.TrimSpace(c.Message().Payload))

	var url string
	if query != "" {
		url = fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%s", query)
	} else {
		// Daily Pokemon — same for the same user each day
		id := dailyPokemonID(userID)
		url = fmt.Sprintf("https://pokeapi.co/api/v2/pokemon/%d", id)
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

	// Build types
	var types []string
	for _, t := range pokemon.Types {
		emoji := typeEmoji[t.Type.Name]
		if emoji == "" {
			emoji = "❓"
		}
		types = append(types, fmt.Sprintf("%s %s", emoji, t.Type.Name))
	}

	// Stats
	statMap := make(map[string]int)
	for _, s := range pokemon.Stats {
		statMap[s.Stat.Name] = s.BaseStat
	}

	// Total stats
	total := 0
	for _, s := range pokemon.Stats {
		total += s.BaseStat
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

	// Caption
	var sb strings.Builder
	if query == "" {
		sb.WriteString(fmt.Sprintf("🔴 %s, сьогодні ти — *%s*!\n\n", userName, name))
	} else {
		sb.WriteString(fmt.Sprintf("🔴 *%s*\n\n", name))
	}
	sb.WriteString(fmt.Sprintf("#%03d | %s\n\n", pokemon.ID, strings.Join(types, " / ")))
	sb.WriteString(fmt.Sprintf("❤️ HP: %d\n", statMap["hp"]))
	sb.WriteString(fmt.Sprintf("⚔️ ATK: %d  🛡 DEF: %d\n", statMap["attack"], statMap["defense"]))
	sb.WriteString(fmt.Sprintf("✨ SP.ATK: %d  🔰 SP.DEF: %d\n", statMap["special-attack"], statMap["special-defense"]))
	sb.WriteString(fmt.Sprintf("💨 SPEED: %d\n", statMap["speed"]))
	sb.WriteString(fmt.Sprintf("\n📊 Total: *%d*", total))
	if ability != "" {
		sb.WriteString(fmt.Sprintf("\n🎯 Ability: %s", ability))
	}

	// Try to send with image
	spriteURL := pokemon.Sprites.Other.OfficialArtwork.Front
	if spriteURL == "" {
		spriteURL = pokemon.Sprites.Front
	}

	if spriteURL != "" {
		photo := &tele.Photo{
			File:    tele.FromURL(spriteURL),
			Caption: sb.String(),
		}
		return c.Send(photo, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	// Fallback to text
	return c.Send(sb.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleRandomPokemon returns a truly random Pokemon (not daily).
func (b *Bot) handleRandomPokemon(c tele.Context) error {
	id := rand.Intn(1010) + 1
	c.Message().Payload = fmt.Sprintf("%d", id)
	// Temporarily override to force random
	return b.handlePokemon(c)
}
