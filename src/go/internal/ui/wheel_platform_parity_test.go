//go:build test

// Platform wheel-dynamics boundary.
//
// Bug (2026-09-02): in the browser one mouse-wheel notch scrolled a menu
// through its entire list. DrumViewTree's wheel dispatch read the raw platform
// delta (`steps := int(wy)`) and handed it to MenuScroll.HandleWheel, which by
// design RESPECTS magnitude — one step, one row. Correct for native GLFW, which
// reports +/-1 per detent; catastrophic in a browser, where Ebiten's JS backend
// assigns `WheelY = -deltaY` RAW (ebiten/internal/ui/input_js.go, which carries
// an upstream "TODO: What if e.deltaMode is not DOM_DELTA_PIXEL?"). One Chrome
// notch arrives as ~100 and scrolled ~100 rows.
//
// The boundary is now explicit and one-way:
//
//	raw platform units --[wheelNotchesFromRaw]--> DETENTS --[WheelNormalizer]--> rows
//	        ^ platform-specific                     ^ platform-neutral from here up
//
// These tests pin both halves. Layering matters: a raw delta of 100 is
// genuinely 100 detents on native GLFW and ~1 detent in a pixel-mode browser,
// so the raw->detent conversion is tested as a pure unit (where deltaMode is
// known) and the detent->rows dispatch is tested through the real Game.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// --- Layer 1: raw platform units -> detents -------------------------------

// TestWheelNotchesFromRaw_OneNotchPerPlatform pins the conversion that makes
// every runtime speak detents. The raw magnitudes are what the DOM actually
// reports for ONE physical wheel notch in each deltaMode.
func TestWheelNotchesFromRaw_OneNotchPerPlatform(t *testing.T) {
	cases := []struct {
		name string
		raw  float64
		mode WheelDeltaMode
		want float64
	}{
		{"chrome_safari_edge_pixel", 100, WheelDeltaPixel, 1},
		{"firefox_line", 3, WheelDeltaLine, 1},
		{"page_mode", 1, WheelDeltaPage, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wheelNotchesFromRaw(tc.raw, tc.mode)
			if got != tc.want {
				t.Errorf("wheelNotchesFromRaw(%v, %v) = %v, want %v — one physical "+
					"notch must be 1.0 detents in every deltaMode", tc.raw, tc.mode, got, tc.want)
			}
		})
	}
}

// TestWheelNotchesFromRaw_SubNotchIsFractional pins that a trackpad glide is
// NOT rounded away at the boundary: small deltas stay fractional so the
// normalizer's carry can accumulate them into real movement.
func TestWheelNotchesFromRaw_SubNotchIsFractional(t *testing.T) {
	got := wheelNotchesFromRaw(12, WheelDeltaPixel)
	if got <= 0 || got >= 1 {
		t.Errorf("wheelNotchesFromRaw(12, pixel) = %v; a sub-notch trackpad delta must "+
			"stay a fraction of a detent (0 < n < 1), not round to 0 or 1", got)
	}
}

// TestWheelNotchesFromRaw_PreservesSign guards the scroll direction across the
// conversion — an inverted sign here would flip every list on one platform.
func TestWheelNotchesFromRaw_PreservesSign(t *testing.T) {
	if up, down := wheelNotchesFromRaw(100, WheelDeltaPixel), wheelNotchesFromRaw(-100, WheelDeltaPixel); up <= 0 || down >= 0 {
		t.Errorf("sign not preserved: +100 -> %v, -100 -> %v", up, down)
	}
}

// --- Layer 2: detents -> rows (WheelNormalizer) ---------------------------

// TestWheelNormalizer_OneNotchOneRow is the core feel contract.
func TestWheelNormalizer_OneNotchOneRow(t *testing.T) {
	n := NewWheelNormalizer(defaultWheelDynamics())
	if got := n.Rows(1); got != 1 {
		t.Errorf("one detent produced %d rows, want 1", got)
	}
}

