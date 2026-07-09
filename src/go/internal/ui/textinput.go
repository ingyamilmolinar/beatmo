package ui

import (
	"image"
	"image/color"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
)

// AcceptTextRune is the input filter for free-text fields (e.g. instrument
// names): it permits any printable Unicode rune — letters, marks, digits,
// punctuation, symbols, and spaces across all scripts — so non-ASCII names
// (é, ñ, 日本語) can be typed. Control characters are rejected. Numeric fields
// keep the ASCII-only default instead (they set no Accept).
func AcceptTextRune(r rune) bool { return unicode.IsGraphic(r) }

// TextInput is a reusable editable text box with cursor support.
type TextInput struct {
	Rect    image.Rectangle
	Style   TextInputStyle
	Text    string
	cursor  int
	focused bool
	anim    float64
	blink   int
	repeat  map[ebiten.Key]int
	// Optional per-field validation and constraints
	// Accept, when non-nil, should return true if rune r is allowed.
	Accept func(rune) bool
	// MaxLen, when >0, caps the number of runes permitted in Text.
	MaxLen int
	// Soft keyboard integration (mobile WASM)
	OnFocusGained func() // called when focus is acquired
	OnFocusLost   func() // called when focus is lost
	InputMode     string // "numeric", "text", etc. — passed to soft keyboard
	prevFocused   bool   // tracks focus transitions
	// MobileInputID, when set, causes Draw() to skip rendering when the
	// corresponding mobile native input is active (the native HTML <input>
	// is visible instead).
	MobileInputID string
	// LogField, when non-empty, causes EventTextCommitted to be emitted on
	// Enter/blur with Field=LogField and Value=the committed text. Leave
	// empty (the default) for domain editors that emit their own event
	// (instrument rename, BPM entry, search query).
	LogField string
}

// NewTextInput constructs a text input with the given rectangle and style.
func NewTextInput(r image.Rectangle, style TextInputStyle) *TextInput {
	return &TextInput{Rect: r, Style: style, repeat: make(map[ebiten.Key]int)}
}

// FitWidth returns a box width that fits text (plus padding + caret margin),
// never below minW. Shared sizing for inline editable boxes.
func FitWidth(text string, minW int) int {
	const pad = 4
	const caretMargin = 8
	w := TextWidth(text) + 2*pad + caretMargin
	if w < minW {
		w = minW
	}
	return w
}

// Focused reports whether the input currently has focus.
func (t *TextInput) Focused() bool { return t.focused }

// SetFocus toggles focus programmatically. Used by the instrument
// menu to hand focus to the search input on '/' and to release it
// on Esc. OnFocusGained / OnFocusLost still fire on transitions in
// the next Update() call so soft-keyboard wiring stays consistent.
func (t *TextInput) SetFocus(focus bool) {
	t.focused = focus
}

// FocusForTest is a test-only helper that sets focus directly without
// going through Update's mouse/key handling. Production code should
// rely on SetFocus or natural mouse/key events.
func (t *TextInput) FocusForTest(focus bool) { t.focused = focus }

// SetText sets the current text and resets the cursor to the end.
func (t *TextInput) SetText(s string) {
	// Enforce MaxLen when set; do not filter content here beyond length.
	if t.MaxLen > 0 {
		rs := []rune(s)
		if len(rs) > t.MaxLen {
			rs = rs[:t.MaxLen]
		}
		s = string(rs)
	}
	t.Text = s
	t.cursor = utf8.RuneCountInString(s)
}

// Value returns the current text value.
func (t *TextInput) Value() string { return t.Text }

// CursorForTest returns the caret's rune index. Read-only observation seam
// for tests and the instMenuSearchState debug export.
func (t *TextInput) CursorForTest() int { return t.cursor }

// Update processes mouse/keyboard input.

