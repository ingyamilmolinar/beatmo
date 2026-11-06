# Synth tab — macro dials on the category cards

Date: 2026-10-09
Status: implemented 2026-10-09 (working tree, uncommitted)
Superseded in part by `2026-10-13-absolute-synth-dials-design.md` (dials are
ABSOLUTE: the reading is a pure function of the document; §2.3, §3 and the
relative / session-memory decisions below no longer hold).
Builds on: `2026-09-05-synth-tab-overview-and-signal-graph-design.md` (the
category root this adds dials to). Reverses that spec's non-goal "macro drag
the whole concept adjustment" — cards still navigate, but each now also
carries up to three dials that move many params at once.

## Problem

The Synth tab root shows plain-English category cards (Sound, Brightness,
Length, … for a violin; Punch, Boom, Tone, … for a kick). A card only
NAVIGATES: tapping it drills into that category's raw params (Brightness →
eight filter / filter-envelope / finish knobs). Nothing on the root changes the
sound, so a beginner who wants "a bit brighter" must learn which of eight raw
knobs means bright, and how far to move each one so the result still sounds
like the instrument.

## Goal

From the grouped root, a user changes the sound with up to **three dials per
category card**. Each dial is a high-level idea ("Bright", "Edge", "Sweep")
that moves a curated set of existing synth params together, each by its own
amount and in its own direction. The raw knobs stay one drill away and show
the values the dials produced.

Dials are **metadata only**: a dial is a vector over existing synth params
(per-param direction + magnitude) plus presentation strings. New kinds of
dials are created by editing data, not code.

## Decisions taken with the user (2026-10-09)

| Question | Decision |
|---|---|
| Where do dials live | Up to 3 dials **per category card** on the root. |
| Dial ↔ param relationship | **Relative**: the dial centre (50%) is the instrument exactly as currently tuned; turning it bends each mapped param away from that value toward a range edge. |
| When does a dial re-centre | **Session memory**: a dial keeps its position while the user works, and re-centres on the current sound when something else changes its params (raw knob edit, Reset, import, undo/redo). |
| Approach | One data-driven dial table + one generic morph function (not per-instrument definitions, not snapshot interpolation). |
| Mapping representation | **Decoupled metadata**: dial = list of `{param, weight, space}` terms; no dial-specific code. |
| Persistence | Dial edits change synth params, which persist in the project JSON through the instrument document like any knob edit. **Nothing about the dials themselves** (positions, anchors, definitions) is ever written to a project, instrument bundle, prefs or library. |

Non-goals: the audio engine, the param block / ABI, the instrument document
schema, the project / bundle file formats, per-instrument dial overrides, a
"surprise me" randomizer, dials inside the drilled view.

## 1. What the user sees and does

### 1.1 Card layout (root)

Each category card keeps its title, one-liner and mini graph. Below the
one-liner, a row of up to three **arc dials**, each with a short label
(`Bright` · `Edge` · `Sweep`). The dial draws:

- a track arc and a value arc in the Visualizers family (never the chrome
  accent — DESIGN.md "Data canvases use the Visualizers family");
- a **centre tick** at 50% ("as tuned");
- dimmed (same dim state as a bypassed drilled card) when every live term of
  the dial sits in a bypassed stage — still interactive.

The card's mini graph morphs live while a dial turns, through the existing
`synthCurveHook` morph seam.

The existing "essentials gauges" row is **replaced** by the dial row: the
gauges previewed raw params, which is exactly what the dials now abstract.
A card with no live dial shows no row (title + one-liner + graph only).

### 1.2 Gestures

| Input | Effect |
|---|---|
| Vertical drag on a dial | Adjusts it. Every detent applies live (next trigger sounds different; no audition). |
| Release | ONE commit: `commitInstrumentParams` → one undo step, one log line. |
| Tap on a dial | Opens the shared `ValueWheel` popup over a `NumberModel` 0–100 % (step 1). Accept = one commit; Esc restores the opening values verbatim (`WheelBinding.OnCancel`). |
| Double-tap on a dial | Returns it to 50 % and writes the anchor values back exactly; one commit. |
| Arrow keys while a dial popup is open | Through the partial-claim tier (`KeyboardKeyClaimant`): Up/Down = value, Left/Right = step; both arrow spellings. |
| Tap anywhere else on the card | Drills in, as today. |
| Mouse wheel over a dial | Through the detent chokepoint, same as other synth controls. |

