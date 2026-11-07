package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Draw walks zones and layers in ascending Z, so a higher Z always renders on top. A new layer
// needs a constant here and a row in the z-order test.
const (
	ZBackground     = 50  // widget surface fills + bottom-sheet surface
	ZRackMask       = 55  // rack-column surface elevation (below row content)
	ZEQPeek         = 60  // mobile EQ peek sparkline
	ZBaseMin        = 100 // base zone range start
	ZTransport      = 100 // transport zone
	ZTimeline       = 110 // timeline zone (cells, playhead, highlights)
	ZRowRack        = 120 // row controls (label, vol, mute, fx, …)
	ZEQPanel        = 130 // EQ / wave / spectrum / meters / scope panel
	ZTransportPulse = 140 // play + record halos pulsing on top of toolbar
	ZViewSwitch     = 150 // mobile segmented EQ/Pads/Wave switch
	ZRowZoomChips   = 160 // mobile row-zoom +/− chips
	ZNotifications  = 170 // toast notifications (top-right of pane)
	ZLayoutGuides   = 180 // column/row dividers (debug-gated, default off)
	ZLayoutPills    = 185 // splitter pills draw ON TOP of guides so the
	// pill stays clickable when the debug overlay is on.
	ZRowEQDivider = 186 // row↔EQ-boundary divider line+pill (drawn above guides)
	ZBaseMax      = 199 // base zone range end
	ZResize       = 200 // layout resize handler
	ZDrumToast    = 210 // mobile fallback status toast (audioTree, topmost —
	//                      see DrumToastLayer / toastHostRect)
	ZOverlayMin = 300 // portal overlays start at 300 + stack index
)

// Layout → Update → Input → Draw. When the tree handles input it propagates suppress to the
// global flag, so legacy code cannot dispatch the press twice.
type DrumViewTree struct {
	// Per tree, so one surface's leftover sub-notch motion cannot bleed into another's.
	wheelNorm *WheelNormalizer

	// Pointers: zoneMap aliases these entries, and an append that reallocates a value slice would
	// orphan them.
	zones   []*zoneEntry
	zoneMap map[string]*zoneEntry

	// Every pixel in the drum pane comes from one of these.
	layers []Layer

	hitIndex *HitIndex
	portal   *OverlayPortal

	// Input capture state: one handler at a time across the entire tree.
	capturedHandler HitHandler
	capturedTag     string

	// True while a separate input system holds capture this press, so the two never both act on it.
	externalCapture func() bool

	// Owned by the tree; written to the global only when inputHandled is set.
	suppress bool

	// Track press state for capture lifecycle.
	wasPressed bool

	// On real mobile a tap injection lands on the same frame the touch ends, so wasPressed never
	// sees a release; recognising the injection edge as a fresh press keeps the tap from being dropped.
	wasInjecting bool

	// A popup opened on touch-down must not close itself when the lift is replayed as a tap; this
	// marks that the portal opened during the current press. Cleared on release, so a second tap works.
	prevPortalOpen        bool
	portalOpenedThisPress bool

	inputHandled bool

	// DrumView.Update checks this so a wheel is not handled twice.
	wheelHandled bool

	// false: finish an in-flight drag or release, but start no new press or wheel, so RootTree can
	// route fresh input to exactly one subtree.
	allowNewPress bool

	dragActive func() bool

	// Keyboard focus: only one zone receives keyboard/char events.
	focusedZone string

	// Screen bounds for portal layout.
	bounds image.Rectangle

	// On the tree, so the glow is one per-frame overlay independent of any zone's sprite cache.
	hoverGlowRect image.Rectangle
	hoverGlowAnim float64
}

type zoneEntry struct {
	zone     Zone
	rect     image.Rectangle
	zIndex   int
	lastRect image.Rectangle // detect rect changes for re-layout
	// Gates BOTH Draw and the HitIndex: a hidden zone that still publishes its catch-all swallows
	// taps meant for the zones beneath.
	visible func() bool
	// Lets the layout pass clear the HitIndex once on a transition instead of every frame.
	lastVisible bool

	// Reused while the parent and clip are unchanged: a fresh SubImage per zone per frame was the
	// dominant allocator behind a WASM OOM.
	subParent *ebiten.Image
	subClip   image.Rectangle
	sub       *ebiten.Image
}

