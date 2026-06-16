//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFunctional_BPMEditor_IsDrawnOverCachedToolbar guards the invisible-editor
// regression: the transport toolbar renders through a CACHE, so the BPM editor
// must be drawn straight to screen in (*TransportZone).Draw, OVER the cached
// blit — not into the cache (which would never invalidate per keystroke). This
// populates the cache first, opens the editor, then asserts it renders during a
// full Draw pass.
func TestFunctional_BPMEditor_IsDrawnOverCachedToolbar(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ { // populate the toolbar cache
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	tz.openBPMEditor()
	if tz.paramEditor == nil || !tz.paramEditor.Active() {
		t.Fatalf("openBPMEditor did not open the editor")
	}
	editorRect := tz.paramEditor.ti.Rect

	// The focused editor draws a blinking caret via the package-level drawCursor.
	var drewInEditor bool
	orig := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) {
		if r.Overlaps(editorRect) {
			drewInEditor = true
		}
		orig(dst, r, c)
	}
	defer func() { drawCursor = orig }()

	if fi.screen == nil {
		fi.screen = ebiten.NewImage(1280, 720)
	}
	g.Draw(fi.screen) // full draw incl. the cached toolbar path
	if !drewInEditor {
		t.Fatalf("BPM editor was NOT drawn during the full Draw pass — invisible-in-production regression (cached toolbar path skipped it)")
	}
}

// Drives arrow keys through the REAL Game.Update loop (the fakeInput harness +
// newDesktopGame live in text_input_escape_functional_test.go).
func TestFunctional_ArrowKeys_BPMEditor(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	tz.openBPMEditor()
	if tz.paramEditor == nil || !tz.paramEditor.Active() {
		t.Fatalf("openBPMEditor did not open the editor")
	}
	start := tz.paramEditor.ti.cursor
	if start == 0 {
		t.Fatalf("precondition: prefilled caret at end (>0), got %d", start)
	}
	cam := g.cam.OffsetX
	fi.pressKey(t, g, ebiten.KeyLeft)
	if g.cam.OffsetX != cam {
		t.Errorf("Left arrow panned grid while editing BPM (%v -> %v)", cam, g.cam.OffsetX)
	}
	if !tz.paramEditor.Active() {
		t.Fatalf("Left arrow closed the BPM editor")
	}
	if tz.paramEditor.ti.cursor != start-1 {
		t.Fatalf("Left arrow did NOT move caret: cursor=%d want %d", tz.paramEditor.ti.cursor, start-1)
	}
}

// TestFunctional_SoftKeyboardArrow_BPMEditor covers the WASM path: with a field
// focused the soft-keyboard proxy holds keyboard focus, so the canvas never sees
// the arrow keydown — the proxy forwards it via SignalSoftKeyboardArrowLeft. This
// fires ONLY the forwarded signal (no canvas key) and asserts the BPM editor
// caret moves. This is the exact bug ("arrows work on EQ but not BPM") now that
// BPM uses the same shared editor.
func TestFunctional_SoftKeyboardArrow_BPMEditor(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	tz.openBPMEditor()
	if tz.paramEditor == nil || !tz.paramEditor.Active() {
		t.Fatalf("openBPMEditor did not open the editor")
	}
	start := tz.paramEditor.ti.cursor
	if start == 0 {
		t.Fatalf("precondition: prefilled caret at end (>0), got %d", start)
	}
	// WASM path: only the forwarded signal, no canvas key.
	SignalSoftKeyboardArrowLeft()
	fi.frame(t, g)
	if tz.paramEditor.ti.cursor != start-1 {
		t.Fatalf("soft-keyboard Left did NOT move BPM caret: cursor=%d want %d", tz.paramEditor.ti.cursor, start-1)
	}
	SignalSoftKeyboardArrowRight()
	fi.frame(t, g)
	if tz.paramEditor.ti.cursor != start {
		t.Fatalf("soft-keyboard Right did NOT move BPM caret back: cursor=%d want %d", tz.paramEditor.ti.cursor, start)
	}
}
