# EQ Wheel-Popup Value Editor — Design

**Date:** 2026-07-02
**Status:** Approved (design), pending implementation plan

## Summary

Add the reusable mobile-style **wheel control with custom precision** (`MobileWheelPopup`,
already used by the synth and sampler tabs) to the **EQ audio-panel tab**. Tapping a band's
dB readout cell — on **both desktop and mobile** — opens the wheel popup instead of the inline
numeric text editor. Numeric text entry is preserved via the wheel's center-box tap
(exactly as synth/sampler already behave). The wheel and the existing EQ curve-drag handles
both write the **same source of truth** (`EQPanelZone.bandGainsDB`).

## Motivation

The EQ tab's per-band dB values are edited today by (a) dragging a handle on the response
curve, or (b) tapping the dB readout cell to open a numeric text input (`ParamValueEditor`).
The synth/sampler tabs offer a richer touch-first editor — a vertical wheel/barrel with a
selectable resolution strip (custom precision) — that the EQ tab lacks. Unifying on that
control gives EQ values the same precise, discoverable editing affordance, and removes a
divergent interaction path.

## Current state (as-explored)

- **Source of truth:** `EQPanelZone.bandGainsDB []float64` is the live working copy while a
  channel is selected, backed by `DrumView.masterGainsDB` (master) and `DrumRow.EQGainsDB`
  (per-instrument rows). Accessors: `dv.eqBandGainsDB()` / `eqBandMuted()`.
- **Write path (UI → audio):** edits fire the zone callbacks wired in `drumview_ctor.go`:
  `OnGainChange(band, db)` (per-frame live audio via `applyMasterEQ` / `applyRowEQ` →
  `audio.SetChannelEQ(...)`), `OnApplyEQ()`, and `OnEQBandCommit()` (one undo step +
  `emitEQBandChange`, fired once on gesture release via `commitEQBand`).
- **Existing numeric input:** `openEQDBEditor(band)` (`eq_panel_zone.go`) opens the shared
  `EQPanelZone.paramEditor` (`ParamValueEditor`) with `eqDBSpec()` — clamp **[-12, +12] dB**,
  step 0.1. Dispatched by `eqDBOpenAdapter.OnPress`.
- **Curve handles ("sliders"):** `curveHandleHitAdapter.OnDrag` writes `z.bandGainsDB[band]`
  and fires `OnGainChange`; commit/undo once on `OnRelease`.
- **Wheel popup:** `MobileWheelPopup` + `WheelBinding` (around a `*Knob` + optional
  `*KnobStepBadge`). Reads/writes the shared `*Knob`; never holds its own value. Opened via a
  `Modal:true, Scrim:true` overlay portal. The `"dB"` unit is already handled by
  `formatParamValueStep`. Currently wired **mobile-only** for synth (`openSynthKnobWheelPopup`)
  and sampler (`openSamplerKnobWheelPopup`).

## Design

### 1. Data flow & single source of truth

On open, build a **transient** `*Knob` seeded from `bandGainsDB[band]` (no persistent per-band
knobs — the EQ tab renders no rotaries). `WheelBinding`:

- `Knob` — transient, `Endless=true`, scale `Min:-12 Max:+12`, `StepMul = badge.Step()`.
- `Def` — `audio.ParamDef{Min:-12, Max:12, Unit:"dB", Label:<band label>}` (matches `eqDBSpec()`).
- `Badge` — `NewKnobStepBadge(def)`; **one shared** step preference for all EQ dB bands.
- `OnChange` — read `knob.Value` → `z.bandGainsDB[band] = v`; `z.curveDirty = true`; fire
  `OnGainChange(band, v)` + `OnApplyEQ()` (identical live path to curve-drag).
- `OnCommit` — fire `OnEQBandCommit()` (one undo step + emit, identical to curve-drag release).
- `OpenEditor` — call existing `openEQDBEditor(band)` (preserves numeric text entry).
- `OnResolution` — `dv.persistKnobStep(badge)`.
- `Title` — the band label; `Accent` — optional band color.