The dial's hit areas sit above the card's drill hit area (`card z + 1`), so a
press on a dial never drills; the card catch-all remains for the rest.

### 1.3 Space

The desktop root is short (two card rows today, gauges already dropped to fit).
Dials make cards taller, so the root grid becomes **scrollable** using the
existing `ControlGrid` smooth scroll. The card count is unchanged. Mobile keeps
its stacked layout with the same card shape.

### 1.4 Drilled view

Unchanged. The raw knobs show whatever values the dials wrote. Editing a raw
knob re-centres every dial that drives that param (§3.3).

## 2. Dials as metadata

### 2.1 The data file

`internal/assets/synth_dials.json`, embedded via `go:embed` next to the other
assets. It is a Beatmo JSON document, so it carries `schema_version`
(`schema.Envelope`, strict current-version check like every other format).

```json
{
  "schema_version": "0.0.1",
  "dials": [
    {
      "id": "bright",
      "cards": ["brightness", "tone"],
      "label": "synth.dial.bright",
      "gloss": "synth.dial.bright.gloss",
      "metric": "bright.centroid",
      "terms": [
        { "param": "filter_cutoff",   "weight":  0.6, "space": "log" },
        { "param": "post_brightness", "weight":  0.5 },
        { "param": "post_darken",     "weight": -0.7 }
      ]
    },
    {
      "id": "drop",
      "cards": ["pitchdrop"],
      "label": "synth.dial.drop",
      "gloss": "synth.dial.drop.gloss",
      "metric": "",
      "terms": [
        { "param": "pitchenv_amt",      "weight": 0.8, "space": "magnitude" },
        { "param": "gen1_kick_pe_amt",  "weight": 0.7 }
      ]
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `id` | Stable, unique dial id. Keys session state. |
| `cards` | The category card id(s) it sits on (`macroGroupDef.ID`). One dial may serve several cards that mean the same idea in the two tables (`brightness` + `tone`, `sound` + `source`, `warmth` + `grit`); `length` and `pitchdrop` exist in both tables under the same id. |
| `label`, `gloss` | i18n keys (`i18n.Key` is a string). The gloss shows as the dial's tooltip / popup caption. |
| `metric` | A `fingerprint` descriptor ID the dial must move monotonically (§5.4), or `""` for dials with no single honest metric (then the test requires only an audible change). |
| `terms[].param` | An existing schema param name — generic (`filter_cutoff`) or gen-slot family (`gen1_snare_tune`). |
| `terms[].weight` | −1…1. **Sign = direction** (+ toward `Max`, − toward `Min`); **size = magnitude**: the fraction of the way to that edge the param travels at the dial's extreme. |
| `terms[].space` | `linear` (default), `log` (Hz-like, positive ranges), `magnitude` (signed amounts: "more" grows `|v|` in its own sign). |

Order of `dials` in the file = display order on every card they serve. The file order of `terms` is
irrelevant to the math.

### 2.2 Package `internal/synthdial`

Pure, no dependency on `internal/ui` or `internal/audio` (imports only
`internal/schema`). Owns everything about dials that is not drawing or input:

- **Types**: `File`, `Dial`, `Term`, `Space`.
- **`Load(b []byte) (File, error)`** — decode + strict schema version +
  structural validation (§2.4 rules that need no schema).
- **`Range{Min, Max float64}`** and a caller-supplied
  `ranges map[string]Range` — the package never sees `audio.ParamDef`.
- **`Morph(anchor float64, pos float64, t Term, r Range) float64`** — the one
  place the math lives (§2.3).
- **`Resolve(f File, card string, live func(param string) (Range, bool)) []ResolvedDial`**
  — the dials on one card for one instrument, each keeping only the terms
  whose param is live there; a dial with zero live terms is dropped; at most
  three per card.

The UI owns rendering, hit areas, session anchors and the
`audio.SetInstrumentParam` writes. Adding a new kind of dial = editing the
JSON (+ its i18n strings). Only a new `space` needs Go.

### 2.3 The morph

Inputs: anchor value `a` (the tuned value), dial position `p ∈ [0,1]`, term
`{w, space}`, range `[lo, hi]`. Let `d = 2p − 1 ∈ [−1, 1]` and
`s = sign(w)·sign(d)`, `k = |w|·|d|`.

- **linear**: normalise `n = (a − lo)/(hi − lo)`. If `s > 0`,
  `n' = n + (1 − n)·k`; if `s < 0`, `n' = n − n·k`; `d = 0` → `n' = n`.
  Result `lo + n'·(hi − lo)`.
