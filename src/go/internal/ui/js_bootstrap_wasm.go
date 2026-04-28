//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// Provide minimal JS hooks early so browser tests can proceed even before the
// Game instance is fully constructed. Game.initJS will override these with the
// real implementations once New() runs.
func init() {
	if !js.Global().Get("startPlay").Truthy() {
		js.Global().Set("startPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return nil }))
	}
	if !js.Global().Get("currentBeat").Truthy() {
		js.Global().Set("currentBeat", js.FuncOf(func(this js.Value, args []js.Value) interface{} { return js.ValueOf(0) }))
	}
	// Wire localStorage-backed favorites. Failures degrade to in-memory.
	prefsStore := userprefs.NewBackingStore(userprefs.Options{})
	SetFavoritesStore(NewPersistedFavoritesStore(prefsStore))
}
