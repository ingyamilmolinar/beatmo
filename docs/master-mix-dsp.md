# Master Mix & EQ — how circuits stay clean when everything plays together

**Date:** 2026-06-10 · **Scope:** startup circuit + the 7 genre templates

## The bug we fixed: browser-only clipping

The combined mix clipped **in the browser** but not on native desktop. Root cause:

- **Native** applies a per-voice headroom of `VoiceHeadroom = 0.18` to every voice
  *before* the voices sum (`engine_stop.go` `renderVoiceIntoInstBuf`). So N voices
  on the same beat sum to a safe level and the master compressor/limiter is just a
  safety net.
- **WebAudio (`src/js/audio.js`) never applied it.** `chain_spec.gen.js` even
  defined `voiceHeadroom: 0.18`, but the per-voice anti-pop gain ramped to `1.0`.
  So in the browser the voices summed **~5× hotter** and slammed the master
  compressor + tanh soft-clip into heavy, audible saturation.

Measured raw summed peak (no headroom = the pre-fix browser condition) vs the
native/with-headroom pre-limiter peak:

| genre | pre-limiter peak (headroom on) | raw sum (headroom off ≈ old browser) |
|---|---|---|
| rock | 0.27 | 1.49 |
| hip-hop | 0.32 | 1.76 |
| pop | 0.38 | 2.10 |
| funk | 0.28 | 1.54 |
| salsa | 0.39 | 2.18 |
| house | 0.34 | 1.91 |
| techno | 0.49 | 2.72 |

Anything whose raw sum is ≫ 0.9 saturates the limiter. The fix: `audio.js` now
ramps each voice's anti-pop gain to `CHAIN_SPEC.voiceHeadroom` (matching native).
After the fix, browser pre-limiter peaks equal the native column (0.27–0.49) — no
clipping.

## How each instrument is EQ'd to mix well

Standard mix-engineering rule, applied programmatically in
`internal/templates/genres.go` (`applyMixEQDefaults` / `defaultHighpassHz`):

- **Kick / bass / sub own the low end** — no high-pass.
- **Everything else is high-passed** so it doesn't pile sub-bass energy into the
  kick/bass region (clears mud, frees headroom). Cutoffs by role:
  - hihat / shaker / crash / ride → 400 Hz (pure highs)
  - toms → 70 Hz (real low body; trim only the sub)
  - clap / snare → 180 Hz
  - cowbell / sidestick / rim → 220 Hz
  - fm-epiano / fm-pluck / fm-lead / modular → 110 Hz (keep body, trim sub)
- Noise percussion also gets a **low-pass** (`LowpassHz`) to tame harsh white-noise
  hiss into colored, musical noise (shaker/open-hat 9 kHz, clap 9 kHz, crash 12 kHz).
- Rows may override any of these by setting `HighpassHz` / `LowpassHz` explicitly.

A row's filters become per-instrument insert effects in the exported JSON
(`effects: [{type:"filter", params:{mode, cutoff, q, mix}}]`, mode 1 = highpass,
0 = lowpass) which the importer applies via `audio.SetInsertEffects`.

The master needs **no** EQ: once each voice carries headroom and the non-bass
instruments are high-passed, every master mix sits 4.8–10 dB below the soft-clip
knee. A master EQ would only dull the result.

## Programmatic DSP guards (run them)

All live in `internal/audio` (native `!test && !js` — they exercise the real CGo
synths + master chain), plus one fast-suite source guard.

| Test | What it catches |
|---|---|
| `TestTemplateMasterMix` | **Master headroom.** Each template's full-mix pre-limiter peak must stay < 0.85 (below the 0.9 soft-clip knee); the startup circuit's worst-case all-instruments-on-the-downbeat peak must stay < 0.9. Fails if a mix is too hot to play without slamming the limiter. |
| `TestTemplateAudioQuality` | Per-instrument + master: no clipping, healthy crest (>2.5), not white-noise-hissy (flatness), no crackle (jump-rate); bass is clean (HF<35%), tonal (harmonic>55%), balanced vs kick; any melodic row stays harmonic (>50%, catches overlap mud). |
| `TestTemplateInsertEffectsAreImportable` | The per-instrument filters round-trip into `audio.EffectSlot` the importer consumes. |
| `TestBassGuitarBannedFromTemplates` | Pins bass-guitar as intrinsically metallic (why it's excluded from bass roles). |
| `TestWebAudioAppliesVoiceHeadroom` (fast suite) | **Source contract:** `audio.js` ramps every voice to `CHAIN_SPEC.voiceHeadroom`, never to `1.0`. Directly guards the browser-clipping regression. |
| `chain_spec_parity.browser.test.js` | The live browser graph runs at `CHAIN_SPEC.sampleRate` with the C stages (effects, EQ, soft clip) on the beatmo-dsp worklet — or on the main-thread fallback host. |
| `xplat_audio_stages.browser.test.js` | **Every stage, both platforms, per sample.** Desktop's offline mixer (`cmd/export_audio --stages`) vs the browser's real graph on an OfflineAudioContext: played voices, lane stops, all 18 effects at four settings, every EQ shape, the master, and whole circuits imported through Go — in Chromium, Firefox and WebKit. |

**Listen to the renders:** `BEATMO_DIAG_WAV=/tmp/diag go test -tags '' -run TestTemplateAudioReport ./internal/audio/` dumps per-genre MIX + per-instrument WAVs and logs the full metrics table (peak/rms/crest/clip%/flatness/HF%/harmonic%/f0).

## Gotchas discovered

- **The soft-clip hides hard clipping.** Counting samples at ±1.0 (`clipFrac`) is
  useless for detecting a too-hot mix — the soft clip keeps the output below
  ±1.0 while distorting. Require the peak to stay below the knee: the soft clip
  is the identity up to 0.9 (`mix_dsp.c` `soft_clip`: `T + (1−T)·tanh((|x|−T)/(1−T))`
  above it, continuous and smooth), so a captured peak (`outputCaptureBuf`,
  after the soft clip — what the device receives, on both platforms) under 0.85
  is the same as a pre-clip peak under 0.85.
- **There is no master compressor** (deleted 2026-10-27). It had been dead on
  both platforms since every import writes the master EQ, which rebuilt the
  master chain without it.
- **Some synth params are silently ignored.** Cowbell ignores `brightness`/`tone`
  and the tom ignores `ring`; only the *wired* knobs (e.g. tom `decay`, cymbal
  `cym_tune`) take effect via the synth-params overlay. Verify with a render, don't
  assume.
- **The pitch resampler couples pitch↔duration.** Downward pitch lengthens notes
  (overlap mud on busy lines); upward pitch shortens but aliases HF-rich voices.
  Shorten the instrument's decay so neither bites.
