package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (g *Game) Layout(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}
	g.winW, g.winH = w, h
	// Update touch screen size for responsive UI sizing
	SetTouchScreenSize(w, h)
	UpdateProfile()

	/* update splitter and drum bounds */
	if g.split == nil {
		g.split = NewSplitter(h)
	}
	// Always stacked layout (mobile and desktop).
	g.split.horizontal = true

	// Detect orientation change and reset state so camera re-centers and
	// auto-sizing recalculates for the new dimension.
	if g.lastHorizontal != nil && *g.lastHorizontal != g.split.horizontal {
		g.centered = false
		g.split.userSet = false
		g.split.ratio = 0
		if g.drum != nil {
			g.drum.labelWidthDirty = true
			g.drum.bgDirty = true
			g.drum.markAllRowsDirty()
			g.drum.markRowControlsDirty()
		}
		// Invalidate grid cache so it rebuilds at new dimensions.
		g.gridCache = nil
	}
	h2 := g.split.horizontal
	g.lastHorizontal = &h2

	// Detect mobile ↔ desktop transition (profile class crossing the 900px threshold).
	small := Profile().IsMobile()
	if g.lastSmallScreen != nil && *g.lastSmallScreen != small {
		g.split.userSet = false
		g.split.ratio = 0
		g.centered = false
		if g.drum != nil {
			g.drum.resetOnScreenModeChange(small)
		}
		g.gridCache = nil
	}
	g.lastSmallScreen = &small

	if g.split.horizontal {
		if g.split.ratio == 0 {
			g.split.ratio = 0.5
		}
		if !g.split.userSet {
			g.split.Y = int(float64(h) * g.split.ratio)
		}
	} else {
		if g.split.ratio == 0 {
			g.split.ratio = 0.5
		}
		if !g.split.userSet {
			g.split.X = int(float64(w) * g.split.ratio)
		}
	}
	// Auto-size drum pane to fit timeline + rows (+ add-row), avoiding wasted space.
	if g.drum != nil && !g.split.userSet && (!runningUnderGoTest() || forceAutoSize) {
		if Profile().IsMobile() {
			// Mobile: always stacked — adaptive split (grid ≥50%, drum capped at 50%)
			y := adaptiveMobilePortraitSplitY(h, g)
			g.split.Y = y
		} else {
			// Desktop: content-based auto-sizing. Use the stable eqPanelHeight
			// constant instead of g.drum.eqH to prevent a feedback loop where
			// WidgetBoard rounding mutates eqH, causing Layout to compute a
			// different split.Y each frame (1-2px jitter).
			want := desktopHeaderH + (len(g.drum.Rows)+1)*g.drum.rowHeight() + eqPanelHeight
			minY := 120
			maxY := h - 120
			// Cap the drum panel (header + rows + EQ) at 50% of window height
			// so it doesn't dominate the screen when many instruments are present.
			if maxDrum := h / 2; want > maxDrum {
				want = maxDrum
			}
			y := h - want
			if y < minY {
				y = minY
			}
			if y > maxY {
				y = maxY
			}
			g.split.Y = y
		}
		if g.split.horizontal {
			if h > 0 {
				g.split.ratio = float64(g.split.Y) / float64(h)
			}
		} else {
			if w > 0 {
				g.split.ratio = float64(g.split.X) / float64(w)
			}
		}
	}
	g.drum.SetBounds(g.split.DrumRect(g.winW, g.winH))
	// Center camera once. During tests we usually keep (0,0) stable, except
	// when default-start behavior is explicitly requested by tests.
	if !g.centered && (!runningUnderGoTest() || enableDefaultStart) {
		if !g.split.horizontal {
			g.cam.OffsetX = float64(g.split.X) / 2
			g.cam.OffsetY = float64(h-gridTopOffset()) / 2
		} else {
			g.cam.OffsetX = float64(w) / 2
			g.cam.OffsetY = float64(g.split.Y-gridTopOffset()) / 2
		}
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
		if !g.split.horizontal {
			g.cam.OffsetX = float64(g.split.X) / 2
			g.cam.OffsetY = float64(h-gridTopOffset()) / 2
		} else {
			g.cam.OffsetX = float64(w) / 2
			g.cam.OffsetY = float64(g.split.Y-gridTopOffset()) / 2
		}
		g.cam.Snap()
		g.centered = true
	}
	// Position the grid-pane "?" help button (top-right corner). Derived every
	// Layout from the splitter geometry so its rect is current for both input
	// (mouse HandleInputResult / blocksAt / menuHit) and Draw without waiting
	// on the draw pass. Empty rect on mobile (gridHelpButtonRect guards it).
	if g.gridHelpBtn != nil {
		g.gridHelpBtn.SetRect(g.gridHelpButtonRect())
	}
	g.logger.Debugf("[GAME] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
	return w, h
}

// adaptiveMobilePortraitSplitY computes a content-based split Y for portrait
// mobile layout. The drum pane is the primary editing surface so it gets up
// to 65% of screen height; the graph stays readable as a 35% reference. When
// fewer rows are present the pane shrinks to fit (floor 30%) so the graph
// reclaims space.
//
// `needed` accounts for *every* surface in the drum pane so the default
// boot has zero rack-modulo slack: header + N row slots + 1 addRow slot +
// the bottom action bar. Without the +1 row slot the addRow button used
// to render either over the bar or with an orphan rack-bg strip beside
// it (screenshot review 2026-05-10); without the bar's height the bar
// rendered over the last row.
func adaptiveMobilePortraitSplitY(h int, g *Game) int {
	rh := TouchRowHeight() // mobile row height (44 px on iPhone-class portraits)
	numRows := 0
	if g.drum != nil {
		numRows = len(g.drum.Rows)
	}
	headerH := Profile().HeaderMaxH // capped header height (refreshWidgetLayout enforces this)
	if headerH <= 0 {
		headerH = mobileHeaderH
	}
	barH := TouchMinTarget() // bottom action bar height
	needed := headerH + (numRows+1)*rh + barH
	maxDrum := h * 65 / 100 // cap drum at 65% — drum is the primary edit surface
	minDrum := h * 30 / 100 // floor drum at 30%
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	return h - drumH
}
