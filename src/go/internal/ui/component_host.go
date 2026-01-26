package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// ComponentHost is the root container that bridges components to the
// Game/DrumView rendering pipeline. It manages a tree of components and
// handles input dispatch with capture semantics.
type ComponentHost struct {
	root    Container
	capture Component
	bounds  image.Rectangle
}

// NewComponentHost creates a new component host with the given bounds.
func NewComponentHost(bounds image.Rectangle) *ComponentHost {
	return &ComponentHost{
		bounds: bounds,
	}
}

// SetRoot sets the root container for this host.
func (h *ComponentHost) SetRoot(root Container) {
	if h.root != nil {
		h.root.Unmount()
	}
	h.root = root
	if root != nil {
		root.Mount(MountContext{Bounds: h.bounds})
	}
}

// Root returns the root container.
func (h *ComponentHost) Root() Container { return h.root }

// SetBounds updates the host's bounds and propagates to the root.
func (h *ComponentHost) SetBounds(bounds image.Rectangle) {
	h.bounds = bounds
	if h.root != nil {
		h.root.SetBounds(bounds)
	}
}

// Bounds returns the host's bounds.
func (h *ComponentHost) Bounds() image.Rectangle { return h.bounds }

// HandleInput dispatches input to the component tree with capture semantics.
// Returns true if any component consumed or captured the input.
func (h *ComponentHost) HandleInput(x, y int, pressed bool) bool {
	if h.root == nil {
		return false
	}

	// If a component has captured input (e.g., during a drag), it receives
	// all input until it releases capture.
	if h.capture != nil {
		result := h.capture.HandleInput(x, y, pressed)
		if !h.capture.Capturing() {
			h.capture = nil
		}
		return result != InputIgnored
	}

	// Dispatch to root (which will dispatch to children)
	pt := image.Pt(x, y)
	if !pt.In(h.bounds) {
		return false
	}

	result := h.root.HandleInput(x, y, pressed)
	if result == InputCaptured {
		h.capture = findCapturing(h.root)
	}
	return result != InputIgnored
}

// Draw renders the component tree.
func (h *ComponentHost) Draw(dst *ebiten.Image) {
	if h.root != nil {
		h.root.Draw(dst)
	}
}

// Capturing returns true if any component is currently capturing input.
func (h *ComponentHost) Capturing() bool {
	return h.capture != nil
}

// ReleaseCapture releases any current input capture.
func (h *ComponentHost) ReleaseCapture() {
	h.capture = nil
}

// findCapturing recursively searches for the component that is capturing input.
func findCapturing(c Component) Component {
	if c.Capturing() {
		return c
	}
	if cont, ok := c.(Container); ok {
		for _, child := range cont.Children() {
			if found := findCapturing(child); found != nil {
				return found
			}
		}
	}
	return nil
}

// WalkComponents calls fn for each component in the tree (depth-first).
func (h *ComponentHost) WalkComponents(fn func(Component)) {
	if h.root != nil {
		walkComponent(h.root, fn)
	}
}

func walkComponent(c Component, fn func(Component)) {
	fn(c)
	if cont, ok := c.(Container); ok {
		for _, child := range cont.Children() {
			walkComponent(child, fn)
		}
	}
}

// FindComponent searches for a component by ID in the tree.
func (h *ComponentHost) FindComponent(id string) Component {
	if h.root == nil {
		return nil
	}
	return findComponentByID(h.root, id)
}

func findComponentByID(c Component, id string) Component {
	if c.ID() == id {
		return c
	}
	if cont, ok := c.(Container); ok {
		for _, child := range cont.Children() {
			if found := findComponentByID(child, id); found != nil {
				return found
			}
		}
	}
	return nil
}
