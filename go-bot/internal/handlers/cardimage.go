package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

var (
	cardFont     *truetype.Font
	cardFontPath = "./model/Roboto-Bold.ttf"
)

var rarityAccent = map[int]color.RGBA{
	1: {120, 120, 130, 255},  // Common — gray
	2: {50, 180, 100, 255},   // Uncommon — green
	3: {60, 120, 220, 255},   // Rare — blue
	4: {170, 70, 220, 255},   // Epic — purple
	5: {240, 190, 40, 255},   // Legendary — gold
}

var rarityBg = map[int]color.RGBA{
	1: {35, 35, 40, 255},
	2: {25, 38, 32, 255},
	3: {25, 30, 45, 255},
	4: {38, 25, 45, 255},
	5: {45, 38, 22, 255},
}

type CardData struct {
	Name        string
	Rarity      int
	Emoji       string
	Description string
	ATK         int
	DEF         int
	SpecialName string
	Special     int
}

func loadCardFont() error {
	if cardFont != nil {
		return nil
	}
	path := os.Getenv("FONT_PATH")
	if path == "" {
		path = cardFontPath
	}
	fontBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read font: %w", err)
	}
	f, err := freetype.ParseFont(fontBytes)
	if err != nil {
		return fmt.Errorf("parse font: %w", err)
	}
	cardFont = f
	return nil
}

// emojiToCodepoint converts emoji string to hex codepoint for Twemoji URL
func emojiToCodepoint(emoji string) string {
	var codepoints []string
	for i := 0; i < len(emoji); {
		r, size := utf8.DecodeRuneInString(emoji[i:])
		if r != 0xFE0F { // skip variation selector
			codepoints = append(codepoints, fmt.Sprintf("%x", r))
		}
		i += size
	}
	return strings.Join(codepoints, "-")
}

// fetchEmojiPNG downloads emoji as PNG from Twemoji CDN
func fetchEmojiPNG(emoji string) (image.Image, error) {
	code := emojiToCodepoint(emoji)
	url := fmt.Sprintf("https://cdn.jsdelivr.net/gh/twitter/twemoji@14.0.2/assets/72x72/%s.png", code)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("emoji not found: %s", code)
	}

	return png.Decode(resp.Body)
}

func renderCard(card CardData) ([]byte, error) {
	if err := loadCardFont(); err != nil {
		return nil, err
	}

	w, h := 360, 480
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	bg := rarityBg[card.Rarity]
	accent := rarityAccent[card.Rarity]

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Top accent bar
	draw.Draw(img, image.Rect(0, 0, w, 6), &image.Uniform{accent}, image.Point{}, draw.Src)

	// Border
	drawCardBorder(img, accent, 2)

	// Inner card area with slightly lighter bg
	innerBg := color.RGBA{bg.R + 10, bg.G + 10, bg.B + 10, 255}
	draw.Draw(img, image.Rect(15, 50, w-15, h-90), &image.Uniform{innerBg}, image.Point{}, draw.Src)

	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(cardFont)
	c.SetClip(img.Bounds())
	c.SetDst(img)

	// Rarity label
	c.SetFontSize(14)
	c.SetSrc(image.NewUniform(accent))
	stars := rarityStars[card.Rarity]
	label := rarityNames[card.Rarity]
	drawCardText(c, 15, 30, fmt.Sprintf("%s %s", stars, label))

	// PWR top right
	power := card.ATK + card.DEF + card.Special
	c.SetFontSize(13)
	c.SetSrc(image.NewUniform(color.RGBA{160, 160, 160, 255}))
	drawCardText(c, w-80, 30, fmt.Sprintf("PWR %d", power))

	// Emoji in center (fetched from CDN)
	emojiImg, err := fetchEmojiPNG(card.Emoji)
	if err == nil {
		// Scale emoji to ~80x80 and center it
		emojiSize := 72
		ex := (w - emojiSize) / 2
		ey := 75
		draw.Draw(img, image.Rect(ex, ey, ex+emojiSize, ey+emojiSize), emojiImg, image.Point{}, draw.Over)
	}

	// Card name centered
	c.SetFontSize(22)
	c.SetSrc(image.NewUniform(color.White))
	nameLines := wrapCardText(card.Name, 18)
	ny := 185
	for _, line := range nameLines {
		textW := len(line) * 11
		drawCardText(c, (w-textW)/2, ny, line)
		ny += 28
	}

	// Divider
	draw.Draw(img, image.Rect(30, ny+5, w-30, ny+6), &image.Uniform{accent}, image.Point{}, draw.Src)

	// Description
	c.SetFontSize(12)
	c.SetSrc(image.NewUniform(color.RGBA{170, 170, 175, 255}))
	descLines := wrapCardText(card.Description, 30)
	dy := ny + 25
	for _, line := range descLines {
		drawCardText(c, 25, dy, line)
		dy += 17
	}

	// Stats bar at bottom
	statsY := h - 75
	draw.Draw(img, image.Rect(10, statsY, w-10, h-10), &image.Uniform{color.RGBA{25, 25, 30, 255}}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(10, statsY, w-10, statsY+2), &image.Uniform{accent}, image.Point{}, draw.Src)

	c.SetFontSize(16)

	// ATK
	c.SetSrc(image.NewUniform(color.RGBA{255, 90, 90, 255}))
	drawCardText(c, 25, statsY+28, fmt.Sprintf("ATK %d", card.ATK))

	// DEF
	c.SetSrc(image.NewUniform(color.RGBA{90, 140, 255, 255}))
	drawCardText(c, 140, statsY+28, fmt.Sprintf("DEF %d", card.DEF))

	// Special
	c.SetFontSize(14)
	c.SetSrc(image.NewUniform(accent))
	drawCardText(c, 25, statsY+55, fmt.Sprintf("%s: %d", card.SpecialName, card.Special))

	// Encode
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawCardText(c *freetype.Context, x, y int, text string) {
	c.DrawString(text, freetype.Pt(x, y))
}

func drawCardBorder(img *image.RGBA, col color.RGBA, t int) {
	b := img.Bounds()
	for i := 0; i < t; i++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.Set(x, b.Min.Y+i, col)
			img.Set(x, b.Max.Y-1-i, col)
		}
		for y := b.Min.Y; y < b.Max.Y; y++ {
			img.Set(b.Min.X+i, y, col)
			img.Set(b.Max.X-1-i, y, col)
		}
	}
}

func wrapCardText(text string, maxChars int) []string {
	words := strings.Fields(text)
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
		} else if len(current)+1+len(word) <= maxChars {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
