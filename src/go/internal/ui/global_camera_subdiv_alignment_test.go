package ui

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// TestGlobalCameraAlignment_Subdivisions verifies that after changing grid
// subdivisions, nodes, edges, and the grid remain pixel-aligned from the
// user's perspective across a few zoom/pan states.
func TestGlobalCameraAlignment_Subdivisions(t *testing.T) {
	// Use embedded default demo which is aligned on multiples of 16 so we can
	// safely reduce subdivisions.
	demoPath := "internal/assets/default_demo.json"
	if _, err := os.Stat(demoPath); err != nil {
		alt := "../assets/default_demo.json"
		if _, err2 := os.Stat(alt); err2 == nil {
			demoPath = alt
		}
	}
	os.Setenv("TUNKUL_DEMO_CONFIG", demoPath)

	// Enable draw logs for easier debugging if this regresses.
	os.Setenv("DEBUG_DRAW_NODES", "1")
	logger := game_log.New(os.Stdout, game_log.LevelDebug)

	g := New(logger)
	g.Layout(1280, 720)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built; check TUNKUL_DEMO_CONFIG path")
	}

	// Helper that checks alignment at current camera state.
	screen := ebiten.NewImage(1280, 720)
	checkAligned := func(note string) {
		g.Draw(screen) // build caches at this state
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		camScale := unitPx / g.grid.Unit()
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		stepPx := g.grid.StepPixels(g.cam.Scale)
		maxDiv := g.grid.MaxDiv()
		tol := 0.9
		// Nodes against grid and projection
		for _, n := range g.nodes {
			mn, ok := g.graph.GetNodeByID(n.ID)
			if !ok || mn.Type == 1 {
				continue
			}
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			cx, cy := (x1+x2)*0.5, (y1+y2)*0.5
			expX := offX + math.Round(float64(n.I)*float64(stepPx)/float64(maxDiv))
			expY := offY + float64(topOffset) + math.Round(float64(n.J)*float64(stepPx)/float64(maxDiv))
			if math.Abs(cx-expX) > tol || math.Abs(cy-expY) > tol {
				t.Fatalf("%s: node/grid mismatch id=%d got(%.2f,%.2f) exp(%.0f,%.0f)", note, n.ID, cx, cy, expX, expY)
			}
			ex, ey := n.X*camScale+offX, n.Y*camScale+offY+float64(topOffset)
			if math.Abs(ex-cx) > tol || math.Abs(ey-cy) > tol {
				t.Fatalf("%s: node/proj mismatch id=%d proj(%.2f,%.2f) ctr(%.2f,%.2f)", note, n.ID, ex, ey, cx, cy)
			}
		}
		// Edge endpoints against node centers
		for i := range g.edges {
			e := &g.edges[i]
			ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
			acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
			bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
			bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
			ex0, ey0 := e.A.X*camScale+offX, e.A.Y*camScale+offY+float64(topOffset)
			ex1, ey1 := e.B.X*camScale+offX, e.B.Y*camScale+offY+float64(topOffset)
			if math.Abs(ex0-acx) > tol || math.Abs(ey0-acy) > tol {
				t.Fatalf("%s: edge start mismatch idA=%d", note, e.A.ID)
			}
			if math.Abs(ex1-bcx) > tol || math.Abs(ey1-bcy) > tol {
				t.Fatalf("%s: edge end mismatch idB=%d", note, e.B.ID)
			}
		}
	}

	// Check across subdivision changes
	states := []struct {
		subdiv int
		scale  float64
		offX   float64
		offY   float64
	}{
		{32, 1.00, 0, 0},
		{16, 1.25, 7, 11},
		{8, 0.90, -13, 5},
		{32, 1.66, -33, 21},
	}
	for _, st := range states {
		if err := g.SetSubdivisions(st.subdiv); err != nil {
			t.Fatalf("SetSubdivisions(%d) failed: %v", st.subdiv, err)
		}
		g.cam.Scale = st.scale
		g.cam.OffsetX += st.offX
		g.cam.OffsetY += st.offY
		g.cam.Snap()
		checkAligned(
			fmt.Sprintf("subdiv=%d scale=%.2f off=(%.0f,%.0f)", st.subdiv, g.cam.Scale, math.Round(g.cam.OffsetX), math.Round(g.cam.OffsetY)),
		)
	}
}
