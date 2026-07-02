package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// notification is stored as a TRANSLATABLE reference (key + args), not a frozen
// rendered string, so the in-band area and the history popup display in the
// CURRENT locale and re-render when the language changes. key == "" falls back
// to the literal text (legacy persisted entries / ad-hoc plain strings).
type notification struct {
	key    i18n.Key // translation key; "" ⇒ use text verbatim
	args   []string // Tf args; an arg prefixed "@" is itself an i18n key (resolved at display)
	text   string   // rendered fallback when key == ""
	isErr  bool
	unixMs int64 // unix millis when raised; 0 in pure-unit tests
}

// display renders the notification in the current locale. Keyed entries
// re-render on every call (so the whole history follows a locale switch); plain
// entries return their frozen text. An arg beginning with "@" is an i18n key
// reference (undo/redo store the action name this way) and is itself translated.
func (n notification) display() string {
	key, args := n.key, n.args
	if key == "" {
		if n.text == "" {
			return ""
		}
		// Legacy / plain entry: reverse-map the frozen English text back to its
		// key so historical (incl. persisted) notifications retranslate when
		// drawn. Unknown strings are shown verbatim.
		k, a, ok := resolveLegacyNotif(n.text)
		if !ok {
			return n.text
		}
		key, args = k, a
	}
	out := make([]any, len(args))
	for i, a := range args {
		if len(a) > 1 && a[0] == '@' {
			out[i] = i18n.T(i18n.Key(a[1:]))
		} else {
			out[i] = a
		}
	}
	return i18n.Tf(key, out...)
}

// initNotifPersistence seeds the live ring from the persisted history (for
// the popup) and wires the write-through sink. Seeding does NOT mark a
// session entry, so the in-band area starts idle until the first new
// notification this session. Gated by UseNotificationsHistory.
func (dv *DrumView) initNotifPersistence() {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	if !UseNotificationsHistory {
		return
	}
	dv.notifStore.seed(NotificationHistory().Load())
	dv.notifStore.persist = func(snap []notification) {
		NotificationHistory().Save(snap)
	}
}

// notifyInfo/notifyError push a plain (already-rendered) string. Prefer the
// keyed variants below so the notification follows the current locale.
func (dv *DrumView) notifyInfo(msg string)  { dv.pushNotif(msg, false) }
func (dv *DrumView) notifyError(msg string) { dv.pushNotif(msg, true) }
func (dv *DrumView) pushNotif(msg string, isErr bool) {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	dv.notifStore.Push(notification{text: msg, isErr: isErr, unixMs: nowUnixMilli()})
}

// pendingNotif is an off-thread-queued notification carrying the i18n key +
// args (not a rendered string), so it renders in whatever locale is active when
// the UI goroutine drains it (game_update.go).
type pendingNotif struct {
	key   i18n.Key
	args  []string
	isErr bool
}

// notifyInfoKey/notifyErrorKey push a TRANSLATABLE notification (i18n key +
// string args) so it renders in whatever locale is active when displayed. An
// arg of the form "@some.key" is resolved as a nested translation key. This is
// the preferred API for every user-facing notification.
func (dv *DrumView) notifyInfoKey(key i18n.Key, args ...string)  { dv.pushNotifKey(key, args, false) }
func (dv *DrumView) notifyErrorKey(key i18n.Key, args ...string) { dv.pushNotifKey(key, args, true) }
func (dv *DrumView) pushNotifKey(key i18n.Key, args []string, isErr bool) {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	dv.notifStore.Push(notification{key: key, args: args, isErr: isErr, unixMs: nowUnixMilli()})
}

// anyDropdownOpen returns true if any dropdown menu is currently open.
// Delegates to the portal system which is the single source of truth.
func (dv *DrumView) anyDropdownOpen() bool {
	return (dv.tree != nil && dv.tree.Portal().IsOpen()) ||
		(dv.audioTree != nil && dv.audioTree.Portal().IsOpen())
}

