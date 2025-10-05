package ui

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestGlobalCameraPixelSnap verifies that the actually drawn node center
// (rounded to the nearest screen pixel) coincides with the grid lattice and
// with rounded world→screen projections used for edges/pulses.
func TestGlobalCameraPixelSnap(t *testing.T) {
	// Use the embedded default demo
	demoPath := "internal/assets/default_demo.json"
	if _, err := os.Stat(demoPath); err != nil {
		alt := "../assets/default_demo.json"
		if _, err2 := os.Stat(alt); err2 == nil {
			demoPath = alt
		}
	}
	os.Setenv("TUNKUL_DEMO_CONFIG", demoPath)
	os.Setenv("DEBUG_DRAW_NODES", "1")
	logger := game_log.New(os.Stdout, game_log.LevelDebug)

	g := New(logger)
	g.Layout(1280, 720)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built")
	}
	screen := ebiten.NewImage(1280, 720)

	check := func(note string) {
		g.Draw(screen)
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		stepPx := g.grid.StepPixels(g.cam.Scale)
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		maxDiv := g.grid.MaxDiv()
		camScale := unitPx / g.grid.Unit()
		for _, n := range g.nodes {
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			cx := (x1 + x2) * 0.5
			cy := (y1 + y2) * 0.5
			// Actual drawn center snaps to integer pixels per game.go
			dcx := math.Round(cx)
			dcy := math.Round(cy)
			// Grid lattice integer pixels
			gx := offX + math.Round(float64(n.I)*float64(stepPx)/float64(maxDiv))
			gy := offY + float64(topOffset) + math.Round(float64(n.J)*float64(stepPx)/float64(maxDiv))
			// Edge/pulse projection rounding
			ex := math.Round(n.X*camScale + offX)
			ey := math.Round(n.Y*camScale + offY + float64(topOffset))
			fmt.Fprintf(os.Stdout, "[PIX] %s id=%d grid=(%d,%d) dc=(%.0f,%.0f) grid=(%.0f,%.0f) edge=(%.0f,%.0f)\n", note, n.ID, n.I, n.J, dcx, dcy, gx, gy, ex, ey)
			if dcx != gx || dcy != gy {
				t.Fatalf("%s: drawn center != grid lattice: got(%.0f,%.0f) want(%.0f,%.0f)", note, dcx, dcy, gx, gy)
			}
			if dcx != ex || dcy != ey {
				t.Fatalf("%s: drawn center != rounded world→screen: got(%.0f,%.0f) want(%.0f,%.0f)", note, dcx, dcy, ex, ey)
			}
		}
	}
	for _, s := range []float64{1.0, 1.33, 0.85} {
		g.cam.Scale = s
		g.cam.Snap()
		check(fmt.Sprintf("scale=%.2f", s))
	}
}
