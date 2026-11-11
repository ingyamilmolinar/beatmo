# Wave Tab — Remove Pause, Adaptive Auto-Scale with Readout

Date: 2026-06-15
Branch: add-core-node-types
Status: Approved design — ready for implementation plan

## Goal

Two user-facing changes to the audio panel's **Wave** tab:

1. **Remove the "pause" button** (the freeze pill) from the Wave tab.
2. **Adaptive Y-scaling**: the waveform always auto-fits the signal's recent
   peak so it is maximally visible regardless of instrument or loudness, with
   a clear on-screen readout of the current Y-scale. Auto is on by default,
   can be disabled, and the user can manually zoom in/out with the scroll wheel.

## Decisions (from brainstorming)

- **Scaling mode**: on by default, user can disable; manual zoom in/out available.
- **Rescale feel**: smoothed peak-hold (instant attack on a louder hit, slow
  decay of the scale) — no per-frame height jitter.
- **Scale readout**: dynamic edge labels (real amplitude) **and** a gain badge.
- **Control placement**: an `AUTO` pill replaces the freeze pill in the Wave
  header; scroll-wheel over the wave zooms and flips AUTO off — identical UX to
  the Chain/Scope tab.

## Current state (as found)

- `render_waveform.go`
  - `drawAnalyzerWaveform` draws the trace. Amplitude→pixel mapping is fixed:
    `drawWaveTrace` uses `halfHeight = rect.Dy()*0.48*yGain` with `yGain`
    hardcoded to `1.0` (lines 124, 238-239).
  - Edge labels are hardcoded `+1 / 0 / -1` (lines 65-67).
  - `overdrawWaveClips` flashes columns where raw `|v| ≥ 1.0` (true clipping).
  - `drawWaveformCursor` already reads the true sample value `wave[idx]`.
- `audio_tab_controls.go`
  - `waveControls` embeds `freezeControls` and shows the only Wave control: a
    freeze pill (`||`/`>`) built by `newFreezePill`, wired to `OnFreezeToggle`.
  - `freezeControls` provides the shared header machinery: `HeaderH()`,
    `HitAreas()`, `SyncFreeze()`, `addHit`, `layoutFreezePill`.
- `eq_panel_zone.go`
  - `waveControls` constructed at ~line 344 with `OnFreezeToggle`.
  - Wave tab draw path (`TabWave` case ~line 522) resolves the channel
    (`resolveChannel` → `state.Master` or `state.Detail`) and calls
    `drawWaveformCached(...)` then `drawWaveformCursor(...)`.
  - `SyncFreeze` + `Draw` of the active tab controls at ~lines 637-644.
- Existing auto-gain reference model: `chain_trace_cache.go:98-111`
  (`renderTracesDirect`): `effectiveGain = 0.9/peak`, clamped `[1.0, 16.0]`;
  `chainZoomHandler` (`chain_panel_zone.go:1111-1151`) does shift/scroll Y-zoom
  and sets `autoGain = false` on scroll.

## Design

### 1. Remove the freeze pill; add an AUTO pill (Wave tab only)

`waveControls` no longer shows a freeze pill. Instead it owns an **`AUTO`
toggle pill** in the header (right-aligned, same slot/size the freeze pill used).

- The freeze pills on Levels/Spectrum/Chain are **untouched**.
- Global analyzer freeze state is still honored by the Wave *renderer*
  (`drawAnalyzerWaveform` still picks `capture.Wave.Samples` and draws the
  `FROZEN` tag when the analyzer is frozen from another tab). Only the Wave
  tab's own button is removed — no renderer regression.
- Shared layout helper: extract the 4-line right-pill placement currently in
  `freezeControls.layoutFreezePill` into a small free helper
  `layoutRightPill(header, btn) (btnH, y, rightEdge int)` so `waveControls` can
  reuse it without embedding freeze-specific glyph logic. `freezeControls`
  keeps using it for the other tabs (no behavior change there).

