# Mobile Synth & Sampler chrome — Implementation Plan

> **For agentic workers:** implement task-by-task with TDD. Steps use `- [ ]`.

**Goal:** On mobile, remove the redundant Synth/Sampler identity banner and the in-panel action-button rows, relocate Preview/Save + a `⋯`(Save As/Reset) overflow onto the always-present sticky bar, and stop truncating stage-pill labels — recovering vertical space with maximal reuse of existing infrastructure. Desktop unchanged.

**Architecture:** All changes gated on `Profile().IsMobile()`. Reuse: `IconOverflow` kebab icon, `ComponentButtonDestructive`/`colError` for Reset, the `eqChannelDropdownOverlay`+`OverlayPortal` pattern for the `⋯` menu, existing DrumView callbacks (`previewActiveSynth`/`SaveActiveRecipe`/`openSaveAsDialog`/`ResetActiveRecipe` and the `sampler-*` tag buttons), the sticky bar's existing button/hit-area machinery, and the `sectionShortLabelKey`+i18n pattern.

**Tech Stack:** Go 1.26, Ebiten, `-tags test` fast path.

## Global Constraints (verbatim from spec)

- Mobile-only; desktop layout untouched.
- No change to the pane-split algorithm (`game_layout.go`).
- Reuse existing infrastructure — no new parallel mechanisms.
- Design system: `IconID`+`DrawIcon` (no raw glyphs), Reset uses `destructive` role, touch-min via `Touch`+`ClipRect`, no new `color.RGBA` literals.
- Bundled Go only: `.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/i18n/`.

---

## Task 1: Stage-pill short labels (ENS / FRM / RES)

**Files:**
- Modify: `src/go/internal/i18n/keys.go` (add 3 consts + extend ordering slice ~:737)
- Modify: `src/go/internal/i18n/catalog_en.go:~404`, `catalog_es.go:~407`
- Modify: `src/go/internal/ui/synth_panel_zone.go:267` (`sectionShortLabelKey`)
- Test: `src/go/internal/ui/synth_stage_short_label_test.go` (new)

**Steps:**
- [ ] Add consts: `KeySynthStageEnsembleShort Key = "synth.stage.ensemble.short"`, `...FormantShort = "synth.stage.formant.short"`, `...ResonatorShort = "synth.stage.resonator.short"`. Append all three to the ordering `[]Key` slice near keys.go:737.
- [ ] en catalog: `KeySynthStageEnsembleShort: "ENS", ...FormantShort: "FRM", ...ResonatorShort: "RES"`. es catalog: same three (`ENS`/`FRM`/`RES` — short technical labels stay identical across locales, matching `ENV`/`F.ENV` precedent).
- [ ] Extend `sectionShortLabelKey` switch with the three new cases returning the new keys.
- [ ] Test `TestStageChipShortLabelNoEllipsis`: for ENSEMBLE/FORMANT/RESONATOR at a narrow `availPx` (e.g. the mobile chip width where the full word overflows), assert `sectionLabelChipFitted(id, availPx)` returns the short label and never contains `"…"`/`"..."`. Cover EN + ES via `i18n.SetLocale`.
- [ ] Run i18n completeness + ui tests; commit.

## Task 2: Sticky bar `SetTrailingActions`

**Files:**
- Modify: `src/go/internal/ui/audio_sticky_bar.go` (new field + setter; layout in `Layout`; hit areas in `rebuildHitAreas`; draw in `Draw`)
- Test: `src/go/internal/ui/audio_sticky_bar_trailing_test.go` (new)

**Design:** `trailingBtns []*Button` set via `SetTrailingActions(btns []*Button)` before `Layout`. In `Layout` (mobile only — desktop right cluster is the tab strip), pack them right-to-left starting at `rightEdge`, left-clamped so they never cross `channelBtn.Rect().Max.X`; each sized to `StyledTextWidth(text,RoleCaption)+AudioPillPadX` (icon-only `⋯` uses a square `btnH` cell). Register each in `rebuildHitAreas` at `parentZIndex+1` with `Touch=true`, `ClipRect=expandToTouchMin(rect)` (mirror `addChannelBtn`). Draw each via its `Button.Draw` after the tab pills. Empty/desktop → no-op.

