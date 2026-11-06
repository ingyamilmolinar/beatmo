# Instrument Loudness Normalization + Headroom Defaults — Design

**Date:** 2026-07-03
**Status:** Approved design, pending spec review → implementation plan

## Problem

Synth instrument loudness is "all over the place" and kick drums are conspicuously
quiet even at 100% instrument volume + 100% master. The user wants:

1. All instruments at a **similar perceived loudness**.
2. Kicks specifically brought up to a comparable level.
3. **Headroom to grow**: defaults must not start at 100%; there must be room to
   turn instruments and the master both up and down.
4. Per-instrument loudness reported **in decibels** (as close as possible).

## Root-cause diagnosis (why it's broken today)

Every synth one-shot passes through `normalizeAndScale` (`src/go/internal/audio/export.go:29`):

1. Render raw DSP — the per-recipe seed `"gain"` (0.45 violin … 1.0 kicks) is applied
   here, at `src/c/modular.c:922`.
2. **Peak-normalize the whole buffer to 1.0** — this *erases* the seed gain as an
   absolute-loudness factor.
3. Multiply by a per-instrument `Amplitude` — a flat **0.8** for nearly every
   instrument (`config.go` / `modular_instruments.go`; exceptions organ 0.65, sax 0.78).
4. 5 ms fade-out.

Consequences:

- The seed `"gain"` values that *look* "all over the place" are normalized away —
  they only affect internal harmonic balance, not final loudness. The real loudness
  knob is `Amplitude`, and it's essentially uniform (0.8).
- **Peak-normalization equalizes peaks, not perceived loudness.** A kick is a sharp
  transient with fast decay at 44–58 Hz; after its peak is set to 0.8 its RMS/average
  energy sits far below 0.8, while a sustained organ/sax holds near 0.8 for the whole
  note. Same peak, very different loudness. Low-frequency equal-loudness weighting and
  weak laptop/phone speakers make kicks vanish further.
- There is **no RMS/LUFS loudness normalization anywhere** — that is the gap.

### Gain staging (verified)

There is exactly **one live per-instrument volume control** (the row slider →
`DrumRow.Volume`) plus the master volume. The per-instrument *audio-channel* volume
(`Channel.Volume()`) exists in the mixer graph but is **not wired to any live UI
slider** (constant 1.0 in a normal session; only the offline exporter sets it). So
lowering the "instrument volume" default halves the signal exactly **once**.

Default per-voice gain product today:

| Stage | Default | Location |
|---|---|---|
| Peak-normalize → ×Amplitude | 0.8 | `config.go` / `modular_instruments.go` |
| Row volume `DrumRow.Volume` | 1.0 | `drumview_ctor.go:182` → `game_trigger_logic.go:133` → `engine_play.go:125` |
| Voice headroom | 0.18 | `chain_spec.go:69` (`VoiceHeadroom`) |
| Instrument channel volume (not user-wired live) | 1.0 | `channels.go:85`, applied `channels.go:314` |
| Master channel volume | 1.0 | `channels.go:541`, applied `channels.go:246` |
| Master compressor | −3 dB / 2.5:1 | `chain_spec.go:42` |
| Soft-clip threshold | 0.9 | `chain_spec.go:54` |

Static product (stages 1–5): `0.8 × 1.0 × 0.18 × 1.0 × 1.0 = 0.144` peak per voice.

## Approach

**Loudness-normalize each instrument to a common target, baked into the existing
per-instrument `Amplitude` constant.** No DSP change, no real-time-path change — the
render pipeline already peak-normalizes to 1.0 and then multiplies by `Amplitude`;
we simply replace the flat 0.8 with a *measured, per-instrument* value.

Rejected alternatives (during brainstorming): a quick hand-tuned kick bump (guesswork,
leaves the general spread unaddressed) and runtime LUFS auto-gain (per-frame cost, risk
to the real-time audio path, overkill for a fixed built-in instrument set).

### 1. Measurement tool (offline, real DSP)

A small offline command renders all ~52 instruments through the **real C DSP engine**
via `synthmatch.RenderInstrument(id, pitch=0, sr, dur)` — built as a **production
binary, NOT under `-tags test`** (the test build swaps in a pure-Go approximation
synth and would measure the wrong thing).

For each instrument it:
- Segments to the loudest window (`fingerprint.AutoSegment`).
- Measures via `wave.NewPeakRMSObserver().Observe(w)` → `Peak`, `RMS`, **`PeakDB`,
  `RMSDB`** (dBFS, `20·log₁₀`), plus `ClipCount`.
- Computes the peak-normalized RMS (`RMS_norm = RMS / Peak`, i.e. `1/crest_factor`).
- Emits a table: instrument, current `RMSDB`/`PeakDB`, computed `Amplitude_i`, and the
  projected post-normalization `RMSDB`/`PeakDB`.

**Metric choice:** flat RMS in dBFS, **not** K-weighted LUFS. LUFS de-emphasizes low
frequencies and would penalize kicks — the opposite of the goal. Flat RMS over the
loudest window is the right perceptual proxy here.

