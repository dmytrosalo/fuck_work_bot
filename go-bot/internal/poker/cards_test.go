package poker

import (
	"testing"

	pk "github.com/chehsunliu/poker"
)

func cards(ss ...string) []pk.Card {
	out := make([]pk.Card, 0, len(ss))
	for _, s := range ss {
		out = append(out, pk.NewCard(s))
	}
	return out
}

func TestLowerRankIsStronger(t *testing.T) {
	royal := Best(cards("Ah", "Kh"), cards("Qh", "Jh", "Th", "2c", "3d"))
	pair := Best(cards("As", "Ad"), cards("7h", "9c", "2s", "4d", "6h"))
	if royal >= pair {
		t.Fatalf("expected royal flush rank (%d) to be numerically lower than pair (%d)", royal, pair)
	}
}

func TestWheelStraight(t *testing.T) {
	wheel := Best(cards("Ah", "2d"), cards("3c", "4s", "5h", "Kd", "Qc"))
	high := Best(cards("Kh", "Qd"), cards("3c", "4s", "5h", "8d", "9c"))
	if wheel >= high {
		t.Fatalf("A-2-3-4-5 (%d) should beat king-high (%d)", wheel, high)
	}
}

func TestNewShuffledDeckIsComplete(t *testing.T) {
	d := NewShuffledDeck()
	if len(d) != 52 {
		t.Fatalf("deck size = %d, want 52", len(d))
	}
	seen := map[pk.Card]bool{}
	for _, c := range d {
		if seen[c] {
			t.Fatalf("duplicate card %v in deck", c)
		}
		seen[c] = true
	}
}
