# Portable, decoupled instrument format — design

**Date:** 2026-07-19
**Status:** Design — awaiting review
**Scope:** `internal/audio/`, `internal/userprefs/` (or a new sibling store), `internal/ui/` — a standalone, shareable, **config-only** instrument file + a persistence store decoupled from user/game preferences.

## Overview

Instruments today are persisted **inside the shared `prefsDoc` blob** (`user_recipes` + `recipe_overrides` + `sample_edits`, `internal/userprefs/store.go:42`), physically mixed with favorites/language/audio-panel state, and there is **no standalone shareable instrument file** (the only single-recipe export, `exportUserRecipe` in `js_exports_synth_recipe.go:370`, is broken — it drops `base_recipe`/`param_seed` — and carries no effects/sample). This design introduces:

1. A **portable, config-only** instrument file (`*.beatmo-inst.json`) holding **1+ instruments**, with a **normalized effects library** instruments reference by id, WAV carried **by reference + content-hash version pin (never embedded)**, and **our own file-format versioning**.
2. A **decoupled instrument store** with its own on-disk/localStorage location, so the prefs blob holds only prefs.
3. **Export / Import** wired into the Synth & Sampler `⋯` menu + file picker, with import collision safety.

## Goals

1. Config-only, human-readable, diff-able instrument files that reconstruct a sound on a machine that has never seen it — **no PCM ever embedded**.
2. Effects are **first-class and normalized**: defined once in a file-level list, referenced by instruments in order (a chain is a list of effect ids; repeats allowed). Instruments are **not** tightly coupled to effect definitions.
3. WAV-based instruments reference the user's WAV **by name + SHA-256 + a friendly version**, pinning the exact bytes; sharing the config never moves audio data.
4. Move instrument data out of `prefsDoc` into its own store; migrate existing users transparently.
5. Own the file-format version integer + migration path (the thing `RecipeDoc` lacks today).

## Non-goals

- No embedded PCM / audio bytes in the shareable file (WAV is a reference).
- No "My Instruments" browse/manage screen (deferred).
- No cloud/URL sharing — files only.
- No delay/reverb **sends** in the file (they route to project-level buses that may not exist on import).
- No change to how audio renders; this is persistence/portability only.

## Current-state anchors (from code map)

- Recipe doc: `RecipeDoc{ID, DisplayName, Category, BaseRecipe, ParamDefs, ParamSeed, Origin}` — `internal/audio/synth_recipe.go:94`. Pure-synth, **versionless**. `RegisterUserRecipeFromBase` (`recipe_overrides.go:248`) builds+registers it (populates base+seed reliably — the correct path the broken export bypasses).
- Prefs coupling: `prefsDoc` sections `user_recipes`/`recipe_overrides`/`sample_edits` (`userprefs/store.go:42`); one blob at `beatmo.prefs` (localStorage) / `prefs.json` (desktop), whole-doc atomic write (`store_js.go:343`, `store_notjs.go:465`).
- Startup apply: `audio.ApplyUserRecipeOverrides` + `UserRecipeSource` (`recipe_overrides.go:49,75`); bootstraps `cmd/beatmo.go:199`, `js_bootstrap_wasm.go:38`.
- Effects: `DrumRow.Effects []audio.EffectSlot` (row-level, ordered insert chain); exported inline as `effects` in `exportInstrument` (`export.go:57`).
- EQ: per-row `EQGainsDB`/`EQBandMuted`/`HPF*`/`LPF*` (`drumview.go:174`).
- Sample edit: `SampleEdit.Fields()` flat map (`sample_edit_descriptor.go:215`) — a post-render transform; the recipe stays the source of truth (so a re-voiced-and-edited built-in needs **no** WAV).
- User WAV PCM: `SampleRecord{PCM, SampleRate}` + `SampleStore` (PCM outside prefs, files/IndexedDB) — `user_samples.go:17`, `store.go:169`.
- Registry/ids: `RegisterRecipe` overwrites an existing id **in place** with no collision guard (`synth_registry.go:56`); user ids namespaced `user.<base>.<hash>` (`recipe_save_sink.go:241`).

## Design

### 1. File format — `*.beatmo-inst.json`

Bundle wrapping a normalized effects library + 1..N instruments:

