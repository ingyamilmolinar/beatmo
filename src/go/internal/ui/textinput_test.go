package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestFitWidthGrowsToValueWithMin(t *testing.T) {
	if got := FitWidth("", 80); got != 80 {
		t.Fatalf("empty got %d want min 80", got)
	}
	long := "A very long instrument name"
	if got := FitWidth(long, 80); got < TextWidth(long) {
		t.Fatalf("got %d clips value (TextWidth=%d)", got, TextWidth(long))
	}
}

func TestTextInputEditing(t *testing.T) {
	assertDefaultParityState(t)
	prevSuppress := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prevSuppress })
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	restore := SetInputForTest(
		func() (int, int) { return 5, 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
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
	t.Cleanup(restore)
	ti.Update()
	restore()
	if ti.Text != "abc" {
		t.Fatalf("got %q", ti.Text)
	}

	// move left and backspace
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	ti.Update()
	restore()

	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyBackspace },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	ti.Update()
	restore()
	if ti.Text != "ac" {
		t.Fatalf("expected ac, got %q", ti.Text)
	}
	if ti.cursor != 1 {
		t.Fatalf("cursor=%d", ti.cursor)
	}
}

func TestTextInputEnterDefocuses(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	mx, my := 1, 1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	defer restore()

	ti.Update() // focus
	pressed = false

	chars = []rune{'1', '2', '3', '\r'}
	ti.Update()

	if ti.Focused() {
		t.Fatalf("expected input to lose focus on enter")
	}
	if ti.Value() != "123" {
		t.Fatalf("expected value '123' got %q", ti.Value())
	}
}

func TestTextInputHighlightAndCursor(t *testing.T) {
	assertDefaultParityState(t)
	style := TextInputStyle{Fill: color.RGBA{10, 20, 30, 255}, Border: color.Black, Cursor: color.White}
	ti := NewTextInput(image.Rect(0, 0, 80, 20), style)
	restore := SetInputForTest(
		func() (int, int) { return 5, 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	ti.Update() // focus
	restore()

	var got color.RGBA
	var cursor bool
	var cursorCol color.RGBA
	oldBtn := drawButton
	oldCur := drawCursor
	drawButton = func(dst *ebiten.Image, r image.Rectangle, f, b color.Color, pressed, topEdgeHighlight bool) {
		got = color.RGBAModel.Convert(f).(color.RGBA)
	}
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) {
		cursor = true
		cursorCol = color.RGBAModel.Convert(c).(color.RGBA)
	}
	defer func() { drawButton = oldBtn; drawCursor = oldCur }()

	ti.Draw(ebiten.NewImage(80, 20))

	if !cursor {
		t.Fatalf("cursor not drawn")
	}
	orig := color.RGBAModel.Convert(style.Fill).(color.RGBA)
	if got == orig {
		t.Fatalf("fill color not adjusted on focus")
	}
	if cursorCol != color.RGBAModel.Convert(style.Cursor).(color.RGBA) {
		t.Fatalf("cursor color=%v want %v", cursorCol, style.Cursor)
	}
}

func TestTextInputBackspaceHold(t *testing.T) {
	assertDefaultParityState(t)
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
	t.Cleanup(restore)
	ti.Update()
	if ti.Text != "abc" {
		t.Fatalf("expected single deletion, got %q", ti.Text)
	}
	for i := 0; i < 65; i++ {
		ti.Update()
	}
	if len(ti.Text) >= 3 {
		t.Fatalf("expected repeat deletion after delay, got %q", ti.Text)
	}
	restore()
}

func TestTextInputCursorBlinks(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	ti.focused = true

	var drawn bool
	oldCur := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) {
		drawn = true
	}
	defer func() { drawCursor = oldCur }()

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
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	ti.SetText("abcd")
	// Click just past the second character — use TextWidth for font-independent positioning.
	clickX := ti.Rect.Min.X + 4 + TextWidth("ab") + 1
	restore := SetInputForTest(
		func() (int, int) { return clickX, ti.Rect.Min.Y + 5 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	ti.Update()
	restore()
	if ti.cursor != 2 {
		t.Fatalf("cursor=%d", ti.cursor)
	}
}

func TestTextInputCursorPosition(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 40, 20), BPMBoxStyle)
	ti.SetText("abcdef")
	ti.cursor = 4
	var cur image.Rectangle
	old := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) {
		cur = r
	}
	defer func() { drawCursor = old }()
	ti.focused = true
	ti.Draw(ebiten.NewImage(40, 20))
	// Compute expected cursor X the same way Draw() does: measure the visible
	// prefix before the cursor using TextWidth (works for both debug and TrueType).
	_, start := ti.visibleText()
	rs := []rune(ti.Text)
	bi := byteIndex(ti.Text, start)
	ci := byteIndex(ti.Text, min(ti.cursor, len(rs)))
	prefix := ti.Text[bi:ci]
	wantX := ti.Rect.Min.X + 4 + TextWidth(prefix)
	if cur.Min.X != wantX {
		t.Fatalf("cursor x=%d want %d", cur.Min.X, wantX)
	}
}

