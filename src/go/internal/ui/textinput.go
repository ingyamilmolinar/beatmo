package ui

import (
	"image"
	"image/color"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
)

// Any printable rune in any script, so non-ASCII names can be typed; numeric fields keep the ASCII default.
func AcceptTextRune(r rune) bool {
	// IsGraphic rejects control and format runes (bidi overrides, zero-width joiners, the DEL sentinel); noncharacters
	// and surrogates are refused too, against pasted junk fonts choke on.
	if !unicode.IsGraphic(r) {
		return false
	}
	if r >= 0xD800 && r <= 0xDFFF { // surrogate halves (never valid standalone)
		return false
	}
	if r == 0xFFFE || r == 0xFFFF { // noncharacters
		return false
	}
	return true
}

// Bounds memory and per-frame work against a pathological paste even when MaxLen is 0.
const textInputHardMaxRunes = 1024

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
	// nil allows the default set.
	Accept func(rune) bool
	// MaxLen, when >0, caps the number of runes permitted in Text.
	MaxLen int
	// Soft keyboard integration (mobile WASM)
	OnFocusGained func() // called when focus is acquired
	OnFocusLost   func() // called when focus is lost
	InputMode     string // "numeric", "text", etc. — passed to soft keyboard
	prevFocused   bool   // tracks focus transitions
	prevMouseDown bool   // tracks left-button press edges (re-focus on re-click)
	// Draw skips while the native <input> covers the box.
	MobileInputID string
	// Leave empty for domain editors that emit their own event (rename, BPM, search).
	LogField string
	// Room for inline adornments without shrinking the visible field.
	PadLeft  int
	PadRight int

	// Native keyboard wiring (see textfield.go): set by NewTextField.
	native   TextFieldSpec
	reg      *textFieldRegistry
	liveTick uint64 // registry tick of the last Update
	listed   bool   // present in reg.fields

	// nil keeps the default near-white.
	TextColor color.Color
}

// NewTextInput constructs a text input with the given rectangle and style.
func NewTextInput(r image.Rectangle, style TextInputStyle) *TextInput {
	return &TextInput{Rect: r, Style: style, repeat: make(map[ebiten.Key]int)}
}

// Never below minW.
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

// OnFocusGained/Lost still fire on the next Update, so the soft-keyboard wiring stays consistent.
func (t *TextInput) SetFocus(focus bool) {
	t.focused = focus
}

// Production uses SetFocus or real input.
func (t *TextInput) FocusForTest(focus bool) { t.focused = focus }

// The smaller of MaxLen and the hard cap.
func (t *TextInput) effectiveMaxLen() int {
	if t.MaxLen > 0 && t.MaxLen < textInputHardMaxRunes {
		return t.MaxLen
	}
	return textInputHardMaxRunes
}

// Truncated to effectiveMaxLen; content filtering is the typing path's job.
func (t *TextInput) SetText(s string) {
	rs := []rune(s)
	if m := t.effectiveMaxLen(); len(rs) > m {
		rs = rs[:m]
		s = string(rs)
	}
	t.Text = s
	t.cursor = utf8.RuneCountInString(s)
}

// Value returns the current text value.
func (t *TextInput) Value() string { return t.Text }

// Read-only, for tests and the instMenuSearchState export.
func (t *TextInput) CursorForTest() int { return t.cursor }

// Update processes mouse/keyboard input.

