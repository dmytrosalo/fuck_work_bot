package poker

import "testing"

// maxDriveActions bounds driveToShowdown/checkOrCallToShowdown's loops.
// MaxSeats(6) seats, 4 streets, and a small margin for a raise or two per
// street comfortably fit inside this; a real bug that stalls the hand (e.g.
// ToAct stuck on a seat that can never legally act) fails the test loudly
// instead of hanging the whole suite.
const maxDriveActions = 200

// checkOrCallToShowdown drives tbl to StageShowdown using only
// call-if-behind / check-otherwise actions (no raises), asserting on any
// unexpected error instead of silently swallowing it — silently ignoring
// Act's error here would just spin the loop on a stage that can never
// change, hanging the test rather than failing it.
func checkOrCallToShowdown(t *testing.T, tbl *Table) {
	t.Helper()
	for i := 0; tbl.Stage != StageShowdown; i++ {
		if i >= maxDriveActions {
			t.Fatalf("hand did not reach showdown within %d actions (stuck at stage %v) — infinite loop guard tripped", maxDriveActions, tbl.Stage)
		}
		s := tbl.Seats[tbl.ToAct]
		var err error
		if s.Bet < tbl.highBet() {
			err = tbl.Act(s.UserID, ActCall, 0)
		} else {
			err = tbl.Act(s.UserID, ActCheck, 0)
		}
		if err != nil {
			t.Fatalf("Act(%s): unexpected error %v", s.UserID, err)
		}
	}
}

func TestShowdownDeltasSumToZero(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Danya", 5000)
	_ = tbl.Sit("u2", "Data", 5000)
	_ = tbl.Sit("u3", "Bo", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}
	// Drive to showdown: everyone calls, then checks down.
	checkOrCallToShowdown(t, tbl)
	deltas := tbl.Showdown()
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("settlement deltas sum to %d, want 0 — money was created or destroyed", sum)
	}
}

func TestShowdownShortStackCannotWinMoreThanPaidIn(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Short", MinBuyIn)
	_ = tbl.Sit("u2", "Big", 5000)
	_ = tbl.Sit("u3", "Also", 5000)
	_ = tbl.StartHand()

	// After StartHand: Seat 0 is Button (no blind), Seat 1 is SB (50), Seat 2 is BB (100).
	// Stacks: [1000, 4950, 4900], Committed: [0, 50, 100], startStack: [1000, 5000, 5000]
	// Manually set Committed to [100, 300, 300], and decrement stacks accordingly.
	tbl.Seats[0].Stack -= 100 // increase Committed from 0 to 100
	tbl.Seats[0].Committed = 100
	tbl.Seats[0].Folded = false
	tbl.Seats[0].AllIn = true

	tbl.Seats[1].Stack -= 250 // increase Committed from 50 to 300
	tbl.Seats[1].Committed = 300
	tbl.Seats[1].Folded = false

	tbl.Seats[2].Stack -= 200 // increase Committed from 100 to 300
	tbl.Seats[2].Committed = 300
	tbl.Seats[2].Folded = false

	// Give the short stack the winning hand.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Ah", "As")
	tbl.Seats[1].Hole = cards("Kd", "Qd")
	tbl.Seats[2].Hole = cards("3c", "5h")

	deltas := tbl.Showdown()
	// With correct awarding:
	// Main pot (300): u1 (AA) wins all → delta +200
	// Side pot (400): u2 (KQ) wins → delta +100
	// u3 loses → delta -300
	if deltas["u1"] != 200 {
		t.Errorf("u1 delta = %d, want 200", deltas["u1"])
	}
	if deltas["u2"] != 100 {
		t.Errorf("u2 delta = %d, want 100", deltas["u2"])
	}
	if deltas["u3"] != -300 {
		t.Errorf("u3 delta = %d, want -300", deltas["u3"])
	}
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("deltas sum to %d, want 0", sum)
	}
}

