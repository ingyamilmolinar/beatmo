//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRecordButtonMobile_DemotedByToneNotGeometry pins the replacement for the
// old "B12" geometric demote.
//
// Record used to be shrunk 8 px top and bottom on mobile so it would not sit at
// equal visual weight with play/stop. That left two ~6 px bands of the
// surrounding chip visible above and below it, which read on screen as a
// horizontal line cutting across the top of its slot. The demotion is now
// carried entirely by TONE — record's muted colRecordIdle against play's
// saturated green — so the box can stay aligned with its siblings.
//
// This test asserts BOTH halves of that trade: the geometry matches, and the
// tint still differs. Dropping either one would restore the bug or lose the
// demotion. See DESIGN.md "Cluster alignment".
func TestRecordButtonMobile_DemotedByToneNotGeometry(t *testing.T) {
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
	if rec.Dy() != play.Dy() || rec.Min.Y != play.Min.Y {
		t.Errorf("record %v must share play's vertical bounds %v — a geometric demote leaves a seam",
			rec, play)
	}
	// Tone still separates them: record is the idle red, play is not.
	tz.decayAnims()
	if tz.recordBtn.IconColor != colRecordIdle {
		t.Errorf("record icon = %v, want colRecordIdle %v — tone is what carries the demotion now",
			tz.recordBtn.IconColor, colRecordIdle)
	}
	if tz.playBtn.IconColor == tz.recordBtn.IconColor {
		t.Error("play and record share an icon color; nothing distinguishes record's visual weight")
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
