# Instrument Identity Decoupling + Category Lookup

**Date:** 2026-06-27
**Status:** Approved design, ready for implementation plan
**Branch:** add-core-node-types

## Problem

A user renaming an instrument should be an immediate, metadata-only change — a
display label, nothing more. Today it is the opposite: renaming **mutates the
instrument's identity**.

In `js_exports_graph_ui.go` `commitRename`:

```go
newID := strings.ToLower(name)        // ID is derived FROM the typed name
audio.RenameInstrument(oldID, newID)  // re-keys the audio registry
// + re-keys samplePath[], customColors[], EQ tracking, label caches, rebuilds menu...
```

So "rename" means "change the ID and cascade through six maps." That is the
coupling we are removing.

Separately, instrument **categories** are a tangle of three overlapping
concepts with no clean lookup API:

1. **Recipe category** — `drum` / `fm` / `modular` (which audio engine). Structural.
2. **Kit role** — `kick` / `snare` / `hat` / `bass` (`DrumRow.Role` + the
   `RoleForInstrument()` heuristic). Semantic routing.
3. **Catalog category** — `SoundMeta.Category`, derived from asset *folder names*,
   used only to group the picker menu.

There is no canonical taxonomy, no `InstrumentsByCategory(cat)` lookup, and no
seam for user-defined categories.

## What is already correct (do not touch)

The codebase already keys functional lookups off an immutable string ID:
`instruments` registry, `DrumRow.Instrument`, voice cache, EQ channel, insert
effects, recipe bindings, import/export matching, and template generation all
resolve by ID. Template JSON files carry both `name` (display) and `id`
(identity); rows/nodes resolve by `id`. **Name is never used as a functional key.**

The single violation is the rename path, which conflates name and ID. Fixing
that, plus building the category lookup API and simplifying the three-category
tangle, is the whole job.

## Constraints

- **No backwards compatibility required.** Sole user. We may delete
  `audio.RenameInstrument`, change project-JSON semantics, and re-key freely
  with no migration shims.
- **Custom-category persistence is deferred.** Build the hardcoded taxonomy and
  lookup API behind a registry seam that *could* accept custom categories later,
  but wire no storage now.
- **Display name is per-instrument (catalog-level), keyed by ID.** Renaming
  updates the name everywhere that ID appears. (Not per-row, not a two-layer
  override.)

## Design

### Core principle

Identity = the immutable instrument ID. Display name and category are **metadata
resolved through lookup functions keyed by ID**. Renaming touches only a name
store; it never changes the ID and never re-keys any other map.

### Component 1 — Display-name store (the headline fix)

A name-override registry keyed by instrument ID, process-global in the `audio`
package alongside `instruments`:

- `SetInstrumentDisplayName(id, name string)`
- `InstrumentDisplayName(id string) string`
- `ClearInstrumentDisplayName(id string)`

Resolution order for `InstrumentDisplayName(id)`:

1. user override (the store)
2. catalog `SoundMeta.Name`
3. recipe `DisplayName()`
4. `PrettyName(id)` (title-cased ID)

This lifts the existing `computeInstLabel` resolution (today in
`drumview_instrument_helpers.go`) into the audio layer so non-UI callers
(songrender, gen-showcase, tests) share one source of truth. The UI's
`computeInstLabel` becomes a thin wrapper that calls
`audio.InstrumentDisplayName(id)` and keeps its per-DrumView cache.

`DrumRow.Name` becomes a render **cache** of `InstrumentDisplayName(id)`, not an
identity-bearing field. It is recomputed on instrument change and on rename, and
is still drawn from the cache for performance.

**Rename collapses to one metadata call.** Every rename site
(`js_exports_graph_ui.go` `commitRename` ×2 at lines ~1073/~1116, and
`drumview_ctor.go:812`) becomes:

```go
audio.SetInstrumentDisplayName(id, name)   // id is unchanged
dv.invalidateLabelCaches()
dv.refreshInstruments()                     // only if the menu is open
```

The deleted machinery, because the ID never changes:

- the `newID := strings.ToLower(name)` derivation
- `audio.RenameInstrument` (the function itself is removed)
- re-keying of `samplePath[oldID→newID]`, `customColors[oldID→newID]`
- EQ-channel-follow logic for an ID change (the ID no longer changes)

`onRowInstrumentChanged` is still called by `SetInstrument` (real instrument
swap) and the kit binder, but **not** by rename anymore — rename is no longer an
instrument change, only a label change. A new, smaller notifier
(`onRowNameChanged` or reuse of `emitRowInstrumentChange` with old==new) carries
the label-only event if any observer needs it.

**Project JSON:** schema unchanged. The per-instrument `name` field now means
"display-name override." Export writes `InstrumentDisplayName(id)`. Import routes
`inst.Name → audio.SetInstrumentDisplayName(inst.ID, inst.Name)` instead of
setting an independent `row.Name`.

