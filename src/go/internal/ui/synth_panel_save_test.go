package ui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Synth-tab Save / Save-As coverage over the instrument library (spec
// 2026-10-04 §6): drives the public SaveActiveRecipe / SaveActiveRecipeAs
// entry points against a private in-memory library and asserts the library
// write, the persisted bytes, the project copy's Origin and the hook event.

// captureRecipeHooks subscribes to recipe lifecycle events and returns a
// thread-safe accessor for the captured payloads. Subscribes per-kind
// because hooks.Subscribe takes a single Kind; the four recipe / kit
// kinds we care about are enumerated explicitly here so the helper
// fails closed if a future Kind is added without coverage.
func captureRecipeHooks(t *testing.T) func() []hooks.Event {
	t.Helper()
	var mu sync.Mutex
	var seen []hooks.Event
	kinds := []hooks.Kind{
		hooks.EventRecipeSaved,
		hooks.EventRecipeCreated,
		hooks.EventRecipeDeleted,
		hooks.EventKitApplied,
	}
	unsubs := make([]func(), 0, len(kinds))
	for _, k := range kinds {
		unsub := hooks.Subscribe(k, func(e hooks.Event) {
			mu.Lock()
			seen = append(seen, e)
			mu.Unlock()
		})
		unsubs = append(unsubs, unsub)
	}
	t.Cleanup(func() {
		for _, u := range unsubs {
			u()
		}
	})
	return func() []hooks.Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]hooks.Event, len(seen))
		copy(out, seen)
		return out
	}
}