**Steps:**
- [ ] Test `TestStickyBarTrailingActionsMobileLayoutAndHits`: build bar, force mobile, `SetTrailingActions([3 dummy buttons])`, `Layout(...)`; assert all 3 get non-empty rects inside the bar, right-aligned, none overlapping the channel pill, and each appears in `HitAreas()`. Desktop: assert `SetTrailingActions` is ignored (rects stay empty).
- [ ] Implement field/setter/layout/hits/draw. Reuse density tokens (`AudioPillPadX`, `AudioPillGap`, `AudioPillH`).
- [ ] Run ui tests; commit.

## Task 3: Synth panel — mobile banner + actions relocation

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`buildSynthTab` headerH; `layoutSynthHeader`/`drawSynthHeader` mobile skip; new persistent `MobileActionButtons()`)
- Modify: `src/go/internal/ui/synth_panel.go` or panel struct file (persistent mobile buttons + overflow menu opener)
- Reuse: `synth_wheel_popup`/`portal()` + a small overlay modeled on `eqChannelDropdownOverlay`
- Test: rewrite `src/go/internal/ui/synth_header_fixed_actions_test.go`

**Design:**
- `buildSynthTab` mobile: `headerH = 0` (no banner band); `layoutSynthHeader` returns immediately on mobile (no caption/waveform/action rects); `drawSynthHeader` skips on mobile. Desktop path unchanged.
- Persistent on `synthPanel`: `mobilePreviewBtn`, `mobileSaveBtn`, `mobileOverflowBtn` (built once). `MobileActionButtons() []*Button` refreshes `mobileSaveBtn` spec (Primary if `synthSaveDirty` else Secondary), sets `mobileOverflowBtn.Icon = string(IconOverflow)`, and returns `[preview, save, overflow]`. Callbacks reuse `dv.previewActiveSynth` / `dv.SaveActiveRecipe`. Overflow OnClick → `dv.openSynthActionMenu()`.
- `openSynthActionMenu()`: open a `PortalEntry` (ID `"synth-action-menu"`, `Scrim:true`, `Anchor: mobileOverflowBtn.Rect()`) whose overlay is a 2-row menu — **Save As…** → `dv.openSaveAsDialog()`, **Reset** → `dv.ResetActiveRecipe()` styled destructive (`ComponentContextMenuItemDestructive` / `colError`). Model the overlay on `eqChannelDropdownOverlay`.

**Steps:**
- [ ] Rewrite the obsolete `TestSynthHeaderActionsAlwaysVisibleNoOverflow` (4-visible assumption) → `TestSynthMobileActionsOnStickyBar`: after `setupMobileSynthTab`, assert `MobileActionButtons()` returns exactly `[preview, save, overflow]`, overflow's `Icon == string(IconOverflow)`, and the in-panel `SynthTabButtons()` no longer contains Save As/Reset on mobile. Add `TestSynthActionMenuOpensSaveAsReset`: invoke overflow OnClick, assert the portal has `"synth-action-menu"` with Save As + Reset items (Reset destructive).
- [ ] Implement mobile-gating in `buildSynthTab`/`layoutSynthHeader`/`drawSynthHeader`.
- [ ] Add persistent mobile buttons + `MobileActionButtons()` + `openSynthActionMenu()` + overlay.
- [ ] Run ui tests; commit.

## Task 4: Sampler panel — mobile title + actions relocation

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (`drawSamplerTab` title skip on mobile; `buildSamplerTab` band; `layoutSamplerControls` mobile drops `actionR`; `MobileActionButtons()`)
- Test: rewrite `src/go/internal/ui/sampler_mobile_actions_test.go`

