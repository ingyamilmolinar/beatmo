package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Stored as key + args, never rendered text, so the history re-renders in the current locale;
// key == "" means literal text.
type notification struct {
	key    i18n.Key // translation key; "" ⇒ use text verbatim
	args   []string // Tf args; an arg prefixed "@" is itself an i18n key (resolved at display)
	text   string   // rendered fallback when key == ""
	isErr  bool
	unixMs int64 // unix millis when raised; 0 in pure-unit tests
}

// An arg starting with "@" is itself an i18n key (undo/redo store the action name that way).
func (n notification) display() string {
	key, args := n.key, n.args
	if key == "" {
		if n.text == "" {
			return ""
		}
		// Reverse-map frozen English text to its key so old entries still retranslate.
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

// Seeding marks no session entry, so the in-band area stays idle until a new notification.
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

// Prefer the keyed variants, which follow the locale.
func (dv *DrumView) notifyInfo(msg string)  { dv.pushNotif(msg, false) }
func (dv *DrumView) notifyError(msg string) { dv.pushNotif(msg, true) }
func (dv *DrumView) pushNotif(msg string, isErr bool) {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	n := notification{text: msg, isErr: isErr, unixMs: nowUnixMilli()}
	dv.notifStore.Push(n)
	if isErr {
		dv.pushErrorToast(n.display())
	}
}

// Carries the key, not rendered text, so it renders in the locale active when drained.
type pendingNotif struct {
	key   i18n.Key
	args  []string
	isErr bool
}

// The preferred API for every user-facing notification: renders in the locale active when
// shown; "@some.key" args are translated too.
func (dv *DrumView) notifyInfoKey(key i18n.Key, args ...string)  { dv.pushNotifKey(key, args, false) }
func (dv *DrumView) notifyErrorKey(key i18n.Key, args ...string) { dv.pushNotifKey(key, args, true) }
func (dv *DrumView) pushNotifKey(key i18n.Key, args []string, isErr bool) {
	if dv.notifStore == nil {
		dv.notifStore = newNotificationStore(notifHistoryCap)
	}
	n := notification{key: key, args: args, isErr: isErr, unixMs: nowUnixMilli()}
	dv.notifStore.Push(n)
	if isErr {
		dv.pushErrorToast(n.display())
	}
}

func (dv *DrumView) anyDropdownOpen() bool {
	return dv.portal().IsOpen()
}

// IsSubdivMenuOpen returns whether the subdivision menu is open.
func (dv *DrumView) IsSubdivMenuOpen() bool {
	return dv.tree != nil && dv.portal().Has("subdiv-menu")
}

// IsInstMenuOpen returns whether the instrument menu is open.
func (dv *DrumView) IsInstMenuOpen() bool {
	return dv.tree != nil && dv.portal().Has("inst-menu")
}

func (dv *DrumView) InstrumentMenuRect() image.Rectangle {
	if !dv.IsInstMenuOpen() {
		return image.Rectangle{}
	}
	return dv.instMenuFullRect
}

// IsColorMenuOpen returns whether the color wheel is open.
func (dv *DrumView) IsColorMenuOpen() bool {
	return dv.tree != nil && dv.portal().Has("color-wheel")
}

// IsContextMenuOpen returns whether the context menu is open.
func (dv *DrumView) IsContextMenuOpen() bool {
	return dv.tree != nil && dv.portal().Has("context-menu")
}

// IsOverflowMenuOpen returns whether the overflow menu is open.
func (dv *DrumView) IsOverflowMenuOpen() bool {
	return dv.tree != nil && dv.portal().Has("overflow-menu")
}

// IsFXPanelOpen returns whether the FX panel is open.
func (dv *DrumView) IsFXPanelOpen() bool {
	return dv.tree != nil && dv.portal().Has("fx-panel")
}

// IsEQChannelOpen returns whether the EQ channel dropdown is open.
func (dv *DrumView) IsEQChannelOpen() bool {
	return dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen()
}

// IsNamingOpen returns whether the WAV-naming dialog is open.
func (dv *DrumView) IsNamingOpen() bool {
	return dv.tree != nil && dv.portal().Has("naming")
}

// The dead zone is the only region where the hold-press is not dispatched live, so only a tap
// there needs injectTouchTap; injecting elsewhere would process the gesture twice.
func (dv *DrumView) tapInScrollDeadZone(x, y int) bool {
	return Profile().IsMobile() && !dv.MobileEQMode() && pt(x, y, dv.rowsRect())
}

func (dv *DrumView) anyDragActive() bool {
	// Any real touch in the row scroll area blocks dispatch (taps are replayed by injection);
	// touches elsewhere pass, so sliders and buttons still work.
	touchDeadZone := Profile().IsMobile() && touchOverrideActive &&
		isMouseButtonPressed(ebiten.MouseButtonLeft) && !isTouchTapInjecting() &&
		!dv.MobileEQMode() &&
		pt(touchOverrideX, touchOverrideY, dv.rowsRect())

	return dv.rowScroll().Dragging() || dv.dragging || dv.scrubbing ||
		(dv.timelineZone != nil && (dv.timelineZone.IsDragging() || dv.timelineZone.IsScrubbing())) ||
		(dv.rowVolGroup() != nil && dv.rowVolGroup().Capturing()) ||
		(dv.transportZone != nil && dv.mainVolGroup() != nil && dv.mainVolGroup().Capturing()) ||
		dv.eqCurveDragBand >= 0 || dv.eqCurveDragFilter != hooks.FilterNone ||
		dv.rowScroll().ScrollingCommitted() || touchDeadZone ||
		dv.instMenuScroll.dragging
}

// Excludes overlay state, so capture is released when the touch leaves the drum view and grid
// panning works on mobile.
func (dv *DrumView) capturingDrag() bool {
	return dv.mouseDownInBounds || dv.anyDragActive() ||
		dv.rowScroll().TouchActive() ||
		(dv.renameComp != nil && dv.renameComp.IsOpen()) || dv.IsNamingOpen() ||
		(dv.layoutHandler != nil && dv.layoutHandler.Capturing())
}

func (dv *DrumView) Capturing() bool {
	return dv.capturingDrag() || dv.anyDropdownOpen()
}

// While a blocking overlay is open, the out-of-tree gestures in Game.Update (grid tap, pan, zoom)
// must not act; an outside tap may only close it.
func (dv *DrumView) PortalHasBlocking() bool {
	return dv.rootTree != nil && dv.rootTree.HasBlockingPortal()
}

// Broader than PortalHasBlocking: an outside tap on a docked panel closes it without reaching
// the grid. Passive tooltips are excluded.
func (dv *DrumView) PortalOwnsInput() bool {
	return dv.rootTree != nil && dv.rootTree.HasInputOwningPortal()
}

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
			dv.eqCurveDragBand >= 0, dv.eqCurveDragFilter != hooks.FilterNone,
			dv.rowScroll().ScrollingCommitted(), touchDZ,
			dv.instMenuScroll.dragging)
	}
	if dv.anyDropdownOpen() && dv.tree != nil {
		dv.logger.Debugf("[pan] portal stack size=%d", dv.portal().StackLen())
		for i := 0; i < dv.portal().StackLen(); i++ {
			if i < len(dv.portal().stack) {
				dv.logger.Debugf("[pan]   portal[%d] id=%q", i, dv.portal().stack[i].ID)
			}
		}
	}
}

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
