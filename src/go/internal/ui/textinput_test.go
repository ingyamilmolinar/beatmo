package ui

import (
	"image"
	"image/color"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTextInputEditing(t *testing.T) {
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	restore := SetInputForTest(
		func() (int, int) { return 5, 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update() // focus
	restore()

	// type abc
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return []rune("abc") },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update()
	restore()
	if ti.Text != "abc" {
		t.Fatalf("got %q", ti.Text)
	}

	// move left and backspace
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool {
			if k == ebiten.KeyLeft {
				return true
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update()
	restore()

	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool {
			if k == ebiten.KeyBackspace {
				return true
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update()
	restore()
	if ti.Text != "ac" {
		t.Fatalf("expected ac, got %q", ti.Text)
	}
	if ti.cursor != 1 {
		t.Fatalf("cursor=%d", ti.cursor)
	}
}

func TestTextInputHighlightAndCursor(t *testing.T) {
	style := TextInputStyle{Fill: color.RGBA{10, 20, 30, 255}, Border: color.Black}
	ti := NewTextInput(image.Rect(0, 0, 80, 20), style)
	restore := SetInputForTest(
		func() (int, int) { return 5, 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update() // focus
	restore()

	var got color.RGBA
	var cursor bool
	oldBtn := drawButton
	oldLine := drawLine
	drawButton = func(dst *ebiten.Image, r image.Rectangle, f, b color.Color, pressed bool) {
		got = color.RGBAModel.Convert(f).(color.RGBA)
	}
	drawLine = func(dst *ebiten.Image, x1, y1, x2, y2 int, c color.Color) {
		cursor = true
	}
	defer func() { drawButton = oldBtn; drawLine = oldLine }()

	ti.Draw(ebiten.NewImage(80, 20))

	if !cursor {
		t.Fatalf("cursor not drawn")
	}
	orig := color.RGBAModel.Convert(style.Fill).(color.RGBA)
	if got == orig {
		t.Fatalf("fill color not adjusted on focus")
	}
}

func TestTextInputBackspaceHold(t *testing.T) {
	ti := NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	ti.focused = true
	ti.SetText("abcd")
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyBackspace },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	for i := 0; i < 5; i++ {
		ti.Update()
	}
	if ti.Text != "abc" {
		t.Fatalf("expected single deletion, got %q", ti.Text)
	}
	for i := 0; i < 13; i++ {
		ti.Update()
	}
	if ti.Text != "ab" {
		t.Fatalf("expected repeat deletion after delay, got %q", ti.Text)
	}
	restore()
}

func TestTextInputCursorBlinks(t *testing.T) {
	ti := NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	ti.focused = true

	var drawn bool
	oldLine := drawLine
	drawLine = func(dst *ebiten.Image, x1, y1, x2, y2 int, c color.Color) {
		drawn = true
	}
	defer func() { drawLine = oldLine }()

	ti.blink = 10
	ti.Draw(ebiten.NewImage(80, 20))
	if !drawn {
		t.Fatalf("expected cursor visible")
	}

	drawn = false
	ti.blink = 40
	ti.Draw(ebiten.NewImage(80, 20))
	if drawn {
		t.Fatalf("cursor should be hidden while blink >= 30")
	}
}

func TestTextInputClickMovesCursor(t *testing.T) {
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	ti.SetText("abcd")
	restore := SetInputForTest(
		func() (int, int) { return ti.Rect.Min.X + 4 + debugCharW*2 + 1, ti.Rect.Min.Y + 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	ti.Update()
	restore()
	if ti.cursor != 2 {
		t.Fatalf("cursor=%d", ti.cursor)
	}
}

func TestTextInputOverflow(t *testing.T) {
	ti := NewTextInput(image.Rect(0, 0, 40, 20), BPMBoxStyle)
	ti.SetText("abcdefghij")
	vis, start := ti.visibleText()
	if utf8.RuneCountInString(vis) > 4 {
		t.Fatalf("visible too long: %q", vis)
	}
	if start != 6 {
		t.Fatalf("start=%d", start)
	}
}
