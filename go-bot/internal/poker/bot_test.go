package poker

import "testing"

func TestHandStrengthPreflopOrdersHands(t *testing.T) {
	aces := handStrength(cards("Ah", "As"), nil)
	suitedConnector := handStrength(cards("8h", "9h"), nil)
	trash := handStrength(cards("2c", "7d"), nil)

	if !(aces > suitedConnector && suitedConnector > trash) {
		t.Fatalf("preflop ordering wrong: AA=%.2f 89s=%.2f 72o=%.2f", aces, suitedConnector, trash)
	}
	if aces > 1.0 || trash < 0.0 {
		t.Errorf("strength out of range: AA=%.2f 72o=%.2f", aces, trash)
	}
}

func TestHandStrengthPostflopOrdersHands(t *testing.T) {
	board := cards("Ah", "Kh", "7d", "2c", "9s")
	twoPair := handStrength(cards("Ad", "Kd"), board)
	highCard := handStrength(cards("3c", "4s"), board)

	if twoPair <= highCard {
		t.Fatalf("postflop ordering wrong: two pair=%.2f high card=%.2f", twoPair, highCard)
	}
	if twoPair > 1.0 || highCard < 0.0 {
		t.Errorf("strength out of range: %.2f %.2f", twoPair, highCard)
	}
}
