package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Z-index conventions for the component tree.
//
// Every drawable participant — Zone or Layer — registers at one of these
// constants. The merged slice in (*DrumViewTree).Draw walks them in
// ascending order, so a layer with a higher Z always renders on top of
// any layer with a lower Z. Adding a new layer requires:
//  1. picking the correct Z constant here (or adding a new one),
//  2. registering it in drumview_ctor.go next to the existing
//     RegisterZone calls, and
//  3. extending the expected list in drumview_layer_zorder_test.go.
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
	//                       interactive pill remains visible/clickable
	//                       when the debug overlay is on.
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
	// zones holds POINTERS, not values. zoneMap aliases the same
	// *zoneEntry objects, and SetZoneRect mutates them in place. Storing
	// values here (with zoneMap holding &zones[i]) was a latent bug: every
	// RegisterZone append could reallocate the backing array, orphaning the
	// zoneMap pointers so SetZoneRect updates were silently lost for every
	// zone except the last-registered. See
	// TestDrumViewTree_SetZoneRectSurvivesLaterRegistrations.
	zones   []*zoneEntry
	zoneMap map[string]*zoneEntry

	// layers is the ordered draw slice (zones + decorative layers, sorted
	// ascending by ZIndex). Built lazily on first Draw and re-sorted when
	// RegisterZone or RegisterLayer mutates it. Every pixel in the drum
	// pane originates from one of these entries — see layer.go.
	layers []Layer

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
	// visible is the optional gate registered via RegisterZoneVisible.
	// When non-nil and returns false, the zone is:
	//   1. not drawn (the existing zoneAsLayer.Visible() path), and
	//   2. NOT included in the input HitIndex (the Update() Layout phase
	//      clears the zone's areas instead of publishing them).
	// Half-wiring the gate (the pre-fix state — Draw gated, HitAreas not)
	// caused the "Pads tab unresponsive" regression: a hidden audio panel
	// still published its full-bounds catch-all `eq-panel-capture` hit
	// area, swallowing taps that should reach the row rack at z=120.
	visible func() bool
	// lastVisible memoises the previous visible() result so the Layout
	// phase can detect transitions (visible→invisible) and clear the
	// HitIndex once instead of re-publishing empty areas every frame.
	lastVisible bool

	// Cached SubImage wrapper for the most recent (screen, clip) pair seen
	// during Draw. Reused across frames when the parent screen pointer and
	// clip rectangle are unchanged. Without this, every Draw call allocates
	// a fresh *ebiten.Image wrapper per zone — at 60 FPS × ~10 zones that
	// was ~600 allocations/sec, the dominant per-frame allocator behind a
	// fast WASM OOM (see playback_alloc_throughput_test.go).
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

// RegisterZone adds a zone to the tree at a given z-index. The zone is
// also wrapped in a Layer adapter and inserted into the merged draw slice
// so a single ordered walk in Draw() handles both zones and decorative
// layers.
func (t *DrumViewTree) RegisterZone(z Zone, zIndex int) {
	t.RegisterZoneVisible(z, zIndex, nil)
}

// RegisterZoneVisible is RegisterZone with an optional visibility callback
// that the tree consults each frame before dispatching Draw. Useful for
// the historical perfDrawLite/simpleDraw flags that previously gated
// zone draws in DrumView.Draw — the tree now owns that decision.
func (t *DrumViewTree) RegisterZoneVisible(z Zone, zIndex int, visible func() bool) {
	// The visible predicate is stored on BOTH the layer (which gates
	// Draw via zoneAsLayer.Visible()) AND the zoneEntry (which gates
	// HitAreas via Update's Layout phase). Single source of truth at
	// the call site, two consumers internally — see zoneEntry.visible.
	e := &zoneEntry{zone: z, zIndex: zIndex, visible: visible, lastVisible: true}
	t.zones = append(t.zones, e)
	t.zoneMap[z.ID()] = e
	t.insertLayer(zoneAsLayer{zone: z, zIndex: zIndex, visible: visible})
}

// RegisterLayer adds a draw-only layer to the tree. Layers are interleaved
// with zones in the merged draw slice, sorted ascending by ZIndex().
// Decorative chrome (background fills, halos, notifications, debug
// overlays) lives here.
func (t *DrumViewTree) RegisterLayer(l Layer) {
	t.insertLayer(l)
}

// insertLayer inserts l into t.layers maintaining ascending ZIndex order.
// Stable: equal ZIndex values keep insertion order.
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