// NewDrumViewTree creates a new tree with a fresh HitIndex and OverlayPortal.
func NewDrumViewTree() *DrumViewTree {
	idx := &HitIndex{}
	return &DrumViewTree{
		zoneMap:  make(map[string]*zoneEntry),
		hitIndex: idx,
		portal:   NewOverlayPortal(idx),
	}
}

func (t *DrumViewTree) RegisterZone(z Zone, zIndex int) {
	t.RegisterZoneVisible(z, zIndex, nil)
}

func (t *DrumViewTree) RegisterZoneVisible(z Zone, zIndex int, visible func() bool) {
	// One predicate, two consumers: the layer gates Draw, the entry gates HitAreas.
	e := &zoneEntry{zone: z, zIndex: zIndex, visible: visible, lastVisible: true}
	t.zones = append(t.zones, e)
	t.zoneMap[z.ID()] = e
	t.insertLayer(zoneAsLayer{zone: z, zIndex: zIndex, visible: visible})
}

func (t *DrumViewTree) RegisterLayer(l Layer) {
	t.insertLayer(l)
}

// Stable: equal ZIndex keeps insertion order.
func (t *DrumViewTree) insertLayer(l Layer) {
	z := l.ZIndex()
	idx := len(t.layers)
	for i, existing := range t.layers {
		if existing.ZIndex() > z {
			idx = i
			break
		}
	}
	t.layers = append(t.layers, nil)
	copy(t.layers[idx+1:], t.layers[idx:])
	t.layers[idx] = l
}

// Production code must not iterate the real slice from outside the tree.
func (t *DrumViewTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
}

func (t *DrumViewTree) SetZoneRect(id string, r image.Rectangle) {
	if e, ok := t.zoneMap[id]; ok {
		e.rect = r
	}
}

// SetBounds updates the overall tree bounds (used for portal screen bounds).
func (t *DrumViewTree) SetBounds(r image.Rectangle) {
	t.bounds = r
	t.portal.SetScreenBounds(r)
}

// Portal returns the overlay portal for zones to open overlays.
func (t *DrumViewTree) Portal() *OverlayPortal {
	return t.portal
}

// HitIndex returns the shared hit index (for testing/inspection).
func (t *DrumViewTree) HitIndexRef() *HitIndex {
	return t.hitIndex
}

func (t *DrumViewTree) topOwnerAt(x, y int) string {
	hits := t.hitIndex.At(x, y)
	if len(hits) == 0 {
		return ""
	}
	return hits[0].ownerID
}

func (t *DrumViewTree) HasZoneForTest(id string) bool {
	_, ok := t.zoneMap[id]
	return ok
}

// SetFocus sets keyboard focus to a zone by ID. Pass "" to clear focus.
func (t *DrumViewTree) SetFocus(zoneID string) {
	t.focusedZone = zoneID
}

// FocusedZoneObj returns the currently focused zone, or nil if none.
func (t *DrumViewTree) FocusedZoneObj() Zone {
	if t.focusedZone == "" {
		return nil
	}
	if e, ok := t.zoneMap[t.focusedZone]; ok {
		return e.zone
	}
	return nil
}

func (t *DrumViewTree) Update() {
	t.Tick()
	t.DispatchInput(true)
}

// The non-input half, which a RootTree runs for every subtree every frame whoever owns input.
func (t *DrumViewTree) Tick() {
	t.inputHandled = false
	t.wheelHandled = false

	t.layoutPass()

	for i := range t.zones {
		t.zones[i].zone.Update()
	}

	t.autoFocusPass()

	t.portal.Update()
}

// So Enter, Backspace and characters reach the zone with a live text edit.
func (t *DrumViewTree) autoFocusPass() {
	if e, ok := t.zoneMap["transport"]; ok {
		if tz, ok := e.zone.(*TransportZone); ok && tz.bpmBox != nil {
			if tz.bpmBox.Focused() {
				t.focusedZone = "transport"
			} else if t.focusedZone == "transport" {
				t.focusedZone = ""
			}
		}
	}

}

