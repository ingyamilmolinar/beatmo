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
func AcceptTextRune(r rune) bool {
	// unicode.IsGraphic already rejects control (Cc) and format (Cf) runes —
	// that excludes bidi overrides (U+202E), zero-width joiners, and the DEL
	// control used as our forward-delete sentinel — while permitting letters,
	// marks, digits, punctuation, symbols and spaces across all scripts. Reject
	// the noncharacters and unpaired-surrogate range explicitly as belt-and-
	// suspenders against pasted junk that some fonts choke on.
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

// textInputHardMaxRunes is an absolute defensive cap on the number of runes any
// text input will hold, enforced even when MaxLen is unset (0). It bounds memory
// and per-frame work against a pathological or malicious paste, independent of
// each field's semantic MaxLen (rename 32, search 40, …) which is usually far
// smaller. A single frame also never processes more than this many typed runes.
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
	prevMouseDown bool   // tracks left-button press edges (re-focus on re-click)
	// MobileInputID, when set, causes Draw() to skip rendering when the
	// corresponding mobile native input is active (the native HTML <input>
	// is visible instead).
	MobileInputID string
	// LogField, when non-empty, causes EventTextCommitted to be emitted on
	// Enter/blur with Field=LogField and Value=the committed text. Leave
	// empty (the default) for domain editors that emit their own event
	// (instrument rename, BPM entry, search query).
	LogField string
	// PadLeft / PadRight reserve horizontal room inside the field for
	// inline adornments (a leading search icon, a trailing clear button)
	// without shrinking the field's visible background. Text, cursor, and
	// the visible-text window all shift/shrink by these amounts.
	PadLeft  int
	PadRight int
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

// effectiveMaxLen returns the rune cap actually enforced: the smaller of the
// field's semantic MaxLen (when set) and the absolute defensive hard cap.
func (t *TextInput) effectiveMaxLen() int {
	if t.MaxLen > 0 && t.MaxLen < textInputHardMaxRunes {
		return t.MaxLen
	}
	return textInputHardMaxRunes
}

// SetText sets the current text and resets the cursor to the end. The value is
// truncated to effectiveMaxLen (so even an unbounded field can't be driven past
// the hard cap); content filtering beyond length is left to the typing path.
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
	// Press edge: this frame the button went down (level → edge). Used to
	// re-assert focus side effects (re-focus the soft-keyboard proxy) when the
	// user re-clicks an ALREADY-focused box — see the focus-transition block.
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
	} else if t.focused && clickedInside {
		// Re-click on an already-focused box: the physical canvas click blurs the
		// hidden soft-keyboard proxy (browser focus moves to the canvas), and with
		// no focus transition OnFocusGained wouldn't otherwise re-fire — the box
		// would silently drop to the lossy ebiten keyboard path (dropped Arrow /
		// Backspace pressed+released between two slow frames). Re-assert focus so
		// the proxy recaptures the keyboard. Gated to the press EDGE because
		// OnFocusGained (softKeyboardShow) clears the proxy's char buffer; firing
		// it every held frame would wipe pending keystrokes.
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

	// Caret navigation is applied BEFORE the character/backspace drain below.
	// When a slow frame batches several key events together, a "move caret then
	// edit" gesture (ArrowLeft… then type; ArrowRight… then Backspace) must land
	// in the right order — editing at the freshly-moved caret, not the stale one.
	// The keyRepeat path covers native (the canvas receives the keydown); the
	// consumeSoftKeyboardArrow* path covers WASM, where the hidden proxy holds
	// keyboard focus and forwards Arrow keys out-of-band. Those return a COUNT
	// (not a flag) so several presses coalesced into one frame each move the caret
	// instead of collapsing to one. keyRepeat is evaluated every frame regardless
	// (it tracks per-key repeat state).
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

	// Home / End jump the caret to the string bounds. Absolute moves, so they
	// are applied after the relative arrow moves; when both fire in one batched
	// frame the later assignment wins (harmless — both are idempotent). Native
	// receives the keydown via keyRepeat; WASM forwards Home/End out-of-band
	// through the proxy (the hidden input holds focus) via consumeSoftKeyboard*.
	// keyRepeat is evaluated unconditionally (never short-circuited) so its
	// per-key repeat state stays maintained every frame — matching the arrows.
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

	// Gather input characters: prefer soft keyboard when active (mobile),
	// fall back to Ebiten's InputChars() (desktop/hardware keyboard).
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
	// Defensive bound: never process more than the hard cap of typed runes in a
	// single frame, so a pathological paste can't spin the insert loop.
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
		t.deleteBackward()
	}
	// Native forward-delete (WASM routes it through the '\x7f' char sentinel above).
	if t.keyRepeat(ebiten.KeyDelete) {
		t.deleteForward()
	}
	return consumed
}

// deleteBackward removes the rune before the caret (Backspace) and moves the
// caret left. No-op at the start of the string.
func (t *TextInput) deleteBackward() {
	if t.cursor <= 0 {
		return
	}
	bi := byteIndex(t.Text, t.cursor)
	prev := byteIndex(t.Text, t.cursor-1)
	t.Text = t.Text[:prev] + t.Text[bi:]
	t.cursor--
}

// deleteForward removes the rune at the caret (Delete). The caret does not move.
// No-op at the end of the string.
func (t *TextInput) deleteForward() {
	if t.cursor >= utf8.RuneCountInString(t.Text) {
		return
	}
	bi := byteIndex(t.Text, t.cursor)
	next := byteIndex(t.Text, t.cursor+1)
	t.Text = t.Text[:bi] + t.Text[next:]
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
	DrawTextAt(dst, txt, t.Rect.Min.X+4+t.PadLeft, ty)
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

// min and max remain for compatibility with other widgets that may override
// drawCursor; keep them exported locally to avoid import cycles.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
