package ui

import (
	"image"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func (g *Game) Layout(w, h int) (int, int) {
	g.winW, g.winH = w, h
	if g.frameBuffer != nil && (g.frameBufferW != w || g.frameBufferH != h) {
		g.frameBuffer = nil
		g.frameBufferW, g.frameBufferH = 0, 0
	}

	/* update splitter and drum bounds */
	if g.split == nil {
		g.split = NewSplitter(h)
	}
	if g.split.ratio == 0 { // first time → store ratio
		g.split.ratio = float64(g.split.Y) / float64(h)
	}
	if !g.split.userSet {
		g.split.Y = int(float64(h) * g.split.ratio)
	}
	// Auto-size drum pane height to fit timeline + rows (+ add-row), avoiding wasted space.
	if g.drum != nil && !g.split.userSet && (!runningUnderGoTest() || forceAutoSize) {
		want := timelineHeight + (len(g.drum.Rows)+1)*g.drum.rowHeight() + g.drum.eqH
		minY := 120
		maxY := h - 120
		y := h - want
		if y < minY {
			y = minY
		}
		if y > maxY {
			y = maxY
		}
		g.split.Y = y
		if h > 0 {
			g.split.ratio = float64(g.split.Y) / float64(h)
		}
	}
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))
	// Center camera once. During tests we usually keep (0,0) stable, except
	// when default-start behavior is explicitly requested by tests.
	if !g.centered && (!runningUnderGoTest() || enableDefaultStart) {
		g.cam.OffsetX = float64(w) / 2
		g.cam.OffsetY = float64(g.split.Y-topOffset) / 2
		g.cam.Snap()
		g.centered = true
	}
	if enableDefaultStart && !g.demoBuilt {
		if !runningUnderGoTest() {
			if !g.demoScheduled {
				g.demoScheduled = true // let Update build it shortly after startup
			}
		} else if len(g.nodes) == 0 {
			// In tests, create a single centered origin node.
			g.pendingStartRow = 0
			g.tryAddNode(0, 0, model.NodeTypeRegular)
		}
	} else if enableDefaultStart && len(g.nodes) == 0 {
		// Fallback when demo is disabled (tests) or failed.
		g.pendingStartRow = 0
		g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	// Ensure centering occurs in tests with default start even if earlier block didn't run.
	if !g.centered && runningUnderGoTest() && enableDefaultStart {
		g.cam.OffsetX = float64(w) / 2
		g.cam.OffsetY = float64(g.split.Y-topOffset) / 2
		g.cam.Snap()
		g.centered = true
	}
	g.logger.Debugf("[GAME] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
	return w, h
}
