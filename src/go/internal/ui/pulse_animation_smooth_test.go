package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Build a simple 1-beat edge for row 0.
func makeSimpleEdge(g *Game) {
	g.pendingStartRow = 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.pendingStartRow = -1
	g.updateBeatInfos()
}

// Test that pulse animation progress is smooth (monotonic and bounded) after changing subdiv.
func TestPulseAnimationSmoothAfterSubdivChange(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Simple edge and BPM
	makeSimpleEdge(g)
	g.drum.SetBPM(120)
	// Stop playback; change subdiv to 8
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set subdiv: %v", err)
	}
	// Start playback and spawn a pulse
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	// Simulate frames and record progress
	var last float64 = -1
	for i := 0; i < 60; i++ {
		setPlayStartForAbs(g, i+1)
		_ = g.Update()
		if g.activePulse == nil {
			continue
		}
		tval := g.activePulse.t
		if last >= 0 {
			// Allow wrap-around at segment boundaries
			if !(last > 0.9 && tval < 0.2) {
				if tval+1e-6 < last {
					t.Fatalf("non-monotonic t: prev=%.4f now=%.4f", last, tval)
				}
				// bounded increment to catch jitter; expect < 0.5 per 5ms at 120 BPM
				if (tval - last) > 0.4 {
					t.Fatalf("excessive t step: prev=%.4f now=%.4f", last, tval)
				}
			}
		}
		last = tval
	}
}

// Ensure Subdiv button draws and can be clicked without affecting animation smoothness.
func TestSubdivButtonDrawAndClickDoesNotStall(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dst := ebiten.NewImage(640, 200)
	dv.Draw(dst, nil, 0, nil, 0)
	r := dv.subdivBtn().Rect()
	if r.Empty() {
		t.Fatalf("subdiv button not laid out")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return true }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 640, 200 })
	dv.Update()
	t.Cleanup(restore)
	restore()
	if !dv.IsSubdivMenuOpen() {
		t.Fatalf("subdiv menu did not open on click")
	}
}