// A withheld subtree still completes an in-flight drag or release, so no gesture is stranded.
// busy reports engagement this frame (input, capture, modal), which RootTree uses for routing.
func (t *DrumViewTree) DispatchInput(allowNewPress bool) (busy bool) {
	t.allowNewPress = allowNewPress

	t.handleInput()

	t.portal.CleanupClosed()

	// The tree owns suppress; written through for non-tree readers (splitter, text input).
	suppressClicksUntilRelease = t.suppress

	return t.inputHandled || t.wheelHandled || t.Capturing() || t.PortalHasModal()
}

// The ONLY place layout and HitIndex publication happen together: Layout clears needLayout, so a
// Layout that skips the publish strands the HitIndex on stale geometry. A hidden zone publishes nothing.
func (t *DrumViewTree) layoutPass() {
	for i := range t.zones {
		e := t.zones[i]
		nowVisible := e.visible == nil || e.visible()
		layoutChanged := e.zone.NeedsLayout() || e.rect != e.lastRect
		visibilityChanged := nowVisible != e.lastVisible
		switch {
		case !nowVisible:
			// Hidden zones are not laid out (some Layouts have side effects); clearing once per transition
			// is enough.
			if visibilityChanged {
				t.hitIndex.Update(e.zone.ID(), nil)
			}
		case layoutChanged || visibilityChanged:
			e.zone.Layout(e.rect)
			e.lastRect = e.rect
			t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
		}
		e.lastVisible = nowVisible
	}
}

// For draw-time layouts, so they can never strand the HitIndex. Idempotent.
func (t *DrumViewTree) EnsureLayouts() {
	t.layoutPass()
}

// The only sanctioned way to force a layout outside the frame loop. Runs even when hidden, to keep
// rects warm for a reveal, but a hidden zone publishes no hit areas.
func (t *DrumViewTree) LayoutZoneNow(id string) {
	e, ok := t.zoneMap[id]
	if !ok {
		return
	}
	e.zone.Layout(e.rect)
	e.lastRect = e.rect
	nowVisible := e.visible == nil || e.visible()
	if nowVisible {
		t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
	} else {
		t.hitIndex.Update(e.zone.ID(), nil)
	}
	e.lastVisible = nowVisible
}

// Zones are clipped to their widget cell; plain layers get the unclipped screen and confine
// themselves, which keeps the ebitestub backend's pixel reads working.
func (t *DrumViewTree) Draw(screen *ebiten.Image) {
	t.DrawContent(screen)
	t.DrawOverlays(screen)
}

// Everything except portals: RootTree draws all subtrees' content, then all overlays.
func (t *DrumViewTree) DrawContent(screen *ebiten.Image) {
	for _, layer := range t.layers {
		if !layer.Visible() {
			continue
		}
		zl, isZone := layer.(zoneAsLayer)
		if !isZone {
			layer.Draw(screen)
			continue
		}
		clip := screen.Bounds()
		if !t.bounds.Empty() {
			clip = clip.Intersect(t.bounds)
		}
		if e, ok := t.zoneMap[zl.zone.ID()]; ok && !e.rect.Empty() {
			clip = clip.Intersect(e.rect)
		}
		if clip.Empty() {
			continue
		}
		if clip == screen.Bounds() {
			layer.Draw(screen)
		} else {
			e := t.zoneMap[zl.zone.ID()]
			var sub *ebiten.Image
			if e != nil && e.subParent == screen && e.subClip == clip && e.sub != nil {
				sub = e.sub
			} else {
				sub = screen.SubImage(clip).(*ebiten.Image)
				if e != nil {
					e.subParent = screen
					e.subClip = clip
					e.sub = sub
				}
			}
			layer.Draw(sub)
		}
	}
	// Above all zones (escaping their sprite caches), below portals, which style their own hover.
	t.drawHoverGlow(screen)
}

func (t *DrumViewTree) DrawOverlays(screen *ebiten.Image) {
	t.portal.Draw(screen)
}

