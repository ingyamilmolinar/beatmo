package ui

import (
	"fmt"
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─── public update ────────────────────────────────────────── */

// refreshLenButtonsStyle re-derives the length +/− buttons' Style and
// IconColor. Both platforms now use the neutral stepper look (same recipe
// as button-secondary) so there is no mobile/desktop branch. Called from
// recalcButtons every layout pass and once from the ctor. Idempotent and O(1).
func (dv *DrumView) refreshLenButtonsStyle() {
	if dv.lenDecBtn == nil || dv.lenIncBtn == nil {
		return
	}
	dv.lenDecBtn.Style = LenDecStyle
	dv.lenDecBtn.IconColor = colIncDecIcon
	dv.lenIncBtn.Style = LenIncStyle
	dv.lenIncBtn.IconColor = colIncDecIcon
}

func (dv *DrumView) recalcButtons() {
	// Re-derive profile-dependent button chrome before doing anything else
	// so the rest of recalcButtons (and the components it positions) see
	// styles that match the current LayoutProfile.
	dv.refreshLenButtonsStyle()
	// Late mobile init: when mobile profile first activates (after Layout()
	// calls SetTouchScreenSize on WASM), collapse EQ and mark layout dirty.
	p := Profile()
	if p.IsMobile() && !dv.mobileEQInited {
		dv.mobileEQInited = true
		dv.mobileEQCollapsed = true
		dv.bgDirty = true
		if dv.widgets != nil {
			// Update widget board for mobile column proportions.
			// At construction time the profile may be desktop on WASM
			// (SetTouchScreenSize hasn't been called yet), so the board
			// was created with desktop weights [1, 3]. Fix that now.
			dv.widgets.SetWeights(p.ColWeights, p.RowWeights)
			dv.widgets.ToggleWidget(WidgetWave, false)
			dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)
		}
		dv.refreshWidgetLayout()
		dv.eqH = 0
	}
	if runningUnderGoTest() {
		// Tests normally collapse the audio panel (eqPanelHeight=0) to keep the
		// drum rows dominant. eqPanelHeightForTest is a seam that lets a
		// functional test exercise the REAL production-floored panel (where the
		// panel top diverges from the widget-board boundary) — the scenario this
		// gate otherwise hides. 0 = default collapse behaviour.
		if eqPanelHeightForTest > 0 {
			eqPanelHeight = eqPanelHeightForTest
		} else {
			eqPanelHeight = 0
			dv.eqH = 0
		}
	}
	dv.calcLabelWidth()

	// Allocate the mobile-only bottom action bar host. This is the bottom
	// sheet that holds the volume icon, view-switch, and overflow controls
	// in the mobile redesign (B3 critique). Desktop leaves the rect empty.
	//
	// Skip allocation when the drum-pane bounds are too tight to fit even
	// one row above the bar (e.g. landscape phones at 140 px tall). The
	// bar would otherwise consume TouchMinTarget px below the header and
	// leave nothing for the rows themselves — visibleRows would collapse
	// to 0 and the rack would not paint. Falling back to the pre-bar
	// layout on these viewports keeps rows visible; vol/view/overflow
	// remain reachable through the transport overflow menu.
	if p.UseBottomSheet {
		barH := TouchMinTarget()
		usable := dv.Bounds.Dy() - dv.headerH - barH
		if usable >= dv.rowHeight() {
			dv.bottomActionBarRect = image.Rect(
				dv.Bounds.Min.X,
				dv.Bounds.Max.Y-barH,
				dv.Bounds.Max.X,
				dv.Bounds.Max.Y,
			)
		} else {
			dv.bottomActionBarRect = image.Rectangle{}
		}
	} else {
		dv.bottomActionBarRect = image.Rectangle{}
	}

	// EQ peek strip is intentionally NOT allocated. The historical 24-px
	// sparkline-above-the-bar surface (mobile EQ collapsed) rendered as
	// a black band on default boot — every band sits at 0 dB so the
	// sparkline is invisible against `colBGBottom`. The bottom segmented
	// control already exposes an "EQ" tab providing the same expand-EQ
	// affordance, so the peek was a redundant orphan surface between
	// the row rack and the action bar (flagged in screenshot review
	// 2026-05-10). The unified-layout invariant
	// (`rowsBottom() == bottomActionBarRect.Min.Y`) is locked in by
	// `mobile_default_unified_layout_test.go`.
	dv.eqPeekRect = image.Rectangle{}

	transport := dv.widgetRects[WidgetTransport]
	if transport.Empty() {
		transport = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y+dv.headerH)
	}
	// On desktop, cap at the widget column width to prevent buttons
	// from extending into the timeline widget area.
	widgetColW := 0
	if dv.widgets != nil {
		widgetColW = dv.widgets.ColWidth(0)
	}
	// Cap transport height to headerH so buttons don't overlap row controls.
	if transport.Dy() > dv.headerH {
		transport.Max.Y = transport.Min.Y + dv.headerH
	}
	if p.IsMobile() {
		// Keep transport within its widget column — no overlap into timeline.
	} else if widgetColW > 0 && transport.Dx() < widgetColW {
		transport.Max.X = transport.Min.X + widgetColW
	}
	leftCol := transport

	// Delegate transport layout to TransportZone when available.
	if dv.transportZone != nil && dv.tree != nil {
		controlLeft := leftCol.Min.X + p.ControlLeftInset
		if controlLeft >= leftCol.Max.X {
			controlLeft = leftCol.Min.X
		}
		topBounds := image.Rect(controlLeft, leftCol.Min.Y, leftCol.Max.X, leftCol.Max.Y)
		dv.tree.SetZoneRect("transport", topBounds)
		// Tell the zone whether DrumView's bottom action bar is hosting
		// vol/view/overflow this frame. When the bar collapses on
		// ultra-short viewports (see bar allocation block above), the
		// zone falls back to the pre-Task-1.3 two-row layout so those
		// three buttons remain reachable inside the top toolbar.
		dv.transportZone.SetUseBottomBar(p.IsMobile() && !dv.bottomActionBarRect.Empty())
		if dv.transportZone.NeedsLayout() || dv.transportZone.rect != topBounds {
			dv.tree.LayoutZoneNow("transport")
		}
		dv.mainVolIconRect = dv.transportZone.mainVolIconRect
		dv.mainVolRect = dv.transportZone.mainVolRect

		// Mobile (Theme 1+4): the bottom action bar hosts the 6-segment
		// view switcher (Pads/EQ/Wave/Spec/Mtr/Scope) at full bar width.
		// The vol icon and overflow kebab now live in the top toolbar
		// (Theme 4) — placed by the transport zone's layoutMobile (we
		// do NOT clear those rects here).
		if p.IsMobile() && !dv.bottomActionBarRect.Empty() {
			bar := dv.bottomActionBarRect
			barPad := ActiveTopBarSpec().Padding
			// Legacy binary view-switch is suppressed on mobile in favor
			// of the segmented control — keep its rect cleared.
			if dv.transportZone.viewSwitchBtn != nil {
				dv.transportZone.viewSwitchBtn.SetRect(image.Rectangle{})
			}
			// Segmented spans the full bar width minus horizontal padding.
			if dv.viewSwitchSegmented != nil {
				segRect := bar
				if barPad > 0 && segRect.Dx() > 2*barPad {
					segRect = image.Rect(segRect.Min.X+barPad, segRect.Min.Y, segRect.Max.X-barPad, segRect.Max.Y)
				}
				dv.viewSwitchSegmented.SetRect(segRect)
			}
			// Re-rebuild hit areas so the segmented control is registered
			// (transport zone's Layout ran before we placed it).
			dv.transportZone.rebuildHitAreas()
			dv.transportZone.SetBarRect(bar)
			// Register the segmented control as a hit area in the transport zone.
			// ZIndex ZViewSwitch (150): it MUST sit above ALL in-panel tab
			// content so the tab switcher can never be occluded — otherwise a
			// tab whose controls overlap the bottom bar strands the user on it.
			// The Chain tab's stage cards (z=141) and swatches (z=142) draw
			// down into the bar; the prior hardcoded z=140 lost to them, so
			// the user could not switch away from Chain. ZViewSwitch is the
			// dedicated constant for exactly this control (above eq controls
			// at 131, chain content at 141/142; below row-zoom chips at 160
			// and portals at 300). Pinned by the view-mode transition matrix.
			dv.tree.HitIndexRef().Update("transport", dv.transportZone.HitAreas())
			// The mobile view-switch segmented control is published into the
			// AUDIO subtree's HitIndex (NOT the transport zone in dv.tree). The
			// switcher overlays the bottom bar that the eq-panel catch-all
			// (z=130, audioTree) also covers; for its ZViewSwitch (150) z to
			// win it must live in the SAME subtree as the catch-all. After the
			// two-subtree split, a hit area in dv.tree can never outrank one in
			// audioTree (separate HitIndexes — audioTree, the higher-z child, is
			// dispatched first and its catch-all consumes the tap). Publishing
			// here keeps the switcher reachable on Chain/Sampler/etc. Pinned by
			// the view-mode transition matrix.
			if dv.audioTree != nil {
				if dv.viewSwitchSegmented != nil && !dv.viewSwitchSegmented.Rect().Empty() {
					dv.audioTree.HitIndexRef().Update("view-switch", []HitArea{{
						Rect:     dv.viewSwitchSegmented.Rect(),
						ClipRect: bar,
						ZIndex:   ZViewSwitch,
						Tag:      "transport-view-segmented",
						Handler:  &segmentedHitAdapter{sc: dv.viewSwitchSegmented},
					}})
				} else {
					dv.audioTree.HitIndexRef().Update("view-switch", nil)
				}
			}
		} else if dv.audioTree != nil {
			// Non-mobile (or collapsed bar): no segmented switcher — clear any
			// stale view-switch hit area from the audio subtree's HitIndex so a
			// mobile→desktop transition can't strand it.
			dv.audioTree.HitIndexRef().Update("view-switch", nil)
		}
	}

	// Delegate row rack layout to RowRackZone when available.
	if dv.rowRackZone != nil && dv.tree != nil {
		rackRect := dv.widgetRects[WidgetRack]
		if rackRect.Empty() {
			rowsTop := dv.Bounds.Min.Y + dv.headerH
			rackRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.rowsBottom())
		}
		// Clamp rack top to match capped headerH.
		rowsTop := dv.Bounds.Min.Y + dv.headerH
		if rackRect.Min.Y < rowsTop {
			rackRect.Min.Y = rowsTop
		}
		// Clamp rack bottom to rowsBottom() so the rack zone never extends
		// over the mobile bottom action bar (Pads/EQ/Wave/Spec/Mtr/Scope)
		// or the EQ peek strip. The WidgetBoard does not know about these
		// pane-level surfaces — it allocates the rack column down to
		// dv.Bounds.Max.Y. Without this clamp the addRowBtn would anchor
		// to dv.Bounds.Max.Y - rh and end up rendered ON TOP OF the
		// segmented switcher in the bottom action bar (regression flagged
		// in screenshot review 2026-05-10).
		if rb := dv.rowsBottom(); rb < rackRect.Max.Y {
			rackRect.Max.Y = rb
		}
		// Override the zone's visible rows to the row count that actually
		// fits inside the rack widget rect (rackVisibleRows) — never the
		// bounds-derived dv.visibleRows() which can over-estimate when
		// the widget board allocates space below the rack to the wave
		// widget. Over-estimating placed per-row widgets at Y bands
		// outside the rack rect, producing the screenshot's "phantom
		// row + addRowBtn-in-the-middle" layout glitch. The complementary
		// invariant (rack + timeline span the full rows-area when EQ is
		// collapsed) is enforced in refreshWidgetLayout, so for the
		// EQ-visible case rackVisibleRows == dv.visibleRows() in practice.
		dv.rowRackZone.SetVisibleRowsOverride(dv.rackVisibleRows())
		dv.tree.SetZoneRect("row-rack", rackRect)
		dv.rowRackZone.SetScreenBounds(dv.Bounds)
		// Bidirectional sync: if RowRackZone's scroll updated its own
		// rowOffset (via scroll handler), adopt that value before we
		// push ours. This prevents overwriting scroll-initiated changes.
		if zoneOff := dv.rowRackZone.RowOffset(); zoneOff != dv.rowOffset {
			dv.rowOffset = zoneOff
		}
		dv.rowRackZone.SetRowOffset(dv.rowOffset)
		dv.rowRackZone.SetSelRow(dv.selRow)
		if dv.rowRackZone.NeedsLayout() || dv.rowRackZone.rect != rackRect {
			dv.tree.LayoutZoneNow("row-rack")
		}
		// RowRack fields are now accessed via accessor methods; no alias sync needed.
	}

	// Legacy transport layout — used when TransportZone is nil (fallback).
	if dv.transportZone == nil {
		// Dynamic button sizing — single row transport.
		pad := p.ControlPadding
		// derive padding from available height
		if pad > leftCol.Dy()/4 {
			pad = leftCol.Dy() / 4
		}
		safeInset := func(r image.Rectangle, pad int) image.Rectangle {
			if r.Empty() {
				return r
			}
			minDim := r.Dx()
			if r.Dy() < minDim {
				minDim = r.Dy()
			}
			maxPad := (minDim - 2) / 2
			if maxPad < 0 {
				maxPad = 0
			}
			minW := 48
			minH := debugCharH + 2
			maxPadW := (r.Dx() - minW) / 2
			if maxPadW < 0 {
				maxPadW = 0
			}
			maxPadH := (r.Dy() - minH) / 2
			if maxPadH < 0 {
				maxPadH = 0
			}
			if maxPadW < maxPad {
				maxPad = maxPadW
			}
			if maxPadH < maxPad {
				maxPad = maxPadH
			}
			if pad > maxPad {
				pad = maxPad
			}
			return insetRect(r, pad)
		}
		// Single-row transport: all controls in one row.
		controlLeft := leftCol.Min.X + p.ControlLeftInset
		if controlLeft >= leftCol.Max.X {
			controlLeft = leftCol.Min.X
		}
		topBounds := image.Rect(controlLeft, leftCol.Min.Y, leftCol.Max.X, leftCol.Max.Y)
		// Helper: ensure gap between adjacent buttons.
		ensureGap := func(left, right *Button) {
			if left == nil || right == nil {
				return
			}
			lr, rr := left.Rect(), right.Rect()
			if lr.Max.X >= rr.Min.X {
				dx := lr.Max.X - rr.Min.X + 2
				rr.Min.X += dx
				rr.Max.X += dx
				right.SetRect(rr)
			}
		}
		clampBtn := func(btn *Button, bounds image.Rectangle) {
			r := btn.Rect()
			if r.Min.Y < bounds.Min.Y {
				r.Min.Y = bounds.Min.Y
			}
			if r.Max.Y > bounds.Max.Y {
				r.Max.Y = bounds.Max.Y
			}
			btn.SetRect(r)
		}
		// Helper: stack two buttons vertically in a column rect.
		stackVertical := func(top, bot *Button, col image.Rectangle, bounds image.Rectangle) {
			split := col.Dy() / 2
			if split < 8 {
				split = col.Dy() / 2
			}
			top.SetRect(col)
			tr := top.Rect()
			tr.Max.Y = tr.Min.Y + split
			top.SetRect(tr)
			bot.SetRect(col)
			br := bot.Rect()
			br.Min.Y = tr.Max.Y
			if br.Max.Y > bounds.Max.Y {
				br.Max.Y = bounds.Max.Y
			}
			if tr.Max.Y > bounds.Max.Y {
				tr.Max.Y = bounds.Max.Y
			}
			if br.Min.Y > br.Max.Y {
				br.Min.Y = br.Max.Y
			}
			bot.SetRect(br)
			clampBtn(top, bounds)
			clampBtn(bot, bounds)
		}

		if p.IsMobile() {
			// Mobile two-row transport using nested grids.
			outerGrid := NewGridLayout(topBounds, []float64{1}, []float64{1, 1})
			// Row 0: Play | Stop | BPM box | BPM+/- | Sub
			row0Grid := outerGrid.SubGrid(0, 0,
				[]float64{1.0, 1.0, 2.0, 1.0, 1.0}, []float64{1})
			// Row 1: VolIcon | ViewSwitch | Overflow
			row1Grid := outerGrid.SubGrid(0, 1,
				[]float64{1.0, 1.0, 1.0}, []float64{1})

			row0Bounds := outerGrid.Cell(0, 0)

			dv.playBtn().SetRect(safeInset(row0Grid.Cell(0, 0), pad))
			dv.stopBtn().SetRect(safeInset(row0Grid.Cell(1, 0), pad))
			dv.bpmBox().Rect = safeInset(row0Grid.Cell(2, 0), pad)
			bpmCol := safeInset(row0Grid.Cell(3, 0), pad)
			stackVertical(dv.bpmIncBtn(), dv.bpmDecBtn(), bpmCol, row0Bounds)
			dv.subdivBtn().SetRect(safeInset(row0Grid.Cell(4, 0), pad))
			// Mobile: volume icon opens popup; no inline slider.
			dv.mainVolIconRect = safeInset(row1Grid.Cell(0, 0), pad)
			if dv.mainVolSlider() != nil {
				dv.mainVolSlider().SetRect(image.Rectangle{})
				dv.mainVolRect = image.Rectangle{}
			}
			if dv.viewSwitchBtn() != nil {
				dv.viewSwitchBtn().SetRect(safeInset(row1Grid.Cell(1, 0), pad))
			}
			if dv.overflowBtn() != nil {
				dv.overflowBtn().SetRect(safeInset(row1Grid.Cell(2, 0), pad))
			}
			// Hide desktop-only buttons on mobile.
			dv.trackBtn().SetRect(image.Rectangle{})
			// Upload/Import/Export behind overflow on mobile.
			dv.uploadBtn().SetRect(image.Rectangle{})
			dv.importBtn().SetRect(image.Rectangle{})
			dv.exportBtn().SetRect(image.Rectangle{})
			// Hide legacy EQ toggle on mobile (replaced by viewSwitchBtn).
			if dv.eqToggleMobile() != nil {
				dv.eqToggleMobile().SetRect(image.Rectangle{})
			}
		} else {
			// Desktop single-row transport using persistent LayoutGroup.
			rowWeights := []float64{1.3, 1.3, 2.2, 0.7, 1.0, 0.3, 1.0, 0.8, 0.8, 0.8}
			if dv.transportGroup() == nil {
				dv.transportZone.transportGroup = NewLayoutGroup("transport", topBounds, rowWeights, []float64{1})
			}
			dv.transportGroup().SetBounds(topBounds)

			dv.playBtn().SetRect(safeInset(dv.transportGroup().Cell(0, 0), pad))
			dv.stopBtn().SetRect(safeInset(dv.transportGroup().Cell(1, 0), pad))
			dv.bpmBox().Rect = safeInset(dv.transportGroup().Cell(2, 0), pad)
			bpmCol := safeInset(dv.transportGroup().Cell(3, 0), pad)
			stackVertical(dv.bpmIncBtn(), dv.bpmDecBtn(), bpmCol, topBounds)
			dv.subdivBtn().SetRect(safeInset(dv.transportGroup().Cell(4, 0), pad))
			// col 5 is flexible spacer
			// Desktop: icon-only volume (popup on click).
			volCell := safeInset(dv.transportGroup().Cell(6, 0), pad)
			dv.mainVolIconRect = volCell
			if dv.mainVolSlider() != nil {
				dv.mainVolSlider().SetRect(image.Rectangle{})
				dv.mainVolRect = image.Rectangle{}
			}
			dv.uploadBtn().SetRect(safeInset(dv.transportGroup().Cell(7, 0), pad))
			dv.importBtn().SetRect(safeInset(dv.transportGroup().Cell(8, 0), pad))
			dv.exportBtn().SetRect(safeInset(dv.transportGroup().Cell(9, 0), pad))
			// Track button positioned in timeline area (see below), not toolbar.
			dv.trackBtn().SetRect(image.Rectangle{})
			// Hide mobile-only buttons on desktop.
			if dv.eqToggleMobile() != nil {
				dv.eqToggleMobile().SetRect(image.Rectangle{})
			}
			if dv.viewSwitchBtn() != nil {
				dv.viewSwitchBtn().SetRect(image.Rectangle{})
			}
			if dv.overflowBtn() != nil {
				dv.overflowBtn().SetRect(image.Rectangle{})
			}
		}
		ensureGap(dv.playBtn(), dv.stopBtn())
	} // end legacy transport layout

	// Timeline/progress lives inside the timeline widget near the bottom of its header row.
	tlWidget := dv.widgetRects[WidgetTimeline]
	if tlWidget.Empty() {
		tlWidget = image.Rect(dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Min.Y, dv.Bounds.Max.X-10, dv.Bounds.Min.Y+dv.headerH+dv.rowsAreaHeight())
	}
	headerTop := tlWidget.Min.Y
	headerH := dv.headerH
	if headerH > tlWidget.Dy() {
		headerH = tlWidget.Dy()
	}
	top := headerTop + headerH - tlBarHeight() - 2
	if top < tlWidget.Min.Y {
		top = tlWidget.Min.Y
	}
	// Compute track button width. The track/lock chip is now the unified
	// LEFT anchor of the band on BOTH platforms (counter + notification area
	// sit to its right), so the timeline reserves left space on both. Mobile
	// uses the touch-target floor; desktop matches the play-button width.
	const trackBtnGap = 4 // breathing room between track button and timeline
	trackBtnW := dv.playBtn().Rect().Dx()
	if trackBtnW <= 0 {
		trackBtnW = 44
	}
	if p.IsMobile() {
		if tm := TouchMinTarget(); tm > trackBtnW {
			trackBtnW = tm
		}
	}
	// bandLeftReserved is the strip the track/lock chip occupies at the band's
	// left on BOTH platforms (chip → counter → notif). On DESKTOP the timeline
	// bar also steps around it (tall strip never paints over the bar). On
	// MOBILE the bar keeps its full width — the short chip overlaps only the
	// bar's left corner, the same trade the old right-edge chip made — so the
	// narrow mobile ruler doesn't drop below its min-width floor
	// (TestMobileDrumTimelineMinPercent).
	bandLeftReserved := trackBtnW + trackBtnGap
	tlLeftReserved := 0
	if !p.IsMobile() {
		tlLeftReserved = bandLeftReserved
	}
	dv.timelineRect = image.Rect(
		tlWidget.Min.X+tlLeftReserved,
		top,
		tlWidget.Max.X,
		top+tlBarHeight(),
	)

	// Beat counter rect: above the timeline bar. Aligned to timelineRect's
	// left edge — that edge already accounts for the track button reservation.
	// infoH must accommodate the pill chrome drawBeatCounter actually paints
	// (`pillH = TextHeight() + 2*pillPadY`); otherwise the pill bleeds below
	// Max.Y into the timeline bar (root cause of the startup-chrome bug).
	const minChromeGap = 2
	pillH := TextHeight() + 2*beatCounterPillPadY
	infoH := debugCharH + 4
	if pillH > infoH {
		infoH = pillH
	}
	bcTop := dv.timelineRect.Min.Y - infoH - minChromeGap
	if bcTop < tlWidget.Min.Y {
		bcTop = tlWidget.Min.Y
	}
	// Band content (counter + notif) starts right of the track chip on both
	// platforms. On desktop this equals timelineRect.Min.X (the bar is also
	// reserved); on mobile the bar starts further left, so anchor explicitly.
	dv.beatCounterRect = image.Rect(
		tlWidget.Min.X+bandLeftReserved, bcTop,
		dv.timelineRect.Max.X, bcTop+infoH,
	)

	// Position len +/- buttons at top-right of timeline widget area on
	// desktop only. On mobile (or in EQ/Wave audio view) the +/− pair
	// next to the timeline is too narrow to discover on phone-class
	// screens (A7 in the screenshot critique); the entries live behind
	// the overflow menu instead.
	if !p.IsMobile() && dv.currentViewMode != viewModeEQ {
		btnW := 36
		// Stack vertically: Inc on top, Dec below. Shrink btnH so the
		// pair fits inside the header band rather than overflowing into
		// the rows area below (`Bounds.Min.Y + headerH`). Pre-clamp fix:
		// btnH defaulted to beatCounterRect.Dy() and the pair extended
		// below the header on every desktop viewport.
		headerBottom := dv.Bounds.Min.Y + dv.headerH
		y := dv.beatCounterRect.Min.Y
		maxStackBtnH := (headerBottom - y) / 2
		btnH := dv.beatCounterRect.Dy()
		if btnH < 20 {
			btnH = 20
		}
		if maxStackBtnH < btnH {
			btnH = maxStackBtnH
		}
		x := dv.timelineRect.Max.X - btnW
		dv.lenIncBtn.SetRect(image.Rect(x, y, x+btnW, y+btnH))
		dv.lenDecBtn.SetRect(image.Rect(x, y+btnH, x+btnW, y+2*btnH))
		// Shrink beat counter and timeline to avoid overlapping the buttons.
		dv.beatCounterRect.Max.X = x - 4
		dv.timelineRect.Max.X = x - 4
	} else {
		dv.lenIncBtn.SetRect(image.Rectangle{})
		dv.lenDecBtn.SetRect(image.Rectangle{})
	}

	// Row-zoom chips — mobile only, [⊕]/[⊖] vertical pair placed in the
	// timeline header band at the right edge of the timeline widget,
	// directly under the transport row 0 (Play/Stop/BPM) and to the right
	// of the transport row 1 cluster (VolIcon/ViewSwitch/Overflow). The
	// pair fits in a single `TouchMinTarget`-wide strip so the timeline
	// ruler keeps its full readable width. The beat counter pill and
	// ruler shrink to leave the chips a clean strip.
	//
	// Computed BEFORE the trackBtn block so the trackBtn's right edge
	// (which is anchored to `beatCounterRect.Max.X`) lands at the already-
	// shrunken pill edge — keeping the chip cluster, beat counter pill,
	// and trackBtn all inside the timeline widget X range.
	//
	// Replaces the prior inline-with-addRow placement at the bottom of
	// the rack column: the add-row "+" stays put below the row controls,
	// the zoom chips move up next to the timeline they affect. Hidden
	// when the bottom action bar collapses (ultra-short viewports).
	if dv.rowRackZone != nil {
		dv.rowRackZone.SetAddRowRightReserve(0)
	}
	if p.IsMobile() && !dv.MobileEQMode() && !dv.bottomActionBarRect.Empty() &&
		!dv.timelineRect.Empty() {
		side := TouchMinTarget()
		rightX := tlWidget.Max.X
		if rightX == 0 {
			rightX = dv.Bounds.Max.X
		}
		chipsRight := rightX
		chipsLeft := chipsRight - side
		// Y span: full timeline header band (beat counter pill + ruler).
		y0 := dv.beatCounterRect.Min.Y
		if dv.beatCounterRect.Empty() || dv.timelineRect.Min.Y < y0 {
			y0 = dv.timelineRect.Min.Y
		}
		y1 := dv.timelineRect.Max.Y
		// Guarantee a TouchMinTarget-tall combined strip even when the
		// header band is shorter than 44 px (small viewports).
		if y1-y0 < side {
			y1 = y0 + side
		}
		mid := (y0 + y1) / 2
		dv.rowZoomChipRect = image.Rect(chipsLeft, y0, chipsRight, y1)
		if dv.rowZoomIncBtn != nil {
			dv.rowZoomIncBtn.SetRect(image.Rect(chipsLeft, y0, chipsRight, mid))
		}
		if dv.rowZoomDecBtn != nil {
			dv.rowZoomDecBtn.SetRect(image.Rect(chipsLeft, mid, chipsRight, y1))
		}
		// Shrink the beat counter pill and timeline ruler bar so they do
		// not paint under the chip strip.
		rightLimit := chipsLeft - SpaceXS
		if dv.timelineRect.Max.X > rightLimit {
			dv.timelineRect.Max.X = rightLimit
		}
		if dv.beatCounterRect.Max.X > rightLimit {
			dv.beatCounterRect.Max.X = rightLimit
		}
	} else {
		dv.rowZoomChipRect = image.Rectangle{}
		if dv.rowZoomIncBtn != nil {
			dv.rowZoomIncBtn.SetRect(image.Rectangle{})
		}
		if dv.rowZoomDecBtn != nil {
			dv.rowZoomDecBtn.SetRect(image.Rectangle{})
		}
	}

	// Position the track/lock chip — the unified LEFT anchor of the band on
	// BOTH platforms. It occupies the reserved left strip (tlWidget.Min.X ..
	// +trackBtnW) that the timeline already steps around, with the counter +
	// notification area to its right. Desktop keeps a tall strip down to the
	// play-button bottom; mobile grows to the touch-target floor.
	if !dv.beatCounterRect.Empty() {
		chipBottom := dv.timelineRect.Max.Y
		if !p.IsMobile() {
			if pb := dv.playBtn().Rect(); !pb.Empty() && pb.Max.Y > chipBottom {
				chipBottom = pb.Max.Y
			}
		} else if h := chipBottom - dv.beatCounterRect.Min.Y; h < TouchMinTarget() {
			chipBottom = dv.beatCounterRect.Min.Y + TouchMinTarget()
		}
		dv.trackBtn().SetRect(image.Rect(
			tlWidget.Min.X, dv.beatCounterRect.Min.Y,
			tlWidget.Min.X+trackBtnW, chipBottom,
		))
	}

	// Carve the dedicated notification area from the right of the band. The
	// counter keeps a left-anchored slot sized to a stable upper-bound width;
	// the remainder (to the band's already-clamped right limit) becomes the
	// notification marquee area. Runs AFTER the len-button / row-zoom-chip
	// shrinks so it respects their right clamps.
	if !dv.beatCounterRect.Empty() {
		bandRight := dv.beatCounterRect.Max.X
		// Size the counter slot to the LIVE rendered readout width (position +
		// time) so the notification area starts right where the beat/timer text
		// ends and reclaims the band's remaining width — the notif used to
		// start after a fixed worst-case slot ("Beat 888 · 88:88"), leaving a
		// large dead gap before it (it looked far smaller than the band
		// allowed). recalcButtons runs every frame and re-carves this band, so
		// the split tracks the live readout (dv.lastElapsedBeats, ≤1 frame
		// stale). beatCounterSlotWidth is the CEILING: a pathological readout
		// (4-digit beats / 2-digit minutes beyond the worst case) can't swallow
		// the whole notif area — the readout clips at the cap instead.
		readoutW := TextWidth(dv.timelineInfo(dv.lastElapsedBeats)) + 2*beatCounterPillPadX
		counterSlotW := readoutW
		if ub := beatCounterSlotWidth(); counterSlotW > ub {
			counterSlotW = ub
		}
		// Clamp to the band itself. On narrow phones this keeps the essential
		// readout from being starved below its text width (the prior half-band
		// clamp clipped "Beat 1" with the notif slot painted over the rest —
		// screenshot review); the readout wins over the notif area on overflow.
		if counterSlotW > dv.beatCounterRect.Dx() {
			counterSlotW = dv.beatCounterRect.Dx()
		}
		dv.beatCounterRect.Max.X = dv.beatCounterRect.Min.X + counterSlotW
		notifLeft := dv.beatCounterRect.Max.X + SpaceSM
		if notifLeft < bandRight {
			dv.notifRect = image.Rect(notifLeft, dv.beatCounterRect.Min.Y, bandRight, dv.beatCounterRect.Max.Y)
		} else {
			dv.notifRect = image.Rectangle{}
		}
	} else {
		dv.notifRect = image.Rectangle{}
	}

	// Delegate timeline zone layout when available.
	if dv.timelineZone != nil && dv.tree != nil {
		// The timeline zone hosts the chrome above the bar (beat counter pill,
		// track button, len +/- buttons) AND the bar itself AND the steps
		// grid below. The clip rect must cover all three vertical bands AND
		// the full horizontal span — len buttons live just past
		// `timelineRect.Max.X` and the track button lives at
		// `tlWidget.Min.X` (left of the bar). Earlier code used
		// `dv.timelineRect.{Min,Max}.X` and `dv.timelineRect.Min.Y` for the
		// clip, which silently clipped the chrome above and outside the bar
		// — beat counter pill rendered into a clipped sub-image, no pixels
		// reached the screen.
		rowsBottom := dv.rowsBottom()
		if p.IsMobile() && dv.MobileEQMode() {
			rowsBottom = dv.Bounds.Max.Y
		}
		tlZoneRect := tlWidget
		if tlZoneRect.Max.Y < rowsBottom {
			tlZoneRect.Max.Y = rowsBottom
		}
		// Union with the chrome rects so a future relayout that pushes any
		// element outside `tlWidget` still gets clipped in (defense in depth).
		tlZoneRect = tlZoneRect.Union(dv.beatCounterRect)
		tlZoneRect = tlZoneRect.Union(dv.timelineRect)
		if r := dv.trackBtn().Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		if r := dv.lenIncBtn.Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		if r := dv.lenDecBtn.Rect(); !r.Empty() {
			tlZoneRect = tlZoneRect.Union(r)
		}
		dv.timelineZone.SetTimelineBarHeight(tlBarHeight())
		dv.timelineZone.SetTimelineBarRect(dv.timelineRect)
		dv.tree.SetZoneRect("timeline", tlZoneRect)
		dv.tree.LayoutZoneNow("timeline")
	}

	// EQ panel is anchored to the Wave widget; if missing, fall back to the bottom of the timeline widget.
	eqWidget := dv.widgetRects[WidgetWave]
	if eqWidget.Empty() {
		eqWidget = image.Rect(tlWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, tlWidget.Max.X, dv.Bounds.Max.Y)
	}
	// When the user has explicitly dragged the EQ divider, the panel height is
	// dv.eqH (which already honors the clamp and overrides the floor). Anchor the
	// rect to it and SKIP the analysis-tab floor expansion below — otherwise the
	// floor re-inflates the panel and the divider drag has no effect (the
	// "unusable divider" bug). Phase 0a otherwise expands every analysis tab to
	// AudioPanelHeightMultiplier × eqPanelHeight (≈3×), clamped to
	// AudioPanelHeightScreenFrac × bounds.Dy, because the widget-board's
	// WidgetWave allocation is too short for the Spectrum/Levels/Chain chrome.
	if dv.userEqH > 0 && !Profile().IsMobile() {
		eqWidget = image.Rect(eqWidget.Min.X, dv.Bounds.Max.Y-dv.eqH, eqWidget.Max.X, dv.Bounds.Max.Y)
	} else if dv.eqPanelZone != nil && dv.eqPanelZone.tabState != nil {
		minPanelH := dv.eqPanelZone.tabState.PanelHeightAt(dv.Bounds.Dy())
		// Synth keeps its prior 240 px floor as a separate hard minimum
		// (header + sections + chrome) — never shorter than that even
		// if the runtime-profile multiplier resolves smaller.
		const minSynthPanelH = 240
		if dv.eqPanelZone.ActiveTab() == TabSynth && minPanelH < minSynthPanelH {
			minPanelH = minSynthPanelH
		}
		if eqWidget.Dy() < minPanelH {
			expanded := image.Rect(
				eqWidget.Min.X,
				dv.Bounds.Max.Y-minPanelH,
				eqWidget.Max.X,
				dv.Bounds.Max.Y,
			)
			// Don't overflow the drum view's top edge — if the bounds
			// truly are smaller than minPanelH, give the panel every
			// available pixel.
			if expanded.Min.Y < dv.Bounds.Min.Y {
				expanded.Min.Y = dv.Bounds.Min.Y
			}
			eqWidget = expanded
		}
	}
	dv.eqRect = eqWidget
	// Mobile EQ mode: use full drum pane area below the transport header.
	// Theme 1: clamp the panel to sit ABOVE the bottom action bar so the
	// 6-segment view switcher remains visible (and tappable) in every
	// mobile mode.
	if p.IsMobile() && dv.MobileEQMode() {
		maxY := dv.Bounds.Max.Y
		if !dv.bottomActionBarRect.Empty() && dv.bottomActionBarRect.Min.Y > 0 {
			maxY = dv.bottomActionBarRect.Min.Y - 1
		}
		dv.eqRect = image.Rect(
			dv.Bounds.Min.X,
			dv.Bounds.Min.Y+dv.headerH,
			dv.Bounds.Max.X,
			maxY,
		)
	}
	// EQ buttons, sliders, and rect layout are owned by EQPanelZone (Phase 2).
	// Delegate layout to the zone, which sets all button/slider rects.
	if len(dv.eqBandVals) != len(eqBandDefs) {
		dv.eqBandVals = make([]float64, len(eqBandDefs))
	}
	if dv.eqPanelZone != nil && dv.audioTree != nil {
		dv.audioTree.SetZoneRect("eq-panel", dv.eqRect)
		// Force immediate layout so slider/button rects are available
		// before Draw or the next Update cycle (fixes first-frame
		// clicks). LayoutZoneNow consults the zone's registered
		// visibility predicate (the canonical "should the EQ panel
		// paint?" decision in drumview_ctor.go): hidden zones get their
		// HitIndex entry cleared so the panel's catch-all can never
		// re-introduce the Pads-tab input leak.
		dv.audioTree.LayoutZoneNow("eq-panel")
	}

	// Layout resize zone — refresh hit areas so column/row divider pills
	// stay clickable after bounds changes and widget board resizes.
	if dv.layoutResizeZone != nil && dv.audioTree != nil {
		dv.audioTree.HitIndexRef().Update("layout-resize", dv.layoutResizeZone.HitAreas())
	}

	// EQ peek strip tap target (mobile, EQ-collapsed only). A single tap
	// inside the 24-px sparkline strip expands the EQ panel — saving the
	// user from hunting through the overflow menu. Registered as a
	// drumview-owned hit area so the tree's input dispatcher routes taps
	// here before they reach any zone below.
	if dv.tree != nil {
		var peekAreas []HitArea
		if !dv.eqPeekRect.Empty() {
			peekAreas = []HitArea{{
				Rect:    dv.eqPeekRect,
				ZIndex:  150, // above bottom action bar buttons
				Handler: &eqPeekHitAdapter{dv: dv},
				Tag:     "eq-peek-expand",
			}}
		}
		dv.tree.HitIndexRef().Update("drumview-eq-peek", peekAreas)

		// Row-zoom chip hit areas — Theme 3 of the mobile UI consistency
		// pass. ⊕/⊖ buttons live above the rows zone on mobile only;
		// rect zeroed elsewhere by recalcButtons.
		var zoomAreas []HitArea
		if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
			zoomAreas = append(zoomAreas, HitArea{
				Rect:    dv.rowZoomIncBtn.Rect(),
				ZIndex:  140, // above rack rows, below overlays
				Handler: &buttonHitAdapter{btn: dv.rowZoomIncBtn},
				Tag:     "drumview-row-zoom-in",
			})
		}
		if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
			zoomAreas = append(zoomAreas, HitArea{
				Rect:    dv.rowZoomDecBtn.Rect(),
				ZIndex:  140,
				Handler: &buttonHitAdapter{btn: dv.rowZoomDecBtn},
				Tag:     "drumview-row-zoom-out",
			})
		}
		dv.tree.HitIndexRef().Update("drumview-row-zoom", zoomAreas)
	}

	// Register focusable rects for mobile soft keyboard gesture-based focus.
	// On small screens, the mobile native input system handles text inputs
	// directly (creating real HTML <input> overlays), so we skip focus-rect
	// registration to avoid the proxy also capturing the touchend gesture.
	softKeyboardClearRects()
	if !p.IsMobile() {
		if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
			r := dv.bpmBox().Rect
			softKeyboardRegisterRect("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "numeric")
		}
		if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			softKeyboardRegisterRect("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			softKeyboardRegisterRect("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
		if dv.renameBox != nil && !dv.renameBox.Rect.Empty() {
			r := dv.renameBox.Rect
			softKeyboardRegisterRect("rename", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "text")
		}
	}

	// Register mobile native input rects (replaces focus-rects for text inputs on mobile)
	if p.IsMobile() {
		mobileInputClear()

		// BPM box — direct rect, re-registered every layout pass. The JS
		// native-input system creates the real <input> synchronously inside the
		// touchend gesture (onCanvasTouchEnd), which only fires when "bpm" is
		// already in the registrations map at touchend time. The shared editor's
		// OpenValue registers "bpm" only after Go processes the tap (1-2 frames
		// later, after this layout's mobileInputClear() has wiped it), so it can
		// never satisfy the gesture handler — registration must live here.
		if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
			r := dv.bpmBox().Rect
			mobileInputRegister("bpm", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.bpmBox().Text, 4, "numeric")
		}

		// Rename — register trigger only when context menu is open.
		// The "Rename" button in the context menu is the trigger (index 1).
		// We must NOT register the kebab button itself as a trigger, otherwise
		// the JS touchend handler intercepts the tap and creates a native
		// rename input instead of letting the context menu open.
		if dv.IsContextMenuOpen() && dv.contextMenuRow >= 0 &&
			dv.contextMenuRow < len(dv.Rows) && dv.contextMenuRow < len(dv.rowLabels()) &&
			len(dv.contextMenuBtns) > 1 {
			renameBtn := dv.contextMenuBtns[1] // "Rename" is index 1
			trigR := renameBtn.Rect()
			labelR := dv.rowLabels()[dv.contextMenuRow].Rect()
			if !trigR.Empty() && !labelR.Empty() {
				mobileInputRegisterTrigger(
					fmt.Sprintf("rename-%d", dv.contextMenuRow),
					trigR.Min.X, trigR.Min.Y, trigR.Dx(), trigR.Dy(),
					labelR.Min.X, labelR.Min.Y, labelR.Dx(), labelR.Dy(),
					dv.Rows[dv.contextMenuRow].Name, 32, "text",
				)
			}
		}

		// Instrument search — direct rect (when inst menu is open)
		if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
			r := dv.instSearchRect
			mobileInputRegister("inst-search", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.instSearchBox.Text, 40, "text")
		}

		// WAV name — direct rect (when naming)
		if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
			r := dv.nameBox.Rect
			mobileInputRegister("wav-name", r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
				dv.nameBox.Text, 32, "text")
		}
	}
}

