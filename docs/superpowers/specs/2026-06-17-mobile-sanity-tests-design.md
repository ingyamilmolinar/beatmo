# Programmatic Mobile Sanity Tests + Instrument-Menu Mobile Bug Fix

**Date:** 2026-06-17
**Branch:** add-core-node-types
**Status:** design (approved scope)

## Goal

Create a programmatic mobile "sanity" browser test that exercises the same
functionality as the **agent** mobile sanity test (`src/js/llm_test/tests/mobile/
sanity_mobile.test.md`) — but **without** an LLM agent and **without** using JS
export functions as the *action* mechanism. Every user action is performed with
**real touch input** (CDP `Input.dispatchTouchEvent`) on the on-screen control;
exports are used only to *read* geometry (where to tap) and to *assert* resulting
state. Use this suite to find and fix a known bug in the **main instrument-menu
popup on mobile**.

## Non-negotiable principle: real input for actions

- **Actions** → real CDP touch (`cdpTap`, `cdpDrag`, `cdpLongPress`,
  `cdpPinch`, `cdpTwoFingerPan`, `cdpLongPressAndSlide` from
  `touch_cdp_helpers.js`) at pixel rects resolved from geometry exports.
- **Targeting / assertions** → geometry/state exports only
  (`rowLabelRect`, `instMenuItemRects`, `rowInstrument`, `nodeRect`,
  `playBtnRect`, `bpmIncBtnRect`, `subdivBtnRect`, `rowMuteBtnRect`,
  `fullLayoutSnapshot`, `camScale`, `isPlaying`, `totalRows`, `totalNodes`, …).
- **Forbidden as actions:** `set_bpm`, `setViewMode`/`setEQTab`, `importJSON`,
  `menu_click`, `openInstMenu`, `click_ui`, or any agent tool. (Reads are fine.)
- **No LLM / no `ANTHROPIC_API_KEY`.** Pure deterministic Playwright + CDP.

## Deliverable

One new file: `src/js/mobile_sanity.browser.test.js`.

- Viewport: Playwright `devices["iPhone 12"]` (390×844, DPR 3) + `hasTouch: true`.
- Build/server/setup: reuse the pattern in `inst_menu_mobile_touch.browser.test.js`
  (`buildMainWasm`, minimal static server, `chromium.launch`).
- Reuse `cdp*` helpers; do not add a new input layer.
- Self-contained pass/fail summary; `process.exit(1)` on any gating failure.
- Add to the JS suite per the JS Test Charter §6 (real input dispatch through the
  canvas, mobile). Register the export(s) it needs in
  `wasm_bridge_smoke.browser.test.js` if any are new.

The existing `inst_menu_mobile_touch.browser.test.js` stays as the focused
instrument-menu repro; the new file is the broader sanity suite and seeds its
instrument-menu scenario from that test's proven pattern.

## Scenarios (mirror sanity_mobile.test.md)

| # | Scenario | Real input | Assert (export) | Gating |
|---|---|---|---|---|
| 1 | Init | load @ iPhone 12 portrait | `totalRows == base` | yes |
| 2 | Play / Stop | tap `playBtnRect` / `stopBtnRect` | `isPlaying` true→false | yes |
| 3 | BPM | tap `bpmIncBtnRect` ×N | bpm increased by N (direction+magnitude, not exact 90) | yes |
| 4 | Subdiv | tap `subdivBtnRect` → tap a subdiv menu item | subdiv changed to tapped value | yes |
| 5 | Row mute toggle | tap `rowMuteBtnRect(1)` twice | muted false→true→false | yes |
| 6 | Bottom-nav views | tap eq / synth / pads segment rects | `viewMode` == each | yes |
| 7 | **Instrument menu** | tap `rowLabelRect(r)` → tap an item; **and** scroll list (`cdpDrag`) then tap a now-visible item | `rowInstrument(r)` == the id of the visually-tapped item, for BOTH the unscrolled and scrolled cases | yes |
| 8 | Node delete | `cdpLongPressAndSlide` on `nodeRect(0,0)` → Delete | `totalNodes` decreased | yes |
| 9 | Pinch zoom | `cdpPinch` out at grid center | `camScale` increased | yes |
| 10 | Two-finger pan | `cdpTwoFingerPan` over grid | `camOffset*` changed | yes |
| 11 | Audio audible | best-effort capture | RMS logged; **non-gating** | no |

Headless software-GL drops frames; each gating action uses bounded retry + settle
(mirror `inst_menu_mobile_touch.browser.test.js`'s retry loops). Anything that
cannot be made deterministic is logged, not silently skipped (no silent caps).

## Instrument-menu bug workflow

Order: **full suite first** (per user). The instrument-menu scenario — especially
the **scroll-then-select** case — is designed to surface the bug.

1. Build the suite, run it on the mobile viewport, capture the actual failure
   (wrong instrument selected / no selection / selection on wrong row).
2. **systematic-debugging**: confirm the true root cause against the live code.
   Leading hypothesis (must be verified, not assumed): the mobile deferred-tap
   path (`drumview_overlay_inst_comp.go` `fireTapAt`, ~lines 995-1035) hit-tests
   the stored tap against instrument-button rects that do **not** account for the
   menu scroll offset, unlike the context menu which subtracts `scrollPx`
   (`drumview_context_menu.go:194-240`). The per-axis dead-zone fix
   (`touch_scroll.go`) is already in, so this is a separate defect.
3. **Fix** in Go, mirroring the context-menu's scroll-aware hit-testing (single
   source of truth for item rects used by both draw and hit-test).
4. **Regression coverage**:
   - Fast deterministic **Go functional test** driving the *real touch state
     machine* (gesture detector + deferred tap + scroll), NOT `SetInputForTest`
     mouse (mouse-sim already passes — see
     `inst_menu_click_responsiveness_test.go:145-168`). Use the real-touch helper
     referenced at `inst_menu_click_responsiveness_test.go:251`.
   - The browser scenario #7 (scrolled select) goes green.

## Risks / mitigations

- **Headless audio flakiness** → audio is non-gating (decided).
- **Frame drops under SW-GL** → bounded retry + settle on every gating step;
  log-and-continue only for non-gating.
- **Missing geometry exports** (e.g. bottom-nav segment rects, subdiv menu item
  rects) → if absent, add a read-only `//go:build js && !test` export and register
  it in the bridge smoke catalogue; never add an action export.
- **Bug root cause differs from hypothesis** → systematic-debugging gates the fix;
  do not implement the scroll-offset fix until the repro confirms it.

## Out of scope

- Desktop audio-panel depth (sanity_audio_panel.test.md per-tab controls).
- Replacing or modifying the agent harness.
- Committing/staging (working-tree only, per project convention).

## Verification

- `GO=$(pwd)/.tools/go/bin/go node src/js/mobile_sanity.browser.test.js` → all
  gating scenarios pass.
- Go fast path for the new functional test:
  `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod
  -run TestInstMenuMobile ./internal/ui`.
- `inst_menu_mobile_touch.browser.test.js` still green.
