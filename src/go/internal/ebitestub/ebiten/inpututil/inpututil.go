//go:build test

// Package inpututil is a stubbed mirror of ebiten's inpututil for tests.
package inpututil

import "github.com/hajimehoshi/ebiten/v2"

// KeysJustPressed is the test-controllable backing map for IsKeyJustPressed.
// Tests in internal/ui inject their own isKeyJustPressed via stubKeys, so this
// map is only consulted when the real (non-injected) function path is used.
var KeysJustPressed = map[ebiten.Key]bool{}

// IsKeyJustPressed reports whether the key was pressed this frame.
func IsKeyJustPressed(k ebiten.Key) bool { return KeysJustPressed[k] }
