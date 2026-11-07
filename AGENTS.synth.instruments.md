## Synth Pipeline: Instrument Recipes

Reference for creating and modifying drum and FM instrument sounds. For the audio pipeline architecture, see `AGENTS.synth.pipeline.md`. For the broader registry / UI / plugin-readiness contract, see `AGENTS.synth.md` § Recipe Registry. For making **pitched/melodic** instruments (strings, brass, bass, hand drums) sound organic rather than synthetic, see § Organic Tuned/Melodic Instrument Synthesis below.

---

### Adding a new built-in instrument (the canonical path)

Every shipped instrument is **one instrument document**
(`src/go/internal/assets/instruments/<id>.json`, listed in `index.json`) holding every
value its voice depends on (see `AGENTS.synth.md` § Instrument documents). There is no
per-instrument C function, Go renderer, seed table or recipe.

> **Reading the older sections below:** they name the Go seeds the documents were
> generated from (`kickSeedWith`, `organicSnareWires`, `fmBassSeed`, … in the deleted
> `instrument_seeds*.go`) and rows of the deleted `modularInstrumentDefs` table. The
> values those sections discuss now live in the instruments' documents under the same
> field names; edit the document, not a seed.

1. **Document**: copy the nearest existing document (same family / `gen1_source`) to
   `internal/assets/instruments/<id>.json`, set `id`/`name`/`category`/`notes`, the
   `render` block (`seconds`, `gate_tail`, `melodic`) and the values that make
   it different, and add the id to `index.json`.
2. **Pack + ids**: `make asset-pack` (the embedded pack; `TestAssetPackIsCurrent`) and
   `go generate ./internal/audio/` (→ the `roster` constants).
3. **Re-pin** what a new/changed sound moves: `make measure-loudness` (writes
   `render.amplitude` into the document, rebuilds the pack), the one-shot golden
   (`synth_golden_pinned_test.go`) and `instrument_fundamentals.gen.go`
   (`make gen-fundamentals` — the note name).

**New DSP** (a sound no existing voice can make) goes in the modular engine
(`src/c/modular_stages.c`) as a new `gen_source` family or a new branch/path of an
existing one — e.g. the cymbal MODAL PLATE path below — with its knobs appended to the
flat param ABI (see "Adding knobs to a gen-slot voice" under the cymbal section). Then
`make build/libdrums.a` (cgo links a PREBUILT static lib — a `.c` edit does nothing
until it is rebuilt) and `make src/js/drums.single.js` (the Emscripten build the
browser renders with).

The instrument surfaces in the UI Synth tab automatically: `InstrumentParamDefs(id)`
derives its knobs from the document, filtered by `FlatFieldDeclared` and the visibility
gates in `synth_panel_zone.go`.

---

### General Synthesis Principles

- **Match the architecture to the instrument character**:
  - **Noise-led** designs work best for snare, clap, shaker, hi-hat — instruments whose identity comes from broadband texture.
  - **Tone-led** designs work best for kick, tom, cowbell, rimshot, sidestick, bass, sub-bass — instruments whose identity comes from pitched or inharmonic resonance.
  - The key question is: "Does this instrument sound like itself because of its *noise character* or its *tonal character*?" Let that answer guide the architecture.
- Use **separate envelopes** for tone vs noise paths (and even for different noise bands). This is key for avoiding "digital spit" and for getting punch + tail.
- For anything more than trivial shaping, use **RBJ biquad filters** (low-pass and band-pass) instead of single-pole filters so spectral focus is stable and controllable.
- Keep per-hit **variation**: tiny detune of body oscillators and small LFOs on noise bands help avoid static, machine-like tone.
- **Saturation as a timbral tool**: `tanh(x * drive)` followed by `softsat()` (which itself is `tanh(x * 1.5)`) generates harmonic richness from simple sinusoids. Higher drive values (3.0-4.0+) can transform clean sines into metallic/crunchy tones without adding noise. Lower drive (1.0-1.5) just rounds peaks gently.
- **High-pass filtering removes unwanted warmth**: When an instrument sounds too "tom-like" or "bassy", insert a 1st-order HP at 200-400 Hz to strip the low-end body. This is cheaper than biquads and effective for simple tonal shaping.

---

## Drum Instruments

> **Engine consolidation (2026, branch `add-core-node-types`).** The
> standalone `render_<name>` drum functions have been **migrated out of
> `src/c/drums.c`** into the modular gen-bank engine
> (`src/c/modular_stages.c`), dispatched by `gen_source` + a
> `gen_*_variant` discriminator: kicks `source==5`, toms `6`,
> snare/rimshot/sidestick `7`, clap `8`, the cymbal family
> (hihat/open-hihat/cowbell/shaker/ride/crash) `9`, Karplus-Strong
> bass-guitar `3`, analytic sub-bass `4`. `drums.c`/`drums.h` now keep
> only `load_audio`, `result_description`, the shared
> `softsat_shared`/`osc_wave_shared` primitives.
> **The DSP was a verbatim transplant**, so the acoustic recipes below
> remain accurate; each section names its CURRENT C function. What varies
> per instrument now lives in its document
> (`internal/assets/instruments/<id>.json`: `render.seconds` sizes the
> render — a fixed time on every platform and at every tempo), not in the C
> block. The per-drum `_p()` renderers
> are gone: the "POST knobs" lines below are the FINISH stage
> (`modular_finish`, `modular.c`), driven by the seed's `post_*` params.

### Snare (noise-led)

**C function**: `gen_snare_main` (`modular_stages.c`, source 7). Shipped as `snare` (`snareTunedSeed`), `snare-fat` (`snareFatSeed`, retuned for crack 2026-09-30) and `snare-lofi` / `snare-ghost` (`snareFatOriginalSeed`, the pre-crack fat sound); the numbers below are the C literals, which each seed scales

- **Body (tone)**:
  - Two decaying sines around low snare fundamentals, with a tiny per-hit detune (seeded: `snare` 199 / 298 Hz, `snare-fat` 186 / 280 Hz).
  - Apply a **very gentle downward pitch drift** across the hit for realism, but keep the body relatively quiet in the final mix so the snare doesn't turn into a synth-bass.
  - Decay should be **shorter than the noise tail**: enough ring to feel like a drum, but most of the length should come from noise.

- **Noise (head + wires)**:
  - One white-noise source, split via biquads into:
    - A **low band** (~350 Hz nominal) for the head "paper" / thud.
    - One or more **mid bands** (~1.8-3.2 kHz nominal) for snare wires.
    - An optional **higher band** (~4.5 kHz nominal) for crispness; keep this subtle to avoid clappy, brittle tails.
    - Every band centre is multiplied by `snare_noise_tune` (the `snare` seed uses 0.7 → ~245 Hz / 1.26-2.24 kHz / 3.15 kHz; `snare-fat` 0.41 since its crack retune — see "A snare needs a CRACK" below; `snareFatOriginalSeed` 0.55).
  - Shape the bands with separate envelopes:
    - Low band: slower decay for weight (short thud, not a drone).
    - Mid bands: very fast attack (crack) plus a shorter tail; this tail sets perceived "ring" more than the pitched body.
  - LFO modulation (~13 Hz) on the wire bands adds organic complexity.

- **Attack & mix**:
  - Moderate front-end gain boost on the mid/high bands for the first few ms; keep it moderate so it doesn't sound like a clap.
  - The C literals aim for a **noise-led mix** (low band ~1.1x, wires ~0.9x, body ~0.45x), but they are further scaled by the seed's `snare_tone_m` / `snare_noise_m` / `snare_wire_m` — with the shipped `snare` seed body and low band come out roughly equal (≈0.5 each, wires ≈0.6 fast / 0.3 tail).
  - Gentle global fade (based on normalized index) to keep the overall length sensible while still allowing ring.

- **POST knobs** (the FINISH stage, `post_decay_rate` 8): `pitch` (re-index via playback rate), `decay` (a post-envelope `exp(-t·decay_rate·(1/decay−1))`), `drive` (saturation), `darken` (a one-pole LP crossfaded in).

### Rimshot (a SNARE voice) and snare-crack (`gen_snare_rimshot`)

**The shipped `rimshot` is `snare.main` (source 7, variant 0), started from `snare-fat`'s document** (`refs tune`, pick
`from: snare-fat`; ear-approved in `make ab-tuned`, 2026-11-07: "ball park"). A real rimshot is a stick striking the
head AND the rim at once, so it is a snare hit with a rim crack: head, wires and rim together — the reference
(`rimshot-vcsl`, a VCSL legacy-snare rimshot) measures noise 0.72, centroid 1459 Hz. The tone-led voice below was the
shipped rimshot until then; the listener heard it as "a high tom or conga", and it ranked #33 on that recording with the
cowbell as its nearest. The next round's ask was "more punch and crack, less air": in `snare.main` terms, punch = the
body tone (`snare_tone_m`, ring length `snare_tone_d` — a decay RATE, so lower rings longer), crack = the wire bands
raised (`snare_tune`, every band centre × it) and their onset boost (`snare_attack` multiplies the WIRES, so on its own
it adds air), air = the low-noise and wire-tail share (`snare_noise_m`, `snare_tail_d`). Shortening the tail is not
"less air" against this recording: it holds −14 dB at 0.5 s, and a faster tail cost the decay term at once.

**`gen_snare_rimshot` (source 7, variant 1) is now `snare-crack`'s voice only** — an 808-style inharmonic resonator:
four fixed partials at 1 : 2.1 : 3.4 : 5.6 (500 / 1050 / 1700 / 2800 Hz × detune, gains 1.0 / 0.95 / 0.8 / 0.5, decays
exp(−40t) … exp(−90t), no pitch drift), a ~5 ms HP noise crack (decay exp(−200t)), an attack spike
`1 + 2·exp(−1500t)`, then HP 400 Hz → `tanh(4x)` → `softsat()`; essentially silent after ~100 ms. It is a drum-machine
clave/rim click, not an acoustic rimshot: snare-crack ranks #32 on the snare recording.

### Sidestick (tone-led)

**C function**: `gen_snare_sidestick` (source 7, variant 2)

- **Architecture**: Dry woody click — **tone-led, purely transient**, no pitch movement.
  - The critical difference from a bad sidestick is **no pitch sweep**. Exponential frequency sweeps create a "laser gun" / "pew pew" sound.

- **Tone (2 inharmonic sines)**:
  - Fixed frequencies at ratio 1 : 2.4 (500 / 1200 Hz x detune).
  - **No pitch sweep** — both frequencies are constant.
  - Fast exponential decays: wood body exp(-100t), rim ping exp(-120t).
  - Amplitudes: 0.5 and 0.45 (nearly equal — both are quiet, transient only).

- **Noise**: Single RBJ bandpass at 1500 Hz, **Q=0.7** (low Q is critical — high Q like 1.5 sounds ringy/synthetic). Amplitude 0.5, decay exp(-150t).

- **Attack boost**: `1.0 + 1.0 * exp(-800t)` — doubles the amplitude in the first ~1ms.

- **Buffer duration**: 0.25s.

- **Design notes**:
  - To avoid "toy laser gun": never use pitch sweeps. Fixed frequencies only.
  - To avoid "ringy synth": keep bandpass Q below 1.0 (0.7 is good).
  - To make it brighter: raise the rim frequency (e.g., 1500 Hz) and the bandpass center.
  - To make it woodier: lower both sine frequencies and increase noise amplitude.

### Kick — Standard (tone-led)

**C function**: `gen_kick_base` (source 5, variant 0; shipped as `kick`)

- **Body (tone)**:
  - Built from a **harmonic sine stack** anchored around ~55 Hz:
    - Fundamental (`f0`) carries most of the weight.
    - 2nd harmonic (`2*f0`) adds low-mid punch.
    - 3rd harmonic (`3*f0`) adds a small amount of "knock" at the front.
    - 4th harmonic (`4*f0`) provides audibility on small speakers.
  - All partials share a **small pitch envelope** (~+10% at onset; `kick_pe_rate` defaults to 30/s, a ~33 ms time constant) so the kick has punch without a "laser" sweep.
  - Amplitude envelopes are exponential and **shorter for higher harmonics** so the tail becomes round and deep.

- **Dark noise thud (rawness)**:
  - A single white-noise source is **heavily low-passed** (one-pole, 0.97/0.03) so only low/mid "thud" remains.
  - Short exponential envelope (exp(-16t)), mixed quietly (0.18).

- **Beater click**:
  - HP-filtered noise burst (~2-3ms) for sharp transient audible on any speaker.
  - Uses separate noise + 1-pole HP (alpha 0.85), decay exp(-120t), amplitude 0.35.

- **Attack shaping**:
  - Front-loaded gain boost (`1.0 + 0.3 * exp(-40t)`) on the tonal stack.

- **POST knobs** (FINISH, `post_decay_rate` 6): `pitch` (re-index), `decay` (post-envelope `exp(-t·decay_rate·(1/decay−1))`), `drive` (saturation), `body` (low-frequency resonance boost via LP mix at 120 Hz).

- **Design notes**:
  - Keep the **fundamental dominant**, with harmonics and noise clearly subordinate.
  - Use **tiny pitch envelopes** on the tone (<= 10-15%) for punch; larger sweeps read as electronic/industrial.
  - Noise should be: strongly low-passed, very short, mixed quietly.
  - For more aggression: slightly stronger 2nd harmonic and a bit more softsat drive, or faster pitch envelope.

### Kick-Deep (tone-led)

**C function**: `gen_kick_deep` (shipped as `kick-deep`)

- **Architecture**: 808-style deep sub kick — lower, boomier, longer sustain than the standard kick.
- **Body**: Fundamental (~42 Hz), 2nd harmonic, and a quiet 3rd (`kick_h3` 0.10, `exp(-12t)`), starting at Pi/2 phase for maximum punch on first sample. Plus a quiet HP click (0.20, `exp(-180t)`).
- **Pitch envelope**: Wider (+15%) and slower (exp(-15t)) than standard kick for deeper, rounder punch.
- **Noise**: Very quiet low-passed thud (0.10 amplitude), shorter decay.
- **Saturation**: Gentle — `tanh(mixed * 1.2) * 0.95` — cleaner than standard kick for a pure sub feel.
- **Buffer duration**: 0.8s (long sustain is the point).

### Kick-Punchy (tone-led)

**C function**: `gen_kick_punchy` (variant 2; shipped as **`kick-snap`** — "Snap Kick". Not to be confused with the `kick-punchy` id, which is the variant-5 "Dry Kick" of the configurable kick stage)

