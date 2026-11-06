# Organic instrument fidelity — DSP and tuning-infra proposal

2026-10-19 · status: **PROPOSAL**, except T1 and a first T3, built the same day
as `refs tune` (§ Built so far). The order and scope of the rest are for
approval.

## Intent

Make the shipped instruments sound more like the instruments they are named
after: more organic where the original is acoustic, more complex and refined
everywhere. The goal is **better versions of the same roster**, not new
instruments. The work is driven by measured gaps rather than by ear-guessing.
The measure is the essence distance; the listener's verdict is the gate.

The why: on 2026-10-18 the essence matcher (validated by real-vs-real
retrieval, `2026-10-17-wav-match-essence-design.md`) mapped every roster
instrument against 82 files (67 distinct recordings). **49 instruments are fidelity gaps**: real
recordings of them exist, and the synth ranks below 3 on every one. The
directions in which they miss are not random. They group by family, and they
agree with the listener's own notes in `ear_verdicts.jsonl` and
`wav_match_verdicts.jsonl`. That makes the
misses an engine and tuning-loop problem, not 49 separate tuning jobs.

## Goals, non-goals, anti-goals

**Goals**
- Close the family-level gaps below: brass too dark, winds and bowed strings
  too airy, drums and idiophones too clean, plucked strings too bright, short
  and out of tune at the top.
- Give an instrument document the means to say what real instruments do: change
  with register and dynamics, vary from note to note, carry pitch-synchronous
  noise, and add contact transients after the tone filter.
- Make the tuning loop optimise the distance that has been validated, across
  several notes, and hand the result to the ear before anything ships.

**Non-goals**
- New instruments, stereo/panning, the matcher's metric itself, templates.
- Purely electronic voices (leads, pads, 808s, FM). They benefit only where they
  share infrastructure (variation, LFO).

**Anti-goals** (each would count as failure even if it "works")
- Tuning to a number while the ear disagrees. `synth-match` did this: its
  fingerprint "improved" the winds and brass by 16–43 % (sax +42.7, oboe +31.2,
  flute +19.9, trumpet +17.4, horn +16.2), and the listener's notes still call
  the flute and oboe breathy/airy and the trumpet "close but needs refinement"
  (the sax, ear-approved in July, is a `p` today).
- Changing a shipped sound without a listening verdict pinned to that exact
  document (`doc_sha` / sound hash).
- Moving a single sample of an untouched document. Every new value is identity
  at its default (`TestSynthGoldenByteIdentity`, render-parity tool).
- Browser and desktop sounding different. All sound-path DSP stays in C, compiled
  for both platforms.
- Committing audio, or a test that reads reference audio.

## What was examined

**Roster (150 documents, by synthesis architecture)**

| family | instruments | how they are built today |
|---|---|---|
| bowed strings | violin, viola, cello, double-bass | `osc.bowed` waveguide + constant HP-noise layer + body bank + LP |
| bowed (pads/sections) | cello-warm, violin-ensemble, viola-pad | additive partials + noise |
| winds | flute, flute-breathy, piccolo, pan-flute, oboe, oboe-full, clarinet, bassoon, sax | additive partials (`gen.wavetable`) + one `gen.noise` layer |
| brass | trumpet, trumpet-mellow, muted-trumpet, trombone, tuba, french-horn(-loud), brass-stab | additive partials + noise |
| plucked | guitars ×6, harp, harpsichord, koto, mandolin, banjo, ukulele, sitar, clavinet, pizz-strings | Karplus-Strong (`ks`); 12 of the 16 `ks` documents also run one of the four FIXED body-bank mode tables (`body_model` 1–4; clavinet, guitar-electric, guitar-electric-neck and guitar-nylon-bright have none, and the same tables serve the bowed strings: violin/viola 1, cello/double-bass 3); no pick position, no dispersion |
| keys | piano-grand, piano-felt (additive 4–5 partials + noise); piano-rhodes, fm-epiano (FM) | no hammer/string/soundboard model |
| struck idiophones | xylophone, glockenspiel, vibraphone, marimba, celesta, tubular-bells, gamelan, steel-drum, singing-bowl, kalimba | additive 2–8 partials (kalimba = KS + partials) |
| drums | kicks ×22, snares ×9, toms ×6, congas ×5, clap ×4, cymbals ×8, cowbell ×3, shaker | dedicated C voices (`kick.physical`, `snare.acoustic`, `cym.*` modal plates, …) |

