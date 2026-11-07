//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Record is demoted by TONE (muted colRecordIdle against play's green), not by shrinking it: a shrunk key leaves
// bands of its chip that read as a stray line. Both halves are checked: geometry matches, tint differs.
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

// Mid-jam mis-taps are a touchscreen risk, so only mobile demotes it.
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
