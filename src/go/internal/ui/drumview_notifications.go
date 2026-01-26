package ui

import (
	"image"

	"github.com/ingyamilmolinar/tunkul/core/model"
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
	return dv.subdivMenuOpen || dv.instMenuOpen || dv.colorMenuOpen || dv.eqChannelOpen
}

// Capturing reports whether the drum view is actively handling a mouse drag
// (e.g. scrollbar, slider, scrubbing, or any overlay) and should therefore
// block camera panning and prevent input from passing through.
func (dv *DrumView) Capturing() bool {
	return dv.scrollDrag || dv.activeSlider >= 0 || dv.scrubbing ||
		dv.dragging || dv.anyDropdownOpen() || dv.renameBox != nil || dv.naming ||
		dv.instMenuScroll.dragging || dv.colorHold || dv.instHold || dv.renameHold ||
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
