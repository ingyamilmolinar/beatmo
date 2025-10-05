package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"image"
	"testing"
)

// Clicking a subdiv menu item should not trigger underlying controls.
func TestSubdivMenuNoClickThrough(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	// Open menu
	c := dv.subdivBtn.Rect()
	cx, cy := (c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	restore()
	if !dv.subdivMenuOpen || len(dv.subdivMenuBtns) == 0 {
		t.Fatalf("subdiv menu not open or empty")
	}
	// Click first menu item
	r := dv.subdivMenuBtns[0].Rect()
	rx, ry := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore = SetInputForTest(func() (int, int) { return rx, ry }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	restore()
	if dv.subdivMenuOpen {
		t.Fatalf("subdiv menu did not close after selection")
	}
	// Ensure rename/edit overlays not opened inadvertently and no row added
	if dv.renameBox != nil {
		t.Fatalf("rename box opened unexpectedly")
	}
	if len(dv.added) != 0 {
		t.Fatalf("row added unexpectedly after subdiv click")
	}
}