func (t *TextInput) Update() bool {
	skipClick := false
	if suppressClicksUntilRelease {
		if isMouseButtonPressed(ebiten.MouseButtonLeft) {
			skipClick = true
		} else {
			suppressClicksUntilRelease = false
		}
	}
	mx, my := cursorPosition()
	consumed := false
	if isMouseButtonPressed(ebiten.MouseButtonLeft) && !skipClick {
		if image.Pt(mx, my).In(t.Rect) {
			t.focused = true
			t.anim = 1
			_, start := t.visibleText()
			rel := mx - (t.Rect.Min.X + 4)
			// Walk runes from start, accumulating TextWidth, to find
			// which character position the click falls on.
			rs := []rune(t.Text)
			idx := start
			accW := 0
			for i := start; i < len(rs); i++ {
				cw := TextWidth(string(rs[i]))
				if accW+cw/2 > rel {
					break
				}
				accW += cw
				idx = i + 1
			}
			if idx < 0 {
				idx = 0
			}
			if idx > len(rs) {
				idx = len(rs)
			}
			t.cursor = idx
			consumed = true
		} else {
			t.focused = false
		}
	}

	// Detect focus transitions for soft keyboard callbacks
	if t.focused && !t.prevFocused {
		if t.OnFocusGained != nil {
			t.OnFocusGained()
		}
	} else if !t.focused && t.prevFocused {
		emitTextCommitted(t.LogField, t.Value())
		if t.OnFocusLost != nil {
			t.OnFocusLost()
		}
	}
	t.prevFocused = t.focused

	if !t.focused {
		t.blink = 0
		t.anim *= 0.85
		if t.anim < 0.01 {
			t.anim = 0
		}
		return consumed
	}

	t.blink++
	if t.blink > 60 {
		t.blink = 0
	}

	// Gather input characters: prefer soft keyboard when active (mobile),
	// fall back to Ebiten's InputChars() (desktop/hardware keyboard).
	var chars []rune
	if softKeyboardActive() {
		skChars := softKeyboardDrainChars()
		for _, r := range skChars {
			if r == '\b' {
				// Handle backspace from soft keyboard
				if t.cursor > 0 {
					bi := byteIndex(t.Text, t.cursor)
					prev := byteIndex(t.Text, t.cursor-1)
					t.Text = t.Text[:prev] + t.Text[bi:]
					t.cursor--
				}
				consumed = true
			} else {
				chars = append(chars, r)
			}
		}
	} else {
		chars = inputChars()
	}

	if len(chars) > 0 {
		for _, r := range chars {
			if r == '\n' || r == '\r' {
				t.focused = false
				consumed = true
			} else {
				// Default filter to printable ASCII unless Accept overrides.
				if t.Accept != nil {
					if !t.Accept(r) {
						continue
					}
				} else {
					if r < 32 || r > 126 {
						continue
					}
				}
				if t.MaxLen > 0 && utf8.RuneCountInString(t.Text) >= t.MaxLen {
					continue
				}
				before := t.Text[:byteIndex(t.Text, t.cursor)]
				after := t.Text[byteIndex(t.Text, t.cursor):]
				t.Text = before + string(r) + after
				t.cursor++
			}
		}
		if !t.focused {
			return true
		}
	}
	// Treat Enter as commit even when it is not part of InputChars().
	// Note: do NOT call emitTextCommitted here. Setting focused=false causes
	// the blur-transition branch (line ~151) to fire on the next Update() frame,
	// which is the sole emitter for EventTextCommitted. Emitting here would
	// produce a double-emit (Enter frame + blur frame).
	if t.focused && isKeyPressed(ebiten.KeyEnter) {
		t.focused = false
		return true
	}

	if t.keyRepeat(ebiten.KeyBackspace) {
		if t.cursor > 0 {
			bi := byteIndex(t.Text, t.cursor)
			prev := byteIndex(t.Text, t.cursor-1)
			t.Text = t.Text[:prev] + t.Text[bi:]
			t.cursor--
		}
	}
	// Left/Right move the caret. The keyRepeat path covers desktop (and any
	// platform where the canvas receives the keydown); consumeSoftKeyboardArrow*
	// covers WASM, where the hidden soft-keyboard proxy holds keyboard focus and
	// the canvas never sees the arrow keydown (the proxy forwards it out-of-band).
	if t.keyRepeat(ebiten.KeyLeft) || consumeSoftKeyboardArrowLeft() {
		if t.cursor > 0 {
			t.cursor--
		}
	}
	if t.keyRepeat(ebiten.KeyRight) || consumeSoftKeyboardArrowRight() {
		if t.cursor < utf8.RuneCountInString(t.Text) {
			t.cursor++
		}
	}
	return consumed
}

