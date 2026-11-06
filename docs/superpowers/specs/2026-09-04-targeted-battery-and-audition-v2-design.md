# Targeted Battery, Calibration, Ablation and Audition v2 — Design

**Date:** 2026-09-04
**Program:** `2026-08-19-reference-grade-templates-program-design.md` (SP2 remainder + the first slice of SP4)
**Builds on:** `2026-08-22-calibrated-analysis-battery-design.md` (phases 1–3 shipped),
`2026-08-22-verdict-to-metric-mapping.md`, `2026-08-21-ear-verdict-round-1-findings.md`
**Status:** DESIGN — approved in conversation 2026-09-04; uncommitted by standing rule.

## Problem — what the live data says

Everything below was measured on 2026-09-04 against the current tree
(`refs validate`, `refs validate -terms`, `refs doctor -pitch`, `refs diagnose`
on tubular-bells, kalimba, snare, violin), not taken from the docs. The docs
have drifted from the data, which is itself finding 2.

1. **The battery is blind where the failures are.** 19 of 59 references have
   no reliable fundamental (`doctor -pitch`), so `noise.fraction`,
   `partial.inharmonicity` and `partial.series_deviation` report N/A on every
   noise-led percussion clip. Four of the eight ❌ families — clap, hihat,
   rimshot, snare — are noise-led percussion. Even on pitched inharmonic
   instruments the f0 anchor misleads: the kalimba reference's partial list
   starts at 3.44×f0 and the tubular-bells synth's at 4.01×f0 while its
   reference starts at 0.62×f0 — the two sides are being measured on
   different pitch grids, so every partial-ratio comparison on bars and bells
   is comparing apples to a different octave of apples.

2. **Calibration is underpowered and undocumented drift has set in.** The live
   good/bad split is 8/6; every 95% CI is ~0.3 wide. Only `inharmonicity`
   clears chance (AUC 1.00) and it fires on 4% of instruments with 16 N/A.
   `AGENTS.synth.instruments.md` records `tristimulus` 0.90 and `crest median`
   0.81 as trusted; the live values are 0.71 and 0.71, "unproven". Something
   was retuned or relabelled since and nothing records what. The recipe-hash
   provenance the SP2 spec promised (§4c) was never built.

3. **The stronger label type is unused.** The seven within-family preference
   picks are lumped into "mid" by `validate`. The SP2 spec names the preference
   test as the stronger gate; it does not exist.

4. **Thresholds were never calibrated, so axes saturate.** `MFCC profile`
   fires on 98% of instruments, `partial stretch` 96%, `spectral irregularity`
   and `vibrato jitter` 100%, `attack rise time` 86%. `refs probe` was built to
   measure audibility steps; no probe result has been fed back into any
   `Drift`/`Mismatch`.

5. **The word rules mostly miss.** 2 of 6 rules agree. "wooden" → inharmonicity
   MISSES the kalimba because inharmonicity already matches (0.010 vs 0.008)
   while the partial *sequence* differs. "warmer" MISSES guitar-nylon,
   "too dark" MISSES bassoon.

6. **References are too short to see ring.** Clips are 2–3 s; `ring T60` is
   ANTI (AUC 0.19); `decay T20/T30` are N/A on 16–26 instruments. Harp and
   tubular-bells are ring complaints.

7. **One note, one dynamic, one hit.** No axis sees how timbre changes across
   register or loudness, whether repeated hits differ, or what happens in the
   onset *after* the voice filter. The guide already records that every noise
   source in the modular pipeline injects pre-filter, so the bowed strings
   cannot make a bow bite — the "slightly fluty" verdict.

8. **The two discriminators the program names as missing are still missing.**
   Modulation spectrum (slow bow-stroke *shape* vs fast chaos at equal
   variance) and partial decorrelation (phase-locked sines vs a living
   harmonic). Sustained instruments are the failing group; these are the
   sustained-instrument axes. SP2 phases 4–8 are all pending.

9. **No closed loop from axis to knob.** Stage ablation is unbuilt. The
   dead-seed ratchet lists 118 seeded parameters that do not change the
   render, so the parameter space is partly inert.

10. **The audition asks one token and one comment.** `y/p/n` or a pick number,
    one free-text note, clips played once real-then-synth through
    `xdg-open` + `sleep`. No replay of a single clip, no per-property signal,
    no structured adjectives.

## Goal

A battery that can see the eight failures; calibration that uses every label
the listener has given and records what render it was heard against; a stage
ablation readout so retuning is evidence-led; and an audition that plays the
reference, the current version and two candidates side by side, then asks
targeted per-family questions whose every answer is a signed axis verdict.

Deliverable for the listener at the end: **three families — tubular-bells,
snare, oboe — each with two candidates**, auditioned through the new session
tool.

## Non-goals

- No composite realism score, no fitted weights (unchanged from SP2).
- No constant-Q transform, no orthogonality ratchet, no C-level voice taps
  (SP2 §2f, §4a, §3b) — deferred; not on the path to hearing better
  instruments this round.
- No engine (C) changes. Every candidate is a recipe-parameter overlay. If
  the diagnostic proves a family cannot reach its reference without a new
  stage (the bowed-string post-filter bite is the known case), that is
  recorded as a finding for the next round, not built here.
- No new instruments yet. Invented instruments are the round after this one;
  the reference-free organic range (§1.6) is built now because it is cheap and
  is what that round will stand on.