func TestShowdownSplitPotOddChip(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	_ = tbl.Sit("u3", "C", 5000)
	_ = tbl.StartHand()

	// After StartHand: Seat 0 is Button, Seat 1 is SB (50), Seat 2 is BB (100).
	// Stacks: [5000, 4950, 4900], Committed: [0, 50, 100], startStack: [5000, 5000, 5000]
	// Manually set to create odd chip from dead chips (folded contributor).
	// Seats A(0) and B(1): Committed 75 each, live, tied winning hand
	// Seat C(2): Committed 1, FOLDED (dead chips)
	// This creates pots at levels [1, 75]:
	// - Level 1: 3 chips (1 from each), eligible [A,B] → split 1 each, remainder 1
	// - Level 75: 148 chips (74 from A, 74 from B), eligible [A,B] → split 74 each

	tbl.Seats[0].Stack -= 75 // increase Committed from 0 to 75
	tbl.Seats[0].Committed = 75
	tbl.Seats[0].Folded = false
	tbl.Seats[0].AllIn = false

	tbl.Seats[1].Stack -= 25 // increase Committed from 50 to 75
	tbl.Seats[1].Committed = 75
	tbl.Seats[1].Folded = false
	tbl.Seats[1].AllIn = false

	tbl.Seats[2].Stack += 99 // decrease Committed from 100 to 1
	tbl.Seats[2].Committed = 1
	tbl.Seats[2].Folded = true // FOLDED: dead chips
	tbl.Seats[2].AllIn = false

	// Identical hands for A and B: the board plays. C is folded, chips are dead.
	tbl.Board = cards("Ah", "Kh", "Qh", "Jh", "Th")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")
	tbl.Seats[2].Hole = cards("6h", "7h") // not evaluated (folded)

	// Set Button explicitly so we can verify odd chip goes to first left-of-button winner
	tbl.Button = 0 // A is button

	deltas := tbl.Showdown()
	// With 3-chip pot, 2 equal winners: 1 chip each + 1 remainder
	// Remainder goes to first winner left of button (button=0, so (0+1)%3=1 → B)
	// Stacks: A=4925+1+74=5000 (delta 0), B=4925+2+74=5001 (delta 1), C=4999+0=4999 (delta -1)
	if deltas["u1"] != 0 {
		t.Errorf("u1 (A, first seat) delta = %d, want 0 (shouldn't get odd chip)", deltas["u1"])
	}
	if deltas["u2"] != 1 {
		t.Errorf("u2 (B, second seat) delta = %d, want 1 (should get odd chip)", deltas["u2"])
	}
	if deltas["u3"] != -1 {
		t.Errorf("u3 (C, folded) delta = %d, want -1", deltas["u3"])
	}
	sum := deltas["u1"] + deltas["u2"] + deltas["u3"]
	if sum != 0 {
		t.Fatalf("split pot deltas sum to %d, want 0 (odd chip must not vanish)", sum)
	}
}

func TestShowdownIdempotent(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	// Drive to showdown
	checkOrCallToShowdown(t, tbl)

	deltas1 := tbl.Showdown()
	stacks1 := make([]int, len(tbl.Seats))
	for i, s := range tbl.Seats {
		stacks1[i] = s.Stack
	}
	if deltas1 == nil {
		t.Fatalf("first Showdown() returned nil, expected deltas")
	}

	// Second call should return nil and not change stacks
	deltas2 := tbl.Showdown()
	if deltas2 != nil {
		t.Fatalf("second Showdown() returned %v, want nil", deltas2)
	}

	stacks2 := make([]int, len(tbl.Seats))
	for i, s := range tbl.Seats {
		stacks2[i] = s.Stack
	}

	for i := range tbl.Seats {
		if stacks1[i] != stacks2[i] {
			t.Fatalf("stacks changed after second Showdown call: seat %d went from %d to %d",
				i, stacks1[i], stacks2[i])
		}
	}
}

func TestShowdownShortBoard(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "A", 5000)
	_ = tbl.Sit("u2", "B", 5000)
	_ = tbl.StartHand()

	// Set board to only 2 cards (preflop-like state with 2 players all-in).
	tbl.Board = cards("Ah", "Kh")
	tbl.Seats[0].Hole = cards("2c", "3c")
	tbl.Seats[1].Hole = cards("4d", "5d")

	// Set Committed and decrement stacks.
	tbl.Seats[0].Stack = 4900
	tbl.Seats[0].Committed = 100
	tbl.Seats[0].AllIn = true

	tbl.Seats[1].Stack = 4900
	tbl.Seats[1].Committed = 100
	tbl.Seats[1].AllIn = true

	deltas := tbl.Showdown()
	// With short board, candidates should split the pot equally.
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("short board deltas sum to %d, want 0", sum)
	}
	// With 200 total and 2 equal splitters: each should get 100, delta = 0.
	if deltas["u1"] != 0 || deltas["u2"] != 0 {
		t.Fatalf("short board split: got u1=%d, u2=%d, want both 0", deltas["u1"], deltas["u2"])
	}
}

