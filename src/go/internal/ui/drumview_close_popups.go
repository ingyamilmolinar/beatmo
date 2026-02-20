package ui

// CancelAllDeferredTaps cancels any pending deferred taps without firing
// them. Called when a long press or other gesture should abort in-progress
// tap tracking.
func (dv *DrumView) CancelAllDeferredTaps() {
	dv.contextMenuDeferredTap.Cancel()
	dv.overflowDeferredTap.Cancel()
	dv.fxPanelDeferredTap.Cancel()
	dv.fxScrollTS.Reset()
	if dv.contextMenuScroll != nil {
		dv.contextMenuScroll.ResetTouch()
	}
}

// closeRename tears down rename state (renameBox, renameComp, hold flag).
func (dv *DrumView) closeRename() {
	wasOpen := dv.renameBox != nil || (dv.renameComp != nil && dv.renameComp.IsOpen())
	dv.renameBox = nil
	dv.renameRow = -1
	dv.renameHold = false
	if dv.renameComp != nil && dv.renameComp.IsOpen() {
		dv.renameComp.Close()
	}
	if wasOpen {
		SuppressClicksUntilMouseUp()
	}
}

// closeNaming tears down WAV-naming state.
func (dv *DrumView) closeNaming() {
	dv.naming = false
	dv.pendingWAV = ""
	dv.nameInput = ""
	dv.nameBox = nil
}

// CloseAllPopups closes all open DrumView popups and overlays.
func (dv *DrumView) CloseAllPopups() {
	dv.instMenuOpen = false
	dv.instMenuDeferredTap.Cancel()
	if dv.instMenuTouchScroll != nil {
		dv.instMenuTouchScroll.ResetTouch()
	}
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
		dv.instMenuComp.Close()
	}
	dv.colorMenuOpen = false
	dv.colorHold = false
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
		dv.colorWheelComp.Close()
	}
	dv.subdivMenuOpen = false
	dv.subdivDeferredTap.Cancel()
	if dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
		dv.subdivMenuComp.Close()
	}
	dv.eqChannelOpen = false
	dv.eqChDeferredTap.Cancel()
	if dv.eqChannelScroll != nil {
		dv.eqChannelScroll.ResetTouch()
	}
	dv.closeFXPanel()
	dv.closeOverflowMenu()
	dv.contextMenuOpen = false
	dv.contextMenuDeferredTap.Cancel()
	if dv.contextMenuScroll != nil {
		dv.contextMenuScroll.HandleDragEnd()
		dv.contextMenuScroll.ResetTouch()
	}
	dv.closeVolumePopup()
	dv.closeMasterVolumePopup()
	dv.closeEQPopup()
	dv.closeRename()
	dv.closeNaming()
}
