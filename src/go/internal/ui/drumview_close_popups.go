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

// closeRename tears down rename state (renameComp).
func (dv *DrumView) closeRename() {
	dv.renameRow = -1
	if dv.renameComp != nil && dv.renameComp.IsOpen() {
		dv.renameComp.Close()
	}
	dv.closeRenamePortal()
}

// closeNaming tears down WAV-naming state.
func (dv *DrumView) closeNaming() {
	dv.pendingWAV = ""
	dv.nameInput = ""
	dv.nameBox = nil
	dv.closeNamingPortal()
}

// CloseAllPopups closes all open DrumView popups and overlays.
// Portal CloseTop invokes OnClose callbacks that handle component teardown,
// boolean clears, and deferred tap cancellation.
func (dv *DrumView) CloseAllPopups() {
	if dv.tree != nil {
		for dv.tree.Portal().IsOpen() {
			dv.tree.Portal().CloseTop()
		}
	}
	if dv.volPopup != nil && dv.volPopup.IsOpen() {
		dv.volPopup.Close()
	}
	if dv.masterVolPopup != nil && dv.masterVolPopup.IsOpen() {
		dv.masterVolPopup.Close()
	}
	dv.closeRename()
	dv.closeNaming()
	dv.CancelAllDeferredTaps()
}
