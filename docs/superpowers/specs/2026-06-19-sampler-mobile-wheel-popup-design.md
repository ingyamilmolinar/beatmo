# Sampler mobile knobs → tap-to-open wheel popup

**Date:** 2026-06-19
**Status:** Approved (design)
**Scope:** `src/go/internal/ui/` only. Mobile-only behavior change. Desktop untouched.

## Goal

On **mobile**, each sampler-tab knob becomes a tap-to-open vertical scroll-wheel
popup — the exact same `MobileWheelPopup` control the synth tab already uses for
its knobs. The existing single-column scroll-list layout of the sampler tab is
**kept**; only the per-knob interaction changes (tap opens a popup instead of an
in-place rotary drag).

On **desktop**, sampler knobs stay rotary dials. No desktop code path changes.

This is pure reuse of an existing control — no new widget type.

## Background

`MobileWheelPopup` (`mobile_wheel_popup.go`) is tab-agnostic. It stores no value
of its own and contains no synth/sampler-specific logic. Everything flows through
a `WheelBinding`:

```go
type WheelBinding struct {
    Knob         *Knob
    Badge        *KnobStepBadge // nil for discrete/enum params
    Def          audio.ParamDef
    Discrete     bool
    OnChange     func()
    OnCommit     func()
    OnResolution func()
    OpenEditor   func()
    Title        func() string
    Accent       func() color.Color
}
```

The synth path is:
`synthKnobHitAdapter.OnPress` → `if Profile().IsMobile() { dv.openSynthKnobWheelPopup(idx, instID) }`
→ `dv.synthWheelPopup.Open(binding, anchor, bounds, headerH)` + `openSynthWheelPortal()`
(portal is `Modal:true, Scrim:true`).

The sampler differs from synth only in that it has no per-knob `instParamBinding`
array; instead the `ParamDef` is built ad-hoc from `samplerKnobScale(idx)` and
`samplerStepPrefName(idx)`. The knob value model is `dv.sampler.knobs[idx]`
(normalized 0..1), applied through `dv.sampler.setKnob` / read via
`dv.sampler.knobValue`. All five sampler knobs (Start, End, Transpose/Pitch,
Detune/Fine, Gain) are **continuous** → `Discrete:false`, badge present.

## Changes (all in `internal/ui/`)

1. **New field** on `DrumView`: `samplerWheelPopup *MobileWheelPopup`, declared
   next to `synthWheelPopup` in `drumview.go`. Initialized in `drumview_ctor.go`
   with `NewMobileWheelPopup()`.

2. **New file `sampler_wheel_popup.go`** (mirrors `drumview_wheel_popup.go`):
   - `func (dv *DrumView) openSamplerKnobWheelPopup(idx int)` —
     - bounds-check `idx` against `samplerKnobCount` / `len(sampler.knobs)`;
     - `dv.CloseAllPopups()`;
     - `sc := samplerKnobScale(idx)`; build
       `def := audio.ParamDef{Name: samplerStepPrefName(idx), Label: <title>, Min: sc.Min, Max: sc.Max, Unit: sc.Unit}`;
     - `k := dv.sampler.knobs[idx]`; `badge := dv.sampler.knobStepBadges[idx]` (nil-safe);
     - build `WheelBinding`:
       - `Knob: k`, `Badge: badge`, `Def: def`, `Discrete: false`,
       - `Title: func() string { return <localized sampler knob name> }`,
       - `OnChange: func() { dv.applySamplerKnob(idx) }`,
       - `OnCommit: func() { dv.commitSamplerEdit() }`,
       - `OnResolution: func() { dv.persistKnobStep(badge) }`,
       - `OpenEditor: func() { dv.openSamplerParamEditor(idx) }`,
       - `Accent: nil`;
     - `anchor := k.Rect()`;
     - `dv.samplerWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)`;
     - `dv.openSamplerWheelPortal()`.
   - `func (dv *DrumView) closeSamplerKnobWheelPopup()` — close popup + portal.
   - `func (dv *DrumView) SamplerWheelPopupRect() image.Rectangle` — empty when
     nil/closed, else `samplerWheelPopup.Rect()` (parity with `SynthWheelPopupRect`,
     for screenshot cropping; cheap, keeps the two tabs symmetric).

   The popup **Title** uses the sampler knob's existing localized name. The
   sampler currently surfaces a group label (`samplerGroupLabel`) and a
   plain-English gloss (`samplerKnobPlainEnglish`). A short per-knob title helper
   `samplerKnobTitle(idx)` will return the localized short name (reusing the
   existing i18n gloss keys); the exact text is a presentation detail resolved in
   implementation, not a structural decision.