// resetOnScreenModeChange resets DrumView state when crossing the mobile ↔ desktop
// threshold. Called from Game.Layout() when the profile's Class changes.
func (dv *DrumView) resetOnScreenModeChange(toSmall bool) {
	pp := Profile() // use fresh profile for the new mode
	if !toSmall {
		// Leaving mobile → desktop: reset mobile flags for next entry.
		// SetMobileEQMode(false) routes through setViewMode, which is the
		// only code path allowed to mutate currentViewMode. Don't write
		// the field directly here — the AST guard test forbids it.
		dv.mobileEQInited = false
		dv.SetMobileEQMode(false)
		dv.mobileEQCollapsed = false
		dv.closeVolumePopup()
		if dv.widgets != nil {
			dv.widgets.SetWeights(pp.ColWeights, pp.RowWeights)
			dv.widgets.ToggleWidget(WidgetWave, true)
		}
	} else {
		// Entering mobile: reset so recalcButtons mobile-init runs
		dv.mobileEQInited = false
		if dv.widgets != nil {
			dv.widgets.SetWeights(pp.ColWeights, pp.RowWeights)
		}
	}
	// Common: invalidate all caches (row height 24↔44px change)
	dv.labelWidthDirty = true
	dv.bgDirty = true
	dv.markAllRowsDirty()
	dv.markRowControlsDirty()
	dv.invalidateRowCaches()
	dv.rowsLayerDirty = true
	dv.toolbarCache = nil
	dv.toolbarCacheHash = 0
	// Force a widget-layout refresh now so derived sizes (headerH, eqH,
	// controlsW, widgetRects) propagate even if the subsequent SetBounds
	// short-circuits because new bounds happen to equal old ones. Defense
	// in depth — SetBounds normally calls this too.
	dv.refreshWidgetLayout()
	// Re-derive profile-dependent button chrome immediately so any draw
	// path that runs before the next recalcButtons (e.g. zone Draw fired
	// from this same frame) sees the new-profile styles.
	dv.refreshLenButtonsStyle()
	dv.CloseAllPopups()
}