- Violin/viola are not retuned this round: their verdict needs the post-filter
  transient stage.

## Design

### 1. Battery additions (`internal/audio/fingerprint`)

All new measurements are registry rows following the phase 1–3 conventions:
non-empty `Doc`, an `Applies` predicate on *measured signal character*,
`OK=false` with a reason rather than a fabricated zero, `Kind`-default
comparator unless a custom `Compare` is genuinely needed. `fingerprint` keeps
importing only `internal/wave` + stdlib.

#### 1.1 Mode tracker — the f0-free artifact

New `Bundle` accessor `b.Modes()` (memoized) → `*ModeSet`:

- STFT via the existing `b.STFT(8192, 2048)` (5.4 Hz bins at 44.1 kHz).
- Per frame: local maxima above −50 dB relative to the frame maximum with a
  prominence of ≥ 6 dB over the neighbouring 3 bins each side; parabolic
  interpolation for sub-bin frequency.
- Link peaks across frames by nearest frequency within ±0.5% (or one bin,
  whichever is larger); a track survives if it spans ≥ 4 frames.
- Per mode: `Hz` (amplitude-weighted mean), `PeakDB`, `PeakFrame`,
  `DecayDBPerSec` (OLS slope of level in dB against time from the track's
  peak to its end, or to −40 dB rel peak), `Track []float64` (dB per frame,
  NaN where absent).
- `Anchor` = the lowest mode whose `PeakDB` is within 20 dB of the strongest.
  Ratios are relative to the anchor, never to `DetectF0`. Both sides of a
  comparison use the same rule, so a bell's hum partial anchors both.
- `Applies` helper `RequiresModes(b)`: audible and ≥ 2 modes.

The tracker is one artifact feeding four descriptor groups (§1.2, §1.3,
§1.5); nothing in it depends on a detected fundamental.

#### 1.2 Group `modal structure` (`GroupModes`)

| ID | Kind | What | Thresholds |
|---|---|---|---|
| `mode.anchor_hz` | Hz | anchor frequency (report-only; catches the "different octave grid" case) | 5% / 15% |
| `mode.ratios` | vector, custom `Compare` | top-12 mode frequencies ÷ anchor, ascending; distance = mean over reference modes of the nearest synth ratio's gap in cents, reported symmetric (max of the two directions) | 30 ¢ / 80 ¢ |
| `mode.count_per_octave` | Ratio | prominent modes per octave over 100 Hz–8 kHz (mode density: bell/cymbal dense, string sparse) | 1.4× / 2.0× |
| `mode.harmonicity` | Fraction | share of mode *energy* within ±3% of an integer multiple of the anchor — the wooden-vs-string axis the kalimba needed | 0.10 / 0.25 |
| `mode.beating_rate_hz` | Rate (Hz) | ‖f₁−f₂‖ of the closest pair of strong modes (both within 20 dB of max) closer than 25 Hz; N/A when none | 1 Hz / 4 Hz |
| `mode.beating_depth` | Fraction | modulation depth of the summed band around that pair, from its envelope | 0.10 / 0.25 |

`Applies: RequiresModes`.

#### 1.3 Group `resolved decay` (`GroupDecayResolved`)

| ID | Kind | What | Thresholds |
|---|---|---|---|
| `decay.per_mode` | vector, L1 mean, Rate units (dB/s) | `DecayDBPerSec` of the top-8 modes, labelled by ratio | 6 / 15 dB/s |
| `decay.slope_vs_freq` | Rate (dB/s per kHz) | OLS slope of per-mode decay rate against mode frequency — nylon/wood (highs die fast) vs steel/metal (highs ring) | 3 / 8 |
| `band.t60` | vector, custom `Compare` | T60 per octave band (125 … 8k Hz) **fitted from the slope of the visible tail** after the band's peak, so a clip that ends above −60 dB still yields a rate-derived T60 rather than a clamped lower bound; distance = mean ‖log₂(ref/synth)‖ | 0.5 (1.4×) / 1.0 (2×) |

`Applies: RequiresModes` for the first two, `RequiresAudible` for `band.t60`.
`env.ring_t60` stays registered but is **demoted** (excluded from the
"worst offending axes" ranking) with a `Doc` pointing here.

#### 1.4 Group `modulation` additions (existing `GroupMod`)

Modulation spectrum of the `Tracks().RMS` and `Tracks().Centroid` tracks:
detrend (remove the OLS line), Hann-window, FFT, keep 0.5–30 Hz.

| ID | Kind | What | Thresholds |
|---|---|---|---|
| `mod.am.rate_hz` | Rate (Hz) | dominant modulation rate of the loudness track | 1 / 3 Hz |
| `mod.am.depth` | Fraction | power at the dominant rate ÷ total 0.5–30 Hz power, scaled by the track's coefficient of variation | 0.05 / 0.15 |
| `mod.am.bands` | vector [gesture 0.5–2, vibrato 2–8, warble 8–30], L1 | where the movement lives — the bow-stroke vs chaos discriminator | 0.10 / 0.25 |
| `mod.am.flatness` | Fraction | spectral flatness of the modulation spectrum — a shaped gesture is peaky, chaos is flat | 0.10 / 0.25 |
| `mod.centroid.depth` | Fraction | same as `mod.am.depth` on the centroid track — brightness movement | 0.05 / 0.15 |

