package ui

import (
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// formatParamForEntry renders a param value for the numeric editor: the enum
// label for discrete params, else a bare number (no unit suffix).
func formatParamForEntry(def audio.ParamDef, v float64) string {
	if lbl := enumLabelFor(def, v); lbl != "" {
		return lbl
	}
	return trimFloat(v)
}

// parseParamEntry parses editor text into a param value. Continuous: parse +
// clamp to [Min,Max], reject non-finite. Discrete: case-insensitive enum-label
// match OR an in-range integer index.
func parseParamEntry(def audio.ParamDef, text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}
	if len(def.Enum) > 0 {
		for i, l := range def.Enum {
			if strings.EqualFold(l, text) {
				return float64(i), true
			}
		}
		if n, err := strconv.Atoi(text); err == nil && n >= 0 && n < len(def.Enum) {
			return float64(n), true
		}
		return 0, false
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if f < def.Min {
		f = def.Min
	} else if f > def.Max {
		f = def.Max
	}
	return f, true
}

// ValueSpec describes how a single numeric surface renders, parses, and sizes
// its value in the shared editor. Format produces the box text from the live
// value; Parse validates+clamps editor text (ok==false ⇒ revert + flash).
type ValueSpec struct {
	Format func(float64) string
	Parse  func(string) (float64, bool)
	MinW   int            // comfortable floor width (px); 0 ⇒ no minimum (fit text)
	Style  TextInputStyle // box style; zero value ⇒ BPMBoxStyle
	MaxLen int            // rune cap; 0 ⇒ editor default (12). Must accommodate the
	//                       widest value Format can produce, or SetText truncates the prefill.
}

// ValueOpen carries the per-open binding for the shared editor.
type ValueOpen struct {
	Spec          ValueSpec
	Anchor        image.Rectangle // readout the editor pops over
	Clamp         image.Rectangle // editor box stays inside this if non-empty
	Get           func() float64
	Set           func(float64)
	MobileInputID string // "" ⇒ desktop popup only; else native mobile input id
}

// ParamValueEditor is the single active numeric-entry box shared across tabs.
type ParamValueEditor struct {
	ti        *TextInput
	spec      ValueSpec
	setValue  func(float64)
	active    bool
	errorAnim float64
	mobileID  string
}

// NewParamValueEditor constructs the shared editor.
func NewParamValueEditor() *ParamValueEditor {
	ti := NewTextInput(image.Rectangle{}, BPMBoxStyle)
	ti.InputMode = "numeric"
	ti.MaxLen = 12
	ti.OnFocusGained = func() { softKeyboardShow("numeric") }
	ti.OnFocusLost = func() { softKeyboardHide() }
	return &ParamValueEditor{ti: ti}
}

// Active reports whether the editor is currently open.
func (e *ParamValueEditor) Active() bool { return e.active }

// OpenValue focuses the editor over o.Anchor, pre-filled with the current
// value (caret at end). The box auto-sizes to fit the value (min o.Spec.MinW)
// and is clamped inside o.Clamp when set.
func (e *ParamValueEditor) OpenValue(o ValueOpen) {
	if e.active {
		e.close()
	}
	e.errorAnim = 0 // clear any pending flash from a prior surface's failed commit
	e.spec = o.Spec
	e.setValue = o.Set
	e.mobileID = o.MobileInputID
	// TextInputStyle has only color.Color (interface) fields; an all-nil
	// style is the unset sentinel meaning "fall back to BPMBoxStyle". We
	// test the fields directly rather than comparing against a hand-coded
	// zero-value style literal (which the component-coverage ratchet flags).
	style := o.Spec.Style
	if style.Fill == nil && style.Border == nil && style.Cursor == nil {
		style = BPMBoxStyle
	}
	e.ti.Style = style
	if o.Spec.MaxLen > 0 {
		e.ti.MaxLen = o.Spec.MaxLen
	} else {
		e.ti.MaxLen = 12
	}
	e.ti.MobileInputID = o.MobileInputID
	text := ""
	if o.Spec.Format != nil && o.Get != nil {
		text = o.Spec.Format(o.Get())
	}
	e.ti.Rect = sizeEditorRect(o.Anchor, text, o.Spec.MinW, o.Clamp)
	e.ti.SetText(text)
	e.ti.SetFocus(true)
	if o.MobileInputID != "" && Profile().IsMobile() {
		r := e.ti.Rect
		mobileInputRegister(o.MobileInputID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), text, e.ti.MaxLen, "numeric")
	}
	e.active = true
}