// clampLength enforces global min/max zoom limits for the drum view.
// Minimum: one full beat (timelineUnitsPerBeat). Maximum: derived from
// the effective cell-drawing width and a platform-aware minimum cell width
// so cells stay visually readable on small screens.
func (dv *DrumView) clampLength(n int) int {
	inc := max1(dv.timelineUnitsPerBeat)
	minLen := inc

	// Use the same effective width that calcLayout uses for cell sizing.
	w := dv.timelineRect.Dx()

	if w > 0 {
		mcw := MinCellWidth()
		if mcw < 1 {
			mcw = 1
		}
		maxLen := w / mcw
		if maxLen < minLen {
			maxLen = minLen
		}
		if n > maxLen {
			n = maxLen
		}
	}

	if n < minLen {
		n = minLen
	}
	return n
}

// changeLength applies a new drum length with clamping and refreshes row
// buffers/caches. Callers should supply the desired length in subdivisions.
func (dv *DrumView) changeLength(newLen int) {
	newLen = dv.clampLength(newLen)
	if newLen == dv.Length {
		return
	}
	dv.lengthChanging = true
	if newLen > dv.Length {
		dv.logger.Debugf("[drumview] length increased to: %d", newLen)
	} else {
		dv.logger.Debugf("[drumview] length decreased to: %d", newLen)
	}
	oldLen := dv.Length
	dv.Length = newLen
	for _, r := range dv.Rows {
		newSteps := make([]bool, dv.Length)
		newTypes := make([]model.NodeType, dv.Length)
		n := oldLen
		if dv.Length < n {
			n = dv.Length
		}
		copy(newSteps[:n], r.Steps[:n])
		copy(newTypes[:n], r.CellTypes[:n])
		r.Steps = newSteps
		r.CellTypes = newTypes
	}
	dv.SetBeatLength(dv.Length) // Update graph's beat length
	dv.bgDirty = true
	dv.markAllRowsDirty()
}

