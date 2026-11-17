# One Sound, one UUID — collapsing the instrument/recipe namespaces

Date: 2026-09-12
Status: proposed

## Problem

"Instrument" is defined in four places, in two incompatible shapes, and the
`-N` suffix carries three different meanings that three different functions
decode three different ways.

### Four registries

Of the 133 ids in `audio.BuiltinInstrumentIDs`:

| Where | Count | Shape |
|---|---|---|
| `modularInstrumentDefs` (`internal/audio/modular_instruments.go`) | 92 | One declarative row: `ID, RecipeID, Display, Category, Seed, Beats, DurationSec, Amplitude`. Derives the voice, the descriptor, the binding, the playable config and the JS table. |
| `engine_instruments.go` `CVariantInstrument` map | 41 | `Name, Render func, Beats, DefaultFX, Post` — voice only. |
| `builtinInstrumentRecipeBindings` (`synth_recipe.go`) | hand-written | id → recipe id, needed only because the 41 do not self-derive. |
| `builtinRecipeDescriptors` | hand-written | recipe metadata, split the same way. |

The modular table is already the intended design — one flat row per sound,
no indirection. The 41 legacy drum/FM ids are the holdouts, and the only
structural thing keeping them out is that `ModularInstrumentDef` has no
`DefaultFX` / `Post` field.

### The two worlds have started duplicating each other

Measured through `FactoryRecipeForInstrument`:

```
snare-1      recipe="drum-snare-rimshot"   rimshot      recipe="drum-snare-rimshot"
kick-1       recipe="drum-kick-punchy"     kick-punchy  recipe="synth-modular-kick-punchy"
```

`snare-1` is `rimshot` plus distortion and a gated tail. "Punchy kick" exists
twice under two recipe ids, one per world.

### `-N` means three things

1. **Shipped variant** — `snare-1` is "Variant set 1: Electronic/Tight"
   (rimshot render + distortion + gate); `snare-2` is "Variant set 2:
   Lo-fi/Dark" (bitcrusher + LP filter). Distinct sounds.
2. **Runtime duplicate instance** — `organ-2` is a second organ row, same
   sound, own display name. `cmd/gen-showcase/main.go:196` emits
   `fmt.Sprintf("%s-%d", ins.ID, n)`; `EnsureInstanceInstrument` promotes it.
3. **A name that merely ends in digits** — `kick-808`, `bass-808`.

Three decoders disagree:

- `audio.baseInstrumentID` (`synth_recipe_dispatch.go:523`) strips `-1`/`-2`,
  no roster check.
- `audio.instanceBaseID` (`instance_instrument.go:19`) strips `-1`..`-9`,
  no roster check.
- `refs.baseInstrumentID` (`doctor.go:33`) regex `-[1-9]` plus a
  base-must-be-known guard.

Meanings 1 and 2 share one namespace: `instanceBaseID("snare-1") == "snare"`.
Templates already ship meaning-2 ids (`piano-grand-2`, `piano-grand-3`,
`violin-2`, `guitar-nylon-2`, `guitar-nylon-3`, `voice-ahh-2`, `celesta-2`).
Nothing breaks today only because no shipped variant happens to collide. The
first template needing two snares emits `snare-2`, `EnsureInstanceInstrument`
returns early because that id is already registered, and the row silently
plays the lo-fi bitcrushed variant instead of a duplicate snare. `kick-808`
survives by having three digits; `bass-8` would not.

### The reference side mirrors it

`internal/wavref/catalogue.json`: 64 instrument-tier refs, 13 of them serving
2+ instruments through a many-to-many `Instruments []string`, and an
`Alternatives` field with zero committed entries. That many-to-many is what
forces `families.go`'s union-find, the `via <base>` / `NO REF`
classification, and `refs`' own `baseInstrumentID`.

## Goal

One entity — the **Sound** — identified by a UUID. Human names become
metadata that may collide freely and be renamed for free. Reference audio in
`$BEATMO_WAV_REFS` is the calibration target for a Sound, joined by UUID.

## Design

### The Sound record

```go
type Sound struct {
    UUID string // identity. Never parsed, never inferred, never reused.

    // Labels — metadata, freely collidable, freely renamable.
    Slug    string   // "kick-punchy"  human handle; MAY collide
    Display string   // "Dry Kick"
    Kind    string   // "kick"  the bucket — replaces the family concept
    Tags    []string // ["acoustic","dry","short"] — search and filter

    // The sound itself.
    Engine    string       // which DSP graph: modular | drum-gen | fm
    Seed      RecipeParams // map[string]float64 — THE config
    DefaultFX []EffectSlot
    Post      func([]float32, int)
    Beats       float64
    DurationSec float64
    Amplitude   float32

    Builtin bool // shipped vs user-created — replaces the "user." prefix
}
```

`Engine` is not a second level of instrument: it names the DSP graph, a small
closed set, and carries no identity. There is no `DerivesFrom`. Two variants
that share a generator each carry their own full `Seed`; duplicated seed
values are accepted deliberately, because inheritance is the two-level
structure this design exists to remove.

