package ui

import (
	"sort"
	"strings"
)

// FuzzyResult holds the result of a fuzzy match against a single string.
type FuzzyResult struct {
	Score     int   // Higher is better; -1 means no match.
	Positions []int // Rune indices in target that matched query chars.
}

// FuzzyMatch scores how well query matches target using subsequence matching.
// Characters in query must appear in target in order, but not necessarily
// contiguously. Returns Score=-1 when query is not a subsequence of target.
//
// Scoring:
//   - +1 per matched character (base)
//   - +1 for exact case match
//   - +5 for consecutive matched characters (gap == 0)
//   - −min(gap, 10) for non-consecutive matches (linear gap penalty, capped)
//   - +10 for match at start of word (after space/hyphen/underscore)
//   - +15 for match at start of string
//   - +20 bonus when query appears as an exact (case-insensitive) substring
//   - per-target length penalty (prefer specific matches)
//
// Implementation: a dynamic-programming search over all valid position
// assignments. Greedy forward matching (the previous implementation) could
// pick a scattered set of positions when a strictly higher-scoring
// contiguous tail existed later in the target — DP rules that out. The
// exact-substring fast path stays because it's both faster and produces
// the same answer as DP for the contiguous case.
//
// Complexity: O(|query| × |target|²). Instrument names are short (<30
// runes) so this is negligible vs. the previous greedy O(|target|).
func FuzzyMatch(query, target string) FuzzyResult {
	qLowerStr := strings.ToLower(query)
	tLowerStr := strings.ToLower(target)
	qLower := []rune(qLowerStr)
	tLower := []rune(tLowerStr)
	tOrig := []rune(target)
	qOrig := []rune(query)

	if len(qLower) == 0 {
		return FuzzyResult{Score: 0}
	}
	if len(qLower) > len(tLower) {
		return FuzzyResult{Score: -1}
	}

	// Exact substring fast path. When the query appears verbatim (case-
	// insensitively) in the target, the contiguous run is the global
	// optimum: every transition is a +5 consecutive bonus. We still apply
	// the per-position scoring + the +20 exact-substring bonus + length
	// penalty so the score is comparable to the DP's output for non-
	// contiguous cases.
	if idx := strings.Index(tLowerStr, qLowerStr); idx >= 0 {
		startRune := runeIndexAtByte(target, idx)
		positions := make([]int, len(qLower))
		for i := range positions {
			positions[i] = startRune + i
		}
		score := scoreContiguous(qOrig, tOrig, tLower, positions)
		score += 20 // exact-substring bonus
		score -= targetLengthPenalty(len(tLower), len(qLower))
		return FuzzyResult{Score: score, Positions: positions}
	}

	res := dpFuzzySubsequence(qOrig, qLower, tOrig, tLower)
	if res.Score < 0 {
		return res
	}
	res.Score -= targetLengthPenalty(len(tLower), len(qLower))
	return res
}

// dpFuzzySubsequence is the DP core. Returns Score=-1 when query is not a
// subsequence of target.
func dpFuzzySubsequence(qOrig, qLower, tOrig, tLower []rune) FuzzyResult {
	qn := len(qLower)
	tn := len(tLower)
	const negInf = -1 << 30

	// dp[i][j] = best score for matching query[0..=i] ending at target[j].
	// prev[i][j] = the j' from which we transitioned. Storing the full
	// table makes traceback O(qn) without recomputation.
	dp := make([][]int, qn)
	prev := make([][]int, qn)
	for i := range dp {
		dp[i] = make([]int, tn)
		prev[i] = make([]int, tn)
		for j := range dp[i] {
			dp[i][j] = negInf
			prev[i][j] = -1
		}
	}

	// Base row: query[0] can land anywhere it matches.
	for j := 0; j < tn; j++ {
		if tLower[j] != qLower[0] {
			continue
		}
		dp[0][j] = perCharScore(qOrig, tOrig, tLower, 0, j)
	}

	// Fill remaining rows. j must be >= i so there's room for i prior matches.
	for i := 1; i < qn; i++ {
		for j := i; j < tn; j++ {
			if tLower[j] != qLower[i] {
				continue
			}
			best := negInf
			bestK := -1
			for k := i - 1; k < j; k++ {
				if dp[i-1][k] == negInf {
					continue
				}
				gap := j - k - 1
				var bonus int
				if gap == 0 {
					bonus = 5
				} else {
					capped := gap
					if capped > 10 {
						capped = 10
					}
					bonus = -capped
				}
				cand := dp[i-1][k] + perCharScore(qOrig, tOrig, tLower, i, j) + bonus
				if cand > best {
					best = cand
					bestK = k
				}
			}
			if bestK >= 0 {
				dp[i][j] = best
				prev[i][j] = bestK
			}
		}
	}

	// Find best ending position.
	bestScore := negInf
	bestEnd := -1
	for j := qn - 1; j < tn; j++ {
		if dp[qn-1][j] > bestScore {
			bestScore = dp[qn-1][j]
			bestEnd = j
		}
	}
	if bestEnd < 0 || bestScore == negInf {
		return FuzzyResult{Score: -1}
	}

	// Traceback.
	positions := make([]int, qn)
	cur := bestEnd
	for i := qn - 1; i >= 0; i-- {
		positions[i] = cur
		cur = prev[i][cur]
	}

	return FuzzyResult{Score: bestScore, Positions: positions}
}

