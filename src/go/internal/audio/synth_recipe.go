package audio

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// ParamDef declares one editable parameter on a SynthRecipe. The schema is
// shipped to UI editors and serialized as a plugin manifest, so the shape is a
// v1 freezing surface — see plan section "Plugin-readiness".
type ParamDef struct {
	Name    string  `json:"name"`
	Label   string  `json:"label,omitempty"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	Unit    string  `json:"unit,omitempty"` // "", "ms", "Hz", "dB", "%", "st"
	Curve   string  `json:"curve,omitempty"`
	Group   string  `json:"group,omitempty"`
	// Enum (v2, additive + omitempty) carries display labels for a DISCRETE
	// param. The param stays float-valued (Min:0, Max:len-1, Default:idx) so all
	// clamp/hash/merge/persist machinery is unchanged; Enum only tells the UI to
	// render a selector instead of a knob. A ParamDef without an enum serializes
	// exactly as v1 (no "enum" key), so existing recipe goldens are unaffected.
	Enum []string `json:"enum,omitempty"`
	// Step (v2, additive + omitempty) marks a non-enum param as DISCRETE with
	// detents at Min, Min+Step, ... Max. Enum params are implicitly Step:1.
	// A zero Step serializes identically to v1 (no "step" key), so existing
	// recipe goldens are unaffected.
	Step float64 `json:"step,omitempty"`
}

// RecipeParams is the IPC-shaped parameter payload. Keys are ParamDef.Name.
type RecipeParams map[string]float64

// SynthRecipe is the plugin-shaped interface every instrument synth must
// satisfy. Built-in recipes wrap C renderers; future WASMSynthRecipe will
// satisfy this exact shape so plugin synths drop in without an interface
// change.
type SynthRecipe interface {
	ID() string
	DisplayName() string
	Category() string
	ParamSchema() []ParamDef
	Render(buf []float32, sampleRate, samples, variant int, p RecipeParams)
}

// RecipeRegistration is the registry entry. Add one per built-in recipe
// (or per loaded plugin in v2+) via RegisterRecipe in an init() function.
type RecipeRegistration struct {
	ID          string
	DisplayName string
	Category    string
	Params      []ParamDef
	New         func() SynthRecipe
}

// RecipeDoc is the unified declarative shape every instrument (builtin or
// user-saved) serializes to. Phase 1 introduces it as the single source of
// truth that both shipped builtins and the user-recipe persistence path
// emit; long-term the builtin descriptor table goes away and every recipe
// is a RecipeDoc.
//
// BaseRecipe distinguishes the two flavours:
//   - empty: a shipped builtin whose renderer is owned by the platform
//     entries file (`builtinRecipeRenderers` natively, stub otherwise).
//   - non-empty: a user-cloned recipe whose audio is produced by delegating
//     to BaseRecipe's renderer with ParamSeed overlaid on the merged
//     default-then-user params at trigger time.
//
// ParamDefs is the full ParamDef set the UI renders (already filtered to
// wired + extras via WiredParamsForRecipe for builtins). ParamSeed carries
// the Save-As snapshot for user clones; it overlays ParamDefs[i].Default
// before registration so the new recipe ships with the saved tone.
type RecipeDoc struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"display_name"`
	Category    string       `json:"category"`
	BaseRecipe  string       `json:"base_recipe,omitempty"`
	ParamDefs   []ParamDef   `json:"param_defs"`
	ParamSeed   RecipeParams `json:"param_seed,omitempty"`
	Origin      string       `json:"origin"`
}

// OriginBuiltin / OriginUser are the only two scopes Phase 1-3 ship. Future
// scopes (`project`, `remote`) slot in here without a code change at the
// callsites that read Origin for telemetry.
const (
	OriginBuiltin = "builtin"
	OriginUser    = "user"
)

// ToRegistration materializes a RecipeRegistration from the doc. The
// factory closure is supplied by the caller because builtin and user
// recipes resolve their renderer differently (platform-specific map vs.
// plugin provider); the doc itself stays renderer-agnostic. ParamSeed (if
// non-empty) is overlaid onto a copy of ParamDefs so the registration's
// declared Defaults reflect the seed without mutating the doc in place.
func (d RecipeDoc) ToRegistration(factory func() SynthRecipe) RecipeRegistration {
	params := make([]ParamDef, len(d.ParamDefs))
	copy(params, d.ParamDefs)
	if len(d.ParamSeed) > 0 {
		for i := range params {
			if v, ok := d.ParamSeed[params[i].Name]; ok {
				if v < params[i].Min {
					v = params[i].Min
				} else if v > params[i].Max {
					v = params[i].Max
				}
				params[i].Default = v
			}
		}
	}
	return RecipeRegistration{
		ID:          d.ID,
		DisplayName: d.DisplayName,
		Category:    d.Category,
		Params:      params,
		New:         factory,
	}
}