### Component 2 — Canonical category taxonomy + lookup API

New `internal/audio/category.go`:

- A hardcoded category set with stable IDs and display names:
  **Kick, Snare, HiHat, Cymbal, Tom, Percussion, Bass, Keys, Guitar, Strings,
  Woodwind, Brass, Lead/Synth, Other.** (Mirrors the recipe groupings already in
  the registry: bowed strings, plucked/guitars, keys, woodwind, brass, bass,
  percussion, etc.)
- Single source of truth: one `map[instrumentID]CategoryID`, built once from the
  existing instrument + recipe tables. Any instrument with no explicit mapping
  resolves to `Other`.
- Public API:
  - `Categories() []Category` — ordered taxonomy for the picker
  - `CategoryOf(id string) CategoryID`
  - `InstrumentsByCategory(cat CategoryID) []string`
- **Extension seam (custom categories, deferred):** the taxonomy lives behind a
  small registry value seeded with the hardcoded defaults. `RegisterCategory`
  and `AssignCategory` exist and mutate the in-memory registry; **no persistence
  is wired**. This is the clean future hook the user asked for without building
  storage now.

### Component 3 — Simplify the three-category tangle

- The picker (`drumview_instruments.go`) stops deriving categories from asset
  folder names (`SoundMeta.Category`) + an ad-hoc fallback. It reads
  `audio.CategoryOf` / `audio.InstrumentsByCategory`. The `instCatByID`
  construction in `refreshInstruments` is removed.
- Replace the `RoleForInstrument()` heuristic with a category-driven lookup
  where role and category coincide (kick/snare/hat/etc.). **Scope guard:** do
  this only if it is a low-risk swap; if routing depends on role subtleties the
  taxonomy does not capture, unify just the UI category and leave routing-role
  untouched. The plan must verify before committing to the role change.
- Recipe-category (`drum`/`fm`/`modular`) stays as the orthogonal **engine**
  axis. Document it as deliberately separate; it is not user-facing and not part
  of the new taxonomy.

### Component 4 — Duplicate-ID template migration

Per-instrument naming means two voices sharing one ID can no longer carry two
names. Six committed templates do exactly that:

- `bach-toccata.json` — `organ` ("Organ"/"Pedal")
- `blackbox-ride-on-time.json` — `organ` ("Pad Lo"/"Pad Hi")
- `asturias.json` — `guitar-nylon`
- `eagles-hotel-california.json` — `guitar-electric`
- `puente-oye-como-va.json` — `conga`
- `wonder-superstition.json` — `guitar-electric`

Fix:

1. gen-showcase auto-suffixes a repeated ID in `specs.go` to a distinct instance
   ID (`organ`, then `organ-2`, …) when building the instruments array.
2. Add a **base-ID fallback in recipe binding**: any `X-N` resolves to `X`'s
   recipe when `X-N` is not explicitly bound. This reuses the existing `-N`
   variant convention (`baseInstrumentID()` already strips the suffix for
   amplitude lookup), so `organ-2` renders identically to `organ` with **zero**
   new binding entries, while owning a distinct identity and its own display
   name. Explicit `X-N` bindings (e.g. `snare-1 → drum-snare-rimshot`) still win.
3. Regenerate the six templates; update gen-showcase drift + musical-fidelity
   tests.

## Testing

New:

- Name-store resolution order (override > catalog > recipe > pretty).
- **Rename-is-metadata-only:** after rename, the instrument ID is unchanged, the
  audio registry is untouched, and EQ / insert effects / pan / sends survive.
- Category lookup: `CategoryOf`, `InstrumentsByCategory`, `Other` fallback,
  registry seam (`RegisterCategory`/`AssignCategory` reflected in lookups).
- Duplicate-ID suffixing in gen-showcase + base-ID recipe fallback renders
  `X-N` identically to `X`.

Simplified:

- Rewrite rename tests that asserted ID-change behavior to assert ID-stability.
- Delete tests tied to the removed `RenameInstrument` cascade.
- Picker category tests read the new API instead of folder-derived categories.

## Risks / verification gates for the plan

- Confirm nothing other than rename relied on `RenameInstrument` (kit binder,
  favorites userprefs keyed by ID, sampler). The name store is purely additive;
  the risk lives in the deletions.
- Confirm the base-ID recipe fallback does not shadow any explicit `X-N`
  binding (explicit wins; fallback only fills gaps).
- Confirm `Role`-from-category is safe before replacing `RoleForInstrument()`;
  otherwise keep role as-is and only unify the UI category (Component 3 scope
  guard).

## Out of scope

- Custom-category persistence (per-project or global storage).
- Any change to the functional ID-keying that already works.
- Sampler ID-minting scheme (`user.sample.%08x`) — IDs are frozen at creation;
  rename no longer recomputes them, so the existing scheme is fine.
