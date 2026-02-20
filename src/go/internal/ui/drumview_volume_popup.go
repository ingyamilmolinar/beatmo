package ui

import (
	"image"
)

// openVolumePopup opens a vertical slider popup for the given row's volume.
func (dv *DrumView) openVolumePopup(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		return
	}
	dv.CloseAllPopups()

	iconRect := image.Rectangle{}
	if rowIdx < len(dv.rowVolSliders()) {
		iconRect = dv.rowVolSliders()[rowIdx].Rect()
	}
	if iconRect.Empty() {
		return
	}

	dv.volPopupRow = rowIdx
	dv.volPopup.Open(iconRect, dv.Bounds, dv.headerH)
	dv.openVolPopupPortal()
}

func (dv *DrumView) closeVolumePopup() {
	dv.volPopup.Close()
	dv.closeVolPopupPortal()
}

/* ─── master volume popup ─────────────────────────────────── */

// openMasterVolumePopup opens a vertical slider popup above the master volume icon.
func (dv *DrumView) openMasterVolumePopup() {
	dv.CloseAllPopups()
	iconRect := dv.mainVolIconRect
	if iconRect.Empty() {
		return
	}
	dv.masterVolPopup.Open(iconRect, dv.Bounds, dv.headerH)
	dv.openMasterVolPopupPortal()
}

func (dv *DrumView) closeMasterVolumePopup() {
	dv.masterVolPopup.Close()
	dv.closeMasterVolPopupPortal()
}
