package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// BaseComponent provides a reusable base implementation of the Component
// interface. Embed this in concrete components to get default behavior
// for common operations.
type BaseComponent struct {
	id        string
	bounds    image.Rectangle
	children  []Component
	parent    Container
	capturing bool
	mounted   bool
}

// NewBaseComponent creates a new base component with the given ID.
func NewBaseComponent(id string) *BaseComponent {
	return &BaseComponent{id: id}
}

// ID returns the component's unique identifier.
func (b *BaseComponent) ID() string { return b.id }

// Mount is called when the component is added to a parent container.
func (b *BaseComponent) Mount(ctx MountContext) {
	b.parent = ctx.Parent
	b.bounds = ctx.Bounds
	b.mounted = true
	// Mount all children
	for _, c := range b.children {
		c.Mount(MountContext{Parent: b, Bounds: c.Bounds()})
	}
}

// Unmount is called when the component is removed from its parent.
func (b *BaseComponent) Unmount() {
	// Unmount all children first
	for _, c := range b.children {
		c.Unmount()
	}
	b.mounted = false
	b.parent = nil
}

// Bounds returns the component's bounding rectangle.
func (b *BaseComponent) Bounds() image.Rectangle { return b.bounds }

// SetBounds sets the component's bounding rectangle.
func (b *BaseComponent) SetBounds(r image.Rectangle) { b.bounds = r }

// Capturing returns whether this component is currently capturing input.
func (b *BaseComponent) Capturing() bool { return b.capturing }

// SetCapturing sets the capture state. Used by subclasses during input handling.
func (b *BaseComponent) SetCapturing(v bool) { b.capturing = v }

// Parent returns the parent container, if any.
func (b *BaseComponent) Parent() Container { return b.parent }

// IsMounted returns whether the component is currently mounted.
func (b *BaseComponent) IsMounted() bool { return b.mounted }

// HandleInput dispatches input to children in reverse order (top-to-bottom).
// Override this method in subclasses to add component-specific input handling.
func (b *BaseComponent) HandleInput(x, y int, pressed bool) InputResult {
	pt := image.Pt(x, y)
	// Dispatch to children in reverse order (last child = topmost)
	for i := len(b.children) - 1; i >= 0; i-- {
		c := b.children[i]
		if pt.In(c.Bounds()) {
			if result := c.HandleInput(x, y, pressed); result != InputIgnored {
				return result
			}
		}
	}
	return InputIgnored
}

// HandleWheel dispatches wheel events to children.
// Override this method in subclasses to add component-specific wheel handling.
func (b *BaseComponent) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}

// Draw renders all children in order (first child = bottom-most).
// Override this method in subclasses to add component-specific rendering.
func (b *BaseComponent) Draw(dst *ebiten.Image) {
	for _, c := range b.children {
		c.Draw(dst)
	}
}

// Children returns the list of child components.
func (b *BaseComponent) Children() []Component { return b.children }

// AddChild adds a child component to this container.
func (b *BaseComponent) AddChild(c Component) {
	b.children = append(b.children, c)
	if b.mounted {
		c.Mount(MountContext{Parent: b, Bounds: c.Bounds()})
	}
}

// RemoveChild removes a child component by ID.
func (b *BaseComponent) RemoveChild(id string) {
	for i, c := range b.children {
		if c.ID() == id {
			if b.mounted {
				c.Unmount()
			}
			// Remove from slice
			b.children = append(b.children[:i], b.children[i+1:]...)
			return
		}
	}
}

// FindChild returns the child with the given ID, or nil if not found.
func (b *BaseComponent) FindChild(id string) Component {
	for _, c := range b.children {
		if c.ID() == id {
			return c
		}
	}
	return nil
}

// ClearChildren removes all child components.
func (b *BaseComponent) ClearChildren() {
	if b.mounted {
		for _, c := range b.children {
			c.Unmount()
		}
	}
	b.children = nil
}

// ChildAt returns the topmost child at the given point, or nil if none.
func (b *BaseComponent) ChildAt(x, y int) Component {
	pt := image.Pt(x, y)
	for i := len(b.children) - 1; i >= 0; i-- {
		c := b.children[i]
		if pt.In(c.Bounds()) {
			return c
		}
	}
	return nil
}

// Interface assertion
var _ Container = (*BaseComponent)(nil)
