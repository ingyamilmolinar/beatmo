package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawDivider renders the grid↔drum splitter — an azure "horizon line" plus a
// pill that highlights on hover to make it discoverable as draggable. The
// line+pill draw and the glow+grow hover animation are owned by the shared
// SplitterHandle (also used by the EQ-boundary divider).
func (g *Game) drawDivider(screen *ebiten.Image) {
	// The divider handle is drawn last (above both panes), so suppress it while
	// a full-screen modal overlay (e.g. the settings overlay) is up — the
	// pill must not poke through the overlay's scrim.
	if g.drum != nil && g.drum.tree != nil && g.drum.tree.Portal() != nil &&
		g.drum.tree.Portal().Has(settingsOverlayID) {
		return
	}
	mX, mY := cursorPosition()
	// Hover only when the cursor is on the visible pill handle (plus the
	// standard SpaceSM forgiveness), NOT anywhere along the full-width divider
	// line. The bare line is not draggable, so lighting it up on a mere Y/X
	// crossing was an invasive affordance.
	//
	// Build the pill rect from g.winW/g.winH — the exact coordinates the pill is
	// DRAWN at below (DrawHorizontalDivider centres it at (g.winW/2, g.split.Y);
	// the vertical form at (g.split.X, g.winH/2)). HandleRect() derives the
	// centre from s.winW/s.totalH instead, which only equals g.winW/g.winH after
	// UpdateResize has run this frame — so using the draw coordinates keeps the
	// glow aligned with the pixels regardless of update ordering.
	var pill image.Rectangle
	if g.split.horizontal {
		pill = SplitterHandleRect(g.winW/2, g.split.Y, true)
	} else {
		pill = SplitterHandleRect(g.split.X, g.winH/2, false)
	}
	hover := image.Pt(mX, mY).In(pill.Inset(-SpaceSM))
	// dividerHover/dividerThick remain published for the highlight regression
	// test; the visual thickness is carried by the handle's bloom + pill.
	g.dividerHover = hover
	g.dividerThick = 2.0
	if hover {
		g.dividerThick = 3.0
	}
	g.split.handle.Advance(hover)
	if g.split.horizontal {
		g.split.handle.DrawHorizontalDivider(screen, 0, g.winW, g.split.Y)
	} else {
		g.split.handle.DrawVerticalDivider(screen, 0, g.winH, g.split.X)
	}
}

// drawCrossScreen paints a simple cross at (x,y) in screen pixels for diagnostics.
func drawCrossScreen(dst *ebiten.Image, x, y, size int, col color.Color) {
	half := size / 2
	// horizontal line
	r1 := image.Rect(x-half, y, x+half+1, y+1)
	drawRect(dst, r1, col, true)
	// vertical line
	r2 := image.Rect(x, y-half, x+1, y+half+1)
	drawRect(dst, r2, col, true)
}

func idOrNil(n *uiNode) any {
	if n == nil {
		return nil
	}
	return n.ID
}

// buildGridTile creates a stepPx×stepPx image that contains all visible grid
// subdivision lines for the current grid configuration. The tile can be
// repeated across the grid pane by translating it according to camera offset.
// gridTileMinBlockPx is the minimum physical size of the grid tile. The grid
// cache is built by tiling this image; tiling a tile smaller than this would
// blow up the per-rebuild blit count (~area/tileW²) when zoomed out. 64px caps a
// full-screen rebuild at a few hundred blits across the whole zoom range.
const gridTileMinBlockPx = 64

