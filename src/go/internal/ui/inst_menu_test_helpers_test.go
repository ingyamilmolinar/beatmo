//go:build test

package ui

import (
	"strconv"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// itoaPad returns a 3-digit zero-padded decimal representation of i.
// Used by inst-menu pagination tests to generate stable, sortable
// instrument labels (Item 000, Item 001, …).
func itoaPad(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// keyState is a tiny test stub for ebiten.IsKeyPressed. Tests press a
// key, call comp.Update() (rising-edge fires), release, then call
// Update again (edge resets). pressKey wraps that two-frame cycle.
type keyState struct {
	pressed map[ebiten.Key]bool
}

func newKeyState() *keyState { return &keyState{pressed: map[ebiten.Key]bool{}} }

func (k *keyState) press(key ebiten.Key) { k.pressed[key] = true }

func (k *keyState) release(key ebiten.Key) { delete(k.pressed, key) }

func (k *keyState) isKeyPressed(key ebiten.Key) bool { return k.pressed[key] }

// installInputStub registers a keyState as the active input source
// for the duration of the test. Returns the keyState so tests can
// drive press/release directly. Restoration is deferred via t.Cleanup.
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