// validateParamDef enforces invariants the UI and JSON layers rely on.
func validateParamDef(d ParamDef) error {
	if d.Name == "" {
		return fmt.Errorf("ParamDef.Name is required")
	}
	if d.Min > d.Max {
		return fmt.Errorf("ParamDef %q: Min=%v exceeds Max=%v", d.Name, d.Min, d.Max)
	}
	if d.Default < d.Min || d.Default > d.Max {
		return fmt.Errorf("ParamDef %q: Default=%v outside [%v,%v]", d.Name, d.Default, d.Min, d.Max)
	}
	return nil
}

// instrumentParamsManager holds per-instrument RecipeParams plus a mapping
// from instrument id to the SynthRecipe id it resolves through.
//
// Per-instrument param mutations go through SetInstrumentParam. The manager:
//   - copies the merged param set,
//   - invalidates voice cache entries for that instrument via the
//     voiceCacheInvalidate hook (wired up in voice_cache.go on native builds),
//   - publishes an EventInstrumentParamChanged hook so external subscribers
//     (eventlogger narrative, scope export, etc.) see the change, and
//   - calls platformInstrumentParamsChanged for the WASM bridge.
//
// Bulk Set (used by the import path) replaces the entire param set so old
// values can't leak past a project load.
type instrumentParamsManager struct {
	mu       sync.RWMutex
	params   map[string]RecipeParams // instrumentID → params
	bindings map[string]string       // instrumentID → recipeID
}

func newInstrumentParamsManager() *instrumentParamsManager {
	return &instrumentParamsManager{
		params:   make(map[string]RecipeParams),
		bindings: make(map[string]string),
	}
}

var instrumentParamsMgr = newInstrumentParamsManager()

// platformInstrumentParamsChanged is invoked after every Set/Reset so
// platform-specific bridges (WASM → window.updateInstrumentParams) can sync
// state. Default is a no-op; the js+wasm build overrides it from
// synth_recipe_wasm.go (added in Phase 5). Tests swap it via t.Cleanup
// following the pattern in effect_chain_extended_test.go:181.
var platformInstrumentParamsChanged = func(id string, p RecipeParams) {}

// SwapPlatformInstrumentParamsChangedForTest replaces the platform
// callback hook and returns the previous value. Used by cross-package
// tests in internal/ui to instrument the bridge; production code never
// reaches this path. Mirrors the existing pattern for
// SetSaveJSONForTest in internal/ui/export.go.
func SwapPlatformInstrumentParamsChangedForTest(fn func(id string, p RecipeParams)) func(id string, p RecipeParams) {
	prev := platformInstrumentParamsChanged
	if fn != nil {
		platformInstrumentParamsChanged = fn
	}
	return prev
}

// voiceCacheInvalidate drops cached voices for a single instrument. The
// native voice_cache.go (Phase 1.4) sets this to globalVoiceCache.ClearInstrument;
// under -tags test || js it stays a no-op (no real cache).
var voiceCacheInvalidate = func(instrumentID string) {}

// SetInstrumentParam updates one parameter for one instrument. The value is
// run through sanitizeParamValue first — NaN/Inf are dropped, out-of-range
// values are clamped to the recipe's declared [Min,Max]. The mutation is
// followed by voice-cache invalidation + hook publish + platform callback,
// all under the manager's lock-free path (the lock is only held during the
// map update).
func SetInstrumentParam(instrumentID, name string, value float64) {
	start := time.Now()
	def := lookupParamDef(instrumentID, name)
	stored, ok := sanitizeParamValue(def, value)
	if !ok {
		// Non-finite input — refuse to store. Hook + platform callback are
		// skipped so observers don't see a phantom edit that didn't happen.
		recordParamRejected()
		return
	}
	m := instrumentParamsMgr
	m.mu.Lock()
	cur, hadEntry := m.params[instrumentID]
	if !hadEntry {
		cur = RecipeParams{}
		m.params[instrumentID] = cur
	}
	// Phase-8 no-op skip: if the value we're about to write equals the value
	// already there, the rest of the dispatch (cache invalidate, hook publish,
	// platform/WASM callback) is pure waste. Stationary drag bursts produce
	// many same-value writes per second; collapsing them at the audio layer
	// covers every caller (UI, JS bridge, JSON import) with one guard.
	if hadEntry {
		if prev, present := cur[name]; present && prev == stored {
			m.mu.Unlock()
			recordParamCoalesced()
			return
		}
	}
	cur[name] = stored
	snapshot := cloneRecipeParams(cur)
	recipeID := m.bindings[instrumentID]
	m.mu.Unlock()

	voiceCacheInvalidate(instrumentID)
	hooks.PublishKind(hooks.EventInstrumentParamChanged, hooks.InstrumentParamPayload{
		Channel: instrumentID,
		Recipe:  recipeID,
		Param:   name,
		Value:   stored,
	})
	platformInstrumentParamsChanged(instrumentID, snapshot)
	recordParamDispatch(time.Since(start))
}