func (t *TextInput) Update() bool {
	if t.reg != nil {
		t.reg.markLive(t) // live this frame → native keyboard wiring armed
	}
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
	// The press edge lets a re-click on a focused box re-assert focus.
	mouseDownNow := isMouseButtonPressed(ebiten.MouseButtonLeft) && !skipClick
	pressEdge := mouseDownNow && !t.prevMouseDown
	t.prevMouseDown = mouseDownNow
	clickedInside := false
	if isMouseButtonPressed(ebiten.MouseButtonLeft) && !skipClick {
		if image.Pt(mx, my).In(t.Rect) {
			clickedInside = pressEdge
			t.focused = true
			t.anim = 1
			_, start := t.visibleText()
			rel := mx - (t.Rect.Min.X + 4 + t.PadLeft)
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
	} else if t.focused && clickedInside {
		// A re-click blurs the hidden proxy (focus moves to the canvas) with no transition to re-fire OnFocusGained,
		// dropping the box to the lossy keyboard path. Press edge only: OnFocusGained clears the proxy buffer.
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

	// Caret moves apply BEFORE the edit drain so a batched "move then edit" lands at the moved caret; the counts let
	// several coalesced presses each move. keyRepeat runs every frame to keep its state.
	leftMoves := consumeSoftKeyboardArrowLeft()
	if t.keyRepeat(ebiten.KeyLeft) {
		leftMoves++
	}
	for i := 0; i < leftMoves && t.cursor > 0; i++ {
		t.cursor--
	}
	rightMoves := consumeSoftKeyboardArrowRight()
	if t.keyRepeat(ebiten.KeyRight) {
		rightMoves++
	}
	runeCount := utf8.RuneCountInString(t.Text)
	for i := 0; i < rightMoves && t.cursor < runeCount; i++ {
		t.cursor++
	}

	// After the relative moves; the later jump wins (both idempotent). keyRepeat runs unconditionally to keep its state.
	homeJumps := consumeSoftKeyboardHome()
	if t.keyRepeat(ebiten.KeyHome) {
		homeJumps++
	}
	if homeJumps > 0 {
		t.cursor = 0
	}
	endJumps := consumeSoftKeyboardEnd()
	if t.keyRepeat(ebiten.KeyEnd) {
		endJumps++
	}
	if endJumps > 0 {
		t.cursor = utf8.RuneCountInString(t.Text)
	}

	// The soft keyboard when active, else Ebiten's InputChars().
	var chars []rune
	if softKeyboardActive() {
		skChars := softKeyboardDrainChars()
		for _, r := range skChars {
			switch r {
			case '\b': // backspace from the soft keyboard
				t.deleteBackward()
				consumed = true
			case '\x7f': // DEL: forward-delete sentinel from the proxy keydown handler
				t.deleteForward()
				consumed = true
			default:
				chars = append(chars, r)
			}
		}
	} else {
		chars = inputChars()
	}
	// A pathological paste cannot spin the insert loop.
	if len(chars) > textInputHardMaxRunes {
		chars = chars[:textInputHardMaxRunes]
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
				if utf8.RuneCountInString(t.Text) >= t.effectiveMaxLen() {
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
	// No emit here: the blur branch next Update is the sole EventTextCommitted emitter, or it would fire twice.
	if t.focused && isKeyPressed(ebiten.KeyEnter) {
		t.focused = false
		return true
	}

	if t.keyRepeat(ebiten.KeyBackspace) {
		t.deleteBackward()
	}
	// Native forward-delete (WASM routes it through the '\x7f' char sentinel above).
	if t.keyRepeat(ebiten.KeyDelete) {
		t.deleteForward()
	}
	return consumed
}

// No-op at the start.
func (t *TextInput) deleteBackward() {
	if t.cursor <= 0 {
		return
	}
	bi := byteIndex(t.Text, t.cursor)
	prev := byteIndex(t.Text, t.cursor-1)
	t.Text = t.Text[:prev] + t.Text[bi:]
	t.cursor--
}

// The caret stays; no-op at the end.
func (t *TextInput) deleteForward() {
	if t.cursor >= utf8.RuneCountInString(t.Text) {
		return
	}
	bi := byteIndex(t.Text, t.cursor)
	next := byteIndex(t.Text, t.cursor+1)
	t.Text = t.Text[:bi] + t.Text[next:]
}

func (t *TextInput) keyRepeat(k ebiten.Key) bool {
	// One repeat cadence for every text input: immediate, ~400 ms delay, then ~10 Hz.
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
	maxW := t.Rect.Dx() - pad*2 - t.PadLeft - t.PadRight
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
	if tc := liveCaptured(t.TextColor); tc != nil {
		DrawTextColorAt(dst, txt, t.Rect.Min.X+4+t.PadLeft, ty, tc)
	} else {
		DrawTextAt(dst, txt, t.Rect.Min.X+4+t.PadLeft, ty)
	}
	if t.focused && t.blink < 30 {
		// Cursor x from the width of text before cursor position.
		rs := []rune(t.Text)
		bi := byteIndex(t.Text, start)
		ci := byteIndex(t.Text, min(t.cursor, len(rs)))
		prefix := t.Text[bi:ci]
		cx := t.Rect.Min.X + 4 + t.PadLeft + TextWidth(prefix)
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

// Local, to avoid an import cycle.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
