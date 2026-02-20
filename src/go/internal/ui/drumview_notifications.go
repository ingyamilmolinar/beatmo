package ui

import (
	"image"

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
// This is used to block input handlers when overlay menus are active.
// Add new dropdown states here to ensure consistent input blocking.
func (dv *DrumView) anyDropdownOpen() bool {
	return dv.isSubdivMenuOpen() || dv.isInstMenuOpen() || dv.isColorMenuOpen() || dv.eqChannelOpen || dv.overflowMenuOpen || dv.contextMenuOpen || dv.fxPanelOpen
}

// isInstMenuOpen returns true if the instrument menu is open via either
// the component or the legacy boolean.
func (dv *DrumView) isInstMenuOpen() bool {
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		return true
	}
	return dv.instMenuOpen
}

// isColorMenuOpen returns true if the color menu is open via either
// the component or the legacy boolean.
func (dv *DrumView) isColorMenuOpen() bool {
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		return true
	}
	return dv.colorMenuOpen
}

// isSubdivMenuOpen returns true if the subdiv menu is open via either
// the component or the legacy boolean.
func (dv *DrumView) isSubdivMenuOpen() bool {
	if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
		return true
	}
	return dv.subdivMenuOpen
}

// anyDragActive reports whether any drag, scrub, slider, or hold interaction
// is in progress. When true, button/label handlers should not fire.
func (dv *DrumView) anyDragActive() bool {
	return dv.rowScroll.Dragging() || dv.dragging || dv.scrubbing ||
		dv.activeSlider >= 0 || dv.activeSliderKind != sliderKindNone ||
		(dv.mainVolSlider != nil && dv.mainVolSlider.dragging) ||
		dv.rowScroll.ScrollingCommitted() ||
		dv.instMenuScroll.dragging || dv.colorHold || dv.instHold || dv.renameHold
}

// hasModalOverlay reports whether any modal overlay is open that should
// keep DrumView's InputDispatcher capture active.
func (dv *DrumView) hasModalOverlay() bool {
	if dv.overlays != nil {
		return dv.overlays.HasOpen()
	}
	return false
}

// Capturing reports whether the drum view is actively handling a mouse drag
// (e.g. scrollbar, slider, scrubbing, or any overlay) and should therefore
// block camera panning and prevent input from passing through.
func (dv *DrumView) Capturing() bool {
	return dv.mouseDownInBounds || dv.anyDragActive() ||
		dv.rowScroll.TouchActive() ||
		dv.hasModalOverlay() ||
		dv.renameBox != nil || dv.naming ||
		(dv.layoutHandler != nil && dv.layoutHandler.Capturing())
}

// BlocksAt reports whether a point (x,y) lies over a temporary overlay such as
// the instrument dropdown or rename dialog. When true, clicks at that position
// should not reach underlying UI elements.
func (dv *DrumView) BlocksAt(x, y int) bool {
	// If point is not within drum view bounds, don't block
	if !image.Pt(x, y).In(dv.Bounds) {
		return false
	}
	// Then check overlay states
	if dv.anyDropdownOpen() || dv.instHold || dv.renameBox != nil || dv.naming {
		return true
	}
	return false
}

type deletedRow struct {
	index  int
	origin model.NodeID
}
