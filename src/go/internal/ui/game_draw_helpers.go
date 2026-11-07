package ui

import (
	"image"
	"image/color"
	"math"
	"runtime"

	"github.com/hajimehoshi/ebiten/v2"
)

func (g *Game) drawDivider(screen *ebiten.Image) {
	// Drawn above both panes, so it must hide while a full-screen modal is up or it pokes
	// through the scrim.
	if g.drum != nil && g.drum.tree != nil && g.drum.portal() != nil &&
		g.drum.portal().Has(settingsOverlayID) {
		return
	}
	mX, mY := cursorPosition()
	// Hover only on the visible pill: the bare line is not draggable. Built from the draw
	// coordinates (g.winW/g.winH), which are current regardless of update order.
	var pill image.Rectangle
	if g.split.horizontal {
		pill = SplitterHandleRect(g.winW/2, g.split.Y, true)
	} else {
		pill = SplitterHandleRect(g.split.X, g.winH/2, false)
	}
	hover := image.Pt(mX, mY).In(pill.Inset(-SpaceSM))
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

// Tiling a tile smaller than this makes a rebuild cost ~area/tileW² blits when zoomed out;
// 64 px keeps it to a few hundred.
const gridTileMinBlockPx = 64

func (g *Game) buildGridTile(stepPx int) *ebiten.Image {
	if stepPx <= 0 {
		return nil
	}
	// A block of several cells, so the blit count stays ~area/gridTileMinBlockPx² at any zoom;
	// per-cell tiling when zoomed out blocks the WASM thread and starves the sequencer.
	cells := 1
	if stepPx < gridTileMinBlockPx {
		cells = (gridTileMinBlockPx + stepPx - 1) / stepPx // ceil
	}
	tileW := cells * stepPx
	g.gridTileW = tileW
	// Grow-only backing: a continuous zoom changes tileW every frame, and reallocating churns the
	// atlas. The transparent margin overlaps the neighbour and blends to a no-op.
	if g.gridTileBacking == nil || g.gridTileBackingSize < tileW {
		sz := 1
		for sz < tileW {
			sz <<= 1
		}
		releaseImage(g.gridTileBacking)
		g.gridTileBacking = newTrackedImage("gridTile", sz, sz)
		g.gridTileBackingSize = sz
	} else {
		// Clear it all, or a previous larger tile bleeds through the margin.
		g.gridTileBacking.Clear()
	}
	img := g.gridTileBacking
	if g.logDrawNodes {
		g.logger.Tracef("[draw/grid] buildGridTile: stepPx=%d cells=%d tileW=%d subs=%d backing=%d", stepPx, cells, tileW, len(g.grid.Subs), g.gridTileBackingSize)
	}
	// Same per-cell rounding as a single-cell tile, so tiling the block is seamless and pixel-identical.
	for _, sub := range g.grid.Subs {
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
	// Sync seconds-per-beat from the applied BPM without touching the edited control, or the draw
	// loop would undo a UI change before Update propagates it.
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm > 0 {
		g.drum.secPerBeat = 60.0 / float64(bpm)
	}
	// Smooth, unquantized beats for the timers; highlights and steps stay quantized.
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
