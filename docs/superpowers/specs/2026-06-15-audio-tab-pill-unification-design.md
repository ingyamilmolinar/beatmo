# Audio-Tab Pill Button Unification + Chain "X" Removal

Date: 2026-06-15
Branch: add-core-node-types
Status: Approved design — ready for implementation plan

## Goal

Make every small "pill" button in the four audio-panel tabs (Wave, Spectrum,
Levels, Chain) share **one component, one style, and one size policy** — and make
them **bigger and touch-friendly**, with **two sizes** (a larger desktop size and
a larger, 44px-touch-min mobile size) selected automatically by Density. Also
**remove the Chain tab's "X" (close) button** entirely.

## Decisions (from brainstorming)

- **One rounded renderer** for all pills (the analyzer tabs' `drawPillTabAt`,
  4px corners). Chain adopts it; its sharp-edge `drawPillButton`/`drawPillChrome`
  + `chainPillScale` text shrink are removed for the chrome pills.
- **Content-fit width** with uniform padding (width hugs each label; icon pills
  square; single-char pills clamped to the square minimum).
- **Bigger + two sizes** via the existing Density axis: desktop = Comfortable
  (~24px), mobile = Spacious (~30px visible) with hit areas expanded to the
  **44px touch-min**.
- **Delete the Chain "X"** (field, ctor, layout, draw, hit area, `OnClose`).

## Current state (as found)

- **Analyzer tabs (Wave/Spectrum/Levels)** — `audio_tab_controls.go` — already
  share `drawPillTabAt` (`audio_sticky_bar.go:335`): 4px rounded (`RadiusMD/2`),
  `colSurface2` fill + `colAccent` border (active), `colButtonBorder` outline
  (inactive), unscaled text. Height = `max(header.Dy()-8, AudioPillH)` (~18px
  desktop). Widths use four tokens: `AudioPillIconW` (icon), `AudioPillWideW`
  (text), `AudioPillNarrowW` ("R"), `AudioPillSlopeMinW` (slope).
- **Chain** — `chain_panel_zone.go` — uses its own `drawPillButton` +
  `drawPillChrome`: **sharp 0px corners** (4 manual `drawRect` borders), text
  shrunk by `chainPillScale()` (0.8–1.05×), **hardcoded 18px height**, widths
  `ChainModePillW` (OVR/SPL/DIF), `ChainAggregateW` (AG/FIT), `CloseButtonSize`
  (freeze/close). Pills: OVR, SPL, DIF, AG, FIT, freeze, **close(X)**.
- **Colors already agree** (both: `colSurface2`+`colAccent` active,
  `colButtonBorder` inactive) — the divergence is corners, text scale, height,
  and width tokens.
- **Density tokens** (`design_density.gen.go`, from DESIGN.md `densities:`):
  `AudioPillH` 16/18/22, `AudioPillIconW` 20/24/28, `AudioPillWideW` 24/28/32,
  `AudioPillNarrowW` 16/18/22, `AudioPillSlopeMinW` 38/44/52, `AudioPillPadX`
  10/12/14, `AudioPillGap` 2/3/4 (Compact/Comfortable/Spacious).
- **Headers:** `audioControlHeaderH = 26` (analyzer), `chainHeaderH = 26` (Chain).
- **Touch-min:** `TouchMinTarget()` returns 44 (`genSpacingTouchMin`).
- **Chain X (`closeBtn`):** field `chain_panel_zone.go:70`; ctor ~197 (`IconClose`,
  `OnClose`); layout ~906; draw `z.drawPillButton(dst, z.closeBtn, false, false)`
  ~673; hit area tag `scope-close-btn` ~1061; `OnClose` closes the audio panel.
- **Crop subjects** exist for Wave/Spectrum/Levels (`SubjectEQTabWave/Spectrum/Levels`,
  `screenshot_subjects.go`) but **none for Chain/Scope**.

## Design

### 1. One canonical pill renderer

`drawPillTabAt(dst, btn, active)` is *the* audio-pill renderer. Chain's chrome
pills (OVR/SPL/DIF/AG/FIT/freeze) draw through it instead of `drawPillButton`.
After migration, `drawPillButton` + `drawPillChrome` are unused by chrome pills;
remove them (verify they aren't used by Chain stage cards — those use
`drawStagePill`, separate). `chainPillScale` is no longer used by chrome pills.

### 2. One size policy — two shared helpers

Add to `audio_tab_controls.go` (used by **all four tabs**):

