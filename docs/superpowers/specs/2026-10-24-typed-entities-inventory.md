# Typed entities — inventory and blast radius

2026-10-24 · status: **recon for a design (nothing changed yet)**. Follows
`2026-10-23-registered-names-report.md` and its review.

## The goal

Every kind of named thing gets:

- a **model**: a struct holding its properties;
- a **typed ID**: `type X string` (or int);
- **one registry** that resolves an ID to its model.

Functions take the typed ID when they only refer to the thing, and take the
model when they read its properties. IDs coming from data (JSON, the JS bridge,
project files) are validated once, where they are loaded. Properties never come
from an ID's spelling. Closed sets get generated typed constants. Open sets
(user instruments, instance rows, sample ids) share the type but resolve
through the lookup.

## How this was measured

- **Signatures:** a `go/packages` type-checked scan of BOTH builds, the default
  one and `-tags test -modfile=go.test.mod`, merged and de-duplicated. Together
  they cover all 2,815 Go files. The scan lists every function parameter,
  result and struct field whose type is `string`, `[]string` or
  `map[string]…` and whose name looks like an identity (`id`, `instID`, `name`,
  `param`, `channel`, `refID`, …). Each one is classified to an entity by its
  name, then by its function name, then by its package.
- **Literals:** every string literal equal to a registry value. The values were
  exported live from the code through a `-overlay` file, so nothing touched the
  tree: 150 instruments, 67 module types, 1,321 ABI fields, 89 knobs, 25 voice
  blocks, 7 macro cards, 18 effect types, 32 effect params, 82 descriptors and
  70 references.
- **Registries:** a read-only agent survey, spot-checked.
- **Caveat:** value matching cannot tell meanings apart. A word that is both an
  instrument and a voice block (`snare`, `kick`, `tom`) is counted under every
  namespace it matches, so those are reported separately. The literal counts
  include the files that define the registries themselves.
- **Scripts:** in the session scratchpad only:
  `/tmp/claude-1000/-home-ymolinar-Repos-beatmo/f765103a-7a6c-4c11-a9ab-44ddf4e9bab2/scratchpad/entscan/`.

## What is already typed

There are about 100 named string or int types in the code. The established
ones show the pattern works here:

- `i18n.Key`: 802 constants, 1,762 uses.
- `model.NodeID`: 961 uses.
- `hooks.Kind`: 80 constants, with registry `ActionRegistry` and lookup
  `MetaFor`.
- `IconID`, `EffectType`, `PanelTab`, `CategoryID`, `Subject`, `ComponentID`,
  `scope.Stage`, `Locale`, `WidgetKind`, `env.Var`, and many small int enums.

The gap is the core audio domain, which is entirely plain strings.

## Inventory

**Prod sigs** counts production parameters, results and struct fields that
carry the ID as a plain string. **Call sites** counts calls to those functions.
**Prod lits** counts production string literals equal to a registry value
(unambiguous matches only). **Test lits** counts the same in test files.

### Tier 1: core audio domain (untyped today, the bulk of the work)

