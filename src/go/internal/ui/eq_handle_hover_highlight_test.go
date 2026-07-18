//go:build test

package ui

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// setEQCursorForTest points the package cursor at (x, y) and leaves every
// other input source inert. Mirrors noInputForTest but with a live cursor so
// the EQ panel's per-frame hover detection sees a hovering pointer.
func setEQCursorForTest(x, y int) func() {
	return SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
}

// coloredRoundedRect records a single drawRoundedRect call's rect + color.
type coloredRoundedRect struct {
	r image.Rectangle
	c color.Color
}

// installColoredRectInterceptor records rect +
// color of every drawRect call. The migrated hover highlight (drawRectHoverGlow)
// bottoms out in drawRect, so this captures it.
func installColoredRectInterceptor(t *testing.T) *[]coloredRoundedRect {
	t.Helper()
	var mu sync.Mutex
	calls := &[]coloredRoundedRect{}
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		mu.Lock()
		*calls = append(*calls, coloredRoundedRect{r, c})
		mu.Unlock()
		orig(dst, r, c, filled)
	}
	t.Cleanup(func() { drawRect = orig })
	return calls
}

// rgbEq reports whether c has the same straight (non-premultiplied) 8-bit RGB
// as want, ignoring alpha. drawNodeBloom paints WithAlphaFromColor(nodeColor,
// alpha), which returns a color.NRGBA carrying the source RGB verbatim with a
// varying alpha — so comparing straight RGB (via NRGBAModel, not the
// alpha-premultiplied .RGBA()) identifies the highlight-colored bloom.
func rgbEq(c color.Color, want color.Color) bool {
	cn := color.NRGBAModel.Convert(c).(color.NRGBA)
	wn := color.NRGBAModel.Convert(want).(color.NRGBA)
	return cn.R == wn.R && cn.G == wn.G && cn.B == wn.B
}

// TestEQHandleHoverStateTracksCursor verifies the EQ panel tracks which
// draggable handle the desktop cursor is hovering — band handles via
// curveHoverBand, HPF/LPF handles via curveHoverFilter — mirroring the
// press-time hit-test geometry exactly, and staying idle on mobile (no hover).
func TestEQHandleHoverStateTracksCursor(t *testing.T) {
	rect := image.Rect(0, 200, 800, 400)

	t.Run("band handle hovered", func(t *testing.T) {
		z, _ := newTestEQPanelZone(nil)
		z.Layout(rect)
		hx, hy := z.eqBandHandlePos(3)

		restore := setEQCursorForTest(hx, hy)
		defer restore()

		z.Update()

		if z.curveHoverBand != 3 {
			t.Fatalf("cursor over band-3 handle: curveHoverBand=%d, want 3", z.curveHoverBand)
		}
		if z.curveHoverFilter != "" {
			t.Fatalf("cursor over band handle: curveHoverFilter=%q, want empty", z.curveHoverFilter)
		}
	})

	t.Run("HPF filter handle hovered", func(t *testing.T) {
		z, _, fs := newTestEQPanelZoneWithFilters(nil)
		fs.hpfEnabled = true
		fs.hpfCutoff = 200
		z.Layout(rect)

		r := z.eqPlotRect()
		hx := freqToX(fs.hpfCutoff, r)
		hy := z.curveYAtX(hx)

		restore := setEQCursorForTest(hx, hy)
		defer restore()

		z.Update()

		if z.curveHoverFilter != "hpf" {
			t.Fatalf("cursor over HPF handle: curveHoverFilter=%q, want %q", z.curveHoverFilter, "hpf")
		}
		if z.curveHoverBand != -1 {
			t.Fatalf("cursor over filter handle: curveHoverBand=%d, want -1 (filter takes priority)", z.curveHoverBand)
		}
	})

	t.Run("cursor off every handle", func(t *testing.T) {
		z, _ := newTestEQPanelZone(nil)
		z.Layout(rect)

		// Top-centre of the panel — above the gain-0 handle row, clear of
		// every handle's hit radius.
		restore := setEQCursorForTest(rect.Dx()/2, rect.Min.Y+2)
		defer restore()

		z.Update()

		if z.curveHoverBand != -1 {
			t.Fatalf("cursor off all handles: curveHoverBand=%d, want -1", z.curveHoverBand)
		}
		if z.curveHoverFilter != "" {
			t.Fatalf("cursor off all handles: curveHoverFilter=%q, want empty", z.curveHoverFilter)
		}
	})

	t.Run("mobile has no hover", func(t *testing.T) {
		forceSmallScreenForTest = true
		defer func() { forceSmallScreenForTest = false }()

		z, _ := newTestEQPanelZone(nil)
		z.Layout(rect)
		hx, hy := z.eqBandHandlePos(3)

		restore := setEQCursorForTest(hx, hy)
		defer restore()

		z.Update()

		if z.curveHoverBand != -1 {
			t.Fatalf("mobile: curveHoverBand=%d, want -1 (no pointer hover on touch)", z.curveHoverBand)
		}
		if z.curveHoverFilter != "" {
			t.Fatalf("mobile: curveHoverFilter=%q, want empty", z.curveHoverFilter)
		}
	})
}