```
// audioPillHeight is the single height for every audio-panel pill, density-
// driven (two effective sizes: Comfortable desktop, Spacious mobile).
func audioPillHeight() int { return Profile().DensityValues().AudioPillH }

// audioPillWidth is content-fit: icon pills are square (= height); text pills
// are TextWidth(label) + 2*AudioPillPadX, clamped to a square minimum so a
// 1-char label ("R") stays comfortably tappable.
func audioPillWidth(btn *Button) int {
	h := audioPillHeight()
	if btn.Icon != "" || btn.Text == "" {
		return h // square icon pill
	}
	w := TextWidth(btn.Text) + 2*Profile().DensityValues().AudioPillPadX
	if w < h {
		w = h
	}
	return w
}
```

A shared right-to-left row placer (extend/replace `layoutRightPill`) positions
each pill with `audioPillHeight()`, `audioPillWidth(btn)`, and `AudioPillGap`,
vertically centered in the header. Every tab (analyzer + Chain) uses it, so the
four old analyzer width tokens and the Chain width tokens collapse into the one
`audioPillWidth` rule. (`AudioPillIconW/WideW/NarrowW/SlopeMinW` and
`ChainModePillW/ChainAggregateW` become unreferenced; see §6.)

### 3. Bigger pills + two sizes (Density)

Bump the height/padding tokens in DESIGN.md `densities:` and regenerate:
- **`AudioPillH`**: `16/18/22` → **`20/24/30`** (Compact/Comfortable/Spacious).
  Desktop pills ~24px, mobile ~30px.
