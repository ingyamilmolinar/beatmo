package ui

// CancelAllDeferredTaps cancels any pending deferred taps without firing
// them. Called when a long press or other gesture should abort in-progress
// tap tracking.
func (dv *DrumView) CancelAllDeferredTaps() {
	dv.contextMenuDeferredTap.Cancel()
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
	dv.fxPanelDeferredTap.Cancel()
	dv.fxScrollTS.Reset()
	dv.instEditorDeferredTap.Cancel()
	dv.instEditorScrollTS.Reset()
	for _, g := range dv.instEditorSectionGrids {
		if g != nil {
			g.EndDrag()
		}
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

// CloseAllPopups closes all open DrumView popups and overlays.
// Portal CloseTop invokes OnClose callbacks that handle component teardown,
// boolean clears, and deferred tap cancellation.
func (dv *DrumView) CloseAllPopups() {
	// Every popup (menus, pickers, FX panel, channel dropdown, wheel popups,
	// dialogs) lives in the single global overlay portal — drain it fully.
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
	dv.CancelAllDeferredTaps()
}

// resetTransientTabState tears down EVERY piece of transient, tab-local
// interaction state that must not survive a view-mode switch. It runs
// unconditionally at the single setViewMode chokepoint so every tab
// transition — toolbar button, segmented control, EQ peek tap — leaves no
// departing tab able to capture, block, or steal input destined for the
// next view, regardless of which view we are entering.
//
// This is the COMPLETE teardown chokepoint. Previously CloseAllPopups was
// only invoked from setViewMode when entering an audio tab (gated on
// MobileEQMode), so switching *into Pads* leaked any open portal/popup. A
// full-screen modal portal (e.g. the WAV-naming overlay, openNamingPortal)
// then filters the HitIndex to itself and blocks ALL input — the reported
// "Sampler->Pads makes input stop working" bug.
//
// Pins (every transient overlay/state an departing tab can hold):
//   - all portal overlays + row/master volume popups + rename + naming
//     (CloseAllPopups) — including any modal portal that would trap input.
//   - the Sampler/Synth Save-As dialog (a focused TextInput; NOT a portal,
//     so CloseAllPopups does not reach it — cancel it explicitly).
//   - the audio panel's channel dropdown (its `channelOpen` flag lives on the
//     zone, so reset it via the zone's own accessor — single source of truth).
//   - in-flight pointer capture (a live drag, e.g. holding a knob).
//   - keyboard/IME focus held by a departing tab's text field.
//
// New transient-overlay openers MUST be torn down here; the discipline test
// TestResetTransientTabState_TearsDownEveryOpener enforces it.
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