// --- FIX 6: end-to-end zero-sum over a REAL, played side-pot hand ----------
//
// Everything above either drives a hand with no raises/all-ins
// (TestShowdownDeltasSumToZero/TestShowdownIdempotent) or hand-builds a pot
// shape directly on Seats, bypassing Act entirely (the short-stack/split-pot
// fixtures above). Neither exercises BuildPots fed by a genuine sequence of
// real Act() calls — raises, calls, and a short all-in — actually
// constructing the side pot. The tests below close that gap: StartHand →
// real Act() betting including at least one short all-in → Showdown, for
// several varied table sizes and all-in timings.
//
// NOTE on the "wrong winner" mutation named in the review's verification
// step: a plain sum(deltas)==0 assertion, by itself, mathematically CANNOT
// catch a bug that awards a pot to the wrong (but still in-hand) seat while
// keeping the total distributed unchanged — swapping which seat receives
// money conserves the total either way, so the sum stays zero regardless of
// who wins. TestShowdownRealPipelineSidePotZeroSum below satisfies FIX 6's
// literal text (sum to zero, real pipeline, several scenarios) with random
// cards. TestShowdownRealPipelineSidePotExactDeltas and its 4-handed
// double-side-pot sibling go further, deterministically overriding
// Board/Hole right before the Showdown() call (same technique the
// hand-built fixtures above use), so the winner is pinned and exact
// per-seat deltas can be asserted — THAT assertion is what actually catches
// a winner-swap mutation; sum-to-zero alone does not. See VERIFICATION in
// the fix report for the mutation actually run against this.

// TestShowdownRealPipelineSidePotZeroSum plays a complete 3-handed hand
// through the real pipeline — a preflop raise, a call, and a short stack's
// all-in call for less than the full bet (the exact shape that produces a
// genuine side pot via BuildPots) — then checks down every street to a real
// (randomly dealt) showdown, and asserts the settlement deltas sum to
// exactly zero.
func TestShowdownRealPipelineSidePotZeroSum(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 5000)
	_ = tbl.Sit("u2", "Bob", 5000)
	_ = tbl.Sit("u3", "Carol", MinBuyIn) // short stack: forces a genuine side pot
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	act := func(userID string, a Action, amount int) {
		t.Helper()
		if err := tbl.Act(userID, a, amount); err != nil {
			t.Fatalf("Act(%s, %v, %d): unexpected error %v", userID, a, amount, err)
		}
	}

	// First to act (button, 3-handed) raises big enough that the short
	// stack cannot fully call.
	first := tbl.Seats[tbl.ToAct].UserID
	act(first, ActRaise, 1200)

	// Drive the rest of preflop: whoever's next either calls (going all-in
	// automatically if it's more than their stack — see Table.post) since
	// nobody folds in this scenario.
	for tbl.Stage == StagePreflop {
		s := tbl.Seats[tbl.ToAct]
		act(s.UserID, ActCall, 0)
	}
	if !tbl.Seats[2].AllIn {
		t.Fatalf("Carol (short stack) not all-in after preflop — test setup didn't produce a side pot")
	}

	// Postflop: the two non-all-in players just check every street down to
	// showdown.
	checkOrCallToShowdown(t, tbl)

	deltas := tbl.Showdown()
	sum := 0
	for _, d := range deltas {
		sum += d
	}
	if sum != 0 {
		t.Fatalf("real-pipeline side-pot hand: deltas sum to %d, want 0 — money was created or destroyed", sum)
	}
}

