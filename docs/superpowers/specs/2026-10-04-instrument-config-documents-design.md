# Instrument config documents — every instrument owns every value

**Status:** design v2, 2026-10-04. v1 was approved section by section in chat. v2 adds the review decisions accepted the same day: modules present only when used, one source rule, layer slot limits, `Init` values used only at creation, a migration runner, notes, one embedded pack, soft ranges, and the 1a–1d split.
**Implementation plan:** `docs/superpowers/plans/2026-10-04-instrument-config-documents.md`.
**Supersedes:** the Phase D plan `docs/superpowers/plans/2026-10-04-config-only-synth-engine-phase-d.md` (engine literals as table defaults). It was rejected because it moved hardcoded values into a defaults layer instead of into instruments.
**Roadmap context:** `docs/superpowers/plans/2026-10-02-config-only-synth-engine.md`. Phases A–C are done: a double-precision param block, the legacy engine deleted, one generated ABI table, a strict C engine, and the FINISH stage.

## 1. Intent

**What the user asked for (2026-10-04):**

- "The idea is the user is in complete control of every piece that makes up the synth instrument."
- "Why do we need defaults if we know the values already? I'm the only user so do not worry about backwards compatibility."
- Choice **C**: tune every number now, with controls grouped by the voice's real building blocks. Building instruments from parts comes later: "would be very cool to grab a set of modules from other instruments. Grouping synth steps and parameters makes a lot of sense."
- "Please simplify the codebase so that each instrument owns every value in its config."
- "Aim to simplify and compose and be as purely functional as possible for the configs and audio paths. … make sure adding new synth steps, configs, blocks, effects, is easy and simple. In the future, we can create a plugin system so users can publish and share their own synths or effects."
- Project export writes plain JSON. The game's templates are embedded compressed.
- Copies of instruments live in templates and projects so their sound is deterministic.
- Keep config versions and the schema so backwards-compatible changes become possible **after** the refactor. Nothing made before or during the refactor is migrated.

**Success:**

1. An instrument is ONE document holding every value its synth voice depends on. There is no default, identity fill, overlay or recipe layer anywhere on the sound path.
2. Every shipped instrument and template sounds bit-identical before and after the switch, **on each platform**. Today's platform differences are kept on purpose (§8, "Kept divergences").
3. Adding a synth step, voice or (later) effect is one declaration plus its DSP. **No existing document has to change.**
4. Config and audio paths are pure functions over immutable documents.
5. Project files stay readable and manageable, and the embedded assets fit the WASM budget.

## 2. Decomposition

| # | Sub-project | Delivers | Sound changes |
|---|---|---|---|
| **1a** | **Model + proof** | `internal/instrument` (documents, registry types, pure functions, migration runner); built-in declarations; 150 generated documents; the proof that they render like today. No runtime switch. | None |
| **1b** | **The switch** | The embedded pack; the project document store and user library; one doc-based voice path on desktop; `setInstrumentBlock` in the browser (JS param maps deleted); the Synth tab's flat-field adapter; Save / Save As / Reset / Restore shipped; project `instrument_docs`; bundle v2; loudness amplitude in documents. | None, except the instruments the user approves from the 1a audit (§7.2) |
| **1c** | **Deletion + tools** | Dev tools and secondary consumers ported; the recipe layer, seeds, identity column and generated JS table deleted; docs updated. | None |
| **1d** | **Templates** | Templates reference pack documents by hash. | None |

**Why the switch is one slice:** the overlay, the browser's two param maps, the recipe defaults and the project delta format all feed each other. Any one switched alone would need a compatibility shim that is deleted a slice later. Inside 1b each task has its own green tests, and the full suites gate the end of the slice.
| 2 | Engine number lift (re-aimed Phase D) | Each hardcoded C voice number becomes a value of its module type. It is added to every document by a migration step (§9). | None (bit-identical) |
| 3 | Synth tab by modules | UI metadata (label, unit, range, tier, gloss) moves from `ModularSynthParamDefs` into the declarations. The Synth tab is grouped by module. The `SetField` adapter is deleted. | — |
| 4 | Module copy between instruments | "Grab modules from another instrument" UI over `CopyModules`. | — |
| later | FX lift; sample documents; plugin system | Voice FX in the browser; WAV instruments as documents; declarations loadable from files. | — |

