package ui

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Phase 4 Synth-tab Save / Save-As coverage. Drives the public
// SaveActiveRecipe / SaveActiveRecipeAs entry points through a stub sink
// and asserts: persistence write fired, registry mutated as expected,
// hook event published, NumNonVerbose count not regressed.

// stubRecipeSink captures every Save call for assertion. Implements
// ui.RecipeSaveSink.
type stubRecipeSink struct {
	mu        sync.Mutex
	overrides map[string]map[string]float64
	recipes   map[string][]byte
}

func newStubRecipeSink() *stubRecipeSink {
	return &stubRecipeSink{
		overrides: map[string]map[string]float64{},
		recipes:   map[string][]byte{},
	}
}

func (s *stubRecipeSink) SaveRecipeOverride(recipeID string, params map[string]float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[string]float64, len(params))
	for k, v := range params {
		cp[k] = v
	}
	s.overrides[recipeID] = cp
	return nil
}

func (s *stubRecipeSink) SaveUserRecipe(recipeID string, doc []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(doc))
	copy(cp, doc)
	s.recipes[recipeID] = cp
	return nil
}

func (s *stubRecipeSink) override(recipeID string) map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overrides[recipeID]
}

func (s *stubRecipeSink) recipe(recipeID string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recipes[recipeID]
}

// withStubSink swaps the global recipe sink to a fresh stub and restores
// the prior sink on cleanup.
func withStubSink(t *testing.T) *stubRecipeSink {
	t.Helper()
	stub := newStubRecipeSink()
	prev := activeRecipeSink()
	SetRecipeSink(stub)
	t.Cleanup(func() { SetRecipeSink(prev) })
	return stub
}

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

// captureBoundInstrument finds an instrument id bound to recipeID in the
// shipped binding table. Used to set up the test by simulating "the user
// is editing this row's instrument". Returns "" if no binding exists
// (test bug if this happens — drum-snare always has bindings).
func captureBoundInstrument(recipeID string) string {
	regs := audio.RecipeRegistrations()
	if regs[recipeID] == nil {
		return ""
	}
	// Find any instrument id that resolves to recipeID via RecipeForInstrument.
	// Iterate well-known seed ids first to keep the test reproducible.
	for _, candidate := range []string{"snare", "kick", "hihat", "fm-bass"} {
		if audio.RecipeForInstrument(candidate) == recipeID {
			return candidate
		}
	}
	return ""
}

// setSynthActiveInstrumentForTest installs a DrumView shim whose
// synthTabActiveInstrument resolves to instID. The Save path's only
// dependency on a real DrumView is that method; nothing else is touched
// by the persistence flow, so a bare struct with one row is sufficient.
func setSynthActiveInstrumentForTest(instID string) *DrumView {
	dv := &DrumView{}
	dv.Rows = []*DrumRow{{Instrument: instID}}
	return dv
}

// TestSynthSave_PersistsOverrideAndPublishesHook drives the Save flow
// for drum-snare. Sets a per-instrument param overlay, calls Save,
// asserts: (a) stub sink received SaveRecipeOverride with the merged
// effective params, (b) registry's default for that recipe was updated,
// (c) EventRecipeSaved was published carrying the recipe id.
func TestSynthSave_PersistsOverrideAndPublishesHook(t *testing.T) {
	const recipeID = "drum-snare"
	instID := captureBoundInstrument(recipeID)
	if instID == "" {
		t.Fatalf("no shipped instrument bound to %q (test fixture broken)", recipeID)
	}
	// Restore the original default on cleanup so subsequent tests see the
	// shipped value.
	var origDecay float64
	for _, p := range audio.RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			origDecay = p.Default
		}
	}
	t.Cleanup(func() {
		audio.UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": origDecay})
		audio.ResetInstrumentParams(instID)
	})

	sink := withStubSink(t)
	events := captureRecipeHooks(t)

	// Simulate the user dragging the decay knob to 2.3.
	audio.SetInstrumentParam(instID, "decay", 2.3)

	dv := setSynthActiveInstrumentForTest(instID)
	got := dv.SaveActiveRecipe()
	if got != recipeID {
		t.Fatalf("SaveActiveRecipe = %q, want %q", got, recipeID)
	}

	// (a) stub sink saw the override.
	saved := sink.override(recipeID)
	if saved == nil {
		t.Fatalf("stub sink received no override for %q", recipeID)
	}
	if saved["decay"] != 2.3 {
		t.Errorf("saved decay = %v, want 2.3 (full=%v)", saved["decay"], saved)
	}

	// (b) registry mutated.
	if audio.RecipeDefaultParams(recipeID)["decay"] != 2.3 {
		t.Errorf("registry default decay = %v, want 2.3", audio.RecipeDefaultParams(recipeID)["decay"])
	}

	// (c) hook fired with the recipe id.
	e := waitForKind(t, events, hooks.EventRecipeSaved)
	p, ok := e.Payload.(hooks.RecipePayload)
	if !ok {
		t.Fatalf("EventRecipeSaved payload = %T, want hooks.RecipePayload", e.Payload)
	}
	if p.RecipeID != recipeID || p.InstrumentID != instID {
		t.Errorf("payload = %+v, want recipe=%q inst=%q", p, recipeID, instID)
	}
}

