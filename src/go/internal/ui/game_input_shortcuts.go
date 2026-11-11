package ui

import (
	"image"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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
// Counter (not flag) semantics: each discrete proxy keydown increments the
// pending count, and TextInput drains the whole count per frame. A flag would
// coalesce multiple distinct presses that arrive between two slow Update frames
// (software-WebGL / GPU-stall load) into a single caret move — dropping presses.
// This mirrors the charBuf accumulation used for typed characters.
var (
	softKeyboardArrowLeftPending  int32
	softKeyboardArrowRightPending int32
)

// SignalSoftKeyboardArrowLeft / SignalSoftKeyboardArrowRight mark a caret-move
// key from the soft-keyboard proxy. Safe to call from the JS callback goroutine.
func SignalSoftKeyboardArrowLeft()  { atomic.AddInt32(&softKeyboardArrowLeftPending, 1) }
func SignalSoftKeyboardArrowRight() { atomic.AddInt32(&softKeyboardArrowRightPending, 1) }

// consumeSoftKeyboardArrowLeft / Right return the number of pending proxy arrow
// presses since the last frame, clearing the counter.
func consumeSoftKeyboardArrowLeft() int {
	return int(atomic.SwapInt32(&softKeyboardArrowLeftPending, 0))
}
func consumeSoftKeyboardArrowRight() int {
	return int(atomic.SwapInt32(&softKeyboardArrowRightPending, 0))
}

// softKeyboardHomePending / softKeyboardEndPending bridge the WASM proxy's Home
// and End keys to the focused TextInput, exactly like the arrow signals above:
// the hidden proxy holds keyboard focus, so the canvas never sees Home/End and
// caret jump-to-start / jump-to-end were dead on WASM. Counter semantics match
// the arrows so nothing is dropped between slow frames (a single applied signal
// is idempotent — the count only guards against total loss).
var (
	softKeyboardHomePending int32
	softKeyboardEndPending  int32
)

// SignalSoftKeyboardHome / SignalSoftKeyboardEnd mark a caret-jump key from the
// soft-keyboard proxy. Safe to call from the JS callback goroutine.
func SignalSoftKeyboardHome() { atomic.AddInt32(&softKeyboardHomePending, 1) }
func SignalSoftKeyboardEnd()  { atomic.AddInt32(&softKeyboardEndPending, 1) }

// consumeSoftKeyboardHome / End return the number of pending proxy Home/End
// presses since the last frame, clearing the counter.
func consumeSoftKeyboardHome() int { return int(atomic.SwapInt32(&softKeyboardHomePending, 0)) }
func consumeSoftKeyboardEnd() int  { return int(atomic.SwapInt32(&softKeyboardEndPending, 0)) }

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

// undoShortcut handles Ctrl/Cmd+Z (undo), Shift+Ctrl/Cmd+Z, Ctrl/Cmd+Y and
// Ctrl/Cmd+R (redo). Ungated node: works even while a text field is focused.
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
			g.performRedo()
		} else {
			g.performUndo()
		}
		return true
	}
	if isKeyJustPressed(ebiten.KeyY) {
		g.performRedo()
		return true
	}
	if isKeyJustPressed(ebiten.KeyR) && !shift {
		g.performRedo()
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

// seekShortcut handles playhead navigation keys: "." = +1 beat, "," = -1 beat
// (Shift = ±4), Home = seek to 0. Uses isKeyJustPressed so a tap = one beat
// (auto-repeat, when the OS provides it, still fires JustPressed edges).
func (g *Game) seekShortcut() bool {
	if g.drum == nil {
		return false
	}
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	step := 1
	if shift {
		step = 4
	}
	acted := false
	if isKeyJustPressed(ebiten.KeyPeriod) {
		g.drum.RequestSeekDeltaBeats(step, hooks.SeekKeyboard)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyComma) {
		g.drum.RequestSeekDeltaBeats(-step, hooks.SeekKeyboard)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyHome) {
		g.drum.RequestSeekSteps(0, hooks.SeekKeyboard)
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

	// Enter accepts (persists) the topmost portal overlay that opts in — the
	// precision wheel popups — mirroring Esc→cancel. Runs before the keyboard-
	// ownership gate: a wheel popup claims only the ARROWS (it is a partial
	// claimant, not a total one), so Enter would otherwise never reach it.
	if g.handleEnter() {
		return
	}

	// A surface that accepts typed/caret keys owns the keyboard this frame, so
	// the grid yields entirely (arrows reach the caret, not the camera). This is
	// the single keyboard-ownership predicate — every text-input surface declares
	// itself via KeyboardClaimant (keyboard_focus.go). Do NOT reintroduce an
	// inline denylist here; add new surfaces to KeyboardClaimed instead.
	if g.KeyboardClaimed() {
		return
	}

	// A control that pops up over a tab (precision wheel, volume slider, armed
	// synth control) owns only the keys it actually handles — the arrows — and
	// lets every other key through. It acts FIRST and reports what it consumed;
	// those keys are then masked for the gated nodes below, so one arrow press
	// can never both move the control and pan the camera. This is the single
	// per-key ownership seam — see keyboard_key_claim.go.
	blocked := g.dispatchClaimedKeys()

	// Gated shortcut keys are owned by per-component nodes (keyboard_shortcuts.go):
	// grid (pan/zoom/reset), transport (Space, BPM±), audio panel (tabs, "?").
	// Do NOT add inline isKeyPressed checks here — add keys to a node instead.
	g.keyboardRouter.dispatchGated(blocked)
}

// gridHelpButtonRect is the screen-space rect of the settings gear button in
// the grid pane's top-right corner. Present on both desktop and mobile (the gear
// is the single Settings entry point on every platform). Empty before the button
// exists.
func (g *Game) gridHelpButtonRect() image.Rectangle {
	if g.gridHelpBtn == nil {
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

// gridFileButtonRect is the screen-space rect of the folder button, one SpaceSM
// gap to the LEFT of the settings gear in the grid pane's top-right corner.
// Same size as the gear. Present on both platforms. Empty before the button (or
// the gear) exists.
func (g *Game) gridFileButtonRect() image.Rectangle {
	if g.gridFileBtn == nil {
		return image.Rectangle{}
	}
	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		return image.Rectangle{}
	}
	w := gear.Dx()
	x1 := gear.Min.X - SpaceSM
	return image.Rect(x1-w, gear.Min.Y, x1, gear.Max.Y)
}

// openFileMenu opens the file-actions menu from the grid folder button.
// overflowMenuID identifies the File (overflow) menu portal entry.
const overflowMenuID = "overflow-menu"

// closeOtherGridButtonMenus closes every top-right button menu except keep, so at
// most one is ever open at a time (the menus are mutually exclusive).
func (g *Game) closeOtherGridButtonMenus(keep string) {
	if g.drum == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	for _, c := range g.gridCornerButtons() {
		if c.menuID != keep && p.Has(c.menuID) {
			p.Close(c.menuID)
		}
	}
}

func (g *Game) openFileMenu() {
	if g.drum == nil {
		return
	}
	g.closeOtherGridButtonMenus(overflowMenuID)
	g.drum.OpenFileMenu()
}

// reconcileGridButtonToggles keeps each top-right grid button's visual in sync
// with whether its menu is actually open, and heals a press whose release edge
// was swallowed by the menu/scrim the button opened. The portal's Has(menuID)
// is the single source of truth for these buttons' active look:
//
//   - toggled == menuOpen, every frame — so however a menu closes (re-tap,
//     click-outside / scrim, Esc, or a triggered system action such as the file
//     picker failing or being cancelled), the button drops back to rest on the
//     next frame instead of staying stuck "pressed" with its menu gone.
//   - while the pointer is UP, clear any latched press so the cap settles and
//     `held` returns to 0. A button that opened a menu never received its
//     release (the menu swallowed it), which both froze the cap down AND left
//     held==1, so the next tap incremented held to 2 and was swallowed — the
//     button went un-clickable. Gating on pointer-up guarantees no active hold
//     is in flight, so this never double-fires OnClick and always leaves the
//     button ready for a fresh click.
//
// pointerDown is this frame's unified left-mouse / primary-touch pressed state.
func (g *Game) reconcileGridButtonToggles(pointerDown bool) {
	if g.drum == nil {
		return
	}
	p := g.drum.portal()
	for _, c := range g.gridCornerButtons() {
		if c.btn == nil {
			continue
		}
		c.btn.SetToggled(p != nil && p.Has(c.menuID))
		if !pointerDown {
			c.btn.ReleaseStuckPress()
		}
	}
}

// toggleFileMenu opens the File (overflow) menu from the grid folder button, or
// closes it if already open (toggle), mirroring the gear/logs buttons.
func (g *Game) toggleFileMenu() {
	if g.drum == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if p.Has(overflowMenuID) {
		p.Close(overflowMenuID)
		return
	}
	g.openFileMenu()
}

// gridLogsButtonRect is the screen-space rect of the logs button, one SpaceSM
// gap to the LEFT of the folder button in the grid pane's top-right corner. Same
// size as the folder/gear. Present on both platforms. Empty before the button
// (or the folder button) exists.
func (g *Game) gridLogsButtonRect() image.Rectangle {
	if g.gridLogsBtn == nil {
		return image.Rectangle{}
	}
	file := g.gridFileButtonRect()
	if file.Empty() {
		return image.Rectangle{}
	}
	w := file.Dx()
	x1 := file.Min.X - SpaceSM
	return image.Rect(x1-w, file.Min.Y, x1, file.Max.Y)
}

// gridThemeButtonRect is the screen-space rect of the theme button, one SpaceSM
// gap to the LEFT of the logs button. Same size. Both platforms. Empty before
// the button (or the logs button) exists.
func (g *Game) gridThemeButtonRect() image.Rectangle {
	if g.gridThemeBtn == nil {
		return image.Rectangle{}
	}
	logs := g.gridLogsButtonRect()
	if logs.Empty() {
		return image.Rectangle{}
	}
	w := logs.Dx()
	x1 := logs.Min.X - SpaceSM
	return image.Rect(x1-w, logs.Min.Y, x1, logs.Max.Y)
}

// themeOverlayID identifies the theme editor portal entry.
const themeOverlayID = "theme"

// toggleThemeOverlay opens the Theme editor, or closes it if already open.
func (g *Game) toggleThemeOverlay() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if p.Has(themeOverlayID) {
		p.Close(themeOverlayID)
		return
	}
	g.closeOtherGridButtonMenus(themeOverlayID)
	g.drum.openThemeOverlay()
}

// toggleLogsPanel opens the Logs Navigation Panel, or closes it if already open.
func (g *Game) toggleLogsPanel() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if p.Has(logsPanelID) {
		p.Close(logsPanelID)
		return
	}
	g.closeOtherGridButtonMenus(logsPanelID)
	g.drum.openLogsPanel()
}

// settingsOverlayID identifies the settings overlay portal entry.
const settingsOverlayID = "settings"

// toggleSettingsOverlay opens the settings overlay, or closes it if already
// open.
func (g *Game) toggleSettingsOverlay() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if p.Has(settingsOverlayID) {
		p.Close(settingsOverlayID)
		return
	}
	g.closeOtherGridButtonMenus(settingsOverlayID)
	g.drum.openSettingsOverlay()
}

