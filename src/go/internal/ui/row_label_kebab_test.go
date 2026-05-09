//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newKebabTestDrumView creates a DrumView with one row for kebab-button tests,
// runs a warm-up frame so layout is initialized, and returns the view.
// Uses desktop layout (1280x800) where the kebab button is visible in-row;
// on mobile the button is hidden (context menu is opened via label tap).
func newKebabTestDrumView(t *testing.T) *DrumView {
	t.Helper()
	const W, H = 1280, 800
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	restore()
	return dv
}

// TestRowKebab_StyledAsChip pins the row-menu button visual contract:
// rounded.sm chip background at AlphaSubtle, an "overflow" icon, and at
// least TouchMinTarget() height. The button is visible on desktop layout
// (col 5 of the row rack); on mobile it is hidden and context menu is
// triggered by tapping the row label instead.
func TestRowKebab_StyledAsChip(t *testing.T) {
	assertDefaultParityState(t)
	// Desktop layout: profile defaults to desktop at 1280×800.

	dv := newKebabTestDrumView(t)

	btns := dv.rowMenuBtns()
	if len(btns) == 0 {
		t.Fatalf("expected at least one row-menu button, got 0")
	}
	b := btns[0]
	if b.Icon != "overflow" {
		t.Errorf("row-kebab Icon=%q, want 'overflow'", b.Icon)
	}
	cs, ok := b.Style.(ButtonStyle)
	if !ok {
		t.Fatalf("row-kebab Style is not ButtonStyle, got %T", b.Style)
	}
	if cs.Radius != RadiusSM {
		t.Errorf("row-kebab Radius=%v, want RadiusSM (%v)", cs.Radius, RadiusSM)
	}
	kebabRect := b.Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	if kebabRect.Dy() < TouchMinTarget() {
		t.Errorf("row-kebab height %d below TouchMinTarget %d", kebabRect.Dy(), TouchMinTarget())
	}

	// The kebab sits in the last column of the row rack (col 5).
	// Verify it's right of the label (col 0). The gap may be large because
	// vol/mute/solo/FX controls sit between them, so we check only that
	// kebab.Min.X > label.Max.X (no overlap to the left of the label).
	groups := dv.rowRackZone.RowGroups()
	if len(groups) == 0 || groups[0].Label == nil || groups[0].Label.Rect().Empty() {
		t.Skip("no label rect to compare alignment")
	}
	labelMaxX := groups[0].Label.Rect().Max.X
	if kebabRect.Min.X < labelMaxX {
		t.Errorf("row-kebab Min.X=%d is left of label Max.X=%d (kebab should be right of label)", kebabRect.Min.X, labelMaxX)
	}
}

// TestRowKebab_TapOpensContextMenu pins the click-through (regression
// guard for the existing kebab tap → context menu flow).
func TestRowKebab_TapOpensContextMenu(t *testing.T) {
	assertDefaultParityState(t)
	// Desktop layout: profile defaults to desktop at 1280×800.

	dv := newKebabTestDrumView(t)

	btns := dv.rowMenuBtns()
	if len(btns) == 0 {
		t.Fatalf("no kebab buttons")
	}
	kebabRect := btns[0].Rect()
	if kebabRect.Empty() {
		t.Skip("kebab[0] rect empty")
	}
	kx := kebabRect.Min.X + kebabRect.Dx()/2
	ky := kebabRect.Min.Y + kebabRect.Dy()/2

	// Simulate a mouse press at the kebab center → fires OnClick → opens context menu.
	restore := SetInputForTest(
		func() (int, int) { return kx, ky },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 800 },
	)
	dv.Update()
	restore()

	if !dv.IsContextMenuOpen() {
		t.Errorf("expected context menu open after kebab tap")
	}
}
