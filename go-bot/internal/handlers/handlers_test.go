package handlers

import "testing"

func TestRandomRoast(t *testing.T) {
	r1 := randomRoast()
	if r1 == "" {
		t.Fatal("expected non-empty roast")
	}
	// Verify variety
	seen := make(map[string]bool)
	for i := 0; i < 50; i++ {
		seen[randomRoast()] = true
	}
	if len(seen) < 3 {
		t.Fatalf("expected variety, got %d unique", len(seen))
	}
}