**Why the UI metadata moves in 3, not 1:** the Synth tab reads `[]ParamDef` in about 25 files (`synth_panel_zone.go` alone uses ten metadata APIs). In sub-project 1 it keeps reading `[]ParamDef`, now derived per instrument from its document (§6). That keeps sub-project 1 free of UI changes, and the grouped tab of sub-project 3 replaces the metadata wholesale.

## 3. The instrument document

```json
{
  "schema_version": "0.0.1",
  "id": "clap-bright",
  "name": "Bright Clap",
  "category": "Modular",
  "notes": "Shared clap voice; a 4 kHz low-pass and a 50% gate make it bright and short.",
  "render": { "beats": 0.25, "seconds": 0.45, "amplitude": 0.8, "gate_tail": 0.5, "melodic": false },
  "stages": {
    "core":     { "v": 1, "on": true, "values": { "pitch": 0, "osc_octave": 0, "osc_detune": 0, "voice_freq_hz": 0, "gain": 1, "noise_draws": 1, "noise_prelude": 0 } },
    "amp_env":  { "v": 1, "on": true, "values": { "amp_attack": 0.005, "amp_decay": 0.3, "amp_sustain": 0.6, "amp_release": 0.2, "amp_curve": 1 } },
    "finish":   { "v": 1, "on": true, "values": { "post_pitch": 0, "post_decay": 1, "post_decay_rate": 8, "post_body": 0, "post_brightness": 0, "post_darken": 0, "post_drive": 0, "out_clamp": 1.5 } }
  },
  "layers": [
    { "type": "clap", "v": 1, "on": true, "values": { "gain": 1, "snare_tune": 1, "snare_noise_d": 30, "snare_tail_d": 8, "snare_noise_m": 1, "snare_attack": 0.3 } }
  ],
  "fx": [ { "type": "fx.filter", "v": 1, "on": true, "values": { "mode": 0, "cutoff": 4000, "q": 0.707, "mix": 1 } } ]
}
```

(Values are illustrative. A real document lists every value of every module it holds.)

**Containers:**

| Field | Holds | Rule |
|---|---|---|
| `stages` | Pipeline stages, keyed by type | One per type. `core` and `finish` are **required** and always on. Every other stage is present only when the instrument uses it. |
| `source` | The oscillator, one module | Optional. Its type picks the DSP: `osc.wavetable`, `osc.noise`, `osc.fm`, `osc.bowed`, `osc.brass`, `osc.reed`, `osc.flute` or `osc.sax`. |
| `layers` | Generator slots, an ordered list | List index = engine slot (`gen<index+1>_*`). `null` = an empty slot. Each type has a **slot limit** (§4). |
| `fx` | The voice's own render-time effects | Ordered. These are rendered into the voice buffer, like today's `DefaultFX`. They are NOT the row's live insert chain. |
| `render` | Render metadata | `beats`, `seconds`, `amplitude` (the effective value, loudness included), `gate_tail`, `melodic`. |

**Rules:**

- **Module shape.** `stages` entries are `{v, on, values, notes?}`. `source`, `layers` and `fx` entries add `type`.
- **Present when used.** An absent module is not in the signal path: `Flatten` writes nothing for it, and the engine skips a stage whose enable field is 0. **Adding a new module type therefore never invalidates an existing document.**
- **`on: false`** means off, values kept. `Flatten` writes nothing for an off module, so switching it back on restores its values.
- **Completeness:** a present module lists exactly the values its type declares — none missing, none extra.
- **`notes`** (document and module) carry tuning provenance as plain text. The generator copies today's Go seed comments into them.
- **`origin`** (project copies only) names the library entry the copy came from. Save, Reset and the dirty dot use it.

**Not in the document (sub-project 1), on purpose:**

- **The row's live insert-effect chain, EQ, pan, sends and the Sampler edit.** They are channel state, keyed by instrument id, and apply to WAV instruments as well as synths. They stay row-level in projects, exactly as today. Making WAV instruments documents is a later sub-project.
- **`noise_seed`.** No caller ever renders with a variant other than 0, so the engine field stays 0 and no module owns it.

