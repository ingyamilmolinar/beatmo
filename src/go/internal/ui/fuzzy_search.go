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
//   - +1 per matched character
//   - +1 for exact case match
//   - +5 for consecutive matched characters
//   - +10 for match at start of word (after space/hyphen/underscore)
//   - +15 for match at start of string
//   - +penalty for longer targets (prefer specific matches)
func FuzzyMatch(query, target string) FuzzyResult {
	qRunes := []rune(strings.ToLower(query))
	tRunes := []rune(strings.ToLower(target))
	tOrig := []rune(target)
	qOrig := []rune(query)

	if len(qRunes) == 0 {
		return FuzzyResult{Score: 0}
	}
	if len(qRunes) > len(tRunes) {
		return FuzzyResult{Score: -1}
	}

	// Check for exact substring — highest quality match.
	if idx := strings.Index(strings.ToLower(target), strings.ToLower(query)); idx >= 0 {
		positions := make([]int, len(qRunes))
		runeIdx := 0
		byteIdx := 0
		for _, r := range target {
			if byteIdx >= idx && runeIdx-runeIndexAtByte(target, idx) < len(qRunes) {
				pi := runeIdx - runeIndexAtByte(target, idx)
				if pi >= 0 && pi < len(qRunes) {
					positions[pi] = runeIdx
				}
			}
			byteIdx += len(string(r))
			runeIdx++
		}
		// Recalculate with simple rune index math.
		startRune := runeIndexAtByte(target, idx)
		for i := range positions {
			positions[i] = startRune + i
		}
		score := scorePositions(qOrig, tOrig, tRunes, positions)
		score += 20 // Exact substring bonus.
		return FuzzyResult{Score: score, Positions: positions}
	}

	// Greedy forward pass: match each query char to the earliest target char.
	positions := make([]int, 0, len(qRunes))
	qi := 0
	for ti := 0; ti < len(tRunes) && qi < len(qRunes); ti++ {
		if tRunes[ti] == qRunes[qi] {
			positions = append(positions, ti)
			qi++
		}
	}
	if qi < len(qRunes) {
		return FuzzyResult{Score: -1}
	}

	score := scorePositions(qOrig, tOrig, tRunes, positions)
	return FuzzyResult{Score: score, Positions: positions}
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

// scorePositions computes the match score for given positions.
func scorePositions(qOrig, tOrig, tLower []rune, positions []int) int {
	score := 0
	for i, pos := range positions {
		score++ // Base point.
		// Exact case match.
		if i < len(qOrig) && pos < len(tOrig) && tOrig[pos] == qOrig[i] {
			score++
		}
		// Consecutive match.
		if i > 0 && pos == positions[i-1]+1 {
			score += 5
		}
		// Start of string.
		if pos == 0 {
			score += 15
		} else if tLower[pos-1] == ' ' || tLower[pos-1] == '-' || tLower[pos-1] == '_' {
			// Start of word.
			score += 10
		}
	}
	// Prefer shorter targets (more specific matches).
	lenPenalty := len(tLower) - len(positions)
	if lenPenalty > 20 {
		lenPenalty = 20
	}
	score -= lenPenalty / 2
	return score
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
