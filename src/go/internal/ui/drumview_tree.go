package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Z-index conventions for the component tree.
const (
	ZBaseMin    = 100 // base zone range start
	ZBaseMax    = 199 // base zone range end
	ZResize     = 200 // layout resize handler
	ZOverlayMin = 300 // portal overlays start at 300 + stack index
)

// DrumViewTree orchestrates the zone-based component tree. It runs a strict
// 4-phase frame loop: Layout → Update → Input → Draw.
// The tree owns its suppress flag. When the tree handles input
// (inputHandled=true), it propagates suppress to the global flag to
// prevent legacy code from double-dispatching.
type DrumViewTree struct {
	zones   []zoneEntry
	zoneMap map[string]*zoneEntry

	hitIndex *HitIndex
	portal   *OverlayPortal

	// Input capture state: one handler at a time across the entire tree.
	capturedHandler HitHandler
	capturedTag     string

	// Suppress flag: prevents new presses from being dispatched until
	// the current press is released. Owned by the tree; propagated to
	// global suppressClicksUntilRelease only when inputHandled is set.
	suppress bool

	// Track press state for capture lifecycle.
	wasPressed bool

	// inputHandled is set when the tree dispatched a press to a handler
	// during the current frame.
	inputHandled bool

	// wheelHandled is set when the tree consumed a wheel event this frame.
	// Legacy code in DrumView.Update() checks this to avoid double-handling.
	wheelHandled bool

	// dragActive returns true when any drag operation is active.
	// The tree skips dispatching new presses during drags.
	dragActive func() bool

	// Keyboard focus: only one zone receives keyboard/char events.
	focusedZone string

	// Screen bounds for portal layout.
	bounds image.Rectangle
}