// SetInstrumentParams replaces the entire param set for an instrument. Used
// by the import path so a project load can't merge stale per-instrument
// state on top of the imported defaults. Each value is sanitized — non-finite
// values are dropped (the key is skipped rather than written), out-of-range
// values are clamped. This protects the same boundary SetInstrumentParam
// does so corrupted project JSON cannot poison the manager.
func SetInstrumentParams(instrumentID string, p RecipeParams) {
	var sanitized RecipeParams
	if len(p) > 0 {
		sanitized = make(RecipeParams, len(p))
		for name, value := range p {
			stored, ok := sanitizeParamValue(lookupParamDef(instrumentID, name), value)
			if !ok {
				continue
			}
			sanitized[name] = stored
		}
	}
	m := instrumentParamsMgr
	m.mu.Lock()
	// No-op skip (mirrors SetInstrumentParam's per-value skip): if the sanitized
	// set equals what is already stored, the cache-invalidate + platform/WASM
	// re-render are pure waste. The JSON import path that EVERY undo/redo restore
	// rides re-applies every instrument's full param set, so without this an undo
	// of a single synth knob re-renders the whole kit's PCM on WASM — the
	// "applying actions is slow" lag.
	if recipeParamsEqual(m.params[instrumentID], sanitized) {
		m.mu.Unlock()
		return
	}
	if len(sanitized) == 0 {
		delete(m.params, instrumentID)
	} else {
		m.params[instrumentID] = sanitized
	}
	snapshot := cloneRecipeParams(m.params[instrumentID])
	m.mu.Unlock()

	voiceCacheInvalidate(instrumentID)
	platformInstrumentParamsChanged(instrumentID, snapshot)
}

// InstrumentParamsMgrLenForTest returns the number of instrument ids the
// params manager currently retains. Soak tests use this to detect leaks
// where SetInstrumentParam grows the manager faster than ResetInstrumentParams
// drops entries — every shipped instrument has a stable id, so the count
// should stay within ~25 in production.
func InstrumentParamsMgrLenForTest() int {
	m := instrumentParamsMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.params)
}

// GetInstrumentParams returns a fresh copy of the per-instrument params (or
// an empty map for unknown instruments). Callers may mutate the returned map
// without affecting the manager state.
func GetInstrumentParams(instrumentID string) RecipeParams {
	m := instrumentParamsMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneRecipeParams(m.params[instrumentID])
}

// ResetInstrumentParams clears the per-instrument override map so the
// instrument falls back to RecipeDefaultParams at the next trigger.
func ResetInstrumentParams(instrumentID string) {
	m := instrumentParamsMgr
	m.mu.Lock()
	_, hadEntry := m.params[instrumentID]
	if hadEntry {
		delete(m.params, instrumentID)
	}
	m.mu.Unlock()
	if !hadEntry {
		return
	}
	voiceCacheInvalidate(instrumentID)
	platformInstrumentParamsChanged(instrumentID, RecipeParams{})
}

// BindInstrumentToRecipe records which SynthRecipe id an instrument resolves
// to. Used to populate the Recipe field on hooks payloads and as the
// fallback when MergeRecipeDefaults is called without an explicit id.
// Empty recipeID clears the binding.
//
// Fires platformInstrumentRecipeChanged (outside the lock) so the WASM
// bridge can repoint the JS renderer at the new recipe — see
// synth_recipe_binding_hook.go.
func BindInstrumentToRecipe(instrumentID, recipeID string) {
	m := instrumentParamsMgr
	m.mu.Lock()
	// No-op skip: re-binding to the recipe already in effect would re-fire the
	// platform/WASM re-bind for no reason. The undo/redo full re-import re-binds
	// every instrument on every restore — skip the unchanged ones.
	if m.bindings[instrumentID] == recipeID {
		m.mu.Unlock()
		return
	}
	if recipeID == "" {
		delete(m.bindings, instrumentID)
	} else {
		m.bindings[instrumentID] = recipeID
	}
	m.mu.Unlock()
	platformInstrumentRecipeChanged(instrumentID, recipeID, recipeExemplarInstrument(recipeID))
}

