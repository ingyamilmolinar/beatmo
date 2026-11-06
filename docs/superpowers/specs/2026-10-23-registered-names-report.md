# Registered names spelled by hand — analysis and centralisation proposal

2026-10-23 · status: **analysis + proposal (nothing changed yet)**

## Why

Instruments, families, voice blocks, module types, ABI params and reference
recordings each have ONE place where they are registered. The code, though,
mostly refers to them by hand-typed strings: `"oboe"`, `"snare"`,
`"filter_cutoff"`, `"oboe-A4"`. Nothing ties a literal to its registry, so when an
instrument is renamed, added or removed, the code that names it neither stops
compiling nor fails a test. A table entry just goes dead, a classifier guesses
wrong, or a fixture quietly tests a different sound.

The goal is to register each name in exactly one place, have code refer to it
through a generated constant, have data refer to it by string but get validated
where it is loaded, and never classify anything by its spelling.

## How this was measured

- **Registries:** the live lists, read from the code itself rather than
  re-typed: `audio.BuiltinInstrumentIDs` (from `index.json`, **150** ids),
  `audio.BuiltinModuleRegistry().Types()` (67 module types),
  `audio.ModularParamNames()` (1,321 ABI fields),
  `fingerprint.DefaultRegistry().All()` (82 descriptors), and
  `wavref.Catalogue()` (70 reference recordings).
- **Go:** every string literal in `src/go` (not `testdata`) parsed with
  `go/ast`, matched exactly against those lists, and classified by syntactic
  context (switch case, comparison, map key, struct field, call argument and
  callee, list element), by test vs production, and by generated vs hand-written.
- **JavaScript:** `src/js/**/*.js` scanned with a regex for quoted strings.
- **Near misses:** a second pass looked for id-shaped literals
  (`word-word`) that name no registered thing, in files that hold instrument
  tables, to catch stale names.
- **Classification:** the shipped `CategoryOf` / `RoleForInstrument` were run
  over the whole roster.

Scripts and raw output (not in the repo):
`/tmp/claude-1000/-home-ymolinar-Repos-beatmo/7dbbf5b1-f879-4730-8981-1772282b87a4/scratchpad/idscan/`
(`scan.go`, `stale.go`, `export_main.go`, `classify_main.go`, `hits.json`,
`classes.tsv`).

## The numbers

Literals equal to a registered name, outside the files that define the
registry itself:

| namespace | registry (source of truth) | production uses | files | test uses | test files | constants today |
|---|---|---|---|---|---|---|
| instrument id | `internal/assets/instruments/index.json` → `instrument_ids_gen.go` (a list only) | **1,086** | 38 | 3,279 | 425 | none |
| ABI param | `modularFields` (`internal/audio/modular_fields.go`) → generated C header + JS | **1,347** | 46 | 1,762 | 163 | none in Go |
| module type | `builtinModuleDecls` (`module_decls.go`) | **291** | 26 | 127 | 41 | none |
| reference id | `internal/wavref/catalogue.json` | 42 (all in `cmd/refs/check_f0.go`) | 1 | 153 | 19 | none |
| descriptor id | `fingerprint` (`IDNoiseFraction = "noise.fraction"`, …) | **0** | 0 | 11 | 4 | **yes** |
| JS instrument ids | — | 15 | — | 379 | ~60 | — |

Descriptor ids are the model to follow: they are constants, and no production
code outside the registry spells one.

**No stale instrument id is in production code today.** Every near miss is a
deliberate fixture (`kick-a`, `snare-cat-1`) or a case label
(`kick-identity`). Whether a table still matches the roster depends on whoever
edits it remembering to.

## Findings, most severe first

### F1 — Instruments are classified by their SPELLING, and it is visibly wrong today

All 150 documents carry `"category": "modular"`, a field that tells you
nothing, so the real category is guessed from the id by four separate pieces of
code:

- `CategoryOf`: `categoryByText` (substring) → `categoryByRole` (from
  `roleHeuristic`), in `internal/audio/category.go`. It feeds the instrument
  menu's category tabs (`drumview_instruments.go:98`).
- `roleHeuristic` / `RoleForInstrument` (`kit.go:180`, substring): a kit maps
  roles to rows with it.
- `synthCategory`, written twice (`catalog_desktop.go:263` and
  `catalog_stub.go:285`, one per build tag), prefix matching.
- `SampleSeconds` (`engine_play.go:245`): a switch on six ids.

Run over the roster, they produce these errors today:

| instrument | gets | why |
|---|---|---|
| `bassoon` | **Bass** tab, kit role `bass` | contains "bass" |
| `tubular-bells`, `fm-bell`, `fm-bell-soft` | **Lead** | contains "bell" |
| `double-bass` | Bass (arguably Strings) | contains "bass" |
| 26 instruments (sitar, koto, gamelan, celesta, music-box, accordion, harmonium, theremin, steel-drum, singing-bowl, glass-wash, glass-armonica, pwm-pad, modular-pad, …) | **Other** | no substring matched |

Three more hand-kept classifications sit beside them and disagree with them:

- **`cmd/refs/match_classes.go`:** a broad kind for all 150 ids (152
  literals). It has a coverage guard, `TestEveryRosterInstrumentHasABroadClass`,
  but it is a parallel copy, and it puts `tubular-bells` under *idiophone* while
  the UI puts it under *Lead*.
- **`MIDIMapping` (`capture_event.go`):** General MIDI note per id (17 ids).
- **`synthmatch.specMap` (`tunable.go`):** a tuning "family" per id (12 ids).

Every new instrument has to be added to some of these by hand, and none of them
can say "I don't know this id".

### F2 — Boundaries that accept an unknown name silently

- **`DrumView.SetInstrument`** discards the error from
  `EnsureInstrumentLoaded`. The 170 instrument literals in
  `internal/ui/scene_catalog.go`, used to set up scenes such as
  `synthTabSceneSetup("oboe")`, would accept a misspelt or renamed id, and the
  scene would quietly show some other row.
