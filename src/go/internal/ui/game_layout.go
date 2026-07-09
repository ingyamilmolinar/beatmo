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
	// (desktopDrumPaneWant below computes the desktop share.)
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
	// The initial default start node is part of startup, NOT a user action: it
	// must not leave an undo step (which would light the transport Undo button at
	// launch). Re-baseline the undo history to the freshly-created document. Gated
	// on having just created the very first node (len was 0), so a re-layout after
	// real edits never wipes the user's undo history.
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
	// Position the grid-pane "?" help button (top-right corner). Derived every
	// Layout from the splitter geometry so its rect is current for both input
	// (mouse HandleInputResult / blocksAt / menuHit) and Draw without waiting
	// on the draw pass. Empty rect on mobile (gridHelpButtonRect guards it).
	if g.gridHelpBtn != nil {
		g.gridHelpBtn.SetRect(g.gridHelpButtonRect())
	}
	g.logger.Debugf("[game] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
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
// mobileAudioPanelMinContentH is the comfortable minimum CONTENT height (excluding
// the transport header + bottom action bar) for the mobile audio panel. Sized so
// the synth knob renders at its full SynthKnobIdeal diameter (not the shrunk
// SynthKnobMin): a ~2-row chip strip + the detail header + one full knob cell +
// section paddings. Synth is the tallest audio tab, so this single floor keeps
// every audio tab roomy (and the sampler control buttons clear of the knob labels).
func mobileAudioPanelMinContentH() int {
	d := Profile().DensityValues()
	// Walk the synth-tab layout chain top-to-bottom so the content floor reserves
	// EVERY band between the panel body and the knob cell (otherwise the cell is
	// clamped and the dial shrinks below SynthKnobIdeal — see
	// TestMobileSynthTab_KnobRendersAtIdealSize):
	//   SynthHeaderH            — Save/SaveAs/Reset header strip (layoutSynthTab)
	//   2*SpaceSM               — layoutSynthSections top+bottom rowR trims
	//   2*SynthChipStripH       — wrapped pipeline chip strip (2 rows at 390px)
	//   SpaceSM                 — gap between chip strip and detail pane
	//   SynthDetailHeaderH      — detail-pane header (title + enable pill)
	//   2*synthSectionPaddingY  — innerR top+bottom padding inside the detail pane
	//   knobCell                — dial + caption + step-badge gap (synthKnobCellHeight)
	// The mobile audio panel reserves a sticky tab-switcher bar at its top
	// (stickyBarHeight); the synth content (contentRect) is everything below it.
	// The split budgets this whole value as the panel height, so include the bar.
	chipStrip := 2 * d.SynthChipStripH
	knobCell := d.SynthKnobIdeal + d.SynthKnobCaptionH + SpaceXS
	synthContent := Profile().SynthHeaderH + 2*SpaceSM + chipStrip + SpaceSM +
		d.SynthDetailHeaderH + 2*synthSectionPaddingY + knobCell
	// The Sampler tab can be the tallest: on mobile it wraps its knobs to two
	// rows so full-font captions ("Ganancia +0 dB") fit without shrinking the
	// dial below the usable minimum (the language-invariant layout). Reserve a
	// header band + a minimum waveform + the two-row control column so the dial
	// stays usable in every language.
	samplerHeaderH := d.SynthHeaderButtonH + 2*SpaceXS
	samplerContent := samplerHeaderH + SpaceXS + d.SamplerWaveMinH + SpaceSM +
		samplerControlMinHeight(d) + SpaceSM
	tallest := synthContent
	if samplerContent > tallest {
		tallest = samplerContent
	}
	// Slack absorbs sub-pixel grid/cell rounding and the panel's 1px bottom-bar
	// inset, so the dial reaches its full ideal diameter rather than 1px short.
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
	// The pane height is tab-INDEPENDENT: it fits both the drum rows AND the
	// tallest audio tab's content, so switching tabs never resizes the panel.
	// Computed in Layout → applied at startup and on every window resize.
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
	// Snap so the rack shows whole rows (no half-blank strip): bump up to the next
	// full row when it fits under the 40%-grid cap, else trim down to whole rows.
	drumH = snapDrumHToWholeRows(drumH, headerH, barH, rh, maxDrum)
	return h - drumH
}

// snapDrumHToWholeRows adjusts the drum-pane height so its rack region (the area
// below the transport header and above the bottom action bar) holds a WHOLE number
// of drum rows — no partial-row blank strip. Rounds UP to include the next full row
// when that still fits under maxDrum; otherwise rounds DOWN to the last whole row.
// Returns drumH unchanged when the rack is already row-aligned or too small for a row.
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

// desktopDrumPaneWant returns the content-based drum-pane height for a
// desktop window of height h: transport header + all rows (+ the add-row
// band) + the audio panel. Uses the stable eqPanelHeight constant instead
// of dv.eqH to prevent a feedback loop where WidgetBoard rounding mutates
// eqH, causing Layout to compute a different split.Y each frame (1-2px
// jitter). Capped at 60% of the window: the rows are the product's core
// surface, so a multi-row circuit gets the space it asks for — the old 50%
// cap booted the 6-row startup demo with a single visible row (D1,
// 2026-07-04 design pass) — while the grid pane always keeps >= 40%.
func desktopDrumPaneWant(h, nRows, rowH int) int {
	want := desktopHeaderH + (nRows+1)*rowH + eqPanelHeight
	if maxDrum := h * 3 / 5; want > maxDrum {
		want = maxDrum
	}
	return want
}
