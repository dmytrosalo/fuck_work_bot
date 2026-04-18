# Collection Web Page

Web page for viewing a user's card collection, linked from the Telegram `/collection` command.

## URL & Server

- `GET /collection/:userID` — public, no auth
- HTTP server runs alongside Telegram bot in a goroutine on `:8080` (Fly.io already expects this port)
- HTML rendered server-side with Go `html/template` — no external dependencies
- App URL: `https://fuck-work-bot.fly.dev`

## `/collection` Telegram Command (simplified)

Replace the current card-list output with a compact summary + link:

```
🃏 Колекція {Name} ({unique}/{total})

⭐ 12 | ⭐⭐ 8 | ⭐⭐⭐ 5 | ⭐⭐⭐⭐ 2 | ⭐⭐⭐⭐⭐ 1

🌐 https://fuck-work-bot.fly.dev/collection/{userID}
```

## Web Page Structure

### Header
- Username
- Collection progress: `45/502 cards`
- Coin balance
- Rarity breakdown counts (same as Telegram summary)

### Card Grid
- Grouped by rarity sections, highest first (Ultra Legendary → Common)
- Each section has a rarity label header with count
- Empty rarity sections are hidden

### Card Display

**Compact (default):**
- Small tile: emoji (large), name, rarity-colored border + background
- Count badge (x3) in corner if >1
- ~3 cards per row on mobile, ~5 on desktop

**Expanded (on click):**
- Card expands inline (CSS transition)
- Shows full stats: ATK, DEF, Special name + value
- Description text
- Rarity stars
- Click again or click another card to collapse

### Styling
- Dark theme using existing color scheme from `cardimage.go`:
  - `rarityBg`: card background colors per rarity
  - `rarityAccent`: border/glow colors per rarity
- Rarity glow effect on card borders
- Mobile-first responsive design (Telegram in-app browser)
- Pure CSS animations for expand/collapse
- Minimal vanilla JS — only click handlers for card toggling
- No external CSS/JS frameworks

## Data Flow

1. HTTP request hits `/collection/:userID`
2. Handler calls `db.GetUserName(userID)` for the header
3. Handler calls `db.GetFullCollection(userID)` — returns all owned cards with full stats, grouped by rarity
4. Handler calls `db.GetBalance(userID, "")` for coin balance
5. Handler calls `db.GetCollectionStats(userID)` for unique/total counts
6. Go template renders HTML with embedded CSS/JS

## Files

### New
- `go-bot/internal/handlers/web.go` — HTTP handler + HTML template (single file, template as const string)

### Modified
- `go-bot/cmd/bot/main.go` — start HTTP server goroutine, pass `db` to web handler
- `go-bot/internal/handlers/cards.go` — simplify `handleCollection` to rarity counts + link
- `go-bot/internal/storage/sqlite.go` — add `GetFullCollection(userID)` and `GetUserName(userID)`

## Storage: New Methods

### `GetFullCollection(userID string) []FullCard`
```go
type FullCard struct {
    ID          int
    Name        string
    Rarity      int
    Category    string
    Emoji       string
    Description string
    ATK         int
    DEF         int
    SpecialName string
    Special     int
    Count       int
}
```
Query: `SELECT c.*, col.count FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? ORDER BY c.rarity DESC, c.name`

### `GetUserName(userID string) string`
Query: `SELECT name FROM balances WHERE user_id = ? LIMIT 1`

### `GetRarityCounts(userID string) map[int]int`
Query: `SELECT c.rarity, COUNT(*) FROM collection col JOIN cards c ON col.card_id = c.id WHERE col.user_id = ? GROUP BY c.rarity`
Used by both the Telegram command and the web page header.