## 4. Module types (the registry)

```go
type ModuleTypeDecl struct {
    Type     string      // "kick.punchy"
    Version  int         // 1 until a migration step exists (§9)
    Kind     Kind        // KindStage | KindSource | KindLayer | KindFX
    Required bool        // core, finish
    Enable   *FieldSet   // field written when ON ({filter_enabled 1}, {source 5}); nil = no switch
    Set      []FieldSet  // constants the TYPE writes when on (osc_type 4, kick_variant 2) — slot-relative for layers
    Global   []FieldSet  // absolute constants a layer type writes when on (kick_enabled 1)
    Capacity int         // layers: highest slot count the C engine serves for this voice (kick 1, cymbal 3); 0 = all 12
    Values   []ValueDecl
}

type ValueDecl struct {
    Name    string    // ABI field name; for layers the slot-relative name ("kick_click" → gen<k>_kick_click)
    Init    float64   // value a NEWLY CREATED module starts with — never read for an existing document
    Allowed []float64 // non-empty = the value must be one of these (osc.wavetable osc_type: 0,1,2,3,12)
}

type FieldSet struct { Field string; Value float64 }
```

- **The value name IS the ABI field name.** There is no `Field` column or name templating. Layers add the slot prefix. Friendlier names can come later as a rename migration.
- **One type per DSP function.** A variant becomes its own type only when it runs a different C function: kick ×9, snare ×4, cymbal ×6. When a variant only picks constants from a table, it stays a value (`tom_variant`, `fm_variant`); sub-project 2 deletes those.
- **Slot limits are real.** `modular_gen_bank_render` silently skips a family voice at `k >= MODULAR_<FAMILY>_SLOTS`. `Validate` rejects a layer past its type's `Capacity`, so a copied kick can never become silence.
- **`Init` is a creation value, not a default.** It is read only when a module is created: adding a stage, switching a source or layer type, or showing an absent stage in the Synth tab before it is touched. Reading or rendering a document never consults it. In sub-project 1, `Init` comes from today's `ModularSynthParamDefs` defaults; sub-project 3 moves it into the declarations together with the rest of the UI metadata.
- **Ranges are not in the registry in sub-project 1.** They stay UI hints in `ParamDef`, and the audio wrapper clamps knob edits as today. `Validate` checks only structure, finiteness, `Allowed` and capacity, so widening or narrowing a knob range never breaks a saved file.
- **Field sharing is normal.** Every layer type reads `gen_wave/freq/freq_mode`, and the clap reads the snare's fields. The coverage guard is "every ABI field is named by at least one declaration" (or is `noise_seed`), not "exactly one".
- **Adding a synth step or voice** = one `ModuleTypeDecl` + the C code that reads its fields + `make gen-synth-abi`.
- **Plugins:** declarations can only recombine fields the C struct has. A plugin cannot add DSP or ABI fields, so the declaration format is not shaped for that.

## 5. Pure functions (package `internal/instrument`, no project imports except `internal/schema`)