Both the wheel and the curve handles write `bandGainsDB` through the **same**
`OnGainChange`/`OnApplyEQ`/`OnEQBandCommit` callbacks, so the source of truth stays single and
audio/undo behavior is identical regardless of which surface edited the value.

### 2. Wiring (mirrors sampler)

- `DrumView.eqWheelPopup *MobileWheelPopup`, constructed in `drumview_ctor.go`.
- `openEQWheelPopup(band int)` — `CloseAllPopups()`, build the `WheelBinding` above,
  `dv.eqWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)` with `anchor =
  z.dbReadoutRect(band)`, then `openEQWheelPortal()`.
- `openEQWheelPortal` / `closeEQWheelPortal` — portal ID `"eq-wheel-popup"`,
  `&mobileWheelPopupPortalOverlay{popup: dv.eqWheelPopup, tag: "eq-wheel-popup"}`,
  `Modal:true, Scrim:true`.
- `eqDBOpenAdapter.OnPress` changes from `openEQDBEditor(band)` →
  `openEQWheelPopup(band)`, **not** gated by `Profile().IsMobile()` (both platforms).

Input isolation is inherited from the modal+scrim portal; `modalOverlayActive()` →
`PortalHasBlocking()` already gates grid tap / camera pan / pinch / two-finger pan.

### 3. Desktop sizing & shared step preference

- The popup sizes from density tokens `MobileWheelW/MobileWheelH`. These exist for all
  densities (desktop→comfortable). Verify the comfortable values render a sane panel on
  desktop; if not, adjust via DESIGN.md → `make gen-design-tokens` following token discipline.
- Use **one shared** EQ-dB step-resolution preference (keyed e.g. `"eq_band_gain"`), since all
  10 bands share range/unit. Chosen precision persists across bands and sessions.

## Testing

Go fast-path tests in `internal/ui` (`-tags test`), written test-first:

1. **Trigger parity (both platforms):** pressing a dB readout cell opens `eq-wheel-popup`
   under both desktop (comfortable) and mobile (spacious) profiles.
2. **Single source of truth:** a wheel `OnChange` and a curve-handle `OnDrag` on the same band
   both mutate `bandGainsDB[band]` and both fire `OnGainChange`/`OnApplyEQ`; value read back is
   identical regardless of surface.
3. **Audio apply:** wheel change on master → `applyMasterEQ` → `audio.SetChannelEQ("main", …)`;
   on a row channel → `applyRowEQ`.
4. **Commit/undo:** release fires `OnEQBandCommit` exactly once → one `EventEQBandChange` undo step.
5. **Numeric entry preserved:** wheel center-box tap invokes `openEQDBEditor` (`ParamValueEditor`).
6. **Input isolation:** popup `Modal`+`Scrim`; `modalOverlayActive()` true; grid/camera
   gestures suppressed (mirror `synth_wheel_popup_isolation_test.go`).
7. **Clamp:** wheel value stays within [-12, +12].

## Out of scope / YAGNI

- No new per-band knob rendering in the EQ panel (wheel is a transient editor only).
- No change to the curve-drag handles, HPF/LPF handles, mute buttons, or channel selector.
- No per-band step preferences (one shared EQ-dB step).
- No change to import/export or the JSON schema (source of truth arrays unchanged).

## Files touched (anticipated)

- `drumview.go` — `eqWheelPopup` field.
- `drumview_ctor.go` — construct `eqWheelPopup`.
- `eq_panel_zone.go` — `eqDBOpenAdapter.OnPress` → wheel; `openEQWheelPopup`.
- `drumview_portal_open.go` — `openEQWheelPortal` / `closeEQWheelPortal`.
- New/extended tests in `internal/ui`.
- Possibly `DESIGN.md` + regenerated tokens if desktop wheel sizing needs a token.
