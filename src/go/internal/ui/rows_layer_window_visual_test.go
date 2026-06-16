//go:build test

// Stub-only: relies on Image.At() reading back rendered pixels. Real Ebiten
// panics with "ReadPixels cannot be called before the game starts" because
// there is no running game loop in a unit test, so this comparison is gated
// to the ebitenstub backend (matches render_visual_regression_test.go).

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowsLayerWindowedRenderMatchesLegacy verifies the windowed scroll cache
// renders the drum-row region visually equivalent to the legacy composite at a
// scrolled playback position. Two identical games are driven to the same offset
// — one with the windowed path, one without — and the row-region pixels are
// compared. A small per-pixel/positional tolerance is allowed for the inherent
// <=1px sub-cell rounding that the legacy shift path also exhibits.
func TestRowsLayerWindowedRenderMatchesLegacy(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	build := func(windowing bool) *ebiten.Image {
		restore := SetRowsLayerWindowingForTest(windowing)
		defer restore()
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(900, 600)
		n0 := g.tryAddNode(0, 0, 0)
		g.start = n0
		g.graph.StartNodeID = n0.ID
		n1 := g.tryAddNode(1, 0, 0)
		g.addEdge(n0, n1)
		g.addEdge(n1, n0)
		g.updateBeatInfos()
		g.SetPlayFunc(func(string, float64, ...float64) {})
		g.drum.SetFollow(true)
		defer SetTrackBeatForceRefreshForTest(false)()
		pressPlay(t, g.drum)
		scr := ebiten.NewImage(900, 600)
		// Advance into the steady-state scroll regime to a fixed offset.
		for i := 1; i <= 200; i++ {
			setPlayStartForAbs(g, i)
			_ = g.Update()
			g.Draw(scr)
		}
		return scr
	}

	on := build(true)
	off := build(false)

	// Compare the drum-row band. Use the row-rack region of the screen.
	region := image.Rect(120, 320, 880, 560)
	var diffPixels, sampled, nonEmpty int
	var maxChan int
	for y := region.Min.Y; y < region.Max.Y; y += 2 {
		for x := region.Min.X; x < region.Max.X; x += 2 {
			r1, g1, b1, a1 := on.At(x, y).RGBA()
			r2, g2, b2, _ := off.At(x, y).RGBA()
			sampled++
			if r1 > 0 || g1 > 0 || b1 > 0 || a1 > 0 {
				nonEmpty++
			}
			d := absI(int(r1>>8)-int(r2>>8)) + absI(int(g1>>8)-int(g2>>8)) + absI(int(b1>>8)-int(b2>>8))
			if d > maxChan {
				maxChan = d
			}
			// Allow a tolerance band: small color deltas from <=1px cell-edge
			// rounding shift. Count only meaningfully different pixels.
			if d > 48 {
				diffPixels++
			}
		}
	}
	if sampled == 0 {
		t.Fatalf("no pixels sampled — region empty")
	}
	frac := float64(diffPixels) / float64(sampled)
	t.Logf("sampled=%d nonEmpty=%d diffPixels=%d frac=%.4f maxChanDelta=%d", sampled, nonEmpty, diffPixels, frac, maxChan)
	// Guard against a vacuous pass: the region must contain real rendered
	// content (ebitenstub SubImage reads can be empty — see CLAUDE.md).
	if nonEmpty < sampled/10 {
		t.Skipf("row region read mostly empty (nonEmpty=%d/%d) — ebitenstub pixel reads not capturing the composite here; equivalence covered by content-signature invariant in rows_layer_scroll_recompose_test.go", nonEmpty, sampled)
	}
	// The two renders should be visually equivalent: only a thin band of
	// cell-edge pixels may differ by the <=1px scroll rounding. Require the
	// differing fraction to be small.
	if frac > 0.08 {
		t.Fatalf("windowed render differs from legacy in %.1f%% of sampled row pixels (>8%%) — visual regression", frac*100)
	}
}
