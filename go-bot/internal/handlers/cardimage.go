package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

var (
	cardFont     *truetype.Font
	cardFontPath = "./model/Roboto-Bold.ttf"
)

var rarityColors = map[int]color.RGBA{
	1: {80, 80, 80, 255},     // Common — dark gray
	2: {30, 120, 70, 255},    // Uncommon — green
	3: {30, 80, 180, 255},    // Rare — blue
	4: {130, 40, 180, 255},   // Epic — purple
	5: {200, 160, 30, 255},   // Legendary — gold
}

var rarityBgColors = map[int]color.RGBA{
	1: {40, 40, 45, 255},
	2: {25, 40, 35, 255},
	3: {25, 30, 50, 255},
	4: {40, 25, 50, 255},
	5: {50, 42, 20, 255},
}

func loadFont() error {
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

func renderCardImage(card CardData) ([]byte, error) {
	if err := loadFont(); err != nil {
		return nil, err
	}

	width, height := 400, 520
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Background
	bg := rarityBgColors[card.Rarity]
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Rarity accent bar at top
	accent := rarityColors[card.Rarity]
	draw.Draw(img, image.Rect(0, 0, width, 8), &image.Uniform{accent}, image.Point{}, draw.Src)

	// Border
	borderColor := rarityColors[card.Rarity]
	drawBorder(img, borderColor, 3)

	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(cardFont)
	c.SetClip(img.Bounds())
	c.SetDst(img)

	// Rarity stars + name
	stars := rarityStars[card.Rarity]
	rarityLabel := rarityNames[card.Rarity]

	// Stars line
	c.SetFontSize(16)
	c.SetSrc(image.NewUniform(accent))
	drawText(c, 20, 35, fmt.Sprintf("%s %s", stars, rarityLabel))

	// Emoji large
	c.SetFontSize(80)
	c.SetSrc(image.NewUniform(color.White))
	emojiWidth := len(card.Emoji) * 20 // rough estimate
	drawText(c, (width-emojiWidth)/2, 160, card.Emoji)

	// Card name
	c.SetFontSize(24)
	c.SetSrc(image.NewUniform(color.White))
	nameLines := wrapText(card.Name, 20)
	y := 220
	for _, line := range nameLines {
		textW := len(line) * 12
		drawText(c, (width-textW)/2, y, line)
		y += 30
	}

	// Divider line
	dividerColor := color.RGBA{accent.R, accent.G, accent.B, 100}
	draw.Draw(img, image.Rect(30, y+5, width-30, y+7), &image.Uniform{dividerColor}, image.Point{}, draw.Src)

	// Description
	c.SetFontSize(13)
	c.SetSrc(image.NewUniform(color.RGBA{180, 180, 180, 255}))
	descLines := wrapText(card.Description, 35)
	y += 25
	for _, line := range descLines {
		drawText(c, 25, y, line)
		y += 18
	}

	// Stats bar at bottom
	statsY := height - 80

	// Stats background
	draw.Draw(img, image.Rect(15, statsY-10, width-15, height-15), &image.Uniform{color.RGBA{30, 30, 35, 255}}, image.Point{}, draw.Src)

	c.SetFontSize(18)
	c.SetSrc(image.NewUniform(color.RGBA{255, 100, 100, 255}))
	drawText(c, 30, statsY+15, fmt.Sprintf("⚔️ %d", card.ATK))

	c.SetSrc(image.NewUniform(color.RGBA{100, 150, 255, 255}))
	drawText(c, 140, statsY+15, fmt.Sprintf("🛡 %d", card.DEF))

	c.SetSrc(image.NewUniform(accent))
	drawText(c, 30, statsY+45, fmt.Sprintf("%s: %d", card.SpecialName, card.Special))

	// Total power bottom right
	total := card.ATK + card.DEF + card.Special
	c.SetFontSize(14)
	c.SetSrc(image.NewUniform(color.RGBA{150, 150, 150, 255}))
	drawText(c, width-100, statsY+45, fmt.Sprintf("PWR: %d", total))

	// Encode
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func drawText(c *freetype.Context, x, y int, text string) {
	pt := freetype.Pt(x, y)
	c.DrawString(text, pt)
}

func drawBorder(img *image.RGBA, col color.RGBA, thickness int) {
	b := img.Bounds()
	for t := 0; t < thickness; t++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.Set(x, b.Min.Y+t, col)
			img.Set(x, b.Max.Y-1-t, col)
		}
		for y := b.Min.Y; y < b.Max.Y; y++ {
			img.Set(b.Min.X+t, y, col)
			img.Set(b.Max.X-1-t, y, col)
		}
	}
}

func wrapText(text string, maxChars int) []string {
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

// renderPackImage renders 3 cards side by side
func renderPackImage(cards []CardData) ([]byte, error) {
	if len(cards) == 0 {
		return nil, fmt.Errorf("no cards")
	}

	cardWidth, cardHeight := 400, 520
	padding := 10
	totalWidth := len(cards)*cardWidth + (len(cards)-1)*padding
	img := image.NewRGBA(image.Rect(0, 0, totalWidth, cardHeight))

	// Dark background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{20, 20, 25, 255}}, image.Point{}, draw.Src)

	for i, card := range cards {
		cardBytes, err := renderCardImage(card)
		if err != nil {
			continue
		}
		cardImg, err := png.Decode(bytes.NewReader(cardBytes))
		if err != nil {
			continue
		}
		x := i * (cardWidth + padding)
		draw.Draw(img, image.Rect(x, 0, x+cardWidth, cardHeight), cardImg, image.Point{}, draw.Over)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