- **`AudioPillPadX`**: `10/12/14` → **`12/16/20`** (roomier text pills).
- Headers grow to contain the taller pill (centered with ~4px top/bottom):
  `audioControlHeaderH` 26 → **32**; `controlHeaderHeight()` mobile 36 → **40**;
  `chainHeaderH` 26 → **32**. (`chainPanelH` may need a matching bump so the
  trace area isn't squeezed — verify.)

### 4. Mobile touch-min hit areas

On mobile, each pill's `HitArea` uses `Touch = true` + `ClipRect =` the pill rect
expanded to `TouchMinTarget()` (44px) — the codebase's standard way to meet the
touch target without growing the *visible* pill past the header band (don't
enlarge the visible Rect past bounds). A shared helper builds the hit area:

```
func audioPillHit(btn *Button, z int, tag string) HitArea {
	ha := HitArea{Rect: btn.Rect(), ZIndex: z,
		Handler: &buttonHitAdapter{btn: btn}, Tag: tag}
	if Profile().IsMobile() {
		ha.Touch = true
		ha.ClipRect = expandToTouchMin(btn.Rect()) // grows to >=44px, bounded
	}
	return ha
}
```
(Reuse the existing `expandToTouchMin` helper used by the EQ HPF/LPF pills.)

### 5. Remove the Chain "X" (close) button

Delete every `closeBtn` reference in `chain_panel_zone.go`: the struct field,
ctor construction, the layout `SetRect`, the `drawPillButton(... closeBtn ...)`
draw call, the `scope-close-btn` hit area, and the `OnClose` callback wiring
(and the `OnClose` field on `ChainCallbacks` if now unused — audit; leave the
field if other code sets it, but the Chain pill no longer renders/handles it).
Removing it also frees space in the mobile chrome row (helps §3 fit).

### 6. Token + dead-code cleanup (remove everything now unused)

**Nothing unused is left behind.** Every token and function that the unification
orphans is deleted in the same change:

- **Remove the orphaned width tokens** from DESIGN.md `densities:`, regenerate
  (`make gen-design-tokens`), and delete the corresponding Go fields/usages:
  `AudioPillIconW`, `AudioPillWideW`, `AudioPillNarrowW`, `AudioPillSlopeMinW`
  (analyzer side — replaced by `audioPillWidth`), and `ChainModePillW`,
  `ChainAggregateW`, plus `ChainAggregateH` and `ChainPillScale`/`chainPillScale`
  if the audit shows they're now unreferenced (Chain side). Verify each with a
  whole-package grep before removing; remove only those with zero remaining refs.
- **Remove the dead functions** once chrome pills migrate to `drawPillTabAt`:
  `drawPillButton`, `drawPillChrome`, and `chainPillScale` (confirm via grep that
  Chain stage cards use `drawStagePill`, not these). `pillGlyphColor` too if it
  becomes unreferenced.
- **`CloseButtonSize`**: audit — if the Chain close button was its only consumer,
  remove it as well (DESIGN.md + gen + Go); if other close buttons use it, keep it.
- **Value changes** to the surviving tokens (`AudioPillH`, `AudioPillPadX`) and
  **any token removal** require the same three-part update every time: DESIGN.md
  YAML, `make gen-design-tokens`, and the `design_md_drift_test.go` want-map (and
  `token_discipline` budgets if literal counts shift). This is the
  token-change checklist from CLAUDE.md — do all parts in one change.

Net effect: the four analyzer width tokens + the Chain width/scale tokens + the
two Chain draw functions are gone; the audio-pill look/size lives in exactly one
renderer (`drawPillTabAt`) + two sizing helpers + two surviving density tokens
(`AudioPillH`, `AudioPillPadX`, plus `AudioPillGap`).

## Components / boundaries

| Unit | Responsibility |
|---|---|
| `drawPillTabAt` | The one pill renderer (rounded, colors, icon/text). |
| `audioPillHeight()` / `audioPillWidth(btn)` | The one size policy, shared by all tabs. |
| `layoutRightPill` (extended) | Right-to-left pill placement with the shared size. |
| `audioPillHit(btn,z,tag)` | Hit area + mobile touch-min expansion. |
| `waveControls`/`levelsControls`/`spectrumControls` | Use the shared sizing + hit helper (drop per-button width tokens). |
| `ChainPanelZone` | Chrome pills draw via `drawPillTabAt`, sized via the shared helpers; close button gone. |

## Testing (robust)

Go fast path (`-tags test`), a new `audio_pill_unification_test.go`:
- **One renderer:** all four tabs' chrome pills render via `drawPillTabAt`
  (Chain no longer calls `drawPillButton` for chrome — assert by interception or
  by structural check that `drawPillButton`/`drawPillChrome` are gone/unused).
- **Uniform height:** every pill across all four tabs has height == `audioPillHeight()`
  for both Comfortable and Spacious densities (`SetDensityForTest`).
- **Content-fit width:** text pill width == `max(TextWidth+2*AudioPillPadX, h)`;
  icon pill width == height (square).
- **Bigger:** `audioPillHeight()` ≥ 24 (Comfortable) and ≥ 30 (Spacious).
- **Mobile touch-min:** every pill's published hit area (mobile) covers ≥ 44px in
  both dimensions (via `Touch`+`ClipRect`).
- **Chain X gone:** Chain publishes no `scope-close-btn` hit area; `closeBtn`
  field removed; the other Chain pills still publish + work.
- **Per-tab still functional:** AUTO/freeze/CLR/K20/log/slope/Pre/R and
  OVR/SPL/DIF/AG/FIT still toggle (drive via existing functional tests).
- **Discipline:** `TestDesignMDDrift`, `TestTokenDiscipline`, forbidden-glyph
  guard still pass after the token bumps **and removals**.
- **No dead code left:** a grep-style check (test or CI assertion) that
  `drawPillButton`, `drawPillChrome`, `chainPillScale`, and the removed width
  tokens have **zero references** in `internal/ui/` — so the cleanup is complete
  and can't silently regress. `go vet` / the build already fails on an unused
  import or undefined symbol; rely on a clean `go build` + an explicit grep in
  the final verification step.

Visual verification:
- **Screenshots:** capture `crop_eq_tab_wave/spectrum/levels` (existing) plus a
  **new `crop_chain` scene + `SubjectChain`** (follow the screenshot-subjects
  recipe in CLAUDE.md: const in `screenshot_subjects.go`, bounds from
  `SubjectRect`, public accessor on ChainPanelZone). Run `make screenshots-all
  SCENES=crop_eq_tab_wave,crop_eq_tab_spectrum,crop_eq_tab_levels,crop_chain`
  (desktop + `MOBILE=1`) and eyeball: pills are uniform, rounded, larger, and the
  Chain X is gone. `scene_crop_test.go` exercises the new crop scene automatically.
- **Browser render:** `audio_panel_render.browser.test.js` still green (tabs draw,
  desktop + mobile).

## Out of scope (YAGNI)

- No change to the pill colors or active-highlight semantics (already consistent).
- No change to Chain's stage cards (`drawStagePill`) or the trace.
- No new pill behaviors; this is look/size/consistency + the X removal only.
- No change to close buttons on *other* surfaces (top bar, sidebar, dialogs).
  (`CloseButtonSize` is removed only if the audit shows the Chain X was its sole
  consumer — see §6.)

## Risks / notes

- **Mobile Chain row fit:** unscaled content-fit pills are wider than the old
  shrunk ones; verify the Chain chrome row fits on a 360px phone (helped by the
  X removal + OVR/SPL/DIF already on a segmented row on mobile). If tight, the
  fallback is a slightly smaller Spacious `AudioPillPadX` — adjust the token.
- **Header/panel height bumps** must not clip the trace/content area — verify
  `chainPanelH` and the analyzer `bodyRect` still have room.
- **Concurrency:** another agent is actively editing `internal/ui` (i18n/synth)
  and has deleted test files mid-task — verify own work via `-run` filters, ride
  out transient `i18n.KeyXxx` breaks, recreate clobbered test files manually.
- **Token discipline:** bumping `AudioPillH`/`AudioPillPadX` requires DESIGN.md +
  drift-test want-map + `make gen-design-tokens` in the same change.