// RecipeForInstrument returns the SynthRecipe id bound to an instrument id
// via BindInstrumentToRecipe. Empty string means no binding.
func RecipeForInstrument(instrumentID string) string {
	m := instrumentParamsMgr
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bindings[instrumentID]
}

// GenericSynthParamDefs returns the 8-knob ParamDef set every drum recipe
// inherits. The semantics + ranges match the C synth_params struct exactly:
//   - pitch: semitone offset (identity 0)
//   - decay/attack: multipliers (identity 1)
//   - tone: -1 dark / 1 bright (identity 0)
//   - drive/body/color/brightness: 0..1 (identity 0)
//
// Recipes can extend this with recipe-specific extras (FM operator params,
// for example) via additional ParamDefs in their registration. Always
// returns a fresh slice so callers may sort or mutate freely.
func GenericSynthParamDefs() []ParamDef {
	return []ParamDef{
		{Name: "pitch", Group: "generic", Min: -24, Max: 24, Default: 0, Unit: "st"},
		{Name: "decay", Group: "generic", Min: 0, Max: 4, Default: 1},
		{Name: "tone", Group: "generic", Min: -1, Max: 1, Default: 0},
		{Name: "attack", Group: "generic", Min: 0, Max: 4, Default: 1},
		{Name: "drive", Group: "generic", Min: 0, Max: 1, Default: 0},
		{Name: "body", Group: "generic", Min: 0, Max: 1, Default: 0},
		{Name: "color", Group: "generic", Min: -1, Max: 1, Default: 0},
		{Name: "brightness", Group: "generic", Min: 0, Max: 1, Default: 0},
	}
}

// builtinRecipeDescriptor describes a built-in synth recipe registered at
// package init time. The descriptor is platform-agnostic; the native build
// (engine_registry_entries_native.go) and the stub build (..._stub.go)
// each supply their own factory closure pointed at by ID lookup.
type builtinRecipeDescriptor struct {
	ID       string
	Display  string
	Category string // "drum" | "fm" | future plugin categories
	// Seed optionally diverges this recipe's default ParamDefs from the
	// schema identity (used by preset variants that share a renderer but
	// ship different default values, e.g. synth-modular-pad). Overlaid onto
	// ParamDefs via RecipeDoc.ParamSeed in BuildBuiltinRecipeDocs. A
	// non-empty Seed marks the recipe as needing a browser bootstrap push
	// of its defaults (see SeedInstrumentDefaultsToPlatform).
	Seed RecipeParams
}