// TestSynthSaveAs_RegistersNewRecipeAndPersists drives the Save-As
// flow. Asserts: a new recipe id is registered, the recipe id pattern
// matches user.<base>.<short>, the stub sink received the doc bytes,
// the doc decodes to a RecipeDoc carrying the seed and base recipe id,
// EventRecipeCreated fires.
func TestSynthSaveAs_RegistersNewRecipeAndPersists(t *testing.T) {
	const baseID = "drum-snare"
	instID := captureBoundInstrument(baseID)
	if instID == "" {
		t.Fatalf("no shipped instrument bound to %q", baseID)
	}
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	sink := withStubSink(t)
	events := captureRecipeHooks(t)

	audio.SetInstrumentParam(instID, "decay", 1.9)

	dv := setSynthActiveInstrumentForTest(instID)
	newID := dv.SaveActiveRecipeAs("")
	if newID == "" {
		t.Fatalf("SaveActiveRecipeAs returned empty id")
	}
	t.Cleanup(func() { audio.UnregisterRecipeForTest(newID) })

	if !strings.HasPrefix(newID, "user.snare.") {
		t.Errorf("new id = %q, want prefix user.snare.", newID)
	}
	if audio.NewRecipe(newID) == nil {
		t.Errorf("NewRecipe(%q) = nil after Save-As", newID)
	}

	raw := sink.recipe(newID)
	if len(raw) == 0 {
		t.Fatalf("stub sink received no payload for %q", newID)
	}
	var doc audio.RecipeDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("doc decode: %v", err)
	}
	if doc.ID != newID {
		t.Errorf("doc.ID = %q want %q", doc.ID, newID)
	}
	if doc.BaseRecipe != baseID {
		t.Errorf("doc.BaseRecipe = %q want %q", doc.BaseRecipe, baseID)
	}
	if doc.Origin != audio.OriginUser {
		t.Errorf("doc.Origin = %q want %q", doc.Origin, audio.OriginUser)
	}
	if doc.ParamSeed["decay"] != 1.9 {
		t.Errorf("seed decay = %v want 1.9 (full=%v)", doc.ParamSeed["decay"], doc.ParamSeed)
	}

	e := waitForKind(t, events, hooks.EventRecipeCreated)
	p, ok := e.Payload.(hooks.RecipePayload)
	if !ok {
		t.Fatalf("EventRecipeCreated payload = %T", e.Payload)
	}
	if p.RecipeID != newID || p.BaseRecipe != baseID || p.InstrumentID != instID {
		t.Errorf("payload = %+v, want recipe=%q base=%q inst=%q", p, newID, baseID, instID)
	}

	// Phase 4 row-rebind: after Save-As, the active row's instrument must
	// resolve to the freshly-cloned recipe so the next trigger uses the
	// saved tone without the user needing to navigate the instrument
	// menu. The overlay must also be cleared (its values just became the
	// new recipe's defaults).
	if got := audio.RecipeForInstrument(instID); got != newID {
		t.Errorf("RecipeForInstrument(%q) = %q after Save-As, want %q (row not rebound)", instID, got, newID)
	}
	overlay := audio.GetInstrumentParams(instID)
	if v, ok := overlay["decay"]; ok && v == 1.9 {
		t.Errorf("per-instrument overlay still carries decay=%v after Save-As — should be cleared (baked into recipe defaults)", v)
	}
	if got := audio.RecipeDefaultParams(newID)["decay"]; got != 1.9 {
		t.Errorf("new recipe %q defaults decay = %v, want 1.9 (seed not applied)", newID, got)
	}

	// Restore the binding so subsequent tests see the shipped state.
	t.Cleanup(func() { audio.BindInstrumentToRecipe(instID, baseID) })
}

