//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// ---------- FuzzyMatch tests ----------

func TestFuzzyMatch_EmptyQuery(t *testing.T) {
	r := FuzzyMatch("", "Kick Drum")
	if r.Score != 0 {
		t.Errorf("empty query should score 0, got %d", r.Score)
	}
	if r.Positions != nil {
		t.Errorf("empty query should have nil positions, got %v", r.Positions)
	}
}

func TestFuzzyMatch_ExactMatch(t *testing.T) {
	r := FuzzyMatch("Kick Drum", "Kick Drum")
	if r.Score < 0 {
		t.Fatal("exact match should have positive score")
	}
	if len(r.Positions) != 9 {
		t.Errorf("expected 9 positions for exact match, got %d", len(r.Positions))
	}
}

func TestFuzzyMatch_SubstringMatch(t *testing.T) {
	r := FuzzyMatch("kick", "Kick Drum")
	if r.Score < 0 {
		t.Fatal("substring match should have positive score")
	}
	// Should match positions 0,1,2,3.
	expected := []int{0, 1, 2, 3}
	if len(r.Positions) != len(expected) {
		t.Fatalf("expected %d positions, got %d: %v", len(expected), len(r.Positions), r.Positions)
	}
	for i, pos := range r.Positions {
		if pos != expected[i] {
			t.Errorf("position[%d] = %d, want %d", i, pos, expected[i])
		}
	}
}

func TestFuzzyMatch_CaseInsensitive(t *testing.T) {
	r := FuzzyMatch("KICK", "kick drum")
	if r.Score < 0 {
		t.Fatal("case-insensitive match should succeed")
	}
}

func TestFuzzyMatch_FuzzySubsequence(t *testing.T) {
	// "kd" should match "Kick Drum" (K...D)
	r := FuzzyMatch("kd", "Kick Drum")
	if r.Score < 0 {
		t.Fatal("fuzzy subsequence 'kd' should match 'Kick Drum'")
	}
	if len(r.Positions) != 2 {
		t.Fatalf("expected 2 positions, got %d: %v", len(r.Positions), r.Positions)
	}
}