// openSettingsOverlay opens the settings portal if not already open. Shared by
// the desktop gear toggle and the overflow-menu Settings entry (mobile + desktop).
func (dv *DrumView) openSettingsOverlay() {
	if dv.tree == nil {
		return
	}
	p := dv.portal()
	if p == nil || p.Has(settingsOverlayID) {
		return
	}
	ov := NewSettingsOverlay(func(l i18n.Locale) {
		old := i18n.ActiveLocale()
		i18n.SetLocale(l)
		persistLanguage(l)
		emitLanguageChanged(old.String(), l.String())
	})
	// Desktop: drop the panel from the grid gear via the shared helper. Mobile
	// (UseBottomSheet) keeps the centered form — a 480px panel anchored to a
	// corner button is worse on a phone (see spec risk 3).
	// Anchor to the grid-pane gear on EVERY screen size (grid-pane-clamped), so
	// the panel never falls back to centering on the portal's screenBounds (=
	// dv.Bounds, the drum pane) and appearing in the drum-view panel — the
	// small-screen "menus in the drum view" bug.
	if g := dv.game; g != nil {
		ov.SetPositioner(func(w, h int) image.Rectangle {
			return g.gridAnchoredPopupRect(g.gridHelpButtonRect(), w, h, GridAnchorOpts{})
		})
	}
	// Shared menu behavior (menuCardEntry): non-modal + scrim, so Esc, the
	// card's close ×, a tap outside and the gear button itself all close it.
	p.Open(menuCardEntry(settingsOverlayID, ov, image.Rectangle{}, false, nil))
}