// perCharScore returns the per-position bonuses at target[ti] when matching
// query[qi]: +1 base, +1 case match, +15 start-of-string OR +10 word boundary.
// Mirrors the bonuses the previous scorePositions applied per position.
func perCharScore(qOrig, tOrig, tLower []rune, qi, ti int) int {
	score := 1 // base
	if ti < len(tOrig) && qi < len(qOrig) && tOrig[ti] == qOrig[qi] {
		score++ // exact case match
	}
	if ti == 0 {
		score += 15
		return score
	}
	if isWordBoundary(tLower[ti-1]) {
		score += 10
	}
	return score
}

// isWordBoundary returns true for the separators that mark the start of a
// new word for scoring purposes.
func isWordBoundary(r rune) bool {
	return r == ' ' || r == '-' || r == '_'
}

// scoreContiguous scores a fully-consecutive run of positions (from the
// exact-substring fast path). Equivalent to the DP path's scoring for an
// all-gap-0 walk: per-position bonuses + +5 consecutive bonus per
// transition.
func scoreContiguous(qOrig, tOrig, tLower []rune, positions []int) int {
	score := 0
	for i, pos := range positions {
		score += perCharScore(qOrig, tOrig, tLower, i, pos)
		if i > 0 {
			score += 5 // consecutive
		}
	}
	return score
}

// targetLengthPenalty discounts longer targets to prefer specific matches.
// Halved and capped at 10 (== 20 raw / 2) so the penalty tops out before
// it can outweigh strong per-position bonuses.
func targetLengthPenalty(targetLen, queryLen int) int {
	diff := targetLen - queryLen
	if diff > 20 {
		diff = 20
	}
	return diff / 2
}

// runeIndexAtByte returns the rune index corresponding to byte offset b in s.
func runeIndexAtByte(s string, b int) int {
	ri := 0
	bi := 0
	for _, r := range s {
		if bi >= b {
			return ri
		}
		bi += len(string(r))
		ri++
	}
	return ri
}

// MenuSearchItem represents a searchable item in a menu list.
type MenuSearchItem struct {
	Key   string // Unique identifier (e.g., instrument ID).
	Label string // Display text to search and highlight against.
}

// MenuSearchResult contains a matched item with scoring and highlight info.
type MenuSearchResult struct {
	Key        string
	Label      string
	Score      int
	Highlights []int // Rune indices in Label that matched the query.
}

// MenuSearcher provides generic fuzzy search filtering for menu lists.
// It searches both key and label, preferring label matches since those
// produce visible highlights.
type MenuSearcher struct{}

// Search filters items by fuzzy-matching query against each item's label
// and key. Returns matched items sorted by score (best first). When query
// is empty, all items are returned with no highlights.
func (s *MenuSearcher) Search(query string, items []MenuSearchItem) []MenuSearchResult {
	q := strings.TrimSpace(query)
	if q == "" {
		results := make([]MenuSearchResult, len(items))
		for i, item := range items {
			results[i] = MenuSearchResult{Key: item.Key, Label: item.Label}
		}
		return results
	}

	var results []MenuSearchResult
	for _, item := range items {
		// Try label first (produces visible highlights).
		lr := FuzzyMatch(q, item.Label)
		// Try key as fallback (no highlights since key isn't displayed).
		kr := FuzzyMatch(q, item.Key)

		if lr.Score < 0 && kr.Score < 0 {
			continue // No match.
		}

		res := MenuSearchResult{Key: item.Key, Label: item.Label}
		// Prefer label match when it succeeds — it produces highlights.
		// Fall back to key match only when label doesn't match at all.
		if lr.Score >= 0 {
			res.Score = lr.Score
			res.Highlights = lr.Positions
		} else {
			res.Score = kr.Score
		}
		results = append(results, res)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}
