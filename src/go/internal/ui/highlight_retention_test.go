//go:build test

package ui

import (
	"testing"
)

// TestHighlightedBeatsScrollAwayEvicts seeds entries with small abs values,
// advances the drum offset past them, and asserts clearExpiredHighlights
// drops every entry with idx < drum.Offset - highlightAbsRetentionSlack.
func TestHighlightedBeatsScrollAwayEvicts(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Seed 10 highlights at row 0, abs 1..10. Use frame=0 so 'until' is 0,
	// meaning the time-based eviction predicate fires too — but isolate the
	// abs-eviction by using a fresh game with frame still at 0 and a future
	// 'until' value.
	for abs := 1; abs <= 10; abs++ {
		key := makeBeatKey(0, abs)
		g.highlightSet(key, int64(g.frame+1_000_000)) // far-future expiry
	}
	if got := len(g.highlightedBeats); got != 10 {
		t.Fatalf("seeded %d highlights, want 10", got)
	}

	// Advance the drum window so abs 1..10 fall well below
	// (Offset - highlightAbsRetentionSlack).
	g.drum.Offset = 1000

	g.clearExpiredHighlights()

	if got := len(g.highlightedBeats); got != 0 {
		t.Errorf("after scroll-away eviction: %d highlights remain, want 0", got)
	}
}

// TestHighlightedBeatsBoundedUnderLongPlayback seeds highlights monotonically
// at increasing abs and asserts the map stays bounded by drum.Length +
// highlightAbsRetentionSlack — the visible window plus its lookback slack.
func TestHighlightedBeatsBoundedUnderLongPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)
	g.drum.SetLength(64)

	// Walk the drum offset forward and seed a highlight at each abs.
	const N = 5_000
	for i := 0; i < N; i++ {
		g.drum.Offset = i
		key := makeBeatKey(0, i)
		g.highlightSet(key, int64(g.frame+1_000_000))
		g.clearExpiredHighlights()
	}

	bound := g.drum.Length + highlightAbsRetentionSlack + 1
	if got := len(g.highlightedBeats); got > bound {
		t.Errorf("len(highlightedBeats)=%d exceeds bound %d (drum.Length=%d + slack=%d)",
			got, bound, g.drum.Length, highlightAbsRetentionSlack)
	}
}