// builtinRecipeDescriptors is the single source of truth for which recipes
// register in v1. Phase 1 shipped the first 6 drums; Phase 2 extends the
// list to all 20 drum + 5 FM recipes (25 total). The shared 8 generic
// ParamDefs apply to every entry via GenericSynthParamDefs(); FM-specific
// extras (per-operator + mod-matrix knobs) land in v2.
var builtinRecipeDescriptors = []builtinRecipeDescriptor{
	// Phase 1 drums.
	{ID: "drum-snare", Display: "Snare", Category: "drum"},
	{ID: "drum-kick", Display: "Kick", Category: "drum"},
	{ID: "drum-hihat", Display: "Hi-Hat", Category: "drum"},
	{ID: "drum-clap", Display: "Clap", Category: "drum"},
	{ID: "drum-tom", Display: "Tom", Category: "drum"},
	{ID: "drum-cowbell", Display: "Cowbell", Category: "drum"},
	// Phase 2 drums (14).
	{ID: "drum-open-hihat", Display: "Open Hi-Hat", Category: "drum"},
	{ID: "drum-tom-high", Display: "Tom High", Category: "drum"},
	{ID: "drum-tom-low", Display: "Tom Low", Category: "drum"},
	{ID: "drum-sub-bass", Display: "Sub Bass", Category: "drum"},
	{ID: "drum-snare-rimshot", Display: "Rimshot", Category: "drum"},
	{ID: "drum-snare-sidestick", Display: "Sidestick", Category: "drum"},
	{ID: "drum-kick-deep", Display: "Kick Deep", Category: "drum"},
	{ID: "drum-kick-punchy", Display: "Kick Punchy", Category: "drum"},
	{ID: "drum-kick-lofi", Display: "Kick Lo-Fi", Category: "drum"},
	{ID: "drum-kick-tight", Display: "Kick Tight", Category: "drum"},
	{ID: "drum-shaker", Display: "Shaker", Category: "drum"},
	{ID: "drum-ride", Display: "Ride", Category: "drum"},
	{ID: "drum-crash", Display: "Crash", Category: "drum"},
	// Phase 2 FM (5).
	{ID: "fm-bass", Display: "FM Bass", Category: "fm"},
	{ID: "fm-bell", Display: "FM Bell", Category: "fm"},
	{ID: "fm-lead", Display: "FM Lead", Category: "fm"},
	{ID: "fm-epiano", Display: "FM E-Piano", Category: "fm"},
	{ID: "fm-pluck", Display: "FM Pluck", Category: "fm"},
	// Unified modular synth voice — exposes the ENTIRE pipeline (osc/FM/ADSR/
	// filter/post) via ModularSynthParamDefs instead of the wired generic set.
	{ID: "synth-modular", Display: "Modular", Category: modularRecipeCategory},
	// Second shipped modular preset: same schema, pad defaults via Seed.
	{ID: "synth-modular-pad", Display: "Modular Pad", Category: modularRecipeCategory, Seed: modularPadSeed},
	// Bowed strings family — LFO→pitch vibrato showcase (Phase-1 seeds).
	{ID: "synth-modular-violin", Display: "Violin", Category: modularRecipeCategory, Seed: violinSeed},
	{ID: "synth-modular-violin-ensemble", Display: "Violin Ensemble", Category: modularRecipeCategory, Seed: violinEnsembleSeed},
	{ID: "synth-modular-cello", Display: "Cello", Category: modularRecipeCategory, Seed: celloSeed},
	{ID: "synth-modular-cello-warm", Display: "Cello Warm", Category: modularRecipeCategory, Seed: celloWarmSeed},
	{ID: "synth-modular-organ-church", Display: "Church Organ", Category: modularRecipeCategory, Seed: organChurchSeed},
	{ID: "synth-modular-scifi-lead", Display: "Sci-Fi Lead", Category: modularRecipeCategory, Seed: sciFiLeadSeed},
	// Plucked strings — Karplus-Strong (Task 2).
	{ID: "synth-modular-guitar-nylon", Display: "Guitar Nylon", Category: modularRecipeCategory, Seed: guitarNylonSeed},
	{ID: "synth-modular-guitar-nylon-bright", Display: "Guitar Nylon Bright", Category: modularRecipeCategory, Seed: guitarNylonBrightSeed},
	{ID: "synth-modular-guitar-steel", Display: "Guitar Steel", Category: modularRecipeCategory, Seed: guitarSteelSeed},
	{ID: "synth-modular-guitar-steel-warm", Display: "Guitar Steel Warm", Category: modularRecipeCategory, Seed: guitarSteelWarmSeed},
	{ID: "synth-modular-guitar-electric", Display: "Guitar Electric", Category: modularRecipeCategory, Seed: guitarElectricSeed},
	{ID: "synth-modular-harp", Display: "Harp", Category: modularRecipeCategory, Seed: harpSeed},
	{ID: "synth-modular-guitar-electric-neck", Display: "Guitar Electric Neck", Category: modularRecipeCategory, Seed: guitarElectricNeckSeed},
	// Keys — additive piano (Task 3).
	{ID: "synth-modular-piano-grand", Display: "Piano Grand", Category: modularRecipeCategory, Seed: pianoGrandSeed},
	{ID: "synth-modular-piano-felt", Display: "Piano Felt", Category: modularRecipeCategory, Seed: pianoFeltSeed},
	// Woodwind family (Task 4).
	{ID: "synth-modular-flute", Display: "Flute", Category: modularRecipeCategory, Seed: fluteSeed},
	{ID: "synth-modular-flute-breathy", Display: "Flute Breathy", Category: modularRecipeCategory, Seed: fluteBreathySeed},
	{ID: "synth-modular-oboe", Display: "Oboe", Category: modularRecipeCategory, Seed: oboeSeed},
	{ID: "synth-modular-oboe-full", Display: "Oboe Full", Category: modularRecipeCategory, Seed: oboeFullSeed},
	// Brass family (Task 5).
	{ID: "synth-modular-trumpet", Display: "Trumpet", Category: modularRecipeCategory, Seed: trumpetSeed},
	{ID: "synth-modular-trumpet-mellow", Display: "Trumpet Mellow", Category: modularRecipeCategory, Seed: trumpetMellowSeed},
	{ID: "synth-modular-french-horn", Display: "French Horn", Category: modularRecipeCategory, Seed: frenchHornSeed},
	{ID: "synth-modular-french-horn-loud", Display: "French Horn Loud", Category: modularRecipeCategory, Seed: frenchHornLoudSeed},
	// Bass guitar — tuned plucked electric bass (renamed from synth-bass).
	{ID: "synth-modular-bass-guitar", Display: "Bass Guitar", Category: modularRecipeCategory, Seed: bassGuitarSeed},
	// Synth bass family — acid / reese / FM / 808.
	{ID: "synth-modular-bass-acid", Display: "Acid Bass", Category: modularRecipeCategory, Seed: bassAcidSeed},
	{ID: "synth-modular-bass-reese", Display: "Reese Bass", Category: modularRecipeCategory, Seed: bassReeseSeed},
	{ID: "synth-modular-bass-fm", Display: "FM Bass DX", Category: modularRecipeCategory, Seed: bassFMSeed},
	{ID: "synth-modular-bass-808", Display: "808 Bass", Category: modularRecipeCategory, Seed: bass808Seed},
	// Modal conga (Task 7).
	{ID: "synth-modular-conga", Display: "Conga", Category: modularRecipeCategory, Seed: congaSeed},
	{ID: "synth-modular-conga-open", Display: "Conga Open", Category: modularRecipeCategory, Seed: congaOpenSeed},
	{ID: "synth-modular-conga-tumba", Display: "Conga Tumba", Category: modularRecipeCategory, Seed: congaTumbaSeed},
	// Masterpiece template set — three new instruments.
	{ID: "synth-modular-organ", Display: "Organ", Category: modularRecipeCategory, Seed: organSeed},
	{ID: "synth-modular-sax", Display: "Saxophone", Category: modularRecipeCategory, Seed: saxSeed},
	// Configurable KICK stage — the modular gen-bank kick voice (source==5)
	// surfaced as a first-class instrument, tuned to a deep, dark DnB sub-kick.
	{ID: "synth-modular-kick-dnb", Display: "DnB Kick", Category: modularRecipeCategory, Seed: dnbKickSeed},
	{ID: "synth-modular-kick-electro", Display: "Electro Kick", Category: modularRecipeCategory, Seed: electroKickSeed},
	{ID: "synth-modular-kick-808", Display: "808 Kick", Category: modularRecipeCategory, Seed: kick808Seed},
	{ID: "synth-modular-kick-acoustic", Display: "Acoustic Kick", Category: modularRecipeCategory, Seed: acousticKickSeed},
	{ID: "synth-modular-kick-punchy", Display: "Punchy Kick", Category: modularRecipeCategory, Seed: punchyKickSeed},
}

