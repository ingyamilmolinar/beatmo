# Synth tab — ABSOLUTE macro dials

Date: 2026-10-13
Status: implemented 2026-10-14 (working tree, uncommitted) — see §8 for where the build deviated
Supersedes: §2.3 (the morph), §3 (state, anchors, re-centring) and the
"relative" / "session memory" rows of the decision table of
`2026-10-09-synth-macro-dials-design.md`, and its §7 notes on the magnetic
centre, one-sided dials and `jnd`. Everything else in that spec (where dials
live, metadata-only dials, never persisted, live apply + one commit, the
gates `requires` / `when`, the played-voice audibility oracle) stays.

## Problem

A macro dial today is RELATIVE: its centre (50 %) means "the instrument as it
was when the dial was first moved", and that anchor lives in session memory
(`DrumView.synthDials`). So the same instrument document shows different dial
positions depending on history: every restart, import, undo/redo, Reset or raw
knob edit re-centres the dial. The knob state is not a function of the sound.
About 150 lines of anchor / self-check / snapshot / read-back / one-sided
`Reach` code exist only to manage that state.

A roster review (150 shipped instruments, 2026-10-13) also found:

- cards that never carry a dial: percussion **Source** (51 drums, pickers only),
  melodic **Sound** on 86 of 99 voices, tom **Punch** (`gen1_tom_stick` was in
  the old spec's Snap row but never reached the JSON), clap **Attack**;
- drum decay params no dial drives (`gen1_env_fast_rate` on 66 instruments,
  `kick_env0`, `kick_hold`, `cym_rattle_decay`, `cym_rattle_hold`,
  `cym_decay_tilt`, `env_tail_rate`, `env_tail_mix`), so Length is weak on drums;
- visible continuous params no dial reaches: the attack boost `gen1_atk_amt`,
  `filtenv_attack`, bow pressure / position / brightness, `formant_sing`,
  `formant_morph_rate`, cymbal `cym_beat` / `cym_tone_m` / `cym_mode_hi` /
  `cym_rattle`, kick `kick_hiss`;
- the three-per-card cap hides Hold on every drum (`length/hold`, accepted in a
  test allowlist).

## Goal

1. A dial's position is a **pure function of the instrument document**. Same
   document → same knob, on every machine, after restart, import, undo/redo.
   Nothing about a dial is stored anywhere.
2. Every term's position on its dial is in **absolute** param units, declared in
   the metadata ("Fade-in 0 = 1 ms, 100 = 2 s").
3. Every dial the UI offers has a **measurable effect**: turning it moves at
   least one low-level synth param by a meaningful amount AND changes the
   PLAYED audio measurably (the played-voice oracle). No exceptions list at
   the end (§6.3).
4. A broader, still approachable, catalogue: more dials and more terms, each
   high-level in name and correct in what it morphs.

## Decisions taken with the user (2026-10-13)

| Question | Decision |
|---|---|
| Turn model | **Proportional shift.** The reading is the weighted mean of each term's absolute position; turning moves every term proportionally toward the edge being turned to, so each term keeps its offset (the instrument keeps its character) and the reading lands exactly on the target. |
| Snap-to-curve / stored macro | Rejected. Snapping flattens an instrument's tuned relations on first touch; a stored macro changes the document schema and the render path. |
| Factory tick | Not in scope. |
| Catalogue | Fill the gaps, add new dials, be generous ("experiment"), but every knob must be measurable in params AND audio. |
| Audibility | Recalibrate the oracle to the absolute model and regenerate the known-weak list; the target is an EMPTY list. |

Non-goals: the audio engine, the param block / ABI, `instrument.Doc`, the
project / bundle formats, a factory tick, dials inside the drilled view,
dials for the WAV-clone MODAL / STRIKE / NOISE stages (no shipped instrument
enables them, so no dial on them could be measured — revisit when one ships).

## 1. The model

### 1.1 Term coordinate

A term maps its param's value `v` onto the dial axis through an absolute SPAN:

```
u(v) = (S(v) − S(from)) / (S(to) − S(from))        (unclamped)
```

| `space` | `S(v)` | Notes |
|---|---|---|
| `linear` (default) | `v` | |
| `log` | `ln(max(v, min(from,to)))` | `from`, `to` > 0. A value below the span floor (e.g. an attack of exactly 0) reads as the floor; it is only rewritten if the term moves. |
| `magnitude` | `|v|` | For signed amounts (a pitch drop). The value's own sign is kept on write (0 takes `+`). `from`, `to` ≥ 0. |

`from` / `to` default to the param's `Min` / `Max` (for `magnitude`: `0` and the
edge on the value's sign side). `to < from` means the param FALLS as the dial
rises (Darken: `from 1, to 0`). The inverse `v(u)` is exact for `u ∈ [0,1]`.

### 1.2 Reading (pure)

```
Reading(values) = Σ wᵢ · clamp(uᵢ, 0, 1) / Σ wᵢ          over the dial's LIVE terms
```

`wᵢ = weight` (> 0, default 1): the term's influence on the reading. The UI
shows `round(100 · Reading)` as an integer 0…100.

Liveness (which terms are live) is itself a function of the document (stage
enables, `requires`, `when`, the voice length) — so the reading is still a pure
function of the document plus the BPM that `settles` reads.

### 1.3 Turn (pure)

`Turn(start values, target ∈ [0,1]) → values`. Let `r = Reading(start)`.

- `target == r` → `start` verbatim (bit-exact).
- Turning UP (`target > r`): every term with `uᵢ < 1` moves to
  `uᵢ + (1 − uᵢ)·k`; terms with `uᵢ ≥ 1` stay.
- Turning DOWN: every term with `uᵢ > 0` moves to `uᵢ·(1 − k)`; terms with
  `uᵢ ≤ 0` stay.
- `k ∈ [0,1]` is chosen so `Reading(Turn) = target`: closed form when every
  `uᵢ ∈ [0,1]` (`k = (t−r)/(1−r)` up, `(r−t)/r` down — the weighted mean of
  the moved coordinates is then exactly `t`); otherwise bisection (Reading is
  continuous and monotone in `k`; 60 iterations).
- `target = 1` (or `0`) → `k = 1`: every term sits exactly at `to` (`from`).

Properties (unit-tested): exact landing; monotone in `target`; endpoints exact;
identity at `r`; a term never moves the wrong way; deterministic (no maps
iterated in an order that changes the floating result).

Turning moves every term TOWARD the same end, so the instrument's character is
kept: Bright on a dark bass and on a bright violin both rise, the bass stays
darker than the violin at any equal reading except 0 and 100.

### 1.4 What is deleted

`DrumView.synthDials`, `synthDialState`, `synthDialSelfCheck`,
`synthDialReadBack`, `snapshotSynthDial` / `restoreSynthDial` (replaced by the
gesture capture), the Import-clears-dial-state rule, `synthdial.Morph`,
`Reach`, `ResolveReachable`, `LiveTerm.travels`, `Term.JND`, the signed
`weight`, the magnetic centre (`synthDialCentreSnap`), the signed offset
readout (`synthDialOffset*`), `synthDialCapAccepted`.

## 2. Metadata format

`internal/assets/synth_dials.json` (same envelope, strict schema version,
unknown fields rejected):

```json
{"id": "fadein", "cards": ["attack"], "label": "synth.dial.fadein",
 "gloss": "synth.dial.fadein.gloss", "metric": "env.attack",
 "terms": [
   {"param": "amp_attack", "from": 0.001, "to": 2.0, "space": "log"}
 ]}
```

| Field | Meaning |
|---|---|
| `id`, `cards`, `label`, `gloss`, `metric`, `metric_falls` | As before. |
| `rest` | `"start"` when the dial's 0 is a natural OFF (Drive, Weight, Room…). Exempts it from the calibration-spread rule (§5.2), nothing else. |
| `terms[].param` | Existing schema param. |
| `terms[].from`, `.to` | Absolute values at dial 0 / 100 (optional, default the param range). |
| `terms[].space` | `linear` / `log` / `magnitude`. |
| `terms[].weight` | > 0, default 1: influence on the reading. |
| `terms[].requires`, `requires_min`, `requires_below`, `when` | Unchanged gates. |

Removed fields: signed `weight` semantics, `jnd`.

### 2.1 Validation

Structural (`synthdial.Load`): unique ids, ≥1 card and term, label + gloss,
`weight > 0`, known space, `from ≠ to` when both set, log spans positive,
magnitude spans ≥ 0, `requires_*` only with `requires`.

Schema-aware (discipline tests over the roster, §6.2): the existing rules
(cards exist, one param → one dial per table, declared + claimed, no picker /
stepped / tuning param, i18n keys in every locale, metric is a descriptor,
gates name real conditions / params) PLUS:

1. **No self-referential gate**: a term's `requires` / `when` may not read a
   param driven by its own dial (turning would make the term vanish mid-turn
   and the reading jump). Today's violators: Length `amp_decay` (`settles`
   reads `amp_decay`) and `post_decay_rate` (`requires post_decay`).
   `post_decay_rate` leaves Length (undialed on all 150 instruments anyway);
   `amp_decay` loses `settles`, and Length's audibility on long-voice
   instruments is carried by the measurability contract (§6.3).
