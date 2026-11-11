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
	// Heading echoes the row name so the popup identifies its subject.
	dv.volPopup.SetTitle(func() string {
		r := dv.volPopupRow
		if r >= 0 && r < len(dv.Rows) {
			return dv.Rows[r].Name
		}
		return ""
	})
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
	dv.masterVolPopup.SetTitle(func() string { return "Master" })
	dv.masterVolPopup.Open(iconRect, dv.Bounds, dv.headerH)
	dv.openMasterVolPopupPortal()
}

func (dv *DrumView) closeMasterVolumePopup() {
	dv.masterVolPopup.Close()
	dv.closeMasterVolPopupPortal()
}

// OpenVolumePopup opens the per-row volume slider popup (mobile).
func (dv *DrumView) OpenVolumePopup(rowIdx int) { dv.openVolumePopup(rowIdx) }

// CloseVolumePopup closes the per-row volume slider popup.
func (dv *DrumView) CloseVolumePopup() { dv.closeVolumePopup() }

// OpenMasterVolumePopup opens the master volume slider popup (desktop).
func (dv *DrumView) OpenMasterVolumePopup() { dv.openMasterVolumePopup() }

// CloseMasterVolumePopup closes the master volume slider popup.
func (dv *DrumView) CloseMasterVolumePopup() { dv.closeMasterVolumePopup() }
