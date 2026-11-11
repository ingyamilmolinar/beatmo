package ui

import "image"

// InputResult describes how a component processed input.
type InputResult int

const (
	InputIgnored  InputResult = iota // Did not handle
	InputConsumed                    // Handled, stop propagation
	InputCaptured                    // Handling ongoing drag
)

// InputHandler provides spatial input handling for UI components.
type InputHandler interface {
	// InputBounds returns the interactive area of the component.
	// Named InputBounds (not Bounds) to avoid conflict with struct fields.
	InputBounds() image.Rectangle
	ZIndex() int
	HandleInput(x, y int, pressed bool) InputResult
	HandleWheel(x, y, steps int) InputResult
	Capturing() bool
}