// LayersForTest returns a snapshot of the merged draw slice in render
// order. Used by drumview_layer_zorder_test.go to assert the z-index
// table is stable. Production code MUST NOT call this — iterating the
// real slice from outside the tree breaks the encapsulation that the
// pipeline-discipline test enforces.
func (t *DrumViewTree) LayersForTest() []Layer {
	out := make([]Layer, len(t.layers))
	copy(out, t.layers)
	return out
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
	t.layoutPass()

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

// layoutPass lays out zones that need it (NeedsLayout, rect change, or a
// visibility transition) and republishes their hit areas. This is the ONLY
// place layout and HitIndex publication may happen together — zone.Layout
// clears the zone's needLayout flag, so a Layout call that skips the
// publish strands the HitIndex on stale geometry forever (the "scrolled
// rack dispatches mute/solo to the wrong row" bug: a scroll landing after
// this pass — wheel adapter in the input phase, momentum in zone Update,
// legacy touch/step-drag after tree.Update — had its needLayout consumed by
// a Draw-time Layout that never republished).
//
// Visibility gate covers HitAreas in addition to Draw: when a zone's
// visible() returns false, the tree publishes an EMPTY hit-area
// slice for it. The pre-fix state half-wired the gate (Draw only),
// which let a hidden audio panel's `eq-panel-capture` catch-all
// swallow input destined for the row rack beneath when the user
// switched mobile bottom-nav from EQ back to Pads.
func (t *DrumViewTree) layoutPass() {
	for i := range t.zones {
		e := t.zones[i]
		nowVisible := e.visible == nil || e.visible()
		layoutChanged := e.zone.NeedsLayout() || e.rect != e.lastRect
		visibilityChanged := nowVisible != e.lastVisible
		switch {
		case !nowVisible:
			// Invisible: clear any previously-published hit areas for
			// this zone. We don't call Layout because (a) the zone has
			// no reason to recompute geometry while hidden and (b)
			// some zones (eq-panel) have side effects in Layout we
			// shouldn't trigger when hidden. Clearing once per
			// transition (visibilityChanged) is sufficient — the hit
			// index keeps the empty owner entry once removed.
			if visibilityChanged {
				t.hitIndex.Update(e.zone.ID(), nil)
			}
		case layoutChanged || visibilityChanged:
			// Visible AND (rect changed OR became visible this frame):
			// re-layout + re-publish.
			e.zone.Layout(e.rect)
			e.lastRect = e.rect
			t.hitIndex.Update(e.zone.ID(), e.zone.HitAreas())
		}
		e.lastVisible = nowVisible
	}
}

// EnsureLayouts runs the same layout+publish pass as Update's Phase 1.
// Draw paths call this instead of zone.Layout directly so a layout that
// happens at draw time (e.g. a scroll flushed after the Update-phase pass)
// can never strand the HitIndex on stale geometry. Idempotent — a clean
// tree makes this a no-op scan.
func (t *DrumViewTree) EnsureLayouts() {
	t.layoutPass()
}

// LayoutZoneNow forces an immediate Layout for one zone at its current
// tree rect and republishes its hit areas, keeping lastRect and the
// HitIndex coherent. This is the ONLY sanctioned way to force a zone
// layout outside the tree's own frame loop (discipline test:
// TestZoneLayoutRoutesThroughTreeDiscipline). Unlike layoutPass, the
// Layout runs even when the zone is currently hidden — callers use this
// to keep widget rects warm for an imminent reveal — but hidden zones
// publish an empty hit-area set so invisible controls can never take
// input. No-op when the id is not registered.
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

// Draw renders the merged Layer/Zone slice in ascending z-order, then
// portal overlays on top.
//
// Clipping policy (mirrors the pre-refactor behaviour the legacy
// drumview_draw.go path implemented by hand):
//   - **Zones** (entries wrapping an underlying Zone via zoneAsLayer) are
//     hard-clipped to the intersection of the tree bounds and their
//     widget rectangle via screen.SubImage. This isolates each zone to
//     its own WidgetBoard cell so the timeline cannot bleed into the
//     rack column, etc.
//   - **Plain Layers** (decorative chrome — background, halos,
//     notifications, debug overlays) receive the unclipped screen image.
//     They are responsible for confining themselves via their own rect
//     math (Visible() + the rects they pass to drawRect/DrawImage). This
//     matches the legacy direct-draw path and — crucially — keeps the
//     ebitestub test backend (where SubImage allocates an independent
//     buffer that is not blitted back to the parent) able to verify
//     pixels via screen.At() in pixel-regression tests.
//
// This is the SINGLE pixel-emission path for the drum pane. The
// render-pipeline discipline test forbids draw primitives in
// drumview_draw.go and drumview_toolbar.go, so any byte that lands on
// screen here originated from a registered Layer or Zone.
func (t *DrumViewTree) Draw(screen *ebiten.Image) {
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
	// Portal overlays on top (Z >= ZOverlayMin).
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

// ClearCapture drops any in-flight pointer capture without synthesizing a
// release. Used by setViewMode's transient-state reset so a drag that was
// live when the user switched tabs (e.g. holding a Sampler knob) cannot
// strand the dispatcher on a handler whose zone is now hidden. Clear only —
// the next press starts a fresh dispatch.
func (t *DrumViewTree) ClearCapture() {
	t.capturedHandler = nil
	t.capturedTag = ""
	t.suppress = false
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
