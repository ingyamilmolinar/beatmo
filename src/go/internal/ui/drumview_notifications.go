package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

type notification struct {
	msg   string
	isErr bool
	ttl   int // frames remaining
}

func (dv *DrumView) notifyInfo(msg string)  { dv.pushNotif(msg, false) }
func (dv *DrumView) notifyError(msg string) { dv.pushNotif(msg, true) }
func (dv *DrumView) pushNotif(msg string, isErr bool) {
	ttl := 180
	if isErr {
		ttl = 240
	}
	dv.notifs = append(dv.notifs, notification{msg: msg, isErr: isErr, ttl: ttl})
	if len(dv.notifs) > 4 {
		dv.notifs = dv.notifs[len(dv.notifs)-4:]
	}
}

// anyDropdownOpen returns true if any dropdown menu is currently open.
// Delegates to the portal system which is the single source of truth.
func (dv *DrumView) anyDropdownOpen() bool {
	return dv.tree != nil && dv.tree.Portal().IsOpen()
}

// Portal-based accessors: read-only queries backed by portal.Has().
// The portal is the single source of truth for overlay state.

// IsSubdivMenuOpen returns whether the subdivision menu is open.
func (dv *DrumView) IsSubdivMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("subdiv-menu")
}

// IsInstMenuOpen returns whether the instrument menu is open.
func (dv *DrumView) IsInstMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("inst-menu")
}

// InstrumentMenuRect returns the on-screen rectangle of the open instrument
// menu (full-list bounds). Returns the zero rect when the menu is closed
// or its bounds have not been populated yet.
func (dv *DrumView) InstrumentMenuRect() image.Rectangle {
	if !dv.IsInstMenuOpen() {
		return image.Rectangle{}
	}
	return dv.instMenuFullRect
}

// IsColorMenuOpen returns whether the color wheel is open.
func (dv *DrumView) IsColorMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("color-wheel")
}

// IsContextMenuOpen returns whether the context menu is open.
func (dv *DrumView) IsContextMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("context-menu")
}

// IsOverflowMenuOpen returns whether the overflow menu is open.
func (dv *DrumView) IsOverflowMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("overflow-menu")
}

// IsFXPanelOpen returns whether the FX panel is open.
func (dv *DrumView) IsFXPanelOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("fx-panel")
}

// IsEQChannelOpen returns whether the EQ channel dropdown is open.
func (dv *DrumView) IsEQChannelOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("eq-channel")
}

// IsNamingOpen returns whether the WAV-naming dialog is open.
func (dv *DrumView) IsNamingOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("naming")
}

// anyDragActive reports whether any drag, scrub, slider, hold interaction,
// or mobile touch dead zone is in progress. When true, the tree skips
// dispatching new presses.
func (dv *DrumView) anyDragActive() bool {
	// Mobile touch dead zone: any real touch press in the row scroll area
	// blocks dispatch. Legitimate taps are replayed via tap injection.
	// Touches outside the row area (transport, EQ, portal overlays)
	// are not blocked, allowing slider and button interaction.
	touchDeadZone := Profile().IsMobile() && touchOverrideActive &&
		isMouseButtonPressed(ebiten.MouseButtonLeft) && !isTouchTapInjecting() &&
		!dv.MobileEQMode() &&
		pt(touchOverrideX, touchOverrideY, dv.rowsRect())

	return dv.rowScroll().Dragging() || dv.dragging || dv.scrubbing ||
		(dv.timelineZone != nil && (dv.timelineZone.IsDragging() || dv.timelineZone.IsScrubbing())) ||
		(dv.rowVolGroup() != nil && dv.rowVolGroup().Capturing()) ||
		(dv.transportZone != nil && dv.mainVolGroup() != nil && dv.mainVolGroup().Capturing()) ||
		dv.eqCurveDragBand >= 0 || dv.eqCurveDragFilter != "" ||
		dv.rowScroll().ScrollingCommitted() || touchDeadZone ||
		dv.instMenuScroll.dragging
}

