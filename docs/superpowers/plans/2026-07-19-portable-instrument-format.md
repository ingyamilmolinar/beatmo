# Portable, decoupled instrument format — Implementation Plan

> **For agentic workers:** implement task-by-task with TDD. Steps use `- [ ]`.

**Goal:** A standalone, config-only, shareable instrument file (`*.beatmo-inst.json`) holding 1+ instruments with a normalized effects library, WAV carried by reference + content-hash version pin (never embedded), plus a persistence store decoupled from the user/game prefs blob, wired into the Synth/Sampler `⋯` menu.

**Architecture:** Format types + normalize/denormalize + hashing live in `internal/audio` (they reference `RecipeDoc`/`EffectSlot`/`SampleEdit`). The decoupled store lives in `internal/userprefs` as its own document + location. Export/import orchestration + UI wiring live in `internal/ui`. All config-only: no PCM in the file.

**Tech Stack:** Go 1.26, `encoding/json`, `crypto/sha256` (pure-Go, wasm-safe), Ebiten UI, `-tags test` fast path.

## Global Constraints (verbatim from spec)

- **No PCM / audio bytes ever embedded** in the shareable file — WAV is a reference (`name` + `sha256` + `version`).
- **Effects normalized**: file-level `effects[]` defined once; instruments carry an ordered `effect_chain` of ids (repeats allowed). Not welded to instruments.
- **Config-only**: recipe (base+seed), synth_params delta, sample_edit, wav_ref, optional insert-FX (normalized) + optional inline EQ. **No sends.**
- **Decoupled store**: instrument data leaves `prefsDoc`; own file/localStorage location; transparent one-time migration.
- **Import never clobbers a built-in**; same-named user instrument → Replace / Keep both.
- **Own `format` magic + integer `version`** with forward migration.
- Bundled Go only: `.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ ./internal/userprefs/ ./internal/ui/ ./internal/i18n/`.

## File Structure

- Create `internal/audio/instrument_bundle.go` — `InstrumentBundle`, `InstrumentDoc`, `EffectDef`, `WavRef` types + `format`/`version` consts + header validation.
- Create `internal/audio/instrument_effects_norm.go` — `NormalizeEffects`/`DenormalizeEffects`.
- Create `internal/audio/content_hash.go` — build-tag-free `Sha256Hex([]byte) string` + `HashSamplePCM`.
- Create `internal/audio/wav_registry.go` — `WavRegistry` (name→versions) type + methods.
- Create `internal/userprefs/instrument_store.go` — shared doc + interface; `internal/userprefs/instrument_store_notjs.go` + `_js.go` for backends.
- Create `internal/ui/instrument_export.go` — build a bundle from an instrument/row.
- Create `internal/ui/instrument_import.go` — apply a bundle (collision policy, wav resolve).
- Modify `internal/ui/action_menu_open.go` — add Export/Import items to synth+sampler `⋯` menus.
- Modify `internal/i18n/{keys,catalog_en,catalog_es}.go` — new strings.

---

## Task 1: Bundle format types + header validation

**Files:**
- Create: `internal/audio/instrument_bundle.go`
- Test: `internal/audio/instrument_bundle_test.go`

**Interfaces produced:**
- `const InstrumentBundleFormat = "beatmo.instrument"`, `const InstrumentBundleVersion = 1`
- `type WavRef struct { Name string; Sha256 string; Version int }`
- `type EffectDef struct { ID string; Name string; Type EffectType; Params map[string]float64; Enabled bool }`
- `type InstrumentDoc struct { Name, Kind, Color string; Recipe RecipeDoc; SynthParams map[string]float64; SampleEdit map[string]float64; WavRef *WavRef; EffectChain []string; EQ *InstrumentEQ }`
- `type InstrumentEQ struct { GainsDB []float64; BandsHz [][2]float64; BandMuted []bool; HPFEnabled bool; HPFCutoffHz float64; LPFEnabled bool; LPFCutoffHz float64 }`
- `type InstrumentBundle struct { Format string; Version int; Effects []EffectDef; Instruments []InstrumentDoc }`
- `func ValidateBundleHeader(b *InstrumentBundle) error` — nil if `Format==InstrumentBundleFormat && Version>=1 && Version<=InstrumentBundleVersion`; typed errors otherwise.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package audio

import (
	"encoding/json"
	"testing"
)