| Function | Contract |
|---|---|
| `Validate(doc, reg) error` | Unknown type/value, missing or extra value, a missing required stage, a stage key ≠ a stage type, a layer past its capacity, a non-finite number, a value outside `Allowed`, a module `v` ≠ its type's version. Errors name the path: `layers[0](kick.punchy).values.kick_click: missing`. |
| `Flatten(doc, reg) []float64` | Zero block. Each present, ON module writes its values, its `Set`/`Global` constants and its `Enable` field. Nothing else is written. |
| `FieldValues(doc, reg) map[string]float64` | UI adapter (sub-project 1 only). Every ABI field's `Init`, overlaid by every present module's values (on or off), with each enable field showing the module's on/off state. |
| `SetValue(doc, path, v)` / `SetField(doc, reg, field, v)` | Return a new document. `SetField` is the sub-project-1 adapter for flat field names: it toggles an enable field, switches a type, materialises an absent stage from `Init`, or sets a value. |
| `CopyModules(dst, src, refs)` | Copies modules (sub-project 4's backend). It respects capacity. |
| `Diff(a, b) []string` | Changed paths. Drives the dirty dot (project copy vs its `origin`). |
| `BlockHash(block) uint64` | Voice-cache key (FNV-1a over the float64 bits), XOR the sample-edit hash as today. |
| `ContentHash(doc) string` | `sha256:<16 hex>` over the canonical JSON. Template references use it. |
| `Migrate(doc, reg) (Doc, error)` | Applies each module's registered steps from its `v` to the type's version (§9). |

**Documents are immutable values.** A mutation always returns a new document that replaces the old one. Undo is the previous document, and the audio path is `render(Flatten(doc)) → fx → gate → normalize`.

## 6. Library, project and editing

- **Library** = the shipped documents (read-only, in the embedded pack) plus the user's saved documents (`instruments.json` on desktop, localStorage `beatmo.instruments` in the browser). A saved document with a shipped id shadows the shipped one in the instrument menu.
- **A project owns copies.** Picking an instrument copies its library document into the project's document store under the row's instrument id, with `origin` = the library id. Rows stay keyed by instrument id, as today, so instance ids (`voice-ahh--2`) each get their own copy.
- **Edit:** a knob runs `SetField` on the project copy. The UI's `ParamDef` list for an instrument is derived from its document (`Default` = `FieldValues`, ranges widened to contain the value). Gestures still produce one undo step each.
- **One visibility rule:** the Synth tab hides a knob whose value the instrument's source or layer type does not declare (`InstrumentDeclaresField`). It replaces four hand-written predicates (family, oscillator model, cymbal modal, kick physical). Stage knobs stay visible even when the stage is absent, dimmed as today. This is sub-project 1's only intended visual change: knobs that never moved the sound disappear (e.g. `kick_h3` on the punchy kick, FM operators on a non-FM source).
- **Save** writes the project copy to the library under its `origin`. **Save As** writes it under a new content-addressed id `user.<slug>.<6 hex>` and repoints `origin`. **Reset** replaces the copy's values with its `origin`'s. **Restore shipped** (⋯ menu) deletes the user's library entry for a shipped id.
- **Saving changes future picks only.** Projects already using an instrument keep their own copy.

**Deleted (sub-project 1):**

- `ModularParamSchemaIdentity`, the `Identity` column, and `newModularBlock`'s identity fill.
- The recipe registry and its defaults: `RecipeDefaultParams`, `recipeShipped`, `UpdateRecipeDefaultsAndInvalidate`, `ResetRecipeToShipped`, `RecipeDefaultsCustomized`, `BuildBuiltinRecipeDocs`.
- The per-instrument overlay and merge: `instrumentParamsMgr`, `MergeRecipeDefaults`, the overlay cache behind `MergedInstrumentParamsRO`.
- The instrument↔recipe binding and `RecipeForInstrument`.
- `user.*` / `clone.*` recipes, `RegisterUserRecipeFromBase`, `CloneInstrument` / `DeleteInstrument`.
- The delta export (`synth_params` vs shipped), `projectFile.Recipes`, and `RecipeDoc`.
- `ApplyUserRecipeOverrides`.
- The two native voice paths (baked fast path + recipe path), merged into one.
- `bakedModularRender`.
- The Go seeds (`instrument_seeds*.go`) and `ModularInstrumentDef.Seed`, which become the generated documents.
- `modular_instruments.gen.js`, `MODULAR_PARAM_IDENTITY`, and the JS `instrumentDefaultParams` / `instrumentSynthParams` / `seedInstrumentDefaults` / `updateInstrumentParams` / `updateInstrumentRecipe` / `instrumentParamsFor` / `identityParamBlock`.

## 7. What the code does today that this fixes

Found while mapping the current paths. Each one disappears because there is one document and one render path:

1. **Two native render paths disagree.** An unedited instrument plays the baked block: identity plus the raw seed, then its `DefaultFX`, then `gateTail`. The moment any knob is edited it switches to the recipe path: ParamDef defaults plus the seed clamped into each knob range, with **no `DefaultFX` and no `gateTail`**. So touching one knob on `clap-bright` removes its filter and its gate.
2. **The browser default is the clamped recipe layer** (`seedInstrumentDefaults` pushes `RecipeDefaultParams`), while native plays the raw seed. Wherever a seed lies outside its knob range, or a live unseeded field's ParamDef default differs from its identity, the two platforms render different voices. Plan Task A6 lists every affected instrument before anything switches.
3. **Bundle `InstrumentDoc.SynthParams` and the store's `sample_edits`/`wav_registry` are dead fields.** Nothing writes or reads them.
4. **Reset never deletes the persisted Save**, so a restart brings the old Save back.

## 8. Engine boundary

- **Go is the only place that builds a block (`Flatten`).** Desktop renders it with `renderModularBlock`.
- **One native voice path:** `render(Flatten(doc))` → `applyDefaultFX(doc.fx)` → `gateTail(render.gate_tail)` → `normalizeAndScale(render.amplitude)` → Sampler edit (if any). For pitched renders, `pitch` is overwritten with the node's rounded pitch, as today. The voice cache keys on `BlockHash`.
- **The browser receives the block per instrument:** `setInstrumentBlock(id, Float64Array, {seconds, amp, melodic})`. It renders it as-is and patches the `pitch` index for melodic per-pitch renders. `audio.js` keeps no parameter maps.
- **The C engine is unchanged.** Fields no present module owns are 0. The proof (§10) shows none of them is read.
- **Kept divergences** (each platform stays bit-identical to itself):
  - The browser applies neither `fx` nor `gate_tail` to synth voices today, and still won't. That is fixed with the FX lift.
  - Native rounds nothing; the JS path does no float32 rounding either. That is unchanged.

## 9. Versions

- **Files carry `schema_version`; every module carries `v`** (its type's version). Readers reject anything not current, naming the version and the path.
- **No migrations of pre-refactor data.** Old projects, library entries and templates are regenerated or dropped, per the user's instruction.
- **The runner exists from 1a:** per type, `Steps []func(Values) Values`, step *i* taking version *i* to *i+1*, applied on read before validation. Reading never rewrites the stored file; saving writes current versions.
- **Sub-project 2 is its first user.** Each lifted literal is one step ("add `kick_click_decay` = 220 to every `kick.punchy`") — bit-identical by construction, and the user's library survives the lift.
- **The baseline** — the versions after which compatibility is promised — is set when sub-project 2 lands.

## 10. Proof and tests

- **Default renders:** for all 150 documents, `render(Flatten(doc))` → fx → gate is **bit-identical** to the baked-path render of today (`TestDocumentsRenderLikeTheBakedSeeds`). After 1b, the parity tool's default dump is bit-identical to `~/.beatmo/render-parity/Ffix2`.
- **Zeroing unowned fields is inaudible:** for every document, every present value at min, max and zero, and every absent optional stage switched on from `Init`: `render(Flatten(d'))` == `render(identityFill(FieldValues(d')))` (`TestFlattenMatchesIdentityFilledBlockEverywhere`).
- **Declarations are honest:**
  - every declared value is live in at least one shipped document of its type, or is listed with a reason in `testdata/dead_module_values.txt` (a ratchet);
  - every ABI field is named by at least one declaration (or is `noise_seed`);
  - every shipped document validates;
  - shipped values lie inside their `ParamDef` ranges, or the derived range widens to contain them.
- **Unit tests** cover every pure function, table-driven, against a tiny fake registry.
- **Templates:** every template renders identically from the pack and from today's JSON (master-mix identity, `make test-real`).
- **Export→import master-mix identity** is adapted to the new files, including Save / Save As / clean machine.
- **Browser:** `xplat_audio_compare` (native == WASM for a pushed block), `synth_save_export_import`, `import_synth_render`, `modular_synth_audible`, plus the bridge catalogue.
- **Screenshots:** the Synth tab keeps its look in sub-project 1, apart from the dead knobs removed by the visibility rule (§6). Desktop and mobile captures of the synth scenes are compared before and after. The ⋯ menu's new "Restore shipped" item gets its own capture.

## 11. Out of scope for sub-project 1

- Lifting C literals (sub-project 2).
- Moving UI metadata into the declarations and grouping the Synth tab by module (3).
- Module copy UI (4).
- Voice FX and gate in the browser (FX lift); WAV instruments as documents; the plugin loader.
- The insert-FX engine ABI. Only the voice-FX document shape changes.