- **Tables keyed by id with no key validation:** a renamed instrument's entry
  goes dead and the code falls back to a default. These include:
  - `familyDeepWords` (`session_questions.go`): written this week; its test
    requires the picks to have an entry but does not reject an unknown key;
  - `gateExemptions` (`check_candidates.go`);
  - `specMap` (`tunable.go`);
  - `MIDIMapping` (`capture_event.go`);
  - `i18n` display names (`internal/i18n/instruments.go`, 5 ids).

  (`synth_focus_graph.go`'s `"kick"`/`"snare"`/`"tom"`/`"bass"` keys are
  voice-block GROUPS, not instrument ids, and
  `TestSynthFocusRenderer_EveryGroupExplicit` already guards them; F3's
  ambiguity is why they look the same. `plain_english.go`'s are knob labels.)
- **By contrast, these fail loudly**, which is the pattern to copy:
  - `gen-showcase`'s `pinInstrument`: "template row %q: %q is not a shipped
    instrument";
  - `TestTunePicksAreValid`;
  - `TestCatalogue_EveryReferencedInstrumentExistsInRoster`;
  - the `index.json` drift test.

### F3 — One spelling, several namespaces

`"snare"` is all of these at once:

- an instrument id;
- an audition family key (families are named by their base instrument);
- a C voice block (`modularFields` `Block: "snare"`, and `slotsOf("snare")`
  in `module_decls.go`);
- a prefix of module types (`snare.main`, `snare.rimshot`);
- a kit role;
- a UI category name (`CatSnare`);
- an i18n key.

`"tom"` and `"clap"` are both instrument ids and module types, and `"drive"` is
both a module type and an ABI param. A grep, a rename or a reviewer cannot tell
which one a literal means. These are the namespaces that exist:

| namespace | example | registered in |
|---|---|---|
| instrument id | `rimshot`, `kick-808` | `index.json` + `instruments/<id>.json` |
| audition family key | `cello` (members cello, cello-warm) | DERIVED: `buildAuditionFamilies` (union-find over the catalogue), named by its base id |
| voice block (C family) | `snare`, `kick`, `tom`, `sax` | `modularFields` `Block` |
| module type | `snare.rimshot`, `filter`, `osc.reed` | `builtinModuleDecls` |
| ABI param | `gen1_snare_tone2`, `filter_cutoff` | `modularFields` |
| UI category | `CatBass`, `CatLead` | `category.go` constants (assigned by spelling, F1) |
| kit role | `kick`, `hat`, `bass` | nowhere; `roleHeuristic` returns literals |
| match kind | `idiophone`, `plucked` | `match_classes.go` map keys |
| tuning family | `subtractive-reed`, `additive-organ` | `synthmatch.specMap` values |
| reference id | `oboe-A4`, `rimshot-vcsl` | `catalogue.json` |
| descriptor id | `noise.persistence` | `fingerprint` constants ✓ |

### F4 — The same table copied once per build tag

- **`InstrumentConfigs`** is defined three times:
  - `config.go` (desktop): 2 entries, `modular` and `modular-pad`;
  - `config_wasm.go`: 2 entries;
  - `config_stub.go` (`-tags test`): 19 entries.

  Its comment says the values "must match RENDER_INFO in audio.js", but
  `RENDER_INFO` is now filled from Go's documents, so that comment is stale.
  Production reads the document first, which makes the table a fallback.
  The stub's 19 hand-typed values disagree with the documents (rimshot amplitude
  0.8 against the document's 0.912).
- **`synthCategory`** has a desktop copy and a stub copy (F1).

### F5 — Large hand-written instrument lists in tools

| file | literals | what | guarded? |
|---|---|---|---|
| `cmd/gen-showcase/template_specs.go` | 386 | kit maps, part → instrument | yes, at generation (`pinInstrument`) |
| `internal/ui/scene_catalog.go` | 170 | scene setups | **no** (F2) |
| `cmd/refs/match_classes.go` | 152 | broad kind per id | coverage test; duplicates F1 |
| `cmd/export_audio/main.go` | 60 | cross-platform render cases | indirectly (the export fails on an unknown doc) |
| `cmd/refs/match_nuisance.go` | 47 | voices for the nuisance sweep | unverified |
| `cmd/refs/check_f0.go` | 42 reference ids | f0 calibration set | unverified |

These lists are deliberate choices, so they should stay lists. They should name
their members through constants, so that a rename breaks the build.

### F6 — ABI params and module types are generated for C and JS, but spelled by hand in Go

`cmd/gen-synth-abi` already turns `modularFields` into the C header and the
JS ABI. Go itself still spells the fields by hand: 1,347 literals in 46 files,
led by:

- `modular_recipe.go`: 203;
- `ui/synth_macro_groups.go`: 203, the Synth tab's category tables;
- `module_decls.go`: 137;
- `export_audio`: 118;
- `modular_stage_params.go`: 97;
- `ui/synth_param_display.go`: 88.

Module types: 291 literals in 26 files (`modular_fields.go` 84,
`modular_recipe.go` 74, `clone/` 23, `synth_focus_graph.go` 11). Write-boundary
validation (`SetInstrumentParam` rejects unknown names) catches a typo at run
time, but only on the path that actually runs it.

### F7 — Tests

There are 3,279 instrument literals in 425 test files and 1,762 param literals
in 163. Tests have every reason to name instruments, so the problem isn't the
naming itself. It's that a rename turns a test into one about nothing, or about
a different sound, instead of a compile error. Constants fix that at no cost to
readability (`audio.ID.Oboe` against `"oboe"`).

### F8 — Documentation already drifted

CLAUDE.md says "133 roster ids" / "all 133 roster ids" in several places; the
roster is 150. The counts should be generated or dropped.

## Proposal

**Principle.** Each kind of name is registered in exactly one place:

- **Code** refers to it through a **generated constant**, so a rename or a
  removal breaks the build.
- **Data** (JSON tables) refers to it by string and is **validated where it is
  loaded**, so a stale entry fails loudly.
- **Properties** (category, role, GM note) live in the **registry entry**,
  never in an id's spelling.

### P1 — Generated instrument-id constants (one leaf package)

Extend the generator that already reads `index.json` (`gen_instrument_ids.go`,
`go generate ./internal/audio/`) to also emit a new leaf package,
`internal/instrumentid`, with no imports, so `i18n`, `ui`, every `cmd` tool and
every test can import it without a cycle:

```go
// Code generated from internal/assets/instruments/index.json; DO NOT EDIT.
package instrumentid

const (
	Kick       = "kick"
	Kick808    = "kick-808"
	FrenchHorn = "french-horn"
	Oboe       = "oboe"
	// … one per roster id, in index order
)

var All = []string{Kick, Kick808, /* … */}
```

- The Go identifier comes from the id (`fm-epiano` → `FmEpiano`, `bass-808` →
  `Bass808`), and the generator fails if two ids map to one identifier.
- `audio.BuiltinInstrumentIDs` becomes `instrumentid.All`, or keeps its name and
  points at it.
- The existing drift test (`instrument_ids_drift_test.go`) already catches a
  stale generation.
- Start with untyped string constants, which drop into every existing
  `string` API. Moving to a named type (`type ID string`) at the API boundaries
  would also stop a module type or param name being passed where an instrument
  id is expected. That's a later, larger step.

### P2 — Properties move into the registry, and the classifiers go away

Give each roster entry its properties as data. That can live in `index.json`, or
as document fields next to `render`, replacing the empty `"category":
"modular"`:

```json
{ "id": "bassoon", "category": "woodwind", "kind": "reed", "role": "", "gm_note": 0 }
```

Each property's allowed values get their own table, from which constants are
generated (`CatWoodwind`, `KindReed`, …), and a document is validated against
those tables. Then:

- `CategoryOf`, `roleHeuristic`/`RoleForInstrument`, both `synthCategory`
  copies and `categoryByText`/`categoryByRole` read the document. The
  spelling-based code is deleted.
- `match_classes.go` reads `kind` instead of keeping its own 150-id copy.
  `TestEveryRosterInstrumentHasABroadClass` becomes "every document declares a
  valid kind".
- `MIDIMapping` reads `gm_note`.
- `SampleSeconds` reads the document's render length.
- Adding an instrument means writing one document. F1's wrong tabs
  (bassoon, the bells) and the 26 in "Other" get fixed by writing their
  category down, which needs a listener/designer decision per instrument (see
  open decisions).

The WAV sample catalogue (`wavCategory`, by folder name) is a separate input,
and its folder → category map is legitimately a table.

### P3 — Families

- **Audition families** are already derived from a single source (the
  catalogue, through union-find). Keep that, and refer to family keys through
  `instrumentid` constants, since a family key is its base instrument id.
- **Family-keyed tables** (`familyDeepWords`, `gateExemptions`, the tune picks)
  either key by constant (Go tables) or live in JSON with load-time validation,
  following `tune_picks.json`/`TestTunePicksAreValid`. Either way there must be
  a test that every key is a roster id.

### P4 — Constants for the other namespaces, from the generators that already exist

| namespace | generator | output |
|---|---|---|
| voice block | `cmd/gen-synth-abi` (already reads `modularFields`) | Go constants `BlockSnare`, `BlockKick`, …; call the concept "voice block" in docs and code, never "family" |
| ABI param | `cmd/gen-synth-abi` | `abi.FilterCutoff = "filter_cutoff"`, `abi.Gen1SnareTone2`, … (1,321) |
| module type | the declaration list itself (`module_decls.go`), 67 types | a `const` per type next to its declaration, or generated from it |
| reference id | new step in `make refs-check`/catalogue tooling | `wavref.RefOboeA4`, … from `catalogue.json` |

Of these, voice blocks and module types are small and remove the F3
ambiguity, so they come first. ABI params are the largest piece of work (1,347
production uses), and the Synth tab tables (`synth_macro_groups.go`) would
benefit most.

### P5 — Boundaries fail loudly

- `SetInstrument` reports an unknown id: it logs at ERROR, and returns the
  error to the scene catalog, so the scene tests fail.
- Every id-keyed table has a "keys ⊆ roster" test, or its keys are
  constants (P1), which makes the check compile-time.
- `InstrumentConfigs` is cut down to the ids that have no document (the two
  `modular` voices), in ONE file. The stub's 19 entries go, so the test build
  reads the same documents production does.

### P6 — A ratchet so it cannot grow back

Add a discipline test in the repo's existing style (`TestTokenDiscipline`,
`TestNoNewTextInputOutsideTextField`): an AST scan, essentially `scan.go`, that
fails on a string literal equal to a registered instrument id, module type or
ABI param outside generated files and the registries themselves:

- It works on a **per-file budget that must only shrink**: over budget fails,
  and under budget also fails until the budget is lowered.
- It starts on production code at today's counts (1,086 instrument literals /
  38 files) and works down. Tests follow once the constants exist.
- It would have caught this week's `familyDeepWords` keys and
  `tune_picks`-style drift at review time.

## Suggested order

1. **P1** instrument constants + **P6** ratchet (production only). Mechanical,
   one generator change, no behaviour change.
2. **P5** fail-loud boundaries + keys ⊆ roster tests. Small, catches the
   silent cases at once.
3. **P2** properties in the documents. Fixes the user-visible F1 defects;
   needs the category decisions below.
4. **P4** voice blocks + module types (small), then reference ids, then ABI
   params (large).
5. Tests migrate to constants file by file, under the ratchet.

## Open decisions

1. **Untyped constants first, or a named `ID` type from the start?** The named
   type catches namespace mix-ups (F3) at compile time but touches every
   `string` API that carries an id.
2. **Where do the properties live:** in each instrument document, or in
   `index.json` beside the roster order?
3. **The category of the 26 instruments now in "Other"**, and of bassoon,
   double-bass, tubular-bells and the FM bells. This is a product decision: it
   decides the instrument menu's tabs.
4. **Should tests migrate (3,279 + 1,762 literals), or only production code?**
5. **Should the JS tests reference ids through a generated `instrument_ids.gen.js`,** like
   `synth_param_abi.gen.js`?