// rowRectForIndex, positionRowWidgets, and positionAddRowBtn are now
// handled exclusively by RowRackZone.

// rowsBottom returns the Y at which the rows area ends — above the EQ
// panel and the mobile bottom action bar. Use this everywhere instead
// of inlining the formula so layout invariants stay in one place.
func (dv *DrumView) rowsBottom() int {
	rb := dv.Bounds.Max.Y - dv.eqH
	if !dv.bottomActionBarRect.Empty() {
		rb -= dv.bottomActionBarRect.Dy()
	}
	if !dv.eqPeekRect.Empty() {
		rb -= dv.eqPeekRect.Dy()
	}
	return rb
}

// nameBoxRect returns the fixed rect for the WAV naming input box.
func (dv *DrumView) nameBoxRect() image.Rectangle {
	return image.Rect(dv.Bounds.Min.X+10, dv.Bounds.Min.Y+110, dv.Bounds.Min.X+300, dv.Bounds.Min.Y+150)
}

// SuppressLayout prevents calcLayout from running until ResumeLayout is called.
// Use during bulk operations (e.g., import) to avoid O(rows × instruments) work.
func (dv *DrumView) SuppressLayout() { dv.layoutSuppressed = true }

// ResumeLayout re-enables layout and performs a single recalculation.
func (dv *DrumView) ResumeLayout() {
	dv.layoutSuppressed = false
	dv.invalidateLabelCaches()
	dv.calcLayout()
}

