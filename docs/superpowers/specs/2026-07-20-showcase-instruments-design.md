# Showcase Instrument Batch — Design Spec

**Date:** 2026-07-20
**Author:** Claude (Opus 4.8) + ingyamilmolinar
**Status:** Approved, implementing

## Goal

Add **24 new built-in modular-synth instruments** — a "balanced & wild" showcase
that mixes usable-but-new sounds (marimba, clarinet, tuba, Rhodes) with genuinely
out-there ones (gamelan, singing bowl, glass wash, morphing alien choir). Purely
config-driven: no C changes, no new DSP. Each instrument is designed from the
synthesis principles in `AGENTS.synth*.md` and the existing seed corpus — **not**
reference-WAV A/B tuned. First renders are strong drafts; the ear-gate is the user.

## Architecture (identical for all 24)

Each instrument is:
1. One `RecipeParams` seed var in a new file `internal/audio/instrument_seeds_showcase.go`.
2. One row in `modularInstrumentDefs` (`internal/audio/modular_instruments.go`).
3. One `melodicRecipeIDs` entry (`internal/audio/melodic_recipes.go`) — all 24 are
   pitched (mallets included), so all 24 are melodic → per-pitch re-render.

Then regenerate:
- `cd src/go && go run ./cmd/gen-modular-instruments > ../js/modular_instruments.gen.js`
- `go generate ./internal/audio/` (rebuilds `instrument_ids_gen.go`)

From the table row, the engine derives: the baked voice, recipe descriptor +
instrument→recipe binding, playable `InstrumentConfig`, audio.js RENDER/RENDER_INFO,
and the id list. `Amplitude: 0.8` on every row (the loudness fallback; no
`make measure-loudness` run this pass).

## Parameter vocabulary used

`osc_type` (0 Sine, 1 Saw, 2 Square, 3 Tri, 4 FM, 7 Bowed, 9 Reed, 12 Voice);
gen bank `genN_source` (1 tone-osc, 2 noise, 3 Karplus-Strong); `genN_wave`
(0 sine, 1 saw, 2 sq, 3 tri); `genN_filt_type` (3 LP, 4 HP, 5 BP RBJ);
KS `genN_ks_sustain/ks_pluck`; amp ADSR; `pitchenv_*`; `lfo_*` (target 0 Amp,
1 Pitch, 2 Cutoff); `filtenv_*`; unison/ensemble; formant vowel bank; body
resonator (`body_model` 1 Violin, 2 Guitar, 3 Cello, 4 Steel). `gain` never 0.5.

## Roster

### A · Mallets & Metals (additive struck, inharmonic partials + amp decay)
1. **marimba** — sine H1 + ~3.9× wood-bar partial, fast warm decay, mallet chiff.
2. **vibraphone** — sine + ~4× metal partial, long ring, LFO→amp tremolo (motor).
3. **glockenspiel** — sine + high inharmonic partials (~5.4×, ~10×), bright long ring.
4. **gamelan** — inharmonic saron partials (~2.76×, ~5.4×) with beating detune.

### B · Plucked & struck strings (Karplus-Strong)
5. **kalimba** — short bright KS, tiny body — thumb piano.
6. **sitar** — KS through high-Q steel body (jawari buzz) + detuned sympathetic KS.
7. **banjo** — very bright fast-decay KS, thin body — twang.
8. **music-box** — FM bell (inharmonic mod ratio), plucked, delicate.

### C · Keys (FM / KS / additive)
9. **piano-rhodes** — 2-op FM tine (mod ratio ~14), soft attack, medium decay.
10. **clavinet** — bright plucky KS + light drive — funk.
11. **harpsichord** — bright KS + 4' octave course, quilled, no dynamics.
12. **celesta** — struck metal (sine + high inharmonic partials), tinkly.

### D · Winds & reeds (additive / subtractive)
13. **clarinet** — hollow odd-harmonic-only additive reed.
14. **tuba** — dark H1–H2 additive low brass, slow onset, octave-down bias.
15. **pan-flute** — breathy near-sine + prominent air noise, hollow.
16. **harmonium** — sustained detuned reed-organ with beating + vibrato.

### E · Synth leads & basses (subtractive)
17. **lead-supersaw** — 7 detuned saws, bright filter — trance.
18. **lead-hoover** — detuned saw + pitch-env drop + filter sweep — rave.
19. **lead-chip** — square, 8-bit game blip.
20. **bass-wobble** — saw, LFO→cutoff wobble + resonance + drive — dubstep.

### F · Experimental / textural
21. **theremin** — pure sine + heavy vibrato + slow swell — eerie.
22. **singing-bowl** — inharmonic metal, very long ring, slow beating — meditative.
23. **glass-wash** — inharmonic slow-swell shimmer pad with beating + slow filter LFO.
24. **voice-alien** — glottal voice + fast formant morph (Ah→Ee) + vibrato ensemble.

## Verification

1. `gofmt` + native build + `GOOS=js` compile check.
2. Audio discipline/completeness/invariants/golden suite green — every seed renders
   finite/bounded/on-pitch/non-silent; existing 40 stay byte-identical.
3. Render a WAV audition of each of the 24 + an HTML index; deliver for the ear-gate.

## Risks / assumptions

- FM keys (Rhodes tine decay) and textural pads are the most likely to need a
  re-tune after the ear-gate. Accepted — first drafts.
- Seed-distinctness guard requires every seed differ from every other; the roster
  is designed to be distinct, verified by the test suite.
