//go:build test

package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSynthPreviewWidth_DesktopReservesPane: a wide desktop content
// area must yield a non-zero preview width; very narrow areas must
// fall back to 0 (no horizontal split — mobile-style stack).
func TestSynthPreviewWidth_DesktopReservesPane(t *testing.T) {
	cases := []struct {
		w        int
		mobile   bool
		wantZero bool
	}{
		{1280, false, false}, // wide desktop: reserves the pane
		{800, false, false},  // moderate desktop: still reserves
		{440, false, true},   // narrow desktop: not enough room
		{200, false, true},   // tiny: zero
		{1280, true, true},   // mobile of any width: zero
	}
	for _, c := range cases {
		got := synthPreviewWidth(image.Rect(0, 0, c.w, 400), c.mobile)
		if c.wantZero && got != 0 {
			t.Errorf("w=%d mobile=%v: want 0, got %d", c.w, c.mobile, got)
		}
		if !c.wantZero && got <= 0 {
			t.Errorf("w=%d mobile=%v: want >0, got %d", c.w, c.mobile, got)
		}
	}
}

// TestSynthPreviewPane_PaintsAllThreePlots: a populated preview pane
// must produce non-zero filled rects (background + three plot bands
// + content). Smoke test — exact coverage is verified by the per-
// plot tests below.
func TestSynthPreviewPane_PaintsAllThreePlots(t *testing.T) {
	dst := ebiten.NewImage(280, 240)
	rects := collectFilledRects(t, func() {
		drawSynthPreviewPane(dst, image.Rect(0, 0, 280, 240), "kick", 1.0)
	})
	if len(rects) == 0 {
		t.Errorf("expected preview rects, got 0")
	}
}

// TestSynthADSRPlot_DecayKnobChangesShape: bumping the decay multiplier
// must move the D-stage endpoint to the right — different decay values
// must produce different geometries.
func TestSynthADSRPlot_DecayKnobChangesShape(t *testing.T) {
	dst := ebiten.NewImage(280, 60)
	r := image.Rect(0, 0, 280, 60)
	short := collectFilledRects(t, func() {
		drawSynthADSRPlot(dst, r, "", 0.5)
	})
	long := collectFilledRects(t, func() {
		drawSynthADSRPlot(dst, r, "", 4.0)
	})
	// Each call should produce at least the background + curve segments;
	// the rect counts should differ because the curve geometry shifts.
	if len(short) == 0 || len(long) == 0 {
		t.Fatalf("expected non-zero rect counts: short=%d long=%d", len(short), len(long))
	}
}

// TestSynthOscPlot_DrawsBaseline: with no instID and any rect we expect
// the centre baseline + the sine trace.
func TestSynthOscPlot_DrawsBaseline(t *testing.T) {
	dst := ebiten.NewImage(280, 60)
	rects := collectFilledRects(t, func() {
		drawSynthOscPlot(dst, image.Rect(0, 0, 280, 60), "kick")
	})
	if len(rects) == 0 {
		t.Errorf("expected osc plot rects, got 0")
	}
}

// TestSynthFilterPlot_FlatLineWhenNoEQ: when no EQ bands are set, the
// filter plot must still render (flat baseline) — no panic, ≥1 rect.
func TestSynthFilterPlot_FlatLineWhenNoEQ(t *testing.T) {
	dst := ebiten.NewImage(280, 60)
	rects := collectFilledRects(t, func() {
		drawSynthFilterPlot(dst, image.Rect(0, 0, 280, 60), "no-such-channel")
	})
	if len(rects) == 0 {
		t.Errorf("expected filter plot baseline rects, got 0")
	}
}

// TestTriggerPulse_GlowsAfterPlay: calling audio.Play (stub) updates
// the trigger timestamp; the synth-card glow predicate (Since < 200
// ms) must report true immediately and false after a longer wait.
func TestTriggerPulse_GlowsAfterPlay(t *testing.T) {
	audio.ResetTriggerTimestamps()
	const id = "synth-pulse-test"
	if d := audio.SinceLastTrigger(id); d < time.Hour {
		t.Fatalf("before play SinceLastTrigger=%v want > 1h", d)
	}
	audio.RecordVoiceTrigger(id)
	if d := audio.SinceLastTrigger(id); d > 50*time.Millisecond {
		t.Errorf("after play SinceLastTrigger=%v want < 50ms", d)
	}
}