`waveControls` gains:
- `autoBtn *Button` — label `AUTO`, toggled-visual via `drawPillTabAt(dst, autoBtn, autoOn)`.
- `OnClick` flips `EQPanelZone.waveAutoGain` (wired through a callback, mirroring
  how other pills call back into the zone, e.g. Levels' Clear-Clips).
- Hit area tag `wave-auto-btn`.
- `SyncAuto(on bool)` to keep the pill visual in sync with the zone's state
  (parallels `SyncFreeze`).

### 2. Adaptive Y-scaling state + smoothing (on `EQPanelZone`)

New fields:
- `waveAutoGain bool` — default **true**.
- `waveYGain float64` — default `1.0`; the manual zoom factor used when AUTO is off.
- `waveSmoothedPeak float64` — smoothing accumulator for peak-hold.

Per data refresh (computed where the wave trace cache rebuilds, so gain and
cached pixels stay coherent — see §4):

```
peak = max(|sample|) over the visible window   // mono: wave; stereo: max(L,R)
// smoothed peak-hold: instant attack, slow release
if peak >= waveSmoothedPeak { waveSmoothedPeak = peak }            // attack
else { waveSmoothedPeak = waveSmoothedPeak*release + peak*(1-release) } // decay
effectiveGain = waveAutoGain ? autoGainFor(waveSmoothedPeak) : waveYGain
```

`autoGainFor(peak)` (pure, unit-tested):
```
if peak < 0.001 { return 1.0 }      // silence → no blow-up of noise floor
g = 0.9 / peak
clamp g to [1.0, 16.0]              // quiet scales up; never below 1×
return g
```

`release` is a named constant (e.g. `waveAutoGainRelease = 0.90` per refresh,
~150 ms decay at the analyzer's ~30 Hz republish rate — tune during impl).
Document the constant with the same rationale style as the runtime profile knobs.

Manual zoom: a `waveZoomHandler` on the Wave content rect mirrors
`chainZoomHandler` — scroll multiplies `waveYGain` by a factor, clamps to
`[0.25, 16.0]`, and sets `waveAutoGain = false`. (Wave allows zoom *out* below
1× since the user explicitly asked to zoom in **and** out; the auto path still
never goes below 1×.)

### 3. Renderer changes (`render_waveform.go`)

`drawAnalyzerWaveform` gains parameters carrying the resolved scale, e.g.
`gain float64, autoOn bool`:

- Pass `gain` into both `drawWaveTrace(...)` calls (mono + stereo) instead of `1.0`.
- **Edge labels** become the real displayed amplitude at the trace edges:
  `edge = 1.0/gain`, formatted to 2 sig figs → `+0.24 / 0 / -0.24`
  (the dashed ±0.5 gridlines become `±0.5*edge` accordingly, or keep them as
  fractional-of-visible and relabel — decide in plan; simplest: they are
  fractions of the visible full-scale so they stay at the same pixels and need
  no relabel).
- **Gain badge** top-right (same slot as `FROZEN`): `AUTO ×4.2` when auto is on,
  `×4.2` (or `MANUAL ×N`) when off. If the analyzer is globally `frozen`, the
  `FROZEN` tag wins that slot (freeze indication takes priority).
- `overdrawWaveClips` is **unchanged** — it keeps reading raw samples at the
  true `|v| ≥ 1.0` threshold, so a quiet signal magnified to full height never
  falsely flashes the red clip overlay.
- `drawWaveformCursor` is unchanged: it already reports the true sample
  amplitude, which remains consistent with the relabeled edges.

### 4. Caching

`effectiveGain` (quantized to avoid churn, e.g. rounded to 0.01) folds into the
wave trace cache key in `wave_trace_cache.go` so the cached frame invalidates
when the scale eases to a new value. Smoothing is advanced in the cache-rebuild
branch (driven by the ~30 Hz analyzer-state pointer change), keeping gain and
pixels coherent — same approach the Chain tab uses.

## Components / boundaries

| Unit | Responsibility |
|---|---|
| `autoGainFor(peak) float64` | Pure gain mapping (silence/clamp). Testable in isolation. |
| `EQPanelZone` wave-scale state | Owns `waveAutoGain`/`waveYGain`/`waveSmoothedPeak`; advances smoothing; resolves `effectiveGain`. |
| `waveControls` | Owns the `AUTO` pill: layout, hit area, visual sync. No scale math. |
| `waveZoomHandler` | Scroll-wheel → manual zoom; disables AUTO. Mirrors `chainZoomHandler`. |
| `drawAnalyzerWaveform` | Pure render from `(samples, gain, autoOn, frozen)` → trace + edge labels + badge. |
| `wave_trace_cache.go` | Caches the rendered trace keyed by state + layout + quantized gain. |

## Testing

Go fast path (`-tags test`):
- `autoGainFor`: quiet→high gain, loud (peak≈1)→1×, clamp at 16×, silence→1×.
- Smoothing ease: louder peak snaps up instantly; quieter peak decays over N refreshes.
- `waveControls`: owns an `AUTO` hit area (tag `wave-auto-btn`), owns **no**
  freeze hit area; `TabWave` dropped from any mobile-freeze matrix; AUTO pill
  visual follows `SyncAuto`.
- Wheel: scroll over Wave content multiplies `waveYGain`, clamps, sets
  `waveAutoGain=false` (drive via the real tree dispatch, per the functional-test
  preference — `[[feedback_functional_tests_over_isolation]]`).
- Renderer: edge label text equals `±(1/gain)` formatting; badge text `AUTO ×N`
  vs `×N`; `FROZEN` wins the slot when frozen.
- Pixel test (extend `audio_panel_render_pixels_test.go`): a quiet synthetic
  channel produces a taller trace (more colored rows) than the same signal at
  gain 1, via the `drawRect` interceptor + `testAnalyzerStateOverride`.
- Browser: re-run `audio_panel_render.browser.test.js` (badge addition; wave
  still renders desktop + mobile).

## Out of scope (YAGNI)

- No per-session persistence of AUTO/zoom state (userprefs) — session-only.
- No dBFS conversion of the readout — linear amplitude only.
- No changes to Spectrum/Levels/Chain/EQ tabs (their freeze pills stay).
- No new design tokens unless the badge needs one; prefer existing accessors.

## Risks / notes

- The freeze-data render path in `drawAnalyzerWaveform` must stay reachable
  (global freeze from other tabs) — verify the `FROZEN` tag still draws.
- Token discipline: any new color must use a `Token*()` accessor / `WithAlpha`
  bucket, not an inline `color.RGBA{}` (ratchet test).
- Per-tab alloc budget on the audio panel — keep the badge/labels alloc-free
  (served colors, no per-frame string alloc beyond the existing label path).