2. **Spans inside the param range** on every instrument that declares it.
3. **Shipped values inside spans**: every shipped instrument's live term value
   lies in `[min(from,to), max(from,to)]` — so a shipped reading is honest.
4. **Calibration spread** (§5.2).

## 3. UI

### 3.1 Dial

- Arc: track over the full 270°; value arc from the 0 end to the reading. No
  centre tick, no half-only track.
- Readout: the integer `0…100`.
- Drag: vertical, 200 px sweeps 0 → 100, quantised to integers. A ±1 magnet at
  the PRESS-TIME reading: landing there writes the press-time values back
  exactly (you can always undo a drag by dragging back).
- Tap: the shared `ValueWheel` over `NumberModel{0..100}`, ladder 1/5/10/25.
  Accept = one commit; Esc restores the open-time values verbatim.
- Mouse wheel: still scrolls the grid (unchanged).

### 3.2 Gesture scope — the only state

Press / wheel-open captures `{inst, card, dial, resolved terms, start values}`.
Each detent: `values = Turn(start, target)` → ONE
`audio.SetInstrumentParamsLive`. Release / Accept → ONE
`commitInstrumentParams` (one undo step, one log line). Esc / cancel → the
start values in one live write. The capture is dropped when the gesture ends.
While a gesture is live, the card's dial set is FROZEN (the root's macro cache
is not rebuilt for the dialled instrument until release): a turn may flip
another dial's liveness (Length moves `amp_decay`, which gates Hold's
`settles`), and the row must not reshuffle under the finger.

