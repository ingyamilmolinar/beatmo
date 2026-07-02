package ui

import (
	"image"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// root_tree.go — the single root of the DrumView component hierarchy.
//
// RootTree composes child DrumViewTree subtrees, each of which owns its own
// HitIndex, capture lifecycle, and overlay portal. The root ticks every
// subtree (layout/update), routes each frame's input to exactly ONE subtree
// (capture-holder → blocking-portal owner → top-down by z), and draws all
// subtree CONTENT then all subtree OVERLAYS so portals/modals composite above
// every subtree.
//
// Isolation is enforced by routing + per-subtree HitIndexes, NOT by spatially
// disjoint hit areas (they are not disjoint — e.g. the row-rack scroll
// catch-all's rect spans the whole drum pane, overlapping the audio panel
// region). Instead: (1) each subtree has its OWN HitIndex, so a press/wheel is
// only ever tested against the zones of the subtree it is dispatched to; (2)
// the root dispatches the higher-z subtree first and WITHHOLDS new presses
// from a lower subtree once an upper one is busy; (3) opaque panel zones
// consume input in their own region (the eq-panel catch-all consumes press AND
// wheel). Together these make a coordinate in one subtree's region unable to
// act on another — so cross-section input bugs (e.g. a wheel over a synth knob
// scrolling the drum rows) cannot occur. The one deliberate cross-subtree
// affordance is DismissOnForeignPress: a withheld subtree may CLOSE a
// non-blocking docked overlay (e.g. the FX panel) on a foreign press, but
// never dispatches that press to its own zones.
//
// Children are ordered by z: lower z draws first / dispatched last; higher z
// draws last (on top) / dispatched first.
type RootTree struct {
	children   []rootChild
	bounds     image.Rectangle
	wasPressed bool
}

type rootChild struct {
	tree *DrumViewTree
	z    int
	name string
}

func NewRootTree() *RootTree { return &RootTree{} }

// AddChild registers a subtree at a z-index. Higher z = drawn on top and
// given input priority in spatial overlaps.
func (r *RootTree) AddChild(name string, tree *DrumViewTree, z int) {
	r.children = append(r.children, rootChild{tree: tree, z: z, name: name})
	sort.SliceStable(r.children, func(i, j int) bool { return r.children[i].z < r.children[j].z })
}

// SetBounds propagates the full DrumView bounds to every subtree (used for
// portal layout so full-screen modals center correctly). Per-zone content is
// confined by each zone's own rect during layout, not by these bounds.
func (r *RootTree) SetBounds(b image.Rectangle) {
	r.bounds = b
	for _, c := range r.children {
		c.tree.SetBounds(b)
	}
}

// Update ticks all subtrees, then dispatches input to exactly one.
func (r *RootTree) Update() {
	for _, c := range r.children {
		c.tree.Tick()
	}
	r.dispatchInput()
	sup := false
	for _, c := range r.children {
		sup = sup || c.tree.Suppressing()
	}
	suppressClicksUntilRelease = sup
}

func (r *RootTree) dispatchInput() {
	// Track the fresh-press edge BEFORE the early returns so the bookkeeping
	// stays consistent across all three dispatch tiers.
	pressedNow := isMouseButtonPressed(ebiten.MouseButtonLeft)
	freshPress := pressedNow && !r.wasPressed
	r.wasPressed = pressedNow

	// 1) If a subtree holds capture, ONLY it acts (so a drag started in one
	//    subtree is never misread as a new press in another).
	if c := r.capturer(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// 2) Else if a subtree has a BLOCKING portal (modal/scrim menu/picker/
	//    dropdown/popup), ONLY it acts. A blocking overlay owns the next click:
	//    its scrim absorbs click-outside to close, and its hit areas must win
	//    over any sibling subtree's zones. Without this, a scrim menu open in
	//    one subtree would let the OTHER subtree (dispatched first by z in the
	//    fresh-dispatch tier) consume the click-outside, stranding the menu
	//    open and leaking input to the wrong surface. Passive overlays
	//    (tooltips, docked panels with no scrim) are deliberately EXCLUDED here
	//    so they don't freeze input to the sibling subtree. Top-most (highest
	//    z) blocking-portal owner wins when both subtrees have blocking
	//    overlays open.
	if c := r.blockingPortalOwner(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// 3) Else fresh dispatch top-down by z: the first subtree that handles
	//    input gates the rest. Isolation does NOT come from spatially disjoint
	//    hit areas (they overlap — see file header); it comes from this
	//    top-down dispatch + the busyAbove gate withholding the press from
	//    lower subtrees + each subtree's separate HitIndex. This also gives
	//    the on-top subtree priority in any boundary overlap. A WITHHELD
	//    subtree (allow==false,
	//    its sibling above consumed the press) still gets to dismiss a
	//    non-blocking docked overlay on a foreign fresh press — a courtesy
	//    close only, NOT a zone dispatch (see DismissOnForeignPress), so the
	//    FX panel closes when a click lands in the sibling section without
	//    leaking that press to this subtree's controls (isolation holds).
	mx, my := cursorPosition()
	busyAbove := false
	for i := len(r.children) - 1; i >= 0; i-- {
		allow := !busyAbove
		busy := r.children[i].tree.DispatchInput(allow)
		if !allow && freshPress {
			r.children[i].tree.DismissOnForeignPress(mx, my)
		}
		busyAbove = busyAbove || busy
	}
}

func (r *RootTree) capturer() *DrumViewTree {
	for _, c := range r.children {
		if c.tree.Capturing() {
			return c.tree
		}
	}
	return nil
}

// HasBlockingPortal reports whether ANY composed subtree currently has a
// BLOCKING overlay (modal or scrim-backed menu/picker/dropdown/popup) open. A
// blocking overlay owns input exclusively, so out-of-tree Game-level handlers
// (grid tap, camera pan/zoom, two-finger pan) must stand down while one is up.
func (r *RootTree) HasBlockingPortal() bool {
	return r.blockingPortalOwner() != nil
}

// blockingPortalOwner returns the top-most (highest z) subtree that has a
// BLOCKING overlay (modal or scrim-backed menu/picker/dropdown/popup) open in
// its portal. A blocking overlay owns the next click cycle, so input is routed
// exclusively to its subtree. Passive overlays (tooltips, docked panels with
// no scrim) are excluded — they must not seize input from a sibling subtree.
func (r *RootTree) blockingPortalOwner() *DrumViewTree {
	for i := len(r.children) - 1; i >= 0; i-- {
		if r.children[i].tree.PortalHasBlocking() {
			return r.children[i].tree
		}
	}
	return nil
}

// Draw composites all subtree content (ascending z), then all subtree
// overlays (ascending z), so portals/modals sit above every subtree.
func (r *RootTree) Draw(screen *ebiten.Image) {
	for _, c := range r.children {
		c.tree.DrawContent(screen)
	}
	for _, c := range r.children {
		c.tree.DrawOverlays(screen)
	}
}

// EnsureLayouts re-runs each subtree's layout+publish pass (used by Draw
// paths that may run without a preceding Update).
func (r *RootTree) EnsureLayouts() {
	for _, c := range r.children {
		c.tree.EnsureLayouts()
	}
}

// ClearCapture drops any in-flight capture across all subtrees.
func (r *RootTree) ClearCapture() {
	for _, c := range r.children {
		c.tree.ClearCapture()
	}
}

// ResetPressEdge clears the root's fresh-press bookkeeping so the next
// dispatch treats the pointer as a new press. Used by the touch tap-injection
// path (game_update.go): a synthesized tap must register as a fresh press at
// every dispatch tier — the subtrees AND the root — or the root's foreign-press
// dismissal edge (freshPress in dispatchInput) won't fire for the injected tap.
func (r *RootTree) ResetPressEdge() { r.wasPressed = false }

// Child returns the named subtree (for layout/zone-rect routing and tests).
func (r *RootTree) Child(name string) *DrumViewTree {
	for _, c := range r.children {
		if c.name == name {
			return c.tree
		}
	}
	return nil
}
