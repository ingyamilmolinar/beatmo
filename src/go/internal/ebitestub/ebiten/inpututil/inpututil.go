//go:build test

// Package inpututil is a stubbed mirror of ebiten's inpututil for tests.
package inpututil

import "github.com/hajimehoshi/ebiten/v2"

// Consulted only on the non-injected path: internal/ui tests inject isKeyJustPressed via stubKeys.
var KeysJustPressed = map[ebiten.Key]bool{}

// IsKeyJustPressed reports whether the key was pressed this frame.
func IsKeyJustPressed(k ebiten.Key) bool { return KeysJustPressed[k] }
