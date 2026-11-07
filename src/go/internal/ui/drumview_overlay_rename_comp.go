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
	// When set on mobile, Open() uses the native HTML input instead of TextInput.
	MobileInputID string
	TextFields    *textFieldRegistry
}

// The desktop box's id; mobile renames type into per-row "rename-<row>" inputs.
const renameBoxMobileInputID = "rename"

var renameTextFieldSpec = TextFieldSpec{ID: renameBoxMobileInputID, Owner: "rename", ProxyOnMobile: true}

// RenameState contains the internal state for the rename component.
type RenameState struct {
	open     bool
	hold     bool   // Capture flag (set when opened, released on mouse-up)
	mobile   bool   // Mobile mode: native HTML input handles text entry
	mobileID string // Active mobile native input ID
}

// RenameComponent is a self-contained text input for renaming.
type RenameComponent struct {
	overlayBase
	props   RenameProps
	state   RenameState
	textBox *TextInput
}

// NewRenameComponent creates a new rename component.
func NewRenameComponent() *RenameComponent {
	return &RenameComponent{
		overlayBase: newOverlayBase(),
	}
}

// SetProps updates the external props.
func (r *RenameComponent) SetProps(p RenameProps) {
	r.props = p
}

// Props returns the current props.
func (r *RenameComponent) Props() RenameProps { return r.props }

// No hold: the portal's capture already prevents double dispatch of the opening press.
func (r *RenameComponent) Open() {
	rect := r.props.AnchorRect
	if rect.Empty() {
		return
	}

	if Profile().IsMobile() && r.props.MobileInputID != "" && mobileInputActive(r.props.MobileInputID) {
		r.state.open = true
		r.state.mobile = true
		r.state.mobileID = r.props.MobileInputID
		r.textBox = nil // No TextInput in mobile mode
		r.SetBounds(rect)
		return
	}

	maxLen := r.props.MaxLen
	if maxLen == 0 {
		maxLen = 32 // default
	}
	r.textBox = NewTextField(r.props.TextFields, renameTextFieldSpec, maxLen)
	r.textBox.Rect = rect
	r.textBox.Accept = AcceptTextRune
	r.textBox.SetText(r.props.InitialText)
	r.textBox.focused = true
	r.textBox.anim = 1
	r.state.open = true
	r.state.mobile = false
	r.state.mobileID = ""
	r.SetBounds(rect)
}

// Close closes the rename dialog.
func (r *RenameComponent) Close() {
	if r.state.mobile && r.state.mobileID != "" {
		mobileInputClose(r.state.mobileID)
	}
	r.state.open = false
	r.state.hold = false
	r.state.mobile = false
	r.state.mobileID = ""
	r.textBox = nil
	r.SetBounds(image.Rectangle{})
	softKeyboardHide()
}

// Discards the edit and resets renameRow, then false so the universal handler closes the portal.
func (r *RenameComponent) HandleEscape() bool {
	if r.props.OnCancel != nil {
		r.props.OnCancel()
	}
	r.Close()
	return false
}

// IsOpen returns whether the rename dialog is currently open.
func (r *RenameComponent) IsOpen() bool {
	if r.state.mobile {
		return r.state.open
	}
	return r.state.open && r.textBox != nil
}

// While open, every key belongs to the rename box.
func (r *RenameComponent) ClaimsKeyboard() bool {
	return r != nil && r.IsOpen()
}

// Value returns the current text value.
func (r *RenameComponent) Value() string {
	if r.textBox == nil {
		return ""
	}
	return r.textBox.Value()
}

// nil on mobile, which uses native text entry.
func (r *RenameComponent) TextBox() *TextInput {
	return r.textBox
}

