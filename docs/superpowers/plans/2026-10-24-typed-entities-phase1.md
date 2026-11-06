# Typed Entities — Phase 1 (Instrument ID) Implementation Plan

> **For agentic workers:** execute inline (superpowers:executing-plans). No
> parallel editing agents: the tree carries uncommitted work.

**Goal:** every instrument identity in the Go code is the typed
`instrument.ID`, and every built-in instrument has a generated constant
(`roster.Oboe`). Behaviour, wire bytes and renders stay identical.

**Why this is lean (revision 2, 2026-10-24).** Revision 1 bundled four projects
into one phase: the typing, a data move of category/role/GM note into
`index.json`, a custom `go/packages` guard tool with a literal-budget ratchet,
and the mixer-channel type. Revision 2 keeps only the typing:

- **No data move yet.** Writing today's spelling-derived categories into
  `index.json` while keeping the spelling rules for non-roster ids gives two
  sources of truth and no behaviour gain. The move happens together with
  correcting the categories (spec B1), as its own step, with goldens and
  screenshots, because that step is visible.
- **No guard tool.** The one failure the compiler cannot see (a named string
  type reaching `syscall/js`, which panics) has a choke point: every export
  returns through `jsFn`. A normaliser there covers every return value; the
  few direct `Set`/`ValueOf` calls are audited once.
- **No literal ratchet.** Tables keyed by instrument use constants, so a
  renamed or deleted instrument breaks the build. The literal worklist comes
  from the compiler itself (the struct trick below), not from a scanner.
- **No characterization goldens.** A type change cannot change behaviour
  except through `any` (the JS bridge, `case string` / `.(string)`), which is
  handled explicitly. The existing byte-identity tests prove the wire.

**Spec:** `docs/superpowers/specs/2026-10-24-typed-entities-design.md`.

## Global constraints

- **No wire change.** JSON tags unchanged; project / instrument / bundle /
  prefs / template bytes, `ContentHash` / `DocSHA` and renders byte-identical.
- **No behaviour change.** Gated changes B1–B5 are out of this plan.
- `instrument.Doc.Category` (`"modular"`) stays a plain `string`.
- `exportInstrument.Origin` is an `int` (a node), not an instrument id.
- Kit roles, kit ids, display names and mixer channel ids stay `string` in
  this phase (channel = its own follow-up).
- Three build configurations build and vet at every step end.
- Never `git stash`, never stage or commit. Work inline.

## Verification (run from `src/go`, `GO=../../.tools/go/bin/go`)

- **BUILD3:** `$GO build ./... && $GO vet ./...`;
  `$GO vet -tags test -modfile=go.test.mod ./...`;
  `GOOS=js GOARCH=wasm $GO vet ./...`.