// waitForKind polls the events accessor briefly until an event of the
// given kind appears, or the test fails. hooks delivery is async (pool
// fanout); without this poll the test races the worker. 10ms × 100
// iterations = 1s max before failing — generous for the hooks.fanout
// pool's typical sub-millisecond delivery.
func waitForKind(t *testing.T, events func() []hooks.Event, kind hooks.Kind) hooks.Event {
	t.Helper()
	for i := 0; i < 100; i++ {
		for _, e := range events() {
			if e.Kind == kind {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hook %q not delivered within budget", kind)
	return hooks.Event{}
}

// setSynthActiveInstrumentForTest installs a DrumView shim whose
// synthTabActiveInstrument resolves to instID. The Save path's only
// dependency on a real DrumView is that method; nothing else is touched
// by the persistence flow, so a bare struct with one row is sufficient.
func setSynthActiveInstrumentForTest(instID instrument.ID) *DrumView {
	dv := &DrumView{}
	dv.Rows = []*DrumRow{{Instrument: instID}}
	return dv
}

// libraryField reads one flat field of id's library document.
func libraryField(t *testing.T, id, field string) float64 {
	t.Helper()
	d, ok := audio.LibraryDoc(instrument.ID(id))
	if !ok {
		t.Fatalf("no library document %q", id)
	}
	return instrument.FieldValues(d, audio.BuiltinModuleRegistry())[field]
}

// persistedField decodes the bytes the sink received for id and reads field.
func persistedField(t *testing.T, lib *memInstrumentLibrary, id, field string) float64 {
	t.Helper()
	raw := lib.docs[instrument.ID(id)]
	if len(raw) == 0 {
		t.Fatalf("the library sink received nothing for %q", id)
	}
	d, err := instrument.ParseFile(raw)
	if err != nil {
		t.Fatalf("persisted %q does not parse: %v", id, err)
	}
	if d.ID != instrument.ID(id) || d.Origin != "" {
		t.Errorf("persisted doc id=%q origin=%q, want id=%q and no origin", d.ID, d.Origin, id)
	}
	return instrument.FieldValues(d, audio.BuiltinModuleRegistry())[field]
}

// TestSynthSave_PersistsLibraryDocAndPublishesHook: Save writes the project
// copy into the library under its origin, persists it, and publishes
// EventRecipeSaved naming the library id and the instrument.
func TestSynthSave_PersistsLibraryDocAndPublishesHook(t *testing.T) {
	newDocsGame(t, roster.Snare)
	lib := withPrivateInstrumentLibrary(t)
	events := captureRecipeHooks(t)

	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.6)
	if !audio.InstrumentDocModified(roster.Snare) {
		t.Fatal("an edited copy must read modified before Save")
	}

	if got := saveRecipeForInstrument(roster.Snare); got != roster.Snare {
		t.Fatalf("SaveActiveRecipe = %q, want snare", got)
	}
	if v := libraryField(t, "snare", "post_decay"); v != 0.6 {
		t.Errorf("library post_decay = %v, want 0.6", v)
	}
	if v := persistedField(t, lib, "snare", "post_decay"); v != 0.6 {
		t.Errorf("persisted post_decay = %v, want 0.6", v)
	}
	if audio.InstrumentDocModified(roster.Snare) {
		t.Error("after Save the copy equals its library entry (dirty dot must clear)")
	}

	e := waitForKind(t, events, hooks.EventRecipeSaved)
	p, ok := e.Payload.(hooks.RecipePayload)
	if !ok {
		t.Fatalf("EventRecipeSaved payload = %T, want hooks.RecipePayload", e.Payload)
	}
	if p.RecipeID != roster.Snare || p.InstrumentID != roster.Snare {
		t.Errorf("payload = %+v, want recipe=snare inst=snare", p)
	}
}

// TestSynthSaveAs_CreatesUserInstrumentAndRepointsOrigin: Save As writes a new
// content-addressed user instrument, persists it, keeps the row's id, repoints
// the project copy at it, and publishes EventRecipeCreated.
func TestSynthSaveAs_CreatesUserInstrumentAndRepointsOrigin(t *testing.T) {
	newDocsGame(t, roster.Snare)
	lib := withPrivateInstrumentLibrary(t)
	events := captureRecipeHooks(t)

	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.4)
	newID := setSynthActiveInstrumentForTest(roster.Snare).SaveActiveRecipeAs("")
	if !strings.HasPrefix(string(newID), "user.") {
		t.Fatalf("SaveActiveRecipeAs = %q, want a user.* id", newID)
	}
	if v := libraryField(t, string(newID), "post_decay"); v != 0.4 {
		t.Errorf("new library entry post_decay = %v, want 0.4", v)
	}
	if v := persistedField(t, lib, string(newID), "post_decay"); v != 0.4 {
		t.Errorf("persisted post_decay = %v, want 0.4", v)
	}
	if v := libraryField(t, "snare", "post_decay"); v == 0.4 {
		t.Error("Save As must not change the shipped instrument's library entry")
	}
	d, _ := audio.InstrumentDoc(roster.Snare)
	if d.Origin != newID || audio.InstrumentDocModified(roster.Snare) {
		t.Errorf("project copy origin=%q modified=%v, want origin=%q and clean", d.Origin, audio.InstrumentDocModified(roster.Snare), newID)
	}
	if _, ok := audio.InstrumentDoc(newID); !ok {
		t.Errorf("the new instrument %q is not pickable", newID)
	}

	e := waitForKind(t, events, hooks.EventRecipeCreated)
	p, ok := e.Payload.(hooks.RecipePayload)
	if !ok {
		t.Fatalf("EventRecipeCreated payload = %T", e.Payload)
	}
	if p.RecipeID != newID || p.BaseRecipe != roster.Snare || p.InstrumentID != roster.Snare {
		t.Errorf("payload = %+v, want recipe=%q base=snare inst=snare", p, newID)
	}
}

// TestSynthSaveAs_PersistedDocReloadsCleanly: the bytes the sink received load
// back through LoadUserInstrumentLibrary (the boot path) with the saved value —
// the persisted format cannot drift from what the loader accepts.
func TestSynthSaveAs_PersistedDocReloadsCleanly(t *testing.T) {
	newDocsGame(t, roster.Kick)
	lib := withPrivateInstrumentLibrary(t)
	audio.SetInstrumentParam(roster.Kick, "post_decay", 0.35)
	newID := setSynthActiveInstrumentForTest(roster.Kick).SaveActiveRecipeAs("Deep One")
	if newID == "" {
		t.Fatal("SaveActiveRecipeAs returned empty id")
	}

	// Simulate a restart: an empty library, then a reload from the bytes alone.
	if err := audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := audio.LibraryDoc(newID); ok {
		t.Fatal("the in-memory library still holds the entry after a reset")
	}
	if err := audio.LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	if v := libraryField(t, string(newID), "post_decay"); v != 0.35 {
		t.Errorf("reloaded post_decay = %v, want 0.35 (value lost across the round trip)", v)
	}
	if d, _ := audio.LibraryDoc(newID); d.Name != "Deep One" {
		t.Errorf("reloaded name = %q, want Deep One", d.Name)
	}
}

// TestSynthSave_NoSinkIsNoOp: with no sink, Save still updates the in-memory
// library and publishes the hook — persistence alone silently no-ops.
func TestSynthSave_NoSinkIsNoOp(t *testing.T) {
	newDocsGame(t, roster.Snare)
	audio.SetInstrumentLibrarySink(nil)
	events := captureRecipeHooks(t)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.55)
	if got := saveRecipeForInstrument(roster.Snare); got != roster.Snare {
		t.Fatalf("SaveActiveRecipe = %q", got)
	}
	if v := libraryField(t, "snare", "post_decay"); v != 0.55 {
		t.Errorf("library not updated when sink absent: %v", v)
	}
	_ = waitForKind(t, events, hooks.EventRecipeSaved)
}

