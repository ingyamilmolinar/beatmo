package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// drawRectCall is one recorded drawRect invocation.
type drawRectCall struct {
	rect   image.Rectangle
	col    color.Color
	filled bool
}

// sameRGBA compares two colors by their RGBA() output (so RGBA and NRGBA
// of equivalent appearance compare equal).
func sameRGBA(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// installDrawRectRecorder replaces the package-level drawRect with a
// recorder for the duration of the test. The caller restores via Cleanup.
func installDrawRectRecorder(t *testing.T) *[]drawRectCall {
	t.Helper()
	var calls []drawRectCall
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, drawRectCall{rect: r, col: c, filled: filled})
	}
	t.Cleanup(func() { drawRect = orig })
	return &calls
}

func countByColor(calls *[]drawRectCall, want color.Color) int {
	n := 0
	for _, c := range *calls {
		if sameRGBA(c.col, want) {
			n++
		}
	}
	return n
}

// TestDrawLevelsDetail_NilChannelDrawsBorderOnly asserts the empty state
// — nil channel should produce only the panel border, no level bars.
func TestDrawLevelsDetail_NilChannelDrawsBorderOnly(t *testing.T) {
	assertDefaultParityState(t)

	w, h := 200, 100
	dst := ebiten.NewImage(w, h)
	rect := image.Rect(0, 0, w, h)

	calls := installDrawRectRecorder(t)

	drawLevelsDetail(dst, rect, nil, nil)

	if got := len(*calls); got != 1 {
		t.Errorf("nil channel should draw exactly 1 rect (border), got %d", got)
	}
}

// TestDrawLevelsDetail_LowLevelUsesLowColor asserts the meter color when the
// peak is well below the mid threshold — must be the low-zone color (golden).
func TestDrawLevelsDetail_LowLevelUsesLowColor(t *testing.T) {
	assertDefaultParityState(t)

	w, h := 240, 120
	dst := ebiten.NewImage(w, h)
	rect := image.Rect(0, 0, w, h)

	ch := &analyzer.ChannelMetrics{
		ID:         "kick",
		Name:       "Kick",
		PeakDB:     -20,
		RMSDB:      -24,
		HeadroomDB: 20,
		ClipCount:  0,
		Active:     true,
	}

	calls := installDrawRectRecorder(t)

	drawLevelsDetail(dst, rect, ch, nil)

	if g := countByColor(calls, meterLow); g < 2 {
		t.Errorf("expected at least 2 low-zone (golden) rects (peak + RMS fill), got %d", g)
	}
	if r := countByColor(calls, meterHigh); r != 0 {
		t.Errorf("expected 0 red rects at low level, got %d", r)
	}
}

// TestDrawLevelsDetail_ClipPaintsRed asserts that a clip event lights up
// the meter bar in red.
func TestDrawLevelsDetail_ClipPaintsRed(t *testing.T) {
	assertDefaultParityState(t)

	w, h := 240, 120
	dst := ebiten.NewImage(w, h)
	rect := image.Rect(0, 0, w, h)

	ch := &analyzer.ChannelMetrics{
		ID:         "kick",
		Name:       "Kick",
		PeakDB:     0,
		RMSDB:      -3,
		HeadroomDB: 0,
		ClipCount:  5,
		Active:     true,
	}

	calls := installDrawRectRecorder(t)

	drawLevelsDetail(dst, rect, ch, nil)

	if r := countByColor(calls, meterHigh); r < 1 {
		t.Errorf("expected at least 1 red rect at 0 dB peak, got %d", r)
	}
}

// TestLevelsLatch_TriggersOnClipIncrease asserts the latch fires when
// ClipCount goes up, and stays active for ~clipLatchFrames frames after.
func TestLevelsLatch_TriggersOnClipIncrease(t *testing.T) {
	assertDefaultParityState(t)

	var l LevelsLatch
	if l.Latched() {
		t.Fatalf("fresh latch should not be active")
	}

	if !l.Update(1, -6, -12) {
		t.Fatalf("latch must activate on clip increase (0 → 1)")
	}
	if l.LatchFramesLeft != clipLatchFrames {
		t.Errorf("latch frame counter: got %d, want %d", l.LatchFramesLeft, clipLatchFrames)
	}

	// No new clip — counter decays.
	for i := 0; i < 10; i++ {
		l.Update(1, -6, -12)
	}
	if l.LatchFramesLeft != clipLatchFrames-10 {
		t.Errorf("latch decay after 10 frames: got %d, want %d", l.LatchFramesLeft, clipLatchFrames-10)
	}

	// New clip resets the latch.
	if !l.Update(2, -6, -12) {
		t.Fatalf("latch must re-activate on clip increase (1 → 2)")
	}
	if l.LatchFramesLeft != clipLatchFrames {
		t.Errorf("latch should reset to %d on new clip, got %d", clipLatchFrames, l.LatchFramesLeft)
	}

	// Drain the latch.
	for i := 0; i < clipLatchFrames+5; i++ {
		l.Update(2, -6, -12)
	}
	if l.Latched() {
		t.Errorf("latch should expire after %d frames of no new clips", clipLatchFrames)
	}
}