- **Architecture**: 909-style electronic kick — short, snappy, cuts through a mix.
- **Body**: Higher fundamental (62 Hz) with only 2 harmonics (f0 + 2*f0) for a clean electronic character.
- **Pitch envelope**: Aggressive +25% sweep with fast decay (exp(-55t)) — gives signature "thwack".
- **Beater click**: Louder (0.45 vs standard's 0.35), brighter HP filter (alpha 0.80) for snappy transient. No noise thud.
- **Attack**: Stronger boost — `1.0 + 0.5 * exp(-60t)`.
- **Saturation**: More aggressive — `softsat(mixed * 0.8) * 1.3` for density.
- **Global fade**: exp(-5.0 * tNorm) (faster cutoff than standard kick).
- **Duration**: `render.seconds` 0.25 (the `kick-snap` row).

### Kick-Lofi (tone-led)

**C function**: `gen_kick_lofi` (shipped as `kick-lofi`)

- **Architecture**: Lo-fi/warm/vintage kick — thick, gritty, sub-focused.
- **Body**: Lower fundamental (50 Hz) with 3 harmonics (50/100/150 Hz) and longer decay for fullness.
- **Pitch envelope**: Gentle +8% sweep with slow decay (exp(-20t)) — round, not snappy.
- **No beater click**: Warm, undefined attack. Thick noise thud instead (amplitude 0.25, less filtered: alpha 0.96/0.04).
- **Built-in bit reduction**: Quantizes output to ~128 levels (7-bit) per sample for lo-fi grit.
- **Double saturation**: `tanh(tanh(x * 1.5) * 1.8)` for harmonic warmth.
- **Built-in one-pole LP**: ~600 Hz on final output to darken the sound.
- **Global fade**: exp(-3.0 * tNorm) (slower, more sustain).
- **Buffer duration**: 0.5s.

### Kick-Tight (tone-led)

**C function**: `gen_kick_tight` (shipped as `kick-tight`)

- **Architecture**: Acoustic/studio kick — natural, controlled, defined.
- **Body**: Medium fundamental (58 Hz) with 3 harmonics (58/116/174 Hz) and fast decay.
- **Pitch envelope**: Minimal +5%, ultra-fast exp(-65t) — barely audible, just adds snap.
- **Beater transient**: RBJ biquad bandpass at 2.5 kHz (Q=0.7) on noise — gives natural "beater on drumhead" sound.
- **Room thump**: Brief bandpass noise at 200 Hz (Q=0.5) with very fast decay (exp(-80t)) for short "room" character.
- **Built-in gate**: Additional envelope multiplier that goes to 0 after ~45% of buffer for tight cutoff.
- **Saturation**: Minimal — `softsat(mixed * 0.45) * 1.1` — clean, natural.
- **Global fade**: exp(-4.5 * tNorm).
- **Duration**: `render.seconds` 0.15.
- **Design notes**: The bandpass beater transient (instead of HP noise) is what gives this variant its natural "acoustic" character. The gate prevents the tail from ringing, making it sit cleanly in a pop/rock mix.

### Hi-Hat Closed / Open (MODAL PLATE + collision rattle)

**Shipped**: `hihat` (closed, variant 0), `hihat-open-bright` (open, variant 1),
`hihat-lofi` and `hihat-pedal` (closed seed + their own DefaultFX). All four run the
cymbal MODAL PLATE voice (`gen_cymbal_modal`, `modular_stages.c`) — see § Cymbals as
modal plates below for the model, the knobs and how they were fitted. Rebuilt
2026-09-28 against a user-supplied acoustic closed hi-hat.

- **Closed** = a heavily damped plate (~396 modes, 1.1–16 kHz, strike hump at 10.6 kHz,
  0.8 oct wide) re-struck by a decaying train of **collision impulses** (the top cymbal
  rattling on the bottom one, ~1950 collisions/s). That rattle is what produces the
  swell, the chattering plateau, and the noise-like spectrum (flatness ≈0.29) of a real
  closed hat — without a noise generator. The fit's best score used a 58 ms rattle / 28
  Np/s ring; the user picked the TIGHTER take by ear (rattle 42 ms, ring 36 Np/s — the
  looser ones "sounded too low pitched"), which is what ships.
- **Open** = the looser fitted plate (58 ms / 28 Np/s) on variant 1 — the voice loosens
  the pedal: rattle decay ×5, ring decay (tail and HF corner term) ×0.3 — PLUS, in gen
  slot 2, a quiet layer (level 0.25) of the LEGACY open-hat square cluster with its
  white-noise hiss off: the "touch of metal" the user picked ("O2") to keep a little of
  the old electronic texture. Layering voices across gen slots is config-only — any seed
  can add `gen2_*` / `gen3_*` voices.

**Partial voice** (`gen_cymbal_open`, the open hi-hat when `gen1_cym_modes` is 0): a
808-style cluster of 6 inharmonic square oscillators. The closed hi-hat's partial voice
(`gen_cymbal_hihat`) was deleted 2026-10-02 — variant 0 always runs the modal plate.

### Tom — Standard (tone-led)

**C function**: `modular_gen_slot_tom` (source 6, `gen_tom_variant` 0; shipped as `tom`)

- **808-style** with resonant attack, warm body decay, and subtle room ambience.
- **Body (tone)**:
  - Fundamental (~150 Hz with detune) plus two overtones at 1.5x and 2.1x ratios.
  - Fast 808-style pitch sweep (exp(-18t)) from 1.25x base to 0.85x base for punch.
  - Longer decay than snare body: fundamental ~exp(-2.8t), overtones slightly faster.
- **Stick impact**: Mid-band focused via 2-pole LP + HP (bandpass-like), longer envelope (exp(-120t)) for presence.
- **Ambient noise**: Very quiet, long-decay (exp(-6t)) low-passed noise for room feel.
- **Resonant attack boost**: `1.0 + 0.4 * exp(-80t)` for bright transient that fades to warm body.
- **POST knobs** (FINISH, decay_rate 8): `pitch` (re-index), `decay` (post-envelope exp(-8t*(1/decay-1))), `drive` (saturation).

### Tom High (tone-led)

**C function**: `modular_gen_slot_tom`, `gen_tom_variant` 1

- Base pitch 170 Hz (brighter), wider sweep (Start*1.3, End*0.88), faster pitch decay (exp(-22t)).
- Faster envelopes: exp(-3.5t), exp(-5.0t), exp(-6.5t). Stronger resonant boost: exp(-90t)*0.5.

### Tom Low (tone-led)

**C function**: `modular_gen_slot_tom`, `gen_tom_variant` 2

- Base pitch 90 Hz (deeper), gentler sweep (Start*1.2, End*0.82), slower pitch decay (exp(-14t)).
- Longer envelopes: exp(-2.2t), exp(-3.2t), exp(-4.5t). Longer ambient tail: exp(-4.0t).

### Clap (noise-led)

**C function**: `modular_gen_slot_clap` (source 8)

- Several tightly spaced **noise bursts** (white noise) at staggered times (~0, 15, 30 ms, plus a slightly later echo at ~60ms, amp ~0.7) to approximate multiple hands clapping and a tiny bit of room.
- The bursts use a fast attack-shaped envelope; the late echo decays at exp(-80t) and the underlying body noise at exp(-7t) — these are **separate** envelopes, not one shared tail.
- Everything is noise-led; there is no pitched component.
- **POST knobs** (FINISH, decay_rate 10): `decay` (post-envelope exp(-10t*(1/decay-1))), `drive` (saturation).

### Cowbell (tone-led)

**C function**: `gen_cymbal_cowbell` (source 9, variant 2)

- **Body**: 4 inharmonic sine partials (~640, 920, 1300, 1900 Hz), per-hit detuned. Gains taper: 1.0, 0.85, 0.6, 0.4. Slow decay (exp(-9t)) gives clear metallic ring.
- **Impact**: Short RBJ band-passed noise burst (1200 Hz, Q=2, exp(-260t)) for stick contact.
- **Metal rasp**: Subtle high-passed noise tail (exp(-60t)) at low amplitude (0.30).
- **POST knobs** (FINISH, decay_rate 12): `pitch` (re-index), `decay` (post-envelope exp(-12t*(1/decay-1))).

### Shaker (noise-led)

**C function**: `gen_cymbal_shaker` (source 9, variant 3)

- **Architecture**: Noise-based with grain-like micro-bursts for rhythmic texture.
- **4 staggered micro-bursts** at ~0, 4, 9, 15ms with tapering amplitudes (1.0, 0.85, 0.7, 0.5) and per-hit timing variation.
- **Spectral shaping**: 1-pole HP (alpha 0.85, no fixed-Hz cutoff) + RBJ bandpass at 8 kHz (Q=0.7) for shimmer.
- **Overall envelope**: exp(-25t) keeps it short and crisp.

### Ride (MODAL PLATE)

**Shipped**: `ride` (variant 4, `gen1_cym_modes` 316) — a modal plate fitted to a
user-supplied jazz ride and chosen by ear (2026-09-28). Duration 1.8 s (`render.seconds` 1.8),
because the plate rings like its 1.7 s reference. See § Cymbals as modal plates.

**Partial voice deleted (2026-10-02)**: the ride's old fixed-sine-partial voice
(`gen_cymbal_ride`) is gone — variant 4 always runs the modal plate (Plate Modes clamps
to ≥ 1).

### Crash (tone-led)

**C function**: `gen_cymbal_crash` (variant 5, legacy fixed-partial voice — not yet a modal plate; the plate path is available to it via `gen1_cym_modes` > 0)

- **Architecture**: Darker, washy cousin of the LEGACY ride voice — 8 sine partials (2-11.5 kHz) + bell + noise wash.
- **Key differences from the legacy ride**:
  - Darker frequency range (starts at 2 kHz vs the legacy ride's 3.1 kHz).
  - Tapered gains emphasize mid partials (3-6 kHz), reducing lowest and highest.
  - Much longer decay: cluster tail exp(-3t), noise exp(-8t).
  - Darker bell (~2.5 kHz vs the legacy ride's 3 kHz).
- **Buffer duration**: 1.5s.

### Bass Guitar (tone-led)

**Shipped**: `bass-guitar` (`bassGuitarSeed`) is **additive**, not Karplus-Strong: a
sine osc one octave down (`osc_octave -1`), sine gens at 2×–6× with per-gen decay rates
(0.8–2.0/s), a band-passed noise-tap pluck (1.1 kHz), `amp_decay` 4 / `amp_sustain` 0 so
it plucks and rings down. `render.seconds` 0.75.

The Karplus-Strong voice (`modular_gen_slot_ks`, `gen_source 3`) still exists and is
used by the harp / guitars / kalimba: an LP-filtered noise-seeded delay line whose length
is `sr / (gen_freq × voice_freq)`, a `0.5·(y[n]+y[n-1])·decay` loop filter with
probabilistic stretch, decay 0.996 by default, plus an ~8 ms noise attack.

### Sub-Bass (tone-led)

**C function**: `modular_gen_slot_analytic` (`gen_source 4`). `sub-bass` / `sub-bass-drive` are config rows (`subBassSeed`, 2026-10-02) and the voice has a Synth-tab family stage (`bass`: Harmonics, Pitch Drop, Drop Speed, Punch, Punch Speed, Body Decay, Drive, Level + the base Pitch, `voice_freq_hz`).

- **Architecture**: 808-style deep sine with pitch envelope.
- **Fundamental**: ~45 Hz, starting at Pi/2 phase for maximum attack punch.
- **Pitch envelope**: `freq * (1.0 + 0.15 * exp(-40t))` — 808-style punch that quickly settles.
- **2nd harmonic**: Very subtle (0.08 mix) for presence on small speakers.
- **Amplitude**: Slow decay (exp(-2t)) for long, clean sustain. Slight attack boost (1.0 + 0.2 * exp(-60t)).
- **Saturation**: Clean — `tanh(mixed * 1.2) * 0.95`, lower drive than drums for pure sub.
- **Buffer duration**: 2.0s.

---

## Cymbals as modal plates (ride, hi-hats — `gen_cymbal_modal`, 2026-09-28)

The ride and all four hi-hats run one physically-motivated voice, `gen_cymbal_modal`
(`src/c/modular_stages.c`), instead of the legacy fixed-partial cymbal renderers.
It is selected per slot by **`gen1_cym_modes` ≥ 1 on a plate variant** — hi-hat (0),
open hi-hat (1), ride (4), crash (5). The cowbell (2) and shaker (3) are not plates
and always take their legacy voice; `gen_cym_modes` 0 (the identity) keeps every
legacy cymbal byte-identical.

### The physics it encodes

- **Nothing in a struck cymbal is noise.** A thin plate is a near-UNIFORM carpet of
  inharmonic modes: plate modal density is constant per Hz (unlike a string's sparse
  harmonic series). What sounds like "wash" or "hiss" is hundreds of ringing modes.
  Modal synthesis of cymbals needs hundreds of modes (Skare & Abel, DAFx-19).
- **Doublets.** Asymmetric plate modes split into pairs a few Hz apart; their slow
  beating is the shimmer / "fuzz" of a ride (the reference ride has pairs 4–12 Hz
  apart).
- **Frequency-dependent damping.** Higher modes radiate and damp faster, so a bright
  strike melts into a warm ring (measured on the reference ride: ~17 dB/s at 1 kHz
  rising to ~45 dB/s at 11 kHz, an exponent ≈0.5; only the top octave — the stick
  tick — dies in tens of ms).
- **The strike sets the spectrum.** Stick contact time, strike point (a random height
  on each mode shape → uneven, Rayleigh-like mode amplitudes) and radiation shape
  which modes are excited — modelled as spectral humps.
- **A hi-hat is two cymbals clamped together** (Sekiguchi & Samejima, Acoust. Sci. &
  Tech. 44(5), 2023): after the stick hit the top cymbal keeps colliding with the
  bottom one (inelastic rattling), adding the noise-like component; a looser pedal
  rattles longer and damps less. That rattle is what gives a closed hat its ~8 ms
  swell and chattering ~40 ms plateau.
- **Ride-level playing is the linear regime** — the nonlinear energy cascade that
  matters for a crash can be ignored for a ride or a hat.

### The model

1. **Mode bank**: `cym_modes` modes spread uniformly between `mode_lo` and `mode_hi`
   (× `cym_tune`) with up to half a slot of scatter. Amplitude = spectral envelope ×
   Rayleigh deviate × random sign. Envelope = log-Gaussian **tone** hump (`tone_f`,
   `tone_w` octaves) + optional **body** hump + optional **sparkle** hump + a flat
   **wash** floor.
2. **Doublets**: when `cym_beat` > 0 every mode gets a partner at `f·(1 + beat·U(0.3,1))`
   at 30–100 % level.
3. **Damping**: `rate(f) = env_tail·(f/1 kHz)^decay_tilt + env_fast·(f/hf_corner)^6` Np/s,
   each mode jittered ±15 %.
4. **Resonators**: Mathews–Smith phasors (a complex state rotated and shrunk by
   `e^(jω−α)` per sample — 4 multiplies, no `sin()`, unconditionally stable), each
   rendered only until it falls 140 dB.
5. **Rattle** (hat variants only): the bank is the plate's impulse response, so the
   output is that response convolved with the stick hit plus a Poisson train of
   collision impulses — rate `rattle_rate·e^(−t/rattle_decay)`, amplitude
   `rattle·e^(−t/rattle_decay)·U(0.2,1)·±1`. Drawn after every mode, so a render
   without rattle consumes the same noise sequence.
   With `cym_drive_delay` > 0 the plate is DRIVEN instead of struck: the stick's own
   impulse response is dropped and the train starts `drive_delay` s after the hit
   (its envelope measured from its own start) — snare wires, which the stick never
   touches and which only start slapping once the snare head moves.
   With `cym_rattle_hold` > 0 the collision RATE decays with its own time constant
   (`rattle_rate·e^(−t/rattle_hold)`) while the AMPLITUDE keeps `rattle_decay`, so the
   train stays dense — a fused buzz — until it is ~80 dB down (event cap 65536; each
   impulse response truncated at −120 dB of its peak). `hold` 0 = the legacy coupled
   decay, byte-identical. See "Rattle is measurable" below for why this exists.
6. **Variant = mounting** (physical, not a preset): ride/crash force the rattle off;
   the open hat multiplies `rattle_decay` ×5 and the ring (`env_tail`, `env_fast`) ×0.3
   — the pedal loosened. So the Variant selector is never a dead control.
7. Onset ramp (`cym_attack`), a 10 % raised-cosine end fade, **peak-normalization** (so
   knob moves change colour, not loudness), then × `tone_m`. Optional rivet **sizzle**
   (`noise_m` / `noise_d`): band-passed noise gated by the plate's own amplitude — never
   a free-running noise burst. No saturation, no LFO.

All random draws come from the slot's seeded noise generator in a fixed order —
deterministic, and the mode layout is part of the sound (see "mode count" below).

### Knobs (Phase-18 tail group, per slot; 0 = modal default)

| Field | Synth-tab label | Meaning (units) | Default when 0 |
|---|---|---|---|
| `cym_modes` | Plate Modes | mode count; ≥1 switches the plate on | legacy voice |
| `cym_mode_lo` / `cym_mode_hi` | Lowest / Highest Mode | band edges (Hz, × tune) | 300 / 18000 (capped < 0.45·sr) |
| `cym_decay_tilt` | Highs Fade | damping exponent vs frequency | 0 (flat) |
| `cym_hf_corner` | Tick Corner | where the extra top-octave damping reaches `env_fast` (Hz) | 10000 |
| `cym_beat` | Shimmer | doublet split (fraction of f) | 0 (no doublets) |
| `cym_tone_f` / `cym_tone_w` | Strike Tone / Width | main hump (Hz / octaves) | 3000 / 0.6 |
| `cym_body_f` / `_w` / `cym_body` | Body Tone / Width / Body | body hump (Hz / oct / gain) | 1500 / 0.15 / 0 |
| `cym_spark_f` / `_w` / `cym_spark` | Sparkle Tone / Width / Sparkle | stick-sparkle hump | 11000 / 0.3 / 0 |
| `cym_wash` | Wash | flat mode floor (gain) | 0 |
| `cym_attack` | Softness | stick-contact onset ramp (s) | 0.0005 |
| `cym_rattle` | Rattle | collision amplitude vs the stick (hats only) | 1.0 |
| `cym_rattle_rate` | Rattle Speed | collisions/s at t=0 | 2000 |
| `cym_rattle_decay` | Rattle Length | rattle time constant (s) | 0.06 |
| `cym_rattle_hold` | Buzz Hold | >0 = the collision RATE decays with this time constant (s) instead of `rattle_decay`: the train stays dense (a buzz) while it fades | 0 (coupled) |
| `cym_drive_delay` | Wire Delay | >0 = a DRIVEN plate: not struck by the stick, excited only by the collision train, which starts this many s after the hit (snare wires) | 0 (struck) |

The legacy cymbal knobs keep working with modal meanings: `cym_tune` scales every
mode, `cym_env_tail` = ring decay at 1 kHz (Np/s), `cym_env_fast` = extra top-octave
damping (Np/s), `cym_tone_m` = level, `cym_noise_m` / `cym_noise_d` = rivet sizzle.

### How the shipped seeds were fitted (repeat this for the crash or a new cymbal)

1. **Measure the reference first** — band × time energy (which bands, how fast each
   decays), the fine envelope, spectral flatness (tonal vs noise-like), and the
   modulation spectrum of a band's envelope (beat rate). The ride's diagnosis came from
   this: every legacy take carried a flat white-noise burst above 11 kHz the reference
   does not have.
2. **Prototype in Python** (numpy) against a scalar objective: mean |dB error| of a
   1/3-octave × time-window energy map vs the reference (+ the fine envelope and
   log-flatness for the hats). Coordinate descent, averaged over 3 random seeds so the
   fit does not chase one realization.
3. **Refit on the REAL C render** with a scratch harness that renders candidate
   param-sets through the production path (`renderModularP` + seed +
   `recipeParamsToModular`) to WAVs in one `go test` run — ~0.3 s per render.
4. **Hand the ear 4–5 labelled, LEVEL-MATCHED takes** plus a level-matched copy of the
   reference, and ship what the ear picks. Results: ride error 2.95 dB (vs 12.8 for
   the best legacy take); closed hat 4.1 (vs 32 legacy on the stricter hat objective).

**Lessons**
- ⚠️ **Mode COUNT acts like a random seed**: changing it reshuffles the whole layout
  (fit error swung 2.9–5 dB across counts). Fine metrics are realization-noisy; fit
  the other knobs at a fixed count and let the ear judge.
- ⚠️ **The energy map is blind to beat rate.** `cym_beat` was set from the modulation
  spectrum (reference beats at 8–20 Hz), yet the ear chose the SLOWER-shimmer take
  ("a bit more metallic"). Don't "correct" a by-ear choice toward a metric.
- **Flatness is what makes a hat organic.** Matching the band map alone left the hat
  tonal (flatness 0.09 vs 0.29); adding flatness to the objective drove mode count up
  and made damping broad — exactly the physics (contact damping).
- A white-noise layer is the fastest way to make a cymbal sound "clappy" and
  "electronic". Density of modes, not noise, is the wash.

### Adding knobs to a gen-slot voice (the checklist this took)

1. ABI: add the rows to `modularFields` (`internal/audio/modular_fields.go`) in the
   family's block, with the identity the knob has when a seed does not set it (that
   identity is ALSO what C sees — there are no C defaults), then `make gen-synth-abi`
   (regenerates `src/c/modular_params.gen.h` + `src/js/synth_param_abi.gen.js`) and
   re-freeze `testdata/modular_schema_layout.json` (`TestModularFieldsReproduceTheShippedLayout`).
   The schema, the Go marshal (`modularBlock`) and the JS layout all derive from the row.
2. Bounds in `modular_recipe.go` (`appendGenSlotDefs`; keep the identity inside the range).
3. UI: a `modularStageKnobs` row (label + macro card) in `modular_family_stages.go`, a
   unit in `modularStageKnobUnits` if it is Hz/s, the `advancedTierParams` entry if it
   is fine-tuning, and a plain-English gloss — an i18n KEY (`i18n/keys.go` + the list,
   `catalog_en.go`, `catalog_es.go`) mapped in `ui/plain_english.go`
   (`TestP4_EveryExposedParamHasPlainEnglish`).
4. Visibility: a knob that only matters in some configuration needs a gate like
   `synthCymbalModalParamHidden` (`synth_panel_zone.go`), which takes the instrument's
   MERGED params — otherwise every instrument of the family shows dead knobs. Contextual
   knobs are exempt from the Advanced-tier cap (`TestP4_TierClassification`).
5. Tests that WILL fail until handled: `TestRenderModularP_EveryParamMutatesInContext`
   needs a context case where the knob has authority; `TestDeclaredModuleValuesAreLive`
   flags a declared value no shipped document moves (list it in
   `dead_module_values.txt`). C reads the field directly — no NaN or
   "0 means default" helper (`TestNoFallbackHelpersRemainInC`); guard only genuinely
   invalid input (a divide by zero, an index), never substitute a value.
6. Rebuild
   `build/libdrums.a` and `src/js/drums.single.js`, re-pin the one-shot golden(s),
   regenerate `instrument_fundamentals.gen.go` (`make gen-fundamentals`) and the
   loudness table (`make measure-loudness`).
7. Browser check: render the id through the real WASM path AFTER its defaults are
   seeded (see `AGENTS.synth.wasm.md`) and compare with a native render at 48 kHz — a
   `play_ui.html` harness needs `play_ui.wasm` rebuilt (`make wasm-playtest`; `make
   wasm` only builds `main.wasm`), or it silently runs the old Go.

Scenes: `crop_synth_ride_modal_root`, `crop_synth_ride_modal_tone`,
`crop_synth_hihat_rattle`, `crop_synth_hihat_punch_more` (Wire Delay),
`crop_synth_hihat_length_more` (Buzz Hold), `crop_synth_cowbell_tone_no_plate`. Guard:
`TestCymbalModalKnobsOnlyWherePlateRuns`.

---

## Organic snares & the snare-vs-tom line (2026-09-29)

The organic snares LAYER existing voices across gen slots — config only (see
`organicSnareWires` in `instrument_seeds.go`):

- **slot 1 = the head** (source 7) with its white-noise wires OFF (`snare_wire_m` 0).
  Variant 3 (acoustic modal bank: inharmonic head modes + a fixed shell hum) RINGS;
  variant 0 (classic) dies in ~200 ms — use it for tight snares.
- **slot 2 = the wires** as the cymbal modal plate on hi-hat variant 0 (so the collision
  rattle runs): a broad (~3 oct), heavily damped mode carpet ~1.1–11 kHz. Snare wires
  physically ARE a collision rattle against the snare head (Bilbao's snare model treats
  them that way). Set `cym_drive_delay` (3–6 ms) so they are DRIVEN, not struck.
- **optional slot 3** = extra paired head modes (plate on ride variant 4 — no rattle,
  ~40 modes 350–2400 Hz, `cym_beat` 0.01) for the "coupled heads" complexity a
  six-mode head lacks, or the legacy metal cluster for an electronic edge.

**Why the old snares sounded fuzzy/noisy:** white-noise wires put energy flat up to
18 kHz; real snares roll off above ~10 kHz. Energy above 10 kHz in the first 100 ms:
real snares ≈ −23 dB, `snare` −19, `snare-acoustic` −14, the old open hat −3.

**Snare vs tom is measurable — check it before calling a voice a snare:**

| | wires (2–8 kHz) vs head (150–1000 Hz), 20–60 / 60–150 / 150–300 ms | head ring (250–1000 Hz to −30 dB) | high energy >6 kHz in the first 10 ms |
|---|---|---|---|
| real tight snare (freesound) | −3 / +2 / +7 dB | 90 ms | −43 dB (the stick hits the HEAD first) |
| current `snare` | −5 / +5 / +9 | 130 ms | −12 |
| `tom-high-acoustic` (heard as a TOM) | −11 / −12 / −20 | 580 ms | −16 |
| `snare-deep` (heard as a snare, "a bit tom-like") | −12 / −11 / −17 | 700 ms | −18 |

In a snare the wires catch up with the head within ~60 ms and outlast it while the
head ring dies (the wires damp the snare head). Wires that stay far under a
long-ringing head = hollow = tom — the first fit against the open acoustic reference
landed there, and the listener immediately heard a high tom (now shipped as one).
"Noisy/rattly at the start" = high energy in the first ~10 ms; a real stick hit puts
almost none there, which is what the driven wire plate fixes.

### Rattle is measurable — `fingerprint.TailRattle` (2026-09-30)

The listener heard "rattle throughout" on every wire-layered take, including ones whose
spectrum/envelope fit looked right — averaged descriptors cannot see it (same energy,
different distribution in time). Cause: with `rattle_hold` 0 the collision RATE decays
with the amplitude, so late in the tail the train thins below ~100/s and the ear hears
each collision as a tick instead of a buzz. Isolated by rendering layers apart: head
alone = clean, wires alone = rattly, collision rate ×16 = clean.

`TailRattle(samples, rate)` = ticks/s summed over 2–4 / 4–8 / 8–16 kHz: each band's
1 ms RMS envelope from 120 ms after the onset, while the band is within 45 dB of the
WHOLE sound's loudest frame (a band's own peak as the reference made faint, inaudible
bands count — a real snare recording scored 170), and a tick = a frame >6 dB above
its 21 ms moving average. Calibration at 44.1 kHz:

| | TailRattle |
|---|---|
| three real snare recordings, a real closed hat | 0 |
| clean shipped drums (hats 21.7, `snare-acoustic` 22.7, all others 0–6) | 0–23 |
| every take the listener called rattly (S4 punch 168, piccolo conga 717, `snare-deep` / `tom-high-acoustic` as first shipped 608 / 663) | 150–720 |
| the same takes with `rattle_hold` 1.5 + rate ×3, or without the wire layer | 0–7 |

Guard: `TestShippedDrumsDoNotRattle` (`drum_rattle_test.go`, real build — `make
test-real`): every gen-source 5–9 drum stays under 50; `BEATMO_RATTLE_REPORT=1` prints
every score. A new rattly drum fails; `knownRattlyDrums` is an exemption ratchet (empty).

Shipped (all picked by ear in A/B rounds; the rows' seeds reproduce the picked takes
exactly):

| id | layering | TailRattle |
|---|---|---|
| `snare-deep` | acoustic head (variant 3) at 172/258 Hz + darker wires, Buzz Hold 1.5 s @ 4615/s | 6.7 |
| `snare-punch` | classic head (`snare`'s) at 210/315 Hz + plate wires with Buzz Hold 1.5 s — the old open hat's "tight, fuzzy snare", made organic; retuned for crack 2026-09-30 (plate centre 7.3 → 1.2 kHz, modes 1.1–6.1 kHz, the head's own wire burst back on at `wire_m` 0.22) | 0 |
| `tom-high-acoustic` | the `snare-deep` head WITHOUT wires (`snareDeepHeadSeed`) — "the only high tom that sounds good"; the 225 Hz fitted head was rejected as a tom | 0 |
| `conga-organic` | acoustic head tuned UP to 330/520 Hz with a long ring, no wires, + slot-3 coupled-heads plate (ride mounting, 40 modes 350–2200 Hz) | 0 |
| `conga-organic-dry` | the same head without the coupled-heads plate — kept as a separate sound ("not the same and both sound great") | 0 |

Wires that are removed must be removed from the SEED, not switched off with
`gen2_source` 0 — the plate params left behind are dead seed params (liveness ratchet);
likewise `snare_noise_d` with `snare_wire_m` 0, and `snare_attack` / `snare_tail_d`
on the classic head. All five baseline the same POST-gate entries as every snare in
`known_dead_seed_params.txt`, and all five are peak-capped in loudness.

**Adding drum rows moves every other instrument's loudness.** `measure-loudness` sets
its target from the MEDIAN instrument, an order statistic: new rows shift the median
one or more neighbours, and every uncapped instrument's amp moves with it. Two rows
moved it ~0.2 % (inside `TestLoudnessTableIsNotStale`'s tolerance, so only the new
entries were added); three more moved it ~0.14 dB (outside), so the table was
re-measured and the ~90 uncapped goldens re-pinned — all length-preserving, and no
capped instrument moved. Expect that re-pin whenever the roster grows.

### A snare needs a CRACK — measure it at equal peak (2026-09-30)

The listener's next verdict on every shipped snare: "lacks the loud crack snare drums
must always have… too weak, soft and fuzzy". It is measurable, but only at EQUAL PEAK:
every snare is peak-capped in the loudness table (`measure-loudness` prints them under
"pinned at peakCap"), so in the app a snare's peak is fixed and what the ear hears as
weak or strong is how much energy sits under that peak. Measured against nine real
snare recordings (local analysis only — licences unknown, never committed), each
normalized to peak 1:

| | real snares (median, range) | shipped before |
|---|---|---|
| crack: RMS of 1–5 kHz over 5–50 ms | −17.7 dB (−13 … −26) | −21 … −26 (the rimshot-based `snare-crack` excepted, −4) |
| early RMS, 0–50 ms (full band) | −7.6 dB (−4.6 … −11.8) | classic heads −10.5 … −11 |
| fizz: RMS above 7 kHz over 0–50 ms | −28 dB (−21 … −37) | `snare-acoustic` −19, `snare-punch` −22 |

- The crack SHARE of the early energy misleads: the classic head scored a high share
  from a 0–10 ms fizzy spike, which is exactly what spends the peak without sounding
  loud. Use the absolute level at peak 1.
- The fix is to move the wires DOWN into 1–2.5 kHz and drive the head harder: on the
  classic head `snare_tune` scales the fixed 1.8/3.2/4.5 kHz wire bands (0.41 → ~0.7–1.8
  kHz); on a modal-plate wire layer, lower `cym_tone_f` / `cym_mode_lo` /
  `cym_hf_corner`. The head's own soft-saturation then makes the start denser, like a
  compressed recorded snare.
- Fit objective (session scratch, reproducible): median of the references' 1/3-octave ×
  time map (windows 0–5 / 5–20 / 20–50 / 50–100 / 100–200 / 200–400 ms, early ones
  weighted up) + a floor on early RMS + a crack floor (−16 dB) + a fizz ceiling (−26 dB).
  Real snares score 2.6–4.8 on the map alone; the picked takes 3.5–3.9.
- Ear verdicts on the fitted takes: `snare-fat` and `snare-punch` — "fire", shipped.
  `snare-acoustic` — still "hollow but high frequency like a conga or high tom", and its
  crack "fuzzy/noisy" where it should be sharp and tight: NOT shipped. `snare-deep` —
  every take "bad and rattly", NOT shipped, although `TailRattle` scored those takes 3–7:
  the metric does not capture what the listener calls rattle there.
- `snare-lofi` / `snare-ghost` kept the pre-crack fat seed (`snareFatOriginalSeed`).

### Toms — research notes for the next step

- A tom is a batter head + a resonant head coupled by the shell's air, with NO snares.
  Ideal membrane modes run 1 : 1.59 : 2.14 : 2.30 : 2.65 : 2.92 : 3.16 …; air loading
  and the resonant head pull the first three toward ~1 : 1.5 : 2, which is why a
  well-tuned tom (like a timpani) has a definite pitch.
- **Pitch glide** (tension modulation): a hard hit stretches the head, so pitch starts
  high and falls over the first ~50–150 ms — more on loose/low floor toms. It is THE
  tom signature; the 808 tom is a sine with exactly that sweep.
- The (0,1) mode radiates efficiently and dies fast; the (1,1) family carries the ring.
  Resonant-head tuning relative to the batter shapes it: equal → long sustain; bottom
  higher → a pitch drop with a choked after-ring; bottom lower → deeper, bending.
- Rough fundamentals: 10" rack ~180–220 Hz, 12" ~140–180 Hz, 14" floor ~100–130 Hz,
  16" floor ~80–110 Hz; rack ring ~0.5–1.5 s, floor ~1–2.5 s.
- Existing pieces: the `tom` family (source 6, 808-style: sine + 1.5×/2.1× overtones +
  a fast pitch sweep), `high-tom-organic` (kick variant 7 modal), and
  `tom-high-acoustic`. A proper tom family wants a membrane-ratio mode layout, a
  tension-modulation glide, doublet beating (two heads) and a stick click — and a tom
  reference recording per size (the corpus has `tom-vcsl`).

---

## Configurable Kick Stage (modular gen-bank kick voice, `source==5`)

The **configurable KICK stage** exposes the gen-bank
kick voice
(`modular_gen_slot_kick`, `gen_source==5`) as a fully tunable, UI-editable
modular instrument — a node's kick is *described by config*, not hard-coded. The
shipped kicks — `kick`, `kick-deep`, `kick-lofi`, `kick-snap`, `kick-tight` (the
five ex-legacy kicks, `drumKick*Seed`), `dnb-kick`, `kick-electro`, `kick-808`,
`kick-acoustic`, `kick-punchy`/"Dry Kick", `zgump-kick`/"Organic Kick", `raw-kick`,
the 2026-10 fuller/realer pass (`kick-fat`, `kick-acoustic-heavy`, and the variant-8 PHYSICAL kicks
`kick-boom-bap`, `kick-brutal`, `kick-rock`, `kick-wood`, `kick-airy`,
`kick-organic-tight`, `kick-muted`, `kick-muted-room` — see § Physical kick and
§ Making a kick FULL and REAL), plus the variant-7 tom `high-tom-organic` — are all
documents of this one stage (once generated from `kickSeedWith`). This is the
path to add new kicks: copy a kick document and change its values (§ Adding a new
built-in instrument).

### Editable params (per gen-slot, slot 1 by convention)

| Param | What it does | Bound |
|---|---|---|
| `kick_enabled` | gates the voice (the Synth-tab KICK enable pill) | 0/1 |
| `kick_variant` | base / deep / punchy / lofi / tight / **hybrid** / **modal** / **acoustic** / **physical** (different baked DSP) | 0–8 |
| `kick_h2` `kick_h3` `kick_h4` | harmonic gains (2f0/3f0/4f0). **Variant 7**: gains of the INHARMONIC modal partials (the spectral shape) | h2 0–2, h3/h4 0–1 |
| `kick_mode_detune` `kick_mode_gain` `kick_mode_decay` | **variant 6** (coupled resonant mode: split Δf, level, ring) AND **variant 7** (`mode_detune`=inharmonic STRETCH of the modal partials, `mode_gain`=high-mode level, `mode_decay`=high-mode damping) | detune 0–0.5, gain 0–2, decay 0–40 (⚠️ `acousticKickSeed` and one other seed set 45 / 55, above the declared max) |
| `kick_reverb` | **variant-7 acoustic only**: baked feedback-comb room/echo TAIL amount (dark, diffuse, decays fast so it never blooms over the attack). 0 = dry | 0–1 |
| `kick_env0` `kick_env1` | fundamental / harmonic exp-decay rates (/s) — **lower = longer sustain** | 0–40 |
| `kick_pe_amt` `kick_pe_rate` | pitch-drop depth (`f0·(1+amt·e^-rate·t)`) + speed — the **drumhead glide** | amt 0–3, rate 0–100 |
| `kick_click` | beater-click amount (HP noise transient) | 0–1 |
| `kick_noise` | noise-thud / room amount | 0–1 |
| `kick_attack` `kick_fade` `kick_sat` | attack-boost amount / global-fade rate (tail) / saturation pre-gain (Phase-9) | atk 0–3, fade 0–32, sat 0–4 |
| `kick_hold` | **variant-8 only** ("Body Hold"): seconds the body rings UNDAMPED before decaying — the held body of a compressed sampled kick. 0 = off | 0–0.3 |
| `kick_hiss` | **variant-8 only** ("Air"): the turbulent air / "vacuum" — near-brown noise in 25–120 Hz riding the body at tone level, plus a faint >1 kHz floor. 0 = off | 0–1 |
| `voice_freq_hz` | the settled fundamental (Hz) | — |

**Variant 8 REMAPS several knobs** (documented in the `gen_kick_physical` header
comment): `click` = beater HARDNESS (contact time 7 ms felt → 0.7 ms hard + slap),
`attack` = strike VELOCITY, `h4` = skin-cluster level, `mode_decay` = skin damping
(×4), `mode_detune` = body ratios harmonic (0) → membrane (0.5), `mode_gain` =
resonant-head level, `reverb` = fixed-pitch shell/room bank, `fade` = extra decay in
ABSOLUTE time, `sat` = drive on the fundamental AND the glue-limiter strength.
`kick_*` are read directly in C (no per-variant fallback literal since Phase C); the
document spells every value. The fundamental = `gen1_freq × voice_freq_hz` (ratio mode).

### Playback: every voice renders its document (historical trap, now structural)

Every instrument's voice is render(`Flatten(doc)`) on both platforms — there is no
"bare voice" path a seeded instrument can fall back to. Before documents, a seeded
built-in whose Render closure did not bake its seed played the unparameterized ~218 Hz
modular voice (a sine blip). `TestDnbKickPlaybackPathRendersKick` still guards the
dispatch: scan a fine 30–400 Hz grid, since a coarse low/high band test passes for both
(218 Hz leaks into the low bins).

### Adding a kick variant

1. Copy a kick document of the same `gen1_kick_variant` and change its `gen1_kick_*`
   values. A **variant-8** kick must spell EVERY variant-8 value (a 0 is a real value:
   a 0 `env0` means "no damping" — the first physical take rang for 540 ms).
   `render.seconds` sizes the render buffer; the global fade is buffer-normalized so
   it sets the tail window.
2. Add the id to `index.json`, then pack / ids / re-pin as in § Adding a new built-in
   instrument.

### Organic vs electronic — what the knobs control perceptually

Tuned A/B against reference WAVs (`cmd/synth-analyze` + a numpy all-metrics
analyzer: f0 / harmonic series / spectral centroid·rolloff·flatness·crest /
8 power bands / temporal centroid / **RMS-envelope + pitch-sweep trace over
time**). The decisive perceptual axes for a kick:

- **ORGANIC** (e.g. `dnb-kick`, tuned to *dflee4 "stuborn"*): a near-PURE deep
  sub (`kick_h2≈0.05`, almost no harmonics), a BIG smooth pitch glide
  (`kick_pe_amt≈2`, ~140→50 Hz — the drumhead tension drop, the #1 organic
  tell), and a **swell-in bloom instead of a click** (`kick_click=0`,
  `kick_attack=0`; enable the modular **amp ADSR** with `amp_attack≈0.02`,
  `amp_sustain=0` so the envelope is absolute-time/buffer-independent). NO sharp
  transient, NO strong harmonics.
- **ELECTRONIC** (e.g. `kick-electro`): the opposite — a strong beater
  `kick_click`, a sharp `kick_attack`, real harmonics (`kick_h2/h3`), a tight
  fast pitch snap, and `kick_sat` drive. Reads bright/punchy (centroid ~2.8 kHz).
- **808 sub** (`kick-808`, variant 1 "deep"): mostly sub with a deliberate body
  (`h2` 0.20, `h3` 0.05 — "not a pure sub"), a big 808 "pew" drop (`pe_amt` 2.4),
  very LOW `env0` (long ring) + a long `render.seconds` — a sustained sub-bass note.
- **ACOUSTIC** (`kick-acoustic`, variant 7): the inharmonic MODAL bank with beater
  excitation plus a baked room tail (`kick_reverb`) — see the ACOUSTIC / REAL-RECORDED
  entry below.
- **PUNCHY / RAW / BRUTAL** (`kick-punchy`, `kick_variant: 5` — the **layered
  kick DSP**). Tuning the single-voice kick toward a punchy/raw SAMPLE
  (*yellowtree hybrid-kick-1*) hit a hard wall after 7 architectures: "raw" grit
  needs broadband noise (→ hiss when synthesized clean) and "punch" needs an
  un-clipped sharp spike (which distortion flattens) — the two fight in one
  voice. **Variant 5** (`modular_gen_slot_kick`, src/c/modular_stages.c) resolves
  it with 3 **DECOUPLED** layers: (1) a pitch-swept harmonic BODY, smoothly
  waveshaped (`tanh`) for raw harmonics; (2) a separate **colored-noise TEXTURE**
  — band-passed, scaled by the body's smooth ENVELOPE and **added on top, NOT run
  through the distortion**; (3) a short bright **CLICK** = attack snap + crest
  punch; final **gentle `tanh` soft-limit (NOT a hard clamp)**. Re-purposed knobs:
  `kick_sat`=body distortion drive, `kick_noise`=texture amount, `kick_click`=attack.

  ⚠️ **DECOUPLING IS WHY IT'S SMOOTH, NOT SPITTY** — the headline lesson. The
  first version *fused* the noise into the waveshaper and multiplied it by the
  body WAVE: the noise then amplitude-modulated at ~2× the pitch and the `tanh`
  amplified its peaks, so the HF content **sputtered** between dark and
  very-bright every few ms ("spit"), and the bright spikes read as "high-pitched".
  **Averaged metrics (flatness/centroid) HID this** — only a **time-resolved
  per-window HF** analysis exposed it (averaged centroid even said the reference
  was *brighter* than the sputtering synth). The pitch sweep must also be **FAST**
  (the variant-5 C default `pe_rate` is 90; the shipped `punchyKickSeed` settled on
  `pe_amt` 0.9 / `pe_rate` 35): a slow sweep leaves the loud body at the high start pitch
  (autocorr **f0=105 Hz**, "high pitched"); fast → the high part is just the brief
  click and the body settles to the low fundamental (f0≈ the reference's 60 Hz).
  Guards: `TestKickVariant5BodyNotSpitty` (`dnb_kick_test.go`: worst upward
  per-window body-HF jump ≤4×) +
  `TestKickVariant5LayeredHasPunchAndCleanTail` (crest≥5). NO new ABI — variant 5
  reuses the kick knobs; only `kick_variant` Max 4→5 + the "Hybrid" enum label.
  The texture knob (`kick_noise`) is now SAFE to push for more rawness (it's
  decoupled → it won't sputter).

- **MODAL / ORGANIC** (`zgump-kick`, `kick_variant: 6` — the **coupled two-mode
  DSP**). Tuned to *83768 zgump kick-pack-0708* (a mid-bass organic thump:
  dominant ~80 Hz, ~71% energy in 60-120 Hz + ~25% in 120-250, big smooth
  240→~68 Hz glide, crest ~3.4, and a **plateau-then-tail-bloom** envelope — NOT
  a plain exp decay). The reference's non-exponential envelope is the acoustic
  signature of **two coupled, slightly-detuned drumhead modes** (batter head +
  resonant head): the batter is struck (loud, fast-damped), energy sloshes into
  the less-damped resonant mode and back, and the two BEAT (Δf = f0·detune). The
  plateau + delayed bloom therefore emerge from a **real mechanism**, not
  hand-drawn envelope segments — which is what lets it reproduce the organic
  complexity WITHOUT overfitting. Variant 6 (`modular_gen_slot_kick`) layers: a
  pitch-swept **batter** mode (fast swell → peaks ~25 ms) + a detuned, less-
  damped, **delayed** resonant mode (its slow ring is the bloom) + harmonics
  that ride the batter's swell (share the `(1-exp(-90t))` ramp — else the onset
  is all high-pitch 2f0/3f0 and the peak jumps to t=0) + a low beater thud + the
  same decoupled low-mid grit texture as variant 5 (additive, never through the
  waveshaper). **New ABI knobs** (`kick_mode_detune`/`gain`/`decay`, Phase-11): detune sets the beat/split Δf (the plateau/bloom period),
  gain the resonant level, decay its ring length. `kick_variant` Max 5→6 + the
  "Modal" enum label. Tuned via the native harness (`zgump_kick_tune_test.go`)
  against `internal/audio/fingerprint`'s **time-resolved kick metrics**
  (`KickAnalyze`/`KickDistance` — envelope-shape trace + landmarks, pitch-glide
  curve, per-window bands, crest, HF-smoothness). Guards:
  `TestZgumpKickIsOrganicModalKick` (bloom present, plateau sustained, glide
  present, HF smooth, bands fundamental-dominant — the anti-"watered-down" net) +
  the seeded-kick playback check in `TestDnbKickPlaybackPathRendersKick`.
  ⚠️ **Buffer-length coupling**: the global fade is buffer-normalized, so the
  swell/bloom only survive if `render.seconds` (the render window, a fixed time — no tempo)
  matches the tuning buffer (~0.35 s). A short buffer squashes them.

- **ACOUSTIC / REAL-RECORDED** (`kick-acoustic`, `kick_variant: 7` — the
  **inharmonic modal bank + reverb DSP**). Tuned to the real sample *36010
  sandyrb dnb-kick-003*. **Why the earlier "tight" variant-4 (pure sine +
  band-pass click + LP noise) sounded synthetic no matter how it was tuned:** a
  real recorded kick's *spectral SHAPE* (its coupled inharmonic membrane/shell
  modes + a real beater impact + room) can't come from one sine — matching bands
  + crest + envelope still leaves the timbre wrong. Variant 7 layers: a small
  **inharmonic modal bank** (fundamental + partials ×2.1/×3.4/×5.2, stretched by
  `mode_detune`, each STRUCK at t=0 → sharp attack, with its own decay) + a
  **band-pass beater click** + **envelope-shaped broadband noise** (recorded air)
  + **body-only saturation** (click added un-compressed so the crest survives) +
  a baked **feedback-comb reverb tail** (`kick_reverb`, Phase-12 ABI knob; dark,
  diffuse, low-feedback so it decays fast and never blooms over the attack).
  Reuses the variant-6 `kick_mode_*` knobs (stretch/high-mode level/damping) — no
  new knobs except `kick_reverb`. `kick_variant` Max 6→7 + "Acoustic" enum.
  The **"High Tom Organic"** instrument (`high-tom-organic`) is this same variant
  saved at a brighter/higher tuning (its mid modes read as a tom); `kick-acoustic`
  is the lower/heavier/fundamental-dominant kick tuning.

### Physical kick (variant 8) — a tension-modulated modal drumhead

`gen_kick_physical` (`src/c/modular_stages.c`). Built from the physics, then fitted
to recordings; every stage exists because a measurement or an ear verdict demanded it:

- **Beater** = a raised-cosine FORCE pulse, contact time 7 ms (felt) → 0.7 ms
  (hard). Its spectrum nulls at 2/Tc and falls −18 dB/oct, so a soft beater drives
  only the body (a muted kick) and a hard one excites the skin cluster — hardness IS
  the excitation. A 3 ms contact SLAP (brightness and level ∝ hardness) supplies the
  broadband smack of a hard beater (AL_BD-style "brutal").
- **Head** = complex one-pole modal ROTATORS (amplitude-stable under per-sample
  frequency modulation; a direct-form biquad changes gain and zippers when ω moves).
  Body modes blend from a harmonic stack (1/2/2.98/3.95) to the MEMBRANE ratios
  measured on real kicks (1/1.72/2.04/2.42) via `mode_detune`; above them a short
  inharmonic skin cluster (5.3–11.2·f0).
- **Pitch glide = tension modulation** (Avanzini/Marogna/Bank): the quasi-static
  tension is proportional to the membrane energy, so every mode moves together:
  `f·sqrt(1 + a(a+2)·v²·Ê·e^(−pe_rate·t))`, Ê = normalized modal energy. `pe_amt`
  keeps the shared meaning (onset = f0·(1+amt) at velocity 1); a harder hit glides
  further.
- **Resonant head** = a less-damped mode driven by the batter fundamental, riding
  the SAME pitch path (bloom/sustain, no beating — see the fuzz lesson below).
- **Shell / room** (`reverb`) = a FIXED-pitch resonator bank (1.38–4.52·f0)
  struck by the pulse + a dark noise burst: the dense 70–200 Hz after-ring of a
  recorded kick. Resonators, never feedback combs (combs groan at n/D Hz).
- **Air / vacuum** (`hiss`) and **Body Hold** (`hold`) — see the table.
- **Fullness**: only the fundamental + resonant head (one pitch) go through a
  BIASED tanh (bias 0.15 → warm H2+H3; neg/pos asymmetry 1.36 ≈ the measured 909's
  1.35), DC-blocked. Everything else is added clean. Then a look-ahead GLUE
  LIMITER (gain-riding, ceiling `1 + 2/(1+sat)`, 1.5 ms look-ahead, 0.5 ms attack,
  25 ms release) holds the transient near the body, and the voice is
  peak-normalized to 0.98.

### Making a kick FULL and REAL — measured guidelines (2026-10)

The user's brief was "the kicks sound OK but weak — fuller, louder, bassier,
punchier", then "all of them sound very electronic — I want a REAL kick". What
those words turned out to mean, measured against the user's sample library:

1. **"Weak" = little energy under a FIXED peak.** Most kicks are pinned at the
   loudness table's peak cap (`make measure-loudness` lists them), so in the app the
   peak is fixed and loudness = how much RMS sits under it. Judge and A/B kicks at
   EQUAL PEAK (−1 dBFS), never RMS-matched — that hides exactly this defect (the
   `refs session`/`make ab` paths RMS-match; use peak-matched files for drums).
   Measured at equal peak: a commercial "raw kick" library median is −8.9 dB
   loudest-300 ms RMS and −5.9 dB early punch (60–200 Hz, 0–60 ms); `kick` was
   −12.2 / −9.2, `kick-electro` −18.0. `kick-fat` (−6.9, crest 3.4) is the fix on
   the variant-0 voice: saturation 0.55 → 1.2 (the old value was effectively linear),
   a bigger faster drop, longer body.
2. **Low crest needs GAIN RIDING, not more tanh.** The kicks the user called "fire"
   (`kick-fat`, a 9th-Wonder sample) have crest ≈ 3.4; a modal voice's transient
   pokes above its saturated body (crest 5.7) and peak normalization then pushes the
   body down. Waveshaping the whole sum to fix it intermodulates the modes (creak);
   a look-ahead limiter flattens the transient without new partials.
3. **"Fuzzy" = WARBLE.** `fingerprint.KickAnalyze().WarbleDepth` (2–30 Hz AM of the
   body): real kicks 0.41–0.62; the rejected fuzzy takes 1.8–3.4. Two causes found:
   a DETUNED resonant head drifting through the glide (beats), and a fixed shell mode
   at 0.86·f0 (~7 Hz beating once the glide settles). Rule: nothing may sit within
   ~±20% of the fundamental at a comparable level unless it rides the same pitch path.
   A Python 4 ms-envelope "warble" did NOT discriminate (it reads the waveform's own
   ripple) — use the Go metric.
4. **"Electronic" = a clean harmonic 1/2/3·f0 stack + a 3–4× glide.** Real kicks:
   membrane modes at ~1.7/2.0/2.4·f0, a glide of only ~2× finished in 20–30 ms,
   a short body (−24 dB by 50–140 ms) and a dense fixed low-mid shell ring.
5. **"Digital" = TONAL FRACTION.** Split each 5 ms frame into a median spectral
   floor (±1/6 oct = noise) and the excess above it (tone): a muted recorded kick is
   47% tonal, a sampled hip-hop kick 25%, the approved airy kick 31% — the rejected
   "digital" muted takes were 88–92%. The missing "air/vacuum" is near-BROWN noise
   concentrated in 25–120 Hz at TONE level (the air the heads pump through the
   shell/port), not hiss: the first two noise sources tried were flat broadband
   (−42..−50 dB to 16 kHz) and the fitter rightly turned them off. Band-energy maps
   are blind to this axis — score tone and noise separately.
6. **Most real kicks are QUIETER than synth kicks at equal peak** (−12..−17 dB vs
   −7..−9): realism and maximal loudness pull apart. Offer both (the user approved
   loud glued kicks AND quiet real ones).
7. **Deep ≠ low pitch alone.** "Much deeper body" was satisfied by a 45 Hz
   fundamental + a noisy (vacuum) body + short hold, fitted to a sampled kick — the
   pitch-only, long-boom takes were not picked.

8. **Retuning an EXISTING kick louder/fuller without changing its kind** (all twelve
   pre-2026-10 kicks were retuned; user-approved by A/B): optimize toward the
   equal-peak targets but PENALIZE drift of the >250 Hz time × ⅓-octave map
   (character), pitch drift (±4–10%), warble, and clipping. What bit:
   - The older variants have **no end fade** — lowering `fade` to lengthen the
     body left `kick` at −12.9 dB when its buffer ended: a truncation CLICK (and
     a boom, not a kick). Constrain the last 5 ms to ≤ −50 dB re peak.
   - The punchy/tight variants end in `softsat·k` into a **±1 hard clamp**; past
     drive ≈ 1.2 the onset flat-tops (thousands of samples). `kick-snap` clips 141
     samples BY DESIGN, so the constraint is "no more clipping than the original".
   - A short buffer (`render.seconds` 0.15) leaves no room for a body (the fade is
     buffer-normalized): snap and electro needed longer buffers (0.25 / 0.4 s).
   - "Louder but the same kick" (kick-tight, kick-acoustic): shrink the onset
     spike (`attack` → 0) and add modest drive; partial blends toward a fuller
     retune ADDED warble (0.8–1.3) — interpolating several knobs is not a safe
     "intensity" control.
   - Normalization then turns the loudest retuned kicks DOWN to the common level
     (kick-deep amp 0.58): that is still louder than before, when they sat capped
     BELOW it. The table normalizes flat RMS, which under-counts bass for the ear.
   - Before retuning a seed in place, freeze every seed CLONED from it
     (`kick-fat` was `cloneSeed(drumKickSeed)`; `raw-kick` built on the Organic
     Kick → `zgumpKickBaseSeed`), and re-verify every approved take byte-identical.

**Workflow that worked** (all scratch, never committed — the reference kicks are
the user's library): render candidates through the production modular render
(`zz_ridemodal_test.go`-style harness, a literal `seed` per take), peak-normalize,
analyse, fit, audition 8-hit 120 bpm loops. The fitter was coordinate descent over
the variant-8 knobs scoring: a time × ⅓-octave map (dB re peak), the 5 ms envelope,
loudness/punch/click/air scalars, the tone and noise maps + tonal fraction, and a
penalty when `WarbleDepth` exceeds the reference. Every C change after an approval
was re-checked for BYTE-IDENTITY of the approved takes (new knobs default to 0 and
draw noise only when on), and the shipped rows were verified byte-identical to the
auditioned WAVs.

### Perceptual kick metrics (`internal/audio/fingerprint/kick_metrics.go`)

`KickAnalyze`/`KickDistance` — TIME-RESOLVED, so averaged scalars can't hide a
regression (a real "spit"/sputter bug was invisible to averaged descriptors).
Layers, added as the ear demanded them:
- **Envelope** trace + landmarks (attack-peak, plateau, tail-bloom), **pitch
  glide** curve, **per-window bands**, **crest**, **HF-smoothness**.
- **Timbre** (weight 2.0): **MFCC** (spectral shape/color), **spectral-centroid**
  trajectory (brightness), **attack-window centroid** (beater brightness),
  **spectral flatness** (noisy-vs-tonal). Added when bands+crest matched but the
  acoustic "sounded nothing like the ref" — the timbre axes diverged.
- **LowMid** (`AttackLowMidRatio` = energy 40-100 / 100-500 Hz over the LOUD
  onset; KICK ≫ 1 fundamental-dominant, TOM < 1 mid-dominant — the kick-vs-tom
  discriminator) + **Reverb** (`TailRatio`, `TailDurationSec`, `TailFlatness`).
  Added when the acoustic "sounded like a high tom" and wanted "echo/reverb".

**Lessons:** (1) Match the **power bands** before chasing the exact harmonic-dB
table — the two conflict, and bands win perceptually. (2) The kick voice's
single-mode envelopes are exponential; a plain single voice CANNOT make a true
flat-plateau-then-cliff (a layered transient+sub reference) buffer-independently.
For an organic plateau+bloom, use the **variant-6 coupled two-mode** approach —
the beating produces the non-exponential envelope from a mechanism (do not chase
temporal-centroid with hand-drawn segments). (3) TRUST THE EAR — render the WAV
for audition; the metric can improve while the sound gets worse. (4) A slow,
gradual pitch glide (the reference glides over ~120 ms) fights band accuracy —
a deep/slow glide keeps early energy in 120-250; when they conflict, **bands
win** and the glide-trace term is the acceptable residual. (5) **Bands + crest +
envelope can all match while it sounds nothing alike** — that means you're
missing TIMBRE (MFCC / spectral centroid / flatness). Add the axis that diverges
rather than re-tuning against a blind metric. (6) A **pure sine body can't sound
"real/acoustic"** — a real recorded kick needs an **inharmonic modal bank** (the
variant-7 approach); no knob-tuning of a sine fixes the spectral shape. (7)
**"Lower pitched" perceptually = FUNDAMENTAL-DOMINANT** (kick), not necessarily a
lower F0: the sandyrb ref's F0 is ~90 Hz but it reads low because its mid modes
are quiet — measure `AttackLowMidRatio`, not just settle Hz. (8) A **reverb tail
raises RMS → fights crest**; keep it a quiet, fast-decaying, DENSE (multi-tap)
wash so it never blooms over the sharp attack (which is the punchy "soul"). (9)
`render.seconds` (the render window) must fit the reverb/tail T60 or the tail AND the crest
mis-measure. (10) A **"creaky door" / wooden buzz = intermodulation roughness**
from `tanh`-saturating a bank of INHARMONIC modes together (the products of
inharmonic partials are dissonant). Cure: keep body saturation LOW and let the
CLICK carry the punch — dropping `kick_sat` 0.8→0.25 halved the mid-band roughness
AND *raised* the crest (saturation was compressing the transient), a win-win. To
hunt a creak, ablate: render the seed with each element zeroed and measure the
tail's per-band energy + warble (`kick_creak_diag_test.go`) — this ruled out the
reverb and isolated the saturation. (11) **Reverb combs creak too** — a
feedback-comb reverb rings at its comb resonances (`n/D` Hz) which BEAT with the
fundamental into a low groan; use SHORT, close, MANY taps + LOW feedback + a small
dry send (a dense diffuse wash) so it never resonates or blooms over the attack.

---

## FM Instruments

The FM synth engine produces tonal, pitched instruments. The shared
`fm_render` core still lives in `src/c/fmsynth.c`, but the **Phase-7 FM
migration removed** the `PRESET_FM_*` tables, the `render_fm_*` /
`render_fm_*_p` wrappers, `fm_apply_params`, and the Go-side
`fmsynth_c.go`. The five presets are now built **inline** in
`modular_gen_slot_fm` (`src/c/modular_stages.c`, dispatched at
`gen_source==10` with `variant` 0=bass 1=bell 2=lead 3=epiano 4=pluck),
which bakes the same constants below and calls `fm_render`. The shipped
FM instruments are config-first rows (`fmBassSeed` … in `instrument_seeds.go`)
whose voices come from `bakedModularRender` like every other table row (the
`renderFM*Voice` helpers are test-only). The numeric recipes below are the
C preset literals; where a seed overrides one (e.g. the e-piano), the seed
value is what ships.

### FM Synthesis Fundamentals

**Core concept**: One oscillator (the *modulator*) modulates the phase of another (the *carrier*). The carrier produces the audible output; the modulator shapes its timbre by creating sidebands around the carrier frequency.

**Key parameters that shape FM timbre**:

| Parameter | Effect | Musical use |
|-----------|--------|-------------|
| Modulation depth (index) | Higher = more sidebands = brighter/harsher | Attack brightness, timbral complexity |
| Frequency ratio (mod:carrier) | Integer = harmonic, non-integer = inharmonic/metallic | Bells use ~1:3.5, bass uses 1:1 |
| Modulator envelope | Fast decay = bright attack fading to warm | Pluck/piano transients |
| Carrier envelope | Controls overall amplitude shape | Sustain, release |
| Number of operators | More ops = more complex spectra | Series chains for brightness, parallel for richness |

**Operator topologies**:
- **Simple pair** (2 ops): One modulator → one carrier. Clean, controllable. Used by fm-bass, fm-bell, fm-pluck.
- **Series chain** (3 ops): op2 → op1 → op0(carrier). Each modulator multiplies complexity. Used by fm-lead.
- **Parallel carriers** (3 ops): Two carriers + shared modulator, or independent carriers. Used by fm-epiano.

**Modulation matrix**: `mod_matrix[src][dst]` defines depth from any operator to any other. Allows feedback (self-modulation), cross-modulation, and arbitrary routing. 0.0 = no connection; typical useful values 0.5 (subtle) to 5.0 (aggressive).

### Engine Architecture

```
Per sample:
  1. Compute pitch envelope (exponential sweep in semitones)
  2. Gather modulation inputs from previous sample's operator outputs
  3. For each operator:
     a. Compute frequency = (base_freq * freq_ratio + freq_offset) * pitch_mult
     b. Compute ADSR envelope
     c. Advance phase accumulator
     d. output = wavetable_lookup(phase + mod_input) * amplitude * envelope
  4. Sum carrier outputs → softsat() → output buffer
```

**ADSR envelope**: Standard 4-stage. Auto-release is triggered at `duration - release_time` so sounds fade cleanly within their buffer.

**Pitch envelope**: `2^(amount * exp(-t/decay) / 12)` — exponential sweep in semitones. Positive values sweep down from above.

**Phase modulation**: Added to the sine argument directly, not to the phase accumulator. Preserves modulator's effect without permanently drifting carrier pitch.

**Wavetable**: Static 4096-sample sine table shared across all operators by default; the `fm_wave` knob can select band-limited saw / square / triangle tables (48 harmonics) instead. `fm_render` does its own inline phase-accumulator + linear-interpolation lookup (it does **not** call `wt_osc_tick_pm()`); the phase-mod value is added at the lookup index, matching "added to the sine argument directly" above.

### FM Bass (`fm-bass`)

- **Architecture**: 2-operator (carrier + modulator), both at 1:1 frequency ratio.
- **Base frequency**: 55 Hz (A1).
- **Modulation**: op1 → op0, depth 2.5. The 1:1 ratio with moderate depth creates a warm, slightly overdriven bass tone. As the modulator's envelope decays, the sound goes from bright to warm.
- **Carrier envelope**: 5ms attack, 300ms decay, sustain 0.6, release 200ms.
- **Modulator envelope**: 1ms attack, 150ms decay, sustain 0.2, release 100ms. Fast mod decay gives "pluck" character.
- **Pitch envelope**: +3 semitones, 60ms decay.
- **Buffer duration**: `render.seconds` 0.75 (it was 1.5 beats at 120 BPM; a note is a fixed time now).
- **Design notes**:
  - Darker/rounder: reduce mod depth (try 1.5) or shorten mod decay.
  - More aggressive: increase mod depth (try 4.0) or add mod sustain.
  - The 1:1 ratio keeps all sidebands harmonic.

### FM Bell (`fm-bell`)

- **Architecture**: 2-operator with inharmonic ratio (1:3.5) for metallic character.
- **Base frequency**: 440 Hz (A4).
- **Modulation**: op1 → op0, depth 3.0. The non-integer 3.5 ratio creates inharmonic sidebands — classic DX7 bell.
- **Carrier envelope**: 1ms attack, 1.5s decay, sustain 0.0, release 300ms. Pure ring-and-fade.
- **Modulator envelope**: 1ms attack, 1.2s decay, sustain 0.0, release 200ms. Slightly shorter than carrier so brightness fades before volume.
- **Pitch envelope**: None (0.0). Bells should not have pitch sweep.
- **Buffer duration**: `render.seconds` 1.0.
- **Design notes**:
  - More metallic: increase ratio (1:5.3, 1:7.1) or add `freq_offset`.
  - Warmer: decrease mod depth or shorten mod decay.
  - `freq_offset` adds fixed Hz to frequency (e.g., ratio 1.0 with offset +7.0 Hz creates beating).

### FM Lead (`fm-lead`)

- **Architecture**: 3-operator series chain (op2 → op1 → op0).
- **Base frequency**: 220 Hz (A3).
- **Modulation**: op2 → op1 (depth 2.0), op1 → op0 (depth 3.5). Cascaded modulation creates bright, harmonically dense sound.
- **Carrier envelope**: 3ms attack, 200ms decay, sustain 0.5, release 150ms.
- **op1 envelope**: 1ms attack, 120ms decay, sustain 0.3, release 100ms.
- **op2 envelope**: 1ms attack, 80ms decay, sustain 0.1, release 50ms. Fastest decay — "edge" fades quickly.
- **Frequency ratios**: op0=1x, op1=2x, op2=3x. All integer, brightness from cascaded depth.
- **Pitch envelope**: +1.5 semitones, 40ms decay.
- **Buffer duration**: `render.seconds` 0.5.
- **Design notes**:
  - Tame brightness: reduce op2 amplitude or decay.
  - More aggressive: increase depths or add self-modulation (mod_matrix[0][0] > 0).
  - Softer: switch to parallel topology instead of series.

### FM E-Piano (`fm-epiano`)

- **Architecture**: 3-operator with paired carriers. op1 modulates op0 (main tone); op2 is independent carrier at 2x for octave shimmer. DX7 Rhodes inspiration.
- **Base frequency**: 220 Hz (`fmEPianoSeed` `gen1_fm_base`).
- **Modulation**: op1 → op0, depth 2.2 (seed `gen1_fm_d2`). Rhodes character from tine decaying faster than tonewheel. op1 carries a +7 Hz `freq_offset` and op2 +0.5 Hz (slow beating).
- **op0 (carrier)**: 2ms attack, 800ms decay, sustain 0.3, release 400ms.
- **op1 (modulator)**: 1ms attack, 200ms decay, sustain 0.1, release 200ms.
- **op2 (shimmer carrier)**: 2x ratio, amplitude 0.30, 500ms decay.
- **Pitch envelope**: None.
- **Buffer duration**: `render.seconds` 1.0.
- **Design notes**:
  - Rhodes character from mod decay ratio: mod ~2-3x faster than carrier. Equal rates = organ.
  - More "tine-like": increase mod depth (try 2.5).
  - "Wurlitzer-like": increase mod sustain to 0.3-0.4.
  - Remove shimmer: set op2 amplitude to 0.

### FM Pluck (`fm-pluck`)

- **Architecture**: 2-operator with fast modulator decay — FM equivalent of Karplus-Strong.
- **Base frequency**: 196 Hz (G3).
- **Modulation**: op1 → op0, depth 4.0. High initial index creates bright transient that simplifies as mod decays.
- **Carrier envelope**: 1ms attack, 200ms decay, sustain 0.0, release 50ms.
- **Modulator envelope**: 0.5ms attack, 40ms decay, sustain 0.0, release 20ms. **Extremely fast decay** is critical.
- **Frequency ratio**: op1 at 2x. Keeps sidebands harmonic, 2x emphasizes odd harmonics.
- **Pitch envelope**: +2 semitones, 30ms decay.
- **Buffer duration**: `render.seconds` 0.25.
- **Design notes**:
  - More guitar-like: lower base freq (110-150 Hz), increase mod depth (5.0+).
  - More harp-like: higher base freq (300-500 Hz), lower mod depth (2.0).
  - Compare with the Karplus-Strong voice (`modular_gen_slot_ks`, used by harp/guitars) — FM pluck is brighter/synthetic, K-S is warmer/acoustic.

### FM Variant Instruments (Post-Processed)

Each base FM instrument has a post-processed variant named for what the post-processing does to the sound. C render is identical; only buffer duration and post-processing differ. The post-processing is declared on each variant's row in `modularInstrumentDefs`: **`DefaultFX`** insert-effect slots (HP/LP = filter modes, "soft clip" = `EffectDistortion`, "crush" = bitcrusher) plus a **`GateTail`** fraction for the two gate-tail variants, which `modularTableVoices` turns into a `gateTail(buf, frac)` post closure (`variants.go`) — not freeform per-sample code.

| Variant | Base | Duration | Post-Processing | Character |
|---------|------|----------|-----------------|-----------|
| `fm-bass-drive` | fm-bass | 0.5 s | HP 60 Hz + soft clip 1.8x | Tighter, more aggressive, cuts sub rumble |
| `fm-bell-soft` | fm-bell | 0.75 s | LP 4000 Hz | Darker, muted bell |
| `fm-lead-crush` | fm-lead | 0.35 s | 5-bit crush + gate tail 60% | Lo-fi, digital, retro |
| `fm-epiano-drive` | fm-epiano | 0.75 s | LP 3000 Hz + soft clip 1.4x | Warmer, vintage |
| `fm-pluck-tight` | fm-pluck | 0.15 s | HP 150 Hz + gate tail 50% | Tighter, thinner, percussive |

### Creating New FM Presets

A new FM **instrument** (a new sound on an existing preset) is just a seed + a row —
`fmSeedWith({...})` with the curated knobs (`gen1_fm_base`, per-op `gen1_fm_r*` /
`_d*` / `_dec*`, pitch envelope, `gen1_wave`) and a `modularInstrumentDefs` row (see §
Adding a new built-in instrument). A new FM **preset topology** (different routing,
operator count or fixed op amplitudes) is a new `variant` case in `modular_gen_slot_fm`
(`src/c/modular_stages.c`) that bakes its constants and calls the shared `fm_render`
core (still in `fmsynth.c`). The old path (static `fm_preset` tables, `render_fm_xxx`
wrappers, `fmsynth_c.go`, hand-edited `RENDER` entries) was removed in the Phase-7 FM
migration.

### Common FM Recipes

| Sound | Ops | Ratio | Mod Depth | Key Trick |
|-------|-----|-------|-----------|-----------|
| Warm bass | 2 | 1:1 | 2-3 | Fast mod decay for pluck attack |
| Metallic bell | 2 | 1:3.5 | 2-4 | Non-integer ratio, long decay |
| Bright brass | 3 | 1:1:1 (series) | 3-5 | High depth, moderate sustain |
| Marimba | 2 | 1:4.0 | 1-2 | Low depth, fast decay, no pitch env |
| Organ | 2+ | 1:2:3 (parallel carriers) | 0.5-1 | Low depth, high sustain |
| Gong | 4 | inharmonic mix | 2-4 | Multiple non-integer ratios, long decay |
| Clav | 2 | 1:3 | 3-5 | Very fast mod decay (~20ms), short carrier |

---

## Organic Tuned/Melodic Instrument Synthesis (Strings, Brass, Bass, Hand Drums)

Reference for making **pitched/melodic** instruments (violin, cello, trumpet, synth bass, conga) sound *organic* rather than like a buzzy distorted-organ. The gen-bank engine (`modular_gen_bank_render` in `src/c/modular_stages.c`, driven from `render_modular_p` in `src/c/modular.c`; params in `src/c/modular.h`) is the substrate. Param shorthand below: `genK_*` ⇒ the real `gen_*[K]` array column (slot K, 0–11). Map:

| Shorthand | Real field | Notes |
|---|---|---|
| `genK_source` | `gen_source[K]` | 0=off 1=osc 2=noise-tap **3=Karplus-Strong 4=analytic-voice** |
| `genK_wave` | `gen_wave[K]` | 0=sine 1=saw 2=square 3=triangle |
| `genK_freq` / `genK_freq_mode` | `gen_freq[K]` / `gen_freq_mode[K]` | mode 0 = ratio×voice-freq, **mode 1 = absolute Hz** (formants!) |
| `genK_filt_type` | `gen_filt_type[K]` | 0=none 1=1pLP 2=1pHP 3=rbjLP 4=rbjHP **5=rbjBP** |
| `genK_filt_freq` / `genK_filt_q` | `gen_filt_freq[K]` / `gen_filt_q[K]` | RBJ Hz / Q |
| `genK_gain` | `gen_gain[K]` | per-slot mix |
| `genK_env_fast_*` / `genK_env_tail_*` | `gen_env_{fast,tail}_{rate,mix}[K]` | dual-rate per-slot envelope |
| `lfo_target` | `lfo_target` | 0=tremolo **1=pitch vibrato** 2=filter sweep |
| `filtenv_amt` / `filtenv_decay` | `filtenv_amt` / `filtenv_decay` | `cutoff *= 2^(amt·e^(-t/decay))` — brightens on attack |
| `filtenv_attack` | `filtenv_attack` | seconds for the cutoff to **RISE** 0→amt octaves before the decay/hold. **0 = legacy onset-bloom (byte-identical); >0 = a slow brightening SWELL** — the brass/wind "spectral envelope tracks loudness" crescendo (set `filtenv_decay` large to HOLD bright after the rise). Added 2026-06 for the trumpet. |
| `bow_dynamics` | `bow_dynamics` | **Bowing-expression** modulator (0=off, byte-identical). A slow smoothed random walk (≈250 ms gestures) drives LOUDNESS and BRIGHTNESS together — keep it GENTLE: 0.25 on the violin was heard as "the sound waves in volume or intensity in a way that the reference does not" and 0.5 on the cello as "vibrating too mechanically" (2026-11-07); ≤ ~0.1 on a solo bowed voice (more pressure → louder + brighter, via a one-pole LP whose cutoff tracks it). This is the organic, non-repeating PERFORMANCE motion a steady synth tone lacks — the "it sounds like an organ" fix. Added 2026-06 for the violin; reusable for any bowed/expressive sustain. |
| `body_model` / `body_mix` | `body_model` / `body_mix` | **Body-resonator BANK** (organic string body). `body_model` 0=off (byte-identical), 1=violin (22 moderate-Q (6–11) signature modes incl. the ~2.6 kHz bridge hill, hardcoded in `modular.c`), 2=classical/nylon guitar, 3=cello, 4=steel guitar. `body_mix` blends the body-shaped signal in (and suppresses the dry as it rises, so the body SHAPES the tone rather than just adding peaks). The bowed saw driven through it gets the violin's woody body coloration; vibrato sweeps harmonics across the sharp modes. Added 2026-06 for the violin. |

### Why a naive recipe sounds synthetic

A single saw through one static filter has: (1) a **static spectrum** (never evolves), (2) **no body/formant resonance**, (3) **no micro-decorrelation**. Real instruments need all three:

- **Time-varying spectrum** — brightness evolves with time *and* dynamics (loud = brighter).
- **Fixed body formants** — resonant peaks at *absolute* frequencies that do **not** track pitch (the instrument's box/bore is a constant filter).
- **Ensemble/vibrato decorrelation** — multiple near-identical voices slightly out of phase/tune.

> Matching steady-state harmonic **ratios** is necessary but **NOT sufficient**. Ratios alone = organ.

### Principles, ranked by payoff-per-effort → engine mechanism

1. **Unison/ensemble detune** *(ENSEMBLES only — never a solo voice)* — 3–7 osc slots detuned ±4–12 cents. Two voices c cents apart beat every harmonic n at `n·f0·(2^(c/1200)−1)` Hz: the shipped tuba (2 voices, 5 ¢, A2) beat H4 at ~1.3 Hz and H8 at ~2.5 Hz, a 4–7 dB wobble on its strong harmonics against the recording's ~2 dB, which the listener heard as "audible waviness the reference does not have" (2026-11-07). A whole-band AM measure misses it — the partials beat at different rates and average out (`mod.am.depth` read the tuba 5× LESS modulated than its recording); measure each harmonic's level over time. Cents→ratio: `r = 2^(c/1200)` (+5¢=1.00289, −7¢=0.99596). Set each `genK_freq` (ratio mode) to the detuned ratio.
2. **Formant / body-resonance band-pass banks** — several **parallel** band-pass slots summed: `genK_source:1`(saw) → `genK_filt_type:5`(rbjBP) at `genK_filt_freq=<formant Hz, ABSOLUTE>` (`genK_freq_mode:1` or fixed Hz), `genK_filt_q`, `genK_gain`. A saw band-passed at a fixed formant boosts whichever harmonic lands near it → real body character. **Combine with ensemble** by detuning each formant slot's source `genK_freq`.
3. **Per-partial envelopes** — additive sine slots, each its own dual-rate envelope (`genK_env_fast_*`/`genK_env_tail_*`); higher partials decay faster.
4. **Brightness-tracks-loudness** — `filtenv_amt`/`filtenv_decay` (cutoff blooms on attack, settles) + velocity→cutoff. **The #1 brass fix.**
5. **Micro-modulation / vibrato** — `lfo_target:1`, 5–8 Hz, ±10–30 cents. Should **delay onset** after attack — use `lfo_delay` (Phase-8G; ramps LFO depth 0→full over `lfo_delay` seconds; applied at all three LFO targets in `modular.c`). *(Formerly a known engine GAP — now implemented.)*
6. **PADsynth render-time spreading** *(future engine addition — IDEAL for our pre-rendered buffers)* — Paul Nasca / ZynAddSubFX. Generate a loopable wavetable where each harmonic is a **Gaussian band** of bins with **randomized phase** (decorrelation) → infinite-ensemble lushness, computed once, looped at playback. Profile per harmonic `i`: `profile(fi) = exp(-(fi/bwi)^2)/bwi`. `N=2^18`; `bw_cents` 10–30 = clean, 60+ = lush; `bwscale=1.0`. A FUTURE **PADsynth wavetable source** for the gen-bank. Pseudocode:
   ```
   freq_amp[N/2] = 0
   for each harmonic h (amp a_h, center f_h):
       bw_Hz   = (2^(bw_cents/1200) - 1) * f_h * bwscale
       bw_bins = bw_Hz / (sr / N)
       for bin i near h:
           x = (i - f_h*N/sr) / bw_bins
           freq_amp[i] += a_h * exp(-x*x) / bw_bins
   for each bin: freq[i] = freq_amp[i] * e^(j·2π·rand())   # randomized phase
   wavetable = real(IFFT(freq)); normalize; loop at playback
   ```
7. **Modal percussion (conga/hand drums)** — short impulse/tone into **high-Q band-pass modes**, each mode its own decay. Ideal membrane modes are **inharmonic** (Bessel ratios 1 : 1.59 : 2.14 : 2.30 : 2.65 …), but **air-loading stretches the low modes toward ~harmonic** (1 : 1.5 : 2 : 2.5 : 3) — that stretch is what makes a drum sound *tuned*.
8. **Karplus-Strong** (`genK_source:3`, `gen_ks_sustain`/`gen_ks_pluck`) for realistic **plucked** strings. (`gen_source` values: 0=off 1=osc 2=noise-tap 3=KS 4=analytic 5=kick 6=tom 7=snare 8=clap 9=cymbal 10=FM.) Full self-sustained **bowed/brass physical models** (STK Bowed/Brass) are **OUT of reach** (need nonlinear feedback loops). Approximate bowed strings with **formant-bank + ensemble + bow-noise** → ~90% there.

### Per-instrument recipe cards

> **⚠️ PHYSICAL-MODEL UPDATE (2026-06).** Several instruments moved from the additive/subtractive cards below to self-oscillating **digital-waveguide** physical models (see [`AGENTS.synth.md`](AGENTS.synth.md) § Digital-Waveguide Physical Models). Current state:
> - **VIOLIN, CELLO** → **bowed-string waveguide** (`osc_type:7`) through their body banks. The cards below describe the older saw+body approach (still valid context for the body modes); the *source* is now the waveguide. User: "amazing".
> - **OBOE** → *(superseded — the shipped `oboeSeed` is now ADDITIVE: osc off, sine gens 2–8 + a band-passed reed-noise tap; its own comment records that the reed waveguide read as a CLARINET)*. Historical: **reed waveguide** (`osc_type:9`, STK-clarinet config: inverting bell, offset 0.7, slope −0.3, **breath 0.8 to cross the oscillation threshold**).
> - **TRUMPET** → **ADDITIVE brass formant** (NOT the saw card below). The real ref is H3-dominant (H2=1.88, H3=2.33 peak, H4=1.14) — place those as gen sines (boost ~3.5× for slot attenuation), keep the crescendo `amp_attack` + `filtenv_attack` bloom + noisy attack + NO vibrato. Same additive approach fixed the FRENCH HORN (its real ref is **H2-dominant ≈2.08** — the "covered" tone; the seed's old "weak H2" ladder was simply measured wrong → **re-measure the REAL reference, don't trust a seed's claimed spectrum**).
> - **FLUTE** → *(superseded — the shipped `fluteSeed` is now ADDITIVE: sine gens 2/3/4 at H1 0.55 / H2 0.15 / H3 0.05 + a gen1 noise-tap band-passed at 2.1 kHz, `filter_cutoff` 2900, a delayed pitch vibrato `lfo_target 1` rate 5.6 depth 0.05, `amp_attack` 0.04; no `ks_*`)*. Historical: KS blown-tube (`gen_source:3`). The "organ" trap: too clean + static. Fixes = **AIR** (~14% breath noise, not 0%), a gentle **PITCH vibrato** (the air-jet wavers — movement breaks the static organ), and a fairly **pure** harmonic stack. (The dedicated air-jet waveguide `osc_type:10` mistunes ~1.47× → not wired.)
> - **SAX** (baritone) → **HYBRID ADDITIVE + FORMANT-FILTERED SAW** (2026-07 retune, `saxSeed`). ⚠️ **Every earlier sax analysis was an OCTAVE-UP artifact**: the mtg `..._c3.wav` ref MEASURES **f0 = 122.8 Hz (≈B2)** by raw FFT peak scan (NOT C2/65 Hz, NOT 130.8 Hz); its fundamental sits ~7 dB below H2, so `DetectF0` locked onto H2 (245.6 Hz) and every "measured profile" (the H3-dominant ladder, the "breath ≈ 0.45") was computed on the doubled grid — the odd harmonics were being counted as NOISE. Fixed in `fingerprint.DetectF0` (odd-harmonic subharmonic evidence check, `TestDetectF0_RichWeakFundamental*`). TRUE bari signature (rel H2=1.0): H1 .47, **H2 1.00 dominant, H3 .97**, H4 .68, H5 .39, **H6 .67 (≈740 Hz knee-formant bump)**, H7 .37, H8 .21, then a REBOUND plateau H14-H16 ≈ .3 (formants at 1.9 kHz −2.4 dB and 2.6 kHz −4.2 dB, cliff above); even/odd 1.25 (conical bore = full even+odd series); noise only ~0.08; vibrato ~2¢ (nearly straight); rise10-90 ≈ 0.22 s; brightness↔loudness coupling +0.77 (from the slow decrescendo). Seed architecture: sines pin H1-H4+H6; **saw→BP@820 Q1.8** fills the knee (H5, H7-H10); **saw→BP@2050 Q2.0** builds the high plateau; narrow breath noise at ~1.2 kHz (louder breath = brighter → positive coupling); `amp_attack 0.5` + slow `amp_decay 2.5 / sustain 0.45` decrescendo; `lfo_depth 0.002` (0.008 rendered 8.5¢ vs the ref's 2.4¢). Matched to within a few % on every fixed-grid metric (`TestSaxTune` / `TestSaxProbe` in cmd/synth-match); **user ear-approved ("sounds great", 2026-07-05)**. The SAXOFONY waveguide (`osc_type:11` `render_sax`, config knobs `osc_sax_*`) remains available but was rejected: chaotically sensitive, and it was being tuned against the octave-wrong profile.
> - **PIANO** (grand + felt) → CLEAN additive struck-string: sine fundamental + a few decaying upper partials (×2/×3/×4, upper ones decaying faster via `gen_env_fast_rate`) + a slight `osc_detune` (the piano's mistuned unison strings = warmth) + an EXPONENTIAL amp decay (`amp_curve:1`) for the struck "ping→tail" (current seeds: `amp_decay` 0.9, `filter_cutoff` 6000 grand / 4000 felt). Two fixes the user asked for: (1) **remove the hammer-NOISE gen** (`gen_source:2`) — it read as background hiss *(since re-added as a SHORT hammer transient: grand gen6 band-passed 2.6 kHz gain 0.22 fast_rate 45, felt gen5 at 1.8 kHz)* ("not a pure piano note"); (2) **remove the phantom SUB-OCTAVE gen** (`gen_freq:0.5`) — a real piano has NO octave below the played note, and the ref (pinkyfinger piano-g is **G5≈790 Hz**, NOT a low G) made DetectF0 lock an octave down AND darkened the tone. Grand = brighter; felt = duller filter + softer strike. Both used by templates (keep both ids). ⚠️ **HARPY vs PIANO** (user: "very harpy/bright, piano keys sound different"): chasing the reference's time-averaged CENTROID made the SUSTAINED tone too bright = harp (a plucked string rings bright the whole note). A piano = a FELT hammer: a bright ATTACK that DECAYS FAST into a WARM body. Fix = give the upper partials FAST decay (`gen_env_fast_rate` ~4/6/8, so they punch then fade) + soft strike (`amp_attack`~0.006) + warmer filter (~3200) — verify the centroid EVOLVES (attack ~1480 → body ~880), don't match the static centroid. Trust the ear over the metric.
> - **HARP** (celtic) → Karplus-Strong plucked string, like the guitars but more OPEN: a brighter clear pluck (`ks_pluck` ~0.25), a much longer free ring (`ks_sustain` ~0.993 + long `amp_decay` — not long enough: at 0.997 / `amp_decay` 4 the voice was 11 dB under `harp-freesound-alt` by 1 s and the listener asked for a longer ring; 0.9985 / 8 follows the recording), a light soundboard (`body_model:2`, low `body_mix`). ⚠️ On a LOW reference note (G2≈98 Hz) the `NoiseRatio` metric reads bogus-high (~0.64) because F0 detects an octave up and half the harmonics fall off-grid → counted as "noise"; verify the patch is actually clean at a HIGH note (C5) where detection is reliable (it measured 0.02). Match the CENTROID, not the low-note noise number.
> - **GUITAR — NYLON vs STEEL**: same Karplus-Strong string + same body box, distinguished by **body-mode Q** and pluck brightness. NYLON (`body_model:2`) = low-Q broad upper modes + soft dark pluck (`ks_pluck` ~0.09) = warm — but a dark `ks_pluck` is not a weak ATTACK: tuned down to 0.018 with the cutoff at 1.8 kHz it was heard as "the string pluck sound needs to be stronger"; the recording carries ~3× the onset noise. Strengthen the pluck with a filter-envelope bloom and a short band-passed noise burst, which leave the ringing tone alone. STEEL (`body_model:4`) = **high-Q ringing upper modes** + bright sharp pluck (`ks_pluck` ~0.6) + long bright sustain = metallic zing. KEY LESSON: a static centroid match is NOT enough — **metallic = high harmonics that RING** (high-Q upper resonances + long HF sustain); nylon = bright attack, fast-decaying highs. The user heard a centroid-matched nylon as "metallic steel" — proof the number can be right while the character is wrong.

Formant frequencies are **ABSOLUTE Hz** (do NOT track pitch). Q ≈ 2–8 unless noted; gains are relative.

**VIOLIN** — saw ensemble + body-mode BP bank + bow noise + vibrato.

| Element | Settings |
|---|---|
| Ensemble | 3 saws, `genK_freq` ratio 1.0 / +5¢(1.00289) / −7¢(0.99596) |
| Body-mode BP bank (absolute Hz) | A0 **272**, CBR **407**, B1− **462**, B1+ **551**; bridge "hill" ~**2300** (broad, low-Q) |
| Vibrato | `lfo_target:1`, **6 Hz**, ±**15¢**, delayed onset |
| Bow noise | noise→rbjBP **1–4 kHz**, **−20…−40 dB**, hotter at attack (couple to attack env) |
| Envelope | slow-ish attack (~30–80 ms), continuous sustain |

**CELLO** — same shape, lower & darker.

| Element | Settings |
|---|---|
| Body modes (absolute Hz) | air ~**110**, corpus ~**190**/**220**, body ~**550**, bridge ~**1500**/**2200** |
| Ensemble | ratio 1.0 / +4¢ / −6¢ |
| Vibrato | `lfo_target:1`, ~**5 Hz**, ±~12–15¢ |

**TRUMPET** — saw + brass formant + **brightness-blooms-on-attack** (the headline brass move).

| Element | Settings |
|---|---|
| Core | saw, `filtenv_amt` high + short `filtenv_decay` so cutoff **blooms then settles** (#1 brass fix) |
| Brass formant | rbjBP ~**1–1.5 kHz**, **Q 2–3** |
| Growl | ~**80 Hz** filter-growl, AD ~**50 ms** |
| Breath noise | broadband, gated to attack |
| Vibrato | `lfo_target:1`, ~**5 Hz**, **delayed** onset |

**SYNTH BASS** — saw + sub, swept resonant LP = the "wow"/pluck.

| Element | Settings |
|---|---|
| Core | saw + sine **sub one octave down** — but **don't overdo sub**; **cut 200–400 Hz mud** (rbjBP notch or trim) |
| Filter | **24 dB/oct LP** (`filt_type` rbjLP, ideally cascaded) with **resonance swept by a decay env** (`filtenv_amt`/`filtenv_decay`) = the pluck "wow" |
| Envelope | **zero sustain** = plucked; **high sustain** = sustained bass |
| Drive | light `gen_sat_k` / post drive |

**CONGA** — tuned membrane (modal) + slap noise. Two-slot family (open tone + slap).

| Element | Settings |
|---|---|
| Fundamental | sine/triangle, mid-conga ~**200 Hz** (quinto ~**250–400**, tumba ~**120–180**) |
| Partials | ~**1.5×** and **2×** fundamental (air-stretched modes) |
| Pitch drop | small **attack pitch-drop** tracking amp decay (`gen_pitch_env_amt`/`_rate`) |
| Open-tone decay | **D ~120–180 ms** |
| Slap slot | separate noise slot, **HP/BP 2–8 kHz**, **D ~2–10 ms** |
| Articulations | **slap** = short tone + bright crack; **muff** = very short tone, no ring |

The shipped `conga` is tuned to a **quinto MUTED SLAP** (ref mrrentapercussionist): slap-noise gen (BP ~2.7 kHz) dominant over a brief membrane under-thump (gen1–3 modes), `amp_decay` ~0.05 (dies ~50 ms, muted), centroid ~1900. ⚠️ **Measuring a percussion HIT**: `synth-analyze`'s centroid/noise read ~0 on a <50 ms transient — its steady-window misses it. FFT the **onset window directly** (first ~50 ms from `argmax(|a|)`): compute centroid + **spectral flatness** (geo-mean/arith-mean of the PSD — high = noisy slap, low = tonal). The ref's full-file centroid (3874) was misleading; the onset-window centroid is 1884.

### Engine-mapping cheat sheet

| Technique | gen-bank mechanism |
|---|---|
| Ensemble detune | N osc slots, `genK_freq` = detuned ratios (±cents) |
| Body formant | rbjBP slot, `genK_filt_freq` = **absolute Hz**, summed parallel |
| Per-partial decay | per-slot `genK_env_fast_*`/`genK_env_tail_*` |
| Brightness-on-attack | `filtenv_amt` + `filtenv_decay` |
| Vibrato | `lfo_target:1` + `lfo_delay` (onset-delay ramp, Phase-8G) |
| Plucked string | `genK_source:3` (Karplus-Strong) |
| Modal percussion | high-Q rbjBP modes, each own decay |
| Bow/breath noise | noise-tap slot → BP, env-coupled to attack |

> **Formant freqs are ABSOLUTE Hz — they do NOT track pitch.** This requires the **per-pitch re-render** (already implemented desktop; WASM via pitch-variant pre-render) to behave correctly across the playable range. A formant authored at 462 Hz must stay at 462 Hz whether the note is C2 or C5.

### Analysis-driven tuning against real recordings (`synth-match`)

The organic seeds for the orchestral instruments were **calibrated against real isolated recordings** with the `synth-match` auto-optimizer, not authored by ear alone.

**Workflow.** `make synth-match INST=<id> REF=<ABSOLUTE path to isolated single-note WAV> [DRY_RUN=1]` runs a Hooke-Jeeves search over the instrument's tunable seed params, minimizing a multi-band fingerprint distance (spectral / MFCC / partials / temporal / envelope / noise / timbre) against the reference, then patches the instrument's document `internal/assets/instruments/<id>.json` through `instrument.SetField` (`synthmatch.PatchInstrumentDocFile`; run `make asset-pack` after; `DRY_RUN=1` previews without patching). The `-ref` path **must be absolute** — the make target `cd`s into `src/go`. The tunable param set per instrument family lives in `internal/audio/synthmatch/tunable.go`.

> ⚠️ **The distance `synth-match` minimises is the one the 2026-10-17 matcher
> rewrite measured as wrong** (`fingerprint.Distance` over `FromWave`,
> `synthmatch/optimize.go`): no attack, f0 re-detected per clip, an absolute
> MFCC floor that lets room tone decide, and the render pitch taken from a bare
> `DetectF0` with no sub-octave/sub-twelfth guard. It is consistent with the
> winds and brass below "improving" by 16–43 % while the listener's notes still
> call the flute and oboe breathy/airy and the trumpet "close but needs
> refinement".
> Read its % gains as "closer on that fingerprint", never as "more like the
> instrument" — judge identity with the essence distance (§
> Tuning by measurement → "Is it still THAT instrument?") and the ear. For an
> essence-driven candidate use `make tune INST=<id>` (`refs tune`) instead.

**Best reference source = University of Iowa MIS** (`theremin.music.uiowa.edu`, "MIS Pitches - 2014") — clean isolated single sustained notes (the ideal monophonic fingerprint). Decode AIFF → mono-44.1k-PCM with ffmpeg. Confirmed-available folders: `Strings/Violin`, `Strings/Cello`, `Woodwinds/Flute`, `Woodwinds/Oboe`, `Woodwinds/Eb Alto Saxophone/AltoSax.NoVib.ff.A4`, `Brass/Trumpet`, `Brass/Horn/Horn.ff.A3`. MIS has **no** guitar, clavinet, conga, electric, or synth-bass — those modern instruments have **no clean public-domain isolated reference** (archive.org keyword search returns mixed full tracks, e.g. a podcast matched "clavinet"), so they cannot be reliably auto-tuned and must be authored/ear-tuned.

**Metrics must be validated against the ear before they are trusted.** `cd src/go && ../../.tools/go/bin/go run ./cmd/refs validate` joins every registered descriptor to the listener verdicts in `internal/audio/testdata/ear_verdicts.jsonl` and reports, per axis, the AUC of "does a bad-sounding instrument score worse than a good-sounding one" with a 95% CI. As of 2026-09-01 only **inharmonicity** (AUC 0.96, CI 0.81-1.00) and **crest median** (0.79, 0.52-1.00) clear chance; **ring T60 is confidently ANTI-correlated** (0.21, CI 0.00-0.47) because every reference is truncated by its own `convert.max_sec`, so both sides clamp to the clip length and the axis reports "fine" on instruments the listener rejected outright. Tuning an instrument to improve `ring T60` made it worse, and a previous tubular-bells rebuild did exactly that while declaring success in its seed comment. Prefer the shallow `decay T10/T20/T30` and `energy sections` axes, which measure inside the clip instead of clamping. Re-run `validate` after adding a descriptor: an axis that fires on >=85% of instruments (`SATURATED`) is a constant, not a measurement.

**Provenance is now catalogue-driven, not script-driven.** `scripts/fetch_synth_refs.sh` and the hand-maintained `testdata/refs/MANIFEST.md` are retired. Every reference WAV's source URL, licence, credit, and (once fetched) its PCM hash and audio-derived measurements live in `internal/wavref/catalogue.json` — the single source of truth, queried via `cd src/go && ../../.tools/go/bin/go run ./cmd/refs list` / `doctor` / `verify`. `make refs-doctor` reports per-instrument coverage (`covered` / `synthetic` / `needs-source` / `unclassified`) and per-template song-reference coverage in one pass; `make fetch-refs [TIER=instrument|song] [ID=<ref-id>] [INSTRUMENT=<id>]` fetches and converts. Crucially, each ref's `measured.semis_from_a3` is **detected from the decoded audio** (via `fingerprint.DetectF0`), never parsed from a filename — upstream filenames and metadata pitch labels have been wrong before. `synth-match` (and any tool picking a render pitch for a reference) should read `measured.semis_from_a3` from the catalogue and render the instrument at that pitch, rather than inferring pitch from the WAV's filename.

**Confidence ranking — instruments calibrated to a real MIS sample** (fingerprint improvement vs the prior seed; re-running synth-match against the same MIS ref reproduces these). Treat these seeds as the *reference baseline* — they are the closest match to the true signal:

| Instrument | Improvement | Notes |
|---|---|---|
| sax | **+42.7%** | closest match of the set |
| violin | **+31.5%** | confirms the additive-partials + body-mode-BP + bow-noise shape above |
| oboe | **+31.2%** | |
| cello | **+25.0%** | |
| flute | **+19.9%** | + the manual fixes below |
| trumpet | **+17.4%** | confirms the filter-env-bloom-on-attack shape above |
| french-horn | **+16.2%** | |

*(piano-grand, piano-felt, organ, and guitar-nylon were tuned only via the `song-compare` mix-axis against **polyphonic** recordings — directional band hints, range/key-confounded — so they are lower-confidence and **not** a calibrated baseline.)*

**Two durable lessons:**
1. **synth-match optimizes a SUSTAINED note**, so its tuned envelope (especially `amp_attack`) can smear fast melodic passages. For a fast template, override with a faster attack — the flute Allemande needed `amp_attack` 0.065 (synth-match's value) → **0.012**.
2. **FLUTE specifics** (osc disabled → the gens are the only source): keep `gen1`/`gen2` near-equal (real flute P1≈P2) and at an *audible* level (≈0.30/0.25, **not** 0.12/0.10), and a clean vibrato is `lfo_depth` ≈**0.05** — 0.14 is ≈±2 semitones of warble.

**mix-axis caveat:** `make song-compare TEMPLATE=<t> REF=<polyphonic recording>` reports per-band boost/cut hints (sub/low/mid/high/air). The direction is useful (organ → +sub +air, −mid) but the metric is range/key-confounded and bounces — don't chase it past the first clear improvement. Physical limit: low organ notes can't reach the air band (6 kHz+) via drawbars (the 1⅓' of A3 is only ~1.3 kHz).

### Ear-confirmed verdicts (2026-06-22) — the ultimate signal

Listening tests override every numeric score. What we actually learned:

- ✅ **Organ — excellent.** The mix-axis scoop (added 32' sub `gen8`, boosted 16', cut the 8'/4' mids, opened the filter to 9000) landed. Mix-axis tuning *can* work very well when the band hints point somewhere physical.
- ✅ **Steel guitar — excellent**, and it was **never auto-tuned** — `guitarSteelSeed` is the original Karplus-Strong genre-masterpiece seed. Don't "improve" it. (Plucked KS strings are the engine's strongest organic voice.)
- ⚠️ **Grand & Felt piano — pleasant but (a) lo-fi/noisy and (b) nearly indistinguishable from each other.** The felt variant must diverge audibly from grand (softer/darker/less harmonic), and both carry too much hiss — the noise-burst gen (`gen5`) and/or the additive-partial harshness needs cleanup. Open item.
- ❌ **Winds & brass (flute, oboe, sax, trumpet, french-horn) — sound bad by ear, despite +17…+42% synth-match fingerprint gains.** **This is the headline lesson: a lower multi-band fingerprint distance against one sustained MIS note does NOT mean a wind instrument sounds real.** The additive-partials-at-static-ratios + simple-LFO-vibrato recipe is fundamentally too static/buzzy for winds — they need the breath/turbulence, attack chiff, formant body, and time-varying spectral envelope that the fingerprint metric barely weights. Improving winds requires *recipe* work (see "Wind & brass synthesis" below), not more synth-match passes.

### Wind & brass synthesis (research-backed) — making the blown instruments sound real

> *(History — the shipped flute has since moved to an additive seed; see the FLUTE bullet above. The blown-KS engine hook below still exists.)*
> ✅ **FLUTE — ear-confirmed good (the blown-waveguide approach works).** The flute uses the blown-waveguide path (below): pitched at all registers incl. high notes, breathes naturally (sustain ripple ~5–6.5 dB ≈ real 6.6), smooth (no "organ"/"industrial-noise"/"radio-hiss"). Treat `fluteSeed`'s **structure** (blown KS + 2-pole breath + gentle tanh limit cycle) as the validated recipe for blown winds. Current `fluteSeed`: `ks_blow 0.18, ks_sustain 0.982, ks_pluck 0.6, filter_cutoff 2400, lfo_depth 0.25 (tremolo breath)`. oboe/sax/trumpet/horn are still additive WIP — port them to this blown path next.
>
> **De-hiss notes (the "old-recording / radio" floor):** the >1 loop gain amplifies broadband content up to the tanh ceiling, so a faint inter-harmonic floor is intrinsic — lowering `gen_ks_blow` (breath *level*) does NOT remove it (the loop re-amplifies whatever's there). What works: (1) the **2-pole lowpass on the breath excitation** (soft air, not white hiss — in `modular_gen_slot_ks`); (2) a **lower post-loop `filter_cutoff`** to cut the 3–8 kHz "radio" band, since the fundamental-heavy tone sits below it; (3) the **slow tremolo LFO (`lfo_target:0`) restores the breathing** the low cutoff would otherwise dull — a slow amplitude wobble, independent of the hiss; (4) **a pitch-relative high-pass at 0.65×f₀ on the blown output** (`modular_gen_slot_ks`, 2-pole, `hpR = exp(-2π·0.65·f0/sr)`) — a tube has no musical content below its fundamental, so this kills the **low-frequency rumble** floor (the bulk of the audible "background noise") at every pitch while leaving the treble tone intact. **`gen_ks_blow` level mainly trades amplitude STEADINESS (less breath = steadier, lower sustain-ripple), not hiss** — low breath = steady (horn), high breath = breathy (flute).

*(Sources: Sound on Sound* Synth Secrets *"Synthesizing Wind Instruments" & "Practical Flute Synthesis" (Gordon Reid); CCRMA waveguide/physical-modelling notes (J.O. Smith); STK (Cook/Scavone) flute model; ZynAddSubFX **PADsynth** algorithm (Paul Nasca); UNSW flute acoustics; aeroacoustics jet-edge literature. Full physical modelling — waveguide bore + nonlinear jet/reed/lip excitation — is the most realistic but needs nonlinear feedback loops the gen-bank engine does NOT have; the cues below are what a subtractive/additive engine CAN deliver.)*

**Deep dive — what makes additive winds "lifeless" and what the literature says to do:**
- **Phase-locked partials = "artificial/ugly" (PADsynth docs, verbatim).** Our gens are *perfect* phase-locked sines → sterile, even with amplitude tremolo + breath noise. The single biggest missing organic element is **per-partial decorrelation**: each real harmonic is a narrow *band* of slightly-detuned, randomly-phased components that beat against each other.
  - **Proper fix = PADsynth** (Nasca): build a looped wavetable where each harmonic `n` is a Gaussian band of bins, **bandwidth grows with `n`** (`BWₙ = n·BW₁`), **random phase per bin**, amplitude/bandwidth-normalised. "If you don't increase bandwidth with frequency it sounds artificial." Computed once, looped — ideal for our pre-rendered buffers. A future **PADsynth gen source** is the highest-fidelity path.
  - **Cheap engine-existing approximation = unison + drift.** The gen-bank already applies `unison_voices`/`unison_detune`/`unison_mix`/`unison_drift_rate`/`unison_drift_depth` to each `src:1` osc gen → each partial becomes a small detuned, slowly-drifting cluster (poor-man's PADsynth). The strings already lean on this; solo winds should use it **subtly** (2–3 voices, ~4–6 ¢) — this is the first thing to try.
- **Attack edge-tone chiff:** a real flute's jet briefly oscillates at a HIGHER mode (octave/12th) before the fundamental saturates (~30–80 ms). That startup "chiff" is a key identity cue — synthesize as a brief octave-up burst / pitch-settle at onset, or a short bright noise transient.
- **STK flute** (what we *can't* fully copy): jet delay-line + bore delay-line + **cubic jet nonlinearity** (x−x³) + breath pressure (DC + noise + vibrato) in a feedback loop. The nonlinearity + feedback are why it never repeats exactly. We approximate the *symptoms* (decorrelation, breath, breathing, chiff), not the mechanism.
- **Per-note variation:** PADsynth starts each note at a random wavetable phase so no two notes are identical (kills the machine-gun/static feel). Our voice-cache variant seeding is reserved infra for this.

**Physical-modeling path (Surge XT / RipplerX / STK) — the most promising, and PARTLY BUILT:**
Additive synthesis (even decorrelated) still reads as a high-pitched organ, *especially on high notes*. The open-source wind synths don't use additive at all — they use **waveguides**: a flute is a TUBE (air column = delay line + feedback filter) excited by **continuous breath noise**.
- **Surge XT String osc** = two filtered Karplus-Strong waveguides with **Burst** (pluck) vs **Constant** (bowed/blown — keep injecting the exciter while held) modes. **RipplerX** = modal resonators incl. "open tube"/"closed tube" + a **noise exciter** with resonant filter + ADSR. **STK flute** = jet delay + bore delay + **cubic jet nonlinearity (x−x³)** + breath(DC+noise+vibrato) in a feedback loop. Common thread: **the noise IS the excitation, blown through a resonant tube** — which is why our KS *steel guitar* sounds organic and the additive flute doesn't.
- **ENGINE HOOK ADDED (golden-safe):** `gen_ks_blow` param on the KS gen source (`modular.h` / `modular_c.go` / schema / `modular_stages.c`). `0` = plucked (legacy, byte-identical → steel guitar golden unaffected); `>0` = **blown tube** — continuous filtered-noise excitation into the tuned delay line (`modular_gen_slot_ks`, the `if (bBlow > 0)` branch). `gen_ks_sustain` = bore feedback, `gen_ks_pluck` = breath-noise color.
- **WORKING (self-limiting solved):** the blown branch in `modular_gen_slot_ks` runs a tuned delay (comb) with a **`tanh`-saturated feedback loop** — the saturation turns runaway buildup into a stable LIMIT CYCLE (loop gain falls as amplitude grows), the bounded-jet behaviour without a second delay line. The loop gain must exceed unity to self-oscillate, so the blown branch scales `gen_ks_sustain × 1.05` (the linear pluck path keeps `gen_ks_sustain ≤1`, so it is untouched and byte-identical). `fluteSeed` now uses it: pitched at every register (E5 −8¢, C6 −5¢), breathy, bounded, sustained. Brightness is darkened **post-loop** by `filter_cutoff` (an *in-loop* lowpass would add group delay → flat pitch — keep `gen_ks_pluck` fairly light, ~0.7).
- **Infra tests** (`modular_blown_test.go`, prod build): `TestBlownKS_BoundedAcrossFeedbackAndPitch` (tanh bounds peak <1.5 across freq×feedback, incl. fb>1), `TestBlownKS_SustainsVsPluckDecays` (continuous excitation: blown tail ≫ plucked tail), `TestBlownKS_Pitched` (autocorrelation peaks at the period). Plus the full audio goldens confirm every **plucked** KS instrument is byte-identical.
- **Param plumbing for `gen_ks_blow`** (the gate, default 0; *historical — since Phase C a param is ONE `modularFields` row + `make gen-synth-abi`, see § Adding knobs to a gen-slot voice*): `modular.h`, `modular_c.go` (struct + `toCModularParams` copy + `recipeParamsToModular` get), `modular_recipe.go` range `{0,2}`, `synth_param_schema.go` default `{0}`, the `modular_genbank_schema_test.go` KS field list, the `synth_param_abi.gen.js` (regen `go run ./cmd/gen-synth-abi`), and the `EveryParamMutates` test's `activateSlotKS` case. **GOTCHA: `src/c/*.c` is compiled into `build/libdrums.a` — edits do nothing until `make build/libdrums.a` rebuilds it** (`go test`/`go run` link the static lib, they don't recompile the C).
- **Smoothness (the "industrial/noisy" fix):** a raw WHITE-noise breath drive makes the tube harsh; the excitation is now a **2-pole lowpass** (soft air, not hiss), and the loop gain is kept **just above unity** (`ks_sustain` ~0.98 → ~1.03) so the `tanh` stays near-linear = a near-sine limit cycle, not a clipped buzz. Result: smooth, tonal (spectral flatness ~0.05), natural breathing (sustain ripple ~5–7 dB ≈ real 6.6), in tune at all registers. Current `fluteSeed`: `ks_blow 0.22, ks_sustain 0.982, ks_pluck 0.6, filter_cutoff 3400`.
- **Remaining (ear):** mellow/dark vs the real reference (centroid ~1200 vs 1855) — the gentle limit cycle is fundamental-heavy. Brighter needs more drive (risks edge) or the STK jet *delay* + cubic for richer odd/even harmonics.
- **BRASS ≠ blown waveguide.** The blown-KS path made the french horn sound like "harp + organ" — at the low breath a steady brass note needs, the tube self-oscillates as a near-sine (organ) with a plucky onset (harp). **Brass wants the SUBTRACTIVE method** (Sound on Sound "Synthesizing Brass"; STK lip-filter): a SAW (full harmonic series) → resonant low-pass → a filter-envelope BLOOM on attack (the "brassiness" = brightness rises then settles; physically the nonlinear wave-steepening that enriches brass with loudness).
- **FRENCH HORN — ADDITIVE, envelope-matched to henkonen fhorn-18 (CC0, C5 ≈525 Hz).** A saw+LP could NOT match it: the real horn has a **BRASS FORMANT** (a plateau/bump at H4 ≈2100 Hz, H4≈H3) that a monotonic low-pass cannot make (at the cutoff that gets H2 right, H4 is always too weak). So **place the partials directly** at the measured ratios (H2 −18, H3 −29, H4 −31 plateau, then gentle rolloff) — like the violin/cello additive seeds. Result of iterative A/B tuning (the `cmd/synth-analyze synth -out` harmonic table vs the real WAV): **mean harmonic error H2–H7 = 2.1 dB**, centroid 1211 vs 1085, pitch −7¢. Anti-organ movement = **unison (3v, detune 6, drift 8)** chorus + a continuous band-passed **breath-noise** tap + a gentle `filtenv` bloom + light vibrato. **Method to match any wind/brass reference: render `synth-analyze synth -inst X -pitch P -out`, FFT both, print the H1–H12 dB table side-by-side, adjust gen gains to close the Δ, lower `filter_cutoff` to fix centroid overshoot from breath/HF junk.** Blown path stays for **air-reed winds** (flute ✓; oboe/sax next).
- *(Current `trumpetSeed` is additive — gens 2–8 + a noise tap — with `amp_attack` 0.45, `filtenv_attack` 0.4, `filtenv_amt` 1.6, `filtenv_decay` 2.0, tremolo `lfo_target 0`; the recipe below is the history that led there.)* **TRUMPET — DYNAMIC real-trumpet model from WHOLE-WAV analysis (sgossner SHTrumpet, CC0, G4≈392 Hz).** FOUR attempts: formant-bank→"synth violin"; subtractive bloom→"sci-fi"; waveguide+saw→"better but still a synth". The breakthrough was analysing the **entire signal's time-evolution**, not a steady window — the real trumpet is DEFINED by dynamics: **(1) a slow CRESCENDO** (amplitude swells over ~500 ms to peak); **(2) the harmonics BLOOM progressively brighter as it swells** (centroid 1539→2084 — "brightness tracks loudness"), the OPPOSITE of an onset-blat; **(3) a NOISY decaying attack** (tonguing/breath, noise-frac 0.6→0.02); **(4) NO vibrato** (a STRAIGHT tone — my ±38¢ pitch LFO was a huge synth tell). Required a **pipeline expansion — the new `filtenv_attack`** (a slow RISING filter swell; the old filtenv only bloomed-then-decayed). Recipe: single saw → resonant LP, `amp_attack 0.55` (crescendo), `filtenv_attack 0.45`/`filtenv_amt 1.9`/`filtenv_decay 4` (rise dark→bright then hold), a strong low-mid noise burst gen for the attack, `lfo_target 0` tiny tremolo (breath, NOT pitch). A/B (whole-WAV): centroid bloom 1318→2589 ≈ real 1382→2070, attack noise 0.18→0.03, no vibrato. Knobs: `amp_attack`+`filtenv_attack` = swell speed, `filtenv_amt`+`filter_cutoff` = bloom brightness, `gen1_gain` = attack "ttah". REMAINING (next ear-pass): the real's H2/H3 bloom to be LOUDER than H1 (a fixed ~900 Hz brass formant) — the sweeping LP can't both bloom and hold that formant; would need a parallel formant gen.
- **VIOLIN — bowed saw → BODY-RESONATOR BANK + BOW-DYNAMICS, tuned to mtg violin-d5 (Good-Sounds, D5≈590 Hz, A=442).** A bowed string is a SAWTOOTH (Helmholtz); a violin = its BODY + the PLAYER. Two engine features: (1) `body_model:1`/`body_mix` — 22 moderate-Q (6-11) band-pass resonators = violin body modes (woody COLORATION; moderate Q so they don't ring into a metallic "weird" drone); (2) `bow_dynamics` — a slow smoothed random walk (a new bow target every 11000 samples ≈ 250 ms at 44.1 kHz) modulating LOUDNESS + BRIGHTNESS together = the PERFORMANCE motion that breaks the static "organ" (sustain-variation 0.16 vs real 0.11; flux 0.21 ≈ real 0.21). **BASS DEBUG (recurring "too much bass" — debugged by rendering with each component TOGGLED OFF and measuring %-energy <1 kHz):** there is NO sub-bass (20-500 Hz = 0 %); the "bass" was the FUNDAMENTAL itself (590 Hz, in 500-1k) at 42 % of energy vs the real's 35 %. The body bank already helps (body-off = 64 %!), and the fix was to **CUT the body's LOW modes (280-660 Hz) that reinforce note fundamentals + rebalance toward the bridge hill (2-4 kHz)** → <1k down to 30 % (now *below* the real), bands matched (1-2k 30 %, 2-4k 31 %). This de-emphasizes the fundamental for ALL notes (the modes are fixed) while staying note-SAFE (low notes still audible; A3 peak loud). Recipe: saw, `body_model 1`/`body_mix 1.0`/`bow_dynamics 0.25`, note-safe HP 130 Hz, vibrato, airy bow noise, `gain 0.3`. **KEY LESSON:** "too much bass" on a high-register instrument is usually the FUNDAMENTAL being over-strong, NOT sub-bass — measure %-energy below the HPF point and toggle each stage to find the culprit; for a violin de-emphasize the fundamental via the body's low-mode gains (note-general) rather than a fixed high-pass (note-killing). Aliveness = TIMBRE (body bank) + PERFORMANCE (bow_dynamics).

- **The structures, by reference spectrum (always FFT the reference FIRST):** *air-reed (flute)* → blown waveguide; *mellow brass, fundamental-dominant + low formant plateau (horn)* → additive partials; *bright brass (trumpet)* → **subtractive DYNAMIC brass: single saw + filter-envelope bloom** (NOT a formant-bank — that measured well but sounded like a synth violin); *double-reed, WEAK fundamental + twin mid formants (oboe)* → additive partials + a reed-noise gen; *plucked string (nylon/steel/electric guitar)* → Karplus-Strong, where **ring/decay time and the pluck timbre matter more than a steady spectrum**. The decisive question is **where is the energy + does it sustain or decay** — strong sustained fundamental → subtractive/blown; weak fundamental or non-monotonic formants → additive/formant-bank; a decaying transient → KS. After the harmonic peaks match, **add broadband noise** to hit the real flatness/centroid (pure harmonic/KS output reads as "sci-fi synth" / too clean).
- **⚠️ A SPECTRAL MATCH IS NOT A PERCEPTUAL MATCH (the trumpet lesson).** The steady-state envelope is necessary, not sufficient. What ALSO defines an instrument: **(1) ONE voice vs many** — `unison_voices > 1` turns any solo instrument into a section/synth-string pad (the #1 reason a brass/wind patch sounds like "synth strings"); **(2) DYNAMICS** — a filter envelope (brass bloom), the attack transient, pitch/amplitude motion. A static, chorused tone with the right spectrum still sounds synthetic. **When a tuned patch sounds wrong despite a good spectral match, suspect unison + missing dynamics BEFORE re-tuning gains.**

Why ours sound static/buzzy: matching the steady-state spectrum is necessary but misses the three dynamic/textural cues the ear uses for "a real player is blowing this":

1. **Brightness tracks loudness/dynamics — the #1 wind+brass cue.** Quiet note ≈ fundamental + ~6 harmonics; loud note = 15+ harmonics. Engine: filter envelope blooms cutoff on attack (`filtenv_amt` octaves / `filtenv_decay` s) + velocity→cutoff.
2. **A SOFT flute spectrum is FUNDAMENTAL-DOMINATED — nearly a sine; a LOUD one is not.** The catalogued `flute-A4` (Iowa MIS `Flute.nonvib.ff.A4`, fortissimo, no vibrato) has H2 > H1 (H1 −5.5, H3 −1.5 dB re H2), and the voice tuned toward it was approved as "very similar to the reference" while the listener asked whether the reference was a flute at all ("sounds like a clarinet or oboe"). Dynamics move a flute's spectrum more than any other cue: say which flute you are matching. MEASURED from a soft expressive flute (sgossner E5 expressive-sustain, CC0, via the analysis recipe below): H1 0 dB, **H2 −18 dB** (≈0.12×), H3/H4 −34 dB, H5 −45 dB, rest trace; inter-harmonic noise floor ≈ −45 dB. **The fatal bug was a near-equal P1≈P2 + full harmonic stack → "high-pitched organ buzz."** Set `gen1` (fundamental) dominant, `gen2` ≈ 0.12×, `gen3/4` ≈ 0.02×, drop the rest. (`fluteSeed` is now built from exactly these ratios.)
3. **Vibrato — and what actually works on a near-sine:** the real flute's "expressive vibrato" at **~5.5 Hz** is *all three at once* — pitch **±16 ¢**, amplitude **12 dB(!)**, brightness/centroid **40 %** swing (all measured). BUT with a **single-target LFO** on a **near-sine** spectrum: a brightness sweep (`lfo_target:2`) moves almost nothing (no harmonics to filter), and amplitude-only (`lfo_target:0`) sounds like a tremolo effect — so **pitch (`lfo_target:1`, ±16 ¢ ⇒ `lfo_depth`≈0.011, delayed onset)** is the right single-LFO choice. **Capturing the real expressive vibrato (pitch+amp+brightness together) needs a multi-target LFO — the clearest next engine upgrade.** *(Earlier guidance here said "flute = brightness vibrato, lfo_target:2" — that was theory from Synth Secrets; the real-data analysis + our near-sine spectrum proves pitch is the working choice.)*
   - **Oboe / sax / brass** use pitch + amplitude vibrato (`lfo_target:1` or `0`), ~5–6 Hz, delayed onset.

   **Spectral-analysis recipe (use on any real reference WAV):** decode to mono PCM (`ffmpeg -ac 1 -ar 44100 -sample_fmt s16`), Hann-window a ~0.5 s sustained slice, `rfft`, read `20·log10(X[h·f0]/X[f0])` per harmonic for the ratios; for vibrato, short-STFT the sustain and track the f0-bin frequency (pitch), peak magnitude (amplitude), and spectral centroid (brightness) over time, then FFT each track to get the ~5–6 Hz rate. This is how `fluteSeed` was rebuilt — **measure the real signal, don't guess ratios.** Render the synth's own note to WAV for an apples-to-apples A/B with `cmd/synth-analyze`: `synth-analyze synth -inst flute -pitch 19 -dur 3 -out /tmp/synth.wav` (pitch = MIDI−57; prod-CGo render).

   **Two hard-won engine facts from the flute A/B (matching a real flute, not just its steady spectrum):**
   - **Amplitude "breathing" is the #1 organ→flute cue** — bigger than pitch vibrato. A real flute's sustain RIPPLES ≈6 dB at the ~5.5 Hz vibrato rate; a dead-flat amplitude reads as an organ no matter how correct the spectrum. With one LFO, `lfo_target:0` (amplitude tremolo, depth ~0.5) buys this — at the cost of pitch vibrato. **Doing pitch + amplitude + brightness at once = a multi-target LFO (the clearest next engine upgrade).**
   - **A noise-tap (breath) gen goes SILENT unless you set its envelope mix.** Per-gen env in `modular_stages.c` is `fast_mix·e^(−fast_rate·t) + tail_mix·e^(−tail_rate·t)` with defaults `fast_mix=1, fast_rate=0, tail=0` (so an *unset* gen is constant unity). Setting only `env_fast_rate` turns the bed into a decay → breath dies. For **continuous** breath set `env_tail_mix≈0.9 + env_tail_rate=0` (constant bed) plus a `fast_mix` attack chiff. Also: the gen-slot path renders upper partials ~10 dB *under* their gain, so boost gen3+ above the bare measured ratios to hit the real centroid.
4. **Excitation waveform by bore** (subtractive path): open pipe (flute) → saw/triangle, then HPF a few hundred Hz + LPF ~2 kHz + moderate resonance; closed pipe (clarinet) → **square** (odd harmonics, hollow) — in the LOW (chalumeau) register only: the catalogued `clarinet-A4` (Iowa ff, clarion register) carries every even harmonic (H2 −10, H4 −12, H6 −24 dB re H1), and adding them to the odd-only additive clarinet took its essence distance 0.78 → 0.28 (#13 → #1); conical/brass + reeds (trumpet/horn/sax/oboe) → **saw** (bright, full series). Reeds add **sustained low-level breath noise**; **flute is better with little/no broadband noise** (Reid) — its breathiness is the attack chiff + filtering, not hiss. Measure before trusting this: the ff Iowa flute holds ~6× the breath of the voice tuned to it (essence noise 0.018 vs 0.003) and a flute with NO held air reads as a reed. *(Note: the additive near-sine path above also works for flute and is what `fluteSeed` uses — it matched the measured spectrum directly.)*
5. **Attack chiff:** flute fast clean attack (~50–100 ms slow passages, faster for runs); reeds/brass slower onset (~100–300 ms) with a brief noise spike at the very start (short-decay noise-tap gen gated to attack).
6. **Formants / presence (absolute Hz, do NOT track pitch):** 2–4 kHz presence + 5–8 kHz brightness; brass ~8 kHz; clarinet ~3 kHz hollow dip. Author as band-pass gen slots.
7. **Reverb is critical for flute** (phase interaction of its two sound sources) — give a flute row a Reverb in its FX panel (there are no send buses since 2026-10-27; a template spec's `Effects` holds only filters today).

**ENGINE constraint** (`modular.c` filter stage, the `cut_sweep` / `filt_env_on` branch): `lfo_target:2` cutoff-sweep and `filtenv` bloom are **mutually exclusive** (`if (cut_sweep) … else if (filt_env_on)`). A flute using brightness vibrato therefore **gives up** the attack bloom — acceptable (clean flute attack). Reeds/brass keep `filtenv` and use pitch/amp vibrato. `lfo_delay` **is** wired for all three targets (onset ramp `min(t/lfo_delay,1)`), so delayed vibrato onset is available — **not** an engine gap.

**PD solo-wind repertoire (sheets available → templates):**
- **Flute:** Bach Partita BWV 1013 (have); **Telemann 12 Fantasias for solo flute, TWV 40:2–13** — the canonical unaccompanied-flute set (IMSLP/Mutopia).
- **Oboe:** Telemann Fantasias (play on oboe); Marcello oboe concerto slow mvt; **Ferling 48 Studies** (PD oboe etudes).
- **Trumpet / Horn:** Baroque trumpet (Bach, Telemann); Mozart horn-concerto solo lines.
- **Sax:** 20th-c. → limited PD; Ferling etudes (adopted for sax) are PD.

### Sources

- euphonics.org — instrument signature modes (violin/cello body resonances)
- ccrma.stanford.edu/~jos — formant filtering, bowed strings, PASP (physical audio signal processing)
- ZynAddSubFX PADsynth algorithm doc (Paul Nasca) — `zynaddsubfx.sourceforge.io/doc/PADsynth/PADsynth.htm`
- Sound On Sound *Synth Secrets* — synthesizing strings, synthesizing brass
- STK (Synthesis ToolKit) — Bowed/Brass physical models (reference for what we approximate)
- Penn State acoustics — vibrating circular membrane (Bessel modes, air-loading stretch)
- TB-303 / acid-bass references — swept resonant-LP "wow" envelope behavior
- music-dsp / KVR DSP archives — formant-bank and ensemble-detune recipes
- Perry Cook & Gary Scavone, *Real Sound Synthesis for Interactive Applications* — modal & physical synthesis
- Julius O. Smith, *Spectral Audio Signal Processing* — additive/spectral spreading background

---

## DSP Primitive Modules

Reusable C building blocks for synthesis. `wavetable.h` is used by `fmsynth.c`, `modular.c` and `modular_stages.c`; `adsr.h` by `modular.c`. `insert_fx.c` uses none of them. Available for any new C synth code.

### Wavetable Oscillator (`src/c/wavetable.c`)

Band-limited wavetable oscillator with guard-point interpolation.

**Core types**:
- `wavetable_t` — Table of `length` samples + 1 guard point (`table[length] == table[0]`), eliminating the branch in linear interpolation.
- `wt_osc_t` — Phase accumulator oscillator referencing a `wavetable_t`.

**Waveform generation** (via Fourier additive synthesis, band-limited):
- `wt_generate_sine(wt, buf, length)`, `wt_generate_saw(wt, buf, length, harmonics)`, `wt_generate_square(wt, buf, length, harmonics)`, `wt_generate_triangle(wt, buf, length, harmonics)`

**Oscillator API**:
- `wt_osc_init(osc, wt, freq, sr)` / `wt_osc_set_freq(osc, freq, sr)` — Phase-preserving frequency set
- `wt_osc_tick(osc)` / `wt_osc_tick_pm(osc, phase_mod)` — Single-sample inline

Default table size: `WT_DEFAULT_LENGTH = 4096` gives <0.01% THD. Buffers must be at least `length + 1` floats.

### ADSR Envelope (`src/c/adsr.c`)

State-machine ADSR with linear and exponential curve options.

**Stages**: `ADSR_IDLE → ADSR_ATTACK → ADSR_DECAY → ADSR_SUSTAIN → ADSR_RELEASE → ADSR_IDLE`

**API**:
- `adsr_init(env, sr, attack_sec, decay_sec, sustain_level, release_sec, exponential)`
- `adsr_trigger(env)` — Note-on (starts from current value for legato)
- `adsr_release(env)` — Note-off
- `adsr_tick(env)` — Single-sample inline

Exponential mode: `value += rate * (target - value)` for natural curves.

### LFO (inline — no standalone module)

There is **no `src/c/lfo.c`** and no general-purpose `lfo_init`/`LFO_SINE`
module. Low-frequency modulation is implemented inline where it is used:

- **`modular.c`** — a per-voice sine LFO driven by `lfo_rate` / `lfo_depth`
  / `lfo_target` (0=tremolo, 1=pitch vibrato, 2=filter-cutoff sweep), with
  `lfo_delay` onset ramp (Phase-8G). This is the LFO the organic-melodic
  recipe cards reference.
- **`insert_fx.c`** — per-effect inline LFOs in chorus, tremolo (sine / triangle / square), flanger, phaser, autowah and tape (wow/flutter).

New C synth code that needs periodic modulation should follow the
`modular.c` inline pattern (phase accumulator into the shared sine
wavetable) rather than expecting a reusable module.

### The FINISH stage (`modular_finish`, `src/c/modular.c`)

The post ops are one pure function of the param block, run after every voice,
always, in this order — each a no-op at its identity:

| Field | Knob | Op |
|---|---|---|
| `post_pitch` | Resample | re-index by `2^(st/12)` (pitch and length move together) |
| `post_decay` (0.01..1) | Length | post-envelope `exp(-t·post_decay_rate·(1/post_decay − 1))`; 1 = full length |
| `post_decay_rate` (≥0.5) | Fade Speed | the envelope's rate |
| `post_body` | Low Boost | 120 Hz one-pole LP blended in |
| `post_brightness` | Crisp | 4–12 kHz one-pole HP blended in |
| `post_darken` | Darken | one-pole LP (`2000 + (1−d)·6000` Hz) CROSSFADED in by `d` |
| `post_drive` | Crunch | `tanh(x·(1+3d))/tanh(1+3d)` CROSSFADED in by `d` |
| `out_clamp` | Limit | hard clip at ±value; 0 = no limit (identity 1.5) |

Then a structural guard zeroes any non-finite sample. There are no enable
gates, order switches or NULL defaults (`post_params`, `post_config`,
`apply_post_params` and `synth_params.h` were deleted in the Phase C fix pass).
A seed that must NOT be limited spells `"out_clamp": 0`. Pinned by
`TestFinishKnobExtremesStayFiniteAndBounded` (every pair of extremes, real
render length) and `TestFinishKnobsRespondProportionally` (the first 1% of a
knob does ≤10% of its work).

---

## Cloning a struck instrument — the analysis half (`make clone`)

`make clone INST=<id>` measures the instrument's reference recording into a
MODEL — never audio: damped modes, a raised-cosine strike (Tc, hardness), the
decay law α(f) = a·(f/1k)^p + b·(f/10k)^6 and third-octave residual noise
bands (level, attack, two-rate decay). Output:
`$BEATMO_WAV_REFS/clones/<id>/analysis.json` + `report.md` (mode table, STFT
cross-check "missed" partials, glide diagnostic, per-band diagnostics,
timings). It refuses clipped, onset-less, short (< 150 ms) and non-modal
(> 50 % residual) input. Code: `internal/audio/clone` (dev-only,
`TestCloneIsDevOnly`), `cmd/clone`.

What the VCSL corpus taught the mode estimator (each one shipped broken first):
- **Noise floor**: the pre-roll when ≥ 20 ms exist, else the lower of the
  quietest non-zero 2 ms frame and a SPECTRAL floor (median Hann bin power of
  the last 4096 samples — noise is broadband, a ring is a few tones). The
  quietest-10 %-of-frames rule read the ring itself as the floor (vibraphone,
  tubular bells ring to the clip end behind < 10 ms of pre-roll).
- **Onset** must clear −40 dB re the peak too: cowbell's digitally silent tail
  made the floor 0 and the onset fired on pre-roll noise 10 ms early.
- **Window start** = 5 ms after the 2 ms envelope first comes within 3 dB of
  its early maximum (not after the maximum: vibraphone beating creeps up for
  70 ms).
- **Order**: ESPRIT proposes poles at the noise-threshold order (≤ 24 per
  band); orthogonal matching pursuit keeps those that explain ≥ 20σ² of the
  band. A fixed ESTER bar (0.3) threw away real tubular-bell partials; the
  untrimmed order gave cancelling pole clusters.
- **Amplitudes** come from ONE joint, truncated-eigen least-squares fit of the
  full-band recording from the onset (dense over the first 0.2 s) — per-band
  amplitudes are only valid inside the band's window and blew up when
  extrapolated back (marimba's phantom 32 Hz mode came back +48 dB). Modes
  that had to fall > 40 dB before the window opened are strike transient,
  not modes.
- Corpus results (2026-10-12): residual marimba 0 %, vibraphone 0 %,
  glockenspiel 5 %, xylophone 5 %, kalimba 7 %, tubular bells 16 %, cowbell
  23 %; tom / conga fit (6 / 21 %) only through mode clusters and report a
  15–20 cent pitch glide — drum heads are out of slice-1 scope. ~1.5–2.5 s per
  instrument (EstimateModes dominates; bands are independent and can go on a
  pool).

### Invert → refine → listen → adopt (2026-10-13)

`make clone` then turns the analysis into a CANDIDATE document and refines
it; nothing reaches the shipped instruments without the ear.

1. **Invert** (`clone.Invert`): a fresh document on `core` + `finish` +
   `modal` + `strike` + `noiseband` (no FX). Modes ranked by energy, −60 dB
   floor, ≤ 128, doublets never split. Rows are stored relative to the models
   the C evaluates live — amplitude ÷ (strike S · hardness tilt), decay ÷ the
   fitted law — and the fitted strike / law are the stages' absolute values,
   so the document reproduces the measurement EXACTLY at the fit pitch
   (`TestInvertReproducesTheMeasuredModes`). A pitched reference → ratio rows,
   `render.melodic`, fit pitch 12·log2(f0/220); an unpitched one → Hz rows in
   the pitch-0 domain (÷ 2^(s/12)), so the playback resample at the session
   pitch s restores it (rows the resample would push out of a column's bounds
   are dropped). `-into <id>` keeps an instrument's identity; default id
   `<inst>-clone`.
2. **Loss** (`clone.Loss`, all four terms reported): MR-STFT (256/1024/4096,
   log-mag L1 floored −80 dB + spectral convergence, first 100 ms ×2),
   2 ms envelope (dB, −60 dB floor), the trusted descriptors (inharmonicity,
   tristimulus, crest median — each net of its own Norm at identity: a
   ratio-kind comparator reads 0.68 on identical inputs) and a prior (mean |dB|
   of amplitude and decay vs the analysis). Candidate aligned by the same
   −40 dB onset rule as the reference, equal PEAK. Scores always come from
   `RenderAsShipped`, the sound the listener hears.
3. **Refine** (`clone.Refine`): seeded CMA-ES (Hansen 2016) over SMOOTH
   corrections baked into the rows — per populated octave a mode gain and
   decay scale, the strike's contact and hardness, per octave of noise rows a
   level and decay scale. Frequencies are LOCKED (a ±10-cent group tune was
   tried and dropped: search noise on sharp partials swamped the 4096-point
   term). σ₀ = 0.05 in the sin-boxed space (0.3 made the average marimba
   candidate 6× worse and found nothing), λ = 4 + ⌊3 ln D⌋, ≤ 150
   generations, stop after 30 without gain; the descriptor term is report-only
   (≈ 0.4 s/score). Renders on a private `async.Pool`; all randomness is in
   `Ask`, so 1 and N workers give the byte-identical document
   (`TestRefineIsDeterministicAcrossWorkerCounts`). A refine that cannot
   improve keeps the analysis-only document.
4. **Outputs** in `$BEATMO_WAV_REFS/clones/<inst>/`: `candidate.json`,
   `meta.json` (reference, fit pitch, `doc_sha`), `docs/<sha>.json` (history
   for `refs validate -loss`), `report.md`/`.json` (loss before/after, refine
   trace, timings), and `reference` / `current` / `candidate.wav` at equal
   peak and the recorded pitch. `render.amplitude` = the measure-loudness crest
   rule (`clone.LoudnessNorm`, which measure-loudness itself calls) against
   the shipped instrument.
5. **Listen**: `make ab-session FAMILIES=<inst> CLONE=1` adds the clone as
   candidate K to the level-matched round (a family absent from the candidates
   file auditions the clone alone) and asks `Ship clone K? y / n / p`; the row
   records `doc_sha` (content hash with `render.amplitude` zeroed) + `accept`.
6. **Adopt**: `make clone-adopt INST=<inst>` writes
   `internal/assets/instruments/<id>.json` (+ `index.json` when new) ONLY when
   the newest session row for that exact `doc_sha` says `y`; then
   `make asset-pack`, `go generate ./internal/audio/`, `make measure-loudness`,
   `make gen-fundamentals`.

`make clone-bench` clones the slice-1 set (tubular bells, kalimba,
glockenspiel, cowbell, marimba, xylophone, vibraphone, tom, conga). Measured
2026-10-13 (8 cores): 24–48 s each; refine lowered log-mag / envelope error on
all nine (e.g. kalimba 5.9 → 2.0 dB / 7.1 → 2.3 dB, vibraphone 2.7 → 1.4 dB /
0.34 → 0.04 dB). Tubular bells stays worst (log-mag 28 → 24 dB): ESPRIT
misses ~18 strong high partials and the noise rows stand in for them — the
next analysis fix. `refs validate -loss` reports each term's within-family
agreement with the ear (ANTI at < 0.5 over ≥ 10 pairs → weight 0); it is
advisory until clone sessions exist.

What the round trip (a known modal document → render → clone) taught:
- The ideal raised-cosine spectrum's NULLS made the inverse ill-posed (a mode
  near one needs ×1000 storage; the fit put every mode on a sidelobe peak:
  Tc 25 ms at +44 dB). STRIKE now uses the spectrum's smooth envelope
  1/√(1 + (π (f·tc)³)²) — same corner, the exact −18 dB/oct asymptote, no
  nulls — in C and Go alike, and a longer contact must fit 0.05 dB better to
  win the tie.
- ESPRIT models an onset noise burst as clusters of fast poles with
  cancelling amplitudes (8× full scale). Poles with T60 < 20 ms or an
  amplitude above the recording's peak are dropped and the rest refitted
  (`Analysis.TransientModes`); the residual's noise rows carry them.
- The two-rate residual fit could put an instantly decaying spike hundreds
  of times the peak at t = 0 (relative cost bounded by 1 per frame): +38 dB
  "noise" in a noiseless document. Its value at t = 0 is now capped at twice
  the measured onset, and a burst shorter than ~24 ms decays to the floor over
  its length instead of rate 0 (which rendered as noise that never ends).
- A clip cut mid-ring ends in a fade no mode can model — round-trip fixtures
  must ring out inside the render.

## Creating New Instruments — Checklist

### Percussion (C noise/tone synthesis)

1. **Can an existing voice do it?** Most new drums are a document on an existing family
   (copy a kick / snare / cymbal document and change values) — no C at all
   (§ Adding a new built-in instrument).
2. **Genuinely new DSP**: add a branch / path to the family's `modular_gen_slot_*` in
   `src/c/modular_stages.c` (e.g. the cymbal modal plate), or a new `gen_source` value.
   Append any new knobs to the flat param ABI as a new tail group (see "Adding knobs to a
   gen-slot voice" in § Cymbals as modal plates).
3. `make build/libdrums.a` (desktop links the prebuilt static lib) and
   `make src/js/drums.single.js` (the browser's Emscripten build; the render exports are
   only `_render_modular_p`, so nothing to add to
   `EXPORTED_FUNCTIONS` for a new voice).
4. `make gen-synth-abi` if the ABI grew (C header + JS layout); declare the new values
   on their module type in `module_registry.go`; `make asset-pack` for a new document.
5. **Test**: fast Go tests, the real-build `go test ./internal/audio/` (goldens,
   `TestDeclaredModuleValuesAreLive`, every-param-mutates), `cmd/gen-fundamentals`, and the browser
   `webaudio_drums_consistency.browser.test.js`.

### Using DSP Primitives in New C Instruments

Prefer reusable modules over inline implementations:
- **Oscillators**: Use `wt_osc_t` with `wt_generate_*()` instead of `sin()`. Initialize a static wavetable once, share across instruments.
- **Envelopes**: Use `adsr_t` for proper ADSR stages. For simple exponential decays, inline `exp()` is still fine.
- **Modulation**: there is no reusable LFO module (`lfo_t` does not exist) — LFOs are inline: the modular voice's `lfo_*` params (vibrato / tremolo / filter sweep, `modular.c`) and per-effect LFOs in `insert_fx.c` (chorus, tremolo, flanger, phaser, autowah, tape).
- **Panning**: per-instrument stereo placement is the mixer's job (channel pan in Go / WebAudio), not the C voice's.

---

## Tuning by measurement: which axes to trust, and what the words mean

Everything in this section is derived from the 44-family listener corpus in
`internal/audio/testdata/ear_verdicts.jsonl` and is reproducible with
`cd src/go && ../../.tools/go/bin/go run ./cmd/refs validate [-terms]`.

### Only three axes have earned trust

`refs validate` scores every registered descriptor by AUC — the probability that
an instrument the listener disliked scores worse on that axis than one they
liked — with a 95% CI, because 8 good / 6 bad families cannot support a bare
number.

| axis | AUC (95% CI) | use |
|---|---|---|
| `inharmonicity` | 0.95 (0.78–1.00) | **trust** — where the overtones sit is what decides wooden vs metallic vs string |
| `tristimulus` | 0.90 (0.67–1.00) | **trust** — which band carries the tone |
| `crest median` | 0.81 (0.55–1.00) | **trust** — transient sharpness |
| `ring T60` | 0.21 (0.00–0.47) | **ANTI — do not tune to it** |
| `centroid spread` | 0.17 (0.00–0.40) | **ANTI** |
| everything else | CI spans 0.50 | unproven on this corpus |

**`ring T60` is actively misleading.** Every reference is truncated by its own
`convert.max_sec`, so anything that rings hits the clip end long before −60 dB
and *both* sides report a clamped lower bound — which compares as "fine" on
instruments the listener rejected outright (harp read "1.1x short"). A previous
tubular-bells rebuild optimised against it and recorded success in its seed
comment while the ear still said "scrape this and try again". Use `decay T10 /
T20 / T30` and `energy sections`, which measure inside the clip instead of
clamping.

**Perceived quality tracks partial structure, not envelope timing.** Xylophone's
T10 is 5.6x off the reference and the listener still said "sounds like the real
thing". Fix the overtones first.

### What the listener's words mean, in numbers

`refs validate -terms` checks each perceptual axis against the families whose
notes actually use the word, and ranks all axes by specificity to that word
(negative class = families with a note using *different* words — silence about
an instrument is not evidence it lacks the property).

| word | axis | direction | evidence |
|---|---|---|---|
| "airy", "breathy", "sounds like a wind instrument / a flute" | `noise persistence` | high | agrees on 5 of 6 families; specificity +0.60 |
| "needs to be warmer / fuller" | `brightness ratio` | high | synth cello 21.4 vs ref 11.1; synth oboe 14.4 vs ref 6.9 |
| "wooden", "marimba-esque" | `partial series deviation` | low | **not** `inharmonicity` — the kalimba's inharmonicity already matched while its partial *sequence* did not |
| "metallic", "bell", "resonant" | `inharmonicity` | low | tubular-bells |
| "goes up and down in time" (about vibrato) | `vibrato jitter` | low | reference double-bass 0.411 vs a metronomic LFO's 0.000 |
| "wavy", "waves in volume or intensity" | each harmonic's level over time (p95−p5 dB in the held body) — **not** `mod.am.depth` | high | tuba: unison beating, 4–7 dB on H1–H3 vs the recording's ~2 dB, while the whole-band AM read 0.018 vs the recording's 0.083; violin: `bow_dynamics` 0.25 |
| "vibrating too mechanically" (bowed) | a periodic pitch component at the LFO rate | present | cello: the 4 Hz LFO was the only periodic motion; the recording drifts at ~1.6 Hz and with the LFO at 0 the waveguide's own ±7 ¢ jitter remains |
| "nasal" (oboe) | the share of the formant harmonic and the holes around it, not `formant.f1_hz` | concentrated | oboe-A4: H3 dominant, H5 a −31 dB hole, H6+ ≈ −21 dB; ours carried H5/H6 at −14/−11 dB, and f1 matched (1421 vs 1472 Hz) throughout |
| "stronger pluck" | onset noise share + the first 300 ms drop | higher | guitar-nylon recording: noise 0.088 vs 0.025, −8.5 dB at 0.3 s vs −5.4 |
| "punch", "crack", "less air" (drums) | body energy 30–80 ms; 2–6 kHz in the first 20 ms; the noise share | up; up; down | rimshot: noise 0.83 vs the recording's 0.72 |

**`noise persistence` (late-half noise / early-half) is the single most useful
new axis.** Wind-vs-string and airy-vs-dry are acoustically the same question: a
blown instrument is excited by turbulent air the whole time it sounds, a bowed or
plucked string has a noisy attack that settles into a clean tone. A whole-signal
noise *average* cannot separate them — the same total noise piled at the onset or
spread evenly reads identically.

**Brightness must be normalised by f0.** `bright.centroid` compares absolute
hertz, so a note an octave up always measures "brighter"; it scored AUC 0.36,
anti-correlated. `timbre.brightness_ratio` divides by the fundamental.

### Calibrating your own ear against the numbers

`make probe-sounds` plays isolated stimuli that vary ONE property at a time at
constant pitch and loudness (`refs probe` prints the measured axis value for
every step). Sets: `warmth`, `noise-persistence`, `airiness`,
`partial-structure`, `attack`, `energy-shape`, `vibrato`, `tristimulus`. Use
them to answer two questions no instrument comparison can: which word you use
for a property, and at which step the difference becomes audible — the second
turns a threshold from a guess into a measurement.

### Is it still THAT instrument? The essence distance (`refs match`, 2026-10-17/18)

The axes above say *how* a voice differs; the essence distance says *whether it
still reads as the instrument*. `fingerprint.AnalyzeEssence` / `EssenceTerms`
compares two clips played at the same note on five terms — **Colour** (energy-
weighted ERB loudness spectrum), **Evolution** (how that colour moves from
attack to body), **Texture** (per-octave noise share: the median bin, which sits
between partials), **Decay** (dB 30–500 ms after the loud onset) and **Attack**
(10→90 % rise, weight 0.1) — with pitch, note length, level and room removed by
construction. Spec + every rejected variant:
`docs/superpowers/specs/2026-10-17-wav-match-essence-design.md`.

**Why it can be trusted where `synth-match`'s distance could not.** Two checks
that grade the distance with no synth in the way:
- **Real vs real**: real recordings of the same instrument (the cello, horn,
  trumpet, violin and piano pairs) rank each other #1. So when a synth voice
  ranks low against its own recordings, the VOICE is off, not the metric.
- **Nuisance invariance**: `make wav-match-nuisance` re-plays every voice at
  ±5 st, a cut-short note, a room, reverb, a bright/dark mic: itself in the
  top 3 on 484 of 490 cells; `TestEssenceSelfIdentifiesUnderNuisance` (real
  build) 16/16. Leave-one-out on the 64 catalogued refs (`make
  wav-match-validate`): own family #1 20, top-3 26, MRR 0.402.

**Use it as the tuning to-do list, then let the ear decide.** `make
wav-match-map` → `$BEATMO_WAV_REFS/matches/MAP.md` lists every roster
instrument as MATCHED / FIDELITY GAP / STAND-IN / UNCOVERED, and for a gap names
the term that loses it and the DIRECTION the voice is off (`Essence.Traits`:
darker/brighter as a colour centroid in Hz, noisier/cleaner as the noise share,
dies faster/holds longer as dB after the peak). `refs match -explain inst,inst
-wavs <file>` prints every term for one recording. After a retune re-run the map
(did the instrument climb on ITS recordings?) and `make wav-match-nuisance`.

**What the 2026-10-18 map says, by family** (first recording of each of the 49
gap instruments; colour is the losing term on 37 of them, and on 30 of the 45
leave-one-out misses in validate). The directions agree with the listener's own
notes in `ear_verdicts.jsonl` (and `wav_match_verdicts.jsonl`), which is why
they are worth acting on:

| family | direction (count) | the ear said |
|---|---|---|
| brass (trumpet-mellow, trombone, tuba, french-horn-loud) | **darker** 4/4 (trombone 503 vs 1622 Hz, tuba 279 vs 768) | trumpet "close but needs refinement" |
| winds (flute, flute-breathy, pan-flute, oboe, oboe-full, clarinet) | **noisier** 5/6 (flute 0.29 vs 0.02, oboe 0.13 vs 0.00) | flute "very breathy", oboe "way too airy/breathy" |
| bowed (violin, viola, cello…) | colour 4/5; violin **darker** (1386 vs 1812 Hz), cello brighter+noisier | "slightly fluty", cello "too airy and breathy" |
| drums / snares | **cleaner** 6/6 and 4/5, drums **darker** 5/6 | rimshot "fuller… rattly/vibrational and cracky" |
| idiophones (xylophone, glockenspiel, kalimba, cowbell…) | **cleaner** 5/8, **holds longer** 5/8 | kalimba "string being plucked… needs wooden", tubular bells "a simple clean tone" |
| plucked (guitar-nylon-bright, harpsichord, koto) | **brighter** 3/3 | guitar-nylon "too pitched up or bright" |

**Turning a gap into a candidate: `refs tune` (`make tune` / `make tune-picks`,
then `make ab-tuned`, then `make tune-adopt`; 2026-10-19).** The tuning loop the
essence distance makes possible, and the replacement for `synth-match` on the
picked instruments. It optimises the RAW essence distance (no hubness),
averaged over every recording of the instrument (each at its measured note,
held as long as it sounds, onset-trimmed — `renderMatchDoc`/`evalDoc` share
the matcher's code), with the clone's CMA-ES over the document's own
continuous values, each in a trust box around its shipped value (±30 % of the
range, ×/÷3 for log values) plus a small pull back toward it. Then a pruning
pass reverts every move the objective does not need (≤ 1 % together), so a
candidate carries only the changes that matter, and T1 ranks shipped and
candidate among the whole roster on each recording (`report.md`). What it
cannot search: structure (partial ratios, waveforms, filter types, a drum's
tune, pitch), the vibrato LFO, unison, formant tables, and anything an off
stage holds. Rules learned building it:
- **It cannot search the `lfo` and `unison` stages, `snare_tune` or a layer's harmonic number** (`tuneSkippedStages` / `tuneSkippedValues`), so a defect that lives there survives any number of rounds: the tuba's unison beating and the cello's metronomic vibrato reached the listener in the round-1 candidates and were fixed by hand (round 2, 2026-11-07), as was the clarinet's missing even harmonics. When the ear names motion ("wavy", "mechanical") look at those stages first.
- **Skip a recording filed under the instrument that is not it** (`skip` in the pick). Round 1 averaged `snare-vcsl` into the snare's objective — a VCSL `HitNS` take, the snares OFF (472 Hz, noise 0.15 against `snare-freesound-alt`'s 1278 Hz, 0.64) — and the candidate came back "an airy breathy high tom".
- **A candidate is tuned against the shipped document of its day.** `clone adopt` keeps the shipped `render.amplitude` (measure-loudness owns it; doc_sha excludes it), drops `origin`, and refuses a candidate whose `render.seconds` differs from the shipped document's unless its pick `set` it — the round-1 candidates still carried the removed `render.beats` and the old web length (2 s for a 1 s note), and a verbatim copy would have undone the fixed note length.
- **A partial table cannot brighten past its last harmonic.** The trombone has
  five partials (H5 at A3 = 1.1 kHz) against a recording whose centroid is
  1.6 kHz; `add_harmonics` appends silent partials (gain 0 = the shipped sound)
  for the search to raise.
- **A pick whose listener note disagrees with the reference is restricted.**
  The harp ranks #1/#2 and is BRIGHTER than one recording, yet the listener
  asks for brighter and longer: its pick holds the voice twice as long
  (`set: render.seconds`) and searches only the string and body (`only`), so the
  matcher cannot drag it away from what was asked.
- **In-sample is a screen, not a verdict.** Most families have one recording,
  so a better rank there proves only that identity was not lost; the ear
  decides, and `tune-adopt` refuses anything without a `y` on that exact
  `doc_sha`.
- **The audition must play what was scored (2026-10-20).** The first
  `make ab-tuned` rendered R/C/T through the generic compare set: every voice
  at 120 BPM (a 1 s oboe, gated, then digital silence, against a 2 s note),
  the catalogue clip cut hard at its trim (clarinet ends on a −0.19 sample),
  and a blocking `paplay` with no pause between clips. The listener heard it
  as "lagged / chopped", reference included -- and it STILL was after the
  render fix: plain `paplay` requests a ~186 ms buffer, PipeWire runs at its
  2048 quantum and the USB S/PDIF DAC underran 36-40 times per R/C/T round
  (`pw-top` ERR on the driver; a sink-monitor loopback cannot see it). The
  session's players now ask for 50 ms (`paplay --latency-msec=50`, `aplay
  --buffer-time`, or `ffplay`): 0 xruns (`TestBlockingPlayersAskForASmallBuffer`).
  Then "the reference does not sound clean, noisy in a vibratory way": the
  R file was bit-exact to the recording, but RAISED -- the compare level plan
  meets every clip at the set's average RMS, and peak-normalised voices are
  far louder than a recording, so the oboe played ×6.06 (+15.6 dB, its hiss
  and room tone with it) up to a 0.99 sample peak, which the 44.1->48 kHz
  resample swung to 1.03-1.04 at the DAC (clipped), and through the
  EasyEffects EQ (+5.75 dB) to 5,000 clipped samples. `tunedLevelGains` keeps
  the recording at gain 1 and brings the voices to it, all within -1 dBTP
  (`truePeak`); the real-build test asserts R == the recording at gain <= 1.
  Measure loudness differences at the DAC: a file can be perfect and still be
  played wrong. Last round ("noise in all sounds", headphones on the Schiit
  stack only, speakers clean): the EasyEffects BACKGROUND service
  (`easyeffects --gapplication-service`) keeps routing every new stream
  through `easyeffects_sink` with its EQ after the window is closed --
  `easyeffects -q` stops it; with it gone the listener heard everything
  clean. The tool-side residue was closed too: the onset trim cut slow rises
  (first sample -26..-33 dB re peak: a click), so every listening copy is
  faded in over the pre-roll it kept (`fadeInLead`; a voice whose attack is
  its first sample is untouched) and `clipFaults` reports "starts on a step";
  and the shared WAV writer (`internal/audio/encode_wav.go`) truncated
  `s*(2^(n-1)-1)` toward zero -- it now rounds on the readers' 2^(n-1) scale
  (`pcmStep`, `TestWAVPCMRoundsToTheNearestStep`). `session_tuned.go` now builds the
  set from the tuner's own code (`newTuneRec`, `settlePitch`,
  `matchTarget.eval`/`evalDoc`), fades the reference's cut, gaps the player,
  and checks every written clip (`clipFaults`, `holdFault` — a WARNING, never
  silent). `TestAbTunedPlaysEveryPickAsItWasTuned` asserts the PLAYED files
  equal the scored renders up to level and 24-bit rounding. Consequence: a
  `render.seconds` edit is inaudible there (both voices are held as long as the
  recording), so the harp pick's T plays identical to C and the session says
  so. Also note the essence reads content below −140 dB: 24-bit rounding of a
  rimshot tail moved its distance 0.037 with no audible change.

**What is NOT signal (measured, removed — do not reintroduce as a tuning target):**
- A harmonic-profile term at the known f0 (0.394 vs 0.393 MRR with/without;
  real-vs-real −0.006): the ERB colour already resolves the low harmonics.
- The legacy `fingerprint.Distance` (timbre/full/perc/fused): it fingerprints a
  sustain window from 0.3 s (no attack; a short hit is fingerprinted on
  silence), re-detects f0 per clip (two clips of one note compared on different
  harmonic grids) and floors the MFCC log at an absolute 1e-10, so room tone 60
  dB down decided piano vs double bass. **`synth-match` still minimises it** —
  see the warning in § Analysis-driven tuning.
- Spectral flatness (dominated by the partial peaks: a −12 dB breath barely
  moves it), loudness-weighted time (a −30 dB reverb tail out-voted a kick),
  decay measured to 2 s (it judged ring LENGTH, so a tuba beat the harp).

**Comparing a voice with a recording — the rules the matcher had to learn:**
- **Same note, measured, not read from the filename.** `matchRefPitch`: YIN can
  lock onto the 2nd or 3rd harmonic of a weak fundamental
  (`subharmonicCorrection` checks ÷2 and ÷3); a note is only named when the
  strong partials form one harmonic series AND a LOUD low harmonic (one of 1..5
  within 6 dB of the loudest) exists — a reed's formant can make the 6th
  harmonic the loudest (the acclivity oboe A440), while an inharmonic cluster
  (the mbira-like kalimba ref) has no pitch at all.
- **Held as long as the recording.** A shipped `render.seconds: 1` is a 1 s
  note against a 2 s reference; `synthmatch.HeldSeconds` lengthens the voice
  to the recording's length (never shortens it), and both sides are
  onset-trimmed the same way.
- **Relative floors only.** An absolute floor lets digital silence vs room tone
  (inaudible) dominate a log spectrum.
- **A bass voice played far above its range turns into a near-sine and wins
  sparse high notes** (tuba at G5 0.413 vs the harp 0.418 on a harp recording).
  Documents carry no playable range yet, so the matcher cannot rule it out.

**Check the reference before tuning to it — several are not what they are
filed as**: the `celtic_harp_g2` download sounds G5; `koto-vcsl` is a glissando,
not a note; `kalimba-vcsl` is an mbira; the glass-armonica ref is wine glasses;
`pan-flute-vcsl` is a recorder; `singing-bowl-freesound` (ixwolf `g1_rr1`) is a
GUESSED label for a harmonic tone on 49 Hz (no bowl is harmonic); `shaker-vcsl`
is a sustained shake gesture, not one hit; `snare-vcsl` (`Snare2_HitNS`) is a hit
with the snares OFF — a tom-like head with no wire buzz; `flute-A4` is a
fortissimo non-vibrato flute (H2 above H1), which reads reedy; `clarinet-A4` is the
clarion register, with even harmonics. A voice can only be as right as the
recording it is tuned against.

### Signal-chain limits worth knowing before you tune

- **The bowed strings cannot produce a bow bite.** References show a broadband
  0.20 noise burst at onset dropping to ~0.010; the synths are flat at ~0.017.
  Three fixes were tried and measured: scaling the multiplicative bow-velocity
  friction (the waveguide shapes it into tone), injecting noise into the string
  state (a resonator returns it *at the harmonics*), and adding it to the
  radiated output (removed by the voice's own 3.1 kHz low-pass). Every noise
  source in the modular pipeline injects **pre-filter**, so nothing broadband
  survives a voice that low-passes for its tone. A bite needs a **post-filter
  transient stage**. Until then the bowed strings will keep reading as "slightly
  fluty", and `noise persistence` is the axis that will show it landing. The
  WAV-clone MODAL and NOISEBAND rows are no exception: `render_modular_p` adds
  them after the gen bank, before the ADSR and filter (only bow dynamics, drive
  and the FINISH ops run after the body bank).
- **The FINISH stage's Limit (`out_clamp`, identity 1.5) is a HARD clip, and an
  additive bank summing past it is clipped there.** The trombone's, tuba's and
  oboe's partial gains add up to 2.7–4.9, so their sines (all phase 0) peak far past
  ±1.5 and the clip writes their upper harmonics: with `out_clamp` 0 the trombone's
  H6–H12 fall from −12…−17 dB to −84 dB (essence 0.60 → 1.41 — the clip IS its
  brass brightness). Consequences: a layer added at a harmonic the clip already
  writes can CANCEL it (raising H6 0.10 → 0.45 took H6 from −5.8 to −18.5 dB; the
  oboe's H5 0.005 → 0.3 sent it to −50 dB), so do not place partials by the
  measured ratios on a clipped bank — halve every partial gain (the voice is
  peak-normalised, so only the clip changes) or brighten through the clip and drive
  instead. This is also why an upper gen seems to render "~10 dB under its gain".
- **A note's loudness is a gain, never a timbre.** The voice cache keys on
  instrument, rate, params hash and pitch — no velocity, and no tempo — so a node at
  volume 0.2 is the same render as at 1.0, scaled. Real soft and loud notes
  differ in brightness and attack (a piano, a drum, brass most of all);
  nothing in the engine models that yet.
- **Every repeat of a note is bit-identical.** `noise_seed` is 0 and the
  round-robin variants are reserved (`voice_cache.go`), so a roll or a
  repeated hi-hat replays one buffer — the "machine gun" tell.
- **Vibrato must not be metronomic.** A mathematically constant rate is the most
  recognisable "LFO, not a player" tell; the listener caught it within one pass.
  `modular_vibrato_phase` integrates a jittered rate. Guarded by
  `TestBowedStringVibratoIsNotMetronomic`.
- **`osc_type` 7–11 (the waveguides) bypass the unison branch.** modular.c's
  dispatch is an if/else-if chain and the physical-model branch returns first.
  Vibrato is applied explicitly inside that branch; unison is still unreachable.
  Guarded by `TestNoSeedConfiguresUnisonOnAWaveguide`.

### A seeded parameter is not necessarily a live one

Four separate wiring bugs have shipped in seeds whose comments described effects
that never rendered:

- `unison_*` on `celloSeed` (five parameters, osc_type 7 bypasses the branch).
- `lfo_target:1` vibrato on all four bowed strings (same branch).
- `genN_gain` on the eight **transplanted** generator voices (`gen_source` 3–10:
  KS, analytic, kick, tom, snare, clap, cymbal, FM). Declared, seeded on 24
  instruments, read by none of them. (Since FIXED for KS, analytic, tom and clap
  — sources 3/4/6/8 now apply it as `slotGain`, 0 → 1; kick, snare, cymbal and FM
  still ignore it.) The kalimba is why it mattered — its seed
  pulls the Karplus-Strong pluck back to 0.4 so the additive bar partials
  dominate, and the pluck had been running at full level, which is exactly the
  "sounds like a plucked string, needs the wooden marimba feel" verdict. Note
  that `gen_gain`'s schema identity is **0**: correct for a wavetable slot (an
  unconfigured slot must be silent), fatal for these (they are activated by
  `gen_source` alone), so 0 means "not configured" there.
- Analyzers computing fields nothing reads — `AnalyzeSnare`/`AnalyzePiano`/
  `ComputeWindAttack`, then `Tristimulus`/vibrato/formants, then
  `EnvelopeSegments.DecaySec`/`PeakSec`/`TotalAudibleSec`. Tristimulus turned out
  to be the second most predictive axis in the battery.

Two tests now make this class of bug loud instead of silent:

- `TestDeclaredModuleValuesAreLive` (`module_liveness_test.go`; it replaced the
  seed-era `TestSeedParamsAreLiveInTheirOwnInstrumentContext` when instruments
  became documents) requires every DECLARED module value to change the render of
  at least one shipped document of its type, **in that document's own
  configuration**. (`TestRenderModularP_EveryParamMutatesInContext` checks from
  the default sine voice, which is why it never caught the unison bug — unison
  genuinely works there.) A value no shipped document moves is listed with a
  reason in `internal/audio/testdata/dead_module_values.txt` (ratchet: a listed
  entry that turns live fails too) — so a NEW value that no document uses yet
  needs a line there, as `osc.brass/reed/flute.*` have. The older
  `known_dead_seed_params.txt` is read by nothing any more.
- `TestNoUnreadMetricFields` parses the package and fails when an analyzer
  computes an exported field nothing outside its own file reads, with an
  allowlist that requires a written reason.

## Key Learnings

### Body decay must be faster than noise tail (Snare)
When the pitched body rings as long as or longer than the noise, the snare sounds "tonal" or "tom-like" instead of crisp. The body provides initial **punch and weight**, not sustain. Noise should carry most of the perceived tail.

### Saturation creates metallic character from sines (Rimshot)
Heavy `tanh` saturation (drive 4.0) on inharmonic sinusoids produces metallic crunch without noise. Saturation generates intermodulation products. Combined with HP filtering (400 Hz), creates raw crunchy metallic hits purely from tones + saturation.

### Pitch sweeps create "laser gun" artifacts (Sidestick)
Exponential pitch sweeps on short percussive sounds create "pew pew" character. For click-like instruments, use **fixed frequencies only**. Click character comes from fast amplitude decay, not frequency movement.

### High bandpass Q creates synthetic ringiness (Sidestick)
Q values of 1.5+ on bandpass noise make short sounds ringy and synthetic. For woody/natural character, keep Q at 0.7 or below.

### Anti-pattern: aggressive post-processing lowpass
Low-pass filters (260-320 Hz) on toms destroy transient character. Keep natural brightness; use gentle saturation instead.

### Modulator envelope decay defines FM timbral evolution
Modulator decay 3-5x faster than carrier creates "bright attack fading to warm sustain" — signature of plucked strings, electric pianos, DX7 patches. Equal rates create static organ tones.

### Non-integer frequency ratios create metallic character (Bell)
Integer ratios (1:1, 1:2, 1:3) = harmonic/tonal. Non-integer (1:3.5, 1:7.1) = metallic/inharmonic. Further from integer = more metallic.

### Series modulation chains multiply spectral complexity (Lead)
Modulators in series: each stage multiplies complexity. Two at depth 2.0 each create far more sidebands than one at depth 4.0. Cascade depths of 2.0-3.5 per stage usually sufficient.

### FM vs subtractive for percussion
FM excels at tonal/pitched instruments. Drums, cymbals, shakers get character from noise shaped by filters and envelopes. FM can approximate via high mod indices but tends to sound "synthetic 80s."

### Enhanced Tom Guidance (808-style)

| Tom | Frequency Range | Decay Time |
|-----|-----------------|------------|
| High | 165-220 Hz | ~100ms |
| Mid | 120-160 Hz | ~130ms |
| Low | 80-100 Hz | ~200ms |

Key 808 characteristics: bridged-T oscillator for resonant attack, diode-controlled pitch sweep, subtle pink noise for room ambience.

### Tin-like / Can Cowbell Variant
Distinct from natural cowbell: inharmonic partials clustered higher and tighter, shorter/steeper decay on lowest partials, mid-focused and louder impact noise. Works as digital percussion color for syncopated accents.

### String-like / Plucked Synthesis (legacy cowbell variant)
Two inharmonic sines (~560/840 Hz) with mild detune, moderate decay (exp(-14t)), short mid-focused noise impact, soft saturation. Useful reference for future string-like synthesis.

---

## Pattern & Feel (UI/JSON side)

- Avoid firing hats on **every subdivision** at high tempos. Drive closed hats mostly on 8ths with accents and probabilities, not every 16th. Let open hats sit on off-beats with longer durations and lower probabilities.
- For snares, keep **kick heavy and snare strong but not constant**; use `logic_kind: "probability"` / `every_n_triggers` on ghost notes and occasional accents.

When adjusting drum DSP, test both:
- A simple 1-2 bar groove for feel (rock backbeat, basic hat pattern).
- A more exposed pattern (single hits in isolation) to judge tails, brightness, and "plasticky versus organic" character.

---

## Showcase Templates — Instrument Configuration

> **Restructured (2026, branch `add-core-node-types`).** The old
> seven generic-genre templates (rock/hip-hop/pop/funk/salsa/house/techno)
> were **replaced** by **30 song/piece showcase templates** — each
> recreates the instrumental signature of one genre-defining masterpiece
> (Bach Toccata, Vivaldi Spring, Mozart K545, Hotel California, Africa,
> So What, Ran Kan Kan, Superstition, Can You Feel It, G Thang, Strings
> of Life, Exodus, Good Times, Girl from Ipanema, The Thrill Is Gone …)
> plus a set of single-instrument demo templates (`air-*`, `bach-*`,
> `felt-prelude`, `sax-blues`, `steel-folk`, `electric-riff`,
> `clav-funk`, `bass-groove`, `conga-tumbao`).

> **SUPERSEDED 2026-09-02.** The template library described above no longer
> exists. Every template recreating a copyrighted song was removed, and the
> library was rebuilt from committed public-domain scores. See
> `docs/superpowers/specs/2026-09-02-public-domain-template-curation-design.md`
> and `docs/score-sources.md`. The section below describes the current
> pipeline; the paragraphs above are historical.

**Where it lives now** (the old `internal/templates/genres.go` is gone):

- **Reference scores**: `internal/scoreref/` — a committed corpus of CC0
  MusicXML plus `catalogue.json` recording each score's work, licence,
  upstream URL and SHA256. Admission is gated on three tests (the work is
  public domain, the encoding is permissively licensed, the provenance is
  verifiable). `docs/score-sources.md` surveys which corpora qualify.
- **Spec table**: `src/go/cmd/gen-showcase/template_specs.go` — one row per
  template: a score id, a measure range, and a mapping from score parts to
  Beatmo instruments. Purely declarative; there is no per-piece code.
- **Pipeline**: `cmd/gen-showcase/score_import.go` — parse → trim to the
  measure range → merge ties → split chords into lanes → map dynamics to
  per-hit volume → map articulations and rit./accel. to groove drift.
- **Generator**: `make templates` (or `go run ./cmd/gen-showcase` from
  `src/go`).
- **Output**: `internal/assets/templates/<stem>.json` (one per template).
- **Menu order + titles**: `internal/assets/templates.go` (`templateGroups`,
  `displayTitles`), cross-checked against the spec table by
  `TestTemplateOrderMatchesAssets`.
- **Guards**: `drift_test.go` (`TestDrift`) rebuilds every spec and fails if
  the committed JSON differs; `fidelity_test.go` checks each circuit against
  its score — every pitch must occur in the trimmed range, notes must
  reconcile (emitted + dropped == score notes), tempo must be the score's
  metronome mark, and volume/groove must appear only where the notation asks.

**There is no hand-authored musical content.** A template's notes are a pure
function of its score, which is what lets the fidelity tests treat the score
as an oracle rather than as documentation. Do not add a note, a volume or a
groove by hand — it will fail `TestTemplateIsDerivedFromScore`.

Each template **selects + tunes existing builtins** — it never adds a C
recipe. The levers set per row in `template_specs.go` are the instrument id,
the octave shift, `MaxVoices` (how many chord lanes the part is split
across), and the per-row mix: `Volume`. Per-node pitch, duration, volume and groove all come from the score.

To change a template: edit its row in `cmd/gen-showcase/template_specs.go`,
run `make templates`, and commit the regenerated JSON (the drift test
enforces it). To add one: catalogue the score in `internal/scoreref` first.
