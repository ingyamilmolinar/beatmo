package ui

import "image"

// TextField is the single way to put an editable text box on screen.
//
// The editing core is TextInput (caret, selection, navigation, filters). What
// NewTextField adds is the box's NATIVE KEYBOARD WIRING, which every surface
// used to hand-wire — and two (the Save As dialogs and the FX "add effect"
// search) never did, so they had no keyboard on a phone:
//
//   - desktop browser: the hidden soft-keyboard proxy, which buffers every key
//     so a short press between two slow frames is never dropped;
//   - mobile: a real native <input> overlaid on the box (the soft keyboard
//     can only be raised by a native element inside the user's gesture).
//
// A field is LIVE while its owner keeps calling Update (every frame it is on
// screen) or while its native input is open. syncNativeGestures arms the
// native rect of every live field (textFieldRegistry.live), so a new text box
// gets a keyboard on both platforms with no edit to native_gesture.go.
type TextField = TextInput

// TextFieldSpec is a text box's native identity.
type TextFieldSpec struct {
	// ID is the stable native-input id ("inst-search", "wav-name", …). It
	// keys the mobile <input> and the result/value polls. Declare it as a
	// *MobileInputID constant (TestTextFieldIDsAreUnique).
	ID string
	// Owner is the DrumViewTree owner tag of the surface the box lives on;
	// the native rect is armed only while that owner is topmost at the box.
	Owner string
	// ProxyOnMobile also focuses the soft-keyboard proxy on mobile (the
	// rename and WAV-name boxes historically did; searches do not, because
	// their native input already raises the keyboard).
	ProxyOnMobile bool
	// TriggerRect, when set, is where a mobile tap opens the native input
	// (default: the box rect). InputRect, when set, is where the native
	// input is placed (default: the trigger rect).
	TriggerRect func() image.Rectangle
	InputRect   func() image.Rectangle
}

// textFieldLiveTicks is how many sync ticks a field stays live after its last
// Update — slack for Update/Draw cadence differences.
const textFieldLiveTicks = 2

// textFieldRegistry tracks a DrumView's text fields. advance is called once
// per syncNativeGestures (the per-frame native projection).
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

// live returns the fields updated within the last textFieldLiveTicks ticks or
// whose native input is open, pruning the rest (a pruned field re-lists on
// its next Update).
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

// NewTextField builds a text box with its native keyboard wiring. reg is the
// owning DrumView's registry (dv.textFields); maxLen caps the rune count.
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

// NativeLiveValue returns the mobile native input's current value when it
// differs from the box (searches mirror it every frame). ok=false on desktop,
// when no native input is open, or when nothing changed.
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

// newReadoutBox builds a DISPLAY-ONLY box in a text-input style: a value
// readout drawn like a field (the transport BPM readout) that is never updated,
// so it never takes focus or keystrokes and has no native keyboard wiring. Its
// value is set by its owner through SetText.
func newReadoutBox(style TextInputStyle) *TextInput {
	return NewTextInput(image.Rectangle{}, style)
}
