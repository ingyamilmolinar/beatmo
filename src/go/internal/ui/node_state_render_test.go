//go:build test

package ui

import (
	"image"
	"image/color"
	"sort"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// nodeColorSignature collects the sorted set of distinct filled-rect colors
// whose rects contain (or closely surround) the node center. The signature is
// used to prove that Regular / Muted / Silent / Invisible nodes render
// distinctly — pixel-diff verified via the drawRect interceptor.
func nodeColorSignature(rects []drawnRect, cx, cy int) string {
	seen := map[color.RGBA]bool{}
	// Sample the node center plus a small neighborhood so border / slash /
	// dashed-outline pixels (a few px off-center) are captured too.
	for dx := -6; dx <= 6; dx += 2 {
		for dy := -6; dy <= 6; dy += 2 {
			pt := image.Pt(cx+dx, cy+dy)
			for _, r := range rects {
				if pt.In(r.Rect) {
					seen[r.Color] = true
				}
			}
		}
	}
	cols := make([]string, 0, len(seen))
	for c := range seen {
		cols = append(cols, string([]byte{c.R, c.G, c.B, c.A}))
	}
	sort.Strings(cols)
	sig := ""
	for _, c := range cols {
		sig += c
	}
	return sig
}

// TestNodeStateRenderingDistinct verifies that a node in each of the four
// states (Regular, Muted, Silent, Invisible) renders with a distinct set of
// drawn colors at its position. Uses RENDER_SAFE so node bodies + the
// state overlays both flow through drawRect and are interceptable.
func TestNodeStateRenderingDistinct(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("RENDER_SAFE", "1")
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = false })

	states := []struct {
		name string
		typ  model.NodeType
		gi   int
	}{
		{"regular", model.NodeTypeRegular, 0},
		{"muted", model.NodeTypeMute, 4},
		{"silent", model.NodeTypeSilent, 8},
		{"invisible", model.NodeTypeInvisible, 12},
	}

	sigs := make(map[string]string)
	for _, st := range states {
		st := st
		t.Run(st.name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(640, 480)
			g.cam.Scale = 1.0
			centerCameraOn(g, st.gi, 2)

			n := g.tryAddNode(st.gi, 2, st.typ)
			if n == nil {
				t.Fatalf("%s: tryAddNode returned nil", st.name)
			}
			g.updateBeatInfos()

			cx, cy := nodeCenter(g, n)
			if cy >= g.split.Y || cy < gridTopOffset() || cx < 0 || cx >= 640 {
				t.Fatalf("%s: node center (%d,%d) outside grid pane", st.name, cx, cy)
			}

			screen := ebiten.NewImage(640, 480)
			rects := collectFilledRects(t, func() {
				g.Draw(screen)
			})
			sig := nodeColorSignature(rects, cx, cy)
			if sig == "" {
				t.Fatalf("%s: no filled rects at node center (%d,%d)", st.name, cx, cy)
			}
			sigs[st.name] = sig
		})
	}

	// All four signatures must be pairwise distinct.
	if t.Failed() {
		return
	}
	names := []string{"regular", "muted", "silent", "invisible"}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			a, b := names[i], names[j]
			if sigs[a] == sigs[b] {
				t.Errorf("node states %q and %q render with identical color signatures (expected distinct)", a, b)
			}
		}
	}
}

// TestSelectedNodeHasHalo verifies the selected node gains a soft-glow halo
// (WithAlpha(primary, AlphaSubtle)) that an unselected node lacks. Drawn via
// drawRect in the RENDER_SAFE selection path.
func TestSelectedNodeHasHalo(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	t.Setenv("RENDER_SAFE", "1")
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.cam.Scale = 1.0
	centerCameraOn(g, 0, 0)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("tryAddNode returned nil")
	}
	g.updateBeatInfos()
	cx, cy := nodeCenter(g, n)

	haloCol := color.RGBAModel.Convert(WithAlpha(colAccent, AlphaSubtle)).(color.RGBA)
	countHalo := func() int {
		screen := ebiten.NewImage(640, 480)
		rects := collectFilledRects(t, func() { g.Draw(screen) })
		cnt := 0
		nodeRegion := image.Rect(cx-24, cy-24, cx+24, cy+24)
		for _, r := range rects {
			if r.Color != haloCol {
				continue
			}
			// Halo bands sit in a ring just outside the node body.
			if r.Rect.Overlaps(nodeRegion) {
				cnt++
			}
		}
		return cnt
	}

	// Unselected: no halo.
	g.sel = nil
	if got := countHalo(); got != 0 {
		t.Fatalf("unselected node drew %d halo rects, want 0", got)
	}

	// Selected: halo appears.
	g.sel = n
	if got := countHalo(); got == 0 {
		t.Errorf("selected node drew no halo rects, want >0 (color=%v)", haloCol)
	}
}