// portalEscapeHandler lets a portal-hosted component intercept Esc before the
// universal handler closes its portal. Return true to KEEP the portal open
// (Esc fully consumed, e.g. a non-empty search field was cleared); return false
// to let the universal handler close the portal (the component may still run
// cleanup such as a cancel callback first).
type portalEscapeHandler interface {
	HandleEscape() bool
}

// portalEnterHandler lets the topmost portal overlay accept (persist) on the
// Enter key, mirroring portalEscapeHandler's cancel-on-Esc. Currently the
// precision wheel popups opt in (Enter persists the edit). Return true to
// consume the key.
type portalEnterHandler interface {
	HandleEnter() bool
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
	if g.drum.portal().IsOpen() {
		if h, ok := g.drum.portal().TopOverlay().(portalEscapeHandler); ok && h.HandleEscape() {
			return true // consumed; leave the portal open
		}
		g.drum.portal().CloseTop()
		return true
	}

	// Headless / no-portal fallback: with a portal present, the topmost
	// overlay's HandleEscape (ConfirmDialog) already ran above (rung 1+2). This
	// covers the portal==nil path (tests) where beginPasteConfirm/beginBreakConfirm
	// never opened an overlay to receive Esc, so pasteConfirmActive/
	// breakConfirmActive would otherwise never get cancelled by this ladder.
	if g.drum.portal() == nil {
		if g.pasteConfirmActive() {
			g.CancelPasteOverwrite()
			return true
		}
		if g.breakConfirmActive() {
			g.CancelBreakCircuit()
			return true
		}
	}

	// 2b. Inline text-input dialogs (Synth/Sampler "Save As") — not portals and
	// not focused zones, so without this they'd fall through to the Stop rung.
	// Cancel + close (discarding the typed name) and consume the Esc.
	if g.drum != nil && g.drum.CancelActiveTextDialog() {
		return true
	}

	// 2c. GroupMenu (marquee-created group popup). Placed before the sidebar
	// rung: the menu is z-above the sidebar (210 vs 200), so Esc should close
	// the topmost surface first. The two are not expected to be open
	// simultaneously by design, but this ordering is defensive if they ever
	// are. Close() already discards a still-provisional group silently (no
	// emit/undo step) via discardProvisional — same zero-trace contract as
	// the close button and click-outside paths.
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		g.groupMenu.Close()
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

	// 5. Any armed grid mode (move/copy/connect).
	if g.modes.Active() != GridModeNone {
		g.modes.Cancel("esc")
		g.moveSkipRelease = false
		return true
	}

	// 6. In-progress link drag.
	if g.linkDrag.active {
		g.linkDrag = dragLink{}
		return true
	}

	// 7. In-flight tree capture (a drag handler holds capture).
	if (g.drum.tree != nil && g.drum.tree.Capturing()) ||
		(g.drum.audioTree != nil && g.drum.audioTree.Capturing()) {
		if g.drum.rootTree != nil {
			g.drum.rootTree.ClearCapture()
		}
		return true
	}

	// 8. Active touch gesture (pinch/pan in flight).
	if globalTouchState.ActiveTouchCount() > 0 {
		globalTouchState.Reset()
		return true
	}

	// 9. Pending editor click state.
	if g.pendingClick || g.camDragged || g.pendingStartRow >= 0 {
		g.pendingClick = false
		g.camDragged = false
		g.pendingStartRow = -1
		return true
	}

	// 9a. Armed synth control (Task 10). Arming a control opens no portal/
	// focus/mode, so without this rung Esc would fall through to the Stop
	// fallback below and reset playback as an unrequested side effect of
	// locking the armed control. Lock it and consume the Esc here instead.
	if g.drum != nil && g.drum.armedControlActive() {
		g.drum.lockSynthControl(g.drum.synthTabActiveInstrument())
		return true
	}

	// 9b. A focused Synth-tab dial (synth_focus.go): Esc returns the graph to
	// the overview instead of stopping playback.
	if g.drum != nil && g.drum.clearSynthFocusIfShown() {
		return true
	}

	// 10. Fallback: nothing pending → Stop playback.
	g.drum.TriggerStop()
	return true
}

// handleEnter is the Enter-key authority for the topmost portal overlay that
// opts in via portalEnterHandler (the precision wheel popups: Enter persists the
// edit and closes). Edge-triggered so a held Enter fires once. Returns true when
// it consumed the key. Unlike Esc there is no universal fallback — Enter only
// acts when a wheel popup is the top overlay.
func (g *Game) handleEnter() bool {
	if !isKeyJustPressed(ebiten.KeyEnter) {
		return false
	}
	if g.drum == nil || !g.drum.portal().IsOpen() {
		return false
	}
	if h, ok := g.drum.portal().TopOverlay().(portalEnterHandler); ok {
		return h.HandleEnter()
	}
	return false
}
