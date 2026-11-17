package ui

import (
	"image"
	"strconv"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Ensure subdiv button exists, opens a dropdown, and selection updates text.
func TestDrumViewSubdivDropdownOpensAndSelects(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	center := dv.subdivBtn().Rect()
	cx, cy := (center.Min.X+center.Max.X)/2, (center.Min.Y+center.Max.Y)/2
	// Press to open
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	t.Cleanup(restore)
	dv.Update()
	restore()
	// Release frame (tree needs press→release cycle)
	restore = SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	restore()
	if !dv.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu did not open")
	}
	if len(dv.subdivMenuBtns) == 0 {
		t.Fatalf("no subdiv menu items")
	}
	selectedText := dv.subdivMenuBtns[0].Text
	// Click first item
	r := dv.subdivMenuBtns[0].Rect()
	rx, ry := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore = SetInputForTest(func() (int, int) { return rx, ry }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu did not close after selection")
	}
	wantText := "\u00f7" + selectedText // ÷ prefix
	if dv.subdivBtn().Text != wantText {
		t.Fatalf("subdiv button text %q want %q", dv.subdivBtn().Text, wantText)
	}
}

// Game wiring: selecting subdiv value applies to grid via SetSubdivisions when stopped.
func TestGameSubdivDropdownAppliesGrid(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum
	dv.recalcButtons()
	// Press to open
	c := dv.subdivBtn().Rect()
	cx, cy := (c.Min.X+c.Max.X)/2, (c.Min.Y+c.Max.Y)/2
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	t.Cleanup(restore)
	_ = g.Update()
	restore()
	// Release frame (tree needs press→release cycle)
	restore = SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	_ = g.Update()
	restore()
	if !dv.IsSubdivMenuOpen() {
		t.Fatalf("menu did not open")
	}
	// Pick any value different from the current grid maxdiv
	current := g.grid.MaxDiv()
	var target *Button
	var targetVal int
	for _, b := range dv.subdivMenuBtns {
		v, err := strconv.Atoi(b.Text)
		if err != nil {
			continue
		}
		if v != current {
			target = b
			targetVal = v
			break
		}
	}
	if target == nil {
		t.Fatalf("no alternate subdiv option found; current=%d", current)
	}
	r := target.Rect()
	rx, ry := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore = SetInputForTest(func() (int, int) { return rx, ry }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	t.Cleanup(restore)
	_ = g.Update()
	restore()
	if g.grid.MaxDiv() != targetVal {
		t.Fatalf("grid maxdiv=%d want %d", g.grid.MaxDiv(), targetVal)
	}
	if g.drum.timelineUnitsPerBeat != targetVal {
		t.Fatalf("timelineUnits=%d want %d", g.drum.timelineUnitsPerBeat, targetVal)
	}
}
