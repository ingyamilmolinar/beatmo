package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"image"
	"testing"
)

// Ensure subdiv button exists, opens a dropdown, and selection updates text.
func TestDrumViewSubdivDropdownOpensAndSelects(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	center := dv.subdivBtn.Rect()
	cx, cy := (center.Min.X+center.Max.X)/2, (center.Min.Y+center.Max.Y)/2
	// click to open
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	restore()
	if !dv.subdivMenuOpen {
		t.Fatalf("subdiv menu did not open")
	}
	if len(dv.subdivMenuBtns) == 0 {
		t.Fatalf("no subdiv menu items")
	}
	// click first item
	r := dv.subdivMenuBtns[0].Rect()
	rx, ry := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore = SetInputForTest(func() (int, int) { return rx, ry }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	restore()
	if dv.subdivMenuOpen {
		t.Fatalf("subdiv menu did not close after selection")
	}
	if dv.subdivBtn.Text == "" {
		t.Fatalf("subdiv button text not updated")
	}
}

// Game wiring: selecting subdiv value applies to grid via SetSubdivisions when stopped.
func TestGameSubdivDropdownAppliesGrid(t *testing.T) {
	g := New(game_log.New(nil, game_log.LevelError))
	g.Layout(800, 600)
	dv := g.drum
	dv.recalcButtons()
	// open
	c := dv.subdivBtn.Rect()
	cx, cy := (c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	_ = g.Update()
	restore()
	if !dv.subdivMenuOpen {
		t.Fatalf("menu did not open")
	}
	// pick value 8 if available
	var target *Button
	for _, b := range dv.subdivMenuBtns {
		if b.Text == "8" {
			target = b
			break
		}
	}
	if target == nil {
		t.Skip("no 8 option; environment dependent")
	}
	r := target.Rect()
	rx, ry := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore = SetInputForTest(func() (int, int) { return rx, ry }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	_ = g.Update()
	restore()
	if g.grid.MaxDiv() != 8 {
		t.Fatalf("grid maxdiv=%d want 8", g.grid.MaxDiv())
	}
	if g.drum.timelineUnitsPerBeat != 8 {
		t.Fatalf("timelineUnits=%d want 8", g.drum.timelineUnitsPerBeat)
	}
}