func (t *TextInput) keyRepeat(k ebiten.Key) bool {
	// Consistent, smooth repeat across all text inputs:
	// - First press fires immediately
	// - Initial delay ~24 frames (~400ms at 60fps)
	// - Then repeat at a fixed interval of 6 frames (~10Hz)
	const delay = 24
	const interval = 6
	if isKeyPressed(k) {
		t.repeat[k]++
		d := t.repeat[k]
		if d == 1 {
			return true
		}
		if d > delay {
			if (d-delay)%interval == 0 {
				return true
			}
		}
	} else {
		t.repeat[k] = 0
	}
	return false
}

// byteIndex returns the byte index of rune i in s.
func byteIndex(s string, i int) int {
	if i <= 0 {
		return 0
	}
	bi := 0
	for n := 0; n < i && bi < len(s); n++ {
		_, sz := utf8.DecodeRuneInString(s[bi:])
		bi += sz
	}
	return bi
}

// visibleText returns substring that fits in the box and the index of the first rune shown.
func (t *TextInput) visibleText() (string, int) {
	pad := 4
	maxW := t.Rect.Dx() - pad*2
	total := utf8.RuneCountInString(t.Text)
	rs := []rune(t.Text)

	// Find how many runes fit from a given start position.
	fitFrom := func(start int) int {
		w := 0
		for i := start; i < total; i++ {
			cw := TextWidth(string(rs[i]))
			if w+cw > maxW {
				return i - start
			}
			w += cw
		}
		return total - start
	}

	start := 0
	maxRunes := fitFrom(0)
	if total > maxRunes {
		switch {
		case t.cursor <= maxRunes:
			start = 0
		case t.cursor >= total-maxRunes:
			start = total - fitFrom(total-maxRunes)
			if start < 0 {
				start = 0
			}
		default:
			start = t.cursor - maxRunes + 1
			if start < 0 {
				start = 0
			}
		}
		maxRunes = fitFrom(start)
	}
	bi := byteIndex(t.Text, start)
	end := byteIndex(t.Text, min(start+maxRunes, total))
	return t.Text[bi:end], start
}

// Draw renders the input.
func (t *TextInput) Draw(dst *ebiten.Image) {
	// Skip drawing when native mobile input is active for this TextInput
	if Profile().IsMobile() && t.MobileInputID != "" && mobileInputActive(t.MobileInputID) {
		return
	}
	t.Style.DrawAnimated(dst, t.Rect, t.focused, t.anim)
	txt, start := t.visibleText()
	th := TextHeight()
	ty := t.Rect.Min.Y + (t.Rect.Dy()-th)/2
	DrawTextAt(dst, txt, t.Rect.Min.X+4, ty)
	if t.focused && t.blink < 30 {
		// Cursor x from the width of text before cursor position.
		rs := []rune(t.Text)
		bi := byteIndex(t.Text, start)
		ci := byteIndex(t.Text, min(t.cursor, len(rs)))
		prefix := t.Text[bi:ci]
		cx := t.Rect.Min.X + 4 + TextWidth(prefix)
		cy := ty
		col := t.Style.Cursor
		if col == nil {
			if t.Style.Border != nil {
				col = t.Style.Border
			} else {
				col = color.White
			}
		}
		cw := debugCharW // cursor width stays thin
		r := image.Rect(cx, cy, cx+cw, cy+th)
		drawCursor(dst, r, col)
	}
}

var drawCursor = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	drawRect(dst, r, col, true)
}

// min and max remain for compatibility with other widgets that may override
// drawCursor; keep them exported locally to avoid import cycles.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