// handleInput processes pointer and keyboard input through the hit index.
func (t *DrumViewTree) handleInput() {
	mx, my := cursorPosition()
	pressed := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// An injected tap on real mobile lands on the same frame the touch ends, so clear wasPressed to
	// see it as a fresh press, unless something is captured.
	portalOpen := t.portal.IsOpen()
	if !portalOpen {
		t.portalOpenedThisPress = false
	} else if pressed && t.wasPressed && !t.prevPortalOpen {
		t.portalOpenedThisPress = true
	}
	t.prevPortalOpen = portalOpen

	injecting := isTouchTapInjecting()
	if injecting && !t.wasInjecting && pressed && t.capturedHandler == nil && !t.portalOpenedThisPress {
		t.wasPressed = false
	}
	t.wasInjecting = injecting

	// Suppress set outside a press cycle (Esc closing a portal) clears on the next frame.
	if t.suppress && !t.wasPressed {
		t.suppress = false
	}

	// Handle release.
	if !pressed && t.wasPressed {
		if t.capturedHandler != nil {
			suppressClicksUntilRelease = false
			t.capturedHandler.OnRelease(mx, my)
			t.capturedHandler = nil
			t.capturedTag = ""
		}
		t.suppress = false
		t.wasPressed = false
		t.portalOpenedThisPress = false
		return
	}

	// Handle ongoing capture (drag).
	if pressed && t.capturedHandler != nil {
		// Captured handlers check the global flag, so a stale value would kill the drag.
		savedSuppress := suppressClicksUntilRelease
		suppressClicksUntilRelease = false
		t.capturedHandler.OnDrag(mx, my)
		suppressClicksUntilRelease = savedSuppress
		t.wasPressed = true
		return
	}

	// One press belongs to ONE subtree for its whole life: "busy" is recomputed each frame, so without
	// this latch a later held frame re-dispatches the same press to a subtree below.
	if !t.allowNewPress && pressed && !t.wasPressed {
		t.wasPressed = true
	}

	// Only new presses are gated; an ongoing drag or release always completes.
	if t.allowNewPress && pressed && !t.wasPressed {
		t.wasPressed = true

		if t.suppress {
			return
		}

		if t.externalCapture != nil && t.externalCapture() {
			return
		}

		dragBlocked := t.dragActive != nil && t.dragActive()

		hits := t.hitIndex.At(mx, my)
		if len(hits) == 0 {
			// Click outside all areas.
			if t.portal.IsOpen() {
				t.portal.CloseTop()
				t.suppress = true
				// The click-away consumes the press, so RootTree withholds it from lower subtrees.
				t.inputHandled = true
			}
			return
		}

		// InputIgnored falls through to the next hit, highest z first.
		for _, hit := range hits {
			if hit.Handler == nil {
				continue
			}

			// Portal overlays still receive input during a drag.
			if dragBlocked && hit.ZIndex < ZOverlayMin {
				return
			}

			// A non-portal hit with a portal open closes the portal and consumes the press.
			if hit.ZIndex < ZOverlayMin && t.portal.IsOpen() {
				t.portal.CloseTop()
				t.suppress = true
				t.inputHandled = true
				return
			}

			// The tree's own flag already guards re-dispatch.
			savedSuppress := suppressClicksUntilRelease
			suppressClicksUntilRelease = false
			result := hit.Handler.OnPress(mx, my)
			suppressClicksUntilRelease = savedSuppress

			// Portals always absorb input in their bounds, so nothing clicks through.
			if result == InputIgnored && hit.ZIndex >= ZOverlayMin {
				result = InputConsumed
			}

			switch result {
			case InputCaptured:
				t.capturedHandler = hit.Handler
				t.capturedTag = hit.Tag
				t.suppress = true
				t.inputHandled = true
				return
			case InputConsumed:
				// One-shot: no capture needed. Suppress old code for this press.
				t.suppress = true
				t.inputHandled = true
				return
			case InputIgnored:
				continue // try next hit in z-order
			}
		}
		return
	}

	// Escape key closes the topmost portal overlay.
	if isKeyPressed(ebiten.KeyEscape) && t.portal.IsOpen() {
		t.portal.CloseTop()
		t.suppress = true
		return
	}

	// A withheld subtree must not even READ the wheel: reading consumes the frame's delta and would
	// starve the owning subtree.
	if t.allowNewPress {
		// Detents, never raw units: one raw browser notch (~100) scrolled a whole menu. The tree's
		// normalizer carries sub-notch trackpad motion between frames.
		nx, ny := wheelNotches()
		if t.wheelNorm == nil {
			t.wheelNorm = NewWheelNormalizer(defaultWheelDynamics())
		}
		steps := t.wheelNorm.Rows(ny)
		if nx != 0 && steps == 0 {
			steps = wheelRowsFromNotches(nx)
		}
		if steps != 0 {
			hits := t.hitIndex.At(mx, my)
			for _, h := range hits {
				if h.Handler == nil {
					continue
				}
				// Knobs take both axes (left/right value, up/down scroll); everyone else gets one axis.
				var res InputResult
				if w2, ok := h.Handler.(wheel2DHandler); ok {
					res = w2.OnWheel2D(mx, my, wheelRowsFromNotches(nx), steps)
				} else {
					res = h.Handler.OnWheel(mx, my, steps)
				}
				if res != InputIgnored {
					t.wheelHandled = true
					break
				}
			}
		}
	}

	// Handle keyboard events: route to focused zone.
	if t.focusedZone != "" {
		if e, ok := t.zoneMap[t.focusedZone]; ok {
			// Check common UI keys.
			for _, k := range commonKeys {
				if isKeyPressed(k) {
					e.zone.HandleKey(k)
				}
			}
			chars := inputChars()
			if len(chars) > 0 {
				e.zone.HandleChars(chars)
			}
		}
	}
}

