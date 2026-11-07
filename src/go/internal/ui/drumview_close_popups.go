package ui

func (dv *DrumView) CancelAllDeferredTaps() {
	dv.contextMenuDeferredTap.Cancel()
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
	dv.fxPanelDeferredTap.Cancel()
	if dv.fxScroll != nil {
		dv.fxScroll.ScrollBehavior().ResetTouch()
	}
	dv.instEditorDeferredTap.Cancel()
	dv.instEditorScrollTS.Reset()
	if g := dv.instEditorMacroGrid; g != nil {
		g.EndDrag()
	}
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

func (dv *DrumView) CloseAllPopups() {
	// Every popup lives in the one global portal.
	for dv.portal().IsOpen() {
		dv.portal().CloseTop()
	}
	if dv.volPopup != nil && dv.volPopup.IsOpen() {
		dv.volPopup.Close()
	}
	if dv.masterVolPopup != nil && dv.masterVolPopup.IsOpen() {
		dv.masterVolPopup.Close()
	}
	dv.closeEQKnobWheelPopup()
	dv.closeRename()
	dv.closeNaming()
	dv.closeNavMenuPortal()
	dv.CancelAllDeferredTaps()
}

// Runs on EVERY view-mode switch, so no departing tab can keep capture, focus or a modal portal
// that blocks input for the next view. Every new transient overlay must be torn down here.
func (dv *DrumView) resetTransientTabState() {
	dv.CloseAllPopups()
	dv.CancelSaveAsDialog()
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.CloseChannelDropdown()
	}
	if dv.rootTree != nil {
		dv.rootTree.ClearCapture()
	}
	if dv.tree != nil {
		dv.tree.SetFocus("")
	}
	if dv.audioTree != nil {
		dv.audioTree.SetFocus("")
	}
}
