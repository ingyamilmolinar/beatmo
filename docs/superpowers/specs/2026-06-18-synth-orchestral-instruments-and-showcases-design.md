# Design: Synthesized Orchestral/Band Instruments + Genre Showcase Circuits

**Date:** 2026-06-18
**Branch:** add-core-node-types
**Status:** Approved design (pending written-spec review → writing-plans)

---

## 1. Goal

Add a family of realistic, **100%-synthesized** melodic instruments — guitars (acoustic
nylon/steel, electric clean), bowed strings (violin, cello), acoustic piano, and winds
(flute, oboe, french horn, trumpet) — plus ~20 recognizable, genre-defining **showcase
circuits** that feature them. Everything is authored as tunable configuration so the
sound can be iterated collaboratively ("tweak them together so they sound better and
better") and the resulting knowledge persisted in docs + memory.

Non-goals (v1): sampled (PCM) instruments; a full physically-modeled string/wind engine;
per-note expression automation beyond what the node graph already supports.

---

## 2. Architecture Decision (settled during brainstorming)

**Every new instrument is a modular "seed"** — a `RecipeParams` (`map[string]float64`)
preset of the existing general **modular voice** (`src/c/modular.c` /
`src/c/modular_stages.c`), registered exactly like the existing `synth-modular-pad`
(`modularPadSeed`). The modular engine is feature-complete: OSC · FM (4-op, mod-matrix) ·
12-slot GEN bank (incl. Karplus-Strong, noise) · PITCH-ENV · LFO · BURST · ADSR · FILTER ·
POST. New timbres are parameter configurations, not new C renderers.

**Per-instrument wiring is ~6 mechanical one-liners** (see §6). The browser renders every
modular seed **generically through `render_modular_p`** — no new render functions,
worklets, or per-instrument DSP in JS.

This was chosen over (a) pure-JSON-only seeds (no named categories / picker entries) and
(b) writing bespoke C renderers per instrument (slow iteration, heavy wiring). The seed
maps are the real authoring work — and exactly the surface we tune together.

### Rollout: all families at once
Per the brainstorming decision, all instrument families and all showcase circuits are
authored in a single batch (not phased family-by-family), with the engine change landing
first. §10 gives a sensible build order within that single batch.

---

## 3. The One Engine Change: LFO Routing (→ true vibrato)

**Problem:** the modular LFO is amplitude-only today (post-mix unipolar dip,
`modular.c:270–277`). Convincing bowed strings and winds need **periodic pitch vibrato**
(5–6 Hz), which a post-mix amplitude multiply cannot produce.

**Change:** add an **`lfo_target`** parameter to the modular schema with three values:

| `lfo_target` | Meaning | Where applied | `lfo_depth` interpretation |
|---|---|---|---|
| `0` (default, identity) | Amplitude tremolo | Post-mix (existing code path) | 0–1 dip depth (unchanged) |
| `1` | **Pitch vibrato** | OSC-stage **wavetable** per-sample frequency path (same place pitch-env already recomputes `wt_osc_set_freq`, `modular.c:232–257`) | ± semitones = `lfo_depth` (0.30 → ±0.30 st = ±30¢) |
| `2` | Cutoff sweep | FILTER stage, per-block biquad coefficient recompute | ± octaves = `lfo_depth` (0.5 → ±0.5 oct) |

**LFO phase consistency:** the LFO is `sin(2π·rate·t)` computed from the sample index, so
the pitch/cutoff/amp application points stay phase-coherent without shared mutable state.
The amp path keeps its existing unipolar `1 − depth·(0.5 − 0.5·sin)` formula for
byte-identity; pitch/cutoff use a bipolar `sin` around center.

**Scope note (wavetable OSC path only):** LFO→pitch modulates the **wavetable oscillator**
(`osc_type` 0–3: sine/saw/square/triangle). To make vibrato uniform across every bowed
string and wind, **those instruments are authored as subtractive saw/square + filter
voices** (not FM) — see §4.2/§4.4/§4.5. This deliberately confines the per-sample
frequency change to the two wavetable branches already in `modular.c`, keeping the change
small and the FM path (`fm_render`) untouched. FM-path vibrato and GEN-bank-slot vibrato
(Karplus-Strong, family voices) are out of scope and unnecessary in v1 (plucked guitars
and additive piano need no vibrato; the existing FM presets are unaffected).

**Byte-identity invariant:** `lfo_target` defaults to `0`; all new LFO behavior is gated.
Every existing instrument renders bit-for-bit unchanged. This is mandatory — the modular
engine's discipline tests assert pre-change byte-identity when new params sit at identity.

**Affected files (engine change):**
- `src/c/modular.c` (LFO application; OSC freq path; filter per-block mod), `modular_stages.c` if helpers needed.
- `src/c/modular.h` / param struct + `synth_param_schema.go` (`modularParamSchema`) — add `lfo_target`.
- `src/go/internal/audio/modular_recipe.go` (`ModularSynthParamDefs` — add ParamDef with label/group/range/default; UI exposes it under the `lfo` group).
- Makefile `EXPORTED_FUNCTIONS` unaffected (no new C symbol; `render_modular_p` already exported).
- Tests: extend modular byte-identity + add LFO→pitch/cutoff behavior tests.

### Brass caveat (explicit v1 tradeoff)
Authentic brass "brightens as it gets louder/at attack" (Chowning: FM index — or, in a
subtractive voice, the filter cutoff — tracks the amplitude envelope). v1 brass uses a
**subtractive saw + static low-pass + amp attack-overshoot + light vibrato** (vibrato
chosen over FM-style brightening per the brainstorming decision). A true attack-brightening
**filter envelope** (cutoff rising on attack) is carried as a small, well-bounded **v2**
engine addition (§11) — it would slot in alongside the LFO→cutoff routing.

---

## 4. Instrument Roster (20 instruments, 3 new picker categories)

Categories are added via a single `synthCategory()` switch extension
(`catalog_desktop.go`): `Strings`, `Winds`, `Keys`. (Plain display strings today; promote
to i18n keys only if/when localization is wanted.)

### 4.1 Strings — Plucked  (GEN-bank Karplus-Strong slot, `gen_source=3`)
Knobs: `gen*_ks_sustain` (decay ρ via target T60), `gen*_ks_pluck` (excitation LP α),
plus global FILTER LP for body brightness and excitation character.

| Instrument | Variant | Character target |
|---|---|---|
| `guitar-nylon` | soft | dark loop (B≈0.10–0.20), long-ish excitation LP, T60≈1.0–1.2 s, mellow |
| `guitar-nylon-bright` | bright | B≈0.25–0.30, slightly crisper pluck |
| `guitar-steel` | bright | B≈0.55–0.80, short/crisp excitation, T60≈2.5–3.5 s, ringing |
| `guitar-steel-warm` | warm | B≈0.45–0.55, softened excitation, shorter sustain |
| `guitar-electric` | bridge | B≈0.50–0.60, narrow focused spectrum, T60≈4–6 s, no body |
| `guitar-electric-neck` | neck | B≈0.40–0.50, rounder/mellower |

> Karplus param→tone mapping (from research): brighter ⇒ raise B and/or ρ; mellower ⇒
> lower B + lowpass the excitation; longer sustain ⇒ ρ→1 via `ρ = 0.001^(1/(f0·T60))`
> (steel/electric 0.97–0.999/period, nylon 0.93–0.97); harder attack ⇒ shorter/brighter
> burst. (Pick-position comb = v2, §11.)

### 4.2 Strings — Bowed  (OSC saw + LP filter + LFO→pitch vibrato + bow-noise tap)
Sawtooth bridge-force spectrum via OSC saw (`osc_type=1`); GEN-bank noise tap
(`gen_source=2`, fast-decaying, band-limited) for bow friction; FILTER LP + body formant
via cutoff; LFO `lfo_target=1` for vibrato; small pitch-env scoop. (Subtractive, not FM —
keeps vibrato on the wavetable path per §3.)

| Instrument | Variant | Character target |
|---|---|---|
| `violin` | solo | vibrato 6.0 Hz / ±18¢ (fade-in), HPF ~700 Hz, brilliance ~3 kHz, A≈70/D≈90/S≈0.88/R≈220 ms |
| `violin-ensemble` | ensemble | added ±3 Hz detune layer (osc_detune / 2nd slot), lighter vibrato, softer attack |
| `cello` | solo | vibrato 4.8 Hz / ±13¢, darker formants, A≈120/D≈160/S≈0.90/R≈380 ms, lower register |
| `cello-warm` | warm | feedback ↓ (darker), more low-mid body, longer release |

### 4.3 Keys  (additive sine partials + hammer noise + unison detune)
GEN-bank sine slots for partials 1–6 at amplitudes ≈ 1.0/0.6/0.4/0.25/0.12/0.05, each
with a **faster per-partial decay for higher partials** (`gen*_env_fast_rate` increasing;
rule of thumb T60_n ≈ T60_1/n); a noise-tap hammer click (2–8 ms, fast decay); slight
**±1.5¢ unison detune** (a doubled slot or `osc_detune`) for life; percussive envelope
(attack 1–5 ms, sustain 0, release 100–400 ms).

| Instrument | Variant | Character target |
|---|---|---|
| `piano-grand` | bright | brighter partial balance, crisper hammer, ~T60_1 ≈ 6–10 s feel |
| `piano-felt` | mellow | rolled-off highs, softer hammer, shorter tail (felt/“soft” piano) |

> v1 uses **harmonic** partials. True inharmonic stretch
> (`f_n = n·f0·√(1+B·n²)`, offsets ∝ n³·f0·B) is **v2** (§11) — needs a per-slot fixed-Hz
> offset param. Harmonic partials + per-partial decay + hammer + unison detune still read
> convincingly as piano.

### 4.4 Winds — Brass  (OSC saw + LP filter + amp attack-overshoot + vibrato; subtractive)
Brightness from the LP cutoff; brass "edge" from the saw's rich harmonics; attack bump
from the amp envelope overshoot. (Vibrato-compatible wavetable path per §3.)

| Instrument | Variant | Character target |
|---|---|---|
| `trumpet` | bright | LP ~3–4 kHz, amp A≈20–30 ms w/ ~15% overshoot / S≈0.55, vibrato 5.5–6 Hz/±20¢ (delayed onset) |
| `trumpet-mellow` | mellow | LP ~2.2 kHz, slower attack ~30–40 ms |
| `french-horn` | soft | LP ~1.8 kHz, gentle A≈60–90 ms, vibrato 5 Hz/±10¢, warmer |
| `french-horn-loud` | loud | LP ~2.8 kHz, A≈40 ms, brighter |

### 4.5 Winds — Woodwind  (OSC sine/saw + filter formant + breath-noise band + chiff + vibrato)
| Instrument | Variant | Character target |
|---|---|---|
| `flute` | pure | OSC sine (`osc_type=0`), breath-noise tap BP ~1.5–4 kHz at −12…−18 dB, attack chiff 30–60 ms, vibrato 5–6 Hz (amp ±10% + small ±20¢ pitch) |
| `flute-breathy` | breathy | louder breath band, stronger chiff |
| `oboe` | reedy | OSC saw/square (buzz) + global **band-pass** filter formant ~1400 Hz (nasal), no breath noise, A≈15–40 ms, vibrato 5–6 Hz/±25¢ |
| `oboe-full` | full | higher cutoff/2nd-formant feel, slightly brighter |

Each seed is a `RecipeParams` map (like `modularPadSeed`). The numbers above are tuning
**starting points** from the research (§Appendix A); refining them by ear is the
collaborative loop. Concrete per-instrument seed values are produced during
implementation and recorded in memory as they're dialed in.

---

## 5. Showcase Circuits (~20: 2 per genre)

Recognizable, genre-defining patterns that feature the new instruments, loadable from the
template menu so they can be auditioned and tweaked.

**Copyright posture (baked into the work):**
- **Classical** = note-for-note transcriptions of **public-domain** works only.
- **All copyrighted-era genres** = **original** patterns built from common-property
  progressions, grooves, clave, montuno, and walking-bass conventions — *evocative of /
  in the style of* famous songs (the song references are tuning pointers for us, never
  transcribed toplines). No protected melody is reproduced.

| Genre | Circuit A | Circuit B | Featured new instruments |
|---|---|---|---|
| Rock | Hard-rock power-chord riff (E, 132) | Classic I-IV-V + pentatonic lead (G, 120) | electric guitar |
| Pop | "Four chords" ballad I-V-vi-IV (C, 75) | Upbeat four-on-floor (G, 118) | piano, flute/strings |
| Electronic | Trance 16th arp (Am, 138) | Future-bass maj9/min9 stabs (C, 150 ½-time) | pluck, strings |
| Hip-Hop | Boom-bap, swung, jazzy m7 loop (Dm, 90) | Trap, 808 slides, triplet hats (Fm, 140 ½-time) | piano/Rhodes, horn stab |
| Classical | **Pachelbel Canon in D** | **Beethoven "Ode to Joy"** | violin, cello, piano |
| House | Chicago piano-stab house (Am, 123) | Deep-house min9 Rhodes (Cm, 122) | piano, Rhodes/epiano |
| Techno | Detroit fixed-interval stabs (Cm, 132) | Minimal 303 acid (Am, 135) | detuned brass/synth stab |
| Jazz | Swing ii-V-I bebop (C, 140–180) | Modal Dorian vamp (Dm) | piano, trumpet, (bass) |
| Salsa | Salsa-dura mambo, 3-2 clave (Cm, ~190) | Cha-cha-chá (C, ~120) | piano montuno, trumpet stabs |
| Funk | "On the one" (Em, 106) | Linear 16th groove (E vamp, 100) | guitar chank, trumpet stabs |

Full transcribable pattern cards (BPM/key/step placements/notes per row) are in
**Appendix B**.

### 5.1 Authoring method
Melodic content encodes as **pitched nodes** on per-row square loops where distance =
time: node `pitch` = semitone offset, `duration` = note-length multiplier; chords =
simultaneous nodes across stacked rows. Existing templates already carry melodic lines
(`salsa.json` fm-epiano montuno on pitches 0-4-7-4), so the format is proven.

To avoid hand-placing grid coordinates for ~20 melodic circuits, build a **small Go
circuit-builder helper** that converts pattern-card data (per row: instrument + step→pitch
list + duration/velocity + synth_params) into the square-loop node graph + emits the
committed circuit JSON. This is a deliverable.

> **Open implementation question (verify first):** whether the `internal/templates` Go
> spec→JSON generator (`cmd/gen-templates`, drift test) is live, or whether the committed
> `internal/assets/templates/*.json` are the hand-authored source of truth (the repo
> comment in `internal/assets/templates.go` says the latter). This decides whether the
> circuit-builder helper plugs into the existing generator or stands alone. It affects
> authoring ergonomics only — not feasibility (melodic JSON authoring already works).

### 5.2 Surfacing
Showcase circuits are loadable templates. To avoid crowding the existing 7-genre menu,
present them under a dedicated grouping (e.g. a "Showcases" template section) or extend
the template registry/order; final UI placement decided in the plan.

---

## 6. Per-Instrument Wiring (touchpoints)

For each new modular-seed instrument (confirmed by code exploration):

1. `src/go/internal/audio/engine_instruments.go` — 1 map entry in `ResetInstruments()`
   (`Render: renderModular`, `Beats: <dur>`).
2. `src/go/internal/audio/synth_recipe.go` —
   - `builtinRecipeDescriptors`: 1 descriptor `{ID, Display, Category: modularRecipeCategory, Seed: <seedVar>}` (the seed-bearing recipe), and
   - `builtinInstrumentRecipeBindings`: 1 binding `"id": "<recipe-id>"`.
   - the seed var itself (a `RecipeParams`) lives near `modularPadSeed` (or a new `seeds_instruments.go`).
3. `src/go/internal/audio/config.go` — 1 `InstrumentConfigs` entry (browser parity).
4. `src/js/audio.js` — 1 `RENDER` entry (`'id': 'render_modular'`) + 1 `RENDER_INFO`
   entry (`{ seconds, amp, paramBlock: 'modular' }`).
5. `src/go/internal/audio/catalog_desktop.go` — extend `synthCategory()` once for the new
   category prefixes (Strings/Winds/Keys).
6. `instrument_ids_gen.go` regenerates on build (auto); no manual edit.

No new audio.js render functions/worklets. Default seed params are pushed to the browser
at bootstrap via the existing `SeedInstrumentDefaultsToPlatform` → `seedInstrumentDefaults`
path, so unedited browser plays use the seed (not modular identity).

---

## 7. Components & Boundaries

- **Engine LFO routing** (C + schema): isolated, identity-gated, byte-compat-tested.
- **Instrument seed library** (Go data + wiring): pure data; each seed independently
  authorable/testable; depends only on the modular recipe + LFO routing.
- **Circuit-builder helper** (Go): pure function pattern-card → node-graph JSON; testable
  in isolation (assert node coords, edges close the loop, pitches survive).
- **Showcase circuits** (committed JSON assets): produced by the helper; validated by
  round-trip + drift tests.
- **Docs/memory**: `AGENTS.synth.instruments.md` gains String/Wind/Key/Piano sections;
  memory entries capture tuned seed learnings.

Each unit has one purpose, a well-defined interface, and is understood/changed without
reading the others' internals.

---

## 8. Testing & Validation

- **Engine:** modular byte-identity tests stay green with `lfo_target=0`; new tests for
  LFO→pitch (vibrato frequency/depth in cents) and LFO→cutoff (per-block sweep).
- **Instruments:** existing recipe-registry discipline tests auto-cover new recipes
  (`synth_recipe_discipline_test.go`, `_invariants_`, `_completeness_`, `c_header_parity_`)
  — boundary-preset NaN/Inf sweep, binding completeness, orphan detection. Add a
  per-instrument render-smoke (non-silent, finite, expected duration).
- **Circuits:** circuit-builder unit tests; round-trip import test (row count, hit counts,
  pitches, synth_params survive); template drift test if wired to the generator.
- **Cross-platform parity:** native vs WASM render compare for a representative subset
  (`xplat_audio_compare.browser.test.js` style) — modular path renders identically.
- **Audition:** all showcase circuits load and play on desktop + browser.

Fast Go path for the bulk:
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/... ./internal/assets/...`

---

## 9. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| LFO→pitch touches the hot OSC freq loop | Identity-gated; byte-identity test; OSC-stage only (bounded) |
| Brass realism without index-brightening | v1 = attack overshoot + vibrato (recognizable); index-brightening = scoped v2 |
| Piano without inharmonicity sounds organ-like | Per-partial faster-decaying highs + hammer + unison detune carry it; stretch = v2 |
| 20 circuits = coordinate tedium / errors | Circuit-builder helper + round-trip tests |
| Generator-vs-hand-authored ambiguity | Verify first (§5.1); helper works either way |
| Browser parity for new instruments | Generic `render_modular_p` path (verified); parity test subset |
| Concurrent external churn on this branch (per memory) | Verify own work via `-run` filters; rebuild `make wasm` before size-sensitive checks |

---

## 10. Build Order (single batch)

1. **Engine:** `lfo_target` routing (C + schema + ParamDef) + tests (byte-identity + vibrato/cutoff).
2. **Categories:** `synthCategory()` Strings/Winds/Keys.
3. **Instrument seeds + wiring:** author all 20 seeds + the 6-touchpoint wiring each; render-smoke tests.
4. **Circuit-builder helper:** pattern-card → square-loop JSON + unit tests.
5. **Showcase circuits:** 20 circuits via the helper; round-trip/drift tests; template surfacing.
6. **Audition pass** (desktop + browser) — the start of the collaborative tuning loop.
7. **Docs + memory:** update `AGENTS.synth.instruments.md`; record tuned-seed learnings.

---

## 11. Carried v2 Enhancements (explicit, not hidden)

Each is a small, well-bounded future engine add; v1 ships convincing approximations.

- **Piano inharmonic stretch:** per-slot fixed-Hz offset param (`f_n = n·f0·√(1+B·n²)`).
- **Attack-brightening filter envelope:** cutoff rising on attack (subtractive brass/reed
  "brightens as it gets louder"); slots in alongside the LFO→cutoff routing.
- **Karplus pick-position comb:** `Hβ(z) = 1 − z^(−βN)` excitation comb for nylon/steel/electric pluck nulling.
- **(Optional)** LFO→FM-path vibrato + LFO shape selection + vibrato onset-delay as
  first-class params (v1 uses wavetable-path sine vibrato + envelope-faked onset).

---

## Appendix A — Synthesis Recipe Cards (tuning starting points)

> Sourced engineering values for each family. `c:m` = carrier:modulator ratio; index =
> FM modulation index; ADSR in ms; sustain 0–1; vibrato Hz/±cents.

**Karplus-Strong (guitars):**
`N = fs/f0 − 0.5`; loop gain `ρ = 0.001^(1/(f0·T60))`; one-pole loop filter
`h0=(1+B)/2, h1=(1−B)/4` (B→1 bright, B→0 dark); excitation = N samples white noise,
lowpassed for softer pluck.
- Nylon: B 0.10–0.30, T60 0.8–1.5 s, soft/long LP burst, center pluck (mellow).
- Steel: B 0.55–0.80, T60 2.0–4.0 s, short/bright burst, near-bridge (shimmer).
- Electric clean: B 0.40–0.60, T60 3–8 s (ρ 0.99–0.999/period), smooth burst, no body.

**Bowed (v1 subtractive: OSC saw + LP + vibrato + bow-noise tap):** saw gives the 1/n
sawtooth bridge-force spectrum directly. Violin: saw → LP cutoff ~3–4 kHz (brilliance),
bow-noise tap BP 2–8 kHz (low level, fast decay); A70/D90/S0.88/R220; vibrato 6.0 Hz/±18¢
fade-in; pitch scoop −15¢→+5¢→0 in ~50 ms; optional 2nd saw detuned +3 Hz for ensemble.
Cello: LP ~2 kHz (darker), more low-mid body, noise BP 1.5–6 kHz; A120/D160/S0.90/R380;
vibrato 4.8 Hz/±13¢. (The FM-feedback-saw route from research is a v2 alternative.)

**Piano (additive):** partials 1,2,3,4,6,8 at 1.0/0.6/0.4/0.25/0.12/0.05; per-partial T60
≈ 15/8/4/2.5/1.2/0.7 s (highs faster, ≈ T60_1/n; bass ×2–4, treble ÷3–4); attack 1–5 ms,
sustain 0, release 100–400 ms; hammer = 2–8 ms band-limited noise burst (brighter at
higher velocity); unison 2–3 copies at 0/+1.5/−1.5¢ (~0.3 Hz beat at C4). Inharmonicity
(v2): B ≈ 0.0001–0.0008 mid, rising to ~0.001–0.01 top octaves.

**Brass (v1 subtractive: OSC saw + LP + attack-overshoot + vibrato):** saw harmonics give
the brass edge; LP cutoff sets brightness; amp attack-overshoot gives the "bite." Trumpet:
LP ~3–4 kHz; A20–30 ms w/ ~15% overshoot /D60–80/S0.55/R80–120; vibrato 5.5–6 Hz/±15–25¢,
delayed onset ~0.2 s. French horn: LP ~1.8–2 kHz; A50–90/D~120/S0.45–0.55/R150–250;
vibrato 5–5.5 Hz/±8–15¢. (FM-brass index-tracking + filter-envelope brightening = v2; the
Chowning c:m=1:1/index numbers in the research remain the v2 reference.)

**Flute (OSC sine + breath + chiff):** OSC sine; breath noise tap BP ~1.5–4 kHz at
−12…−18 dB (continuous) + attack chiff (5–10 ms attack → breath floor over 30–60 ms);
A40–80/D60–120/S0.75–0.85/R80–150; vibrato 5–6 Hz, primarily amp ±10% + small pitch ±15–35¢.

**Oboe (OSC saw/square + BP formant):** saw/square buzz → global band-pass filter formant
~1400 Hz (nasal, Q 2–4); optional brighter cutoff for `oboe-full` (2nd-formant feel);
A15–40/D40–90/S0.8–0.9/R60–120; vibrato 5–6 Hz/±20–40¢; no breath noise. (Per-op fixed
formant carrier from research = v2 refinement.)

---

## Appendix B — Showcase Pattern Cards (transcribable)

Grid convention: 16 steps/bar = sixteenths. Step 1=beat1, 5=beat2, 9=beat3, 13=beat4;
offbeat "ands" = 3,7,11,15. Middle C = C4. Copyrighted-era cards are original constructions
from common-property grooves/progressions; classical cards are exact public-domain
transcriptions.

### Rock
- **A — Hard-rock power-chord riff** — E, 132 BPM, 1 bar. Drums: Kick 1,9 · Snare 5,13 ·
  Closed hat 1,3,5,7,9,11,13,15. Guitar (root+5th power chords): 1 E5, 3 E5, 5 E5, 7 G5,
  9 A5, 11 A5, 13 G5, 15 D5. Bass (8ths): E1,E1,E1,G1,A1,A1,G1,D1.
- **B — Classic I-IV-V + pentatonic lead** — G major, 120 BPM, 4 bars. Drums: Kick 1,9 ·
  Snare 5,13 · Hat 8ths. Chords 1/bar I-IV-V-IV: G / C / D / C. Bass: root on 1 & 9, walk
  G1-A1-B1 into C bar 1. Lead (G minor pentatonic) steps 1,4,7,9,13: G4-Bb4-C5-D5-G4.

### Pop
- **A — Four-chords ballad I-V-vi-IV** — C, 75 BPM, 4 bars. Piano pads: C / G / Am / F.
  Bass roots half notes (1 & 9): C2 / G1 / A1 / F1. Drums: Kick 1,9 · soft snare 5,13 ·
  shaker 1,5,9,13. Flute/strings counter (1,5,9,13): C-bar E5-G5-C5-G4 · G-bar D5-B4-D5-A4
  · Am-bar A4-C5-E5-C5 · F-bar C5-A4-F4-C5.
- **B — Upbeat four-chords** — G, 118 BPM, 4 bars. Chords: G / D / Em / C. Bass: 8th pulse
  on root + octave pop on 15. Drums: Kick 1,5,9,13 · Clap 5,13 · hat 3,7,11,15. Lead
  (1,4,7,9,13): G-bar G4-B4-D5-B4-A4 · D-bar A4-F#4-D5-F#4 · Em-bar E4-G4-B4-E5 · C-bar
  G4-E4-C4.

### Electronic
- **A — Trance 16th arp** — Am, 138 BPM, 4 bars, Am-F-C-G. Drums: Kick 1,5,9,13 · open hat
  3,7,11,15 · closed hat evens · Clap 5,13. Arp (pluck, 1 note/step, Am bar): A3-C4-E4-A4
  /C4-E4-A4-C5/E4-A4-C5-E4/A4-E4-C4-A3 (transpose shape to F/C/G). Strings swell: triad
  per bar. Bass: driving 16ths on root (rest step1).
- **B — Future-bass stabs** — C, 150 BPM (½-time feel), 4/4. Chords: Cmaj9 → Am9 →
  Fmaj9sus2 → G6/9. Stabs (supersaw, gated): steps 1,4,7,8,11,14. Drums: Kick 1,11 ·
  snare/clap 9 · hat rolls. 808 roots (1 & 11): C1-A0-F0-G0.

### Hip-Hop
- **A — Boom-bap** — Dm, 90 BPM, swung, 2 bars. Drums: Kick 1,7,11 (+ghost 14) · Snare
  5,13 (laid-back) · swung hat 8ths. Sub bass: D1 step1, G1 step9. Rhodes/piano (rootless,
  1/bar): Dm9 (F3-A3-C4-E4) / G13 (F3-A3-B3-E4). Stab: muted trumpet A4 on 11.
- **B — Trap** — Fm, 140 BPM (½-time), 1 bar. Drums: snare/clap 9 · 808 kick 1,4,7,11,14 ·
  hat 16th bed + roll into 9 + triplet burst 13–15. 808 slide bass: F1(1), F1(4),
  Ab1→C2(7→9 up), C2→F1(11→14 down). Dark motif: pad Fm9; pluck lead F4-Ab4-C5-Db5-C5 on
  1,5,9,11,13.

### Classical (exact, public-domain)
- **A — Pachelbel, Canon in D** — D major, ~64 BPM, 4/4, 4 bars. Cello ground bass
  (note at 1 & 9): D-A-B-F#-G-D-G-A (octaves e.g. D3-A2/B2-F#2/G2-D2/G2-A2). Implied
  harmony (1/bass note): D-A-Bm-F#m-G-D-G-A. Violin first phrase (half notes, 1 & 9):
  F#5-E5 / D5-C#5 / B4-A4 / B4-C#5. Piano: block the implied chords under the violin.
  (Octave registration edition-dependent; tempo free — no autograph marking.)
- **B — Beethoven, "Ode to Joy" (Sym. 9)** — D major, ~120 BPM, 4/4, 4 bars. Melody
  (violin; quarters on 1,5,9,13 except bar 4): F#4-F#4-G4-A4 / A4-G4-F#4-E4 / D4-D4-E4-F#4 /
  **F#4 dotted-quarter(1) → E4 eighth(4) → E4 held(5+)**. Cello/bass: double the theme an
  octave down for the authentic unison opening; harmonize I (D) with V (A) under phrase
  ends. (C-major teaching version also available.)

### House
- **A — Chicago piano house** — Am, 123 BPM. Drums: Kick 1,5,9,13 · Clap 5,13 · open hat
  3,7,11,15 · closed hat odds · rim 2,4,6,8,10,12,14,16. Piano stab Am7 (A3-C4-E4-G4) on
  1,4,7,8,11,14. Deep bass A1 on 1,5,9,13 (4-bar Am-F-C-G → A1/F1/C2/G1).
- **B — Deep house min9 Rhodes** — Cm, 122 BPM, swung, 4 bars. Chords (open min9):
  Cm9 (C3-Eb3-G3-Bb3-D4) → Am9 → Abm9 ×2. Drums: Kick 1,5,9,13 · Clap 5,13 · open hat
  3,7,11,15 · swung 16th hat. Rhodes stabs on 3,7,14. Sub bass (1/bar): C1-A0-Ab0-Ab0.

### Techno
- **A — Detroit fixed-interval stabs** — Cm, 132 BPM. Drums: Kick 1,5,9,13 · Clap 5,13 ·
  hat 3,7,11,15 · perc stab 7,15. Detuned brass/synth stab Cm (C3-Eb3-G3 +C4) sliding
  Cm-Gm-Fm-Abm, on 3,7,11,15 (+6,14), tight filter decay. Acid bass C2 16ths, accent 1 & 11.
- **B — Minimal 303 acid** — Am, 135 BPM. Drums: Kick 1,5,9,13 · Clap 13 · rim 7 · open hat
  15 · hat 3,7,11,15. 303 line (●=accent/slide): 1 A1● · 3 A2●slide · 5 A1● · 6 C2 ·
  7 A1slide · 8 E2● · 9 A1● · 11 A2●slide · 13 A1● · 14 G2slide · 16 A2●. Cutoff opens over loop.

### Jazz
- **A — Swing ii-V-I bebop** — C, 140–180 swing, 2 bars (Dm7|G7|Cmaj7|Cmaj7). Piano
  rootless (Charleston comp, hits 1 & 7): Dm7 F3-A3-C4-E4 / G7 F3-A3-B3-E4 / Cmaj7
  E3-G3-B3-D4. Walking bass (quarters 1,5,9,13): D2-F2-A2-Ab2 / G2-B2-D3-F2 / C3-E3-G2-A2 /
  C3-B2-A2-G2. Drums: ride spang-a-lang 1,5,7,9,13,15 · hat foot 5,13. Trumpet 8th line:
  bar1 D4-E4-F4-G4-A4-B4-C5-C#5; bar2 D5-B4-G4-F4-E4-Ab4-G4-F4.
- **B — Modal Dorian vamp** — D Dorian, 120–140, Dm7 held. Piano quartal voicings
  (D3-G3-C4-F4 and E3-A3-D4-G4) on 1 and 7/15. Bass: pedal D2 (1 & 9) or walk
  D2-A2-F2-E2. Drums: brushed ride spang-a-lang + hat 5,13. Trumpet Dorian motif (8ths):
  D4-F4-G4-A4-B4-A4-G4-F4 (hang on raised-6th B♮).

### Salsa
- **A — Salsa-dura mambo** — Cm vamp (Cm–G7), ~190 BPM (cut-time), clave 1 bar. Son clave
  3-2 = steps 1,4,7,11,13. Cowbell 1,5,9,13 (+ghosts). Conga (toms) open 7,15. Timbales
  cáscara 1,3,5,8,11,13,15. Tumbao bass (downbeat empty, anticipations on 7 & 13): Cm bar
  G2(7),G2(13) → G7 bar D3(7),C3(13). Piano montuno (3-2): Cm bar G4(3),Eb4(7),C4(9),
  G4(11),Eb4(15); G7 bar D4(1),B4(3),G4(7),D4(9),B4(11),G4(15). Trumpet stab Cm on 13 (+7).
- **B — Cha-cha-chá** — C / Dm7-G7, ~120 BPM, 2 bars. "Cha-cha-cha" figure on
  13→15→1(next). Güiro 1,5,7,9,13,15. Conga (toms) open 13,15. Timbales/cowbell 1,5,9,13
  (beat 4 open). Clave 3-2 or 2-3. Tumbao bass: Dm7 A2(7),G2(13) → G7 D3(7),D2(13). Piano
  montuno: Dm7 A4(3),F4(5),D4(7),A4(11),F4(13); G7 B4(3),G4(5),D4(7),B4(11),F4(13). Brass
  stab on 13/15.

### Funk
- **A — "On the one"** — Em/E Dorian, 106 BPM, 1 bar. Drums: Kick 1,8,11 · Snare 5,13
  (+ghost 4,12) · closed hat all 16 · open hat 15. Slap bass: E1(1,thumb), E1-dead(4),
  E2(7,pop), G1(9), A1(11), E2(14,pop), D1(16). Guitar muted chank stabs: 3,6,7,10,14,15
  (+open accent 2). Trumpet stab Em9 (E4-G4-B4-D5-F#5) on 1, answer on 11.
- **B — Linear 16th groove** — E vamp (Em7/E9), 100 BPM, 1 bar. Drums (linear, one voice/
  step): Kick 1,4,9,12 · Snare 5,13 (+ghost 7,15) · closed hat 2,3,6,8,10,11,14,16. Slap
  bass octave pops: E1(1), E1-dead(3), E2(4,pop), E1(6), E1(9), E2(11,pop), D2(12,pop),
  E1-dead(14), G1(16). Chord vamp Em7 (E3-G3-B3-D4) chank on 2,8,11,14.

---

## Appendix C — Sources

Synthesis (CCRMA/Stanford, Sound on Sound Synth Secrets, Chowning 1973 JAES, Dexed/DX7,
Wikipedia inharmonicity/Karplus-Strong) and genre pattern sources (Attack Magazine,
MusicRadar, Drumeo, Learn Jazz Standards, clave/montuno/tumbao references, public-domain
classical via Wikipedia/IMSLP) are catalogued in the research transcripts produced during
brainstorming; the load-bearing primary citations (Karplus T60/brightness math, Chowning
brass c:m=1:1/index mechanism, piano inharmonicity formula + measured B values, son clave
3-2 = steps 1,4,7,11,13, tumbao anticipation rules, Pachelbel ground bass + violin phrase,
Ode to Joy signature dotted-quarter+eighth) are cross-checked by ≥2 sources.