// TestSynthSaveAs_PersistedDocReregistersCleanly is the "cross-binding
// round-trip" guard for Phase 5: the JSON the sink received must be
// decodable back into a RecipeDoc that registers cleanly through the
// plugin path — proving the persistence format the JS bridge marshals
// and unmarshals doesn't drift from the audio.RecipeDoc shape. Drives
// the same Save-As code path the synth-tab button uses, then simulates
// a "restart and reload from disk" cycle by unregistering the recipe
// and re-registering from the persisted bytes alone.
func TestSynthSaveAs_PersistedDocReregistersCleanly(t *testing.T) {
	const baseID = "drum-kick"
	instID := captureBoundInstrument(baseID)
	if instID == "" {
		t.Fatalf("no shipped instrument bound to %q", baseID)
	}
	t.Cleanup(func() {
		audio.ResetInstrumentParams(instID)
		audio.BindInstrumentToRecipe(instID, baseID)
	})

	sink := withStubSink(t)
	audio.SetInstrumentParam(instID, "decay", 1.4)

	dv := setSynthActiveInstrumentForTest(instID)
	newID := dv.SaveActiveRecipeAs("")
	if newID == "" {
		t.Fatalf("SaveActiveRecipeAs returned empty id")
	}
	t.Cleanup(func() { audio.UnregisterRecipeForTest(newID) })

	raw := sink.recipe(newID)
	if len(raw) == 0 {
		t.Fatalf("sink received no payload")
	}

	// Simulate restart: drop the in-memory registration, then re-register
	// from the persisted JSON alone (this is the path
	// audio.ApplyUserRecipeOverrides takes at production bootstrap).
	audio.UnregisterRecipeForTest(newID)
	if audio.NewRecipe(newID) != nil {
		t.Fatalf("recipe %q still registered after UnregisterRecipeForTest", newID)
	}

	var doc audio.RecipeDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("doc decode: %v", err)
	}
	if _, err := audio.RegisterUserRecipeFromBase(doc.ID, doc.DisplayName, doc.BaseRecipe, doc.ParamSeed); err != nil {
		t.Fatalf("re-register from persisted bytes: %v", err)
	}
	if audio.NewRecipe(newID) == nil {
		t.Fatalf("NewRecipe(%q) = nil after re-register from persisted JSON", newID)
	}
	got := audio.RecipeDefaultParams(newID)
	if got["decay"] != 1.4 {
		t.Errorf("re-registered recipe defaults decay = %v, want 1.4 (seed lost across JSON round-trip)", got["decay"])
	}
}

// TestSynthSave_NoSinkIsNoOp asserts SaveActiveRecipe still updates the
// in-process registry and publishes a hook when no sink is registered.
// Persistence simply silently no-ops; the in-process side effects must
// still fire so the user hears their edit.
func TestSynthSave_NoSinkIsNoOp(t *testing.T) {
	const recipeID = "drum-snare"
	instID := captureBoundInstrument(recipeID)
	var origDecay float64
	for _, p := range audio.RecipeRegistrations()[recipeID].Params {
		if p.Name == "decay" {
			origDecay = p.Default
		}
	}
	t.Cleanup(func() {
		audio.UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"decay": origDecay})
		audio.ResetInstrumentParams(instID)
	})

	// Explicitly clear the sink for this test.
	prev := activeRecipeSink()
	SetRecipeSink(nil)
	t.Cleanup(func() { SetRecipeSink(prev) })

	events := captureRecipeHooks(t)
	audio.SetInstrumentParam(instID, "decay", 1.55)

	dv := setSynthActiveInstrumentForTest(instID)
	if got := dv.SaveActiveRecipe(); got != recipeID {
		t.Fatalf("SaveActiveRecipe = %q", got)
	}
	if audio.RecipeDefaultParams(recipeID)["decay"] != 1.55 {
		t.Errorf("registry not updated when sink absent")
	}
	_ = waitForKind(t, events, hooks.EventRecipeSaved)
}

// TestHooks_NumNonVerboseUpdated guards the constant in hooks/events.go.
// If Phase 4 forgot to bump it, KindAll[:NumNonVerbose] would silently
// truncate the four new recipe / kit kinds and the eventlogger coverage
// path would emit them anyway — confusing log output.
func TestHooks_NumNonVerboseUpdated(t *testing.T) {
	if hooks.NumNonVerbose != 45 {
		t.Errorf("hooks.NumNonVerbose = %d, want 45 (Phase 4 added 4 recipe/kit kinds; Sampler added 2 sample kinds; factory Reset added 1; the non-destructive sample-edit descriptor added 1)", hooks.NumNonVerbose)
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