- **FAST:** `$GO test -tags test -modfile=go.test.mod ./...`.
- **REAL:** `make test-real` (repo root) for steps touching `internal/audio`.
- **BRIDGE:** (repo root)
  `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
  for steps touching `js_exports_*`.

## The struct trick (worklist without a tool)

To list every place a string literal or plain string becomes an
`instrument.ID`, temporarily change `type ID string` to
`type ID struct{ s string }` and run `$GO build -gcflags=-e ./... 2>&1`
(plus the other two configurations). Every implicit conversion becomes a
compile error with its `file:line`. Change the type back afterwards. Used in
Steps 3 and 4.

---

### Step 1 — The type and the constants

**Files:** create `internal/instrument/id.go`; modify `internal/instrument/doc.go`
(`Doc.ID`, `Doc.Origin`), `internal/audio/gen_instrument_ids.go`,
`internal/audio/instrument_ids_drift_test.go`; generated
`internal/instrument/roster/roster_gen.go`; whatever the compiler flags.

- `type ID string` in `internal/instrument` (std-only, stays a leaf).
- `Doc.ID ID`, `Doc.Origin ID`, tags unchanged.
- The generator also writes `internal/instrument/roster/roster_gen.go`:
  `const <CamelCase> instrument.ID = "<id>"` per roster id
  (`kick-808` → `Kick808`, a leading digit gets an `I` prefix) and
  `var All = []instrument.ID{…}` in index order. It fails when two ids map to
  one identifier or an id collides with `All`.
- `audio.BuiltinInstrumentIDs` stays `[]string` in this step.
- The drift test re-runs the generator and compares BOTH generated files.
- Compile fallout fixed with plain conversions only (`string(d.ID)`,
  `instrument.ID(s)`).
- Tests: `TestDocJSONUnchangedByTypedID` (every shipped document parses and
  re-marshals to the same bytes as before; `ContentHash` unchanged — the pack
  test already proves the latter), `roster` test: `len(All)` and order equal
  `audio.BuiltinInstrumentIDs`.
- Exit: BUILD3, FAST, REAL.

### Step 2 — JS bridge safety

**Files:** `internal/ui/js_exports_helpers.go` (+ test), the three known
`any`-switch sites (`internal/ui/js_exports_slimbar_diag.go`,
`internal/env/env.go`, `cmd/refs/match.go`).

- `jsFn` passes its result through `jsSafe(v any) any`: a value whose kind is
  string but whose type is not `string` becomes a plain `string`; `[]any` and
  `map[string]any` are walked recursively (copy-on-write, only when an element
  changes). Everything else passes through untouched.
- `jsSafe` is pure Go with no build tag, so it is unit-tested in the fast suite
  (`TestJSSafeConvertsNamedStrings`, `TestJSSafeLeavesPlainValuesAlone`,
  `TestJSSafeDoesNotCopyWhenNothingChanges`).
- The `any`-switch sites get a `case instrument.ID` beside `case string` when
  an id can reach them.
- After Step 3, a one-off scratch audit (not committed) lists every
  `Set`/`SetIndex`/`Call`/`Invoke`/`New`/`ValueOf` argument whose type is a
  named string; each is converted with `string(...)`.
- Exit: BUILD3, FAST, BRIDGE.

### Step 3 — Signatures, bottom-up, one area per sub-step

Order: (a) `internal/audio` top level; (b) audio subpackages
(`synthmatch`, `clone`, `songrender`, `synthdiag`), `internal/assets/instrumentpack`,
`hooks`, `userprefs`, `i18n`, `eventlogger`, `eventstream`; (c) `internal/ui`;
(d) `cmd/*` and `core/*`.

- An identity parameter, result, field, map key or slice element of type
  `string` becomes `instrument.ID`. `audio.BuiltinInstrumentIDs` becomes
  `roster.All` (type `[]instrument.ID`) in (a).
- Conversions happen where data enters: bridge args, JSON, flags, prefs,
  file names. Mid-pipeline `instrument.ID(x)` conversions from (a) are
  removed as (b)–(d) type their side.
- Each sub-step: BUILD3, FAST; REAL for (a) and (b); BRIDGE for (c).

### Step 4 — Production literals → constants

- Struct trick → the worklist of literal sites in production code.
- Replace each literal naming a roster instrument by its `roster.X`
  constant. A literal that is not on the roster (instance `--N`, `user.*`,
  sample ids, deliberately unknown ids) stays and is left as is.
- Tables keyed by instrument use constant keys, so the compiler checks them.
  Tables whose keys come from data files (`familyDeepWords`, `specMap`,
  `tune_picks.json`, …) get a "keys ⊆ roster" test; a failure is a real stale
  entry and is reported to the user, not fixed silently.
- Exit: BUILD3, FAST, REAL, BRIDGE.

### Step 5 — Docs

- CLAUDE.md: `instrument.ID` is the instrument identity, `roster.X` names a
  built-in, never pass a named type to `syscall/js` outside `jsFn`; adding an
  instrument → `go generate ./internal/audio/` also refreshes `roster`.
- Spec status → Phase 1 done; memory updated.

## Follow-ups (separate, each approved on its own)

1. `audio.ChannelID` (master can never be passed as an instrument).
2. Delete `InstrumentConfigs` (unreachable for every roster id).
3. "Categories are data": category / role / GM note into `index.json` AND
   corrected (B1) in one step, with goldens and before/after screenshots.
4. Tests migrate to constants, opportunistically.

## Outcome (2026-10-24)

Steps 1–5 done; tree green except the three pre-existing, non-causal failures
proven on a pre-change copy (`TestWasmAssetSizeBudget` = the local
`main.wasm`; `TestGitignoreHidesEveryAudioFile` = only inside a scratch copy
whose `.git` is a symlink; `TestEveryDialIsMeasurable` = today's C sources vs
`synth_dial_known_weak.json`, identical before the change).

Decisions taken while building (each small, all reversible):

- **Mixer routing stays `string`** (channels, analyzers, EQ, sends, mixer
  slots); the insert-effect API takes `instrument.ID` and converts once
  inside `effect_chain.go`. hooks: `RowChangePayload`, `InstrumentParamPayload`,
  `RecipePayload`, `SamplePayload`, `FavoritePayload`, `CustomWAVPayload`,
  `InstrumentRenamePayload`, `KitPayload.Members` are typed; the EQ / filter /
  insert-effect `Channel` fields wait for `ChannelID`.
- **`cmd/*` converts at its boundary** into the typed API (flags, catalogue
  rows, tool JSON). Typing `cmd/refs` internally waits for Phase 3: it mixes
  families, instruments and catalogue entries, a modelling question.
- **Tooling stayed scratch**: the JS audit (`go/packages`), the signature
  rewriter and the literal scanner were run, not committed. The audit found
  30 `syscall/js` call sites that compiled and would have panicked in the
  browser; all now pass `string(id)`.
- **Findings for the user:** `i18n.builtinInstruments` keyed `"bass"`, which no
  shipped instrument is (removed in the second pass below).

## Second pass (2026-10-24): stale references removed, gaps closed

The user asked to remove every reference to something that does not exist,
with no exception lists, and to close the remaining gaps. Done:

- **Stale references removed:** the i18n `"bass"` default name (+ its key and
  both translations); the three `InstrumentConfigs` tables (unreachable: every
  shipped instrument has a document) — `ConfigForInstrument` is one untagged
  file; a committed `src/go/scope_export.jsonl` debug dump naming a retired id
  (now gitignored); comments and fixtures naming retired ids (`kick-1`,
  `snare-2`, `fm-epiano-1`, `tom-high`, `voice-bass`, …) in Go and JS; the LLM
  agent prompt examples (`kick-1`, the nonexistent param `osc_tune`); a
  duplicate instance-id decoder in a test (`baseInstrument`).
- **`channel.ID`** (`internal/channel`): every mixer channel API, the
  analyzer / scope / scope-export services, hooks payloads, the UI's EQ /
  Levels / Chain state and the recording channel list. `channel.Master`
  replaces the `"main"` literals; the analyzer, scope and scope export now
  name the master bus `channel.Master` too (they said `"master"`; recording
  files keep the `master.<ext>` stem through `audio.RecordingMasterID`). The JS
  audit found 18 more `syscall/js` sites a `channel.ID` would have crashed.
- **`instrument.Role`** for kit roles; `Kit.Members` and `KitPayload.Members`
  are `map[instrument.Role]instrument.ID`.
- **Categories and kit roles are data** (`builtinProperties`, values exactly
  what the spelling rules gave, so the menu does not change); the spelling
  rules only classify instruments the user named. The catalog groups a synth
  under its menu category (`synthCatalogCategory`) instead of a second set of
  prefix rules.
- **Remaining string-typed instrument ids typed:** string-keyed maps in the UI
  (instrument colours, sample paths, the menu's colour / category / search
  maps, the Synth tab's per-instrument state), `ImportResult`, the user library
  file, `CaptureEvent`, song-match hints, time manifests, the Sampler preview
  id, the wav catalogue's `Ref.Instruments`, and every `cmd/*` tool
  (`gen-showcase` specs, `export_audio`, `clone`, `gen-fundamentals`,
  `measure-loudness`, `refs`). The DESIGN.md colour generator emits roster
  constants.
- **Tests use the constants** and typed helpers; synthetic ids that looked like
  built-ins became roster constants or obviously-user ids.
- Templates regenerate byte-identical; JS cannot be typed, so browser tests keep
  their id strings (each must name a shipped instrument).
