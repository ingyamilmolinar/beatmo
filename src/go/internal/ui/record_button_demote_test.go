//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRecordButtonMobile_VisuallyDemoted verifies B12: on mobile the
// record button has a smaller drawn rect than play/stop so the red dot
// doesn't sit at equal visual weight with the primary transport actions.
func TestRecordButtonMobile_VisuallyDemoted(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	play := tz.playBtn.Rect()
	rec := tz.recordBtn.Rect()
	if rec.Empty() || play.Empty() {
		t.Fatalf("rects unexpectedly empty: play=%v rec=%v", play, rec)
	}
	if rec.Dy() >= play.Dy() {
		t.Fatalf("record height %d should be < play height %d on mobile (B12 demote)", rec.Dy(), play.Dy())
	}
	if rec.Dx() >= play.Dx() {
		t.Fatalf("record width %d should be < play width %d on mobile (B12 demote)", rec.Dx(), play.Dx())
	}
}

// TestRecordButtonDesktop_Unchanged is a regression guard that desktop
// keeps record at parity with play/stop. Demote is a mobile-only call
// because mid-jam mis-tap risk is much higher on touchscreens.
func TestRecordButtonDesktop_Unchanged(t *testing.T) {
	setupMobileTest(t, false)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	tz := g.drum.transportZone
	if tz == nil {
		t.Fatal("transport zone not initialized")
	}
	play := tz.playBtn.Rect()
	rec := tz.recordBtn.Rect()
	if rec.Empty() || play.Empty() {
		t.Fatalf("rects unexpectedly empty: play=%v rec=%v", play, rec)
	}
	if dh := rec.Dy() - play.Dy(); dh < -2 || dh > 2 {
		t.Fatalf("desktop record height %d should match play %d within 2 px", rec.Dy(), play.Dy())
	}
}