// capturingDrag reports whether the drum view is physically handling a
// drag, scroll, or slider interaction. This excludes overlay/portal state
// so that the InputDispatcher capture is released when the touch moves
// outside drum bounds, allowing grid panning to work on mobile.
func (dv *DrumView) capturingDrag() bool {
	return dv.mouseDownInBounds || dv.anyDragActive() ||
		dv.rowScroll().TouchActive() ||
		(dv.renameComp != nil && dv.renameComp.IsOpen()) || dv.IsNamingOpen() ||
		(dv.layoutHandler != nil && dv.layoutHandler.Capturing())
}

// Capturing reports whether the drum view is actively handling a mouse drag
// (e.g. scrollbar, slider, scrubbing, or any overlay) and should therefore
// block camera panning and prevent input from passing through.
func (dv *DrumView) Capturing() bool {
	return dv.capturingDrag() || dv.anyDropdownOpen()
}

// logCapturingState logs a detailed breakdown of what makes Capturing() true.
// Used for diagnosing mobile panning regressions.
func (dv *DrumView) logCapturingState() {
	touchDZ := Profile().IsMobile() && touchOverrideActive &&
		isMouseButtonPressed(ebiten.MouseButtonLeft) && !isTouchTapInjecting() &&
		!dv.MobileEQMode() &&
		pt(touchOverrideX, touchOverrideY, dv.rowsRect())
	dv.logger.Debugf("[pan] Capturing breakdown: "+
		"mouseDown=%v anyDrag=%v touchActive=%v dropdown=%v rename=%v naming=%v layout=%v",
		dv.mouseDownInBounds, dv.anyDragActive(),
		dv.rowScroll().TouchActive(),
		dv.anyDropdownOpen(),
		dv.renameComp != nil && dv.renameComp.IsOpen(),
		dv.IsNamingOpen(),
		dv.layoutHandler != nil && dv.layoutHandler.Capturing())
	if dv.anyDragActive() {
		dv.logger.Debugf("[pan] anyDragActive breakdown: "+
			"scrollDrag=%v dragging=%v scrubbing=%v "+
			"tlDrag=%v tlScrub=%v "+
			"rowVol=%v mainVol=%v "+
			"eqBand=%v eqFilter=%v scrollCommit=%v touchDZ=%v instScroll=%v",
			dv.rowScroll().Dragging(), dv.dragging, dv.scrubbing,
			dv.timelineZone != nil && dv.timelineZone.IsDragging(),
			dv.timelineZone != nil && dv.timelineZone.IsScrubbing(),
			dv.rowVolGroup() != nil && dv.rowVolGroup().Capturing(),
			dv.transportZone != nil && dv.mainVolGroup() != nil && dv.mainVolGroup().Capturing(),
			dv.eqCurveDragBand >= 0, dv.eqCurveDragFilter != "",
			dv.rowScroll().ScrollingCommitted(), touchDZ,
			dv.instMenuScroll.dragging)
	}
	if dv.anyDropdownOpen() && dv.tree != nil {
		dv.logger.Debugf("[pan] portal stack size=%d", dv.tree.Portal().StackLen())
		for i := 0; i < dv.tree.Portal().StackLen(); i++ {
			if i < len(dv.tree.Portal().stack) {
				dv.logger.Debugf("[pan]   portal[%d] id=%q", i, dv.tree.Portal().stack[i].ID)
			}
		}
	}
}

// BlocksAt reports whether a point (x,y) lies over a temporary overlay such as
// the instrument dropdown or rename dialog. When true, clicks at that position
// should not reach underlying UI elements.
func (dv *DrumView) BlocksAt(x, y int) bool {
	if !image.Pt(x, y).In(dv.Bounds) {
		return false
	}
	if dv.anyDropdownOpen() || (dv.renameComp != nil && dv.renameComp.IsOpen()) || dv.IsNamingOpen() {
		return true
	}
	return false
}

type deletedRow struct {
	index  int
	origin model.NodeID
}