`Applies: RequiresAudible` and ≥ 32 frames (else N/A "clip too short for a
modulation spectrum").

#### 1.5 Group `partial decorrelation` (`GroupAliveness`)

| ID | Kind | What | Thresholds |
|---|---|---|---|
| `partial.bandwidth_cents` | Rate (cents) | mean −3 dB width of the top-8 modes on a 32768-point FFT over the sustain window, minus the Hann main-lobe width, in cents; the PADsynth argument made measurable | 5 / 15 ¢ |
| `partial.independence` | Fraction | 1 − mean pairwise Pearson correlation between the detrended dB tracks of the top-8 modes; phase-locked additive ≈ 0, a living harmonic > 0 | 0.10 / 0.25 |

`Applies: RequiresModes` and a sustain of ≥ 0.5 s for `bandwidth`
(`RequiresSustain`).

#### 1.6 Group `onset character` (`GroupOnset`) and noise-character additions

| ID | Kind | What | Applies | Thresholds |
|---|---|---|---|---|
| `onset.bite_ratio` | Ratio | mean(frame energy × flatness) over the first 30 ms ÷ the same over 300–600 ms — broadband bite that survived the voice filter, with no f0 needed | audible | 2× / 4× |
| `onset.flatness` | Fraction | spectral flatness of the first 20 ms | audible | 0.10 / 0.25 |
| `onset.pitch_glide_cents` | Rate (cents) | f0 at 20–40 ms vs f0 at 200–300 ms (plucked strings glide down, brass lips slur) | pitched | 10 / 30 ¢ |
| `noise.residual_centroid_hz` | Hz | centroid of the harmonic-notched spectrum — hiss (high) vs wire/rattle (mid) | pitched | 15% / 40% |
| `noise.decay_vs_tone` | Ratio | T20 of the residual energy track ÷ T20 of the harmonic energy track (both from `Tracks`: `RMS²·NoiseFraction` and `RMS²·(1−NoiseFraction)`) — "noisy throughout" vs a noise that belongs to the onset | pitched, percussive | 1.5× / 3× |

#### 1.7 Reference-free organic range

The organic set — every descriptor in §1.4, §1.5, §1.6 plus
`noise.persistence`, `vibrato.jitter`, `track.crest.median` — describes
*aliveness*, not identity. Their real-world spread is computed once over
every resolvable reference and committed as
`internal/audio/fingerprint/testdata/organic_range.json`:
`{ "<id>": { "p10": …, "p50": …, "p90": …, "n": … } }`.

`refs organic -instrument X [-pitch P] [-dur D]` renders the instrument and
prints, per organic descriptor, its value and where it sits: `below`,
`inside` or `above` the p10–p90 band, with a position
`(v − p50) / (p90 − p10)`. No reference is needed — this is the tool an
invented instrument will be judged with next round. `TestOrganicRangeGolden`
regenerates the table and fails on drift unless `-update` is passed, so a
corpus change is a visible diff.

### 2. Calibration (`cmd/refs`)

#### 2.1 Preference test

`refs validate` gains a `PREF` column: over every (chosen, rejected) pair, the
share of pairs where `Delta.Norm(chosen) < Delta.Norm(rejected)`, with the
pair count and a sign-test p-value. Pairs come from: the numbered picks in
`ear_verdicts.jsonl`; and every session record (§4.4) where a candidate was
rated `better` than current, or two candidates received different ratings.
An axis with PREF ≥ 0.75 on ≥ 6 pairs is labelled `PREDICTIVE (pref)` even
when its AUC CI spans chance — the spec's "Test 2 is the stronger gate".

#### 2.2 Thresholds from the good group

`refs calibrate-thresholds` computes, per descriptor, the ✅-group's
distribution of `‖Signed‖` in the descriptor's own units and writes
`internal/audio/fingerprint/testdata/thresholds.json`:
`{ "<id>": { "drift": p50(good), "mismatch": p90(good), "n": …, "source": "corpus" } }`.
The registry loads the file at init (embedded) and overrides `Drift`/`Mismatch`
where an entry exists; hand values remain for descriptors without one. An
entry may carry `"source": "probe"` with a listener-supplied audibility step
from `refs probe`, which wins over the corpus value. A descriptor whose
post-calibration fires-rate on the good group still exceeds 50% is listed as
`DEMOTED` by `validate` and excluded from the ranking. The file is a
ratchet: `TestThresholdsGoldenCurrent` fails if a regeneration differs.

#### 2.3 Verdict provenance

`earVerdict` gains `recipe_sha` (omitempty), and every session record
carries one per clip. It is `audio.InstrumentRecipeHash(instID)` — the
existing `hashRecipeParams` over the merged recipe, exported. `validate`
prints `STALE` beside any family whose current hash differs from the one its
verdict was heard against, and counts stale families in the header. The
44 existing rows have no hash and are reported as `unhashed`, never
`STALE`.

#### 2.4 Word rules rebound

`termRules` are rewritten against the new axes, each with the listener's
literal words:

| words | axis | synth reads |
|---|---|---|
| airy, breathy, wind instrument, like a flute | `noise.persistence` | high |
| warmer, fuller, needs warmness, too bright, pitched up or bright | `timbre.brightness_ratio` | high |
| too dark, dark and mellow, duller | `timbre.brightness_ratio` | low |
| wooden, marimba | `mode.harmonicity` | high (too harmonic) |
| metallic, bell, resonant, wavy, waves in time | `mode.beating_depth` | low |
| longer decay, let it ring, prolongs | `band.t60` | low |
| too sustained, ghost, lingers | `env.decay_t20` | high |
| hissy, fuzzy | `noise.flatness` | low (coloured, not broadband) |
| rattly, crack, snap | `onset.bite_ratio` | low |
| fluty (about a string) | `onset.bite_ratio` | low |
| nasal | `formant.f1_hz` | low |

`validate -terms` is unchanged in mechanism; the acceptance is that the
existing notes now agree on ≥ 8 of the 11 rules where a family carries the
word.

### 3. Stage ablation (`internal/audio/synthdiag`, new, `!test && !js`)

- `Stages` is a table `name → identity override set`, applied only when the
  stage is active in the merged recipe:
  `osc` (`osc_enabled 0`), `gen1…gen8` (`genN_source 0` — **not** `genN_gain 0`,
  because for transplanted sources gain 0 means "unconfigured"), `ensemble`
  (`unison_voices 1, unison_detune 0, unison_drift_rate 0, unison_drift_depth 0`),
  `fm`, `pitchenv`, `lfo`, `burst`, `filter`, `filtenv`, `formant`, `drive`,
  `post`, `kick` (their `_enabled 0`), `resonator` (`body_mix 0`),
  `bow` (`bow_dynamics 0`).
- `Ablate(instID, pitch, sr, dur) (Report, error)`: render full, then once per
  active stage via `synthmatch.RenderWithParams`; RMS-match every ablation to
  the full render; build a `Bundle` for each; for every descriptor applicable
  to the **full** render compute `Delta(full, ablated).Norm`. Output is a
  stage × descriptor sensitivity matrix.
- `refs diagnose -instrument X -ablate`: beneath the normal report, for every
  axis at `MISMATCH`, the top-3 stages by sensitivity. `-all -ablate` prints
  the matrix. The wording is "sensitivity", never a decomposition that sums.
- Reproduction gate: on `violin`, ablating `resonator` must be the top stage
  for `timbre.brightness_ratio` (the "too much bass was the fundamental via
  the body's low modes" finding that was previously reached by hand).

### 4. Audition v2 (`cmd/refs session`)

#### 4.1 Candidates file

`internal/audio/testdata/audition_candidates.json`:

```json
{
  "tubular-bells": {
    "reference": "tubular-bells-vcsl",
    "base": "tubular-bells",
    "candidates": [
      {"id": "A", "label": "per-mode decay + beating pair",
       "hypothesis": "decay.per_mode and mode.beating_depth were MISMATCH; lfo ablation showed …",
       "overrides": {"gen3_freq": 1.004, "gen1_env_fast_rate": 0.02}},
      {"id": "B", "label": "…", "hypothesis": "…", "overrides": {}}
    ]
  }
}
```

A test asserts every override key is a known `ParamDef` for the base's
recipe and that each candidate's render differs from the base (the liveness
check, so a candidate cannot silently be the current version).

#### 4.2 Playback

`player.go`: pick the first available of `paplay`, `aplay`,
`ffplay -nodisp -autoexit -loglevel quiet`, else the `OPEN` + sleep fallback;
`BEATMO_PLAYER` overrides. Every player call blocks until the clip ends. The
existing `-sh` plan path stays for `ab-all`/`ab-notes`; the session runs
entirely inside Go so single-clip replay is possible.

#### 4.3 Prompt flow per family

Clips: `R` reference (decoded), `C` current (base, no overrides), `A`, `B` —
all rendered at the reference's measured pitch and duration and RMS-matched,
as `compare` does today. Written to a temp dir as WAVs.

1. Play `R C A B` once. Prompt: `replay: r c a b all | Enter to rate | s skip | q quit`.
   Any replay key returns to the same prompt.
2. For `A`, then `B`:
   `How close is A to the reference? 1 nothing like it … 5 could be the real thing`
   `Against the current version: b better  e same  w worse  (r replay, s skip, q quit)`
   (see amendment 12: `s` is SKIP at every prompt, so "same" is `e` for EVEN)
3. Family questions from the table (§4.5), each answered for `A` and `B`
   on one line, e.g. `breathiness: A=4 B=2`. Scale is
   `1 far too little · 2 a bit too little · 3 about right · 4 a bit too much · 5 far too much`;
   `-` skips. Each question binds one descriptor and its "high" meaning, so
   an answer of 4 or 5 is "synth reads high on this axis".
4. Adjectives: a numbered global list —
   `airy dull harsh thin full metallic wooden plasticky buzzy hollow nasal muddy static alive synthetic real` —
   answered per candidate as numbers (`A=3 7  B=12`), `-` for none.
5. Optional note: `anything else? (Enter to skip)`.
6. `r`, `s`, `q` work at every prompt exactly as in `audition`.

#### 4.4 Session record

`internal/audio/testdata/ear_sessions.jsonl`, one line per family per
session:

```json
{"session":"2026-09-04T18:02:11Z","family":"oboe","reference":"oboe-A4",
 "semis_from_a3":12,"dur_sec":2.0,
 "current":{"id":"oboe","recipe_sha":"…"},
 "candidates":[
   {"id":"A","label":"…","recipe_sha":"…","rating":4,"vs_current":"better",
    "scales":[{"q":"breathiness","axis":"noise.persistence","answer":2}],
    "adjectives":["nasal","real"]},
   {"id":"B", … }],
 "note":"…"}
```

`answer` is stored centred: `-2 … +2`. `validate` consumes sessions for
preference pairs (§2.1) and a new `validate -sessions` table: per axis, how
often the listener's sign agrees with the measured `Delta.Direction` on the
same clip.

#### 4.5 Question table (`session_questions.go`)

| family | question | axis | 5 means |
|---|---|---|---|
| tubular-bells | ring length | `band.t60` | rings too long |
| tubular-bells | waviness / beating | `mode.beating_depth` | too wavy |
| tubular-bells | metallic complexity | `mode.count_per_octave` | too many overtones |
| tubular-bells | brightness | `timbre.brightness_ratio` | too bright |
| tubular-bells | strike noise | `onset.bite_ratio` | too much clank |
| snare | wire buzz vs hiss | `noise.flatness` | too much broadband wire |
| snare | tail length | `env.decay_t20` | rings too long |
| snare | body / low thump | `snare.low_body_frac` | too much body |
| snare | crack | `onset.bite_ratio` | too much crack |
| snare | tonal ring | `noise.fraction` | too tonal (scale inverted: 5 = too pure) |
| oboe | breathiness | `noise.persistence` | too breathy |
| oboe | warmth | `timbre.brightness_ratio` | too bright (inverted: 1 = too warm) |
| oboe | nasal / reedy | `formant.f1_hz` | too nasal |
| oboe | steadiness | `mod.am.flatness` | wobbles like a machine |
| oboe | attack speed | `env.attack` | too slow |

Adding a family is adding rows. A discipline test asserts every `axis` is a
registered descriptor ID.

#### 4.6 Make target

`make ab-session FAMILIES=tubular-bells,snare,oboe` builds `refs` and runs
`refs session -families …`. No shell plan.

### 5. The three retunes

Evidence-led, in this order, after §1–§3 exist: for each family, run
`diagnose -ablate` on the current version, then author two candidates whose
`hypothesis` names the mismatching axes and the stage the ablation matrix
says has leverage. Candidate A addresses the top mismatch axes through the
highest-leverage stage; candidate B takes a structurally different route.
Starting points, to be confirmed or replaced by the tool:

- **tubular-bells** — first resolve the pitch-grid discrepancy (`mode.anchor_hz`
  will say whether the synth's hum partial is present at all); then match
  `decay.per_mode` and `mode.beating_*`. B: a different partial set derived
  from `mode.ratios` of the reference with the hum at its measured level.
- **snare** — the tool already says: noise of the wrong kind (flatness
  0.599 → 0.204) and a plateau where the reference decays. A: broaden the
  noise and shorten `env.decay_t20`. B: rebalance body vs wire using the
  `snare.*` rows.
- **oboe** — `noise.persistence` high and `timbre.brightness_ratio` high.
  A: cut the persistent reed-noise bed and darken. B: engage the formant
  stage for the nasal peak the listener asked for.

Pre-audition gate per candidate: fewer `MISMATCH` rows than current among the
axes that are `PREDICTIVE` or bound by a family question, and no
`PREDICTIVE` axis made worse. This is a filter, not the verdict; the verdict
is the listener's.

### 6. Testing

- **Known-answer tests** per analyzer using `internal/wave/testgen.go` and
  the `synthgen_test.go` helpers: a 5 Hz tremolo peaks at 5 Hz in
  `mod.am.rate_hz`; a phase-locked additive tone scores `partial.independence`
  ≈ 0 while independently-enveloped partials score > 0.3; a tone at ratios
  1 : 2.76 : 5.40 (free bar) scores `mode.harmonicity` < 0.3 while a harmonic
  tone scores > 0.9; two sines 4 Hz apart give `mode.beating_rate_hz` ≈ 4;
  an impulse + decaying tone gives `band.t60` within 10% of the constructed
  decay; a clip whose tail ends at −20 dB still yields a finite `band.t60`.
- **Registry discipline** extended: every new descriptor has `Doc`,
  `Applies`, thresholds (hand or file), and an entry in `organic_range.json`
  when it is in the organic set.
- **Classifier replay gate** `diagnose_classifier_replay_test.go` stays
  byte-identical (existing descriptors are untouched except `ring T60`'s
  demotion flag, which does not change its verdict).
- **Ablation** reproduction gate (§3) as a `!test` CGo test.
- **Session prompts** tested with scripted stdin like `audition_prompt_test.go`;
  the record round-trips; `validate` reads sessions and existing verdicts
  together.
- **Candidates file** liveness + known-param test (§4.1).
- **Never bare `./...`**: name packages (`./internal/audio/fingerprint/`,
  `./cmd/refs/`, `./internal/audio/synthdiag/`).
- The listening gate is the user: three families, two candidates each,
  through `make ab-session`.

### 7. Order

| Phase | Contents | Gate |
|---|---|---|
| 1 | Mode tracker; `modal structure`; `resolved decay` | KATs; kalimba and tubular-bells now report `mode.*` rows instead of caveats; snare/hihat/clap/rimshot get `mode.*` and `band.t60` rows |
| 2 | Modulation, decorrelation, onset/noise-character rows | KATs; violin shows `partial.independence` ≈ 0 and `onset.bite_ratio` far below reference |
| 3 | Organic range golden + `refs organic`; preference test; thresholds file; provenance; word rules | `validate -terms` agrees on ≥ 8 of 11 rules; saturated set shrinks to ≤ 2 axes |
| 4 | `synthdiag` ablation + `diagnose -ablate` | violin reproduction gate |
| 5 | Candidates for tubular-bells, snare, oboe | pre-audition gate (§5) per candidate |
| 6 | Session tool, question table, record format, Make target | scripted-stdin tests; a dry run with `-plan` |
| 7 | Listening session (user) | verdicts recorded; winners promoted to the instrument table |

### 8. Risks

| Risk | Mitigation |
|---|---|
| Mode tracker picks noise peaks on percussion, inflating mode counts | prominence + 4-frame persistence; `mode.count_per_octave` is compared ref-vs-synth, so a rule that over-counts both sides still ranks |
| Thresholds from 8 good families are fragile | p50/p90 not min/max; probe entries override; the file is a visible ratchet, and `validate` still reports rank-based AUC that thresholds cannot move |
| Candidate overrides on the snare hit legacy knob names | the candidates test renders and asserts the override mutates the output |
| A candidate that measures better sounds worse | the pre-audition gate is a filter only; sessions record both, and the preference test will show which axes lied |
| Session prompts are long | 5 questions per family, `-` to skip any, `q` at any point |

### 9. Success criteria

1. Every noise-led percussion family and every bar/bell family has
   comparable `mode.*` and `band.t60` rows; zero "SUPPRESSED" among the
   eight ❌ families on those groups.
2. `validate` reports AUC, PREF and STALE; `validate -terms` agrees on ≥ 8 of
   11 rules; ≤ 2 axes remain SATURATED.
   **NOT MET as written — see amendment 11 for the measured state (3 of the 9
   rules that have a family agree).**
3. `diagnose -ablate` names a highest-leverage stage for every MISMATCH axis
   and reproduces the violin body finding.
4. `organic_range.json` exists and `refs organic` runs on any instrument
   without a reference.
5. Six candidates rendered, each passing the pre-audition gate, and one
   session record per family after the user listens.

## Amendments made during execution (2026-09-04)

Recorded as rulings in the SDD ledger; the text above is left as approved and
these entries override it where they conflict.

1. **§1.2 beating is per-mode ripple, not a resolved pair.** A bell's beating
   partials are closer than the 8192-point bin (5.4 Hz) by definition, so the
   "closest pair of strong modes" path is N/A on the reference as well as the
   synth. `mode.beating_rate_hz` / `mode.beating_depth` now measure each strong
   mode's own amplitude ripple (rate and depth of its detrended level track)
   with **common-mode rejection**: a mode's ripple counts only when its rate is
   not shared (±0.3 Hz) by every other strong mode; a lone strong mode
   qualifies unconditionally (an unresolved pair). A global LFO tremolo
   (vibraphone, mandolin seeds) therefore reads N/A; per-partial beating reads.
   `partial.independence` (§1.5) is the second opinion.
2. **§1.3 `band.t60` counts only bands that carry energy.** A band is unfit
   unless its peak energy is within 30 dB of the loudest band's peak in the
   same signal; the comparison stays the spec's mean ‖log₂‖ over bands fit on
   both sides, and the display names the worst band
   (`mean 2.0x (worst 1k 2.2x)`). Without the gate the harp's sub-fundamental
   125/500 Hz bands, which carry no energy, dominated the result.
3. **§1.4 limitation.** The OLS-line detrend cannot remove an exponential
   decay, so the gesture band (0.5–2 Hz) carries envelope leakage on every
   decaying note. The rate stays inside 0.5–30 Hz (lower bin bound is a
   ceiling) and the bands sum to one; but a gesture-band reading on a
   percussive clip is not evidence of a bow-stroke shape.
4. **§1.5 `partial.bandwidth_cents` applies only with ≥ 0.5 s of sustain**,
   as written; the whole-wave fallback is an honest N/A. `partial.independence`
   is clamped to [0, 1].
5. **§1.1** the tracker links across a one-frame gap and caps the link
   tolerance at 4 bins so noise cannot hop tracks. The bridged frame is **not
   interpolated**: `modeMaxMissFrames` keeps the track alive against its last
   known frequency without recording an amplitude for a frame nothing was
   measured in, so `Mode.Track` carries `NaN` there. The per-mode decay fit
   therefore **skips** NaN frames rather than stopping at them (it stops only
   at the −40 dB floor or the end of the track); breaking at the first NaN
   discarded every frame after the gap, which is exactly the history the gap
   tolerance exists to keep, and collapsed a beating mode's decay fit onto
   the few frames before its first amplitude null.
6. **§1.6** `noise.residual_centroid_hz` is f0-free: the notch sits at every
   strong tracked mode when the fundamental is unreliable and at k·f0 when it
   is reliable, so the row exists for clap/snare without resting on a
   zero-confidence f0. `noise.decay_vs_tone` reports N/A on an unreliable f0
   (the `noise.fraction` pattern) and when both decay tracks are clamped by
   the clip end. `onset.pitch_glide_cents` uses 20–60 ms vs 200–320 ms windows
   (YIN needs about four periods; 20 ms is two at 100 Hz).
7. **§2.2 calibration statistic and comparators.** `‖Signed‖` is the wrong
   quantity for the Seconds/Ratio (folded ratio) and Hz (percent) classifiers;
   the calibration uses `Delta.Mag`, the magnitude each classifier actually
   compares in its own units (a folded ratio for Seconds/Ratio, a percent for
   Hz, a plain difference otherwise); `Norm × mismatch` is not that once a
   comparator transforms its thresholds (`band.t60` compares mean|log₂|). `Descriptor.Compare` receives `(ref, synth, drift,
   mismatch)` so custom comparators calibrate too. Entries need n ≥ 5 and use
   the interpolating percentile (with 8 good families a "p90" is near the
   maximum; say so). The frozen 2026-08-22 diagnose baseline is kept — the
   frozen audio is the oracle — and a second replay golden records the
   verdicts of the same pairs under calibrated thresholds; no listener-named
   verdict may read `ok` under calibration.
8. **§3 ablation sensitivity and gate.** Sensitivity is
   `Norm(full, ablated) − Norm(full, full)`: the self-comparison is each
   descriptor's exact no-effect floor (a folded ratio of 1 gives
   `Norm = 1/mismatch`, not 0), so zero effect reads 0.00 on every Kind.
   The violin reproduction gate is "resonator among the top three stages for
   brightness ratio"; the measured order is filter first, resonator second —
   the hand finding concerned energy below 1 kHz, which the battery does not
   measure directly.
9. **§5 the Task 15 pre-audition gate.** A candidate must pass, on top of §5's
   own wording:
   - **`gateExemptions`.** Two axes are exempt, each for a recorded reason, and
     each documented with its numbers rather than silently dropped:
     `tubular-bells` / `timbre.brightness_ratio` (the ratio is taken against a
     `DetectF0` result the bell's inharmonic partial grid makes meaningless, so
     the row measures the pitch detector, not the brightness) and
     `oboe` / `env.attack` (the reference clip is a 1.14 s SWELL — the
     performance, not the instrument; a tongued oboe onset is ≤ 0.1 s).
     An exemption also removes the axis from the UNMEASURABLE trap, which is a
     known cost.
   - **A threshold-independent DIRECTION criterion.** On the family's
     listener-named axes, `|synth − ref|` must shrink by ≥ 25 % on at least one
     and grow by > 25 % on none. This is deliberately independent of the
     calibrated thresholds: a candidate that moves an axis from just past
     mismatch to just inside it passes a verdict test while changing almost
     nothing, and a threshold recalibration must not be able to turn a failing
     candidate into a passing one.
   - **`minLiveBoundAxes = 2`.** At least two of the family's bound axes must be
     LIVE (measurable on both sides). Below that the gate is judging one number
     and the dead axes are logged by name, because "the gate passed" and "the
     gate could not see anything" are not the same result.
   - **`-override` key validation.** An unknown parameter key is rejected rather
     than silently ignored — an ignored override renders the shipped instrument
     and reports it as a candidate.
10. **§2.4 the longer-decay word rule.** "prolongs" is removed from the
    `longer decay → band T60 (want low)` rule's word list. On tubular-bells it
    described the wavy metallic CHARACTER (already caught by the beating-depth
    rule through "wavy" / "waves in time"), not clip length, and the axis
    correctly reports that synth ringing LONGER than the reference, not
    shorter. Harp alone then carries the rule.
11. **§9.2 is NOT met; this is the measured state.** `refs validate -terms`
    now prints the arithmetic as an aggregate line under the TERM table
    (`rules with a family: N; agreeing (AGREE==FIRED>0): a; backwards: b;
    silent (FIRED==0): s`) so the claim has code behind it rather than being
    re-counted by eye — the earlier "8 of 9 agree" reading counted MISS rows
    (an axis that never fired) as agreement. Measured over
    `ear_verdicts.jsonl` with the calibrated thresholds:

    > rules with a family: 9; agreeing: 3; backwards: 1; silent: 5

    - **agreeing (3):** `airy → noise persistence` (2/2 fired, both in the
      implied direction), `metallic → beating depth` (2/2), `longer decay →
      band T60` (1/1, harp).
    - **backwards (1):** `wooden → mode harmonicity` fires on kalimba and
      rimshot in the OPPOSITE direction. The rule stays as the spec's
      hypothesis but is UNVERIFIABLE against these notes: both seeds changed
      after the verdicts were recorded (the kalimba `gen_gain` fix targeted
      exactly "sounds like a plucked string"), and a real kalimba reads 0.91
      harmonic under the strong-mode window — so "wooden = inharmonic" may
      simply be wrong for tines. Pending re-audition.
    - **silent (5):** `fluty`, `warmer`, `too dark`, `rattly`, `nasal` — the
      calibrated thresholds do not fire at all on the families whose notes use
      those words, so the axes have nothing to agree or disagree with. This is
      a statement about the thresholds and the corpus size, not evidence the
      axes are wrong; it is also why the criterion cannot be met by loosening
      thresholds, which would trade silence for noise.
12. **`s` is SKIP at every prompt.** `promptVsCurrent` used to read `s` as
    "same" while every other prompt in `refs session` reads it as skip through
    `controlKind`, so a listener who had typed `s` a dozen times to skip a clip
    would type it here meaning skip and silently record a data point claiming
    the candidate changed nothing. The vocabulary is now
    `b better  e same  w worse`, with `r`/`s`/`q` meaning replay/skip/quit
    everywhere. §4.3 step 2 is amended accordingly.
13. **`mode.ratios` is DEMOTED.** The row compares the symmetric mean
    nearest-neighbour gap between two mode ladders, which measures mode
    ABSENCE rather than mistuning: when the synth simply lacks a partial the
    reference has, the nearest surviving neighbour supplies the gap, so a
    missing mode scores LARGER than a badly mistuned one. The corpus proves
    it — calibration over the listener-approved group put the p50 at 520 cents,
    nearly half an octave, on instruments nobody called mistuned. It stays
    measured and PRINTED (the ratio list beside it is the clearest view of
    partial structure the report has) and is excluded from every ranking: the
    stage-sensitivity routing list, the `-all` MISMATCH matrix, `validate`'s
    trusted set and threshold calibration. Its `thresholds.json` entry is
    removed, so it falls back to its declared 30/80 cents. `refs diagnose` now
    prints `[demoted]` beside the verdict of any demoted row.
14. **`mod.am.rate_hz` is N/A on the lowest usable bin.** The OLS detrend
    removes a straight LINE and an exponential decay is not one, so a
    decay-only note's residual envelope curvature wins the bottom bin on every
    clip: before this change 57 of 59 corpus references reported a "rate", with
    p10 = p50 = 0.672 Hz — the floor bin itself — and the axis scored 0/11 on
    the preference pairs. The descriptor now returns
    `Unavailable("dominant modulation sits in the lowest usable bin — envelope
    leakage, not a rate")` when the peak bin is the lowest usable one. After the
    change 10 references have a measurable rate over a real range
    (p10 1.01 Hz, p50 1.34 Hz, p90 2.92 Hz). `Depth`, `Bands` and `Flatness`
    stay available: they describe the shape of the whole spectrum and remain
    meaningful when its peak sits at the floor.
15. **Vector rows print a headline, never component 0.** `Measurement.Primary()`
    returns `Values[0]`, which is right for a COMPOUND measurement (`env.ring_t60`
    puts its headline first and carries flags behind it) and wrong for an
    all-peer vector. `band.t60`'s component 0 is the 125 Hz band, very often the
    unfittable sentinel `-1`, so the report printed `-1.000 s` — not a duration
    at all — while the verdict came from the mean over the fittable bands;
    `mode.ratios`' component 0 is the anchor's gap to itself, a constant
    `0 cents`; `decay.per_mode`'s is the lowest mode's rate. `Descriptor.Headline`
    now renders `(vector: 6/10 bands measurable)` for `ScopeBand`,
    `(vector: N components)` for other vectors and `(track: N frames)` for
    tracks. Separately, `reducedDeltaDisplay` lets a comparator that formats its
    own value pass the finished string through verbatim, instead of
    `compareReduced` appending a second rendering of the same number
    (`mean 2.43x (worst 4k 6.52x) 1.280`).
16. **Synthesised thresholds say so.** Both degenerate branches of
    `calibrate-thresholds` now write a `Note`: `p90 <= p50 over n=<n>; mismatch
    synthesised as 1.5*drift`, and `p50 == 0 over n=<n>; drift synthesised as
    mismatch/1.5`. `Source` stays `corpus` (the measured half really is a
    percentile, and `hand`/`probe` mean pinned-against-recalibration, which
    these are not), but a reader comparing two corpus entries can now tell a p90
    earned from eight families from a multiple of the p50 —
    `mode.harmonicity`'s 0.349/0.524 pair read like two percentiles when only
    one of them was measured.
17. **§4.3 prompt flow, revised at the listener's request (2026-09-05).**
    Replay is available at every prompt and never restarts the family:
    `r` reference, `c` current, `a`/`b` a candidate, `all` everything, `rel`
    only the clips the current question is about (rating: R + that
    candidate; against-current: R + C + that candidate; a scale or adjective
    question: R + that clip). After any number of replays the same
    unanswered question is asked again; earlier answers are kept. The
    against-current keys are `+` better, `=` same, `-` worse (words
    accepted), because `b` is candidate B's replay key. Scale questions and
    the adjective list are asked one clip at a time, so the listener types a
    bare number (or numbers), never `A=4`; `-` skips that clip. `s` skips the
    family and `q` quits at every prompt.
18. **§4.3/§4.4 drafts and per-family adjectives (2026-09-06, at the listener's
    request).** Every accepted answer is written to a draft
    (`<sessions>.<family>.draft.json`); a quit or a crash keeps it, and the
    next run of the family resumes at the first unanswered prompt with every
    earlier answer intact, writes one record and removes the draft. `s`
    (skip family) discards the draft. Several replay keys may be typed on one
    line (`r a`). The adjective list is per family (`familyAdjectives`: a
    short vocabulary drawn from the listener's notes), with the global list
    as fallback; it is asked one clip at a time.
19. **§4.3/§4.4 the ship decision (2026-09-06, at the listener's request).**
    After the adjectives and before the note, the session asks which version
    should SHIP regardless of closeness to the reference: `0` keep the
    current instrument, `1`/`2` a candidate. Recorded as `ship` ("C" or the
    candidate id) on the session record and drafted like every other answer.
    It is a liking signal, not a closeness signal: the listener may prefer a
    synth that is nothing like the reference (the snare), so `ship` is kept
    out of the closer-to-reference preference test and is what Task 16's
    promotion step reads.
