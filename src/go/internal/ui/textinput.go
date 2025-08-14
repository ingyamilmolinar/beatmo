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
		if d == 1 || d > 15 && (d-15)%3 == 0 {
			return true
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
		start = total - maxRunes
	}
	bi := byteIndex(t.Text, start)
	return t.Text[bi:], start
}

// Draw renders the input.
func (t *TextInput) Draw(dst *ebiten.Image) {
	t.Style.DrawAnimated(dst, t.Rect, t.focused, t.anim)
	txt, start := t.visibleText()
	ebitenutil.DebugPrintAt(dst, txt, t.Rect.Min.X+4, t.Rect.Min.Y+4)
	if t.focused && t.blink < 30 {
		cx := t.Rect.Min.X + 4 + debugCharW*(t.cursor-start)
		cy := t.Rect.Min.Y + 4
		drawLine(dst, cx, cy, cx, cy+debugCharH-2, colorWhite)
	}
}

var drawLine = func(dst *ebiten.Image, x1, y1, x2, y2 int, col color.Color) {
	rect := image.Rect(min(x1, x2), min(y1, y2), max(x1, x2)+1, max(y1, y2)+1)
	drawRect(dst, rect, col, true)
}

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