- **log**: same, in `ln` space over `[max(lo, ε), hi]` (`ε` = 1e-6 of `hi`
  when `lo ≤ 0`).
- **magnitude**: let `σ = sign(a)` (`+1` when `a == 0`), `m = |a|`, and
  `edge = hi` if `σ > 0` else `−lo`. Apply the linear rule to `m` over
  `[0, edge]`; result `σ·m'`.

Properties (unit-tested): exact at `p = 0.5`; monotone in `p` for each term;
always within `[lo, hi]`; at `p = 1` (or 0) a term with `|w| = 1` reaches the
edge; continuous at the centre. Because no two dials on one instrument share a
param (§2.4 rule 5), dials compose independently and order never matters.

### 2.4 Validation rules

Checked at `Load` (structural) and by the discipline test over the full
instrument roster (schema-aware). Any violation fails the test; at runtime an
invalid file logs an ERROR and the root shows no dials (cards behave exactly
as today) — never a crash.

1. `schema_version` is current.
2. Ids unique; every `cards` entry names a card in the melodic or percussion table.
3. At most **three** dials are *resolved* per card per instrument (`Resolve` keeps the first three with live terms, in file order). The file may list more for a card (e.g. `length` serves both tables); the discipline test fails when the cap hides a dial that has live terms on some shipped instrument, unless that `(instrument, dial)` pair is in a commented allowlist in the test.
4. `weight ∈ [−1, 1]`, non-zero; `space` is known.
5. Within one card **table** (melodic or percussion — a dial belongs to the table of each card it serves), a param appears in **at most one dial**. An instrument uses exactly one table, so the dials on any one instrument never share a param: they compose independently and the re-centre self-check (§3.3) cannot ping-pong. (Across tables a param may be reused with a different direction — melodic Fade-in raises `amp_attack`, percussion Snap lowers it.)
6. Every term's param is declared by at least one shipped instrument
   (`audio.InstrumentParamDefs` over `audio.BuiltinInstrumentIDs`).
7. No term drives an **enum**, **stepped**, or **tuning** param (`pitch`,
   `osc_detune`, `osc_octave`, `post_pitch`, any `ParamDef.Step > 0` or
   `Enum`). Pickers stay pickers; note names stay correct.
8. A term's param must be claimed by at least one of the dial's cards (the
   `buildMacroGroups` claim, i.e. `def.Params` or the registry `Macro` via
   `macroConceptFor`), so a dial never moves params that live in another card.
9. `label` / `gloss` keys exist in the en **and** es catalogues.
10. `metric` is `""` or a registered `fingerprint` descriptor ID.
11. `log` terms only on params with `Max > 0`.

### 2.5 Initial catalogue

One row per dial = one entry in the file, in file (display) order. Weights are
starting values; the real-render direction test (§5.4) is the authority, and
any term it finds backwards is flipped during implementation. `↑` = positive
weight, `↓` = negative, unmarked = `↑`. Family knobs are `gen1_<field>`;
terms an instrument does not declare simply drop out of `Resolve`.