func TestFuzzyMatch_FuzzyAbbreviation(t *testing.T) {
	// "sd" should match "Snare Drum" (S...D)
	r := FuzzyMatch("sd", "Snare Drum")
	if r.Score < 0 {
		t.Fatal("fuzzy abbreviation 'sd' should match 'Snare Drum'")
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	r := FuzzyMatch("xyz", "Kick Drum")
	if r.Score >= 0 {
		t.Errorf("'xyz' should not match 'Kick Drum', got score %d", r.Score)
	}
}

func TestFuzzyMatch_QueryLongerThanTarget(t *testing.T) {
	r := FuzzyMatch("very long query", "Hi")
	if r.Score >= 0 {
		t.Error("query longer than target should not match")
	}
}

func TestFuzzyMatch_SingleChar(t *testing.T) {
	r := FuzzyMatch("k", "Kick Drum")
	if r.Score < 0 {
		t.Fatal("single char 'k' should match 'Kick Drum'")
	}
	if len(r.Positions) != 1 || r.Positions[0] != 0 {
		t.Errorf("expected position [0], got %v", r.Positions)
	}
}

func TestFuzzyMatch_SubstringScoresHigherThanSubsequence(t *testing.T) {
	// "ick" is a substring of "Kick"; "ikd" is only a subsequence.
	rSub := FuzzyMatch("ick", "Kick Drum")
	rSeq := FuzzyMatch("ikd", "Kick Drum")
	if rSub.Score < 0 || rSeq.Score < 0 {
		t.Fatal("both should match")
	}
	if rSub.Score <= rSeq.Score {
		t.Errorf("substring 'ick' (score %d) should score higher than subsequence 'ikd' (score %d)",
			rSub.Score, rSeq.Score)
	}
}

func TestFuzzyMatch_WordBoundaryBonus(t *testing.T) {
	// "d" at word boundary (start of "Drum") should score higher than "r" (mid-word).
	rBound := FuzzyMatch("d", "Kick Drum")
	rMid := FuzzyMatch("r", "Kick Drum")
	if rBound.Score < 0 || rMid.Score < 0 {
		t.Fatal("both should match")
	}
	if rBound.Score <= rMid.Score {
		t.Errorf("word boundary 'd' (score %d) should score higher than mid-word 'r' (score %d)",
			rBound.Score, rMid.Score)
	}
}

func TestFuzzyMatch_StartOfStringBonus(t *testing.T) {
	// Matching at position 0 should score highest.
	rStart := FuzzyMatch("k", "Kick")
	rMid := FuzzyMatch("c", "Kick")
	if rStart.Score < 0 || rMid.Score < 0 {
		t.Fatal("both should match")
	}
	if rStart.Score <= rMid.Score {
		t.Errorf("start 'k' (score %d) should score higher than mid 'c' (score %d)",
			rStart.Score, rMid.Score)
	}
}

func TestFuzzyMatch_Positions(t *testing.T) {
	tests := []struct {
		query, target string
		wantPositions []int
	}{
		{"kd", "Kick Drum", []int{0, 5}},
		{"hi", "Hi-Hat", []int{0, 1}},
		{"hh", "Hi-Hat", []int{0, 3}},
	}
	for _, tt := range tests {
		r := FuzzyMatch(tt.query, tt.target)
		if r.Score < 0 {
			t.Errorf("FuzzyMatch(%q, %q): expected match, got no match", tt.query, tt.target)
			continue
		}
		if len(r.Positions) != len(tt.wantPositions) {
			t.Errorf("FuzzyMatch(%q, %q): positions %v, want %v",
				tt.query, tt.target, r.Positions, tt.wantPositions)
			continue
		}
		for i, pos := range r.Positions {
			if pos != tt.wantPositions[i] {
				t.Errorf("FuzzyMatch(%q, %q): position[%d] = %d, want %d",
					tt.query, tt.target, i, pos, tt.wantPositions[i])
			}
		}
	}
}

// ---------- MenuSearcher tests ----------

func testMenuItems() []MenuSearchItem {
	return []MenuSearchItem{
		{Key: "kick", Label: "Kick Drum"},
		{Key: "snare", Label: "Snare Drum"},
		{Key: "hihat", Label: "Hi-Hat"},
		{Key: "clap", Label: "Clap"},
		{Key: "tom_hi", Label: "High Tom"},
		{Key: "tom_lo", Label: "Low Tom"},
		{Key: "ride", Label: "Ride Cymbal"},
		{Key: "crash", Label: "Crash Cymbal"},
	}
}

func TestMenuSearcher_EmptyQuery(t *testing.T) {
	var s MenuSearcher
	results := s.Search("", testMenuItems())
	if len(results) != 8 {
		t.Errorf("empty query should return all %d items, got %d", 8, len(results))
	}
	for _, r := range results {
		if len(r.Highlights) > 0 {
			t.Errorf("empty query should produce no highlights for %q", r.Label)
		}
	}
}

func TestMenuSearcher_SubstringFilter(t *testing.T) {
	var s MenuSearcher
	results := s.Search("drum", testMenuItems())
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'drum', got %d", len(results))
	}
	for _, r := range results {
		if r.Key != "kick" && r.Key != "snare" {
			t.Errorf("unexpected result: %q", r.Key)
		}
	}
}

func TestMenuSearcher_FuzzyFilter(t *testing.T) {
	var s MenuSearcher
	// "kd" is a fuzzy match for "Kick Drum" (K...D)
	results := s.Search("kd", testMenuItems())
	found := false
	for _, r := range results {
		if r.Key == "kick" {
			found = true
			if len(r.Highlights) == 0 {
				t.Error("expected highlights for fuzzy match 'kd' on 'Kick Drum'")
			}
		}
	}
	if !found {
		t.Error("expected 'kick' in results for fuzzy query 'kd'")
	}
}

func TestMenuSearcher_SortedByScore(t *testing.T) {
	var s MenuSearcher
	results := s.Search("tom", testMenuItems())
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results for 'tom', got %d", len(results))
	}
	// Scores should be non-increasing.
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("results not sorted: score[%d]=%d > score[%d]=%d",
				i, results[i].Score, i-1, results[i-1].Score)
		}
	}
}

func TestMenuSearcher_Highlights(t *testing.T) {
	var s MenuSearcher
	results := s.Search("hi", testMenuItems())
	for _, r := range results {
		if r.Key == "hihat" {
			if len(r.Highlights) != 2 {
				t.Errorf("expected 2 highlights for 'hi' on 'Hi-Hat', got %d: %v",
					len(r.Highlights), r.Highlights)
			}
			break
		}
	}
}

func TestMenuSearcher_KeyMatchFallback(t *testing.T) {
	var s MenuSearcher
	// "tom_hi" is the key but "High Tom" is the label. Search for "tom_h" — should
	// match via key even if label doesn't contain the underscore.
	results := s.Search("tom_h", testMenuItems())
	found := false
	for _, r := range results {
		if r.Key == "tom_hi" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'tom_hi' to match via key fallback for query 'tom_h'")
	}
}