// TestShowdownRealPipelineSidePotExactDeltas is the discriminating sibling
// of the test above: same real preflop betting (raise, call, short all-in
// producing a genuine side pot), but Board/Hole are deterministically
// overridden right before the Showdown() call — same technique the
// hand-built pot fixtures elsewhere in this file already use — pinning the
// winner. u2 (deliberately NOT seat index 0 — see below) is given both the
// strongest hand AND the highest final commitment (tied with u1), making u2
// eligible for and the sole winner of every pot (main + side), so u2's
// expected delta is simply the WHOLE table total minus u2's own
// commitment, and every other seat's expected delta is just the negative
// of what they put in. This exact-value assertion is what actually catches
// a "wrong winner, same total distributed" mutation — a bare sum-to-zero
// check cannot (see the file-level NOTE above). The winner is deliberately
// NOT seat index 0: a mutation of the shape "always award to
// candidates[0]" would coincidentally still pass a test whose true winner
// happens to sit at index 0, since pot.Eligible lists candidates in
// ascending seat order — this is exactly what made an earlier draft of
// this test fail to catch that mutation.
func TestShowdownRealPipelineSidePotExactDeltas(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 5000)
	_ = tbl.Sit("u2", "Bob", 5000)
	_ = tbl.Sit("u3", "Carol", MinBuyIn)
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	act := func(userID string, a Action, amount int) {
		t.Helper()
		if err := tbl.Act(userID, a, amount); err != nil {
			t.Fatalf("Act(%s, %v, %d): unexpected error %v", userID, a, amount, err)
		}
	}

	first := tbl.Seats[tbl.ToAct].UserID
	act(first, ActRaise, 1200)
	for tbl.Stage == StagePreflop {
		s := tbl.Seats[tbl.ToAct]
		act(s.UserID, ActCall, 0)
	}
	if !tbl.Seats[2].AllIn {
		t.Fatalf("Carol (short stack) not all-in after preflop — test setup didn't produce a side pot")
	}
	checkOrCallToShowdown(t, tbl)

	// Capture each seat's real, genuinely-bet commitment before Showdown()
	// zeroes it, and confirm the intended shape actually happened: u1/u2
	// tied at the table's highest commitment (both eligible for every
	// pot), u3 strictly lower (excluded from the top-level side pot —
	// that's the side pot).
	committed := make([]int, len(tbl.Seats))
	total := 0
	for i, s := range tbl.Seats {
		committed[i] = s.Committed
		total += s.Committed
	}
	if committed[0] != committed[1] {
		t.Fatalf("u1/u2 committed = %d/%d, want equal (both at the table's highest commitment)", committed[0], committed[1])
	}
	if committed[2] >= committed[0] {
		t.Fatalf("u3 committed = %d, want strictly less than u1/u2's %d (no side pot would exist otherwise)", committed[2], committed[0])
	}

	// Deterministically make u2 (NOT seat index 0 — see the doc comment
	// above) the best hand at a dry, unpaired board — same pattern
	// TestShowdownShortStackCannotWinMoreThanPaidIn uses.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Kd", "Qd") // Alice: ace-high at best
	tbl.Seats[1].Hole = cards("Ah", "As") // Bob: overpair, strongest
	tbl.Seats[2].Hole = cards("3c", "5h") // Carol: weakest

	deltas := tbl.Showdown()

	wantU1 := -committed[0]
	wantU2 := total - committed[1]
	wantU3 := -committed[2]
	if deltas["u1"] != wantU1 {
		t.Errorf("u1 delta = %d, want %d (won nothing, loses exactly what was committed)", deltas["u1"], wantU1)
	}
	if deltas["u2"] != wantU2 {
		t.Errorf("u2 delta = %d, want %d (sole winner of every pot)", deltas["u2"], wantU2)
	}
	if deltas["u3"] != wantU3 {
		t.Errorf("u3 delta = %d, want %d (short stack, won nothing)", deltas["u3"], wantU3)
	}
	sum := deltas["u1"] + deltas["u2"] + deltas["u3"]
	if sum != 0 {
		t.Fatalf("deltas sum to %d, want 0", sum)
	}
}

