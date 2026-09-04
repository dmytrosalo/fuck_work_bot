package poker

import (
	"strings"
	"testing"
)

func TestHandNameIdentifiesCombinations(t *testing.T) {
	cases := []struct {
		name  string
		hole  []string
		board []string
		want  string
	}{
		{"pair on the board", []string{"Ah", "Kd"}, []string{"As", "7c", "2d"}, "пара"},
		{"two pair", []string{"Ah", "Kd"}, []string{"As", "Kc", "2d"}, "дві пари"},
		{"trips", []string{"Ah", "Ad"}, []string{"As", "Kc", "2d"}, "трійка"},
		{"straight", []string{"9h", "8d"}, []string{"7s", "6c", "5d"}, "стрит"},
		{"flush", []string{"Ah", "9h"}, []string{"7h", "3h", "2h"}, "флеш"},
		{"full house", []string{"Ah", "Ad"}, []string{"As", "Kc", "Kd"}, "фул-хаус"},
		{"quads", []string{"Ah", "Ad"}, []string{"As", "Ac", "Kd"}, "каре"},
		{"straight flush", []string{"9h", "8h"}, []string{"7h", "6h", "5h"}, "стрит-флеш"},
		{"nothing", []string{"Ah", "9d"}, []string{"7s", "3c", "2d"}, "старша карта"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, used := HandName(cards(c.hole...), cards(c.board...))
			if got != c.want {
				t.Errorf("HandName = %q, want %q", got, c.want)
			}
			// How MANY cards come back is per-hand-class and is covered by
			// TestHighlightsOnlyTheCombination; what matters here is that
			// whatever comes back is real and visible to this player.
			if c.want != "старша карта" && len(used) == 0 {
				t.Fatalf("no cards to highlight for a %s", c.want)
			}
			// Every highlighted card must be one the player can actually see.
			available := map[string]bool{}
			for _, s := range append(append([]string{}, c.hole...), c.board...) {
				available[s] = true
			}
			for _, u := range used {
				if !available[u.String()] {
					t.Errorf("highlighted %s, which is in neither hole nor board", u)
				}
			}
		})
	}
}

// Preflop there is no five-card hand, so only a pocket pair is worth naming.
func TestHandNamePreflop(t *testing.T) {
	got, used := HandName(cards("Kh", "Kd"), nil)
	if got != "пара" {
		t.Errorf("pocket pair = %q, want пара", got)
	}
	if len(used) != 2 {
		t.Errorf("highlighted %d cards, want the 2 hole cards", len(used))
	}

	if got, used := HandName(cards("Kh", "7d"), nil); got != "" || used != nil {
		t.Errorf("unpaired preflop = %q/%v, want nothing rather than something misleading", got, used)
	}
	if got, _ := HandName(nil, nil); got != "" {
		t.Errorf("no cards = %q, want empty", got)
	}
}

// The highlighted five must be the ones that actually make the hand: a flush
// must highlight five of a suit, not any five cards that happen to score.
func TestHandNameHighlightsTheRightFive(t *testing.T) {
	_, used := HandName(cards("Ah", "9h"), cards("7h", "3h", "2h", "Kd", "Qc"))
	if len(used) != 5 {
		t.Fatalf("got %d cards", len(used))
	}
	for _, c := range used {
		if !strings.HasSuffix(c.String(), "h") {
			t.Errorf("flush highlight includes %s, which is not a heart", c)
		}
	}
}

// TestHighlightsOnlyTheCombination pins the rule that highlighting is about
// the cards MAKING the hand, not the best five. A pair with three unrelated
// kickers must light up two cards, and high card must light up nothing —
// outlining every card on the table conveys no information at all.
func TestHighlightsOnlyTheCombination(t *testing.T) {
	cases := []struct {
		name      string
		hole      []string
		board     []string
		wantName  string
		wantCards int
		wantRanks []string // every highlighted card must have one of these ranks
	}{
		{"pair highlights two", []string{"9h", "Kd"}, []string{"9s", "7c", "2d"},
			"пара", 2, []string{"9"}},
		{"two pair highlights four", []string{"9h", "Kd"}, []string{"9s", "Kc", "2d"},
			"дві пари", 4, []string{"9", "K"}},
		{"trips highlights three", []string{"9h", "9d"}, []string{"9s", "Kc", "2d"},
			"трійка", 3, []string{"9"}},
		{"quads highlights four", []string{"9h", "9d"}, []string{"9s", "9c", "Kd"},
			"каре", 4, []string{"9"}},
		{"high card highlights nothing", []string{"Ah", "9d"}, []string{"7s", "3c", "2d"},
			"старша карта", 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			name, used := HandName(cards(c.hole...), cards(c.board...))
			if name != c.wantName {
				t.Fatalf("name = %q, want %q", name, c.wantName)
			}
			if len(used) != c.wantCards {
				t.Fatalf("highlighted %d cards (%v), want %d", len(used), used, c.wantCards)
			}
			for _, u := range used {
				r := u.String()[:len(u.String())-1]
				ok := false
				for _, want := range c.wantRanks {
					if r == want {
						ok = true
					}
				}
				if !ok {
					t.Errorf("highlighted %s, a kicker rather than part of the %s", u, c.wantName)
				}
			}
		})
	}
}

// Hands that genuinely use all five cards must still highlight all five.
func TestFiveCardHandsHighlightAllFive(t *testing.T) {
	for _, c := range []struct {
		name  string
		hole  []string
		board []string
	}{
		{"straight", []string{"9h", "8d"}, []string{"7s", "6c", "5d"}},
		{"flush", []string{"Ah", "9h"}, []string{"7h", "3h", "2h"}},
		{"full house", []string{"Ah", "Ad"}, []string{"As", "Kc", "Kd"}},
		{"straight flush", []string{"9h", "8h"}, []string{"7h", "6h", "5h"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, used := HandName(cards(c.hole...), cards(c.board...)); len(used) != 5 {
				t.Errorf("%s highlighted %d cards, want all 5", c.name, len(used))
			}
		})
	}
}