func (dv *DrumView) calcLayout() {
	if dv.layoutSuppressed {
		return
	}
	dv.calcLabelWidth()
	if len(dv.Rows) > 0 {
		w := dv.timelineRect.Dx()
		if w <= 0 {
			w = dv.Bounds.Dx() - dv.labelW - dv.controlsW
		}
		dv.cell = w / len(dv.Rows[0].Steps)
		if dv.cell < 1 {
			dv.cell = 1
		}
	}
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.rowsBottom())
	}
	// Clamp rack top to match capped headerH (widget board may allocate more).
	if panelRect.Min.Y < rowsTop {
		panelRect.Min.Y = rowsTop
	}
	// RowRackZone owns per-row buttons/sliders and the add-row button.
	// Ensure the zone's entries match the current row count by triggering
	// a re-layout — through the tree, so the HitIndex is republished in
	// the same step. Accessor methods on DrumView delegate to the zone.
	dv.tree.LayoutZoneNow("row-rack")
	// If timeline dimensions changed, row sprite caches must be rebuilt.
	if dv.rowCacheW != dv.timelineRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		dv.rowCacheW = dv.timelineRect.Dx()
		dv.rowCacheH = dv.rowHeight()
		dv.markAllRowsDirty()
	}
	// Seed a sane default popup rect so layout tests have dimensions even
	// before the menu opens.
	if dv.instMenuFullRect.Dx() == 0 {
		minMenuW := dv.labelW + dv.controlsW/2
		if minMenuW < 260 {
			minMenuW = 260
		}
		hostW := dv.widgetRects[WidgetRack].Dx()
		if hostW == 0 {
			hostW = dv.Bounds.Dx()
		}
		if minMenuW > hostW {
			minMenuW = hostW
		}
		dv.instMenuFullRect = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y, dv.Bounds.Min.X+minMenuW, dv.Bounds.Min.Y+dv.rowHeight()*3)
	}
	// Invalidate row controls cache when button positions change.
	dv.markRowControlsDirty()
}

