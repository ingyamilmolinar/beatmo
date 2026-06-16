package ui

import (
	"image"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// softKeyboardEscPending bridges the WASM soft-keyboard proxy's Escape key to
// the universal Esc handler. On the browser, focusing a text input moves
// keyboard focus to a hidden HTML <input> (the "beatmo-kb-proxy"), so the
// canvas/ebiten never receives the Esc keydown and isKeyJustPressed(Escape)
// stays false — Esc did nothing for every text input. The JS proxy now forwards
// Escape by calling SignalSoftKeyboardEscape (via a registered js.Func), and
// handleEscape consumes the flag as if Esc were pressed on the canvas.
var softKeyboardEscPending int32

// SignalSoftKeyboardEscape marks that Esc was pressed while the soft-keyboard
// proxy held focus. Safe to call from the JS callback goroutine.
func SignalSoftKeyboardEscape() { atomic.StoreInt32(&softKeyboardEscPending, 1) }

// consumeSoftKeyboardEsc returns true once per pending soft-keyboard Escape,
// clearing the flag.
func consumeSoftKeyboardEsc() bool { return atomic.SwapInt32(&softKeyboardEscPending, 0) == 1 }

// softKeyboardArrowLeftPending / softKeyboardArrowRightPending bridge the WASM
// proxy's caret-navigation keys to the focused TextInput. Like Esc, ArrowLeft/
// ArrowRight land on the hidden proxy <input> (which holds keyboard focus while
// a field is active), so the canvas/ebiten never sees them and isKeyPressed
// stays false — caret navigation was dead on WASM. The JS proxy keydown handler
// forwards them via these signals; the focused TextInput.Update consumes them.
// Flag (not counter) semantics give one caret move per frame while held, which
// matches the desktop keyRepeat first-press cadence.
var (
	softKeyboardArrowLeftPending  int32
	softKeyboardArrowRightPending int32
)

// SignalSoftKeyboardArrowLeft / SignalSoftKeyboardArrowRight mark a caret-move
// key from the soft-keyboard proxy. Safe to call from the JS callback goroutine.
func SignalSoftKeyboardArrowLeft()  { atomic.StoreInt32(&softKeyboardArrowLeftPending, 1) }
func SignalSoftKeyboardArrowRight() { atomic.StoreInt32(&softKeyboardArrowRightPending, 1) }

// consumeSoftKeyboardArrowLeft / Right return true once per pending proxy arrow,
// clearing the flag.
func consumeSoftKeyboardArrowLeft() bool {
	return atomic.SwapInt32(&softKeyboardArrowLeftPending, 0) == 1
}
func consumeSoftKeyboardArrowRight() bool {
	return atomic.SwapInt32(&softKeyboardArrowRightPending, 0) == 1
}

// panStep is the per-frame pan distance (screen px) for arrow-key panning.
const panStep = 24.0

// zoomKeyStep mirrors a single wheel notch's strength in zoomAtScreen.
const zoomKeyStep = 8.0

// handleGlobalShortcuts dispatches keyboard shortcuts for common actions. It is
// called from Game.Update immediately after handleUndoRedoKeys (BEFORE seqMu is
// acquired), so — like undo/redo — it works everywhere (over popups, during
// drags) but must touch ONLY UI-layer state: camera, transport pulse flags,
// audio-tab state, and the overlay portal. It must never mutate the graph or
// predictor (those require seqMu) and must never call into drum.Update.
//
// Order matters: handleEscape runs first (so Esc still cancels even with a text
// field focused), then a focused text input swallows all remaining action keys
// (otherwise Space could not be typed into the BPM box / name fields).
// valueEditorActive reports whether the shared inline numeric editor is open on
// any surface — synth/sampler (dv.paramEditor) or EQ dB (the eq-panel zone's
// editor). Global keyboard shortcuts must yield to it so the editor's caret
// keys (Left/Right), commit (Enter), and revert (Esc) are not stolen by the
// grid-pan / transport shortcuts.
func (dv *DrumView) valueEditorActive() bool {
	if dv.paramEditor != nil && dv.paramEditor.Active() {
		return true
	}
	if dv.eqPanelZone != nil && dv.eqPanelZone.paramEditor != nil && dv.eqPanelZone.paramEditor.Active() {
		return true
	}
	if dv.transportZone != nil && dv.transportZone.paramEditor != nil && dv.transportZone.paramEditor.Active() {
		return true
	}
	return false
}

// cancelActiveValueEditor cancels whichever shared numeric editor is currently
// open (revert, no write) and reports whether one was active. Used by the Esc
// ladder so the soft-keyboard proxy's forwarded Escape reaches the editor even
// though the canvas never sees the keydown.
func (dv *DrumView) cancelActiveValueEditor() bool {
	if dv.paramEditor != nil && dv.paramEditor.Active() {
		dv.paramEditor.cancel()
		return true
	}
	if dv.eqPanelZone != nil && dv.eqPanelZone.paramEditor != nil && dv.eqPanelZone.paramEditor.Active() {
		dv.eqPanelZone.paramEditor.cancel()
		return true
	}
	if dv.transportZone != nil && dv.transportZone.paramEditor != nil && dv.transportZone.paramEditor.Active() {
		dv.transportZone.paramEditor.cancel()
		return true
	}
	return false
}

// undoShortcut handles Ctrl/Cmd+Z (undo), Shift+Ctrl/Cmd+Z and Ctrl/Cmd+Y
// (redo). Ungated node: works even while a text field is focused. Mirrors the
// pre-Phase-3 handleUndoRedoKeys body.
func (g *Game) undoShortcut() bool {
	if g.undoManager == nil {
		return false
	}
	ctrl := isKeyPressed(ebiten.KeyControlLeft) || isKeyPressed(ebiten.KeyControlRight) ||
		isKeyPressed(ebiten.KeyMetaLeft) || isKeyPressed(ebiten.KeyMetaRight)
	if !ctrl {
		return false
	}
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	if isKeyJustPressed(ebiten.KeyZ) {
		if shift {
			g.undoManager.Redo()
		} else {
			g.undoManager.Undo()
		}
		return true
	}
	if isKeyJustPressed(ebiten.KeyY) {
		g.undoManager.Redo()
		return true
	}
	return false
}

// gridShortcut handles the grid pane's keys: arrow-pan (level/continuous),
// [ ] zoom (level/continuous), 0 reset (edge). Returns whether any grid key
// acted this frame.
func (g *Game) gridShortcut() bool {
	acted := false
	panned := false
	if isKeyPressed(ebiten.KeyArrowLeft) {
		g.cam.OffsetX += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowRight) {
		g.cam.OffsetX -= panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowUp) {
		g.cam.OffsetY += panStep
		panned = true
	}
	if isKeyPressed(ebiten.KeyArrowDown) {
		g.cam.OffsetY -= panStep
		panned = true
	}
	if panned {
		g.cam.Snap()
		acted = true
	}
	if isKeyPressed(ebiten.KeyBracketRight) || isKeyPressed(ebiten.KeyBracketLeft) {
		cx := float64(g.split.GridW(g.winW)) / 2
		cy := float64(gridTopOffset()) + float64(g.split.GridH(g.winH))/2
		steps := zoomKeyStep
		if isKeyPressed(ebiten.KeyBracketLeft) {
			steps = -zoomKeyStep
		}
		g.zoomAtScreen(cx, cy, steps)
		acted = true
	}
	if isKeyJustPressed(ebiten.Key0) {
		g.cam.Reset()
		acted = true
	}
	return acted
}

// transportShortcut handles Space (play/pause) and BPM nudge (+/- by 1, incl.
// numpad). Returns whether any transport key acted.
func (g *Game) transportShortcut() bool {
	if g.drum == nil {
		return false
	}
	acted := false
	if isKeyJustPressed(ebiten.KeySpace) {
		g.drum.TriggerPlayPause()
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyEqual) || isKeyJustPressed(ebiten.KeyNumpadAdd) {
		g.drum.SetBPM(g.drum.BPM() + 1)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyMinus) || isKeyJustPressed(ebiten.KeyNumpadSubtract) {
		g.drum.SetBPM(g.drum.BPM() - 1)
		acted = true
	}
	return acted
}

// audioPanelShortcut handles number keys 1-7 (select audio-panel tab in display
// order) and "?" (toggle the settings overlay). Returns whether a key acted.
func (g *Game) audioPanelShortcut() bool {
	acted := false
	if g.drum != nil && g.drum.eqPanelZone != nil {
		tabs := AllPanelTabs()
		for k := ebiten.Key1; k <= ebiten.Key7; k++ {
			if isKeyJustPressed(k) {
				if idx := int(k - ebiten.Key1); idx < len(tabs) {
					g.drum.eqPanelZone.SetActiveTab(tabs[idx])
				}
				acted = true
				break
			}
		}
	}
	if isKeyJustPressed(ebiten.KeySlash) {
		g.toggleSettingsOverlay()
		acted = true
	}
	return acted
}

func (g *Game) handleGlobalShortcuts() {
	if g.drum == nil {
		return
	}

	// Esc is the universal cancel; it owns its own focus handling.
	if g.handleEscape() {
		return
	}

	// A surface that accepts typed/caret keys owns the keyboard this frame, so
	// the grid yields entirely (arrows reach the caret, not the camera). This is
	// the single keyboard-ownership predicate — every text-input surface declares
	// itself via KeyboardClaimant (keyboard_focus.go). Do NOT reintroduce an
	// inline denylist here; add new surfaces to KeyboardClaimed instead.
	if g.drum.KeyboardClaimed() {
		return
	}

	// Gated shortcut keys are owned by per-component nodes (keyboard_shortcuts.go):
	// grid (pan/zoom/reset), transport (Space, BPM±), audio panel (tabs, "?").
	// Do NOT add inline isKeyPressed checks here — add keys to a node instead.
	g.keyboardRouter.dispatchGated()
}

// gridHelpButtonRect is the screen-space rect of the settings gear button in
// the grid pane's top-right corner. Empty on mobile (the settings overlay is
// desktop-only) or before the button exists.
func (g *Game) gridHelpButtonRect() image.Rectangle {
	if g.gridHelpBtn == nil || Profile().IsMobile() {
		return image.Rectangle{}
	}
	const sz = 30
	const inset = 10
	// Hug the grid pane's true top-right corner (gridRect.Min is the pane's
	// top-left), not the +gridTopOffset content origin.
	gr := g.split.GridRect(g.winW, g.winH)
	x1 := gr.Max.X - inset
	y0 := gr.Min.Y + inset
	return image.Rect(x1-sz, y0, x1, y0+sz)
}

// settingsOverlayID identifies the settings overlay portal entry.
const settingsOverlayID = "settings"

// toggleSettingsOverlay opens the settings overlay, or closes it if already
// open.
func (g *Game) toggleSettingsOverlay() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.tree.Portal()
	if p == nil {
		return
	}
	if p.Has(settingsOverlayID) {
		p.Close(settingsOverlayID)
		return
	}
	g.drum.openSettingsOverlay()
}

