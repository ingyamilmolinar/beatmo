//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// Minimal hooks before the Game exists, so browser tests can start; initJS replaces them.
func init() {
	// Read before ui.New builds the tab strip and bottom nav.
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

	if ns, ok := prefsStore.(userprefs.NotificationHistoryStore); ok {
		SetNotificationHistoryStore(NewPersistedNotificationHistory(ns))
	}

	// Its own localStorage key, separate from prefs; loaded before the first project so saved instruments play.
	instStore := userprefs.NewInstrumentStore(userprefs.Options{})
	audio.SetInstrumentLibrarySink(instStore)
	// Pre-versioning Sampler edits join their instrument's first version (read once, never written).
	oldEdits, _ := prefsStore.(audio.SampleEditSource)
	if err := audio.LoadUserInstrumentLibrary(instStore, oldEdits); err != nil {
		println("WARN [instruments]", err.Error())
	}
	audio.ResetInstruments()

	// Saved samples re-register before the first Layout reads the instrument picker.
	if ss, ok := prefsStore.(userprefs.SampleStore); ok {
		adapter := NewSamplePersistAdapter(ss)
		audio.SetSampleSink(adapter)
		audio.SetUseSampleStore(true)
		audio.ApplySavedSamples(adapter)
	}

	// Keyed by param name, global rather than per instrument.
	if ap, ok := prefsStore.(userprefs.AudioPanelStore); ok {
		SetAudioPanelStateSink(ap)
	}
	if ks, ok := prefsStore.(userprefs.KnobStepStore); ok {
		SetKnobStepSink(ks)
	}

	// Before the first Layout, so the first frame is in the chosen language.
	if ls, ok := prefsStore.(userprefs.LanguageStore); ok {
		SetLanguageSink(ls)
		ApplyStoredLanguage()
	}
}