// builtinInstrumentRecipeBindings maps every shipped instrument id (base
// + variant) to its SynthRecipe id, so SetInstrumentParam edits flow
// through the registry at trigger time. The variant entries inherit the
// underlying renderer of their base sound (e.g. "snare-1" uses
// renderSnareRimshot, so its recipe is drum-snare-rimshot, not drum-snare),
// keeping the recipe binding aligned with the audio that actually plays.
var builtinInstrumentRecipeBindings = map[string]string{
	// Core 6 drums.
	"snare":   "drum-snare",
	"kick":    "drum-kick",
	"hihat":   "drum-hihat",
	"clap":    "drum-clap",
	"tom":     "drum-tom",
	"cowbell": "drum-cowbell",
	// New distinct drum instruments.
	"rimshot":   "drum-snare-rimshot",
	"sidestick": "drum-snare-sidestick",
	"kick-deep": "drum-kick-deep",
	"shaker":    "drum-shaker",
	"ride":      "drum-ride",
	"crash":     "drum-crash",
	"sub-bass":  "drum-sub-bass",
	// Variant set 1 (electronic/tight) — bound to the actual base renderer used.
	"snare-1":    "drum-snare-rimshot",
	"kick-1":     "drum-kick-punchy",
	"hihat-1":    "drum-open-hihat",
	"tom-1":      "drum-tom-high",
	"clap-1":     "drum-clap",
	"cowbell-1":  "drum-cowbell",
	"sub-bass-1": "drum-sub-bass",
	// Variant set 2 (lo-fi/dark).
	"snare-2":   "drum-snare",
	"kick-2":    "drum-kick-lofi",
	"hihat-2":   "drum-hihat",
	"tom-2":     "drum-tom-low",
	"clap-2":    "drum-clap",
	"cowbell-2": "drum-cowbell",
	// Variant set 3 (ghost / pedal / tight).
	"snare-ghost": "drum-snare",
	"kick-tight":  "drum-kick-tight",
	"hihat-pedal": "drum-hihat",
	"clap-tight":  "drum-clap",
	// FM base + variants share the same recipe (only DefaultFX differs).
	"fm-bass":     "fm-bass",
	"fm-bell":     "fm-bell",
	"fm-lead":     "fm-lead",
	"fm-epiano":   "fm-epiano",
	"fm-pluck":    "fm-pluck",
	"fm-bass-1":   "fm-bass",
	"fm-bell-1":   "fm-bell",
	"fm-lead-1":   "fm-lead",
	"fm-epiano-1": "fm-epiano",
	"fm-pluck-1":  "fm-pluck",
	// Unified modular synth voice.
	"modular":     "synth-modular",
	"modular-pad": "synth-modular-pad",
	// Bowed strings family.
	"violin":          "synth-modular-violin",
	"violin-ensemble": "synth-modular-violin-ensemble",
	"cello":           "synth-modular-cello",
	"cello-warm":      "synth-modular-cello-warm",
	"organ-church":    "synth-modular-organ-church",
	"scifi-lead":      "synth-modular-scifi-lead",
	// Plucked strings — guitars (Task 2).
	"guitar-nylon":         "synth-modular-guitar-nylon",
	"guitar-nylon-bright":  "synth-modular-guitar-nylon-bright",
	"guitar-steel":         "synth-modular-guitar-steel",
	"guitar-steel-warm":    "synth-modular-guitar-steel-warm",
	"guitar-electric":      "synth-modular-guitar-electric",
	"harp":                 "synth-modular-harp",
	"guitar-electric-neck": "synth-modular-guitar-electric-neck",
	// Keys — piano (Task 3).
	"piano-grand": "synth-modular-piano-grand",
	"piano-felt":  "synth-modular-piano-felt",
	// Woodwind (Task 4).
	"flute":         "synth-modular-flute",
	"flute-breathy": "synth-modular-flute-breathy",
	"oboe":          "synth-modular-oboe",
	"oboe-full":     "synth-modular-oboe-full",
	// Brass (Task 5).
	"trumpet":          "synth-modular-trumpet",
	"trumpet-mellow":   "synth-modular-trumpet-mellow",
	"french-horn":      "synth-modular-french-horn",
	"french-horn-loud": "synth-modular-french-horn-loud",
	// Bass guitar (renamed from synth-bass) + synth bass family.
	"bass-guitar": "synth-modular-bass-guitar",
	"bass-acid":   "synth-modular-bass-acid",
	"bass-reese":  "synth-modular-bass-reese",
	"bass-fm":     "synth-modular-bass-fm",
	"bass-808":    "synth-modular-bass-808",
	// Modal conga (Task 7).
	"conga":       "synth-modular-conga",
	"conga-open":  "synth-modular-conga-open",
	"conga-tumba": "synth-modular-conga-tumba",
	// Masterpiece template set — three new instruments.
	"organ": "synth-modular-organ",
	"sax":   "synth-modular-sax",
	// Configurable KICK stage instruments.
	"dnb-kick":      "synth-modular-kick-dnb",
	"kick-electro":  "synth-modular-kick-electro",
	"kick-808":      "synth-modular-kick-808",
	"kick-acoustic": "synth-modular-kick-acoustic",
	"kick-punchy":   "synth-modular-kick-punchy",
}