// openSettingsOverlay opens the settings portal if not already open. Shared by
// the desktop gear toggle and the overflow-menu Settings entry (mobile + desktop).
func (dv *DrumView) openSettingsOverlay() {
	if dv.tree == nil {
		return
	}
	p := dv.tree.Portal()
	if p == nil || p.Has(settingsOverlayID) {
		return
	}
	p.Open(PortalEntry{
		ID: settingsOverlayID,
		Overlay: NewSettingsOverlay(func(l i18n.Locale) {
			i18n.SetLocale(l)
			persistLanguage(l)
		}),
		// The overlay paints its own full-screen scrim + centers itself, so
		// the portal scrim is left off. Non-modal: a click outside the panel
		// routes through the tree's click-outside → CloseTop.
		Modal: false,
		Scrim: false,
	})
}

// portalEscapeHandler lets a portal-hosted component intercept Esc before the
// universal handler closes its portal. Return true to KEEP the portal open
// (Esc fully consumed, e.g. a non-empty search field was cleared); return false
// to let the universal handler close the portal (the component may still run
// cleanup such as a cancel callback first).
type portalEscapeHandler interface {
	HandleEscape() bool
}

// handleEscape is the SINGLE authority for the Esc key. It walks a precedence
// ladder and performs exactly ONE cancel action, then returns true. With
// nothing pending, the final rung requests Stop. This consolidates what used to
// be scattered Esc checks (sidebar/connect/move in handleEditor, portal in the
// tree) into one coherent order. Edge-triggered so a held Esc fires once.
func (g *Game) handleEscape() bool {
	// Always consume the soft-keyboard Esc flag so it can't linger a frame.
	// Either source (canvas Esc keydown, or the WASM proxy forwarding) fires it.
	softEsc := consumeSoftKeyboardEsc()
	if !isKeyJustPressed(ebiten.KeyEscape) && !softEsc {
		return false
	}

	// 1+2. Open portal overlay (help, instrument menu, rename, dialogs). Give
	// the topmost overlay first dibs on Esc so a text-bearing component can
	// clear/cancel its own field (e.g. clear a search) before we close it.
	if g.drum.tree != nil && g.drum.tree.Portal() != nil && g.drum.tree.Portal().IsOpen() {
		if h, ok := g.drum.tree.Portal().TopOverlay().(portalEscapeHandler); ok && h.HandleEscape() {
			return true // consumed; leave the portal open
		}
		g.drum.tree.Portal().CloseTop()
		return true
	}
	// Audio-panel subtree portal (channel dropdown, synth-overflow-sheet).
	if g.drum.audioTree != nil && g.drum.audioTree.Portal() != nil && g.drum.audioTree.Portal().IsOpen() {
		if h, ok := g.drum.audioTree.Portal().TopOverlay().(portalEscapeHandler); ok && h.HandleEscape() {
			return true // consumed; leave the portal open
		}
		g.drum.audioTree.Portal().CloseTop()
		return true
	}

	// 2a2. Shared inline numeric editor (transport BPM, EQ dB, synth/sampler
	// knob readout). The editor self-handles a *canvas* Escape in its own
	// Update, but the soft-keyboard proxy forwards Esc via consumeSoftKeyboardEsc
	// (the canvas never sees the keydown), so it would never reach the editor.
	// Cancel it here (revert, no write) and consume the Esc. Edge-triggered, so
	// this rung also covers the canvas Esc before the editor's own Update sees it.
	if g.drum != nil && g.drum.cancelActiveValueEditor() {
		return true
	}

	// 2b. Inline text-input dialogs (Synth/Sampler "Save As") — not portals and
	// not focused zones, so without this they'd fall through to the Stop rung.
	// Cancel + close (discarding the typed name) and consume the Esc.
	if g.drum != nil && g.drum.CancelActiveTextDialog() {
		return true
	}

	// 3. Node sidebar.
	if g.sidebar.IsOpen() {
		g.sidebar.Close()
		return true
	}

	// 4. Focused text input — delegate Esc to the focused zone's own HandleKey,
	// which reverts the edit and blurs (transport BPM box, EQ dB inputs). This
	// reuses the exact path the tree uses to route Esc to a focused zone.
	if g.drum.tree != nil && g.drum.tree.focusedZone != "" {
		if z := g.drum.tree.FocusedZoneObj(); z != nil {
			z.HandleKey(ebiten.KeyEscape)
		}
		g.drum.tree.SetFocus("")
		return true
	}
	// Focused text input inside the audio-panel subtree (EQ dB inputs).
	if g.drum.audioTree != nil && g.drum.audioTree.focusedZone != "" {
		if z := g.drum.audioTree.FocusedZoneObj(); z != nil {
			z.HandleKey(ebiten.KeyEscape)
		}
		g.drum.audioTree.SetFocus("")
		return true
	}

	// 5. Connect mode.
	if g.connectMode {
		g.cancelConnectMode()
		return true
	}

	// 6. Move mode.
	if g.moveMode {
		g.cancelMoveMode()
		return true
	}

	// 7. In-progress link drag.
	if g.linkDrag.active {
		g.linkDrag = dragLink{}
		return true
	}

	// 8. In-flight tree capture (a drag handler holds capture).
	if (g.drum.tree != nil && g.drum.tree.Capturing()) ||
		(g.drum.audioTree != nil && g.drum.audioTree.Capturing()) {
		if g.drum.rootTree != nil {
			g.drum.rootTree.ClearCapture()
		}
		return true
	}

	// 9. Active touch gesture (pinch/pan in flight).
	if globalTouchState.ActiveTouchCount() > 0 {
		globalTouchState.Reset()
		return true
	}

	// 10. Pending editor click state.
	if g.pendingClick || g.camDragged || g.pendingStartRow >= 0 {
		g.pendingClick = false
		g.camDragged = false
		g.pendingStartRow = -1
		return true
	}

	// 11. Fallback: nothing pending → Stop playback.
	g.drum.TriggerStop()
	return true
}