func TestMenuSearcher_NoResults(t *testing.T) {
	var s MenuSearcher
	results := s.Search("zzz", testMenuItems())
	if len(results) != 0 {
		t.Errorf("expected 0 results for 'zzz', got %d", len(results))
	}
}

func TestMenuSearcher_WhitespaceQuery(t *testing.T) {
	var s MenuSearcher
	results := s.Search("   ", testMenuItems())
	if len(results) != len(testMenuItems()) {
		t.Errorf("whitespace query should return all items, got %d", len(results))
	}
}

func TestMenuSearcher_PreferLabelOverKey(t *testing.T) {
	var s MenuSearcher
	// "ride" matches both key="ride" and label="Ride Cymbal". Should prefer label.
	results := s.Search("ride", testMenuItems())
	for _, r := range results {
		if r.Key == "ride" {
			if len(r.Highlights) == 0 {
				t.Error("expected highlights from label match for 'ride'")
			}
			break
		}
	}
}

// ---------- Button highlight rendering tests ----------

func TestButton_HighlightsFieldDefaultsNil(t *testing.T) {
	btn := NewButton("Test", DropdownStyle, nil)
	if btn.Highlights != nil {
		t.Error("Highlights should default to nil")
	}
}

func TestButton_HighlightsSetOnInstMenuButtons(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := newInstMenuInInstrumentsMode(t)

	// Type a fuzzy query.
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	// Type "kd" (fuzzy for "Kick Drum").
	chars := []rune{'k', 'd'}
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	if len(comp.instBtns) == 0 {
		t.Fatal("expected at least one button after fuzzy search 'kd'")
	}

	// Check that the matching button has highlights set.
	foundHighlighted := false
	for _, btn := range comp.instBtns {
		if len(btn.Highlights) > 0 {
			foundHighlighted = true
			break
		}
	}
	if !foundHighlighted {
		t.Error("expected at least one button with Highlights set after fuzzy search")
	}
}

func TestInstMenuSearchBox_FuzzyMatchResults(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick Drum", Category: ""},
			{ID: "snare", Label: "Snare Drum", Category: ""},
			{ID: "hihat", Label: "Hi-Hat", Category: ""},
			{ID: "clap", Label: "Clap", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// Set search to "kd" — fuzzy match for "Kick Drum".
	comp.SetSearchText("kd")

	if len(comp.state.filteredInsts) == 0 {
		t.Fatal("expected fuzzy match for 'kd' to find 'Kick Drum'")
	}

	foundKick := false
	for _, id := range comp.state.filteredInsts {
		if id == "kick" {
			foundKick = true
		}
	}
	if !foundKick {
		t.Errorf("expected 'kick' in filtered results for fuzzy query 'kd', got %v",
			comp.state.filteredInsts)
	}

	// Verify highlights stored.
	hl, ok := comp.state.searchHighlights["kick"]
	if !ok || len(hl) == 0 {
		t.Error("expected searchHighlights for 'kick' with fuzzy query 'kd'")
	}
}

func TestInstMenuSearchBox_FuzzyAbbreviationMatch(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick Drum", Category: ""},
			{ID: "snare", Label: "Snare Drum", Category: ""},
			{ID: "hihat", Label: "Hi-Hat", Category: ""},
			{ID: "crash", Label: "Crash Cymbal", Category: ""},
			{ID: "ride", Label: "Ride Cymbal", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// "cc" should fuzzy-match "Crash Cymbal" (C...C)
	comp.SetSearchText("cc")
	found := false
	for _, id := range comp.state.filteredInsts {
		if id == "crash" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'crash' in fuzzy results for 'cc', got %v", comp.state.filteredInsts)
	}
}

func TestInstMenuSearchBox_FuzzyBestMatchFirst(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick Drum", Category: ""},
			{ID: "snare", Label: "Snare Drum", Category: ""},
			{ID: "hihat", Label: "Hi-Hat", Category: ""},
			{ID: "clap", Label: "Clap", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// "cl" should put "Clap" first (exact prefix match) before any others.
	comp.SetSearchText("cl")
	if len(comp.state.filteredInsts) == 0 {
		t.Fatal("expected results for 'cl'")
	}
	if comp.state.filteredInsts[0] != "clap" {
		t.Errorf("expected 'clap' first for 'cl', got %q", comp.state.filteredInsts[0])
	}
}