**Design:**
- Mobile: `drawSamplerTab` skips the `KeyCapSamplerTitle` draw; header band still hosts nothing on mobile (buttons already cleared by `layoutSamplerHeaderButtons`), so shrink/skip the header band on mobile so the waveform rises.
- `layoutSamplerControls` mobile: keep the Rev/Norm/Fade `toggleR`; DROP the `actionR` (Preview/Save/Save As/Reset) — that space returns to the knob grid. Reclaim the row height in `samplerControlMinHeight` (subtract one `SynthHeaderButtonH` row on mobile).
- `MobileActionButtons()` returns `[samplerButtonByTag("sampler-preview"), samplerButtonByTag("sampler-save"), mobileOverflowBtn]`; overflow opens `"sampler-action-menu"` → Save As (`sampler-save-as` action) / Reset (`sampler-reset` action, destructive).

**Steps:**
- [ ] Rewrite `sampler_mobile_actions_test.go`: assert mobile `MobileActionButtons()` = `[sampler-preview, sampler-save, overflow]`; assert `sampler-save-as`/`sampler-reset` in-panel rects are empty on mobile; assert overflow menu opens both. Assert title not drawn on mobile (via a draw-intercept or a `samplerTitleVisible()` predicate returning false on mobile).
- [ ] Implement mobile-gating; reclaim knob-grid height.
- [ ] Run ui tests; commit.

## Task 5: EQPanelZone wiring (compute trailing actions per tab)

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go:~1223` (before `stickyBar.Layout`) and Draw path `:~824`
- Test: `src/go/internal/ui/eq_panel_sticky_actions_test.go` (new) or extend Task 3/4 tests via full `Game.Update`

**Design:** before `z.stickyBar.Layout(...)`:
```go
var trailing []*Button
if Profile().IsMobile() {
    switch z.tabState.ActiveTab() {
    case TabSynth:
        trailing = z.drum.synthPanelMobileActions()
    case TabSampler:
        trailing = z.drum.samplerPanelMobileActions()
    }
}
z.stickyBar.SetTrailingActions(trailing)
```
(`synthPanelMobileActions`/`samplerPanelMobileActions` are thin DrumView forwarders to the panels' `MobileActionButtons()`.)

**Steps:**
- [ ] Test `TestStickyBarShowsSynthAndSamplerActionsOnMobile`: drive full `Game.Update` on Synth tab (mobile), assert the sticky bar's `HitAreas()` include the 3 action buttons with non-empty rects inside the bar; switch to Sampler, assert the same; switch to Wave, assert none.
- [ ] Wire the trailing-actions computation in Layout (and ensure buttons render in Draw).
- [ ] Run ui tests; commit.

## Task 6: Verify + screenshots

**Steps:**
- [ ] Full fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/i18n/`.
- [ ] Design guards: `-run 'TestDesignMDDrift|TestTokenDiscipline'` on `./internal/ui/`.
- [ ] Rebuild WASM (`make wasm`) and re-capture `mobile_bottom_nav_synth` + `mobile_bottom_nav_sampler`; visually confirm banner gone, `[Preview][Save][⋯]` on the sticky bar, no `…` on stage pills, larger editor region, `⋯` menu shows Save As + red Reset.
- [ ] Send before/after screenshots to the user.

## Self-review notes
- Spec coverage: banner (T3,T4) · actions+kebab (T2,T3,T4,T5) · destructive Reset (T3,T4) · short labels (T1) · touch-min (T2) · desktop untouched (mobile gates in T3/T4, desktop trailing=nil in T5) · split untouched (headerH→0 only lowers content floor). ✓
- Obsolete tests explicitly rewritten (T3 `TestSynthHeaderActionsAlwaysVisibleNoOverflow`, T4 `sampler_mobile_actions_test.go`) — not left red.
- Reuse: no new icon, no new button role, no new menu framework (portal + eqChannelDropdown pattern).
