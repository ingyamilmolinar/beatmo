package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Digits-only filter for tests.
func onlyDigits(r rune) bool { return r >= '0' && r <= '9' }

func TestTextInputAcceptAndMaxLen(t *testing.T) {
	assertDefaultParityState(t)
	ti := NewTextInput(image.Rect(0, 0, 100, 20), BPMBoxStyle)
	ti.focused = true
	ti.Accept = onlyDigits
	ti.MaxLen = 4
	// Simulate user typing "12a3!45"; only digits should be accepted and max 4 runes kept.
	chars := []rune{'1', '2'}
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 80, 20 },
	)
	defer restore()
	_ = ti.Update()
	// feed 'a','3'
	chars = []rune{'a', '3'}
	_ = ti.Update()
	// feed '!','4','5'
	chars = []rune{'!', '4', '5'}
	_ = ti.Update()
	if got := ti.Value(); got != "1234" {
		t.Fatalf("unexpected value: %q want %q", got, "1234")
	}
}
