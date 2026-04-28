## Synth Pipeline: Instrument Recipes

Reference for creating and modifying drum and FM instrument sounds. For the audio pipeline architecture, see `AGENTS.synth.pipeline.md`.

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

All drum renderers live in `src/c/drums.c` with Go CGo bindings in `src/go/internal/audio/drums_c.go`. Each follows the pattern `render_<name>(float *out, int sampleRate, int samples)`. Parameterized variants use `render_<name>_p(float *out, int sr, int samples, synth_params *p)`.

### Snare (noise-led)

**C function**: `render_snare` / `render_snare_p`

- **Body (tone)**:
  - Two decaying sines around low snare fundamentals (~200 Hz and ~330 Hz), with a tiny per-hit detune.
  - Apply a **very gentle downward pitch drift** across the hit for realism, but keep the body relatively quiet in the final mix so the snare doesn't turn into a synth-bass.
  - Decay should be **shorter than the noise tail**: enough ring to feel like a drum, but most of the length should come from noise.

- **Noise (head + wires)**:
  - One white-noise source, split via biquads into:
    - A **low band** (~350 Hz) for the head "paper" / thud.
    - One or more **mid bands** (~1.8-3.2 kHz) for snare wires.
    - An optional **higher band** (~4.5 kHz) for crispness; keep this subtle to avoid clappy, brittle tails.
  - Shape the bands with separate envelopes:
    - Low band: slower decay for weight (short thud, not a drone).
    - Mid bands: very fast attack (crack) plus a shorter tail; this tail sets perceived "ring" more than the pitched body.
  - LFO modulation (~13 Hz) on the wire bands adds organic complexity.

- **Attack & mix**:
  - Moderate front-end gain boost on the mid/high bands for the first few ms; keep it moderate so it doesn't sound like a clap.
  - Aim for a **noise-led mix**: low band ~1.1x, wires ~0.9x, body ~0.4x.
  - Gentle global fade (based on normalized index) to keep the overall length sensible while still allowing ring.

- **`_p()` parameters**: `pitch` (re-index via playback rate), `decay` (multiply envelope constants), `drive` (saturation via `sp_saturate()`), `tone` (one-pole LP/HP based on sign).

### Rimshot (tone-led)

**C function**: `render_snare_rimshot`

- **Architecture**: 808-inspired inharmonic resonator — **tone-led with heavy saturation**, fundamentally different from the snare.
  - **NOT a modified snare**: rimshots get their character from inharmonic ringing tones and saturation, not noise bands.

- **Tone (4 inharmonic partials)**:
  - Fixed frequencies at ratios 1 : 2.1 : 3.4 : 5.6 (500 / 1050 / 1700 / 2800 Hz x detune).
  - **No pitch drift** — fixed frequencies are critical. Pitch sweeps make it sound like a tom.
  - Gains taper with frequency: 1.0, 0.95, 0.8, 0.5.
  - Exponential decays get faster for higher partials: exp(-40t), exp(-50t), exp(-65t), exp(-90t).

- **Transient**: Brief HP noise crack (~5ms). White noise minus 1st-order LP at 5 kHz. Amplitude 0.7, decay exp(-200t). Provides the initial "stick hit" without dominating.

- **Attack spike**: `1.0 + 2.0 * exp(-1500t)` — boosts the first ~1ms by 3x for hard impact.

- **Signal chain**: (tones + transient) x attack → 1st-order HP at 400 Hz → `tanh(x * 4.0)` → `softsat()`.
  - The **high HP cutoff (400 Hz)** strips all warmth/body, leaving only metallic ring.
  - The **hard drive (4.0)** generates dense harmonics from the sinusoids, creating the raw crunchy character.
  - The **double saturation** (tanh then softsat) is what makes it sound metallic rather than clean.

- **Buffer duration**: 0.3s (signal is essentially silent after ~100ms).

- **Design notes**:
  - To make it crunchier: increase tanh drive, raise HP cutoff, add higher partials.
  - To make it more tonal/bell-like: reduce drive, lower HP cutoff, remove the 4th partial.
  - Never add biquad noise bands or pitch drift — these push it back toward snare/tom territory.

### Sidestick (tone-led)

**C function**: `render_snare_sidestick`

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

**C function**: `render_kick` / `render_kick_p`