```jsonc
{
  "format": "beatmo.instrument",   // fixed magic; rejects foreign JSON
  "version": 1,                    // OUR integer format version
  "effects": [                     // normalized library — each effect defined ONCE
    { "id": "fx1", "name": "Crunch", "type": "bitcrusher", "params": {…}, "enabled": true },
    { "id": "fx2", "name": "Room",   "type": "reverb",     "params": {…}, "enabled": true }
  ],
  "instruments": [
    {
      "name": "Warm 808",
      "kind": "synth" | "sample" | "wav",
      "color": "#C87850FF",                 // optional
      "recipe": { RecipeDoc },              // base_recipe + param_defs + param_seed (reliably populated)
      "synth_params": { … },                // optional effective delta vs shipped
      "sample_edit": { start_frac, gain_db, … },   // optional non-destructive edit
      "wav_ref": { "name": "my-808.wav", "sha256": "9f2c…", "version": 3 }, // ONLY kind:"wav"
      "effect_chain": [ "fx1", "fx2", "fx1" ],     // ORDERED references into effects[]; repeats allowed
      "eq": { "gains_db": [...10], "bands_hz": [[lo,hi]…], "hpf": {…}, "lpf": {…} } // optional
    }
  ]
}
```

**Normalized effects (per the decoupling directive).** `effects[]` is the single source of each effect's definition; instruments carry only an ordered `effect_chain` of ids. On export, a row's `[]EffectSlot` is deduplicated by content into `effects[]` (shared across instruments in a pack) and replaced by id references. On import/apply, `effect_chain` is denormalized back into the target row's `[]EffectSlot`. EQ stays inline on the instrument — it's a single fixed 10-band structure, not a pluggable, order-variable chain unit.

**Per-kind content (config-only, no PCM):**
- `synth` — `recipe` (+ optional `synth_params`); no `wav_ref`.
- `sample` — a built-in re-voiced/edited: `recipe` + `sample_edit`; **no** `wav_ref` (the recipe re-renders, then the edit applies).
- `wav` — a user WAV: `wav_ref` (reference + pinned version); `recipe`/`synth_params` minimal or absent.

### 2. WAV reference + versioning (hash + local registry)

The file **never** carries audio bytes. A `wav_ref` pins the exact WAV version:
- `sha256` — authoritative content hash (the portable truth).
- `name` — the WAV's filename (resolution hint + human anchor).
- `version` — a friendly monotonic label (v1, v2…) from a **local, file-based WAV registry** we own.

**The WAV registry** lives in the new instrument store namespace (§3), mapping `name → [ { version:int, sha256, location?, cachedSampleKey } ]`. When a WAV's content changes (new hash), it gets the next `version`. `location` is the on-disk path where available (desktop); `cachedSampleKey` points at the existing `SampleStore` (files/IndexedDB) copy used for playback (browsers can't re-read a path without a fresh pick).

**Import resolution of a `wav_ref`:**
- Registry has an entry whose `sha256` matches → bind the instrument to it (existing `PutUserSample`/sample path). Done.
- `name` present but no matching hash → **version mismatch**: "This instrument wants *my-808.wav* v3 (9f2c…); you have v4. Use current / Locate file." (existing confirm-menu pattern).
- `name` absent → **missing**: prompt "Locate *my-808.wav*" via the file picker; hash the picked file; if it matches, register it as that version and bind; if not, offer to use it anyway (records a new version) or cancel.

This keeps the config portable text; sharing a WAV instrument means also sharing the WAV file, which the recipient locates once and the hash verifies.

### 3. Decoupled instrument store

A **new store with its own location**, separate from `prefsDoc`:

| Store | Holds | Desktop / Browser |
|---|---|---|
| `prefs` (unchanged) | favorites, audio-panel, notifications, knob-steps, language | `prefs.json` / `beatmo.prefs` |
| **`instruments` (new)** | user instruments (recipe docs), builtin recipe-overrides, sample-edits, effects library, WAV registry | `instruments.json` / `beatmo.instruments` |
| `samples` (existing) | raw PCM (incl. WAV cache keyed by hash) | files / IndexedDB |

Implemented as a new store surface (new `internal/instrlib` package, or a distinct document+location within `internal/userprefs` reusing its atomic-write/localStorage primitives — decided at plan time; the invariant is **physical separation from the prefs blob**). The startup apply path (`ApplyUserRecipeOverrides` and the sample-edit apply) reads from this store instead of prefs.