| Entity | Registry → model → lookup | N | Prod sigs (pkgs) | Call sites | Prod lits | Test lits | Notes |
|---|---|---|---|---|---|---|---|
| **Instrument** | `index.json` → `instrument.Doc` → `audio.InstrumentDoc` / `ShippedDoc` / `LibraryDoc` | 150 built-in + open set | **~690** (23 pkgs: ui 281, audio 187, cmd/refs 129) | ~2,700 | 908 (+~250 ambiguous) | 1,537 | Open set: roster, `user.<slug>.<hash>`, `<base>--N`, sample ids. `Doc.Category` = `"modular"` in all 150. Properties (category, kind, role, GM note) are guessed from spelling. |
| **Mixer channel** | `channels.go` (`mainChannelID="main"`) → `Channel` | instruments + master | 24 + the analyzer API | — | — | — | The ID is an instrument id or master. Master is spelled `"main"` in Go/JS channels and `"master"` in scope, analyzer, recording and the tour. |
| **Synth param (ABI field)** | `modularFields` → `modularField` (unexported) + `ParamDef` → `StageParamDefByName` | 237 rows / 1,321 flat | 145 (11 pkgs) | ~250 | ~1,430 (most in the registry files) | 1,697 | About 12 parallel tables keyed by param name: knob ranges/units, tiers, finish knobs, display, `plainEnglishMap` (keyed by LABEL), dial tuning, JS required fields. Wants ONE `Param` model. |
| **Synth knob** | `modularStageKnobs` (`"wave"` → `gen1_wave`) | 89 | 7 | 33 | 365 | 308 | A second name for a param. Model it as the param's short key, or fold it into Param. |
| **Module type** | `module_decls.go` → `instrument.ModuleTypeDecl` → `Registry.Type` | 67 | 2 | 14 | 117 (+~200 shared with block) | 31 | `"fx."+EffectType` is built by string concatenation. |
| **Voice block / family / stage group** | `modularFields.Block` (25), `modularFamilyStages` (6), `ParamDef.Group` (~22), `synthSectionID` (19 ints) | — | ~70 | ~145 | 155 + 135 shared | 40 | **One stage has four names**: module `amp_env`, group `env`, toggle `env_enabled`, section `Envelope`, plus copies in `synthdiag.Stages()` and `modularStageGroups`. **One family has four spellings**: `cymbal` / `cym` / `cym.*` / hat-ride-crash. This needs a modelling decision, not just a type. |
| **Numeric selectors** | `gen source` (11), `osc_type` (13), kick/snare/cym/tom/fm variants, `filter_type`, `lfo_target`, `body_model`, `formant_vowel` | ~10 enums | — | — | — | — | Float magic numbers spread over Go declarations, family rows, `sourceFamilyBlock`, stale `modularFields.Doc` and C if-chains. |

### Tier 2: catalog-backed and closed sets (medium)

| Entity | Registry → model → lookup | N | Typed today? | Gaps |
|---|---|---|---|---|
| Insert effect type | `insert_effects.go` → `EffectRegistration` → `EffectRegistrations()` | 18 | ✓ `EffectType` | `model.EffectOverride.Type` is a string. Copies in `builtinEffectTypes`, the audio.js switch, `WORKLET_INSERT_FX_TYPES` and C. 36 prod JS literals. |
| Effect param | `EffectRegistration.Params` → `EffectParamDef` | ~70 | ✗ | 272 prod literals in 51 files; `EffectSlot.Params map[string]float64`. |
| Effect category | registration `Category string` | 4 | ✗ | Hand-written order and display tables in `drumview_fx_panel.go`. |
| Send bus + params | separate functions; `sendFXParamDefs` | 2 / 6 | ✗ | `"delay.time_ms"` strings. |
| Instrument category | `category.go` → `Category` → `CategoryOf` | 15 + runtime | ✓ `CategoryID` | Assigned by SPELLING. `AssignCategory` exists and is unused. Clashes with `Doc.Category`. |
| Kit role | `roleHeuristic` switch | 10 | ✗ | No registry at all. `DrumRow.Role`, project `role`, `Kit.Members` keys. |
| Kit | runtime `kitReg` → `Kit` → `KitForID` | open | ✗ | JS `applyKit` / `createKit`. |
| GM drum note | `audio.MIDIMapping` + `gen-showcase gmKit` | ~35 | ✗ | **The two tables disagree**: rimshot → 37 in one, 37 → sidestick in the other. |
| Fingerprint descriptor | `fingerprint/registry_*.go` → `Descriptor` → `Registry.ByID` | ~54 registered / 73 consts | untyped consts | `thresholds.json` keys, `synth_dials.json` metric. `validate_terms.go` keys on display NAMES. |
| Descriptor group | untyped `GroupXxx` (display text) | 17 | ✗ | Baseline JSON keys on the display text. |
| Reference recording | `wavref/catalogue.json` → `wavref.Ref` → `Cat.Ref(id)` | 70 | ✗ | 42 hand ids in `check_f0.go`. Tier/Status untyped. |
| Score | `scoreref/catalogue.json` → `scoreref.Score` | 142 | ✗ | **No by-id lookup.** `TemplateSpec.ScoreID`. |
| Template | `assets/templates/*.json` → `assets.Template` | 83 | ✗ | **No by-id lookup.** Stems are repeated in 4–5 places (`template_specs.go`, `templateGroups`, `displayTitles`, `scoreref`, `wavref`). |
| Synth dial | `synth_dials.json` → `synthdial.Dial` → `synthDialByID(card, dial)` | 40 | ✗ | Label/Gloss are i18n keys stored as strings. `When` keys into `synthDialConditions`. |
| Macro / concept card | `synth_macro_groups.go` → `macroGroupDef` → `macroConceptFor` | ~19 | ✗ | Also referenced from `modularStageKnob.Macro` and dial `cards`. |
| Node type | `core/model/node_types.go` | 5 | ✓ `NodeType` (int) | A string in the project file and hooks payloads; `ui/import.go` compares `"invisible"` raw. |
| Logic kind | doc comment + aliases | 5 (+4) | ✗ | Hand lists in the sidebar, trigger logic and JS exports. |
| Groove kind | comment only | 2 | ✗ | |
| Chord quality | `core/model/chord.go` → `ChordQuality` → `ChordQualityByID` | 20 | id is a string | `exportChord.Quality`. |
| Sample-edit field | `sample_edit_descriptor.go` → `SampleEdit` | 9 | ✗ | prefs + project `map[string]float64`. |
| synthmatch family / refs vocabulary | `specMap`, `match_classes.go`, `familyDeepWords`, `tune_picks.json` | small | ✗ | Tool-local. The broad class duplicates the instrument category (F1). |