// TestWheelNormalizer_CarriesSubNotchDeltas pins that a slow trackpad glide
// eventually moves the list instead of being silently truncated to zero.
func TestWheelNormalizer_CarriesSubNotchDeltas(t *testing.T) {
	n := NewWheelNormalizer(defaultWheelDynamics())
	total := 0
	for i := 0; i < 10; i++ {
		total += n.Rows(0.12) // ~1/8 notch per event
	}
	if total < 1 {
		t.Errorf("ten sub-notch events produced %d rows; the remainder must carry "+
			"between frames so slow trackpad scrolling still moves the list", total)
	}
}

// TestWheelNormalizer_CapsPerFrameButCarriesRemainder pins that one violent
// flick glides instead of teleporting — and that the excess is not discarded.
func TestWheelNormalizer_CapsPerFrameButCarriesRemainder(t *testing.T) {
	d := defaultWheelDynamics()
	n := NewWheelNormalizer(d)
	first := n.Rows(9)
	if first != d.MaxRowsPerFrame {
		t.Errorf("a 9-detent flick emitted %d rows in one frame, want the cap %d",
			first, d.MaxRowsPerFrame)
	}
	second := n.Rows(0)
	if second <= 0 {
		t.Errorf("the capped remainder was discarded: follow-up frame emitted %d rows, "+
			"want the flick to keep gliding", second)
	}
}

// TestWheelNormalizer_StaleCarryDecays pins that a remainder left over from an
// old gesture cannot accumulate into a phantom jump much later.
func TestWheelNormalizer_StaleCarryDecays(t *testing.T) {
	n := NewWheelNormalizer(defaultWheelDynamics())
	n.Rows(0.4) // leaves carry, emits nothing
	for i := 0; i < 120; i++ {
		if got := n.Rows(0); got != 0 {
			t.Fatalf("idle frame %d emitted %d rows from stale carry", i, got)
		}
	}
}

// --- Layer 3: end-to-end through the real dispatcher ----------------------

// scrollRowsForNotches drives a real Game with the template menu open, feeding
// exactly one frame of wheel movement expressed in DETENTS, and reports how
// many rows the menu scrolled.
func scrollRowsForNotches(t *testing.T, notches float64) int {
	t.Helper()
	assertDefaultParityState(t)

	var mx, my int
	fed := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if !fed {
				fed = true
				return 0, -notches // negative = scroll down the list
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.overflowPage = 1
	dv.OpenOverflowMenu()

	scratch := ebiten.NewImage(1280, 720)
	for i := 0; i < 5; i++ {
		_ = g.Update()
		g.Draw(scratch)
	}

	sc := dv.OverflowMenuScrollForTest()
	if sc == nil {
		t.Fatal("overflow MenuScroll is nil")
	}
	if !sc.HasScroll() {
		t.Fatal("harness: template menu does not overflow, nothing to scroll")
	}
	before := sc.ScrollBehavior().VS.First

	popup := dv.overflowPopupRect()
	mx = (popup.Min.X + popup.Max.X) / 2
	my = popup.Min.Y + popup.Dy()*3/4

	fed = false
	_ = g.Update()
	g.Draw(scratch)

	return sc.ScrollBehavior().VS.First - before
}

// TestMenuWheel_OneNotchScrollsOneRow is the end-to-end regression test: with
// the boundary in place, one detent moves the menu exactly one row no matter
// which platform produced it.
func TestMenuWheel_OneNotchScrollsOneRow(t *testing.T) {
	if got := scrollRowsForNotches(t, 1); got != 1 {
		t.Errorf("one detent scrolled %d menu rows, want 1", got)
	}
}

// TestMenuWheel_FlickIsCapped is the direct regression for the reported bug:
// a large single wheel event must not run through the whole list at once.
func TestMenuWheel_FlickIsCapped(t *testing.T) {
	d := defaultWheelDynamics()
	got := scrollRowsForNotches(t, 40)
	if got > d.MaxRowsPerFrame {
		t.Errorf("a 40-detent flick scrolled %d menu rows in ONE frame (cap %d).\n"+
			"This is the browser bug: an unnormalized delta reaching MenuScroll "+
			"runs through the entire list on a single wheel event.", got, d.MaxRowsPerFrame)
	}
	if got < 1 {
		t.Errorf("a 40-detent flick scrolled %d rows; it must still move the list", got)
	}
}
