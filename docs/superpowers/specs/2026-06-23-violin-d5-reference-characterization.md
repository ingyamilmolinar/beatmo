# Violin D5 Reference — Full Characterization

**File:** `356181__mtg__violin-d5.wav` (MTG/UPF Good-Sounds dataset, A=442 tuning)
**Format:** 24-bit / 48 kHz / mono / 2.675 s
**Method:** numpy ground-truth + the Beatmo `fingerprint` pipeline (validated by `internal/audio/fingerprint/violin_golden_test.go`).

This is the target Phase B tunes the synthesized violin toward.

## Pitch & vibrato
- **F0 = 591.3 Hz** (D5 ≈ 587 at A=440; this sample is A=442, so ~591). The old `DetectF0` reported 195 Hz — a /3 sub-harmonic octave error, now fixed.
- **Vibrato: rate ≈ 3.65 Hz, extent ≈ ±3–5 cents.** This is a relatively *straight* Good-Sounds note — shallow, slowish vibrato (not the 6 Hz / ±20 c of an expressive romantic vibrato). Tuning the synth to a deep 6 Hz vibrato would NOT match this reference.

## Harmonic structure (sustain)
- Normalized partial amplitudes H1..H8: **1.00, 0.75, 0.22, 0.58, 0.37, 0.03, 0.03, 0.005**.
- Even/odd energy ratio ≈ **0.76** (odd-leaning, typical bowed string).
- Strong H2 and H4; H6+ fall off quickly in the steady state.
- Inharmonicity ≈ near-zero (a good harmonic string).

## Spectral envelope / body resonance
- **Bridge hill ≈ 2.3–2.5 kHz** (metric: 2531 Hz) — the violin's signature body/bridge resonance. Additional envelope peaks near ~557 Hz, ~3.5 kHz, ~4.2 kHz.
- Spectral centroid (sustain) ≈ **2340 Hz**; mean centroid over the note ≈ 2140 Hz.
- Spectral flatness ≈ **0.094** — tonal with a modest bow-noise floor.

## The two brightness↔loudness couplings (the key dynamic finding)
The violin has **two distinct, real couplings** depending on time scale:
- **Macro (whole note): corr(centroid, RMS) ≈ +0.57 to +0.65.** The note swells brighter as it gets louder and darkens as it decays — the expressive crescendo. THIS is the "alive vs. organ" trait Phase B must reproduce (the current synth does the opposite: centroid slope −38 Hz/s, static).
- **Micro (within steady sustain): corr ≈ −0.74.** Removing the macro swell, moment-to-moment *louder = slightly duller* — the fixed body formants don't track the amplitude/pitch micro-modulation. A subtler real property.

Both are now measured: `Coupling.MacroCentroidRMSCorr` (whole signal) and `Coupling.CentroidRMSCorr` (sustain window).

## Envelope
- **Attack 10–90% ≈ 0.5 s** (slow bow swell; time-to-90% ≈ 0.72 s). The current synth attacks in ~0.03 s — far too fast.
- Long decay tail; the note darkens as it decays (centroid 2206 → 1125 Hz over the second half).

## Implications for Phase B (synth tuning)
1. **Brightness must track loudness over the note** (macro +0.6) — needs the filter cutoff / harmonic content to rise with the amplitude envelope (filter-env tracking amplitude, or bow-dynamics driving brightness). This is the headline change.
2. **Slow bow attack** (~0.4–0.5 s), not a fast onset.
3. **Bridge hill at ~2.4 kHz** — the body resonator bank should emphasize this region (it does at `body_model:1`); verify the rendered centroid lands ~2.3 kHz, not the current ~1.0–1.4 kHz (too dull).
4. **Restore upper harmonics** — current synth collapses H6+ toward zero (centroid 1432 vs 2170); the reference keeps meaningful H2–H5.
5. **Shallow vibrato (~4 Hz, ±5 c)** to match THIS sample — current seed `lfo_rate:5.8, lfo_depth:0.05` is close on rate; depth is roughly right.
6. **The optimizer stub renderer is blind to `filter_cutoff`** (`synthmatch/render_stub.go`) — a real Phase-B gap: auto-tuning under `-tags test` can't descend on cutoff. Phase B should either tune via the prod (CGo) renderer or teach the stub to respond to cutoff.

## Metric coverage added (Phase A)
`fingerprint.Fingerprint` now additionally carries: `Vibrato` (rate/extent/jitter/AM/onset), `Harmonics` (per-harmonic attack/peak-time/sustain/decay + even-odd-over-time), `SpectralEnv` (formants + bridge hill), `Coupling` (micro + macro centroid-RMS corr + brightness modulation), `NoiseProf` (harmonic/residual split + residual centroid + per-band residual). `Distance` weighs vibrato, coupling, formant, and harmonic-trajectory terms.
