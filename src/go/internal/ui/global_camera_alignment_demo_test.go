package ui

import (
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestGlobalCameraAlignment_DefaultDemo loads the default demo via the test
// buildDemo path and verifies, at the global camera view level, that:
//   - Every visible node center is phase-aligned with the grid pixel lattice
//   - Every edge endpoint projects to the exact same screen pixel as its node
//   - After an initial Draw pass (which builds caches), alignment still holds
func TestGlobalCameraAlignment_DefaultDemo(t *testing.T) {
	// Point buildDemo to the embedded default demo JSON.
	// Tests run under src/go; path below resolves from there.
	// Resolve demo path relative to current working directory (package folder).
	demoPath := "internal/assets/default_demo.json"
	if _, err := os.Stat(demoPath); err != nil {
		// When running from package directory ./internal/ui, adjust path.
		alt := "../assets/default_demo.json"
		if _, err2 := os.Stat(alt); err2 == nil {
			demoPath = alt
		}
	}
	old := os.Getenv("TUNKUL_DEMO_CONFIG")
	os.Setenv("TUNKUL_DEMO_CONFIG", demoPath)
	defer os.Setenv("TUNKUL_DEMO_CONFIG", old)

	// Use the demo instead of auto default-start node.
	SetDefaultStartForTest(false)
	defer SetDefaultStartForTest(false)

	g := New(testLogger)
	g.Layout(1280, 720)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built; check TUNKUL_DEMO_CONFIG path")
	}

	screen := ebiten.NewImage(1280, 720)
	checkAll := func() {
		// Draw once to rebuild caches at the current camera state
		g.Draw(screen)
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		camScale := unitPx / g.grid.Unit()
		stepPx := g.grid.StepPixels(g.cam.Scale)
		maxDiv := g.grid.MaxDiv()
		tol := 0.75
		proj := func(wx, wy float64) (sx, sy float64) {
			sx = wx*camScale + offX
			sy = wy*camScale + offY + float64(topOffset)
			return
		}
		for _, n := range g.nodes {
			mn, ok := g.graph.GetNodeByID(n.ID)
			if !ok || mn.Type == 1 { // invisible
				continue
			}
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			cx, cy := (x1+x2)*0.5, (y1+y2)*0.5
			expX := offX + math.Round(float64(n.I)*float64(stepPx)/float64(maxDiv))
			expY := offY + float64(topOffset) + math.Round(float64(n.J)*float64(stepPx)/float64(maxDiv))
			if math.Abs(cx-expX) > tol || math.Abs(cy-expY) > tol {
				t.Fatalf("node/grid phase mismatch at id=%d grid=(%d,%d): got(%.2f,%.2f) exp(%.0f,%.0f)", n.ID, n.I, n.J, cx, cy, expX, expY)
			}
		}
		for i := range g.edges {
			e := &g.edges[i]
			ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
			acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
			bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
			bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
			ex0, ey0 := proj(e.A.X, e.A.Y)
			ex1, ey1 := proj(e.B.X, e.B.Y)
			if math.Abs(ex0-acx) > tol || math.Abs(ey0-acy) > tol {
				t.Fatalf("edge start mismatch idA=%d: edge(%.2f,%.2f) node(%.2f,%.2f)", e.A.ID, ex0, ey0, acx, acy)
			}
			if math.Abs(ex1-bcx) > tol || math.Abs(ey1-bcy) > tol {
				t.Fatalf("edge end mismatch idB=%d: edge(%.2f,%.2f) node(%.2f,%.2f)", e.B.ID, ex1, ey1, bcx, bcy)
			}
		}
	}
	// Initial camera
	checkAll()
	// Some zoom and pan states at the global view level
	for _, st := range []struct{ s, x, y float64 }{
		{0.85, 0, 0}, {1.25, 15, 9}, {1.66, -33, 21},
	} {
		g.cam.Scale = st.s
		g.cam.OffsetX += st.x
		g.cam.OffsetY += st.y
		g.cam.Snap()
		checkAll()
	}
}

// no extra helpers