// invalidateLabelCaches marks label-related caches dirty so they are
// recomputed on the next frame. Call this when instruments or rows change.
func (dv *DrumView) invalidateLabelCaches() {
	dv.labelWidthDirty = true
	dv.instLabelCache = nil
}

// calcLabelWidth adjusts labelW to fit the longest row or instrument label
// while keeping it within half of the viewport width.
func (dv *DrumView) calcLabelWidth() {
	// Fast path: return cached value if not dirty and data hasn't changed.
	maxAllowed := dv.Bounds.Dx() / 2
	// Check if underlying data has changed (catches direct modifications).
	dataChanged := len(dv.Rows) != dv.labelCacheRowCount || len(dv.instOptions) != dv.labelCacheInstLen
	if !dataChanged && len(dv.Rows) == len(dv.labelCacheRowNames) {
		// Quick check if any row name differs from cached.
		for i, r := range dv.Rows {
			if r.Name != dv.labelCacheRowNames[i] {
				dataChanged = true
				break
			}
		}
	}
	if !dv.labelWidthDirty && !dataChanged && dv.cachedLabelW > 0 {
		// Re-apply maxAllowed constraint in case bounds changed without
		// invalidating the cache (e.g., window resize).
		target := dv.cachedLabelW
		if target > maxAllowed {
			target = maxAllowed
		}
		dv.labelW = target
		return
	}

	maxPx := 0
	for _, r := range dv.Rows {
		if w := TextWidth(r.Name); w > maxPx {
			maxPx = w
		}
	}
	for _, id := range dv.instOptions {
		lbl := dv.instDisplayLabel(id)
		if w := TextWidth(lbl); w > maxPx {
			maxPx = w
		}
	}
	pad := SpaceXS*2 + 12
	target := maxPx + pad
	if target < 80 {
		target = 80
	}
	// Cache the pre-clamped value so bounds changes don't require full recompute.
	dv.cachedLabelW = target
	dv.labelCacheRowCount = len(dv.Rows)
	dv.labelCacheInstLen = len(dv.instOptions)
	// Cache row names to detect direct modifications.
	if cap(dv.labelCacheRowNames) < len(dv.Rows) {
		dv.labelCacheRowNames = make([]string, len(dv.Rows))
	} else {
		dv.labelCacheRowNames = dv.labelCacheRowNames[:len(dv.Rows)]
	}
	for i, r := range dv.Rows {
		dv.labelCacheRowNames[i] = r.Name
	}
	if target > maxAllowed {
		target = maxAllowed
	}
	dv.labelW = target
	dv.labelWidthDirty = false
}