| Dial | Cards (table) | Terms | Metric |
|---|---|---|---|
| Richness | sound (M), source (P) | `fm_op1..4_depth`, `fm_d1..4` | `bright.centroid` |
| Bite | sound (M) | `osc_bow_vel`, `osc_sax_blow` | `bright.centroid` |
| Air | sound (M) | `osc_sax_breath` | `noise.fraction` |
| Bright | brightness (M), tone (P) | `filter_cutoff` log, `post_brightness`, `post_darken` ↓, `harm_mix`, `cym_tone_f` | `bright.centroid` |
| Sizzle | tone (P) | `snare_wire_m`, `snare_noise_m`, `cym_noise_m`, `cym_wash` | `noise.fraction` |
| Edge | brightness (M), tone (P) | `filter_resonance` | `""` |
| Sweep | brightness (M), tone (P) | `filtenv_amt`, `filtenv_decay` | `""` |
| Fade-in | attack (M) | `amp_attack` log, `filtenv_attack` | `env.attack` |
| Snap | punch (P) | `amp_attack` ↓log, `burst_sharp`, `kick_click`, `kick_attack`, `snare_attack`, `tom_stick`, `cym_attack`, `cym_spark`, `atk_amt` | `env.attack` (expect ↓) |
| Length | length (M+P) | `amp_decay`, `post_decay`, `post_decay_rate` ↓, `kick_env1`, `kick_fade`, `kick_mode_decay`, `snare_tail_d`, `snare_noise_d`, `snare_tone_d`, `tom_ring`, `cym_env_tail`, `cym_noise_d`, `fm_dec1..4` | `env.decay` |
| Room | length (M+P) | `kick_reverb`, `tom_room` | `env.total_audible` |
| Hold | length (M+P) | `amp_sustain` | `env.total_audible` |
| Tail | length (M+P) | `amp_release` | `env.release` |
| Spread | width (M) | `unison_detune` | `""` |
| Blend | width (M) | `unison_mix` | `""` |
| Drift | humanfeel (M) | `unison_drift_depth`, `unison_drift_rate`, `ens_scatter` | `""` |
| Vibrato | humanfeel (M) | `ens_vib_depth`, `ens_vib_rate` (0.3) | `mod.sustain_variation` |
| Looseness | humanfeel (M) | `ens_humanize`, `ens_jitter` | `""` |
| Depth | movement (M) | `lfo_depth` | `mod.sustain_variation` |
| Speed (LFO) | movement (M) | `lfo_rate` log | `""` |
| Delay | movement (M) | `lfo_delay` | `""` |
| Body | body (M), boom (P) | `body_mix` | `""` |
| Expression | body (M) | `bow_dynamics` | `""` |
| Weight | boom (P), warmth (M) | `post_body`, `kick_h2`, `kick_h3`, `kick_h4`, `kick_noise`, `snare_tone_m`, `tom_o1`, `tom_o2`, `cym_body` | `""` |
| Ring | boom (P) | `kick_mode_gain`, `cym_body_w` | `""` |
| Drive | warmth (M), grit (P) | `drive`, `post_drive`, `kick_sat`, `sat_k` | `bright.rolloff` |
| Drop | pitchdrop (M+P) | `pitchenv_amt` magnitude, `kick_pe_amt`, `tom_sweep`, `fm_pe_amt`, `pitch_env_amt` | `""` |
| Speed (drop) | pitchdrop (M+P) | `pitchenv_decay` ↓log, `kick_pe_rate`, `pitch_env_rate`, `fm_pe_decay` | `""` |
| Character | voice (M) | `formant_mix`, `formant_dry` ↓ | `""` |
| Breath | voice (M) | `formant_breath` | `noise.fraction` |
| Shift | voice (M) | `formant_shift` log | `bright.centroid` |
| Spread (flam) | flam (M) | `burst1..4_off` | `""` |
| Strength | flam (M) | `burst1..4_amp` | `""` |
| Sharp | flam (M) | `burst_sharp` | `""` |

Reading the table per instrument:
- **Length card**: file order Length · Room · Hold · Tail with the cap of three.
  A violin declares no `kick_reverb`/`tom_room`, so Room drops and it shows
  Length · Hold · Tail; a kick shows Length · Room (+ Hold if it declares a
  live sustain).
- **Rule 5 holds per table**: `amp_attack` is in Fade-in (melodic) and Snap
  (percussion); `burst_sharp` in Snap (percussion) and Sharp (melodic) — each
  pair spans two tables, never one. `post_body` sits only in Weight (serving
  boom and warmth), and `body_mix` only in Body.