// factoryRecipeForInstrument returns the shipped SynthRecipe id for a built-in
// instrument id, straight from builtinInstrumentRecipeBindings. Unlike
// RecipeForInstrument (which reads the live, mutable binding table that the
// Sampler's Save clears) this is the authoritative, restart-safe factory
// identity — a factory Reset uses it to recognise a built-in even after its
// live binding was cleared or the app was restarted. Returns "" for non-built-in
// ids (user samples / Save-As clones).
func factoryRecipeForInstrument(instID string) string {
	if r, ok := builtinInstrumentRecipeBindings[instID]; ok {
		return r
	}
	// Instance variants ("organ-2") resolve to their base instrument's recipe
	// so a duplicated voice renders identically without its own binding entry.
	// instanceBaseID is the tag-neutral strip shared with EnsureInstanceInstrument
	// (the dispatch-file baseInstrumentID carries a !test build tag and can't be
	// used here).
	if base := instanceBaseID(instID); base != instID {
		return builtinInstrumentRecipeBindings[base]
	}
	return ""
}

// bindBuiltinInstrumentRecipes wires each built-in instrument id to its
// SynthRecipe so SetInstrumentParam edits propagate through the registry
// at trigger time. Called from ResetInstruments in both the native and
// stub build paths so the binding shape is identical across builds.
func bindBuiltinInstrumentRecipes() {
	for instID, recipeID := range builtinInstrumentRecipeBindings {
		BindInstrumentToRecipe(instID, recipeID)
	}
}