func (g *Game) buildGridTile(stepPx int) *ebiten.Image {
	if stepPx <= 0 {
		return nil
	}
	// Multi-cell block tile. Tiling a stepPx-sized tile across the screen costs
	// ~area/stepPx² DrawImage blits — quadratic as the camera zooms OUT (stepPx
	// shrinks): tens of thousands of blits per frame at full zoom-out, which
	// blocks Draw on the single WASM thread long enough to starve the sequencer
	// goroutine (audio degrades, worse the further out). We build a block of
	// `cells` grid cells so the physical tile is at least gridTileMinBlockPx wide;
	// the caller tiles at this block period, capping the blit count at
	// ~area/gridTileMinBlockPx² regardless of zoom. The rendered lattice is
	// pixel-identical (the block is the same periodic lattice, pre-repeated).
	cells := 1
	if stepPx < gridTileMinBlockPx {
		cells = (gridTileMinBlockPx + stepPx - 1) / stepPx // ceil
	}
	tileW := cells * stepPx
	g.gridTileW = tileW
	// Grow-only backing texture: reuse the existing backing when it is large
	// enough (Clear()+redraw), growing to the next power of two ≥ tileW. A
	// continuous zoom changes tileW; reusing the backing keeps per-frame
	// allocations at zero (atlas churn on the single WASM thread also starves
	// audio). The backing may be larger than tileW with a transparent margin; the
	// caller tiles at step = tileW so the margin overlaps the neighbouring tile
	// and alpha-blends to a no-op (pixel-identical to a freshly-sized tile).
	if g.gridTileBacking == nil || g.gridTileBackingSize < tileW {
		sz := 1
		for sz < tileW {
			sz <<= 1
		}
		releaseImage(g.gridTileBacking)
		g.gridTileBacking = newTrackedImage("gridTile", sz, sz)
		g.gridTileBackingSize = sz
	} else {
		// Clear the whole backing so stale pixels from a previous (larger) tile
		// don't bleed through the transparent margin of the new one.
		g.gridTileBacking.Clear()
	}
	img := g.gridTileBacking
	if g.logDrawNodes {
		g.logger.Tracef("[DRAW/GRID] buildGridTile: stepPx=%d cells=%d tileW=%d subs=%d backing=%d", stepPx, cells, tileW, len(g.grid.Subs), g.gridTileBackingSize)
	}
	// Draw the lattice across [0,tileW)²: lines repeat with period stepPx, each
	// spanning the full block extent so tiling the block is seamless. Line
	// positions use the same per-cell rounding as a single-cell tile —
	// round(j·stepPx/div) == m·stepPx + round(k·stepPx/div) for j = m·div+k — so
	// the result is pixel-identical to the old stepPx tiling.
	for _, sub := range g.grid.Subs {
		// Minimum pixel spacing: use rounded per-line positions rather than
		// integer division so rounding error is evenly distributed and aligns
		// with world-to-screen math.
		minPx := float64(stepPx) / float64(sub.Div)
		if minPx < float64(sub.MinPx) {
			continue
		}
		// Thickness in px; clamp to at least 1.
		t := sub.Style.Width
		if t < 1 {
			t = 1
		}
		thick := int(math.Round(t))
		if thick < 1 {
			thick = 1
		}
		nLines := cells * sub.Div
		// Vertical lines (span the full block height).
		for j := 0; j < nLines; j++ {
			x := int(math.Round(float64(j) * float64(stepPx) / float64(sub.Div)))
			if x >= tileW {
				continue
			}
			var op ebiten.DrawImageOptions
			op.GeoM.Scale(1, float64(tileW))
			op.GeoM.Translate(float64(x), 0)
			// Expand thickness by drawing additional pixels to the right.
			for dx := 0; dx < thick; dx++ {
				op2 := op
				op2.GeoM.Translate(float64(dx), 0)
				img.DrawImage(pixel(sub.Style.Color), &op2)
			}
		}
		// Horizontal lines (span the full block width).
		for j := 0; j < nLines; j++ {
			y := int(math.Round(float64(j) * float64(stepPx) / float64(sub.Div)))
			if y >= tileW {
				continue
			}
			var op ebiten.DrawImageOptions
			op.GeoM.Scale(float64(tileW), 1)
			op.GeoM.Translate(0, float64(y))
			for dy := 0; dy < thick; dy++ {
				op2 := op
				op2.GeoM.Translate(0, float64(dy))
				img.DrawImage(pixel(sub.Style.Color), &op2)
			}
		}
	}
	return img
}

func (g *Game) drawDrumPane(dst *ebiten.Image) {
	// Keep DrumView's seconds-per-beat in sync with the engine/app-lied BPM for
	// timeline counters without overriding the user-edited BPM control value.
	// This avoids a race where UI changes are undone by the draw loop before
	// Game.Update() can propagate them to the engine.
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm > 0 {
		g.drum.secPerBeat = 60.0 / float64(bpm)
	}
	// Use a smooth, non-quantized beat value for timeline counters to avoid
	// jitter in the displayed timers, while highlights and steps remain
	// quantized via internal counters.
	g.drum.simpleDraw = g.simpleDraw
	g.drum.perfDrawLite = false // EQ visualizer always enabled - already optimized with early exits
	g.drum.Draw(dst, g.highlightSnapshotByRow(len(g.drum.Rows)), g.frame, g.drumBeatInfos, g.displayBeat())
}

func (g *Game) maybeYield() {
	if g == nil {
		return
	}
	if !RuntimeProf().YieldInDrawHelpers {
		return
	}
	if !g.perfMode.FastPathEnabled() {
		return
	}
	runtime.Gosched()
}