- Dial ids are unique slugs (`speed-lfo`, `speed-drop`, `spread-flam`); the
  parenthesised names above are display disambiguation only — the visible
  label is `Speed` / `Spread` in both cases.

New i18n keys: `synth.dial.<id>` and `synth.dial.<id>.gloss` for every dial,
added to `keys.go` (and its exhaustive key list) and both catalogues.

## 3. State, anchors, live apply and commit

### 3.1 Session state

`DrumView.synthDials map[dialKey]*dialState`, `dialKey{inst, dialID}`.
`dialState{pos float64; anchor map[string]float64}`. Created lazily on the
first move of a dial. Never serialised. Cleared by `Game.Close`.

### 3.2 Turning a dial

1. On the first move with no state: capture `anchor` = the current value of
   each live term param (from the instrument document), `pos = 0.5`.
2. Each detent: `pos = clamp(new)`, and for each live term
   `v = synthdial.Morph(anchor[param], pos, term, range)` →
   `audio.SetInstrumentParam(inst, param, v)` (live apply; the per-call
   coalescing that knob drags use applies unchanged; the lane picks up the
   voice revision).
3. Release / popup Accept / double-tap reset: `dv.commitInstrumentParams(inst)`
   once.

### 3.3 Re-centring (one self-check, not N hooks)

Each Synth-tab layout, for every dial with state, compare each live term's
current document value with `Morph(anchor, pos)`. Any difference beyond
1e-9 relative → delete the state (the dial reads 50 % on the current sound).
This catches every path that changes a driven param without enumerating them:
raw knob edit, Reset, Restore shipped, import, undo/redo re-import, the JS
bridge, instrument switch then edit. The check reads values the layout
already holds (no per-frame allocation; budget test §5.6).

### 3.4 Undo

A dial gesture is one undo step (the commit). Undo restores the params through
the normal re-import; the self-check then re-centres the dial on the restored
sound. Redo likewise.

## 4. Export / import

Dials are a lens. A dial edit is a write to the row's instrument document via
`audio.SetInstrumentParam`, exactly like a raw knob edit, so it is exported in
`instrument_docs` and re-imported with no new code. Contract:

- **Sound round-trips**: turn dials → export → wipe every global → import →
  identical params and identical master mix.
- **Dials never reach a file**: no dial id, position, anchor or definition
  appears in the project JSON, an instrument bundle, prefs, or the user
  library. The project format, `instrument.Doc` and the bundle format are
  unchanged.
- **Imported dials start centred** on the imported sound (no state survives
  the import; the self-check also drops any stale state for a re-used id).
- **Viewing / tapping without moving changes nothing**: opening the root,
  capturing nothing, opening and cancelling a dial popup leaves the export
  byte-identical.

## 5. Testing

### 5.1 `internal/synthdial` (pure, `-tags test`)
- `Morph`: exact at centre; monotone per term over a dense `p` sweep; within
  range for random anchors / weights / spaces; endpoint reaches `|w|` of the
  way; log and magnitude modes (incl. `a = 0`, negative anchors); centre
  continuity.
- `Load`: rejects each structural rule (§2.4 1–4, 11) with a named error;
  accepts the shipped file.
- `Resolve`: drops dead terms, drops empty dials, caps at three in file order.

### 5.2 Discipline over the shipped file + roster (`internal/ui`, `-tags test`)
`TestSynthDialsFileIsValid` — schema-aware rules 5–10 across
`audio.BuiltinInstrumentIDs`; `TestEveryDialTermIsClaimedByItsCard` (rule 8);
`TestNoDialDrivesAPickerOrTuning` (rule 7); `TestDialI18nKeysExist` (rule 9).

### 5.3 Real input through `Game.Update` (`-tags test`)
- Drag a dial on violin Brightness → the **audio layer's** document values of
  `filter_cutoff` / `post_brightness` / `post_darken` moved in their declared
  directions; one `EventInstrumentParamsCommitted` and one undo step for the
  whole gesture.
- Tap a dial → `ValueWheel` opens; arrows step; Enter commits once; Esc
  restores the opening values bit-exactly.