// countBloomRingsAt counts filled rounded-rects of the given color (matched on
// straight RGB, alpha-agnostic) centred on (cx, cy) — the drawNodeBloom
// signature. Used to detect a hover bloom in a specific color at a handle.
func countBloomRingsAt(calls []coloredRoundedRect, want color.Color, cx, cy int) int {
	n := 0
	for _, call := range calls {
		if !rgbEq(call.c, want) {
			continue
		}
		ccx := (call.r.Min.X + call.r.Max.X) / 2
		ccy := (call.r.Min.Y + call.r.Max.Y) / 2
		if abs(ccx-cx) <= 1 && abs(ccy-cy) <= 1 {
			n++
		}
	}
	return n
}

// TestEQHandleHoverUsesHandleOwnColor verifies that a hovered EQ handle glows
// with the handle's OWN color (band handles = colEQHandle amber, HPF/LPF =
// colEQFilterHandle cyan) rather than the white colHighlight, via the shared
// shape-agnostic drawRectHoverGlow. Detected differentially: hovering must ADD
// own-color glow rects (drawRect) over the not-hovered baseline; no white glow
// must appear at the handle.
func TestEQHandleHoverUsesHandleOwnColor(t *testing.T) {
	rect := image.Rect(0, 200, 800, 400)

	t.Run("band handle glows in its own amber color, not white", func(t *testing.T) {
		z, _ := newTestEQPanelZone(nil)
		z.Layout(rect)
		hx, hy := z.eqBandHandlePos(4)
		scratch := ebiten.NewImage(800, 400)

		calls := installColoredRectInterceptor(t)

		off := setEQCursorForTest(rect.Dx()/2, rect.Min.Y+2)
		z.Update()
		z.Draw(scratch)
		baseOwn := countBloomRingsAt(*calls, colEQHandle, hx, hy)
		off()

		*calls = (*calls)[:0]

		on := setEQCursorForTest(hx, hy)
		z.Update()
		z.Draw(scratch)
		hoverOwn := countBloomRingsAt(*calls, colEQHandle, hx, hy)
		hoverWhite := countBloomRingsAt(*calls, colHighlight, hx, hy)
		on()

		if hoverOwn <= baseOwn {
			t.Fatalf("hovering band-4 handle did not add own-color (amber) glow rects: baseline=%d hover=%d", baseOwn, hoverOwn)
		}
		if hoverWhite != 0 {
			t.Fatalf("band-4 hover drew %d white glow rects; want the handle's own color, not white", hoverWhite)
		}
	})

	t.Run("HPF handle glows in its own cyan color, not white", func(t *testing.T) {
		z, _, fs := newTestEQPanelZoneWithFilters(nil)
		fs.hpfEnabled = true
		fs.hpfCutoff = 200
		z.Layout(rect)

		r := z.eqPlotRect()
		hx := freqToX(fs.hpfCutoff, r)
		hy := z.curveYAtX(hx)
		scratch := ebiten.NewImage(800, 400)

		calls := installColoredRectInterceptor(t)

		off := setEQCursorForTest(rect.Dx()/2, rect.Min.Y+2)
		z.Update()
		z.Draw(scratch)
		baseOwn := countBloomRingsAt(*calls, colEQFilterHandle, hx, hy)
		off()

		*calls = (*calls)[:0]

		on := setEQCursorForTest(hx, hy)
		z.Update()
		z.Draw(scratch)
		hoverOwn := countBloomRingsAt(*calls, colEQFilterHandle, hx, hy)
		hoverWhite := countBloomRingsAt(*calls, colHighlight, hx, hy)
		on()

		if hoverOwn <= baseOwn {
			t.Fatalf("hovering HPF handle did not add own-color (cyan) glow rects: baseline=%d hover=%d", baseOwn, hoverOwn)
		}
		if hoverWhite != 0 {
			t.Fatalf("HPF hover drew %d white glow rects; want the handle's own color, not white", hoverWhite)
		}
	})

	t.Run("no hover, no glow", func(t *testing.T) {
		z, _ := newTestEQPanelZone(nil)
		z.Layout(rect)
		hx, hy := z.eqBandHandlePos(4)
		scratch := ebiten.NewImage(800, 400)

		calls := installColoredRectInterceptor(t)

		restore := setEQCursorForTest(rect.Dx()/2, rect.Min.Y+2)
		defer restore()

		z.Update()
		z.Draw(scratch)

		if got := countBloomRingsAt(*calls, colHighlight, hx, hy); got != 0 {
			t.Fatalf("no hover: %d white glow rects at the band handle, want 0", got)
		}
	})
}