// init runs at package-load time under every build tag so the
// builtinInstrumentRecipeBindings table is wired before any caller
// queries RecipeForInstrument. ResetInstruments still calls
// bindBuiltinInstrumentRecipes explicitly for the !test && !js and
// test paths (idempotent — re-binding the same ID is a no-op), but this
// init() is what makes the WASM build pick up the bindings without
// requiring a ResetInstruments() call at startup.
func init() {
	bindBuiltinInstrumentRecipes()
}

// BuildBuiltinRecipeDocs projects builtinRecipeDescriptors into the
// unified RecipeDoc shape every recipe (builtin + user-saved) is registered
// through after Phase 1. ParamDefs are filled in from WiredParamsForRecipe
// so each doc carries the same per-recipe knob set the registry currently
// uses; Origin is fixed to OriginBuiltin; BaseRecipe is empty (builtins own
// their renderer in the platform entries file).
//
// Returns a fresh slice (with fresh ParamDef slices) so callers can mutate
// without affecting the canonical source. Order follows builtinRecipeDescriptors.
func BuildBuiltinRecipeDocs() []RecipeDoc {
	out := make([]RecipeDoc, 0, len(builtinRecipeDescriptors))
	for _, d := range builtinRecipeDescriptors {
		var params []ParamDef
		if d.Category == modularRecipeCategory {
			// The modular voice exposes its own full param set, not the
			// wired-generic subset; it reads modular_params, not sp_*. Its
			// generator selector is osc_type (no Native option).
			params = ModularSynthParamDefs()
		} else {
			params = WiredParamsForRecipe(d.ID)
			if params == nil {
				params = GenericSynthParamDefs()
			}
		}
		// WiredParamsForRecipe returns a fresh slice already; defensively
		// copy so test mutations on one doc can't leak to others through
		// any future caching layer.
		paramsCopy := make([]ParamDef, len(params))
		copy(paramsCopy, params)
		// Preset variants (e.g. synth-modular-pad) bake their divergent
		// defaults straight into ParamDefs — builtins carry their final
		// defaults in ParamDefs and leave ParamSeed empty (the invariant
		// TestRecipeDocRoundtripAllRecipes enforces). Values are clamped to
		// each param's [Min,Max].
		if len(d.Seed) > 0 {
			for i := range paramsCopy {
				if v, ok := d.Seed[paramsCopy[i].Name]; ok {
					if v < paramsCopy[i].Min {
						v = paramsCopy[i].Min
					} else if v > paramsCopy[i].Max {
						v = paramsCopy[i].Max
					}
					paramsCopy[i].Default = v
				}
			}
		}
		out = append(out, RecipeDoc{
			ID:          d.ID,
			DisplayName: d.Display,
			Category:    d.Category,
			ParamDefs:   paramsCopy,
			Origin:      OriginBuiltin,
		})
	}
	return out
}

// registerBuiltinRecipes calls RegisterRecipe for every builtin RecipeDoc,
// delegating the factory closure to the build-specific entries file. The
// factoryFor closure receives the full RecipeDoc and returns the New func;
// native builds wrap a C renderer (looked up by doc.ID), stub builds return
// a no-op recipe.
func registerBuiltinRecipes(factoryFor func(doc RecipeDoc) func() SynthRecipe) {
	for _, doc := range BuildBuiltinRecipeDocs() {
		RegisterRecipe(doc.ToRegistration(factoryFor(doc)))
	}
}

// cloneRecipeParams returns a deep copy of p (which may be nil). The output
// is always non-nil so callers can range freely.
func cloneRecipeParams(p RecipeParams) RecipeParams {
	out := make(RecipeParams, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// recipeParamsEqual reports whether two param sets are identical (same keys,
// same values). Empty and nil are equal. Used to skip redundant re-applies in
// SetInstrumentParams so a full re-import (undo/redo) doesn't re-render
// unchanged instruments.
func recipeParamsEqual(a, b RecipeParams) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// hashRecipeParams returns a deterministic FNV-1a hash over the (name,value)
// pairs of p. The hash is order-independent (keys are sorted before hashing),
// so two maps with identical content always hash to the same value even when
// Go's map iteration order differs.
//
// The hash backs voiceCacheKey.paramsHash; collisions would cause stale audio
// to be served, so the determinism + differentiation tests live in
// synth_recipe_test.go as a v1 freezing-surface invariant.
func hashRecipeParams(p RecipeParams) uint64 {
	h := fnv.New64a()
	if len(p) == 0 {
		return h.Sum64()
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf [8]byte
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(p[k]))
		h.Write(buf[:])
	}
	return h.Sum64()
}
