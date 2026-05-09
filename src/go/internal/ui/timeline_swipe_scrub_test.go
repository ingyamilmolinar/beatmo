//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTimelineScrub_MobileHitAreaSpansFullTimelineRect verifies the
// mobile scrub hit surface is at least TouchMinTarget tall (B5 critique:
// the bar-only hit area was too narrow for thumb scrubs).
func TestTimelineScrub_MobileHitAreaSpansFullTimelineRect(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatalf("timeline-scrub hit area not found")
	}
	if scrubArea.Rect.Dy() < TouchMinTarget() {
		t.Fatalf("mobile scrub hit area height %d < TouchMinTarget %d", scrubArea.Rect.Dy(), TouchMinTarget())
	}
}

// TestTimelineScrub_HitAreaExcludesLenButtons verifies the enlarged
// scrub area does NOT swallow taps on the len ± buttons (when they
// exist on a given layout).
func TestTimelineScrub_HitAreaExcludesLenButtons(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timelineZone nil")
	}
	scrubArea := findHitAreaByTagPrefix(z.HitAreas(), "timeline-scrub")
	if scrubArea == nil {
		t.Fatalf("timeline-scrub hit area not found")
	}
	if z.lenIncBtn != nil {
		if r := z.lenIncBtn.Rect(); !r.Empty() && !scrubArea.Rect.Intersect(r).Empty() {
			t.Errorf("scrub hit area %v overlaps lenIncBtn %v", scrubArea.Rect, r)
		}
	}
	if z.lenDecBtn != nil {
		if r := z.lenDecBtn.Rect(); !r.Empty() && !scrubArea.Rect.Intersect(r).Empty() {
			t.Errorf("scrub hit area %v overlaps lenDecBtn %v", scrubArea.Rect, r)
		}
	}
}
