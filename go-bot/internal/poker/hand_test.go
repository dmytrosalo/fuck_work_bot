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
			if len(used) != 5 {
				t.Fatalf("returned %d cards, want exactly 5 to highlight", len(used))
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
