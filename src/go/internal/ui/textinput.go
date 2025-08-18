package ui

import (
	"image"
	"image/color"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

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
}

// NewTextInput constructs a text input with the given rectangle and style.
func NewTextInput(r image.Rectangle, style TextInputStyle) *TextInput {
	return &TextInput{Rect: r, Style: style, repeat: make(map[ebiten.Key]int)}
}

// Focused reports whether the input currently has focus.
func (t *TextInput) Focused() bool { return t.focused }

// SetText sets the current text and resets the cursor to the end.
func (t *TextInput) SetText(s string) {
	t.Text = s
	t.cursor = utf8.RuneCountInString(s)
}

// Value returns the current text value.
func (t *TextInput) Value() string { return t.Text }

// Update processes mouse/keyboard input.
func (t *TextInput) Update() bool {
	mx, my := cursorPosition()
	consumed := false
	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		if image.Pt(mx, my).In(t.Rect) {
			t.focused = true
			t.anim = 1
			txt, start := t.visibleText()
			rel := mx - (t.Rect.Min.X + 4)
			idx := rel/debugCharW + start
			if idx < 0 {
				idx = 0
			}
			if idx > utf8.RuneCountInString(t.Text) {
				idx = utf8.RuneCountInString(t.Text)
			}
			t.cursor = idx
			_ = txt
			consumed = true
		} else {
			t.focused = false
		}
	}

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

	if chars := inputChars(); len(chars) > 0 {
		for _, r := range chars {
			if r == '\n' || r == '\r' {
				continue
			}
			before := t.Text[:byteIndex(t.Text, t.cursor)]
			after := t.Text[byteIndex(t.Text, t.cursor):]
			t.Text = before + string(r) + after
			t.cursor++
		}
	}

	if t.keyRepeat(ebiten.KeyBackspace) {
		if t.cursor > 0 {
			bi := byteIndex(t.Text, t.cursor)
			prev := byteIndex(t.Text, t.cursor-1)
			t.Text = t.Text[:prev] + t.Text[bi:]
			t.cursor--
		}
	}
	if t.keyRepeat(ebiten.KeyLeft) {
		if t.cursor > 0 {
			t.cursor--
		}
	}
	if t.keyRepeat(ebiten.KeyRight) {
		if t.cursor < utf8.RuneCountInString(t.Text) {
			t.cursor++
		}
	}
	return consumed
}

func (t *TextInput) keyRepeat(k ebiten.Key) bool {
	if isKeyPressed(k) {
		t.repeat[k]++
		d := t.repeat[k]
		if d == 1 {
			return true
		}
		if d > 60 {
			step := d - 60
			accel := step / 30
			if accel > 5 {
				accel = 5
			}
			interval := 6 - accel
			if step%interval == 0 {
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
	maxRunes := (t.Rect.Dx() - pad*2) / debugCharW
	total := utf8.RuneCountInString(t.Text)
	start := 0
	if total > maxRunes {
		switch {
		case t.cursor <= maxRunes:
			start = 0
		case t.cursor >= total-maxRunes:
			start = total - maxRunes
		default:
			start = t.cursor - maxRunes + 1
			if start < 0 {
				start = 0
			}
		}
	}
	bi := byteIndex(t.Text, start)
	end := byteIndex(t.Text, min(start+maxRunes, total))
	return t.Text[bi:end], start
}

// Draw renders the input.
func (t *TextInput) Draw(dst *ebiten.Image) {
	t.Style.DrawAnimated(dst, t.Rect, t.focused, t.anim)
	txt, start := t.visibleText()
	ebitenutil.DebugPrintAt(dst, txt, t.Rect.Min.X+4, t.Rect.Min.Y+4)
	if t.focused && t.blink < 30 {
		cx := t.Rect.Min.X + 4 + debugCharW*(t.cursor-start)
		cy := t.Rect.Min.Y + 4
		col := t.Style.Cursor
		if col == nil {
			if t.Style.Border != nil {
				col = t.Style.Border
			} else {
				col = colorWhite
			}
		}
		r := image.Rect(cx, cy, cx+debugCharW, cy+debugCharH)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var colorWhite = color.White
