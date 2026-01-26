package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// RenameProps contains the external state passed to the rename component.
type RenameProps struct {
	// AnchorRect is used for positioning the text input.
	AnchorRect image.Rectangle
	// InitialText is the initial text to show in the input.
	InitialText string
	// MaxLen is the maximum length of the input.
	MaxLen int
	// OnCommit is called when the rename is committed (Enter pressed).
	OnCommit func(newName string)
	// OnCancel is called when the rename is cancelled (Escape or click outside).
	OnCancel func()
}

// RenameState contains the internal state for the rename component.
type RenameState struct {
	open bool
	hold bool // Capture flag (set when opened, released on mouse-up)
}

// RenameComponent is a self-contained text input for renaming.
type RenameComponent struct {
	BaseComponent
	props   RenameProps
	state   RenameState
	textBox *TextInput
}

// NewRenameComponent creates a new rename component.
func NewRenameComponent(id string) *RenameComponent {
	return &RenameComponent{
		BaseComponent: *NewBaseComponent(id),
	}
}

// SetProps updates the external props.
func (r *RenameComponent) SetProps(p RenameProps) {
	r.props = p
}

// Props returns the current props.
func (r *RenameComponent) Props() RenameProps { return r.props }

// Open opens the rename dialog with the current props.
func (r *RenameComponent) Open() {
	rect := r.props.AnchorRect
	if rect.Empty() {
		return
	}
	r.textBox = NewTextInput(rect, BPMBoxStyle)
	r.textBox.MaxLen = r.props.MaxLen
	if r.textBox.MaxLen == 0 {
		r.textBox.MaxLen = 32 // default
	}
	r.textBox.SetText(r.props.InitialText)
	r.textBox.focused = true
	r.textBox.anim = 1
	r.state.open = true
	r.state.hold = true
	r.SetBounds(rect)
}

// Close closes the rename dialog.
func (r *RenameComponent) Close() {
	r.state.open = false
	r.state.hold = false
	r.textBox = nil
	r.SetBounds(image.Rectangle{})
}

// IsOpen returns whether the rename dialog is currently open.
func (r *RenameComponent) IsOpen() bool {
	return r.state.open && r.textBox != nil
}

// Value returns the current text value.
func (r *RenameComponent) Value() string {
	if r.textBox == nil {
		return ""
	}
	return r.textBox.Value()
}

// HandleInput processes mouse and keyboard input for the rename dialog.
func (r *RenameComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !r.state.open || r.textBox == nil {
		return InputIgnored
	}

	// If hold is active (just opened), capture until release
	if r.state.hold {
		if !pressed {
			r.state.hold = false
		}
		return InputCaptured
	}

	pt := image.Pt(x, y)

	// Check for Enter to commit BEFORE textBox.Update() consumes the key
	if isKeyPressed(ebiten.KeyEnter) {
		if r.props.OnCommit != nil {
			r.props.OnCommit(r.textBox.Value())
		}
		r.Close()
		return InputConsumed
	}

	// Check for Escape to cancel
	if isKeyPressed(ebiten.KeyEscape) {
		if r.props.OnCancel != nil {
			r.props.OnCancel()
		}
		r.Close()
		return InputConsumed
	}

	// Handle keyboard input via TextInput.Update()
	// Note: TextInput.Update() checks ebiten keyboard state internally
	if r.textBox.Update() {
		return InputConsumed
	}

	// If pressed outside text box, cancel
	if pressed && !pt.In(r.textBox.Rect) {
		if r.props.OnCancel != nil {
			r.props.OnCancel()
		}
		r.Close()
		SuppressClicksUntilMouseUp()
		return InputConsumed
	}

	// If within bounds, consume to prevent click-through
	if pt.In(r.textBox.Rect) {
		return InputConsumed
	}

	return InputIgnored
}

// Draw renders the rename text input.
func (r *RenameComponent) Draw(dst *ebiten.Image) {
	if !r.state.open || r.textBox == nil {
		return
	}
	r.textBox.Draw(dst)
}

// Capturing returns whether the component is capturing input.
func (r *RenameComponent) Capturing() bool {
	return r.state.hold
}

// InputBounds returns the text box bounds for overlay compatibility.
func (r *RenameComponent) InputBounds() image.Rectangle {
	if !r.state.open || r.textBox == nil {
		return image.Rectangle{}
	}
	return r.textBox.Rect
}

// HandleWheel consumes wheel events to prevent pass-through.
func (r *RenameComponent) HandleWheel(x, y, steps int) InputResult {
	if !r.state.open {
		return InputIgnored
	}
	return InputConsumed
}