// HandleInput processes mouse and keyboard input for the rename dialog.
func (r *RenameComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !r.state.open {
		return InputIgnored
	}

	// Mobile mode: poll native input for result
	if r.state.mobile {
		// If hold is active (just opened), capture until release
		if r.state.hold {
			if !pressed {
				r.state.hold = false
			}
			return InputCaptured
		}

		if val, committed, ok := mobileInputPollResult(r.state.mobileID); ok {
			if committed && val != "" {
				if r.props.OnCommit != nil {
					r.props.OnCommit(val)
				}
			} else {
				if r.props.OnCancel != nil {
					r.props.OnCancel()
				}
			}
			r.Close()
			return InputConsumed
		}
		// While mobile input is active, consume all input
		return InputConsumed
	}

	if r.textBox == nil {
		return InputIgnored
	}

	// Before the hold phase, so Enter works right after a programmatic open.
	if isKeyPressed(ebiten.KeyEnter) {
		if r.props.OnCommit != nil {
			r.props.OnCommit(r.textBox.Value())
		}
		r.Close()
		return InputConsumed
	}
	if isKeyPressed(ebiten.KeyEscape) {
		if r.props.OnCancel != nil {
			r.props.OnCancel()
		}
		r.Close()
		return InputConsumed
	}

	// If hold is active (just opened), capture until release
	if r.state.hold {
		if !pressed {
			r.state.hold = false
		}
		return InputCaptured
	}

	// The native input may not have been active yet when Open() ran.
	if Profile().IsMobile() && r.props.MobileInputID != "" {
		if val, committed, ok := mobileInputPollResult(r.props.MobileInputID); ok {
			if committed && val != "" {
				if r.props.OnCommit != nil {
					r.props.OnCommit(val)
				}
			} else {
				if r.props.OnCancel != nil {
					r.props.OnCancel()
				}
			}
			r.Close()
			return InputConsumed
		}
	}

	pt := image.Pt(x, y)

	wasFocused := r.textBox.Focused()
	if r.textBox.Update() {
		// A soft keyboard sends Enter as '\n', which defocuses without a key press: treat focus loss as commit.
		if wasFocused && !r.textBox.Focused() {
			if r.props.OnCommit != nil {
				r.props.OnCommit(r.textBox.Value())
			}
			r.Close()
		}
		return InputConsumed
	}

	// If pressed outside text box, cancel
	if pressed && !pt.In(r.textBox.Rect) {
		if r.props.OnCancel != nil {
			r.props.OnCancel()
		}
		r.Close()
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
	if !r.state.open || r.state.mobile || r.textBox == nil {
		return
	}
	r.textBox.Draw(dst)
}

func (r *RenameComponent) ClearHold() {
	r.state.hold = false
}

// Capturing returns whether the component is capturing input.
func (r *RenameComponent) Capturing() bool {
	return r.state.hold
}

// InputBounds returns the text box bounds for overlay compatibility.
func (r *RenameComponent) InputBounds() image.Rectangle {
	if !r.state.open {
		return image.Rectangle{}
	}
	if r.state.mobile {
		return r.props.AnchorRect
	}
	if r.textBox == nil {
		return image.Rectangle{}
	}
	return r.textBox.Rect
}

// Called per frame by the portal's updateFn, outside HandleInput's mouse/hold logic.
func (r *RenameComponent) PollKeyboard() {
	if !r.state.open || r.state.mobile || r.textBox == nil {
		return
	}
	if isKeyPressed(ebiten.KeyEnter) {
		if r.props.OnCommit != nil {
			r.props.OnCommit(r.textBox.Value())
		}
		r.Close()
		return
	}
	if isKeyPressed(ebiten.KeyEscape) {
		if r.props.OnCancel != nil {
			r.props.OnCancel()
		}
		r.Close()
		return
	}
	// Poll mobile input on small screens (same as HandleInput).
	if Profile().IsMobile() && r.props.MobileInputID != "" {
		if val, committed, ok := mobileInputPollResult(r.props.MobileInputID); ok {
			if committed && val != "" {
				if r.props.OnCommit != nil {
					r.props.OnCommit(val)
				}
			} else {
				if r.props.OnCancel != nil {
					r.props.OnCancel()
				}
			}
			r.Close()
			return
		}
	}
	wasFocused := r.textBox.Focused()
	r.textBox.Update()
	// Soft keyboard sends Enter as '\n' → TextInput defocuses → detect as commit.
	if wasFocused && !r.textBox.Focused() {
		if r.props.OnCommit != nil {
			r.props.OnCommit(r.textBox.Value())
		}
		r.Close()
	}
}

// HandleWheel consumes wheel events to prevent pass-through.
func (r *RenameComponent) HandleWheel(x, y, steps int) InputResult {
	if !r.state.open {
		return InputIgnored
	}
	return InputConsumed
}