// TestShowdownRealPipelineDoubleSidePotExactDeltas varies the scenario
// further: 4-handed, everyone limps preflop, then a big flop bet forces a
// DIFFERENT short stack all-in on a LATER street, producing two side-pot
// levels (main pot open to all 4, one side pot open to the 3 bigger
// stacks). Same deterministic-card + exact-delta technique as the sibling
// test above.
func TestShowdownRealPipelineDoubleSidePotExactDeltas(t *testing.T) {
	tbl := NewTable("t1", 1)
	_ = tbl.Sit("u1", "Alice", 5000)
	_ = tbl.Sit("u2", "Bob", 5000)
	_ = tbl.Sit("u3", "Carol", 5000)
	_ = tbl.Sit("u4", "Dana", MinBuyIn) // short stack: goes all-in on the flop
	if err := tbl.StartHand(); err != nil {
		t.Fatalf("StartHand: %v", err)
	}

	act := func(userID string, a Action, amount int) {
		t.Helper()
		if err := tbl.Act(userID, a, amount); err != nil {
			t.Fatalf("Act(%s, %v, %d): unexpected error %v", userID, a, amount, err)
		}
	}

	// Preflop: everyone calls the big blind (limps) so nobody is forced
	// all-in yet — the side pot gets created on the flop instead, varying
	// the timing from the 3-handed preflop-all-in scenario above.
	for tbl.Stage == StagePreflop {
		s := tbl.Seats[tbl.ToAct]
		if s.Bet < tbl.highBet() {
			act(s.UserID, ActCall, 0)
		} else {
			act(s.UserID, ActCheck, 0)
		}
	}

	// Flop: a big bet and calls force Dana (short stack) all-in for less
	// than the full bet.
	betterOnFlop := tbl.Seats[tbl.ToAct].UserID
	act(betterOnFlop, ActRaise, 1300) // Bet on a fresh street is still ActRaise (there's no open bet yet)
	for tbl.Stage == StageFlop {
		s := tbl.Seats[tbl.ToAct]
		act(s.UserID, ActCall, 0)
	}
	if !tbl.Seats[3].AllIn {
		t.Fatalf("Dana (short stack) not all-in after the flop — test setup didn't produce a side pot")
	}

	// Turn and river: the three remaining non-all-in players check down.
	checkOrCallToShowdown(t, tbl)

	committed := make([]int, len(tbl.Seats))
	total := 0
	for i, s := range tbl.Seats {
		committed[i] = s.Committed
		total += s.Committed
	}
	// The three big stacks should have matched each other above Dana's
	// commitment — that gap is the side pot this test exists to exercise.
	if committed[0] != committed[1] || committed[1] != committed[2] {
		t.Fatalf("u1/u2/u3 committed = %d/%d/%d, want all equal (tied at the table's highest commitment)", committed[0], committed[1], committed[2])
	}
	if committed[3] >= committed[0] {
		t.Fatalf("u4 committed = %d, want strictly less than the others' %d (no side pot would exist otherwise)", committed[3], committed[0])
	}

	// Give Carol (u3) the best hand: highest commitment (tied) → eligible
	// for both pot levels, and the strongest hand → wins both outright.
	tbl.Board = cards("2c", "7d", "9h", "Jc", "4s")
	tbl.Seats[0].Hole = cards("Kd", "Qd") // Alice: ace-high at best
	tbl.Seats[1].Hole = cards("3c", "5h") // Bob: weak
	tbl.Seats[2].Hole = cards("Ah", "As") // Carol: overpair, strongest
	tbl.Seats[3].Hole = cards("6h", "8h") // Dana: weak

	deltas := tbl.Showdown()

	wantU3 := total - committed[2]
	wantU1 := -committed[0]
	wantU2 := -committed[1]
	wantU4 := -committed[3]
	if deltas["u3"] != wantU3 {
		t.Errorf("u3 (Carol) delta = %d, want %d (sole winner of every pot)", deltas["u3"], wantU3)
	}
	if deltas["u1"] != wantU1 {
		t.Errorf("u1 (Alice) delta = %d, want %d", deltas["u1"], wantU1)
	}
	if deltas["u2"] != wantU2 {
		t.Errorf("u2 (Bob) delta = %d, want %d", deltas["u2"], wantU2)
	}
	if deltas["u4"] != wantU4 {
		t.Errorf("u4 (Dana) delta = %d, want %d (short stack, won nothing)", deltas["u4"], wantU4)
	}
	sum := deltas["u1"] + deltas["u2"] + deltas["u3"] + deltas["u4"]
	if sum != 0 {
		t.Fatalf("deltas sum to %d, want 0", sum)
	}
}