- **Body (tone)**:
  - Built from a **harmonic sine stack** anchored around ~55 Hz:
    - Fundamental (`f0`) carries most of the weight.
    - 2nd harmonic (`2*f0`) adds low-mid punch.
    - 3rd harmonic (`3*f0`) adds a small amount of "knock" at the front.
    - 4th harmonic (`4*f0`) provides audibility on small speakers.
  - All partials share a **small, fast pitch envelope** (~+10% at onset, decaying in a few ms) so the kick has punch without a "laser" sweep.
  - Amplitude envelopes are exponential and **shorter for higher harmonics** so the tail becomes round and deep.

- **Dark noise thud (rawness)**:
  - A single white-noise source is **heavily low-passed** (one-pole, 0.97/0.03) so only low/mid "thud" remains.
  - Short exponential envelope (exp(-16t)), mixed quietly (0.18).

- **Beater click**:
  - HP-filtered noise burst (~2-3ms) for sharp transient audible on any speaker.
  - Uses separate noise + 1-pole HP (alpha 0.85), decay exp(-120t), amplitude 0.35.

- **Attack shaping**:
  - Front-loaded gain boost (`1.0 + 0.3 * exp(-40t)`) on the tonal stack.

- **`_p()` parameters**: `pitch` (re-index), `decay` (multiply envelope constants), `drive` (saturation), `body` (low-frequency resonance boost via LP mix at 120 Hz).

- **Design notes**:
  - Keep the **fundamental dominant**, with harmonics and noise clearly subordinate.
  - Use **tiny pitch envelopes** on the tone (<= 10-15%) for punch; larger sweeps read as electronic/industrial.
  - Noise should be: strongly low-passed, very short, mixed quietly.
  - For more aggression: slightly stronger 2nd harmonic and a bit more softsat drive, or faster pitch envelope.

### Kick-Deep (tone-led)

**C function**: `render_kick_deep`

- **Architecture**: 808-style deep sub kick — lower, boomier, longer sustain than the standard kick.
- **Body**: Only fundamental (~42 Hz) and 2nd harmonic, starting at Pi/2 phase for maximum punch on first sample.
- **Pitch envelope**: Wider (+15%) and slower (exp(-15t)) than standard kick for deeper, rounder punch.
- **Noise**: Very quiet low-passed thud (0.10 amplitude), shorter decay.
- **Saturation**: Gentle — `tanh(mixed * 1.2) * 0.95` — cleaner than standard kick for a pure sub feel.
- **Buffer duration**: 0.8s (long sustain is the point).

### Kick-Punchy (tone-led)

**C function**: `render_kick_punchy`

