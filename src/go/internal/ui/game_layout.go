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

	// Re-center and re-size for the new orientation.
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
	if g.drum != nil && !g.split.userSet && (!runningUnderGoTest() || forceAutoSize) {
		if Profile().IsMobile() {
			// Mobile: always stacked — adaptive split (grid ≥50%, drum capped at 50%)
			y := adaptiveMobilePortraitSplitY(h, g)
			g.split.Y = y
		} else {
			// Desktop: content-based auto-sizing (see desktopDrumPaneWant).
			want := desktopDrumPaneWant(h, len(g.drum.Rows), g.drum.rowHeight())
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
	// Tests keep (0,0) stable unless they ask for the default start.
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
	createdInitialNode := false
	if enableDefaultStart && !g.demoBuilt {
		if !runningUnderGoTest() {
			if !g.demoScheduled {
				g.demoScheduled = true // let Update build it shortly after startup
			}
		} else if len(g.nodes) == 0 {
			// In tests, create a single centered origin node.
			g.pendingStartRow = 0
			g.tryAddNode(0, 0, model.NodeTypeRegular)
			createdInitialNode = true
		}
	} else if enableDefaultStart && len(g.nodes) == 0 {
		// Fallback when demo is disabled (tests) or failed.
		g.pendingStartRow = 0
		g.tryAddNode(0, 0, model.NodeTypeRegular)
		createdInitialNode = true
	}
	// The default start node is startup, not a user action, so it must not leave an undo step.
	// Only right after the first node, so later re-layouts never wipe real history.
	if createdInitialNode && g.undoManager != nil {
		g.undoManager.OnExternalLoad()
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
	// Every Layout, so input and Draw see a current rect without waiting for the draw pass.
	g.syncGridCornerButtonRects()
	g.logger.Debugf("[game] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
	return w, h
}

// Content floor for the mobile audio panel: tall enough for the synth knob at its full ideal size
// (the tallest tab), so every tab stays roomy.
func mobileAudioPanelMinContentH() int {
	d := Profile().DensityValues()
	// Every band between the panel body and the knob cell must be reserved, or the cell is clamped
	// and the dial shrinks below ideal. Includes the sticky tab bar.
	chipStrip := 2 * d.SynthChipStripH
	knobCell := d.ParamCardH
	synthContent := Profile().SynthHeaderH + 2*SpaceSM + chipStrip + SpaceSM +
		d.SynthDetailHeaderH + 2*synthSectionPaddingY + knobCell
	// The Sampler wraps to two knob rows on mobile so full-length captions fit in every language.
	samplerHeaderH := d.SynthHeaderButtonH + 2*SpaceXS
	samplerContent := samplerHeaderH + SpaceXS + d.SamplerWaveMinH + SpaceSM +
		samplerControlMinHeight(d) + SpaceSM
	tallest := synthContent
	if samplerContent > tallest {
		tallest = samplerContent
	}
	// Slack for sub-pixel rounding, so the dial reaches its full diameter.
	return stickyBarHeight() + tallest + SpaceSM
}

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
	rowsNeed := headerH + (numRows+1)*rh + barH
	// Tab-independent: fits the rows and the tallest tab, so switching tabs never resizes the pane.
	audioNeed := headerH + mobileAudioPanelMinContentH() + barH
	needed := rowsNeed
	if audioNeed > needed {
		needed = audioNeed
	}
	maxDrum := h * 60 / 100 // grid always >= 40% on EVERY tab
	minDrum := h * 30 / 100
	drumH := needed
	if drumH > maxDrum {
		drumH = maxDrum
	}
	if drumH < minDrum {
		drumH = minDrum
	}
	// Whole rows only, so no half-blank strip.
	drumH = snapDrumHToWholeRows(drumH, headerH, barH, rh, maxDrum)
	return h - drumH
}

// Rounds up to the next row when it fits under maxDrum, else down.
func snapDrumHToWholeRows(drumH, headerH, barH, rh, maxDrum int) int {
	if rh <= 0 {
		return drumH
	}
	rackH := drumH - headerH - barH
	if rackH < rh {
		return drumH
	}
	rem := rackH % rh
	if rem == 0 {
		return drumH
	}
	if up := drumH + (rh - rem); up <= maxDrum {
		return up
	}
	return drumH - rem
}

// The stable eqPanelHeight, not dv.eqH: board rounding mutates eqH and the split would jitter.
// Capped at 60 % so the rows get room while the grid keeps 40 %.
func desktopDrumPaneWant(h, nRows, rowH int) int {
	want := desktopHeaderH + (nRows+1)*rowH + eqPanelHeight
	if maxDrum := h * 3 / 5; want > maxDrum {
		want = maxDrum
	}
	return want
}