// Portal-based accessors: read-only queries backed by portal.Has().
// The portal is the single source of truth for overlay state.

// IsSubdivMenuOpen returns whether the subdivision menu is open.
func (dv *DrumView) IsSubdivMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("subdiv-menu")
}

// IsInstMenuOpen returns whether the instrument menu is open.
func (dv *DrumView) IsInstMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("inst-menu")
}

// InstrumentMenuRect returns the on-screen rectangle of the open instrument
// menu (full-list bounds). Returns the zero rect when the menu is closed
// or its bounds have not been populated yet.
func (dv *DrumView) InstrumentMenuRect() image.Rectangle {
	if !dv.IsInstMenuOpen() {
		return image.Rectangle{}
	}
	return dv.instMenuFullRect
}

// IsColorMenuOpen returns whether the color wheel is open.
func (dv *DrumView) IsColorMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("color-wheel")
}

// IsContextMenuOpen returns whether the context menu is open.
func (dv *DrumView) IsContextMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("context-menu")
}

// IsOverflowMenuOpen returns whether the overflow menu is open.
func (dv *DrumView) IsOverflowMenuOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("overflow-menu")
}

// IsFXPanelOpen returns whether the FX panel is open.
func (dv *DrumView) IsFXPanelOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("fx-panel")
}

// IsEQChannelOpen returns whether the EQ channel dropdown is open.
func (dv *DrumView) IsEQChannelOpen() bool {
	return dv.audioTree != nil && dv.audioTree.Portal().Has("eq-channel")
}

// IsNamingOpen returns whether the WAV-naming dialog is open.
func (dv *DrumView) IsNamingOpen() bool {
	return dv.tree != nil && dv.tree.Portal().Has("naming")
}

// anyDragActive reports whether any drag, scrub, slider, hold interaction,
// or mobile touch dead zone is in progress. When true, the tree skips
// dispatching new presses.
func (dv *DrumView) anyDragActive() bool {
	// Mobile touch dead zone: any real touch press in the row scroll area
	// blocks dispatch. Legitimate taps are replayed via tap injection.
	// Touches outside the row area (transport, EQ, portal overlays)
	// are not blocked, allowing slider and button interaction.
	touchDeadZone := Profile().IsMobile() && touchOverrideActive &&
		isMouseButtonPressed(ebiten.MouseButtonLeft) && !isTouchTapInjecting() &&
		!dv.MobileEQMode() &&
		pt(touchOverrideX, touchOverrideY, dv.rowsRect())

	return dv.rowScroll().Dragging() || dv.dragging || dv.scrubbing ||
		(dv.timelineZone != nil && (dv.timelineZone.IsDragging() || dv.timelineZone.IsScrubbing())) ||
		(dv.rowVolGroup() != nil && dv.rowVolGroup().Capturing()) ||
		(dv.transportZone != nil && dv.mainVolGroup() != nil && dv.mainVolGroup().Capturing()) ||
		dv.eqCurveDragBand >= 0 || dv.eqCurveDragFilter != "" ||
		dv.rowScroll().ScrollingCommitted() || touchDeadZone ||
		dv.instMenuScroll.dragging
}

// capturingDrag reports whether the drum view is physically handling a
// drag, scroll, or slider interaction. This excludes overlay/portal state
// so that the InputDispatcher capture is released when the touch moves
// outside drum bounds, allowing grid panning to work on mobile.
func (dv *DrumView) capturingDrag() bool {
	return dv.mouseDownInBounds || dv.anyDragActive() ||
		dv.rowScroll().TouchActive() ||
		(dv.renameComp != nil && dv.renameComp.IsOpen()) || dv.IsNamingOpen() ||
		(dv.layoutHandler != nil && dv.layoutHandler.Capturing())
}

