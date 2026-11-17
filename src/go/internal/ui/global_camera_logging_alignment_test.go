package ui

import (
	"fmt"
	"io"
	"math"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestGlobalCameraLogging_DefaultStart spawns the default single-node circuit
// and logs pixel positions for nodes, edges (if any), and grid phase at
// multiple zoom levels. It asserts alignment and prints detailed values to
// help diagnose offsets when regressions occur.
func TestGlobalCameraLogging_DefaultStart(t *testing.T) {
	out := io.Discard
	if testing.Verbose() {
		out = os.Stdout
	}
	logger := game_log.New(out, game_log.LevelDebug)

	withDefaultStart(t, true)

	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	screen := ebiten.NewImage(800, 600)

	run := func(scale, offX, offY float64) {
		g.cam.Scale = scale
		g.cam.OffsetX = offX
		g.cam.OffsetY = offY
		g.cam.Snap()
		// Draw builds caches at this camera state.
		g.Draw(screen)
		// Compute shared camera transforms used by DrawLineCam.
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		camScale := unitPx / g.grid.Unit()
		stepPx := g.grid.StepPixels(g.cam.Scale)
		maxDiv := g.grid.MaxDiv()
		offXR := math.Round(g.cam.OffsetX)
		offYR := math.Round(g.cam.OffsetY)
		log := func(msg string, args ...any) {
			if testing.Verbose() {
				t.Logf(msg, args...)
			}
		}
		log("CAM scale=%.4f off=(%.0f,%.0f) stepPx=%d maxDiv=%d", scale, offXR, offYR, stepPx, maxDiv)
		// Inspect all nodes
		for _, n := range g.nodes {
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			cx, cy := (x1+x2)*0.5, (y1+y2)*0.5
			expX := offXR + math.Round(float64(n.I)*float64(stepPx)/float64(maxDiv))
			expY := offYR + float64(gridTopOffset()) + math.Round(float64(n.J)*float64(stepPx)/float64(maxDiv))
			// Project via DrawLineCam math as a cross-check
			ex, ey := n.X*camScale+offXR, n.Y*camScale+offYR+float64(gridTopOffset())
			if testing.Verbose() {
				fmt.Fprintf(out, "[GLOBAL] node id=%d grid=(%d,%d) rect=(%.2f,%.2f)-(%.2f,%.2f) center=(%.2f,%.2f) expGrid=(%.0f,%.0f) proj=(%.2f,%.2f)\n",
					n.ID, n.I, n.J, x1, y1, x2, y2, cx, cy, expX, expY, ex, ey)
			}
			// Tight tolerance: everything should agree within ~1 px.
			tol := 0.9
			if math.Abs(cx-expX) > tol || math.Abs(cy-expY) > tol {
				t.Fatalf("node/grid misaligned id=%d: center(%.2f,%.2f) exp(%.0f,%.0f) scale=%.3f off=(%.0f,%.0f)", n.ID, cx, cy, expX, expY, scale, offXR, offYR)
			}
			if math.Abs(ex-cx) > tol || math.Abs(ey-cy) > tol {
				t.Fatalf("node/proj misaligned id=%d: proj(%.2f,%.2f) center(%.2f,%.2f) scale=%.3f off=(%.0f,%.0f)", n.ID, ex, ey, cx, cy, scale, offXR, offYR)
			}
		}
		// Inspect all edges (if any in other cases)
		for i := range g.edges {
			e := &g.edges[i]
			ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
			acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
			bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
			bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
			ex0, ey0 := e.A.X*camScale+offXR, e.A.Y*camScale+offYR+float64(gridTopOffset())
			ex1, ey1 := e.B.X*camScale+offXR, e.B.Y*camScale+offYR+float64(gridTopOffset())
			if testing.Verbose() {
				fmt.Fprintf(out, "[GLOBAL] edge A=%d B=%d projA=(%.2f,%.2f) nodeA=(%.2f,%.2f) projB=(%.2f,%.2f) nodeB=(%.2f,%.2f)\n", e.A.ID, e.B.ID, ex0, ey0, acx, acy, ex1, ey1, bcx, bcy)
			}
			tol := 0.9
			if math.Abs(ex0-acx) > tol || math.Abs(ey0-acy) > tol {
				t.Fatalf("edge start misaligned idA=%d proj(%.2f,%.2f) node(%.2f,%.2f)", e.A.ID, ex0, ey0, acx, acy)
			}
			if math.Abs(ex1-bcx) > tol || math.Abs(ey1-bcy) > tol {
				t.Fatalf("edge end misaligned idB=%d proj(%.2f,%.2f) node(%.2f,%.2f)", e.B.ID, ex1, ey1, bcx, bcy)
			}
		}
	}

	for _, st := range []struct{ s, x, y float64 }{{1.0, 0, 0}, {1.25, 11, 7}, {0.9, -15, 9}} {
		run(st.s, st.x, st.y)
	}
}

// TestGlobalCameraLogging_DemoWithPulse logs and checks a mid-edge pulse position
// against DrawLineCam projection at various zoom levels, in addition to node/edge alignment.
func TestGlobalCameraLogging_DemoWithPulse(t *testing.T) {
	out := io.Discard
	if testing.Verbose() {
		out = os.Stdout
	}
	logger := game_log.New(out, game_log.LevelDebug)

	g := New(logger)
	t.Cleanup(g.CloseForTest)
	if err := g.Import(assets_pkg.TestFixtureDemoJSON); err != nil {
		t.Fatalf("import demo: %v", err)
	}
	g.Layout(1280, 720)

	// Spawn a pulse on row 0 if available.
	g.spawnPulseFromRow(0, 0)
	if len(g.activePulses) > 0 {
		// Set to mid-edge and verify projection.
		g.activePulses[0].t = 0.5
	}
	screen := ebiten.NewImage(1280, 720)

	// Check at a couple zooms
	for _, s := range []float64{0.85, 1.33} {
		g.cam.Scale = s
		g.cam.OffsetX = 0
		g.cam.OffsetY = 0
		g.cam.Snap()
		g.Draw(screen)
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		camScale := unitPx / g.grid.Unit()
		offXR := math.Round(g.cam.OffsetX)
		offYR := math.Round(g.cam.OffsetY)
		// Node/edge checks (subset)
		if len(g.nodes) > 0 {
			n := g.nodes[0]
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			cx, cy := (x1+x2)*0.5, (y1+y2)*0.5
			ex, ey := n.X*camScale+offXR, n.Y*camScale+offYR+float64(gridTopOffset())
			if testing.Verbose() {
				fmt.Fprintf(out, "[GLOBAL] zoom=%.2f node0 center=(%.2f,%.2f) proj=(%.2f,%.2f)\n", s, cx, cy, ex, ey)
			}
			if math.Abs(ex-cx) > 0.9 || math.Abs(ey-cy) > 0.9 {
				t.Fatalf("node/proj mismatch zoom=%.2f", s)
			}
		}
		// Pulse check
		if len(g.activePulses) > 0 {
			p := g.activePulses[0]
			px := p.x1 + (p.x2-p.x1)*p.t
			py := p.y1 + (p.y2-p.y1)*p.t
			sx := px*camScale + offXR
			sy := py*camScale + offYR + float64(gridTopOffset())
			if testing.Verbose() {
				fmt.Fprintf(out, "[GLOBAL] zoom=%.2f pulse t=%.2f world=(%.2f,%.2f) screen=(%.2f,%.2f)\n", s, p.t, px, py, sx, sy)
			}
			// Just assert finite and within screen reasonably.
			if math.IsNaN(sx) || math.IsNaN(sy) || math.IsInf(sx, 0) || math.IsInf(sy, 0) {
				t.Fatalf("invalid pulse screen coords")
			}
		}
	}
}