- **Architecture**: 909-style electronic kick — short, snappy, cuts through a mix.
- **Body**: Higher fundamental (62 Hz) with only 2 harmonics (f0 + 2*f0) for a clean electronic character.
- **Pitch envelope**: Aggressive +25% sweep with fast decay (exp(-55t)) — gives signature "thwack".
- **Beater click**: Louder (0.45 vs standard's 0.35), brighter HP filter (alpha 0.80) for snappy transient. No noise thud.
- **Attack**: Stronger boost — `1.0 + 0.5 * exp(-60t)`.
- **Saturation**: More aggressive — `softsat(mixed * 0.8) * 1.3` for density.
- **Global fade**: exp(-5.0 * tNorm) (faster cutoff than standard kick).
- **Buffer duration**: 0.3s.

### Kick-Lofi (tone-led)

**C function**: `render_kick_lofi`

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

**C function**: `render_kick_tight`

- **Architecture**: Acoustic/studio kick — natural, controlled, defined.
- **Body**: Medium fundamental (58 Hz) with 3 harmonics (58/116/174 Hz) and fast decay.
- **Pitch envelope**: Minimal +5%, ultra-fast exp(-65t) — barely audible, just adds snap.
- **Beater transient**: RBJ biquad bandpass at 2.5 kHz (Q=0.7) on noise — gives natural "beater on drumhead" sound.
- **Room thump**: Brief bandpass noise at 200 Hz (Q=0.5) with very fast decay (exp(-80t)) for short "room" character.
- **Built-in gate**: Additional envelope multiplier that goes to 0 after ~45% of buffer for tight cutoff.
- **Saturation**: Minimal — `softsat(mixed * 0.45) * 1.1` — clean, natural.
- **Global fade**: exp(-4.5 * tNorm).
- **Buffer duration**: 0.3s.
- **Design notes**: The bandpass beater transient (instead of HP noise) is what gives this variant its natural "acoustic" character. The gate prevents the tail from ringing, making it sit cleanly in a pop/rock mix.

### Hi-Hat Closed (noise-led)

**C function**: `render_hihat` / `render_hihat_p`

- **808-style cluster of 6 inharmonic square oscillators** in the upper mids/highs (~4.1-11.8 kHz), with slight per-hit detune and random initial phases.
- Sum the cluster, then **high-pass** it to remove low hum and DC; add a high-passed white-noise layer for extra sizzle.
- LFO wobble (~10-14 Hz per partial) avoids static ringing.
- Envelopes: fast attack (exp(-180t)) + longer metallic tail (exp(-35t)).
- **`_p()` parameters**: `decay` (multiply by exp(-20t*(1/decay - 1))), `brightness` (HP filter at 4000-12000 Hz, mixed at 0.5), `drive` (saturation).

### Hi-Hat Open (noise-led)

**C function**: `render_open_hihat`

- Reuses the same source and filters as closed hat with:
  - Longer cluster and noise envelopes (tail exp(-22t) for cluster, exp(-18t) for noise).
  - Slightly less detune (0.015 vs 0.02) and flatter gain profile for more shimmer.
  - Noise more prominent (0.9x vs 0.45x) for sizzly feel.

### Tom — Standard (tone-led)

**C function**: `render_tom` / `render_tom_p`

- **808-style** with resonant attack, warm body decay, and subtle room ambience.
- **Body (tone)**:
  - Fundamental (~150 Hz with detune) plus two overtones at 1.5x and 2.1x ratios.
  - Fast 808-style pitch sweep (exp(-18t)) from 1.25x base to 0.85x base for punch.
  - Longer decay than snare body: fundamental ~exp(-2.8t), overtones slightly faster.
- **Stick impact**: Mid-band focused via 2-pole LP + HP (bandpass-like), longer envelope (exp(-120t)) for presence.
- **Ambient noise**: Very quiet, long-decay (exp(-6t)) low-passed noise for room feel.
- **Resonant attack boost**: `1.0 + 0.4 * exp(-80t)` for bright transient that fades to warm body.
- **`_p()` parameters**: `pitch` (re-index), `decay` (multiply by exp(-8t*(1/decay-1))), `drive` (saturation).

### Tom High (tone-led)

**C function**: `render_tom_high`

- Base pitch 170 Hz (brighter), wider sweep (Start*1.3, End*0.88), faster pitch decay (exp(-22t)).
- Faster envelopes: exp(-3.5t), exp(-5.0t), exp(-6.5t). Stronger resonant boost: exp(-90t)*0.5.

### Tom Low (tone-led)

**C function**: `render_tom_low`

- Base pitch 90 Hz (deeper), gentler sweep (Start*1.2, End*0.82), slower pitch decay (exp(-14t)).
- Longer envelopes: exp(-2.2t), exp(-3.2t), exp(-4.5t). Longer ambient tail: exp(-4.0t).

### Clap (noise-led)

**C function**: `render_clap` / `render_clap_p`

- Several tightly spaced **noise bursts** (white noise) at staggered times (~0, 20, 40 ms, plus a slightly later echo at ~75ms) to approximate multiple hands clapping and a tiny bit of room.
- A shared exponential decay envelope (exp(-7t)) controls the overall tail.
- Everything is noise-led; there is no pitched component.
- **`_p()` parameters**: `decay` (multiply by exp(-10t*(1/decay-1))), `drive` (saturation).

### Cowbell (tone-led)

**C function**: `render_cowbell` / `render_cowbell_p`

- **Body**: 4 inharmonic sine partials (~640, 920, 1300, 1900 Hz), per-hit detuned. Gains taper: 1.0, 0.85, 0.6, 0.4. Slow decay (exp(-9t)) gives clear metallic ring.
- **Impact**: Short, low-passed noise burst (exp(-260t)) for stick contact.
- **Metal rasp**: Subtle high-passed noise tail (exp(-60t)) at low amplitude (0.25).
- **`_p()` parameters**: `pitch` (re-index), `decay` (multiply by exp(-12t*(1/decay-1))).

### Shaker (noise-led)

**C function**: `render_shaker`

- **Architecture**: Noise-based with grain-like micro-bursts for rhythmic texture.
- **4 staggered micro-bursts** at ~0, 4, 9, 15ms with tapering amplitudes (1.0, 0.85, 0.7, 0.5) and per-hit timing variation.
- **Spectral shaping**: HP noise (1-pole at ~5 kHz) + RBJ bandpass at 8 kHz (Q=0.7) for shimmer.
- **Overall envelope**: exp(-25t) keeps it short and crisp.

### Ride (tone-led)

**C function**: `render_ride`

- **Architecture**: Bright metallic ring with bell-like tone — 10 sine partials (3.1-15 kHz) + bell component + noise wash.
- **Partials**: Sine-based (softer than hi-hat's squares), tapered gains, per-hit detune and phase randomization.
- **Bell**: Extra sine at ~3 kHz with slow decay (exp(-6t)) for clear ping.
- **Envelope**: Fast initial (exp(-40t)) + long tail (exp(-8t)) for sustained ring.
- **LFO**: Organic shimmer at ~7 Hz.
- **Buffer duration**: 1.0s.

### Crash (tone-led)

**C function**: `render_crash`

- **Architecture**: Darker, washy cousin of the ride — 8 sine partials (2-11.5 kHz) + bell + noise wash.
- **Key differences from ride**:
  - Darker frequency range (starts at 2 kHz vs ride's 3.1 kHz).
  - Tapered gains emphasize mid partials (3-6 kHz), reducing lowest and highest.
  - Much longer decay: cluster tail exp(-3t), noise exp(-8t).
  - Darker bell (~2.5 kHz vs ride's 3 kHz).
- **Buffer duration**: 1.5s.

### Bass Guitar (tone-led)

**C function**: `render_bass_guitar`

- **Architecture**: Karplus-Strong plucked string synthesis.
- **Delay line**: Buffer size = sampleRate / 55 Hz, initialized with LP-filtered noise (alpha 0.35) for soft pluck character.
- **Loop filter**: `0.5 * (y[n] + y[n-1]) * decayFactor` with probabilistic stretch (50% chance to skip filter per sample, preserving high harmonics longer).
- **Decay factor**: 0.996 for sustain appropriate to bass.
- **Attack transient**: ~8ms noise burst for finger/pick sound.
- **Buffer duration**: 1.5s.

### Sub-Bass (tone-led)

**C function**: `render_sub_bass`

- **Architecture**: 808-style deep sine with pitch envelope.
- **Fundamental**: ~45 Hz, starting at Pi/2 phase for maximum attack punch.
- **Pitch envelope**: `freq * (1.0 + 0.15 * exp(-40t))` — 808-style punch that quickly settles.
- **2nd harmonic**: Very subtle (0.08 mix) for presence on small speakers.
- **Amplitude**: Slow decay (exp(-2t)) for long, clean sustain. Slight attack boost (1.0 + 0.2 * exp(-60t)).
- **Saturation**: Clean — `tanh(mixed * 1.2) * 0.95`, lower drive than drums for pure sub.
- **Buffer duration**: 2.0s.

---

## FM Instruments

The FM synth engine (`src/c/fmsynth.c`) produces tonal, pitched instruments. Go CGo bindings in `src/go/internal/audio/fmsynth_c.go`. All renderers follow `render_fm_<name>(float *out, int sr, int samples)`.

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
     a. Compute frequency = base_freq * freq_ratio * pitch_mult + freq_offset
     b. Compute ADSR envelope
     c. Advance phase accumulator
     d. output = wavetable_lookup(phase + mod_input) * amplitude * envelope
  4. Sum carrier outputs → softsat() → output buffer
```

**ADSR envelope**: Standard 4-stage. Auto-release is triggered at `duration - release_time` so sounds fade cleanly within their buffer.

**Pitch envelope**: `2^(amount * exp(-t/decay) / 12)` — exponential sweep in semitones. Positive values sweep down from above.

**Phase modulation**: Added to the sine argument directly, not to the phase accumulator. Preserves modulator's effect without permanently drifting carrier pitch.

**Wavetable**: Static 4096-sample sine table shared across all operators. `wt_osc_tick_pm()` for per-sample lookup with phase modulation.

### FM Bass (`fm-bass`)

- **Architecture**: 2-operator (carrier + modulator), both at 1:1 frequency ratio.
- **Base frequency**: 55 Hz (A1).
- **Modulation**: op1 → op0, depth 2.5. The 1:1 ratio with moderate depth creates a warm, slightly overdriven bass tone. As the modulator's envelope decays, the sound goes from bright to warm.
- **Carrier envelope**: 5ms attack, 300ms decay, sustain 0.6, release 200ms.
- **Modulator envelope**: 1ms attack, 150ms decay, sustain 0.2, release 100ms. Fast mod decay gives "pluck" character.
- **Pitch envelope**: +3 semitones, 60ms decay.
- **Buffer duration**: 1.5 beats.
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
- **Buffer duration**: 2.0 beats.
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
- **Buffer duration**: 1.0 beats.
- **Design notes**:
  - Tame brightness: reduce op2 amplitude or decay.
  - More aggressive: increase depths or add self-modulation (mod_matrix[0][0] > 0).
  - Softer: switch to parallel topology instead of series.

### FM E-Piano (`fm-epiano`)

- **Architecture**: 3-operator with paired carriers. op1 modulates op0 (main tone); op2 is independent carrier at 2x for octave shimmer. DX7 Rhodes inspiration.
- **Base frequency**: 261.63 Hz (C4).
- **Modulation**: op1 → op0, depth 1.8. Rhodes character from tine decaying faster than tonewheel.
- **op0 (carrier)**: 2ms attack, 800ms decay, sustain 0.3, release 400ms.
- **op1 (modulator)**: 1ms attack, 300ms decay, sustain 0.1, release 200ms.
- **op2 (shimmer carrier)**: 2x ratio, amplitude 0.25, 500ms decay.
- **Pitch envelope**: None.
- **Buffer duration**: 2.0 beats.
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
- **Buffer duration**: 0.5 beats.
- **Design notes**:
  - More guitar-like: lower base freq (110-150 Hz), increase mod depth (5.0+).
  - More harp-like: higher base freq (300-500 Hz), lower mod depth (2.0).
  - Compare with `render_bass_guitar` (Karplus-Strong) — FM pluck is brighter/synthetic, K-S is warmer/acoustic.

### FM Variant Instruments (Post-Processed)

Each base FM instrument has a `-1` variant with Go-side post-processing. C render is identical; only buffer duration and post-processing differ.

| Variant | Base | Duration | Post-Processing | Character |
|---------|------|----------|-----------------|-----------|
| `fm-bass-1` | fm-bass | 1.0 beats | HP 60 Hz + soft clip 1.8x | Tighter, more aggressive, cuts sub rumble |
| `fm-bell-1` | fm-bell | 1.5 beats | LP 4000 Hz | Darker, muted bell |
| `fm-lead-1` | fm-lead | 0.7 beats | 5-bit crush + gate tail 60% | Lo-fi, digital, retro |
| `fm-epiano-1` | fm-epiano | 1.5 beats | LP 3000 Hz + soft clip 1.4x | Warmer, vintage |
| `fm-pluck-1` | fm-pluck | 0.3 beats | HP 150 Hz + gate tail 50% | Tighter, thinner, percussive |

### Creating New FM Presets

1. Define preset as `static const fm_preset` in `fmsynth.c`.
2. Write render wrapper: `EXPORT void render_fm_xxx(float *out, int sr, int samples) { fm_render(&PRESET, out, sr, samples); }`
3. Declare in `fmsynth.h`.
4. **Makefile**: Append `_render_fm_xxx` to `EXPORTED_FUNCTIONS`.
5. **CGo bridge**: Add wrapper in `fmsynth_c.go`.
6. **Register**: Add to `engine_instruments.go`, all `config*.go` files, and `audio.js` (both `RENDER` and `RENDER_INFO`).
7. **Regenerate IDs**: `cd src/go/internal/audio && go run gen_instrument_ids.go`.

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

## DSP Primitive Modules

Reusable C building blocks for synthesis and modulation. Used internally by `fmsynth.c`, `insert_fx.c`, and the LFO module. Available for any new C synth code.

### Wavetable Oscillator (`src/c/wavetable.c`)

Band-limited wavetable oscillator with guard-point interpolation.

**Core types**:
- `wavetable_t` — Table of `length` samples + 1 guard point (`table[length] == table[0]`), eliminating the branch in linear interpolation.
- `wt_osc_t` — Phase accumulator oscillator referencing a `wavetable_t`.

**Waveform generation** (via Fourier additive synthesis, band-limited):
- `wt_generate_sine(wt, buf, length)`, `wt_generate_saw(wt, buf, length, harmonics)`, `wt_generate_square(wt, buf, length, harmonics)`, `wt_generate_triangle(wt, buf, length, harmonics)`

**Oscillator API**:
- `wt_osc_init(osc, wt, freq, sr)` / `wt_osc_set_freq(osc, freq, sr)` — Phase-preserving frequency set
- `wt_osc_process(osc, out, N)` — Block render
- `wt_osc_process_fm(osc, freq_mod, out, N, sr)` — Block render with per-sample FM
- `wt_osc_tick(osc)` / `wt_osc_tick_pm(osc, phase_mod)` — Single-sample inline

Default table size: `WT_DEFAULT_LENGTH = 4096` gives <0.01% THD. Buffers must be at least `length + 1` floats.

### ADSR Envelope (`src/c/adsr.c`)

State-machine ADSR with linear and exponential curve options.

**Stages**: `ADSR_IDLE → ADSR_ATTACK → ADSR_DECAY → ADSR_SUSTAIN → ADSR_RELEASE → ADSR_IDLE`

**API**:
- `adsr_init(env, sr, attack_sec, decay_sec, sustain_level, release_sec, exponential)`
- `adsr_trigger(env)` — Note-on (starts from current value for legato)
- `adsr_release(env)` — Note-off
- `adsr_process(env, out, N)` — Block render
- `adsr_tick(env)` — Single-sample inline

Exponential mode: `value += rate * (target - value)` for natural curves.

### LFO (`src/c/lfo.c`)

General-purpose low-frequency oscillator using the wavetable module.

**Shapes**: `LFO_SINE`, `LFO_TRIANGLE`, `LFO_SAW`, `LFO_SQUARE`, `LFO_RANDOM_SH`

**API**:
- `lfo_init(lfo, sr, shape, rate_hz, depth, center, table_buf)` — `table_buf` NULL for sine (uses static global), else `WT_DEFAULT_LENGTH+1` floats
- `lfo_process(lfo, out, N)` — Block render: `center ± depth * oscillator`
- `lfo_modulate(lfo, buf, N)` — Multiply existing buffer by LFO (for AM/tremolo)

### Synth Parameters (`src/c/synth_params.h`)

8-knob parameter struct for customizing C renderers:

```c
typedef struct {
    float pitch;       // Semitones offset (0 = default)
    float decay;       // Decay multiplier (1.0 = default)
    float tone;        // Brightness: -1 dark, 0 default, 1 bright
    float attack;      // Attack multiplier
    float drive;       // Saturation amount (0 = none, 1 = heavy)
    float body;        // Low-frequency resonance (0 = default, 1 = max)
    float color;       // Timbral character (-1..1, instrument-specific)
    float brightness;  // High-frequency content (0 = default, 1 = max)
} synth_params;
```

Helper functions: `sp_freq(base, p)` = `base * 2^(pitch/12)`, `sp_env_decay(base, p)` = `base * decay`, `sp_filter_cutoff(base, p)` = `base * 2^tone`, `sp_saturate(x, p)` = `tanh(x*(1+3*drive)) / tanh(1+3*drive)`.

### Stereo Panning (`src/c/pan.c`)

Equal-power pan law: `gain_l = cos(angle)`, `gain_r = sin(angle)` where `angle = (pan + 1) * π/4`.

- `pan_set(pan_t*, pan)` — Set position (-1 left, 0 center, +1 right)
- `pan_process_mono_to_lr(pan, in, left, right, N)` — Accumulate into separate L/R
- `pan_process_mono_to_interleaved(pan, in, out, N)` — Write interleaved stereo

---

## Creating New Instruments — Checklist

### Percussion (C noise/tone synthesis in `drums.c`)

1. Write renderer: `render_myinst(float *out, int sr, int samples)` in `drums.c`. Optional `_p()` variant with `synth_params*`.
2. Declare in `drums.h`.
3. **Makefile**: Append `_render_myinst` to Emscripten `EXPORTED_FUNCTIONS`.
4. **CGo bridge**: Add wrapper in `drums_c.go`.
5. **Register**: Add to `engine_instruments.go` (desktop config), `audio.js` (`RENDER` + `RENDER_INFO`), all `config*.go` files.
6. **Regenerate IDs**: `cd src/go/internal/audio && go run gen_instrument_ids.go`.
7. **Test**: Fast Go tests + browser `drums_consistency.browser.test.js`.

### Using DSP Primitives in New C Instruments

Prefer reusable modules over inline implementations:
- **Oscillators**: Use `wt_osc_t` with `wt_generate_*()` instead of `sin()`. Initialize a static wavetable once, share across instruments.
- **Envelopes**: Use `adsr_t` for proper ADSR stages. For simple exponential decays, inline `exp()` is still fine.
- **Modulation**: Use `lfo_t` for periodic modulation (vibrato, tremolo, filter sweep). S&H shape useful for random per-cycle variation.
- **Panning**: Use `pan_t` for stereo placement of individual outputs.

---

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