### Tier 3: UI (mostly typed; fix the places that get around the type)

| Entity | Typed | What gets around it |
|---|---|---|
| Panel tab | ✓ `PanelTab` | Slug parsing is written 3× (`setEQTab`, `SetActiveEQTab(string)` with 43 callers, `viewModeFromSlug`). |
| View mode | ✓ `viewMode` | Mirrors PanelTab + "pads". |
| Inst-menu mode | ✓ twice | `instMenuMode` (`drumview.go:35`) AND `InstMenuMode` (`drumview_overlay_inst_comp.go:115`). |
| Icon | ✓ `IconID` | `Button.Icon string`; 104 `string(IconID)` conversions. |
| Hooks event kind | ✓ `hooks.Kind` | `undo_action_labels.go` is keyed by the English label, not the Kind. |
| i18n key / locale | ✓ | prefs `Language` string; `NotificationRecord.Key` string; dial labels. |
| Screenshot subject | ✓ `Subject` | `AllSubjects()` misses a few. |
| Scene | ✗ (`Scene.Name`) | ~273 scenes; `RunScene(name)` is a linear scan. |
| Theme token / category / preset / swatch | ✗ | `genColorByName` (148), 9 categories, 6 presets, 35 swatches (+ the hand copy `canonicalSwatchNames`). `theme.Doc` maps are keyed by plain string. |
| Design component | ✓ `ComponentID` | variant names untyped. |

### Out of scope (already fine)

The small internal int enums (`GestureKind`, `CommitKind`, `log.Level`, …),
`env.Var`, density and screen class.

## Boundaries where an ID enters as a string (validate once here)

- **JS bridge:** 66 `args.Str(i)` reads in `js_exports_*.go` (16 named `id`).
- **Project file:** `instruments[].id`, `instrument_docs` keys, `role`, node
  `type`/`logic_kind`, effect slots, `sample_edit` keys, `theme` maps.
- **Instrument bundle and user library:** `user.*` ids, effect chains.
- **Prefs:** favorites (instrument ids), `knob_steps` (param names), `language`,
  `sample_edits`, theme.
- **Embedded data:** `index.json`, `synth_dials.json`, `catalogue.json` (wavref,
  scoreref), templates, `thresholds.json`.
- **Tool data:** `tune_picks.json`, `ear_verdicts.jsonl`, `ear_sessions.jsonl`.
- **Go ↔ JS/C name tables:** effect types and params, the param ABI (already
  generated), EQ bands, the melodic set (already pushed from Go).

## Found along the way (small, independent)

1. The two General MIDI tables disagree on note 37.
2. Master channel is `"main"` in channels/JS and `"master"` in scope, analyzer,
   recording and the tour.
3. `src/js/audio_config.generated.js` is dead; only `eslint.config.js` mentions
   it.
4. Duplicate types `instMenuMode` / `InstMenuMode`.
5. `InstrumentConfigs` is unreachable for every roster id (the document is read
   first), so all three build-tag copies can go.
6. `Doc.Category = "modular"` while `CategoryID` exists and is assigned by
   spelling (bassoon → Bass, bells → Lead, 26 → Other incl. marimba,
   vibraphone, xylophone, tuba, piccolo).
