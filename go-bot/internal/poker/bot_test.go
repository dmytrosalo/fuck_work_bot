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
	// Sanity band: premium holdings should be well above 0.8
	if aces < 0.8 {
		t.Errorf("AA preflop=%.2f, expected > 0.8", aces)
	}
	// Sanity band: trash should be well below 0.2
	if trash > 0.2 {
		t.Errorf("72o preflop=%.2f, expected < 0.2", trash)
	}
}

func TestHandStrengthPostflopOrdersHands(t *testing.T) {
	// Hands that DIVERGE between preflop and postflop:
	// A-6 offsuit is weak preflop (~0.22) but makes a wheel straight on 2-3-4-5 board
	// K-K is strong preflop (~0.85) but only has a pair on the wheel board
	// The postflop evaluator must run for wheel to beat KK; preflop heuristic alone
	// would order them wrong (KK > A6).
	board := cards("2h", "3h", "4h", "5h", "9d")
	wheel := handStrength(cards("Ah", "6d"), board)     // makes A-2-3-4-5 straight
	pairKings := handStrength(cards("Kc", "Kd"), board) // just has pair of Kings

	if wheel <= pairKings {
		t.Fatalf("postflop ordering wrong: wheel=%.2f KK=%.2f (wheel must beat pair)", wheel, pairKings)
	}
	if wheel > 1.0 || pairKings < 0.0 {
		t.Errorf("strength out of range: wheel=%.2f KK=%.2f", wheel, pairKings)
	}
	// Sanity band: wheel (straight) should be premium, well above 0.8
	if wheel < 0.8 {
		t.Errorf("wheel straight=%.2f, expected > 0.8", wheel)
	}
	// Sanity band: pair of kings with no improvement should be moderate, below 0.6
	if pairKings > 0.6 {
		t.Errorf("KK on wheel board=%.2f, expected < 0.6", pairKings)
	}
}