// Only keys both real Ebiten and the test stub define.
var commonKeys = []ebiten.Key{
	ebiten.KeyEnter,
	ebiten.KeyEscape,
	ebiten.KeyBackspace,
}

// Suppress returns whether the tree is suppressing new presses.
func (t *DrumViewTree) Suppress() bool {
	return t.suppress
}

// CapturedTag returns the debug tag of the currently captured handler.
func (t *DrumViewTree) CapturedTag() string {
	return t.capturedTag
}

// Capturing returns true if a handler is currently captured.
func (t *DrumViewTree) Capturing() bool {
	return t.capturedHandler != nil
}

// Read before Tick resets the flags, so it describes the previous frame.
func (t *DrumViewTree) handledInputLastFrame() bool {
	return t.inputHandled || t.wheelHandled || t.Capturing()
}

// Drops capture without a release, so a drag live during a tab switch cannot strand the
// dispatcher on a now-hidden zone.
func (t *DrumViewTree) ClearCapture() {
	t.capturedHandler = nil
	t.capturedTag = ""
	t.suppress = false
	// Deliberately keeps wasPressed: this runs mid-dispatch, and zeroing it desyncs press/release
	// and eats the next tap. The release frame clears it.
}

// Only the click-outside dismissal, never a zone dispatch, so a click in one section closes the
// other's transient panel without acting on its controls. Blocking overlays are left alone.
func (t *DrumViewTree) DismissOnForeignPress(mx, my int) {
	if t.portal == nil || !t.portal.IsOpen() || t.portal.HasBlocking() {
		return
	}
	for _, h := range t.hitIndex.At(mx, my) {
		if h.ZIndex >= ZOverlayMin {
			return
		}
	}
	t.portal.CloseTop()
}

func (t *DrumViewTree) Suppressing() bool { return t.suppress }

func (t *DrumViewTree) PortalHasModal() bool {
	if t.portal == nil {
		return false
	}
	return t.portal.HasModal()
}

func (t *DrumViewTree) PortalHasBlocking() bool {
	if t.portal == nil {
		return false
	}
	return t.portal.HasBlocking()
}

func (t *DrumViewTree) PortalOwnsInput() bool {
	if t.portal == nil {
		return false
	}
	return t.portal.OwnsInput()
}

// InputHandled returns true if the tree dispatched to a handler this frame.
func (t *DrumViewTree) InputHandled() bool {
	return t.inputHandled
}

// WheelHandled returns true if a tree hit handler consumed the wheel event this frame.
func (t *DrumViewTree) WheelHandled() bool {
	return t.wheelHandled
}

// SetDragActive sets the callback used to check if any drag is active.
func (t *DrumViewTree) SetDragActive(fn func() bool) {
	t.dragActive = fn
}

func (t *DrumViewTree) SetExternalCapture(fn func() bool) {
	t.externalCapture = fn
}
