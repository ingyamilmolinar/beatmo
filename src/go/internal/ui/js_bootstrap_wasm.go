//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

	// Notification history: persist the in-band notification log across
	// sessions so the history popup survives a reload.
	if ns, ok := prefsStore.(userprefs.NotificationHistoryStore); ok {
		SetNotificationHistoryStore(NewPersistedNotificationHistory(ns))
	}

	// Phase 3: apply user-saved recipe overrides + register user recipes
	// from localStorage. The parity gate is flipped on here (production
	// bootstrap only) so test binaries leave shipped defaults intact.
	// The store satisfies audio.UserRecipeSource via the RecipeStore
	// subset; nil-source / decode failures inside Apply are non-fatal.
	if rs, ok := prefsStore.(userprefs.RecipeStore); ok {
		audio.SetUseUserRecipeOverrides(true)
		_ = audio.ApplyUserRecipeOverrides(rs)
		// Phase 4: Synth-tab Save / Save-As writes back through the
		// same store. RecipeStore satisfies ui.RecipeSaveSink.
		SetRecipeSink(rs)
	}

	// Sampler tab: cross-session user-sample persistence (IndexedDB on
	// browser). Mirrors the recipe wiring above. ApplySavedSamples re-registers
	// every persisted sample for playback before the first Layout reads the
	// instrument picker.
	if ss, ok := prefsStore.(userprefs.SampleStore); ok {
		adapter := NewSamplePersistAdapter(ss)
		audio.SetSampleSink(adapter)
		audio.SetUseSampleStore(true)
		audio.ApplySavedSamples(adapter)
	}

	// Non-destructive sample-edit descriptors: rehydration runs through
	// audio.SetSampleEdit, which fires platformSampleEditChanged → JS
	// updateSampleEdit, so the browser render cache reflects the saved edits
	// from the first play.
	if se, ok := prefsStore.(userprefs.SampleEditStore); ok {
		SetSampleEditSink(se)
		audio.ApplySavedSampleEdits(se)
	}

	// Knob step-rung persistence: remember each param's chosen step badge
	// rung across reloads. Keyed by param name (global, not per-instrument).
	if ks, ok := prefsStore.(userprefs.KnobStepStore); ok {
		SetKnobStepSink(ks)
	}

	// UI language: apply the persisted locale before the first Layout so the
	// first frame renders in the user's chosen language.
	if ls, ok := prefsStore.(userprefs.LanguageStore); ok {
		SetLanguageSink(ls)
		ApplyStoredLanguage()
	}
}
