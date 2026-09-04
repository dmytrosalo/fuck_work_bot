package poker

import (
	"math/bits"

	pk "github.com/chehsunliu/poker"
)

// handClassUA maps the evaluator's rank class (1 = strongest) to its
// Ukrainian name, matching the rest of the bot's user-facing text.
var handClassUA = map[int32]string{
	1: "стрит-флеш",
	2: "каре",
	3: "фул-хаус",
	4: "флеш",
	5: "стрит",
	6: "трійка",
	7: "дві пари",
	8: "пара",
	9: "старша карта",
}

// HandName returns the Ukrainian name of the best five-card hand available
// from hole+board, together with the exact five cards that form it so the
// client can highlight them.
//
// The evaluator only accepts 5, 6 or 7 cards and reports a rank, not which
// cards produced it, so the winning five are found by trying every
// five-card subset — at most 21 of them for a full seven — and keeping the
// strongest. Lower ranks are stronger, as in bestOf.
//
// Preflop there is no five-card hand to evaluate, so the only thing worth
// naming is a pocket pair; anything else returns empty and the client shows
// nothing rather than something misleading.
func HandName(hole, board []pk.Card) (string, []pk.Card) {
	all := make([]pk.Card, 0, len(hole)+len(board))
	all = append(all, hole...)
	all = append(all, board...)

	if len(all) < 5 {
		if len(hole) == 2 && hole[0].Rank() == hole[1].Rank() {
			return handClassUA[8], []pk.Card{hole[0], hole[1]}
		}
		return "", nil
	}

	bestRank := int32(-1)
	var bestSet []pk.Card
	n := len(all)
	for mask := 0; mask < 1<<n; mask++ {
		if bits.OnesCount(uint(mask)) != 5 {
			continue
		}
		set := make([]pk.Card, 0, 5)
		for i := 0; i < n; i++ {
			if mask&(1<<i) != 0 {
				set = append(set, all[i])
			}
		}
		if r := pk.Evaluate(set); bestRank == -1 || r < bestRank {
			bestRank, bestSet = r, set
		}
	}
	if bestRank == -1 {
		return "", nil
	}
	return handClassUA[pk.RankClass(bestRank)], bestSet
}