- Tap the card body → drills (unchanged); press on a dial never drills.
- Double-tap → anchor values restored exactly, dial at 50 %.
- Raw knob edit of a driven param → dial re-centres; undo after a dial
  gesture → params restored and dial re-centres.
- No dial gesture ever auditions (`synth_no_auto_audition` style).
- Dimmed dial over a bypassed stage still adjusts.

### 5.4 Real-render direction (`!test`, `make test-real`)
`TestEveryDialMovesItsMetric`: for every shipped instrument × every dial
resolved on it, render at `p = 0, 0.5, 1` through the real C path; `p = 0.5`
renders identical to the untouched instrument; with a `metric`, the
descriptor moves monotonically in the declared direction (Snap: attack ↓);
without one, the `p = 0` and `p = 1` renders both differ from `p = 0.5`
(audible). Skips loudly under `-tags test`.

### 5.5 Export / import (extends the existing guards)
- **Fast path** (`-tags test`), new `synth_dial_export_import_test.go`:
  turn two dials through real input → export → reset → import → every driven
  param equal to the pre-export value, re-export byte-identical; the exported
  JSON contains no `dial` key nor any dial id; after import the dials have no
  state (read 50 %); open root + open/cancel a dial popup → export
  byte-identical to before.
- **Real build** (`!test`), extend the headline identity harness
  (`export_import_master_audio_*_test.go`): `applyPostSaveEdits` also turns a
  dial (through the dial input path) on one synth row, so
  `TestExportImportMasterMixIsByteIdentical/{same-session,clean-machine}` and
  `…AcrossReExport` cover dial edits; `TestHeadlineEditsAreAudible` gains an
  assertion that the dial edit alone moves the master mix (no vacuous
  identity).

### 5.6 Budgets and visuals
- Alloc + draw-call budgets (`synth_drawcall_budget_test.go`,
  `synth_live_param_edit_alloc_test.go`) re-measured with dials on the root;
  steady frames allocate nothing for dials; card sprites keyed on dial
  positions.
- Theme guards (`theme_drift_test.go`) pass on the new scenes.
- Scenes (kept as regression surfaces): `crop_synth_dials_root` (violin),
  `crop_synth_dials_perc_root` (kick-808), `crop_synth_dials_turned` (Bright
  at 85 %, graph morphed), `crop_synth_dials_popup` (ValueWheel open on a
  dial), each with `MobileSetup`. Captured desktop + mobile and inspected
  before the work is called done; prefs isolated with `XDG_CONFIG_HOME`.

## 6. Files

| File | Change |
|---|---|
| `internal/synthdial/{dial.go,morph.go,load.go,resolve.go}` + tests | New package. |
| `internal/assets/synth_dials.json`, `internal/assets/dials_embed.go` | New data + embed. |
| `internal/i18n/{keys.go,catalog_en.go,catalog_es.go}` | Dial label / gloss keys. |
| `internal/ui/synth_dials.go` | Load-once, `Resolve` per card (cached with the macro-group cache key), session state, self-check, live apply / commit. |
| `internal/ui/synth_dial_render.go` | Arc dial drawing (token colours, Visualizers family). |
| `internal/ui/synth_macro_render.go` | Card layout: dial row replaces gauges; scrollable root grid; dial hit areas at card z + 1. |
| `internal/ui/synth_dial_input.go` | Dial hit adapter (drag / tap / double-tap / wheel), `ValueWheel` binding, key claimant. |
| `internal/ui/scene_catalog.go` | The four scenes. |
| `internal/ui/*_test.go` | §5.2–5.5. |
| `CLAUDE.md` | One paragraph under the Synth tab row: dials = metadata in `synth_dials.json`, `internal/synthdial`, never persisted. |

## 7. Implementation notes — where the build deviated (2026-10-09)

- **No double-tap reset.** A tap opens a modal wheel, so a second tap lands on
  its scrim. Instead the drag has a magnetic centre (±2 %, writes the anchor
  back exactly) and 0 is a row in the wheel.
- **Mouse wheel over a dial scrolls the grid**, like every un-armed synth
  control, instead of adjusting the dial.
