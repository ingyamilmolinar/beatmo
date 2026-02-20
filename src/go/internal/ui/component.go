package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Component is the unified interface for UI components that combines
// input handling, rendering, and lifecycle management.
type Component interface {
	// ID returns a unique identifier for this component.
	ID() string

	// Lifecycle methods
	Mount(ctx MountContext)
	Unmount()

	// Layout
	Bounds() image.Rectangle
	SetBounds(image.Rectangle)

	// Input handling (embeds InputHandler semantics)
	HandleInput(x, y int, pressed bool) InputResult
	Capturing() bool

	// Rendering
	Draw(dst *ebiten.Image)
}

// Container is an optional interface for components that have children.
type Container interface {
	Component
	Children() []Component
	AddChild(Component)
	RemoveChild(id string)
}

// MountContext provides context during component mounting.
// Note: This is separate from ComponentContext (used by ComponentRegistry)
// to avoid coupling the new component framework to the existing plugin system.
type MountContext struct {
	// Parent is the parent container, if any.
	Parent Container
	// Bounds is the initial bounds assigned to the component.
	Bounds image.Rectangle
}

// Drawable is a minimal interface for components that only need drawing.
// Useful for simple, non-interactive visual elements.
type Drawable interface {
	Draw(dst *ebiten.Image)
}

// Boundable is an interface for components that have spatial bounds.
type Boundable interface {
	Bounds() image.Rectangle
	SetBounds(image.Rectangle)
}

// InputHandlerComponent is an interface that combines Component with the
// existing InputHandler interface for backward compatibility.
type InputHandlerComponent interface {
	Component
	InputHandler
}

// ComponentAdapter wraps an existing InputHandler to make it compatible
// with the Component interface. This allows gradual migration.
type ComponentAdapter struct {
	id      string
	handler InputHandler
	mounted bool
}

// NewComponentAdapter creates a Component from an existing InputHandler.
func NewComponentAdapter(id string, handler InputHandler) *ComponentAdapter {
	return &ComponentAdapter{id: id, handler: handler}
}

func (a *ComponentAdapter) ID() string                  { return a.id }
func (a *ComponentAdapter) Mount(ctx MountContext)      { a.mounted = true }
func (a *ComponentAdapter) Unmount()                    { a.mounted = false }
func (a *ComponentAdapter) Bounds() image.Rectangle     { return a.handler.InputBounds() }
func (a *ComponentAdapter) SetBounds(r image.Rectangle) {} // No-op; handler manages its own bounds
func (a *ComponentAdapter) HandleInput(x, y int, p bool) InputResult {
	return a.handler.HandleInput(x, y, p)
}
func (a *ComponentAdapter) Capturing() bool { return a.handler.Capturing() }

func (a *ComponentAdapter) Draw(dst *ebiten.Image) {
	// Adapter doesn't handle drawing; the underlying handler
	// should be drawn separately if it has a Draw method.
}

func (a *ComponentAdapter) HandleWheel(x, y, steps int) InputResult {
	return a.handler.HandleWheel(x, y, steps)
}
