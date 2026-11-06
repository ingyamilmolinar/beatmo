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
	// `?devtools=0` previews the production UI in a developer WASM (a release
	// WASM has the developer surfaces off already). Read before ui.New builds
	// the tab strip and the bottom nav from the available-tab lists.
	if loc := js.Global().Get("location"); loc.Truthy() {
		if q := loc.Get("search"); q.Truthy() {
			if params := js.Global().Get("URLSearchParams"); params.Truthy() {
				if params.New(q).Call("get", "devtools").String() == "0" {
					SetDevToolsForcedOff(true)
				}
			}
		}
	}
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

	// The user's instrument LIBRARY (saved instrument documents, spec
	// 2026-10-04 §6) lives in its OWN localStorage key, separate from
	// beatmo.prefs. Loaded before the first project ResetInstruments so saved
	// instruments are playable and listed (and pushed to audio.js by
	// initJSSynthRecipe's PushAllInstrumentDocsToPlatform).
	instStore := userprefs.NewInstrumentStore(userprefs.Options{})
	audio.SetInstrumentLibrarySink(instStore)
	// Sampler edits saved before versioning join their instrument's first
	// version (read once, never written again).
	oldEdits, _ := prefsStore.(audio.SampleEditSource)
	if err := audio.LoadUserInstrumentLibrary(instStore, oldEdits); err != nil {
		println("WARN [instruments]", err.Error())
	}
	audio.ResetInstruments()

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

	// Knob step-rung persistence: remember each param's chosen step badge
	// rung across reloads. Keyed by param name (global, not per-instrument).
	if ap, ok := prefsStore.(userprefs.AudioPanelStore); ok {
		SetAudioPanelStateSink(ap)
	}
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