// eqPeekHitAdapter routes a tap on the mobile EQ peek strip to expand
// the EQ panel. Mirrors the view-cycle audio-view entry: clears the
// collapsed flag and switches the mobile view mode to audio so the
// next layout pass allocates the full-pane EQ rect.
type eqPeekHitAdapter struct {
	dv *DrumView
}

func (h *eqPeekHitAdapter) OnPress(x, y int) InputResult {
	if globalTouchState != nil && globalTouchState.RecentMultiTouch() {
		return InputIgnored
	}
	if h.dv == nil {
		return InputIgnored
	}
	h.dv.mobileEQCollapsed = false
	h.dv.setViewMode(viewModeEQ)
	h.dv.bgDirty = true
	return InputCaptured
}

func (h *eqPeekHitAdapter) OnDrag(x, y int)                     {}
func (h *eqPeekHitAdapter) OnRelease(x, y int)                  {}
func (h *eqPeekHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// segmentedHitAdapter routes taps on the mobile Pads/EQ/Wave segmented
// control. HitTest dispatches the segment click (which calls setViewMode
// via the onClick callback set at construction time in drumview_ctor.go).
type segmentedHitAdapter struct{ sc *SegmentedControl }

func (h *segmentedHitAdapter) OnPress(x, y int) InputResult {
	if h.sc.HitTest(x, y) {
		return InputCaptured
	}
	return InputIgnored
}
func (h *segmentedHitAdapter) OnDrag(x, y int)                     {}
func (h *segmentedHitAdapter) OnRelease(x, y int)                  {}
func (h *segmentedHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }
