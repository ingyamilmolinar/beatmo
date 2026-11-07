//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Desktop (1280x800), where the kebab shows in-row.
func newKebabTestDrumView(t *testing.T) *DrumView {
	t.Helper()
	const W, H = 1280, 800
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: roster.Kick,
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

// The neutral body at rounded.control, an "overflow" icon, at least TouchMinTarget() tall.
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
	cs, ok := b.Style.(rowToggleVisual)
	if !ok {
		t.Fatalf("row-kebab Style is not the row-control visual, got %T", b.Style)
	}
	if cs.Radius != RadiusControl {
		t.Errorf("row-kebab Radius=%v, want RadiusControl (%v)", cs.Radius, RadiusControl)
	}
	kebabRect := b.Rect()
	if kebabRect.Empty() {
		t.Skip("kebab button rect empty (not visible in this layout)")
	}
	if kebabRect.Dy() < TouchMinTarget() {
		t.Errorf("row-kebab height %d below TouchMinTarget %d", kebabRect.Dy(), TouchMinTarget())
	}

	// The last rack column, right of the label with the other controls between them.
	groups := dv.rowRackZone.RowGroups()
	if len(groups) == 0 || groups[0].Label == nil || groups[0].Label.Rect().Empty() {
		t.Skip("no label rect to compare alignment")
	}
	labelMaxX := groups[0].Label.Rect().Max.X
	if kebabRect.Min.X < labelMaxX {
		t.Errorf("row-kebab Min.X=%d is left of label Max.X=%d (kebab should be right of label)", kebabRect.Min.X, labelMaxX)
	}
}

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
