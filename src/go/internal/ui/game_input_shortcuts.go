package ui

import (
	"image"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// With a text input focused the browser keyboard goes to a hidden proxy <input>, so the canvas
// never sees Esc; the proxy forwards it through SignalSoftKeyboardEscape.
var softKeyboardEscPending int32

// Safe from the JS callback goroutine.
func SignalSoftKeyboardEscape() { atomic.StoreInt32(&softKeyboardEscPending, 1) }

func consumeSoftKeyboardEsc() bool { return atomic.SwapInt32(&softKeyboardEscPending, 0) == 1 }

// The hidden proxy holds focus, so the canvas never sees caret keys; the proxy forwards them.
// Counters, not flags: presses arriving between two slow frames must not coalesce into one move.
var (
	softKeyboardArrowLeftPending  int32
	softKeyboardArrowRightPending int32
)

// Safe from the JS callback goroutine.
func SignalSoftKeyboardArrowLeft()  { atomic.AddInt32(&softKeyboardArrowLeftPending, 1) }
func SignalSoftKeyboardArrowRight() { atomic.AddInt32(&softKeyboardArrowRightPending, 1) }

func consumeSoftKeyboardArrowLeft() int {
	return int(atomic.SwapInt32(&softKeyboardArrowLeftPending, 0))
}
func consumeSoftKeyboardArrowRight() int {
	return int(atomic.SwapInt32(&softKeyboardArrowRightPending, 0))
}

// Same bridge and counter semantics as the arrows.
var (
	softKeyboardHomePending int32
	softKeyboardEndPending  int32
)

// Safe from the JS callback goroutine.
func SignalSoftKeyboardHome() { atomic.AddInt32(&softKeyboardHomePending, 1) }
func SignalSoftKeyboardEnd()  { atomic.AddInt32(&softKeyboardEndPending, 1) }

func consumeSoftKeyboardHome() int { return int(atomic.SwapInt32(&softKeyboardHomePending, 0)) }
func consumeSoftKeyboardEnd() int  { return int(atomic.SwapInt32(&softKeyboardEndPending, 0)) }

// panStep is the per-frame pan distance (screen px) for arrow-key panning.
const panStep = 24.0

// zoomKeyStep mirrors a single wheel notch's strength in zoomAtScreen.
const zoomKeyStep = 8.0

// Runs before seqMu is taken, so it works everywhere but may touch only UI state: never the graph,
// the predictor or drum.Update. Esc first, so it cancels even over a focused text field.

// Ungated: works while a text field is focused.
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

// JustPressed, so a tap is one beat (OS auto-repeat still fires edges).
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

	// Before the ownership gate: a wheel popup claims only the arrows, so Enter would never reach it.
	if g.handleEnter() {
		return
	}

	// The single total-ownership predicate: surfaces declare themselves via KeyboardClaimant, never an
	// inline denylist here.
	if g.KeyboardClaimed() {
		return
	}

	// Partial claimants act first and report the keys they consumed; those are masked for the gated
	// nodes, so one arrow press never both moves a control and pans the camera.
	blocked := g.dispatchClaimedKeys()

	// Gated keys belong to per-component nodes; never add inline isKeyPressed checks here.
	g.keyboardRouter.dispatchGated(blocked)
}

// The single Settings entry point on every platform; empty before the button exists.
func (g *Game) gridHelpButtonRect() image.Rectangle {
	if g.gridHelpBtn == nil {
		return image.Rectangle{}
	}
	const sz = 30
	const inset = 10
	// The pane's true corner, not the +gridTopOffset content origin.
	gr := g.split.GridRect(g.winW, g.winH)
	x1 := gr.Max.X - inset
	y0 := gr.Min.Y + inset
	return image.Rect(x1-sz, y0, x1, y0+sz)
}

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

const overflowMenuID = "overflow-menu"

// The menus are mutually exclusive.
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

// The portal is the source of truth for each button's look, so however a menu closes the button
// rests next frame. While the pointer is up, clear a press whose release the menu swallowed, or the next
// tap is eaten; pointer-up guarantees no hold is in flight.
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
	// Anchored to the grid-pane gear on every screen size, so it never falls back to centering in
	// the drum pane.
	if g := dv.game; g != nil {
		ov.SetPositioner(func(w, h int) image.Rectangle {
			return g.gridAnchoredPopupRect(g.gridHelpButtonRect(), w, h, GridAnchorOpts{})
		})
	}
	// Non-modal with a scrim, so Esc, ×, an outside tap and the gear all close it.
	p.Open(menuCardEntry(settingsOverlayID, ov, image.Rectangle{}, false, nil))
}

// true keeps the portal open (Esc consumed, e.g. a search cleared); false lets the universal
// handler close it.
type portalEscapeHandler interface {
	HandleEscape() bool
}

// true consumes the key.
type portalEnterHandler interface {
	HandleEnter() bool
}

// The SINGLE authority for Esc: one cancel action per press down a precedence ladder; with
// nothing pending it requests Stop. Edge-triggered.
func (g *Game) handleEscape() bool {
	// Always consumed, so it cannot linger a frame.
	softEsc := consumeSoftKeyboardEsc()
	if !isKeyJustPressed(ebiten.KeyEscape) && !softEsc {
		return false
	}

	// The topmost overlay gets first dibs, so a component can clear its own field before closing.
	if g.drum.portal().IsOpen() {
		if h, ok := g.drum.portal().TopOverlay().(portalEscapeHandler); ok && h.HandleEscape() {
			return true // consumed; leave the portal open
		}
		g.drum.portal().CloseTop()
		return true
	}

	// Without a portal (tests) the confirm dialogs never got an overlay to receive Esc.
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

	// Not portals or focused zones, so without this they would fall through to Stop.
	if g.drum != nil && g.drum.CancelActiveTextDialog() {
		return true
	}

	// Before the sidebar: the menu sits above it. Close() discards a provisional group silently.
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		g.groupMenu.Close()
		return true
	}

	// 3. Node sidebar.
	if g.sidebar.IsOpen() {
		g.sidebar.Close()
		return true
	}

	// The same path the tree uses to route Esc to a focused zone.
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

	// Arming opens no portal, focus or mode, so without this rung Esc would stop playback.
	if g.drum != nil && g.drum.armedControlActive() {
		g.drum.lockSynthControl(g.drum.synthTabActiveInstrument())
		return true
	}

	// Back to the overview, not Stop.
	if g.drum != nil && g.drum.clearSynthFocusIfShown() {
		return true
	}

	// 10. Fallback: nothing pending → Stop playback.
	g.drum.TriggerStop()
	return true
}

// Edge-triggered; no fallback: Enter only acts when a wheel popup is on top.
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
