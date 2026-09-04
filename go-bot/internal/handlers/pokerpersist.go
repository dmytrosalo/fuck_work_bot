package handlers

import (
	"encoding/json"
	"log"
	"time"

	"github.com/dmytrosalo/fuck-work-bot/internal/poker"
)

// snapshotMaxAge bounds how stale a stored table may be before it is
// discarded rather than restored. A table abandoned days ago should not
// reappear with everyone's old stacks; the sweeper's own idle reclaim is
// shorter, so in practice this only catches tables that were live when the
// process died.
const snapshotMaxAge = 2 * time.Hour

// persistTable writes one table's snapshot. Caller must hold the table lock
// and not h.mu — Snapshot reads the seats, and the DB write happens with the
// table lock held, the same shape settle already uses.
func (h *PokerHub) persistTable(tbl *poker.Table) {
	if h.db == nil {
		return
	}
	raw, err := json.Marshal(tbl.Snapshot())
	if err != nil {
		return
	}
	if err := h.db.SavePokerTable(tbl.ID, tbl.ChatID, string(raw)); err != nil {
		log.Printf("[poker] snapshot save failed for table %s: %v", tbl.ID, err)
	}
}

// RestoreTables rebuilds tables persisted by a previous process. Called once
// at startup, before the bot starts serving, so a player reopening the app
// finds their seat and chips where they left them instead of a dead link.
//
// Every restored table starts at StageWaiting with any in-progress hand
// voided and its committed chips returned — see poker.RestoreTable. Nothing
// about balances changes here: settlement is per hand, so an unfinished hand
// never touched one.
func (h *PokerHub) RestoreTables() {
	if h.db == nil {
		return
	}
	h.db.PrunePokerTables(snapshotMaxAge)

	restored, seats := 0, 0
	for _, raw := range h.db.LoadPokerTables() {
		var snap poker.TableSnapshot
		if err := json.Unmarshal([]byte(raw), &snap); err != nil || snap.ID == "" {
			continue
		}
		tbl := poker.RestoreTable(snap)
		if len(tbl.Seats) == 0 {
			// Nobody left with chips: not worth resurrecting, and keeping it
			// would hold the chat's /poker to an empty table.
			h.db.DeletePokerTable(snap.ID)
			continue
		}

		h.mu.Lock()
		h.tables[tbl.ID] = tbl
		h.lastActivity[tbl.ID] = time.Now()
		// Restore each player's hub-wide claim too. Without this a returning
		// player is seated at a table the hub does not believe they are at,
		// and could take a second funded seat elsewhere.
		for _, s := range tbl.Seats {
			h.seatedAt[s.UserID] = tbl.ID
			seats++
		}
		h.mu.Unlock()
		restored++
	}
	if restored > 0 {
		log.Printf("[poker] restored %d table(s) with %d seated player(s)", restored, seats)
	}
}