// Capturing reports whether the drum view is actively handling a mouse drag
// (e.g. scrollbar, slider, scrubbing, or any overlay) and should therefore
// block camera panning and prevent input from passing through.
func (dv *DrumView) Capturing() bool {
	return dv.capturingDrag() || dv.anyDropdownOpen()
}

// PortalHasBlocking reports whether a BLOCKING overlay (modal or scrim-backed
// menu/picker/dropdown/popup, e.g. the mobile synth-knob wheel popup) is open
// in any of the drum view's composed input trees. While one is up the overlay
// owns input exclusively: gestures handled OUTSIDE the tree in Game.Update (grid
// tap, camera pan/zoom, two-finger pan) must not act, so they never leak
// through the scrim to background surfaces. The only effect a tap outside the
// overlay may have is closing it (the tree's click-outside / Esc path).
func (dv *DrumView) PortalHasBlocking() bool {
	return dv.rootTree != nil && dv.rootTree.HasBlockingPortal()
}

// logCapturingState logs a detailed breakdown of what makes Capturing() true.
// Used for diagnosing mobile panning regressions.
func (dv *DrumView) logCapturingState() {
	touchDZ := Profile().IsMobile() && touchOverrideActive &&
		isMouseButtonPressed(ebiten.MouseButtonLeft) && !isTouchTapInjecting() &&
		!dv.MobileEQMode() &&
		pt(touchOverrideX, touchOverrideY, dv.rowsRect())
	dv.logger.Debugf("[pan] Capturing breakdown: "+
		"mouseDown=%v anyDrag=%v touchActive=%v dropdown=%v rename=%v naming=%v layout=%v",
		dv.mouseDownInBounds, dv.anyDragActive(),
		dv.rowScroll().TouchActive(),
		dv.anyDropdownOpen(),
		dv.renameComp != nil && dv.renameComp.IsOpen(),
		dv.IsNamingOpen(),
		dv.layoutHandler != nil && dv.layoutHandler.Capturing())
	if dv.anyDragActive() {
		dv.logger.Debugf("[pan] anyDragActive breakdown: "+
			"scrollDrag=%v dragging=%v scrubbing=%v "+
			"tlDrag=%v tlScrub=%v "+
			"rowVol=%v mainVol=%v "+
			"eqBand=%v eqFilter=%v scrollCommit=%v touchDZ=%v instScroll=%v",
			dv.rowScroll().Dragging(), dv.dragging, dv.scrubbing,
			dv.timelineZone != nil && dv.timelineZone.IsDragging(),
			dv.timelineZone != nil && dv.timelineZone.IsScrubbing(),
			dv.rowVolGroup() != nil && dv.rowVolGroup().Capturing(),
			dv.transportZone != nil && dv.mainVolGroup() != nil && dv.mainVolGroup().Capturing(),
			dv.eqCurveDragBand >= 0, dv.eqCurveDragFilter != "",
			dv.rowScroll().ScrollingCommitted(), touchDZ,
			dv.instMenuScroll.dragging)
	}
	if dv.anyDropdownOpen() && dv.tree != nil {
		dv.logger.Debugf("[pan] portal stack size=%d", dv.tree.Portal().StackLen())
		for i := 0; i < dv.tree.Portal().StackLen(); i++ {
			if i < len(dv.tree.Portal().stack) {
				dv.logger.Debugf("[pan]   portal[%d] id=%q", i, dv.tree.Portal().stack[i].ID)
			}
		}
	}
}

// BlocksAt reports whether a point (x,y) lies over a temporary overlay such as
// the instrument dropdown or rename dialog. When true, clicks at that position
// should not reach underlying UI elements.
func (dv *DrumView) BlocksAt(x, y int) bool {
	if !image.Pt(x, y).In(dv.Bounds) {
		return false
	}
	if dv.anyDropdownOpen() || (dv.renameComp != nil && dv.renameComp.IsOpen()) || dv.IsNamingOpen() {
		return true
	}
	return false
}

type deletedRow struct {
	index  int
	origin model.NodeID
}