func TestInstrumentBundleRoundTrip(t *testing.T) {
	in := InstrumentBundle{
		Format:  InstrumentBundleFormat,
		Version: InstrumentBundleVersion,
		Effects: []EffectDef{{ID: "fx1", Name: "Crunch", Type: EffectType("bitcrusher"), Params: map[string]float64{"bits": 6}, Enabled: true}},
		Instruments: []InstrumentDoc{{
			Name: "Warm 808", Kind: "synth", Color: "#C87850FF",
			Recipe:      RecipeDoc{ID: "user.kick.abc", BaseRecipe: "drum-kick", Origin: OriginUser},
			SynthParams: map[string]float64{"tune": 0.4},
			EffectChain: []string{"fx1", "fx1"},
			EQ:          &InstrumentEQ{GainsDB: []float64{0, 0, 3}},
		}},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out InstrumentBundle
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := ValidateBundleHeader(&out); err != nil {
		t.Fatalf("valid header rejected: %v", err)
	}
	if out.Instruments[0].EffectChain[0] != "fx1" || len(out.Instruments[0].EffectChain) != 2 {
		t.Errorf("effect chain not preserved: %v", out.Instruments[0].EffectChain)
	}
	if out.Instruments[0].Recipe.BaseRecipe != "drum-kick" {
		t.Errorf("base recipe lost: %q", out.Instruments[0].Recipe.BaseRecipe)
	}
}

func TestValidateBundleHeaderRejects(t *testing.T) {
	if ValidateBundleHeader(&InstrumentBundle{Format: "nope", Version: 1}) == nil {
		t.Error("foreign format accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Format: InstrumentBundleFormat, Version: InstrumentBundleVersion + 1}) == nil {
		t.Error("future version accepted")
	}
	if ValidateBundleHeader(&InstrumentBundle{Format: InstrumentBundleFormat, Version: 1}) != nil {
		t.Error("valid header rejected")
	}
}
```

- [ ] **Step 2: Run — expect FAIL** (`ValidateBundleHeader`/types undefined). `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestInstrumentBundle ./internal/audio/`

- [ ] **Step 3: Implement `internal/audio/instrument_bundle.go`** with the structs above (json tags: `name,kind,color,recipe,synth_params,omitempty` etc.; `WavRef`/`EQ` pointers `omitempty`; `EffectChain` `effect_chain,omitempty`) and:

```go
const (
	InstrumentBundleFormat  = "beatmo.instrument"
	InstrumentBundleVersion = 1
)

func ValidateBundleHeader(b *InstrumentBundle) error {
	if b == nil {
		return fmt.Errorf("instrument bundle: nil")
	}
	if b.Format != InstrumentBundleFormat {
		return fmt.Errorf("instrument bundle: unrecognized format %q", b.Format)
	}
	if b.Version < 1 || b.Version > InstrumentBundleVersion {
		return fmt.Errorf("instrument bundle: unsupported version %d (max %d)", b.Version, InstrumentBundleVersion)
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit.** (SKIP — repo convention: never stage/commit; leave uncommitted.)

## Task 2: Effects normalize / denormalize

**Files:**
- Create: `internal/audio/instrument_effects_norm.go`
- Test: `internal/audio/instrument_effects_norm_test.go`

**Interfaces produced:**
- `func NormalizeEffects(chain []EffectSlot) ([]EffectDef, []string)` — dedup by `(Type, Params)` into defs with ids `fx1,fx2,…`; return ordered id chain (repeats reference the same id; a repeated slot with same Type+Params reuses its id).
- `func DenormalizeEffects(defs []EffectDef, chain []string) []EffectSlot` — resolve each id → a fresh `EffectSlot{Type, Enabled, Params(copy)}`; skip unknown ids (lenient).

**Design:** identity key = `Type` + canonical JSON of sorted `Params`. `Enabled` is carried on the `EffectDef` and applied per placement (all placements of one def share Enabled — acceptable; a differing Enabled makes a distinct def because it's part of the def, so include Enabled in the dedup key too).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package audio

import "testing"

func TestNormalizeDedupsAndPreservesOrder(t *testing.T) {
	chain := []EffectSlot{
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}},
		{Type: "reverb", Enabled: true, Params: map[string]float64{"mix": 0.3}},
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 6}}, // dup of #0
	}
	defs, ids := NormalizeEffects(chain)
	if len(defs) != 2 {
		t.Fatalf("want 2 deduped defs, got %d", len(defs))
	}
	if len(ids) != 3 || ids[0] != ids[2] {
		t.Errorf("order/repeat not preserved: %v", ids)
	}
	back := DenormalizeEffects(defs, ids)
	if len(back) != 3 || back[0].Type != "bitcrusher" || back[1].Type != "reverb" || back[2].Type != "bitcrusher" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
	if back[0].Params["bits"] != 6 {
		t.Errorf("params lost: %+v", back[0].Params)
	}
}

func TestDenormalizeSkipsUnknownID(t *testing.T) {
	back := DenormalizeEffects([]EffectDef{{ID: "fx1", Type: "reverb"}}, []string{"fx1", "ghost"})
	if len(back) != 1 {
		t.Errorf("unknown id not skipped: %+v", back)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** using a canonical key (sort param keys, format `type|k=v;…|enabled`), a `map[key]string` (key→assigned id), append defs in first-seen order, deep-copy params on both directions (mirror `copyParams` semantics from `effect_chain.go`).
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** (skip per convention).

## Task 3: Content hash (wasm-safe) + WAV registry

**Files:**
- Create: `internal/audio/content_hash.go` (NO build tags — compiles for native/wasm/test)
- Create: `internal/audio/wav_registry.go`
- Test: `internal/audio/wav_registry_test.go`

**Interfaces produced:**
- `func Sha256Hex(b []byte) string`
- `func HashSamplePCM(pcm []float32, sampleRate int) string` — deterministic hash of LE-float32 bytes + sr (reuse the encode path used by the sample store).
- `type WavVersion struct { Version int; Sha256 string }`
- `type WavRegistry struct { … }` with `func NewWavRegistry() *WavRegistry`, `func (r *WavRegistry) Register(name, sha string) WavVersion` (returns existing version if hash already known for name, else next-int version), `func (r *WavRegistry) Lookup(name, sha string) (WavVersion, bool)`, `func (r *WavRegistry) Versions(name string) []WavVersion`, and `MarshalJSON`/`UnmarshalJSON` (map name→[]WavVersion).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package audio

import "testing"

func TestSha256HexStable(t *testing.T) {
	if Sha256Hex([]byte("abc")) != Sha256Hex([]byte("abc")) {
		t.Fatal("hash not stable")
	}
	if Sha256Hex([]byte("abc")) == Sha256Hex([]byte("abd")) {
		t.Fatal("hash collision on different input")
	}
}

func TestWavRegistryVersioning(t *testing.T) {
	r := NewWavRegistry()
	v1 := r.Register("kick.wav", "hashA")
	if v1.Version != 1 {
		t.Fatalf("first version = %d, want 1", v1.Version)
	}
	if again := r.Register("kick.wav", "hashA"); again.Version != 1 {
		t.Errorf("same content got new version %d", again.Version)
	}
	v2 := r.Register("kick.wav", "hashB")
	if v2.Version != 2 {
		t.Errorf("changed content version = %d, want 2", v2.Version)
	}
	if got, ok := r.Lookup("kick.wav", "hashA"); !ok || got.Version != 1 {
		t.Errorf("lookup by hash failed: %+v ok=%v", got, ok)
	}
	if _, ok := r.Lookup("kick.wav", "hashZ"); ok {
		t.Error("unknown hash resolved")
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** `content_hash.go` (`crypto/sha256` + `encoding/hex`; `HashSamplePCM` reuses the LE-float32 encoding) and `wav_registry.go` (a `map[string][]WavVersion` behind a mutex; JSON round-trips the map).
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** (skip).

## Task 4: Decoupled instrument store (own location) + interfaces

**Files:**
- Create: `internal/userprefs/instrument_store.go` (doc + constants + interface)
- Create: `internal/userprefs/instrument_store_notjs.go` (`//go:build !js`), `internal/userprefs/instrument_store_js.go` (`//go:build js`)
- Test: `internal/userprefs/instrument_store_test.go`

**Interfaces produced:**
- `const InstrumentsFileName = "instruments.json"`, `const InstrumentsLocalStorageKey = "beatmo.instruments"`
- `type instrumentsDoc struct { UserRecipes map[string]json.RawMessage; RecipeOverrides map[string]map[string]float64; SampleEdits map[string]map[string]float64; WavRegistry json.RawMessage }` (all `omitempty`)
- `type InstrumentStore interface { LoadUserRecipes() (map[string][]byte, error); SaveUserRecipe(id string, doc []byte) error; DeleteUserRecipe(id string) error; LoadRecipeOverrides() (map[string]map[string]float64, error); SaveRecipeOverride(recipeID string, params map[string]float64) error; LoadSampleEdits() (map[string]map[string]float64, error); SaveSampleEdit(instID string, fields map[string]float64) error; LoadWavRegistry() ([]byte, error); SaveWavRegistry(raw []byte) error; WaitFlushed(); Close() }`
- `func NewInstrumentStore(opts Options) InstrumentStore` (platform-selected).

**Design:** mirror `fileStore`/`localStorageStore` but with `InstrumentsFileName`/`InstrumentsLocalStorageKey` and the `instrumentsDoc` schema. Reuse the same atomic temp+rename (desktop) and `localStorage.setItem` (browser) idioms; may reuse the `userprefs.persist` async pool or a sibling. **This store note:** it satisfies both the audio `UserRecipeSource` (LoadRecipeOverrides+LoadUserRecipes) and the existing `RecipeStore` write surface, so bootstraps can point `audio.ApplyUserRecipeOverrides` + `ui.SetRecipeSink` at it.

- [ ] **Step 1: Write the failing test** — construct via `NewInstrumentStore(Options{Dir: t.TempDir()})` (desktop path under `-tags test` compiles the `!js` file); Save a user recipe + an override + a wav registry blob; new instance Loads them back; assert the prefs store file is untouched (separate path).
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement** the doc, constants, interface, and both backends (mirror `store_notjs.go`/`store_js.go`, own path/key, own marshal that json-indents `instrumentsDoc`).
- [ ] **Step 4: Run — expect PASS.**
- [ ] **Step 5: Commit** (skip).

## Task 5: Boot migration prefs → instrument store

**Files:**
- Modify: `internal/userprefs/instrument_store.go` — add `func MigrateFromPrefs(src RecipeStore & SampleEditStore, dst InstrumentStore) (migrated bool, err error)`.
- Modify bootstraps `cmd/beatmo.go` + `internal/ui/js_bootstrap_wasm.go` — call migration before wiring, then point `ApplyUserRecipeOverrides`/`SetRecipeSink` at the instrument store.
- Test: `internal/userprefs/instrument_migration_test.go`

**Design:** if `dst` is empty (no user recipes, no overrides, no sample edits) AND `src` (prefs) has any → copy all three sections into `dst`; return `migrated=true`. Idempotent: a second run sees `dst` non-empty → no-op. The old prefs sections are left in place but no longer written (bootstraps stop pointing the recipe sink at prefs); a later release can prune them.

- [ ] **Step 1: Write the failing test** — seed a prefs store with a user recipe + override + sample edit; empty instrument store; `MigrateFromPrefs` → assert `migrated==true` and all three land in `dst`; run again → `migrated==false`, no duplication.
- [ ] **Step 2–4:** implement, run green.
- [ ] **Step 5:** wire the two bootstraps (behind the same gates that call `ApplyUserRecipeOverrides` today); commit (skip).

## Task 6: Single-instrument export builder

**Files:**
- Create: `internal/ui/instrument_export.go`
- Test: `internal/ui/instrument_export_test.go`

**Interfaces produced:**
- `func (dv *DrumView) BuildInstrumentBundle(rowIdx int) (audio.InstrumentBundle, error)` — assembles ONE `InstrumentDoc` (+ normalized `effects[]`) from the row's instrument.

**Design (mirror `export.go:594-616` for recipe/params/sample_edit, `:561-563` for effects, `exportEQ` for eq):**
- `instID := dv.Rows[rowIdx].Instrument`.
- Recipe: `recipeID := audio.RecipeForInstrument(instID)`. If non-empty → build `RecipeDoc` via a NEW `audio.BuildInstrumentRecipeDoc(instID)` helper (Task 6a below) that populates `BaseRecipe`+`ParamSeed=delta` correctly; `Kind="synth"` (or `"sample"` if a sample-edit is present; `"wav"` if `audio.IsUserSample(instID)`).
- `SynthParams` = same delta-vs-shipped map (also stored for redundancy/inspection).
- `SampleEdit` = `audio.SampleEditFor(instID).Fields()` when present.
- `WavRef` = when `audio.IsUserSample(instID)`: hash the PCM (`audio.HashSamplePCM`), `Register` in the wav registry, emit `{Name, Sha256, Version}` — **no PCM**.
- Effects: `defs, ids := audio.NormalizeEffects(audio.GetInsertEffects(instID))`.
- EQ: from `dv.Rows[rowIdx]` (GainsDB/BandMuted/HPF/LPF) → `*audio.InstrumentEQ`, omitted when all-zero/disabled.

**Task 6a helper** `internal/audio/instrument_recipe_doc.go` — `func BuildInstrumentRecipeDoc(instID string) (RecipeDoc, error)`: resolve recipe id; `effective := MergeRecipeDefaults(recipe, GetInstrumentParams(instID))`; `shipped := RecipeShippedDefaults(recipe)`; `seed := delta(effective, shipped)`; return `RecipeDoc{ID: recipe (or user id), DisplayName, Category, BaseRecipe: baseOf(recipe), ParamDefs: reg.Params (copy), ParamSeed: seed, Origin}`. For a `user.*` recipe, `BaseRecipe` = its registration's base; for a builtin, `BaseRecipe` = the builtin id itself (so import can re-derive via `RegisterUserRecipeFromBase`). Unit-test this helper directly (round-trips through `RegisterUserRecipeFromBase` → identical effective params).

- [ ] **Step 1: Write failing test** — build a `Game`, bind row 0 to `snare`/`drum-snare`, set a param + one insert effect + an EQ gain; `bundle, _ := g.drum.BuildInstrumentBundle(0)`; assert `bundle.Instruments[0].Recipe.BaseRecipe != ""`, `ParamSeed` non-empty, `len(bundle.Effects)==1`, `EffectChain` references it, `EQ != nil`.
- [ ] **Step 2–4:** implement 6a helper + builder, run green.
- [ ] **Step 5:** commit (skip).

## Task 7: Single-instrument import applier + collision policy

**Files:**
- Create: `internal/ui/instrument_import.go`
- Test: `internal/ui/instrument_import_test.go`

**Interfaces produced:**
- `type ImportCollision struct { DocIndex int; Name string }`
- `func (dv *DrumView) ApplyInstrumentBundle(b *audio.InstrumentBundle, resolve func(ImportCollision) CollisionChoice, applyToRow int) ([]string, error)` — returns the new instrument ids registered. `CollisionChoice = Replace | KeepBoth`.

**Design:**
- `ValidateBundleHeader` first.
- For each `InstrumentDoc`:
  - Compute the target user id. If `doc.Recipe.ID` collides with a **built-in** recipe/instrument → always re-namespace to a fresh `user.*` id (`generateUserRecipeID`). If it collides with an existing **user** instrument → call `resolve(...)`: `Replace` reuses the id; `KeepBoth` re-namespaces.
  - Register: `audio.RegisterUserRecipeFromBase(newID, doc.Name, doc.Recipe.BaseRecipe, doc.Recipe.ParamSeed)` (the correct path).
  - `wav_ref`: `audio.ResolveWavRef(doc.WavRef, dv.wavRegistry(), locateFn)` (Task 7a) → binds PCM via the existing sample path or surfaces a locate/mismatch outcome (see below).
  - Effects: `audio.SetInsertEffects(newID, audio.DenormalizeEffects(b.Effects, doc.EffectChain))`.
  - EQ: apply to `applyToRow` if provided (`row.EQGainsDB=…; dv.applyRowEQ(row)`), else store on the instrument record.
  - `SampleEdit`: `audio.SetSampleEdit(newID, audio.SampleEditFromFields(doc.SampleEdit))` when present.
  - Persist the reconstructed `RecipeDoc` to the instrument store (`SaveUserRecipe`).

**Task 7a** `internal/audio/wav_resolve.go` — `func ResolveWavRef(ref *WavRef, reg *WavRegistry, locate func(name string) ([]float32, int, bool)) (WavResolveResult, error)` where `WavResolveResult` is one of `{Matched, VersionMismatch, Located, Missing}`. Pure enough to unit-test with a fake `locate`.

- [ ] **Step 1: Write failing test** — round-trip: `bundle,_ := g.drum.BuildInstrumentBundle(0)`; on a fresh `Game`, `ApplyInstrumentBundle(&bundle, replaceResolver, 0)`; assert effective params of the new instrument equal the source's, effects chain equals source, and importing a bundle whose `Recipe.ID` equals a builtin does NOT change that builtin (`RecipeShippedDefaults` unchanged). Add a collision test: importing twice with `KeepBoth` yields two distinct ids.
- [ ] **Step 2–4:** implement 7a + applier, run green.
- [ ] **Step 5:** commit (skip).

## Task 8: UI wiring — `⋯` menu items, file IO, collision confirm, i18n

**Files:**
- Modify: `internal/ui/action_menu_open.go` (add items), `internal/ui/drumview.go` + `drumview_ctor.go` + `drumview_update.go` (an `instImportCh`/handler mirroring `importCh`; a `wavRegistry` accessor backed by the instrument store), `internal/ui/instrument_export.go`/`instrument_import.go` (IO glue).
- Modify: `internal/i18n/{keys,catalog_en,catalog_es}.go`.
- Test: `internal/ui/instrument_menu_io_test.go`

**Design:**
- i18n keys: `KeyExportInstrument = "menu.export_instrument"` ("Export instrument…"/"Exportar instrumento…"), `KeyImportInstrument = "menu.import_instrument"` (…), `KeyInstrumentImported`, `KeyInstrumentExported`, `KeyWavVersionMismatch`, `KeyWavLocate`, `KeyReplace`, `KeyKeepBoth`. Add each to `keys.go` const + completeness list + both catalogs.
- `openSynthActionMenu`/`openSamplerActionMenu`: append `actionMenuItem{Label: i18n.T(i18n.KeyExportInstrument), OnSelect: func(){ dv.exportActiveInstrument() }}` and `{Label: i18n.T(i18n.KeyImportInstrument), OnSelect: func(){ dv.importInstrumentFromFile() }}`.
- `exportActiveInstrument()`: `b,_ := dv.BuildInstrumentBundle(dv.selRow)`; `raw,_ := json.MarshalIndent(b, "", "  ")`; reuse the export download path (`saveJSON`) with name `<instName>.beatmo-inst.json`.
- `importInstrumentFromFile()`: `selectJSONAsyncFn(func(data,name,err){ dv.instImportCh <- importResult{data,name,err} })`; drain in `drumview_update.go` (mirror `importCh` at `:117`): unmarshal → `ApplyInstrumentBundle` with a resolver that opens the collision confirm menu; publish a notification.
- Collision confirm reuses the existing confirm menu / `actionMenuComponent` (Replace / Keep both).

- [ ] **Step 1: Write failing test** — assert both menus contain the Export + Import items (labels); with a `SetSaveJSONForTest` capture, invoking Export writes a valid `InstrumentBundle` (`ValidateBundleHeader` passes) named `*.beatmo-inst.json`; with `selectJSONAsyncFn` stubbed to feed a known bundle, invoking Import registers the instrument (new id appears, effective params match).
- [ ] **Step 2–4:** implement wiring + i18n, run green.
- [ ] **Step 5:** commit (skip).

## Task 9: Retire/redirect the broken single-recipe export

**Files:** Modify `internal/ui/js_exports_synth_recipe.go` — make `exportUserRecipe` delegate to `audio.BuildInstrumentRecipeDoc` (so `BaseRecipe`+`ParamSeed` are populated) or route both JS exports through the new bundle path; update `wasm_bridge_smoke.browser.test.js` catalogue if the shape changes.

- [ ] Fix `exportUserRecipe` to emit a doc that `importUserRecipe` can round-trip; add a Go test asserting export→import reproduces effective params. Commit (skip).

## Task 10: Verify + docs

- [ ] Full fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ ./internal/userprefs/ ./internal/ui/ ./internal/i18n/`.
- [ ] Design guards: `-run 'TestDesignMDDrift|TestTokenDiscipline'` on `./internal/ui/`.
- [ ] Add the new file to `internal/deploy/manifest_drift_test.go` expectations only if a new runtime asset was referenced (none expected — pure Go).
- [ ] Update `AGENTS.md`/`CLAUDE.md` Import/Export section with the new `*.beatmo-inst.json` format.

## Self-review notes
- Spec coverage: format+normalized effects (T1,T2) · wav ref+hash+registry (T3,T7a) · decoupled store (T4) · migration (T5) · correct export (T6,T6a,T9) · import+collision (T7) · UI+IO+i18n (T8) · verify (T10). ✓
- Type consistency: `EffectDef`/`EffectChain`/`WavRef`/`InstrumentEQ` names used identically across tasks; `NormalizeEffects`/`DenormalizeEffects` signatures match T2↔T6/T7; `BuildInstrumentRecipeDoc` shared by T6+T9.
- Reuse: `RegisterUserRecipeFromBase`, `GetInsertEffects`/`SetInsertEffects`, `SampleEdit.Fields`/`FromFields`, `saveJSON`/`selectJSONAsyncFn`, `actionMenuItem`, userprefs atomic-write/localStorage primitives, `RecipeShippedDefaults` delta logic. No new parallel mechanisms.
- No-commit: repo convention — leave all work uncommitted; the `git commit` steps are placeholders only.