3. **Portal wiring** in `drumview_portal_open.go` — copy of the synth pair:
   - `openSamplerWheelPortal()` opens `PortalEntry{ ID:"sampler-wheel-popup",
     Overlay:&mobileWheelPopupPortalOverlay{popup:dv.samplerWheelPopup,
     tag:"sampler-wheel-popup"}, Modal:true, Scrim:true,
     OnClose:func(){ dv.samplerWheelPopup.Close() } }`.
   - `closeSamplerWheelPortal()` closes `"sampler-wheel-popup"`.

   `CloseAllPopups()` already drains all open portals (calling each entry's
   `OnClose`), so the popup is torn down automatically on any global close — no
   change needed there.

4. **The single branch** in `samplerKnobHitAdapter.OnPress`
   (`sampler_panel_zone.go`): immediately after the existing `paramEditor.Active()`
   guard, add:

   ```go
   if Profile().IsMobile() {
       h.dv.openSamplerKnobWheelPopup(h.idx)
       return InputConsumed
   }
   ```

   Everything below it (step-badge tap, readout tap, rotary `HandleInputResult`)
   remains the **desktop** path, unchanged.

## What does NOT change

- Desktop sampler knobs: rotary, in-place drag — byte-identical.
- Sampler tab layout (single-column scroll list, group labels, captions, badges).
- `samplerKnobScrollAdapter` (the scroll-list body/thumb scroll behavior).
- `MobileWheelPopup`, `WheelBinding`, the portal overlay, and all synth wiring.

## Testing (new `sampler_wheel_popup_test.go`, fast `-tags test` path)

All tests drive the **production tree dispatch** (not the dead handler directly),
forcing the mobile profile via `forceSmallScreenForTest = true` + `UpdateProfile()`
(per the project gotcha that `SetForceMobileProfile` is a no-op under `-tags test`).

1. **Mobile opens popup**: force mobile, lay out the sampler tab, dispatch a real
   press on a sampler knob's hit rect → assert `dv.samplerWheelPopup.IsOpen()`
   and the `"sampler-wheel-popup"` portal entry is open.
2. **Desktop does NOT open popup**: desktop profile, same press → popup stays
   closed; the rotary path captures (`InputCaptured`). Regression guard for
   "leave desktop untouched."
3. **Wheel drag mutates value + commits on release**: open popup, drag the barrel,
   assert the knob value changed and `commitSamplerEdit` ran exactly once per
   gesture (one undo step) on release.
4. **Center-tap opens numeric editor**: tap the popup's center value box → assert
   `dv.paramEditor.Active()` (via the `OpenEditor` callback → `openSamplerParamEditor`).
5. **Input isolation**: with the popup open, gestures over it do not leak to the
   grid / row scroll (mirror `synth_wheel_popup_isolation_test.go`).

## Risks / notes

- The sampler `ParamDef` is synthesized (no Label/Enum/Step source), so the popup
  title relies on a small new localized helper rather than `def.Label`. Low risk;
  the i18n keys already exist.
- `commitSamplerEdit` is the established one-undo-step-per-gesture site; reusing it
  as `OnCommit` keeps undo semantics identical to the rotary path.
- `audio.SetSampleEdit` is process-global (per memory) — tests must clear/rebaseline
  the sample edit to avoid cross-test bleed.