`Kind` replaces the inferred family. Bucketing becomes `GROUP BY Kind`, and
"the same kind but not the same sound" becomes expressible for the first
time: two Sounds may share `Kind: "snare"` while being unrelated in name.

### Identity and persistence

UUIDv4, minted from `crypto/rand` (~15 lines, WASM-safe). No new dependency:
`google/uuid` would cost bytes against the tracked WASM budget.

Persisted references carry the slug redundantly so diffs and logs stay
greppable, with only the UUID load-bearing:

```json
{"sound": "b7f3a1c2-4e5d-4a91-9c3f-2d8e1a6b0f47", "slug": "kick-punchy"}
```

The slug is refreshed on write and **ignored on read**. UI and logs print
`Display` / `Slug`; the UUID surfaces only when disambiguating a collision.

Lookup by slug returns a set. A unique hit resolves; an ambiguous one is an
error listing the candidate UUIDs, and `-uuid` disambiguates. Collisions are
legal; only ambiguous lookups fail.

### Two files, split by who writes them

The user-facing split is by **writer**, not by two levels of instrument:

| File | Written by | Holds |
|---|---|---|
| `internal/audio/sounds.go` | humans, tuning by ear | UUID, labels, engine, `Seed` **with its tuning-rationale comments**, FX, envelope |
| `internal/soundlib/references.json` | tools (`refs adopt` / `fetch` / `ab-all`) | UUID → wav file, provenance, licence, trim, measurements, ear verdict |

`internal/soundlib/references.json` replaces `internal/wavref/catalogue.json`
as the reference record; `internal/wavref` keeps its directory logic
unchanged (`Root`, `SourcesDir`, `Resolve`, the ffmpeg convert), since
`$BEATMO_WAV_REFS` and its `sources/` drop-in are already a single root with
a single environment variable. Only the schema moves: `Instruments []string`
plus `Alternatives` become one `Sound` UUID.

Joined by UUID. Keeping seeds in Go preserves the tuning prose
(`"amp_attack": 0.018, // swell → peak ~18 ms (the reference blooms in, no
click)`), which is among the highest-value documentation in the repo and
would not survive a JSON round-trip written by tools.

`refs adopt` therefore never rewrites `sounds.go` in place. When a drop-in
mints a new Sound it appends through the generator and prints the row; when
it binds to an existing Sound it writes only `references.json`.

### Tuning rationale is a protected asset

The comments around the seed values are not decoration — they are the record
of why each sound is tuned the way it is, and they are the only place that
knowledge exists. Current volume:

| File | Comment lines / total |
|---|---|
| `internal/audio/instrument_seeds.go` | 811 / 1710 (~48%) |
| `internal/audio/instrument_seeds_showcase2.go` | 203 / 393 |
| `internal/audio/instrument_seeds_showcase.go` | 163 / 421 |
| `internal/audio/engine_instruments.go` | 71 |
| `internal/audio/modular_instruments.go` | 102 |

That includes **278 per-param trailing rationale comments** of the form
`"amp_attack": 0.018, // swell → peak ~18 ms (the reference blooms in, no
click)`, plus per-row exception markers (`// conga-tumba: DurationSec 0.6 ≠
Beats-derived 0.5 — preserved exactly from config.go`) and the batch
provenance headers.

**Invariant: no phase of this migration may reduce that body of prose.** It
moves with the rows it explains; it is never regenerated away.

**Mechanism — surgical patch, never regenerate.** This problem is already
solved in-tree: `internal/audio/synthmatch/seedpatch.go` `PatchSeedBlock`
rewrites a `var <name> = RecipeParams{…}` block by replacing individual
values in place, preserving comments, key order and untouched keys, and
gofmt-ing the result. It carries a regression test for a `//` appearing
inside a trailing comment (`TestPatchSeedBlock_PreservesSlashComment`), which
an earlier `[^,\n/]+` pattern truncated.

Every machine write to `sounds.go` MUST follow that discipline:

- extend the same surgical approach to whole rows (`PatchSoundRow`) for the
  writes this migration needs — inserting a minted `UUID`, adding `Slug` /
  `Kind` / `Tags`, attaching `DefaultFX`;
- **never** build the file by marshalling a Go model back to source. An
  AST/template regeneration round-trip is what would silently delete all
  1,177 lines, and it would look like a successful run.

**Guard: `TestSoundTuningRationaleRetained`.** A ratchet on comment-line
count per sound-defining file, mirroring the existing
`token_discipline_test.go` `allowedLiteralBudget` pattern — it fails when the
count drops below the committed floor, and equally when the floor is stale
after prose is added, so the floor can only move up. A phase that drops a
comment fails loudly instead of quietly.

**Prose stays prose.** Rationale is promoted to a struct field only where a
*tool* must read the fact (e.g. a deliberate-exception marker a generator
checks). Turning explanatory prose into data fields for its own sake trades
one readable sentence for several unreadable ones.

### The WAV rule

