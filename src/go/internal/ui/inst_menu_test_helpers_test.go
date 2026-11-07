//go:build test

package ui

import (
	"strconv"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Stable, sortable labels (Item 000, Item 001, …).
func itoaPad(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

type keyState struct {
	pressed map[ebiten.Key]bool
}

func newKeyState() *keyState { return &keyState{pressed: map[ebiten.Key]bool{}} }

func (k *keyState) press(key ebiten.Key) { k.pressed[key] = true }

func (k *keyState) release(key ebiten.Key) { delete(k.pressed, key) }

func (k *keyState) isKeyPressed(key ebiten.Key) bool { return k.pressed[key] }

func installInputStub(t *testing.T) *keyState {
	t.Helper()
	ks := newKeyState()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		ks.isKeyPressed,
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1024, 768 },
	)
	t.Cleanup(restore)
	return ks
}
