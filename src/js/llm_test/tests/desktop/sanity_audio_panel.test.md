---
name: sanity_audio_panel
platform: desktop
tags: [sanity, audio-panel, regression]
fixture: multi_row.json
max_iterations: 95
viewport: 1280x720
required_checkpoints: [audio_audible, eq_band_changed, wave_active, spectrum_slope_changed, spectrum_pre_toggled, levels_k20_toggled, chain_displaymode_changed, chain_ag_toggled, synth_param_changed, sampler_tab]
---

# Sanity: Audio Panel — all 7 tabs (Desktop)

Exercises a few real interactions per audio-panel tab (EQ · Wave · Spectrum · Levels · Chain ·
Synth · Sampler) and verifies each via read-back, plus real audio capture. The `multi_row` fixture
has 3 instruments for a rich signal. Switch tabs with `switch_tab {slug}`; click chrome pills by name
(`click_ui button="chrome:slope"` etc.); drag handles/knobs with the `drag` tool; verify with
`checkpoint_export` against the matching getter. The State hint lists `activeTab`/`channel`.

## Rules
- Resilient assertions only: read a baseline with `read_export`, do the REAL interaction, then
  `checkpoint_export {expect:{ne:<baseline>}}` (assert the value CHANGED — never pixel-exact).
- 2-attempt retry then `ISSUE: [P0|P1|P2|P3] ...` and move on. No page reload. **STOP after the summary.**

## Phase 0 — Audio capture
1. `measure_audio {name:"audio_audible"}` (hard-verifies captured RMS>0.001). Then `click_ui button="play"` so the tabs show live data for the rest of the test.

## Phase 1 — EQ
2. `switch_tab {slug:"eq"}`. `read_export {export:"eqControlsSnapshot", path:"gainsDB.3"}` (baseline G3).
3. Change band 3's gain through the real handler: `call_export {export:"setEQBandGain", args:["main", 3, 6]}` (the EQ curve handle is tree-captured and won't move via a raw pixel-drag headless). 
4. `checkpoint_export {name:"eq_band_changed", export:"eqControlsSnapshot", path:"gainsDB.3", expect:{ne: G3}}`.
5. (Exercise) toggle `click_ui button="hpf"` then `click_ui button="lpf"`; switch channel via `click_ui button="eqChannel"` → pick an instrument → back to Master. Report `ISSUE` on anything odd.

## Phase 2 — Wave
6. `switch_tab {slug:"wave"}` → `checkpoint {name:"wave_active", expect:{activeTab:"wave"}}`. Screenshot — confirm the waveform renders. (The log/lin freq toggle lives on the Spectrum tab, not here.)

## Phase 3 — Spectrum
7. `switch_tab {slug:"spectrum"}`. `read_export {export:"spectrumSlope"}` (S); `click_ui button="chrome:slope"`; `checkpoint_export {name:"spectrum_slope_changed", export:"spectrumSlope", expect:{ne: S}}`.
8. `read_export {export:"preOverlay"}` (P); `click_ui button="chrome:pre"`; `checkpoint_export {name:"spectrum_pre_toggled", export:"preOverlay", expect:{ne: P}}`.
9. (Exercise) the log/lin freq toggle is here: `read_export {export:"freqScaleLog"}`, `click_ui button="chrome:logFreq"`, confirm `freqScaleLog` flipped. Also `click_ui button="chrome:resetHold"`.

## Phase 4 — Levels
10. `switch_tab {slug:"levels"}`. `read_export {export:"k20View"}` (K); `click_ui button="chrome:k20"`; `checkpoint_export {name:"levels_k20_toggled", export:"k20View", expect:{ne: K}}`. (Exercise) `click_ui button="chrome:clearClips"`.

## Phase 5 — Chain
11. `switch_tab {slug:"chain"}`. `read_export {export:"chainDisplayMode"}` (D); `click_ui button="chrome:split"` (or "chrome:diff"); `checkpoint_export {name:"chain_displaymode_changed", export:"chainDisplayMode", expect:{ne: D}}`.
12. `read_export {export:"autoGain"}` (A); `click_ui button="chrome:ag"`; `checkpoint_export {name:"chain_ag_toggled", export:"autoGain", expect:{ne: A}}`. (Exercise) `click_ui button="chrome:freeze"` then unfreeze.

## Phase 6 — Synth
13. `switch_tab {slug:"synth"}`. `read_export {export:"rowInstrument", args:[0]}` → instrument id ID. `read_export {export:"synthKnobRects"}` → take the FIRST knob: note its `name` (K), `min`, `max`. (Note: getInstrumentParams only returns OVERRIDDEN params, so reading K before changing it returns undefined — that's expected; don't get stuck on it.)
14. Pick an exact value `targetV` strictly between min and max but NOT the midpoint (e.g. range −24..24 → use 7; range 0..1 → use 0.6). `call_export {export:"setInstrumentParam", args:[ID, K, targetV]}`. THEN immediately record: `checkpoint_export {name:"synth_param_changed", export:"getInstrumentParams", args:[ID], path:K, expect: targetV}` (the override now equals targetV exactly). Do not skip recording this checkpoint.
15. (Exercise) click a synth stage chip + the **Reset** footer button; observe. Report `ISSUE` if a knob/Save/Reset misbehaves.

## Phase 7 — Sampler
16. `switch_tab {slug:"sampler"}` → `checkpoint {name:"sampler_tab", expect:{activeTab:"sampler"}}`.
17. (Exercise) `read_export {export:"getInstrumentParams", args:[ID], path:"samplerTranspose"}`, then `call_export {export:"setInstrumentParam", args:[ID, "samplerTranspose", 5]}`, and confirm it changed. Report `ISSUE` if the Sampler tab renders nothing.

Stop playback. Report any issues as `ISSUE:` lines, output a one-line summary, and STOP.