Adopting a file resolves its stem against slugs:

- matches an existing Sound → that file becomes the Sound's calibration
  target (`references.json` only);
- matches no slug → mints a new Sound: fresh UUID, `Kind` from the stem
  prefix, `Seed` cloned from the nearest same-`Kind` Sound, awaiting
  `synth-match`;
- ambiguous → error listing the candidate UUIDs.

A WAV cannot generate a recipe. It is the target `synth-match` optimizes a
seed against, which is what makes "one instrument variant per WAV file" a
rule rather than a convention.

## What is deleted

| Gone | Replaced by |
|---|---|
| `builtinInstrumentRecipeBindings`, `builtinRecipeDescriptors` | the Sound *is* the recipe |
| `RecipeForInstrument`, `FactoryRecipeForInstrument`, `factoryRecipeForInstrument` | direct UUID lookup |
| `audio.baseInstrumentID`, `refs.baseInstrumentID`, `instanceBaseID` | nothing parses an id |
| `EnsureInstanceInstrument` + gen-showcase `%s-%d` suffixing | two rows share a UUID; display name lives on the row |
| `families.go` union-find | `GROUP BY Kind` |
| `Ref.Instruments []string`, `Ref.Alternatives` | `Sound string` (UUID), 1:1 |
| `RecipeBaseOf`, `RecipeIsBuiltin`, `user.*` prefix sniffing | `Builtin bool` |
| `CVariantInstrument` map (41 ids) | folded into the one table |
| `instrument_ids_gen.go` roster | derived from the Sound table |

The `-N` collision class disappears by construction.

## Migration

`schema.Version` is `0.0.1` pre-release and every reader already applies the
three-way accept / reject / migrate rule, so legacy documents land in the
existing seam.

A frozen `legacy_ids.json` maps every old string id (including every `-N`
variant and every template instance id) to a UUID. Generated once, never
hand-edited after. Readers resolve a non-UUID value through it; writers emit
UUIDs. Affected documents: project files, built-in templates, prefs
(favorites, per-instrument state), kits, instrument bundles.

## Risks

- **A regenerated UUID silently orphans every saved project.** This is the
  worst failure mode in the design. Minting lives in a separate
  `gen-sounds -mint` mode that refuses to touch any row that already has a
  UUID, guarded by `TestSoundUUIDsAreStable`, which pins UUID↔slug against a
  committed golden.
- **Byte-identity.** Renames and table folds must not change rendered audio.
  `TestSynthGoldenByteIdentity` is the gate and must stay green through every
  phase.
- **One-time rewrite of every persisted document**, including all built-in
  templates. Large diff, landed once.
- **Silent prose loss.** A regenerate-from-model write to `sounds.go` would
  delete ~1,177 lines of tuning rationale and report success. Prevented by
  the surgical-patch rule and `TestSoundTuningRationaleRetained`; see
  "Tuning rationale is a protected asset".
- **WASM surface.** `modular_instruments.gen.js` and the JS bridge exports
  carry UUIDs. A `resolveSound("kick-punchy")` export keeps console debugging
  ergonomic.

## Phases

Each phase is independently shippable and strictly removes indirection.

0. **Kill suffix semantics.** Rename the 18 shipped `-N` variants to
   descriptive slugs (`snare-ghost`, `kick-tight`, `hihat-pedal`,
   `clap-tight` are already descriptive and keep their names), delete the two
   stray `baseInstrumentID`s, give `instanceBaseID` a roster check, change
   gen-showcase's suffix form to `--N`. Introduces `legacyInstrumentIDs`, the
   append-only old-id → current-id table that Phase 2 later re-points at
   UUIDs; without it a rename silences instruments in saved projects. No
   audio change; byte-identity proves it.

   The `baseInstrumentID` deletion is NOT free: it feeds
   `normalizeAndScale` → `ConfigForInstrument(...).Amplitude`, and 17 of the
   22 variants resolve to a different amplitude unfolded, because
   `instrument_loudness_gen.go` carries base rows only (`fm-bell-1` would go
   0.4473 → 0.8). The fold is replaced by a declared `loudnessAliases` table,
   not removed. Plan:
   `docs/superpowers/plans/2026-09-12-phase0-kill-suffix-semantics.md`.
1. **One struct.** Add `DefaultFX` / `Post` to the def, fold the 41 legacy
   ids in, delete the `CVariantInstrument` map and both hand-written tables.
   The 71 comment lines in `engine_instruments.go` move with the rows they
   explain; `TestSoundTuningRationaleRetained` gates the fold.
2. **UUID identity.** Mint UUIDs, add labels (`Slug`/`Kind`/`Tags`), add
   `legacy_ids.json`, migrate readers and writers. Every edit to `sounds.go`
   goes through `PatchSoundRow`, not a regenerated file.
3. **Reference join.** `references.json` keyed 1:1 by UUID; delete the
   union-find and the borrow classification; `refs families` becomes
   `GROUP BY Kind`.
4. **Adopt mints Sounds.** The WAV rule above.