// TestHooks_NumNonVerboseUpdated guards the constant in hooks/events.go.
// If Phase 4 forgot to bump it, KindAll[:NumNonVerbose] would silently
// truncate the four new recipe / kit kinds and the eventlogger coverage
// path would emit them anyway — confusing log output.
func TestHooks_NumNonVerboseUpdated(t *testing.T) {
	// Derive the expected count from the source of truth (KindAll + IsVerbose)
	// rather than hardcoding a magic number, so adding/removing a Kind doesn't
	// require editing this test. The invariant the constant must satisfy:
	// NumNonVerbose equals the number of non-verbose kinds, AND every verbose
	// kind sits at the tail — i.e. KindAll[:NumNonVerbose] contains zero verbose
	// kinds and KindAll[NumNonVerbose:] contains only verbose kinds. Otherwise
	// KindAll[:NumNonVerbose] would mis-slice and the eventlogger coverage path
	// would emit (or drop) the wrong kinds.
	wantCount := 0
	for _, k := range hooks.KindAll {
		if !hooks.IsVerbose(k) {
			wantCount++
		}
	}
	if hooks.NumNonVerbose != wantCount {
		t.Errorf("hooks.NumNonVerbose = %d, want %d (count of non-verbose kinds in KindAll)", hooks.NumNonVerbose, wantCount)
	}
	for i, k := range hooks.KindAll {
		if i < hooks.NumNonVerbose && hooks.IsVerbose(k) {
			t.Errorf("verbose kind %q at index %d is inside the non-verbose prefix KindAll[:%d]", k, i, hooks.NumNonVerbose)
		}
		if i >= hooks.NumNonVerbose && !hooks.IsVerbose(k) {
			t.Errorf("non-verbose kind %q at index %d is outside the non-verbose prefix KindAll[%d:]", k, i, hooks.NumNonVerbose)
		}
	}
	// Sanity check: the recipe/kit + sampler kinds are present in KindAll and
	// within the non-verbose prefix.
	want := map[hooks.Kind]bool{
		hooks.EventRecipeSaved:       true,
		hooks.EventRecipeCreated:     true,
		hooks.EventRecipeDeleted:     true,
		hooks.EventKitApplied:        true,
		hooks.EventSampleSaved:       true,
		hooks.EventSampleCreated:     true,
		hooks.EventSampleReset:       true,
		hooks.EventSampleEditChanged: true,
	}
	found := 0
	for _, k := range hooks.KindAll[:hooks.NumNonVerbose] {
		if want[k] {
			found++
		}
	}
	if found != 8 {
		t.Errorf("recipe + kit + sample kinds in non-verbose prefix = %d, want 8", found)
	}
}