- **Readout is a signed offset** (−100…+100, 0 = as tuned) rather than 0–100 %,
  matching the bipolar arc; the wheel uses the same range. The wheel title is
  the dial label only (the header line cannot fit the one-liner).
- **Dials never dim — an inaudible dial does not resolve.** The real-render test
  found 819 problems on the first run, most of them dials whose terms sat in a
  bypassed stage. A term is now live only when its stage is ON (single-enable
  stages; VOICE has one enable per gen-slot family and is not gated), plus two
  metadata gates: `requires` (a param that must be non-zero, e.g.
  `body_mix` needs `body_model`) and `when` (a named engine condition mirroring
  the C dispatch: `ensemble` — the unison path runs only for a wavetable osc
  with no active pitch envelope or LFO vibrato; `pitchenv` — wavetable/FM osc;
  `release` — sustain > 0 or decay ≥ 0.25 s). Final run: 1375 (instrument, dial)
  pairs, 0 inaudible, 0 wrong-direction.
- **`metric_falls`** replaces "Snap: attack ↓" prose. Metrics that disagreed
  across instruments were removed (Drive, Depth, Vibrato, Shift, Bite → audible
  only); Length measures `env.total_audible`, Drive's `rolloff` was
  bin-quantized.
- **Catalogue changes from the render test / discipline tests:** Air removed
  (no shipped instrument surfaces the sax params); Bite = bow velocity only,
  weight flipped negative; Snap's cymbal attack flipped negative; Fade-in lost
  `filtenv_attack` (claimed by Brightness); Edge is melodic-only; Length card
  order Length · Room · Tail · Hold with `length/hold` the accepted cap victim
  on drums; `kick_noise` moved to Weight.
- **Card tap drills on RELEASE** (tap-vs-scroll arbiter), because the dials
  make the root scroll.
- Every `Import` (incl. undo/redo re-imports) clears dial state: the
  value-equality self-check cannot see a load that restores exactly the values a
  dial last wrote.
- **Popup scene is `synth_dials_popup`, not `crop_…`**: the wheel draws outside
  the Synth panel, so a panel crop would cut it off.
- **Mobile root (2026-10-10)**: a dial card row is ~half of a phone's cards area
  (185 px area, 86 px row), and the ControlGrid peek band (0.4 row) cost the
  second row — one card above ~110 px of blank, with the scroll top-fade washing
  out that card's title. The root grid now opts out (`SetPeekBand(false)`; the
  drilled view, which shares the grid, opts back in) and shows two cards; the
  scrollbar still signals overflow. The dial scenes scroll the root to their
  subject card (`ScrollToIndex`), so the mobile captures show the turned dials
  and the open wheel instead of the Sound card.
- **Audibility pass (2026-10-10).** A user soloing a row heard no change from
  the dials. Root causes, all fixed or ratcheted:
  - The audibility gate (`samePCM` on a raw 4 s render) was vacuous; the new
    contract measures the PLAYED voice (`TestEveryDialDirectionIsAudibleWhenPlayed`):
    1181 of 2750 directions were inaudible → 251 of 1943, all listed with a
    reason in `testdata/synth_dial_known_weak.json` (ratchet).
  - 312 directions could not move at all (every term at its edge): dials are
    now ONE-SIDED (`synthdial.Reach`), and per-term `jnd` drops travel that is
    too small to hear.
  - New metadata: `jnd`, `requires_min` / `requires_below`, the `settles`
    condition (voice length = `render.beats` × 60/BPM).
  - A detent published one half-applied document per term; now one
    `audio.SetInstrumentParamsLive` write.
  - Drum Length terms pointed decay RATES up (shorter); flipped. Crisp
    (`post_brightness`) was a level cut, not a brightener: removed from Bright,
    then (2026-10-12) the FINISH stage was fixed in the engine — Crisp is a
    3 kHz high shelf and Low Boost a 200 Hz low shelf (0..+12 dB), Resample is
    band-limited (`finish_stage_response_test.go`) — and Crisp is a Bright term
    again (weight 1.0). Weak directions 251 → 234 (Bright up 37 → 21).
  - Found on the way: a C wavetable init race that corrupted oscillator tables
    for a whole session under concurrent cold renders (fixed, `src/c/once.h`).