Declared but **unused** by every shipped document:
- the wind and brass waveguides `osc.brass`, `osc.reed`, `osc.flute` and
  `osc.sax`; the first three expose **no values**, so a document cannot tune them;
- the WAV-clone `modal` / `strike` / `noiseband` stages. Nine clone candidates
  (`$BEATMO_WAV_REFS/clones/`: conga, cowbell, glockenspiel, kalimba, marimba,
  tom, tubular-bells, vibraphone, xylophone) have no listening verdict recorded.

**The map, by family** (2026-10-18; direction on the first recording of each gap
instrument; colour is the losing term on 37 of 49):

| family | direction | the listener said |
|---|---|---|
| brass | **darker** 4/4: trombone 503 vs 1622 Hz (#66), tuba 279 vs 768 Hz (#63), horn-loud 660 vs 1000, trumpet-mellow 1138 vs 1712 | trumpet "close but needs refinement" |
| winds | **noisier** 5/6: flute noise 0.29 vs 0.02 (#39), pan-flute 0.35 vs 0.03, oboe 0.13 vs 0.00 | flute "very breathy", oboe "way too airy/breathy… need warmness" |
| bowed | colour 4/5: violin **darker** 1386 vs 1812 Hz, viola #45, cello brighter + noisier | "slightly fluty", cello "too airy and breathy", "needs more deep string bowing texture" |
| drums/snares | **cleaner** 6/6 and 4/5 (rimshot noise 0.15 vs 0.72), drums **darker** 5/6 | rimshot "fuller… rattly/vibrational and cracky (snap)" |
| idiophones | **cleaner** 5/8, **holds longer** 5/8 (xylophone −5 vs −11 dB; it is also darker, 689 vs 1795 Hz) | kalimba "string being plucked… provide the wooden marimba-esq feel", tubular bells "a simple clean tone… a resonant wavy metallic ring" |
| plucked | **brighter** 3/3 gap voices (guitar-nylon-bright 1731 vs 909 Hz, dies faster) | guitar-nylon "too pitched up or bright". The harp is NOT a gap (#2) and its notes ask the opposite way: "much longer decay" and "brighter, louder, more metallic and last for longer" |
| shaker | #51, **dies faster** −30 vs −7 dB | — |

**Measured for this proposal** (scratch probe via `go -overlay`; tree untouched;
`RenderAsShipped` at 48 kHz, a fine DFT scan around the expected fundamental,
cents relative to the voice's own pitch 0):

| voice | +12 | +19 | +24 | +28 | +31 | +36 |
|---|---|---|---|---|---|---|
| every `ks` voice (guitars, harp, harpsichord, koto, mandolin, banjo, ukulele, sitar, clavinet, pizz, kalimba) | −1 | −7 | **−18** | +7 | +13 | +9 |
| violin (`osc.bowed`) | −2 | −4 | −5 | −7 | −7 | −9 |
| piano-grand, flute, marimba (additive) | 0 | 0 | 0 | 0 | 0 | 0 |

The plucked strings are out of tune by up to 18 cents at the six intervals
measured. The pattern is identical across all of them (every `ks` voice is
melodic with pitch 0 at A3), because the Karplus-Strong delay is a whole number
of samples (`delayLen = (int)(sr/f + 0.5)`, `modular_stages.c`). A model of
that rounding (220 Hz, 48 kHz, plus the stretch filter's average 0.125-sample
delay) reproduces all six measured values to within 1 cent and predicts worse
notes BETWEEN them: −19 at +26, −25 at +30, **−29 at +34** — re-measure every
semitone before D1. The existing `TestEveryPitchedInstrumentSoundsInTune` could
not see it: it takes the median over node pitches {−12, −5, 0, 4, 7, 12} and
never probes above +12. The violin
drifts flat as it climbs. Its waveguide subtracts a constant 1 sample for the
loop's extra delay, which its own comment measured at 1.02–1.08 samples across
the range (`render_bowed_string`, modular.c ~209–216). A residual of a few
hundredths of a sample is worth more cents as the period shrinks, so that
residual is the likely cause.

**Engine facts verified in the source**

- Every noise source enters before the ADSR and the tone filter. That includes
  the gen-bank noise, the formant breath and the MODAL/NOISEBAND rows. Only bow
  dynamics, drive and FINISH run after the body bank.
- The voice cache key has no velocity. A node's volume is a gain, never a timbre.
- No document sets `noise_seed`, and round-robin variants are reserved. Every
  repeat of a note is bit-identical.
- The LFO is single-target, and `lfo_target:2` and `filter_env` are mutually
  exclusive.
- The shaker is four fixed noise bursts within 15 ms; the reference is a
  sustained shake.
- `synth-match` minimises the legacy `fingerprint.Distance` with Hooke-Jeeves
  over a hand-written param table (`synthmatch/tunable.go`).

## Diagnosis — six generic causes

1. **Snapshot synthesis.** Each additive voice is one note at one dynamic, with
   partials at fixed ratios. Played elsewhere, the whole spectrum transposes;
   real bodies, bores and formants stay put in Hz. Real brass also brightens as
   it gets louder (nonlinear wave steepening); a fixed table cannot. The
   evidence is that brass is darker on every recording, and that a bass voice
   played at G5 turns into a near-sine and wins sparse high notes.
2. **Noise is a separate, constant layer, in the wrong place, of the wrong
   kind.** In real winds and bowed strings the turbulence is modulated by the
   reed opening or the slip phase, so it pulses at the fundamental and fuses
   with the tone (Chafe 1990). A constant high-passed hiss is heard as a second
   sound: "airy", "breathy". Drums and idiophones have the opposite problem:
   their contact noise (stick, beater, tine, wires) is missing or filtered
   away, because noise can only enter before the filter.
3. **Plucked strings are static and out of tune.** Whole-sample delay
   (measured above), no pick-position comb, no dispersion (stiffness), and one
   loss setting for all frequencies. The body response is one of four fixed
   mode tables shared by every instrument that picks it, never fitted to the
   instrument's own recording.
4. **The physical models that would fix families 1–2 for winds and brass exist
   but are unusable.** They have no exposed values, are untuned, and no
   document uses them.
5. **No performance.** Identical repeats, no velocity timbre, and a vibrato
   that moves only one of pitch, amplitude or brightness at a time — a real
   player moves all three together.
6. **The tuning loop optimises the wrong thing on one note.** `synth-match`'s
   objective is the distance the matcher rewrite discredited. The clone loss
   (MR-STFT) is right for reconstructing one struck note, but nothing checks
   identity. References are one note at one dynamic, several are mislabelled,
   and nothing scores a candidate document on the matcher before an ear session.

## Proposals

Two tracks. **T** = tuning infrastructure (no sound change; it makes every D
item measurable). **D** = DSP engine (every new value is identity at its
default).

### T1 — Score a candidate document on the matcher

`refs match -candidate <doc.json | clone dir> [-wavs …]` ranks a candidate
beside the whole roster on every recording labelled as its family. It reports
the candidate's rank, the per-term distance, the traits, and the shipped
document's rank on the same recordings. It reuses `rankRoster`, `matchLabel`,
`RenderAsShipped`, hubness and `frames.json`, and is cached like the rest.

First use: the nine waiting clones. Each clone was fitted to the only recording
of its instrument, so this is an in-sample screen: it can reject a clone that
lost the instrument's identity, but it cannot prove one generalises. That is
the ear's job. Clones that beat their shipped voice go to `make ab-session
FAMILIES=… CLONE=1`; the verdict decides, and `make clone-adopt` ships. The
kalimba clone was fitted to an mbira recording (T5), so it is held back until
there is a kalimba reference, unless the listener likes it on its own.

Accept: a candidate identical to the shipped document reproduces the shipped
rank exactly.

### T2 — Fidelity ratchet

A committed `cmd/refs/testdata/match_fidelity.json` records each roster
instrument's best rank and its terms on its own recordings. It is re-derived by
`refs check -only match-fidelity`, like the hubness, and never by `go test`
(references are local). A retune that loses rank shows in the diff, the same
role `dead_module_values.txt` plays for dead module values. The ratchet only
records; the ear still decides.

### T3 — An essence-driven optimiser

The objective is the raw essence distance (no hubness), summed over every
recording of the family. Each recording is rendered at its measured note
(`matchRefPitch`), held for its length (`VoiceBPMFor`) and onset-trimmed, so
one note can no longer be over-fitted.

The search is the clone's CMA-ES (`internal/audio/clone/cmaes.go`: seeded, parallel), over
the instrument's ESSENTIAL params (`ParamDef.Tier`) or its macro dials, bounded
by each param's range. This replaces both Hooke-Jeeves and the hand table in
`tunable.go`.

The output is a candidate document under `$BEATMO_WAV_REFS/tuned/<id>/`
(`candidates/` already holds `refs candidates`' competing reference WAVs).
It never patches a shipped document; it goes through T1, then the ear. The
`synth-match` CLI keeps its name with the new loss, or is retired (builder's
call).

"Family" means the audition family (`buildAuditionFamilies`: an instrument and
its variants, e.g. trumpet + trumpet-mellow), not the broad kind (brass).

Accept, where the family has two or more recordings: leave one out, and the
candidate ranks higher than its start on the held-out one. Where it has one,
there is no held-out claim: the candidate must rank higher on that recording,
and the ear is the only test of generalisation. Most families have one
recording today, which is why T5's extra references matter. Before any family
is tuned, run it on two families whose verdict is already "y" (e.g. marimba,
double-bass) and show it does not move them away from their recordings.

### T4 — "Which control moves this trait"

For each gap, the map names the control that moves the losing trait, and by
roughly how much. It measures the trait sensitivity of each macro dial and
essential param (finite differences on `Essence.Traits`, reusing the
measurability machinery behind `TestEveryDialMovesItsMetric`). `refs match
-map` then prints, for example, "violin: 430 Hz darker → Brightness +18". This
is a hand-tuning aid that needs no optimiser.

### T5 — References that can teach register and dynamics

- **Fix the labels** found on 2026-10-18 (koto glissando, kalimba = mbira,
  glass armonica = wine glasses, pan flute = recorder, the singing-bowl guess,
  harp "g2" = G5). Add a `gesture` kind to the catalogue (single note /
  glissando / shake / roll) so a gesture is never a tuning target for a single
  hit.
- **Add 3–5 notes across the range at pp/mf/ff per family** where a free
  isolated source exists. Iowa MIS has chromatic pp/mf/ff for strings, winds,
  brass and piano. These go into the local corpus only, never committed. D4
  and the T3 multi-note objective need them.
- **Add `render.range`** (playable low/high) to melodic documents, sourced from
  the references and organology. The matcher then skips voices outside their
  range (removing the tuba-at-G5 class of miss), and the UI can later warn.

### D1 — In-tune strings (refinement, every plucked and bowed instrument)

Add fractional-delay tuning to the Karplus-Strong loop (a first-order Thiran
allpass for the fractional part, with the loss filter's phase delay subtracted;
Jaffe & Smith 1983, Laakso et al. 1996). For the bowed waveguide, solve the
loop's extra delay per pitch (or tabulate the measured 1.02–1.08 samples)
instead of subtracting a constant 1.

Guard it with `TestPluckedAndBowedVoicesInTune` (real build). It covers the 20
affected voices only, at every semitone from −24 to +36 (or `render.range`
once T5 adds it): within ±3 cents of the voice's own pitch 0, measured by a
fine DFT scan around the expected fundamental over 30–330 ms (the probe that
found the defect). Every other melodic voice keeps the existing 25-cent
`TestEveryPitchedInstrumentSoundsInTune`, because many detune, scoop or are
inharmonic on purpose. This is a **deliberate sound
change** (pitch only) for 20 instruments (16 Karplus-Strong, 4 bowed); their
goldens regenerate with that reason.

### D2 — A post-filter contact stage

A short-transient table rendered **after** the filter and body bank: bow bite,
tongue/lip attack chiff, hammer thump, stick and tine click, rimshot crack.
These are NOISEBAND rows with one new column, `post` (0 = before the filter as
today, 1 = after the body bank). It shares the existing table and render code,
so it stays inside today's table budget (see Constraints), and a document can
mix both kinds of row. This is the "post-filter
transient stage" `AGENTS.synth.instruments.md` already names as the missing
piece for bow bite.

Targets: bowed "fluty/darker", drums and idiophones "cleaner", rimshot
"cracky". Accept: the texture and evolution terms move toward the recordings,
and the listener's words stop appearing.

### D3 — Pitch-synchronous (pulsed) noise

A `gen.noise` layer gains `sync` (0..1) and `sync_width`. The noise is
amplitude-modulated by a periodic window locked to the voice's fundamental; at
`sync` 0 it is byte-identical to today. Inside the waveguides, the noise is
driven by the model's own flow (reed opening, bow slip), which is the physical
version of the same thing.

Targets: winds and bowed strings ("airy", "breathy", "fluty"). Accept: the
texture term and `noise persistence` move toward the references, the "airy"
notes stop, and a `probe-sounds` set (`pulsed-noise`) lets the listener confirm
the step at which it fuses.

### D4 — Register and dynamics as document data ("zones")

A `zones` table of anchors at pitches, and optionally at velocity layers. Each
anchor holds values for a declared subset of fields: partial gains, cutoff,
decay rates, noise level, strike hardness. Values are interpolated in
log-pitch per render. Like the existing tables it holds model parameters only,
is edited through `instrument.SetTable`, never a knob, and is fitted from the
T5 multi-note references. It needs a third table type with a fixed column set
(anchor pitch, anchor velocity, field id, value), so a document picks its
fields by row, not by column. Today's rule allows exactly two table types, and
their declared capacities (modal 128×5 + noiseband 32×7 = 864 values) must sum
to at most 1,024 — which is the most any ONE document can carry, since a
document holds at most one of each table. No shipped document uses either table
yet. A useful zones table therefore needs that rule changed (decision 1 below).

**Register first.** It changes nothing in the cache key, because voices are
already rendered at pitch.

**Velocity second.** A quantised dynamic layer (3 layers) joins the desktop
voice-cache key and the browser render-cache key, and the node volume selects
the layer plus a residual gain. This touches the lane, the prewarm and the
export identity harness, so it gets its own spec.

### D5 — Wind and brass waveguides to production

1. Expose values for `osc.reed`, `osc.brass` and `osc.flute`: breath-pressure
   envelope, reed/lip parameters, bore loss and bell filter.
2. Re-measure the flute waveguide's pitch first. AGENTS.synth.md records a
   ~1.47× mistuning, while the comment in `render_flute` says the STK 0.66666
   bore pre-scale fixes it, and the line below the comment does not apply it.
   Then fix it with the right jet/bore ratio plus D1's fractional delays.
3. Drive D3's pulsed noise from the flow.
4. Tune each with T3 against MIS notes. The additive document stays until a
   waveguide document wins on essence AND by ear.

Order: clarinet (the reed is already stable) → sax (`osc.sax` already has knobs)
→ trumpet/trombone (the lip model needs work) → flute. Brass gets "brightens
when loud" from the lip nonlinearity rather than from a filter envelope.

### D6 — Struck and plucked refinement

- **Struck:** adopt the clone candidates through T1 and the ear. Re-run the
  clone analysis on tubular bells for its ~18 missed high-frequency partials.
- **Karplus-Strong:** add a pick-position comb on the excitation, a dispersion
  allpass (stiffness: harpsichord bite, low piano), and a loss-filter cutoff
  that sets how fast the highs die relative to the fundamental (Välimäki et al.
  1996). Add a fitted body table (the body bank's band-pass form, as rows like
  `modal`) for guitar and harp. The harp gets the longer, brighter, more
  metallic ring the listener asked for.

### D7 — A piano model

Build it on the existing stages:
- MODAL rows with stretched partials (inharmonicity B per zone);
- paired, slightly detuned modes for the two-stage decay and beating of
  coupled unison strings (Weinreich 1977);
- STRIKE as hammer felt, with hardness from D4's velocity layer;
- D2 for the thump;
- D4 zones across the range.

Fit it from the MIS piano notes (Bank, Zambon & Fontana 2010 is the modal
piano reference design). The map shows piano-grand at G5 as a near-sine; the
real one has partials at 1.6 and 3.2 kHz.

### D8 — Variation and expression

- **Deterministic round robin.** The variant index is a pure function of the
  arrangement: hash(row, node, trigger count) mod N. It selects `noise_seed`
  and a small, declared jitter (pitch ±3 cents, decay ±5 %, strike ±). Export
  identity and the lane oracle stay exact, because the same arrangement always
  picks the same variants. Two things the build must change: `noise_seed` only
  seeds the noise oscillators (`osc_type` 5/6) — the Karplus-Strong slot seeds
  its own stream with a fixed 0 (`modular_stages.c`, `noise_ma_init(&noise, 0)`)
  and the drum voices draw from their own streams — so the variant seed has to
  reach every stream; and `voice_cache.go`'s `roundRobinEntry` already cycles
  variants with an atomic counter, which is per-process state, not a function
  of the arrangement, and must be replaced by the arrangement hash.
- **Multi-target vibrato.** Pitch, amplitude and brightness from one jittered
  LFO, already named in the AGENTS docs as "the clearest next engine upgrade".
- **Remove the `lfo_target:2` / `filter_env` exclusivity.**

### D9 — Stochastic particle shakers (PhISEM)

Cook's physically informed stochastic model (1997): random bead collisions,
filtered by the shell's resonances, with collision density following a shake
energy envelope. A node plays ONE stroke (a drum machine triggers a shaker per
eighth or sixteenth; a held shake would fight the sequencer). The −30 vs −7 dB
gap was measured against a sustained shake, a gesture T5 rules out as a target,
so D9 needs a single-stroke shaker reference before its acceptance means
anything. It replaces the four-burst shaker and gives the same family a
maracas/cabasa voice. The snare-wire rattle can reuse it later.

### D10 — PADsynth source for sections and choirs

Each harmonic becomes a band whose bandwidth grows with harmonic number and
carries random phases (Nasca). It is computed once per note and looped. It is
for violin-ensemble, viola-pad, voice-ahh/opera, organ and harmonium. The AGENTS
docs already call it the highest-fidelity path for ensembles.

## Built so far (2026-10-19)

`refs tune` (`cmd/refs/tune.go`) = T1 + a first T3, with `make tune`,
`make tune-picks`, `make ab-tuned` (the A/B listening round) and
`make tune-adopt`.
- **Objective**: the raw essence distance averaged over every recording of the
  instrument (catalogue + any `TUNE_WAVS` file that names it), each played
  exactly as `refs match` plays it (`renderMatchDoc` / `matchTarget.evalDoc`).
  There are no leave-one-out claims yet; most families have one recording.
- **Search**: the clone's CMA-ES over the document's OWN continuous values,
  each in a trust box around its shipped value, plus a 0.05-weighted pull
  back. A pruning pass then reverts the moves that buy < 1 % together. The
  origin is the shipped document bit for bit
  (`TestTuneStartsAtTheShippedDocument`). `add_harmonics` raises silent
  partials for the brass, whose five-partial tables cannot reach the
  recordings' centroid.
- **Picks** (`cmd/refs/testdata/tune_picks.json`): the listener's note plus the
  map's gap per instrument. A pick whose note contradicts its reference is
  restricted (harp).
- **Output**: candidates go to `$BEATMO_WAV_REFS/tuned/<inst>/` and are never
  shipped. `refs session -tuned` plays R / C / T at the current voice's note
  with a y/n/p gate on the candidate's `doc_sha`; `clone adopt -kind tuned`
  ships only on a `y`.

First run (40 generations, seed 1; rank among the whole roster, hubness-corrected):

| pick | recording | shipped | candidate |
|---|---|---|---|
| oboe | oboe-A4 / acclivity A440 | #14 / #9 | #1 / #1 |
| flute | flute-A4 | #39 | #1 |
| clarinet | clarinet-A4 | #14 | #1 |
| trombone (+6 harmonics) | trombone-A3 | #66 | #1 |
| tuba (+6 harmonics) | tuba-A2 | #63 | #1 |
| cello | cello-C2 / freesound | #2 / #3 | #1 / #1 |
| violin | violin-D5 / freesound | #10 / #15 | #1 / #1 |
| rimshot | rimshot-vcsl | #33 | #17 |
| snare | freesound / vcsl | #2 / #46 | **#4** / #12 (a trade: worse on one recording) |
| guitar-nylon | guitar-nylon-freesound | #2 | #1 |
| harp (render.beats 2 → 4, sustain/body only) | freesound / vcsl-e5 | #2 / #1 | #2 / #1 (the note's "ring longer" is note length, which the matcher ignores by design) |

All of these are in-sample: each is the recording the candidate was fitted to.
Nothing ships until `make ab-tuned` records a `y`. The drums improve least:
their gap is the post-filter contact noise that D2 adds, which no document
value can reach.

## Recommended order

| phase | items | sound change | why first |
|---|---|---|---|
| 0 | T1 (built), T2, T5 labels; clones through T1 → ear → adopt; the `refs tune` picks → `make ab-tuned` → adopt | only adopted candidates | Makes everything after measurable; nine clones and eleven tuned candidates are waiting. |
| 1 | D1 | pitch of 20 plucked + bowed voices | Smallest, clearest defect; measured today. |
| 2 | D3, D2 | winds, bowed, drums, idiophones | The two largest complaint classes ("airy", "clean"). |
| 3 | T3, T4, T5 multi-note refs | retunes, each ear-gated | Retuning pays off once the engine can express the fix. |
| 4 | D5, D4 register + `render.range` | winds, brass | The darkest family and the register misses. |
| 5 | D8, D4 velocity, D7, D9, D10 | broad | Largest infrastructure; own specs. |

Each phase ends with the same check:
- `make wav-match-map` and the T2 diff: did the family climb on its own
  recordings?
- `make wav-match-nuisance`.
- The real-build goldens: untouched documents byte-identical.
- A listening session whose verdicts are recorded.

## Constraints

- **Identity at defaults.** `TestSynthGoldenByteIdentity`,
  `TestRenderAsShippedMatchesPlayback`, the render-parity dump/compare. Any
  deliberate change regenerates the goldens with a stated reason.
- **Both platforms.** A C change means rebuilding `drums.single.js`; the
  cross-platform audio tests stay green.
- **Determinism.** Export→import master-mix identity, and the lane oracle
  (`TestLaneHeardEqualsLegacyDispatched`).
- **Knob discipline.** New values go through `modularFields` + `make
  gen-synth-abi`. Liveness: `TestDeclaredModuleValuesAreLive`; a new value no
  shipped document uses yet gets a reasoned line in
  `testdata/dead_module_values.txt`. A value that becomes a macro dial must pass
  `TestEveryDialIsMeasurable`.
- **The table rule** (`TestTablesAreModelParametersOnly`,
  `module_tables_test.go`): exactly two table types, their declared capacities
  summing to at most 1,024 values (864 declared; that sum is the most one
  document can carry, and no shipped document uses either table yet), every
  column physically bounded. D2 fits inside it (a `post` column makes noiseband
  32×8: 896 in all); D4, D6's body table and D7 do not.
- **Verdict pins.** A verdict is pinned to `InstrumentSoundHash`, which hashes
  the document's params only, so an ENGINE change (D1, D3 inside a waveguide)
  leaves old verdicts looking current. T2 adds the engine build (or a hash of
  the rendered audio) to what a pin records, so the listener is asked again
  when the sound changed underneath a document.
- **Render cost.** Waveguides and modal banks cost more than additive slots;
  the browser prewarms voices, and render is about half of the matcher's time.
  Ceiling (my call): no new or retuned voice may render slower than today's
  slowest shipped voice (hihat-open-bright, ~153 ms per render on the matcher's
  profile; the average is ~16 ms).
- **References.** Local only, never committed; calibrations through `refs check`.

## Left to the builder

- Exact filter designs (Thiran order, loss-filter fit).
- The pulse window shape for D3.
- The zone interpolation details.
- The PhISEM constants.
- Field names.
- Whether `synth-match` keeps its name.
- Which essential params T3 searches per family.
- How T1 names candidates in its table.

## Decisions for the listener

1. **The table rule.** Raise it so D4 zones, D6 body tables and D7 piano modes
   fit? The recommendation is to keep "model parameters only, every column
   physically bounded", allow more than two table types, and raise the
   per-document ceiling (e.g. 4,096 values per document, counted over the
   tables a document actually carries rather than over every declared
   capacity). Without this, D4, D6's body table and D7 are out, and the plan
   stops at D1–D3, D5, D8–D10.
2. **Engine changes and listening.** D1 changes the sound of 20 instruments
   (plucked up to 18 cents at the measured intervals and a modelled ~29 between
   them, bowed up to 9) without touching their documents.
   Should it ship on the tuning measurement plus regenerated goldens? Or only
   after one listening pass over those 20 voices? The same question comes back
   for every engine-level change, which is why T2 re-pins verdicts.
3. **Where to start.** Phase 0 then 1 is recommended.
4. **Velocity timbre (D4).** Should soft notes get darker and softer-attacked,
   not just quieter?
5. **Round robin (D8).** Drums only, or melodic voices too?
6. **More references (T5).** Is it OK to download multi-note, multi-dynamic
   Iowa MIS recordings into the local corpus?

## Calls made in this document (check these)

- D2 shares the NOISEBAND table with a `post` column rather than adding a table.
- D1's guard covers only the 20 affected voices at ±3 cents; everyone else
  keeps the 25-cent test.
- T3 uses audition families, and claims generalisation only where a family has
  two or more recordings.
- T1 is an in-sample screen for clones; the kalimba clone waits for a real
  kalimba reference.
- A shaker node plays one stroke.
- The render-cost ceiling is today's slowest shipped voice.
- Tuned candidates go to `$BEATMO_WAV_REFS/tuned/`, never into the shipped
  documents.

## Sources

- C. Chafe, "Pulsed noise in self-sustained oscillations of musical
  instruments", ICASSP 1990.
- D. Jaffe & J. O. Smith, "Extensions of the Karplus-Strong plucked-string
  algorithm", Computer Music Journal 7(2), 1983.
- T. Laakso, V. Välimäki, M. Karjalainen, U. Laine, "Splitting the unit delay",
  IEEE Signal Processing Magazine, 1996.
- V. Välimäki, J. Huopaniemi, M. Karjalainen, Z. Jánosy, "Physical modeling of
  plucked string instruments with application to real-time sound synthesis",
  JAES 44(5), 1996.
- G. Weinreich, "Coupled piano strings", JASA 62, 1977.
- B. Bank, S. Zambon, F. Fontana, "A modal-based real-time piano synthesizer",
  IEEE TASLP 18(4), 2010.
- P. Cook, "Physically informed sonic modeling (PhISM): synthesis of percussive
  sounds", Computer Music Journal 21(3), 1997.
- P. Nasca, PADsynth algorithm (ZynAddSubFX documentation).
- STK (Cook & Scavone): Bowed, Clarinet, Saxofony, Brass, Flute.
- University of Iowa Electronic Music Studios, Musical Instrument Samples.
