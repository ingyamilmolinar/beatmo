package ui

import (
	"image"
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