// commit parses the text; on success writes + closes, on failure reverts
// (no write), flashes the error, and closes.
func (e *ParamValueEditor) commit() {
	if e.spec.Parse != nil {
		if v, ok := e.spec.Parse(e.ti.Value()); ok {
			if e.setValue != nil {
				e.setValue(v)
			}
			e.close()
			return
		}
	}
	e.errorAnim = 1
	e.close()
}

// cancel closes the editor without writing (Escape / revert).
func (e *ParamValueEditor) cancel() {
	e.close()
}

func (e *ParamValueEditor) close() {
	e.active = false
	e.ti.SetFocus(false)
	if e.mobileID != "" {
		mobileInputClose(e.mobileID)
		e.mobileID = ""
	}
}

// Update drives the wrapped TextInput; when focus is lost (Enter or click
// outside) it commits. Call once per frame. Also decays the error flash.
func (e *ParamValueEditor) Update() {
	if !e.active {
		if e.errorAnim > 0 {
			e.errorAnim *= 0.85
			if e.errorAnim < 0.01 {
				e.errorAnim = 0
			}
		}
		return
	}
	if e.mobileID != "" && Profile().IsMobile() && mobileInputActive(e.mobileID) {
		if val, committed, ok := mobileInputPollResult(e.mobileID); ok {
			if committed {
				e.ti.SetText(val)
				e.commit()
			} else {
				e.cancel()
			}
		}
		return
	}
	if isKeyPressed(ebiten.KeyEscape) {
		e.cancel()
		return
	}
	wasFocused := e.ti.Focused()
	e.ti.Update()
	if wasFocused && !e.ti.Focused() {
		e.commit()
	}
}

// Draw renders the editor box (and error flash). The text input draws only
// when active, but the error flash draws whenever errorAnim > 0 so the
// red pulse is visible even after the editor has closed on invalid input.
func (e *ParamValueEditor) Draw(dst *ebiten.Image) {
	if e.active {
		e.ti.Draw(dst)
	}
	if e.errorAnim > 0 {
		drawRect(dst, e.ti.Rect, fadeColor(colError, e.errorAnim), false)
	}
}

// Open keeps the legacy ParamDef entry point (used by existing tests). It
// builds a ValueSpec from def and opens a desktop-only popup (no native input).
func (e *ParamValueEditor) Open(def audio.ParamDef, anchor image.Rectangle, getValue func() float64, setValue func(float64)) {
	e.OpenValue(ValueOpen{
		Spec:   paramSpec(def),
		Anchor: anchor,
		Get:    getValue,
		Set:    setValue,
	})
}

// paramSpec builds a ValueSpec from a synth/sampler ParamDef.
func paramSpec(def audio.ParamDef) ValueSpec {
	d := def
	return ValueSpec{
		Format: func(v float64) string { return formatParamForEntry(d, v) },
		Parse:  func(s string) (float64, bool) { return parseParamEntry(d, s) },
		MinW:   72,
		Style:  BPMBoxStyle,
		MaxLen: 12,
	}
}

// sizeEditorRect returns the editor box rect: width grows to fit text (min
// minW), re-centered horizontally on anchor's center, height = anchor height,
// clamped inside clamp when clamp is non-empty.
func sizeEditorRect(anchor image.Rectangle, text string, minW int, clamp image.Rectangle) image.Rectangle {
	const pad = 4
	w := FitWidth(text, minW)
	cx := (anchor.Min.X + anchor.Max.X) / 2
	h := anchor.Dy()
	if h <= 0 {
		h = TextHeight() + 2*pad
	}
	r := image.Rect(cx-w/2, anchor.Min.Y, cx-w/2+w, anchor.Min.Y+h)
	if !clamp.Empty() {
		if r.Dx() > clamp.Dx() {
			r = image.Rect(clamp.Min.X, r.Min.Y, clamp.Max.X, r.Max.Y)
		}
		if r.Min.X < clamp.Min.X {
			r = r.Add(image.Pt(clamp.Min.X-r.Min.X, 0))
		}
		if r.Max.X > clamp.Max.X {
			r = r.Add(image.Pt(clamp.Max.X-r.Max.X, 0))
		}
	}
	return r
}