type zoneEntry struct {
	zone     Zone
	rect     image.Rectangle
	zIndex   int
	lastRect image.Rectangle // detect rect changes for re-layout
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

// RegisterZone adds a zone to the tree at a given z-index.
func (t *DrumViewTree) RegisterZone(z Zone, zIndex int) {
	e := zoneEntry{zone: z, zIndex: zIndex}
	t.zones = append(t.zones, e)
	t.zoneMap[z.ID()] = &t.zones[len(t.zones)-1]
}

// SetZoneRect sets the layout rectangle for a zone, triggering re-layout
// if the rect changed.
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

// SetFocus sets keyboard focus to a zone by ID. Pass "" to clear focus.
func (t *DrumViewTree) SetFocus(zoneID string) {
	t.focusedZone = zoneID
}

// Update runs the 4-phase frame loop: Layout → Update → Input → Draw
// (Draw is called separately via Draw()).
//
// The tree owns its suppress flag. After input dispatch, if the tree
// handled input this frame it propagates suppress to the global flag
// so legacy code doesn't double-dispatch.
func (t *DrumViewTree) Update() {
	t.inputHandled = false
	t.wheelHandled = false

	// Phase 1: Layout — only for zones that need it or whose rect changed.
	for i := range t.zones {
		e := &t.zones[i]
		if e.zone.NeedsLayout() || e.rect != e.lastRect {
			e.zone.Layout(e.rect)
			e.lastRect = e.rect
			t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
		}
	}

	// Phase 2: Update — every zone, every frame.
	for i := range t.zones {
		t.zones[i].zone.Update()
	}

	// Phase 2.5a: Auto-focus transport zone when BPM box is focused.
	// This ensures keyboard events (Enter, Escape) are routed to the
	// transport zone's HandleKey while BPM text editing is active.
	if e, ok := t.zoneMap["transport"]; ok {
		if tz, ok := e.zone.(*TransportZone); ok && tz.bpmBox != nil {
			if tz.bpmBox.Focused() {
				t.focusedZone = "transport"
			} else if t.focusedZone == "transport" {
				t.focusedZone = ""
			}
		}
	}

	// Phase 2.5b: Auto-focus eq-panel zone when a dB input is focused.
	if e, ok := t.zoneMap["eq-panel"]; ok {
		if ez, ok := e.zone.(*EQPanelZone); ok {
			if ez.dbInputFocused >= 0 {
				t.focusedZone = "eq-panel"
			} else if t.focusedZone == "eq-panel" {
				t.focusedZone = ""
			}
		}
	}

	// Phase 2.5c: Update portal overlays (momentum, animations).
	t.portal.Update()

	// Phase 3: Input.
	t.handleInput()

	// Phase 3.5: Auto-close overlays whose components closed themselves
	// during handleInput (e.g., subdiv menu button selection).
	t.portal.CleanupClosed()

	// Tree is the single source of truth for suppress; write through to
	// the global flag so non-tree consumers (splitter.go, textinput.go)
	// see the same value.
	suppressClicksUntilRelease = t.suppress
}

// Draw renders all zones in z-order (ascending), then portal overlays on top.
// Two clipping constraints are enforced together: the tree bounds (outer
// envelope) and each zone's own widget rectangle (e.rect, populated by
// Layout). The intersection ensures zones cannot escape their WidgetBoard
// cell or the tree's overall bounds. Falls back gracefully when either
// rectangle is unset.
func (t *DrumViewTree) Draw(screen *ebiten.Image) {
	// Draw zones in registration order (assumed ascending z-index).
	for i := range t.zones {
		e := &t.zones[i]
		clip := screen.Bounds()
		if !t.bounds.Empty() {
			clip = clip.Intersect(t.bounds)
		}
		if !e.rect.Empty() {
			clip = clip.Intersect(e.rect)
		}
		if clip.Empty() {
			continue
		}
		if clip == screen.Bounds() {
			e.zone.Draw(screen)
		} else {
			sub := screen.SubImage(clip).(*ebiten.Image)
			e.zone.Draw(sub)
		}
	}
	// Portal overlays on top.
	t.portal.Draw(screen)
}

// handleInput processes pointer and keyboard input through the hit index.
func (t *DrumViewTree) handleInput() {
	mx, my := cursorPosition()
	pressed := isMouseButtonPressed(ebiten.MouseButtonLeft)

	// Clear stale suppress when no press cycle is active. Suppress set
	// during a press cycle (click-outside, InputConsumed) is maintained
	// via wasPressed until release clears both. Suppress set outside a
	// press cycle (e.g., Escape key closing portal) is cleared here on
	// the next frame regardless of whether the next frame has a press.
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
		return
	}

	// Handle ongoing capture (drag).
	if pressed && t.capturedHandler != nil {
		// Clear global suppress during tree-managed drag (same as OnPress).
		// Captured handlers (e.g., Slider.HandleInputResult) check the
		// global flag; clearing it prevents a stale flag from killing the
		// drag. Save/restore matches the OnPress pattern below.
		savedSuppress := suppressClicksUntilRelease
		suppressClicksUntilRelease = false
		t.capturedHandler.OnDrag(mx, my)
		suppressClicksUntilRelease = savedSuppress
		t.wasPressed = true
		return
	}

	// Handle new press.
	if pressed && !t.wasPressed {
		t.wasPressed = true

		if t.suppress {
			return
		}

		dragBlocked := t.dragActive != nil && t.dragActive()

		hits := t.hitIndex.At(mx, my)
		if len(hits) == 0 {
			// Click outside all areas.
			if t.portal.IsOpen() {
				t.portal.CloseTop()
				t.suppress = true
			}
			return
		}

		// Dispatch to hit areas in z-order (highest first). When a handler
		// returns InputIgnored, fall through to the next hit. This ensures
		// higher-z components (e.g., slider groups) get first dispatch
		// priority over lower-z components (e.g., label buttons).
		for _, hit := range hits {
			if hit.Handler == nil {
				continue
			}

			// Block dispatch for non-portal areas when drag is active.
			// Portal overlays (volume popup sliders) always receive input.
			if dragBlocked && hit.ZIndex < ZOverlayMin {
				return
			}

			// When the hit is a non-portal area (z < ZOverlayMin) and
			// a portal overlay is open, close the portal (click-outside).
			if hit.ZIndex < ZOverlayMin && t.portal.IsOpen() {
				t.portal.CloseTop()
				t.suppress = true
				return
			}

			// Temporarily clear global suppress during tree-managed press.
			// Mirrors the OnDrag pattern. The tree's own t.suppress flag
			// already guards against re-dispatch, making the global flag
			// redundant during tree-managed dispatch.
			savedSuppress := suppressClicksUntilRelease
			suppressClicksUntilRelease = false
			result := hit.Handler.OnPress(mx, my)
			suppressClicksUntilRelease = savedSuppress

			// Portal overlays always absorb input within their bounds.
			// Promote InputIgnored → InputConsumed to guarantee suppress
			// and inputHandled are set, preventing click-through.
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

	// Handle wheel events — dispatched unconditionally to hit areas.
	{
		wx, wy := wheel()
		steps := int(wy)
		if wx != 0 && steps == 0 {
			steps = int(wx)
		}
		if steps != 0 {
			hits := t.hitIndex.At(mx, my)
			for _, h := range hits {
				if h.Handler == nil {
					continue
				}
				if h.Handler.OnWheel(mx, my, steps) != InputIgnored {
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

// commonKeys is the set of keys checked for zone keyboard routing.
// Limited to keys available in both real Ebiten and the test stub.
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
