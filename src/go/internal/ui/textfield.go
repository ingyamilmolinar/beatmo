package ui

import "image"

// The single way to put an editable box on screen: TextInput plus the box's native keyboard wiring (desktop proxy,
// mobile <input>). A field is LIVE while its owner calls Update, and syncNativeGestures arms every live one.
type TextField = TextInput

// TextFieldSpec is a text box's native identity.
type TextFieldSpec struct {
	// Declare it as a *MobileInputID constant; it keys the mobile input and its polls.
	ID string
	// The native rect is armed only while that owner is topmost at the box.
	Owner string
	// Searches leave it off: their native input already raises the keyboard.
	ProxyOnMobile bool
	// Defaults: the box rect, then the trigger rect.
	TriggerRect func() image.Rectangle
	InputRect   func() image.Rectangle
}

// Slack for Update/Draw cadence differences.
const textFieldLiveTicks = 2

// advance runs once per syncNativeGestures.
type textFieldRegistry struct {
	tick   uint64
	fields []*TextInput
}

func newTextFieldRegistry() *textFieldRegistry { return &textFieldRegistry{tick: 1} }

func (r *textFieldRegistry) advance() {
	if r != nil {
		r.tick++
	}
}

// markLive records that f was updated this tick (called from TextInput.Update).
func (r *textFieldRegistry) markLive(f *TextInput) {
	if r == nil {
		return
	}
	f.liveTick = r.tick
	if !f.listed {
		f.listed = true
		r.fields = append(r.fields, f)
	}
}

// A pruned field re-lists on its next Update.
func (r *textFieldRegistry) live() []*TextInput {
	if r == nil {
		return nil
	}
	kept := r.fields[:0]
	for _, f := range r.fields {
		recent := f.liveTick+textFieldLiveTicks >= r.tick
		if recent || (f.native.ID != "" && mobileInputActive(f.native.ID)) {
			kept = append(kept, f)
			continue
		}
		f.listed = false
	}
	for i := len(kept); i < len(r.fields); i++ {
		r.fields[i] = nil
	}
	r.fields = kept
	return kept
}

// maxLen caps the rune count.
func NewTextField(reg *textFieldRegistry, spec TextFieldSpec, maxLen int) *TextInput {
	f := NewTextInput(image.Rectangle{}, BPMBoxStyle)
	f.MaxLen = maxLen
	f.InputMode = "text"
	f.native = spec
	f.reg = reg
	f.OnFocusGained = func() {
		if spec.ProxyOnMobile || !Profile().IsMobile() {
			softKeyboardShow("text")
		}
	}
	f.OnFocusLost = func() {
		if spec.ProxyOnMobile || !Profile().IsMobile() {
			softKeyboardHide()
		}
	}
	return f
}

// NativeID returns the box's native-input id ("" for a box without one).
func (t *TextInput) NativeID() string { return t.native.ID }

// !ok on desktop, with no native input open, or when nothing changed.
func (t *TextInput) NativeLiveValue() (string, bool) {
	if t.native.ID == "" || !Profile().IsMobile() {
		return "", false
	}
	v, ok := mobileInputGetValue(t.native.ID)
	if !ok || v == t.Text {
		return "", false
	}
	return v, true
}

// fieldTriggerRect / fieldInputRect resolve the spec's optional rects.
func (t *TextInput) fieldTriggerRect() image.Rectangle {
	if t.native.TriggerRect != nil {
		return t.native.TriggerRect()
	}
	return t.Rect
}

func (t *TextInput) fieldInputRect() image.Rectangle {
	if t.native.InputRect != nil {
		return t.native.InputRect()
	}
	return image.Rectangle{}
}

// textFieldSoftKeyboardCandidates: desktop browser proxy rects for live fields.
func (dv *DrumView) textFieldSoftKeyboardCandidates() []NativeRectCandidate {
	var out []NativeRectCandidate
	for _, f := range dv.textFields.live() {
		if f.Rect.Empty() {
			continue
		}
		out = append(out, NativeRectCandidate{Owner: f.native.Owner, Rect: f.Rect,
			Intent: NativeIntent{Channel: NativeSoftKeyboard, ID: f.native.ID, InputMode: "text"}})
	}
	return out
}

// textFieldNativeInputCandidates: mobile native <input> rects for live fields.
func (dv *DrumView) textFieldNativeInputCandidates() []NativeRectCandidate {
	var out []NativeRectCandidate
	for _, f := range dv.textFields.live() {
		trig := f.fieldTriggerRect()
		if trig.Empty() {
			continue
		}
		out = append(out, NativeRectCandidate{Owner: f.native.Owner, Rect: trig,
			Intent: NativeIntent{Channel: NativeTextInput, ID: f.native.ID, Text: f.Text,
				MaxLen: f.MaxLen, InputMode: "text", InputRect: f.fieldInputRect()}})
	}
	return out
}

// DISPLAY-ONLY: never updated, so it never takes focus or keystrokes.
func newReadoutBox(style TextInputStyle) *TextInput {
	return NewTextInput(image.Rectangle{}, style)
}