**Migration (transparent):** on boot, if the new store is empty and `prefsDoc` still has `user_recipes`/`recipe_overrides`/`sample_edits`, copy them into the new store and stop writing them to prefs (keep reading the old sections as a fallback for one release). PCM already lives outside prefs — untouched. No user action; nothing lost.

### 4. Export / Import UX + collision safety

- The Synth & Sampler **`⋯` menu** (built in the prior change) gains **Export instrument…** and **Import instrument…**, reusing the existing `ui.dialog` async file picker (WAV-upload / project-import infra).
- **Export:** the active instrument → gather recipe (base+seed), `synth_params` delta, `sample_edit`, `wav_ref` (compute/lookup hash+version), the row's EQ, and the row's effects **normalized** into `effects[]` + `effect_chain` → write a 1-instrument bundle (download in browser / native-save on desktop). (A multi-instrument "pack" is the same writer over a set — the format already supports it; a pack-selection UI is out of scope but the writer is not.)
- **Import:** pick file → validate `format`/`version` → per instrument:
  - **Never clobber a built-in** — always re-namespace to a fresh `user.*` id.
  - **Same-named USER instrument exists** → confirm **Replace** / **Keep both**.
  - Register the recipe via `RegisterUserRecipeFromBase` (reliable base+seed); resolve `wav_ref` (§2); denormalize `effect_chain` → row `[]EffectSlot`; apply `eq`; persist to the new store.
  - Imported instruments apply to a row through the normal instrument menu; applying restores the instrument's FX chain + EQ to that row.

### 5. Fix the broken single-recipe export

The new export path builds the `RecipeDoc` from the live registration **with** `base_recipe` + `param_seed` (the Save-As reconstruction, not the broken `exportUserRecipe`). The old `exportUserRecipe`/`importUserRecipe` JS exports are redirected to the new codepath (or retired) so there is one correct round-trip.

### 6. Versioning scheme (ours)

- **Bundle:** `format` magic + integer `version` (start `1`). Import rejects a missing/foreign `format`, rejects `version` greater than the running app's max, and migrates older versions forward (additive `omitempty` fields need no bump).
- **WAV:** monotonic per-file `version` in the local registry; content change ⇒ new version; `sha256` is the cross-machine pin.
- `RecipeDoc` stays versionless internally — the bundle version covers the file.

## Testing

- **Go round-trip** (`internal/audio` / `internal/ui`, fast path): export→import yields identical effective params, `sample_edit`, EQ, and a byte-equal normalized `effects`/`effect_chain`; effect dedup produces one library entry for two instruments sharing settings; `effect_chain` order + repeats preserved.
- **WAV pin:** matching hash binds; changed bytes → version-mismatch branch; missing name → locate branch; hashing is stable.
- **Collision:** import never overwrites a built-in id; same-named user instrument triggers Replace/Keep-both; Keep-both re-namespaces.
- **Store decoupling + migration:** new store writes to its own location; prefs blob no longer gains instrument sections; boot migration copies legacy prefs sections once and is idempotent.
- **Versioning:** foreign `format` rejected; future `version` rejected; older `version` migrates.
- **i18n:** new menu items + dialog strings in en + es (drift guard).
- Follow the functional-test doctrine — drive `Game.Update`, not hand-built structs.

## Risks

- **Effect param schema drift** — a normalized effect must carry enough (`type` + `params`) to reconstruct across app versions; unknown effect types on import degrade gracefully (skip with a notification, don't fail the whole import).
- **Browser WAV resolution** — no persistent file paths; rely on the hash-keyed sample cache + a locate prompt. Import of a wav-kind instrument on a fresh browser must clearly ask for the file rather than silently drop it.
- **Store migration correctness** — legacy `prefsDoc` sections must migrate exactly once and never double-apply; the boot path must be idempotent and safe if the new store write fails mid-way.
- **Recipe id collisions across imports** — enforce re-namespacing + the collision guard `RegisterRecipe` currently lacks.

## Out of scope (possible follow-ups)

- "My Instruments" library/browse UI; pack-selection UI (multi-export chooser).
- Sends (delay/reverb) in the file; cloud/URL sharing; embedding audio.
