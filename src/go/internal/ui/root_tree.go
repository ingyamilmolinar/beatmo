package ui

import (
	"image"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
)

// Each subtree owns its HitIndex, capture and portal; input goes to exactly ONE (capture-holder → blocking portal
// → top-down by z) and overlays draw after all content. Isolation comes from routing, not disjoint rects.
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

// Higher z draws on top and gets input first.
func (r *RootTree) AddChild(name string, tree *DrumViewTree, z int) {
	r.children = append(r.children, rootChild{tree: tree, z: z, name: name})
	sort.SliceStable(r.children, func(i, j int) bool { return r.children[i].z < r.children[j].z })
}

// For portal layout; zone content is confined by each zone's own rect.
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
	// Before the early returns so the bookkeeping holds across all three tiers.
	pressedNow := isMouseButtonPressed(ebiten.MouseButtonLeft)
	freshPress := pressedNow && !r.wasPressed
	r.wasPressed = pressedNow

	// A drag begun in one subtree is never read as a new press in another.
	if c := r.capturer(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// A blocking overlay owns the next click, or the other subtree (dispatched first by z) eats the click-outside and
	// strands the menu open. Passive overlays are excluded so they never freeze the sibling.
	if c := r.blockingPortalOwner(); c != nil {
		for _, ch := range r.children {
			ch.tree.DispatchInput(ch.tree == c)
		}
		return
	}
	// The first subtree that handles input gates the rest. A WITHHELD subtree may still close a non-blocking docked
	// overlay on a foreign press, never dispatching it to its own zones.
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

// Out-of-tree Game handlers (grid tap, camera) stand down while one is up.
func (r *RootTree) HasBlockingPortal() bool {
	return r.blockingPortalOwner() != nil
}

// Any interactive overlay, so a press outside the docked FX panel or the channel dropdown lets its click-outside
// close it; passive tooltips excluded.
func (r *RootTree) HasInputOwningPortal() bool {
	for i := len(r.children) - 1; i >= 0; i-- {
		if r.children[i].tree.PortalOwnsInput() {
			return true
		}
	}
	return false
}

// Passive overlays are excluded: they must not seize a sibling's input.
func (r *RootTree) blockingPortalOwner() *DrumViewTree {
	for i := len(r.children) - 1; i >= 0; i-- {
		if r.children[i].tree.PortalHasBlocking() {
			return r.children[i].tree
		}
	}
	return nil
}

// The SAME precedence as dispatchInput: a native-gesture rect is armable only where a real press would reach its owner.
func (r *RootTree) TopmostOwnerAt(x, y int) string {
	if c := r.capturer(); c != nil {
		return c.topOwnerAt(x, y)
	}
	if c := r.blockingPortalOwner(); c != nil {
		return c.topOwnerAt(x, y)
	}
	for i := len(r.children) - 1; i >= 0; i-- {
		if o := r.children[i].tree.topOwnerAt(x, y); o != "" {
			return o
		}
	}
	return ""
}

func (r *RootTree) Draw(screen *ebiten.Image) {
	for _, c := range r.children {
		c.tree.DrawContent(screen)
	}
	for _, c := range r.children {
		c.tree.DrawOverlays(screen)
	}
}

// For Draw paths that run without an Update.
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

// A synthesized tap must be a fresh press at every tier, or the root's foreign-press dismissal never fires.
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