### 2. The gain formula

After peak-normalization a buffer has peak 1.0 and `RMS_norm = 1/crest`. To land every
instrument at a common RMS target `L`:

```
Amplitude_i = clamp( L / RMS_norm_i , 0 , peakCap )
```

- High-crest signals (kicks: sharp click, low body) → **higher** `Amplitude`.
- Sustained-loud signals (organ, sax, strings) → **lower** `Amplitude`.
- Since the normalized buffer peaks at 1.0, the post-scale peak equals `Amplitude_i`,
  so `peakCap` directly bounds clipping.

### 3. Target `L` and peak cap (uniform for every instrument)

**One strategy applied identically to all instruments — no per-instrument or
per-category special-casing.** Kicks were only the user's example of a quiet
instrument; they get the exact same formula as everything else.

- `L` (single common RMS target) is **calibrated from the measured data** so the
  *typical/median* instrument stays near its current level — the app must not feel
  quieter overall despite the change. `L` will be stated in dBFS in the measurement
  output.
- `peakCap` is a **single value shared by all instruments** (the clamp in the formula),
  chosen to prevent clipping. High-crest signals (kicks, plucks) that would need
  `Amplitude > peakCap` to reach `L` are capped and so will not fully reach `L` — this
  is the physical reality of a transient signal and is applied **uniformly**, not as a
  kick exception. The existing soft-clip (0.9) + hard clamp shape any transient over
  gracefully.
- Loudness-matching pulls the loud sustained instruments *down* toward the common
  target, so previously-quiet instruments (kicks among them) become proportionally more
  present in the mix, and the master compressor stops being slammed by the loudest
  instruments. This relative rebalancing is a natural consequence of the uniform
  strategy, not a kick-specific fix.

### 4. Headroom defaults → 50%

- Fresh **row/instrument** volume: `1.0 → 0.5` (`drumview_ctor.go:182` **and** the
  runtime row-append seed — search `&DrumRow{` / `append(dv.Rows` / `row_rack_zone.go:707`).
- Fresh **master** volume: `1.0 → 0.5`, set at master-channel init/reset — **not** in
  the shared `newChannel` (`channels.go:85`), which would wrongly halve instrument
  channels too. Set the main channel to 0.5 post-creation (reset path) and/or via the
  `MainVolume` initial value the transport slider reads.
- Volume sliders stay **0–100%** (`Slider` is a 0..1 control). 50% gives room both up
  and down. `L` is calibrated so 50%/50% is a comfortable reference level and 100%/100%
  is "cranked" (limiter engaged, no harsh clipping).

### 5. Explicitly NOT changed

- Seed `"gain"` values (normalized away; cosmetic).
- The DSP / C engine and the real-time audio path.
- **Saved projects**: import restores stored volumes (`export.go:435`,
  `js_exports_graph_ui.go`); only *fresh* projects get the 50% defaults. The mute-toggle
  0-sentinel semantics (a stored 0 means muted, restored on unmute) must be re-checked so
  unmute restores to the row's prior value, not a hard-coded 1.0.

## Components / files touched

| Unit | Purpose | Files |
|---|---|---|
| Loudness measurement command | Render all instruments (real DSP), measure dB, compute `Amplitude_i` table | new `src/go/cmd/loudness-measure/` (or a `make` target) |
| Per-instrument `Amplitude` values | Baked loudness gains | `src/go/internal/audio/config.go`, `modular_instruments.go` |
| Volume defaults | Row + master start at 0.5 | `drumview_ctor.go:182`, row-append seed, master-channel init |
| Loudness guard test | Pin the per-instrument dB spread so it can't drift | new test in `internal/audio` |

## Verification

- **Before/after per-instrument dB table** proving the spread tightens (goal: RMS
  within a tight band, target ≈ ±2 dB, vs. today's wide spread); previously-quiet
  instruments (kicks among them) audibly up.
- **No clipping at 100%/100%**: verify via offline mixer render + peak/clip check that a
  representative dense circuit at full user gain stays within the soft-clip/limiter's
  graceful region (no sustained hard clamp).
- **Guard test** pinning the loudness spread (new golden) so future instrument edits
  can't silently re-introduce the spread.
- **Ear check** on desktop (`make run`) and browser (`make wasm`) — instruments
  balanced at the 50% default, previously-quiet instruments now present.

## Known test fallout (expected, part of the plan)

- Cross-platform audio goldens (`xplat_audio_compare`, `ifx_xplat_golden`, render tables)
  — regenerate.
- `all_instruments_pitch_table_test.go`, `instrument_seeds_test.go` RMS/peak assertions
  — update thresholds.
- Any test asserting default row volume `1.0` — update to `0.5`; re-check mute-toggle
  0-sentinel restore behavior.
- Watch `TestPhase2Native` / the "never seed gain:0.5" constraint — that concerns the
  seed `"gain"` param, which we are **not** touching (we touch `Amplitude`), but verify.

## Non-goals

- No runtime/live auto-gain.
- No change to the slider range (stays 0–100%).
- No re-tuning of instrument *timbre* — loudness only.