### 3.3 Up to six dials per card

`MaxDialsPerCard` 3 → 6. The dial row lays out as rows of at most three cells
(`macroCardDialRects` → `n ≤ 3`: one row; `4..6`: two rows, the second
left-aligned on the same column grid). Card height grows by one dial row when a
card has more than three dials; the root grid already scrolls. If the cells of
a row would be narrower than one arc, the row breaks into more rows instead of
drawing nothing (today it silently returns no rects). Verified by screenshot
on desktop and mobile (§6.6); if six does not fit legibly on mobile the cap is
lowered there through a density field, not by dropping metadata.

## 4. Catalogue

`↑` the param rises with the dial (`from < to`), `↓` it falls. Spans in
brackets are starting values; unlisted spans default to the param range and
are tuned by the calibration test (§5.2). Directions are confirmed by the real
render (§6.4) and flipped if wrong. `M` = melodic table, `P` = percussion.

### 4.1 Melodic

| Card | Dials (display order) |
|---|---|
| sound | **Richness** (FM op depths ↑, `fm_d1..4` ↑) · **Bite** (bowed: `osc_bow_slope` ↑ pressure, `osc_bow_vel` ↓) · **Glassy** (`osc_bow_pos` ↓ — bowing toward the bridge) · **Brilliance** (`osc_bow_loss`, direction by metric) |
| brightness | **Bright** (`filter_cutoff` ↑ log [80 Hz → 18 kHz], `post_brightness` ↑ [0 → 1], `post_darken` ↓ [1 → 0], `gen1_harm_mix` ↑) · **Edge** (`filter_resonance` ↑ log [0.7 → 12]) · **Sweep** (`filtenv_amt` ↑, `filtenv_decay` ↑) · **Bloom** NEW (`filtenv_attack` ↑ log [1 ms → 1 s]; requires `filtenv_amt` ≠ 0) |
| attack | **Fade-in** (`amp_attack` ↑ log [1 ms → 2 s]) · **Click** NEW (`gen1_atk_amt` ↑, `gen1_atk_rate` ↓ — a longer, louder attack boost) |
| length | **Length** (`amp_decay` ↑ log [20 ms → 4 s], `post_decay` ↑, `gen1_fm_dec1..4` ↑, `gen1_env_fast_rate` ↓ log, `gen1_env_tail_rate` ↓ log, `gen1_env_tail_mix` ↑) · **Tail** (`amp_release` ↑ log [10 ms → 4 s], `when: release`) · **Hold** (`amp_sustain` ↑, `when: settles`) |
| width | **Spread** (`unison_detune` ↑) · **Blend** (`unison_mix` ↑) |
| humanfeel | **Drift** · **Vibrato** · **Looseness** (terms unchanged) |
| movement | **Depth** (`lfo_depth` ↑) · **Speed** (`lfo_rate` ↑ log) · **Delay** (`lfo_delay` ↑) |
| body | **Body** (`body_mix` ↑) · **Expression** (`bow_dynamics` ↑) |
| warmth | **Weight** (`post_body` ↑ [0 → 1]) · **Drive** (`drive`, `post_drive`, `gen1_sat_k` ↑) |
| pitchdrop | **Drop** (magnitude) · **Speed** (unchanged terms) |
| voice | **Character** (`formant_mix` ↑, `formant_dry` ↓) · **Breath** (`formant_breath` ↑) · **Shift** (`formant_shift` ↑ log) · **Shine** NEW (`formant_sing` ↑ — the singer's-formant ring) · **Talk** NEW (`formant_morph_rate` ↑ log; `when: morphs` = `formant_morph_to` ≠ `formant_vowel`) |
| flam (+ punch P) | **Spread** · **Strength** · **Sharp** (melodic only — `burst_sharp` is Snap's on drums) |

### 4.2 Percussion

| Card | Dials |
|---|---|
| punch | **Snap** (+ `gen1_tom_stick` ↑, `gen1_atk_amt` ↑ — fills the empty tom Punch card) · **Spread** / **Strength** (the flam dials, shown when the Burst stage is on) |
| boom | **Weight** · **Ring** · **Body** · **Air** NEW (`gen1_kick_hiss` ↑) |
| pitchdrop | **Drop** · **Speed** |
| tone | **Bright** (+ `gen1_cym_mode_hi` ↑ log) · **Sizzle** (+ `gen1_cym_rattle` ↑) · **Shimmer** NEW (`gen1_cym_beat` ↑, `gen1_cym_tone_m` ↑ — the plate's metallic ring) · **Sweep** |
| length | **Length** (+ `gen1_kick_env0` ↓, `gen1_kick_hold` ↑, `gen1_cym_rattle_decay` ↑, `gen1_cym_rattle_hold` ↑, `gen1_cym_decay_tilt` ↓, `gen1_env_fast_rate` ↓, `gen1_env_tail_rate` ↓, `gen1_env_tail_mix` ↑; drum decay RATES on log spans) · **Room** · **Tail** · **Hold** (the cap no longer hides it) |
| source | none — this card holds the model pickers only (variant, generator wave); its continuous params belong to Sizzle (noise) and Weight (tone), and one param has one dial. |
| grit | **Drive** |

`rest: "start"` dials: Drive, Weight, Room, Depth, Delay, Vibrato, Looseness,
Drift, Ring, Body, Click, Shimmer, Air, Bloom, Talk, Shine, Breath.

New condition `morphs` (mirrors the C formant dispatch: the morph LFO runs only
toward a different vowel). New i18n keys `synth.dial.<id>` /
`synth.dial.<id>.gloss` in en + es for Glassy, Brilliance, Bloom, Click, Shine,
Talk, Air, Shimmer, plus updated glosses for dials whose terms changed. Glosses
say what the dial does in plain words ("Bloom — how slowly the brightness
opens up"), never the param name.

Explicitly left as raw knobs (no correct single-direction "more"): pitch-like
params (`kick_mode_detune`, `snare_tone2`, every `*_tune` / `*_freq` — note
names depend on them), cymbal sparkle SHAPE (`cym_spark_f`,
`cym_spark_w`, `cym_hf_corner`), `out_clamp` (0 = off is a discontinuity),
`out_scale` (level, not timbre), FM op ratios (tuning).

## 5. Calibration

### 5.1 Spans are design data

Spans are chosen musically (the whole sweep should be useful) and must hold
every shipped value (rule 3). A span narrower than the param range is fine:
raw knob edits outside it read as 0 / 100 and move continuously back in (§1.3).

### 5.2 Spread gate

For each dial without `rest`: across the shipped instruments where it is live,
the median reading lies in [20, 80] and at most 25 % of them read within 5 of
either end. Dials with `rest: "start"` must have their span start at the
param's neutral value (0 dB, 0 drive, 0 depth…). The test logs the reading
distribution per dial (min / median / max) on every run.

## 6. Testing

### 6.1 `internal/synthdial` (pure, `-tags test`)

`Coord` / `Value` round-trip per space; `Reading` weighted mean with clamping;
`Turn`: exact landing (dense target sweep, random spans / values / weights /
spaces), monotone, endpoints exact, identity at the start reading, out-of-span
terms never move the wrong way and reach the span, magnitude keeps sign, log
floor, bisection agrees with the closed form where both apply. `Load` rejects
each structural fault; accepts the shipped file.

### 6.2 Discipline over the shipped file + roster (`-tags test`)

Existing rules (kept), rules 1–4 of §2.1, the cap test against six, coverage
(every dial reachable on a shipped root or after adding its stage).

### 6.3 Measurability contract — the headline guard

`TestEveryDialIsMeasurable` (`!test`, real C renders through
`audio.RenderAsPlayed`, `make test-real`). For every shipped instrument × every
dial resolved on it, AND for the dials that appear only once their stage is
switched on (Flam, Bloom on voices without a filter envelope…) on the
instrument with that stage on:

- **Params**: each END that is ≥ 10 units from the shipped reading moves at
  least one term by ≥ 5 % of its span (in its coordinate).
- **Audio**: that end is AUDIBLE vs the shipped sound in the played voice (the
  existing oracle: third-octave band envelope ≥ 1 dB over the note, or ≥ 2 dB
  over ~70 ms, or partials shift ≥ 5 cents), AND the 0 ↔ 100 sweep is audible.
- Remedies when a case fails, in order: fix the span or terms; add a gate
  (`requires` / `when`) so the dial does not show where it cannot be heard;
  drop the term or dial. An inaudible dial must not resolve.
- `testdata/synth_dial_known_weak.json` is regenerated against this oracle and
  must END EMPTY. While the work is in progress it is a ratchet; any entry left
  at the end needs the user's explicit sign-off with a reason.

`TestEveryDialMovesItsMetric` is kept with the absolute reference: from the
shipped reading to each end ≥ 10 units away, a dial with a `metric` moves it the
declared way.

### 6.4 Real input through `Game.Update` (`-tags test`)

- Drag a dial → the AUDIO layer's values equal `Turn(start, target)` exactly;
  one `EventInstrumentParamsCommitted`, one undo step.
- Drag away and back to the press reading → bit-exact restore.
- Tap → wheel; arrows step; Enter commits once; Esc restores verbatim.
- Card tap drills; a press on a dial never drills.
- A raw knob edit moves the reading by exactly `Reading(new values)`.
- **Determinism** (`TestSynthDialReadingIsAFunctionOfTheDocument`): turn three
  dials on two instruments, record every reading on the root; then (a) a fresh
  Game importing the same export, (b) export → wipe every global → import,
  (c) undo + redo, (d) close + reopen the Synth tab: every reading identical.
- No dial gesture auditions.
- The card's dial set does not change during a gesture that flips another
  dial's liveness (Length × Hold).

### 6.5 Export / import

The existing guards stay (`TestSynthDialEditsRoundTripThroughTheProjectFile`,
`TestSynthDialStateNeverReachesTheProjectFile`, the headline master-mix
identity harness turning a dial). The "import clears dial state" case is
deleted — there is no state.

### 6.6 Visual

Scenes (kept as regression surfaces), updated for the absolute readout:
`crop_synth_dials_root` (violin), `crop_synth_dials_perc_root` (kick-808),
`crop_synth_dials_turned` (Bright set to an absolute 85),
`synth_dials_popup`; new: `crop_synth_dials_six` (a card with more than three
dials — the drum Length card), `crop_synth_dials_voice` (a formant voice:
Character · Breath · Shift · Shine · Talk). Captured desktop + `MOBILE=1` with
`XDG_CONFIG_HOME` isolated, and inspected before the work is called done. Alloc
and draw-call budgets re-measured (`synth_drawcall_budget_test.go`).

## 7. Files

| File | Change |
|---|---|
| `internal/synthdial/{dial.go,coord.go,turn.go,resolve.go}` + tests | Span coordinate, `Reading`, `Turn`; delete `Morph` / `Reach`. |
| `internal/assets/synth_dials.json` | Every term re-expressed as spans; new terms and dials (§4). |
| `internal/i18n/{keys.go,catalog_en.go,catalog_es.go}` | New dial keys + revised glosses. |
| `internal/ui/synth_dials.go` | Delete session state; gesture capture; `morphs` condition; liveness unchanged otherwise. |
| `internal/ui/synth_dial_input.go` | Drag / wheel through `Turn`; press-reading magnet. |
| `internal/ui/synth_dial_render.go`, `synth_macro_render.go` | 0…100 arc + readout; multi-row dial layout; cap 6. |
| `internal/ui/scene_catalog.go` | Updated + new scenes. |
| `internal/ui/*_test.go`, `internal/ui/testdata/synth_dial_known_weak.json` | §6. |
| `CLAUDE.md` | Rewrite the "Synth macro dials" paragraph: absolute, pure reading, no session state, measurability contract. |
| `docs/superpowers/specs/2026-10-09-synth-macro-dials-design.md` | Status line points at this spec for the superseded sections. |

## 8. Implementation notes — where the build deviated (2026-10-14)

- **Spans clip to each instrument's own range** (`SpanFor`): param ranges differ
  per family (`gen1_env_fast_rate` is [0.5, 12] on the bass family, [0, 2000]
  elsewhere, and conga ships 18). Rule 2 became "a declared end lies within the
  WIDEST range some instrument declares".
- **Resting dials read 0 at or past their off end**: Edge starts at Q 0.707
  (Butterworth — below it is inaudible), Fade-in at 5 ms and is now a resting
  dial; a shipped value past the off end is honest at 0 (rule 3 allows it).
- **Calibration near-end rule counts only "widenable" ends**: a sound pinned at
  an end its params cannot pass (an organ's sustain of 1) is the param's truth.
- **Rows hold up to four cells where they fit** (desktop cards 3, phone cards 4),
  not "at most three": at three per row every four-dial card doubled every
  phone card's height and a phone showed one card again (the 2026-10-10 fix).
- **Engine mirrors added**: `filtenv` (the filter envelope is ignored on the
  LFO-cutoff path and with the filter off), `morphs`, and `synthDialRangeCaps`
  (Tail / Fade-in ≤ half the voice). The C ADSR caps where the release STARTS
  at half the voice but keeps the full release TIME for the ramp, so a raw
  Release past half the voice gets louder and hard-cuts — an engine defect left
  unfixed here (it would change shipped sounds, goldens and the WASM build).
- **Catalogue changes from the real renders**: Brilliance (`osc_bow_loss`)
  removed — it moves the centroid opposite ways on cello vs violin; Air and
  Tail lost their metrics (descriptor blind spots; audibility proven by the
  contract); `gen1_env_tail_*` dropped from Length (no card claims them);
  Richness weights its main modulator; formant dials Shift/Shine/Talk require
  `formant_mix ≥ 0.15`; knob ranges `snare_tone_m` → [0, 3] and
  `kick_mode_decay` → [0, 60] (shipped values exceeded them).
- **The audibility oracle gained a per-band criterion** (one third-octave band
  ≥ 3 dB): a +20 dB Q-12 resonance averaged to 0.67 dB across the spectrum.
- **The metric test measures the played voice** (`RenderAsPlayed`), like the
  contract; `dialMetricExceptions` is empty.
- **The weak list did not end empty**: 635 of 6280 checked ends/sweeps (372 on
  shipped roots, 263 only with a stage switched on), every dial with a reason —
  content-dependent cases (filter dials on sine / additive / band-limited
  sources, near-ceiling sustain, short drum transients). Hiding them needs
  either runtime measurement or a measured table, both rejected on 2026-10-10;
  left for the user's sign-off.
- **Found on the way**: the root card draw took each card's graph by slot index
  while the layout skips cards scrolled out of view (wrong graph once
  scrolled) — fixed by looking cards up by id.
- **Final review fixes**: a drag whose capture is dropped without a release
  (Esc, a view switch) is CANCELLED — start values restored, freeze cleared —
  by `healSynthDialGesture` (Synth-tab build + `DrumView.Update`); the freeze
  also requires the cached macros to belong to the instrument. Log spans floor
  only values ≤ 0 (a positive value below the span is out of span). The span /
  calibration guards resolve at the default tempo, because the voice length
  feeds the Tail / Fade-in caps and the `settles` gate — so **a reading is a
  pure function of the document AND the project tempo** (the project file holds
  both; an instrument alone, e.g. a bundle, can read differently per tempo).