// cursor alignment regression test: ensure caret sits exactly after the
// preceding glyphs with no extra spacing.
func TestTextInputCursorAlignment(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	ti.focused = true
	ti.SetText("abcd")
	ti.cursor = 2 // between b and c

	var cur image.Rectangle
	old := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) { cur = r }
	defer func() { drawCursor = old }()

	ti.Draw(ebiten.NewImage(80, 20))

	// Compute expected cursor X using TextWidth for the visible prefix.
	_, start := ti.visibleText()
	bi := byteIndex(ti.Text, start)
	ci := byteIndex(ti.Text, min(ti.cursor, len([]rune(ti.Text))))
	prefix := ti.Text[bi:ci]
	want := ti.Rect.Min.X + 4 + TextWidth(prefix)
	if cur.Min.X != want {
		t.Fatalf("cursor x=%d want %d", cur.Min.X, want)
	}
}

func TestTextInputOverflow(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 40, 20), BPMBoxStyle)
	ti.SetText("abcdefghij")
	vis, start := ti.visibleText()
	total := utf8.RuneCountInString(ti.Text)
	visLen := utf8.RuneCountInString(vis)
	// Visible text must fit within the box.
	pad := 4
	maxW := ti.Rect.Dx() - pad*2
	if TextWidth(vis) > maxW {
		t.Fatalf("visible text too wide: %q (%dpx > %dpx)", vis, TextWidth(vis), maxW)
	}
	// Cursor is at end (SetText moves it there), so start+visLen must equal total.
	if start+visLen != total {
		t.Fatalf("expected start+visLen==total, got %d+%d!=%d", start, visLen, total)
	}
	// Must actually overflow (not all chars visible).
	if visLen >= total {
		t.Fatalf("expected overflow, all %d chars visible", total)
	}
}

func TestTextInputVisibleTextStart(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 40, 20), BPMBoxStyle)
	ti.SetText("abcdefghij")
	ti.cursor = 0
	vis, start := ti.visibleText()
	// With cursor at 0, start must be 0.
	if start != 0 {
		t.Fatalf("start=%d want 0", start)
	}
	// Visible text must fit within the box.
	pad := 4
	maxW := ti.Rect.Dx() - pad*2
	if TextWidth(vis) > maxW {
		t.Fatalf("visible text too wide: %q (%dpx > %dpx)", vis, TextWidth(vis), maxW)
	}
	// Visible text must be a prefix of the full text.
	if len(vis) == 0 || !strings.HasPrefix(ti.Text, vis) {
		t.Fatalf("vis=%q is not a prefix of %q", vis, ti.Text)
	}
}

func TestTextInputMidCursorWindow(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 40, 20), BPMBoxStyle)
	ti.SetText("abcdefghij")
	ti.cursor = 5
	vis, start := ti.visibleText()
	visLen := utf8.RuneCountInString(vis)
	// Cursor must be within visible window.
	if start > ti.cursor {
		t.Fatalf("start=%d > cursor=%d", start, ti.cursor)
	}
	if ti.cursor > start+visLen {
		t.Fatalf("cursor=%d not visible (start=%d visLen=%d)", ti.cursor, start, visLen)
	}
	var cur image.Rectangle
	old := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, c color.Color) {
		cur = r
	}
	defer func() { drawCursor = old }()
	ti.focused = true
	ti.Draw(ebiten.NewImage(40, 20))
	// Compute expected cursor X using TextWidth for the visible prefix.
	rs := []rune(ti.Text)
	bi := byteIndex(ti.Text, start)
	ci := byteIndex(ti.Text, min(ti.cursor, len(rs)))
	prefix := ti.Text[bi:ci]
	wantX := ti.Rect.Min.X + 4 + TextWidth(prefix)
	if cur.Min.X != wantX {
		t.Fatalf("cursor x=%d want %d", cur.Min.X, wantX)
	}
}

func TestTextInputDrawAnimatedPreservesBounds(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 100, 24), BPMBoxStyle)
	ti.focused = true
	ti.anim = 1
	var got image.Rectangle
	old := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, f, b color.Color, pressed, topEdgeHighlight bool) {
		got = r
	}
	defer func() { drawButton = old }()
	ti.Draw(ebiten.NewImage(100, 24))
	if got.Dy() != 20 || got.Dx() != 96 {
		t.Fatalf("animRect=%v", got)
	}
}

func TestTextInputRightArrowMovesCursor(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	ti.focused = true
	ti.SetText("abc")
	ti.cursor = 0
	restore := SetInputForTest(
		func() (int, int) { return -1, -1 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyRight },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	ti.Update()
	if ti.cursor != 1 {
		t.Fatalf("after Right cursor=%d want 1", ti.cursor)
	}
}
