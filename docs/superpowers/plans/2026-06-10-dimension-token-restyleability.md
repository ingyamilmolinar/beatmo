# Dimension-Token Restyleability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the hardcoded sizing/position/radius literals in the UI chrome and audio-panel tab components into DESIGN.md design tokens so every component (especially the tab components) is re-styleable from one place, satisfying the "no component should hardcode size/position/style" rule.

**Architecture:** Beatmo already governs *colors/spacing/rounded/animations/geometry* via DESIGN.md YAML front matter → `make gen-design-tokens` → `design_*.gen.go` → drift/discipline guards. Control *sizing* lives in the `densities:` block (Compact/Comfortable/Spacious) read via `Profile().DensityValues().XxxField`; layout-tied divergence lives in `profileOverrides.desktop/mobile`. This plan extends those existing token systems — it introduces NO new architecture. Each phase: edit DESIGN.md → `make gen-design-tokens` → update the drift want-map → replace literals with token reads → TDD a guard/invariant test → run token guards.

**Tech Stack:** Go 1.23, DESIGN.md YAML + `gen_design_tokens` codegen, `-tags test` fast path.

**Hard rules carried from the codebase:**
- Bundled Go only: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ...`.
- After any DESIGN.md edit run `make gen-design-tokens` and update the want-map in `internal/ui/design_md_drift_test.go` for any **drift-mapped** token (colors, spacing, rounded, geometry, animations). Density/profile tokens are guarded by `TestDesignTokensFreshness` (regen-and-compare), not the drift want-map.
- Token guards that must stay green every phase: `TestDesignMDDrift`, `TestTokenDiscipline`, `TestDesignTokensFreshness`, `TestDesignMDLintSnapshot`.
- Baseline: full `internal/ui` sits at exactly 8 pre-existing failures (mobile-touch/BPM/synth-knob flakes). No phase may add a 9th.
- DO NOT tokenize: proportional ratios (0.48, 0.5, 60/100 splits), audio/DSP constants (dbSpan=24, 20Hz/22kHz, ADSR shape), ±1 pixel-snap nudges, loop bounds, sentinels, math (Taylor terms).

---

## File Structure

| File | Responsibility | Phases |
|---|---|---|
| `DESIGN.md` (YAML front matter) | Source of truth for new `rounded.xxs/xs`, `densities:` sizing tokens, `profileOverrides` header heights | all |
| `src/go/internal/ui/design_*.gen.go` | Generated — never hand-edit | all (regen) |
| `src/go/internal/ui/theme.go` / `touch_sizes.go` | `RadiusXXS`/`RadiusXS` Go aliases | 0 |
| `src/go/internal/ui/design_md_drift_test.go` | Drift want-map for rounded/spacing/geometry | 0,2,6 |
| `src/go/internal/ui/rounded_literal_discipline_test.go` | **NEW** — ratchet guard: no bare-int radius literal in `drawRoundedRect*` calls in budgeted files | 0 |
| `game_node_sidebar.go` | Sidebar dims + `sidebarTextScale` density token | 1,2 |
| `transport_zone.go` | Transport chevron/insets/group-radius | 2 |
| `audio_sticky_bar.go` | Tab chrome pill sizing (shared audio-chrome tokens) | 3 |
| `chain_panel_zone.go` | Chain tab stage-card gaps/badges | 4 |
| `synth_panel_zone.go`, `synth_preview_layer.go`, `sampler_panel_zone.go` | Synth/Sampler tab sizing + header-height profileOverrides | 5 |
| `render_meters.go`, `render_spectrum.go`, `render_waveform.go`, `eq_panel_zone.go` | Tab render content: shared `audioLabelMarginW`, row/strip heights | 6 |

---

## Phase 0 — Radii scale + discipline ratchet (foundational)

**Files:**
- Modify: `DESIGN.md` (`rounded:` block)
- Modify: `src/go/internal/ui/theme.go` (add `RadiusXXS`, `RadiusXS` aliases)
- Modify: `src/go/internal/ui/design_md_drift_test.go` (`wantRounded` map)
- Create: `src/go/internal/ui/rounded_literal_discipline_test.go`
- Modify call sites (bare-int radius → token): `render_meters.go:700-701` (4), `game_node_sidebar.go:1303` (3→xxs), `:1502,:1519` (6), `synth_panel_zone.go:1424-1425,1676-1677,1787-1788` (8→RadiusSM), `:1475-1476,1533-1534,1628-1629,1760` (6→RadiusXS), `sampler_panel_zone.go:619` (8→RadiusSM), `:625` (6→RadiusXS)

**Decision (locked):** ADD below the current scale — non-disruptive, no remap of `sm:8`:
```yaml
rounded:
  xxs: 4
  xs: 6
  sm: 8   # existing
  md: 12  # existing
  ...
```
Go: `RadiusXXS = 4`, `RadiusXS = 6`. Map literals: 3→`RadiusXXS`, 4→`RadiusXXS`, 6→`RadiusXS`, 8→`RadiusSM` (already exists).

- [ ] **Step 1 (RED): discipline test.** Create `rounded_literal_discipline_test.go`: walk a budget map of files; for each, regex-match `drawRoundedRect(` / `drawRoundedRectBorder(` calls whose radius argument (the int positional arg before the trailing bool) is a **bare integer literal** (not `Radius*`, not an identifier/expression). Budget map starts at the **current** counts (e.g. `render_meters.go:2`, `synth_panel_zone.go:8`, `sampler_panel_zone.go:2`, `game_node_sidebar.go:3`). Assert `count <= budget` AND `count >= budget` (ratchet, mirroring `token_discipline_test.go`). Run: expect PASS at current counts (characterization).
- [ ] **Step 2:** Add `rounded.xxs:4`, `rounded.xs:6` to DESIGN.md; `make gen-design-tokens`; add `RadiusXXS`/`RadiusXS` aliases in `theme.go`; add both to `wantRounded` in `design_md_drift_test.go`. Run `TestDesignMDDrift|TestDesignTokensFreshness` → PASS.
- [ ] **Step 3:** Replace the bare-int radius literals listed above with `RadiusXXS`/`RadiusXS`/`RadiusSM`. After each file, decrement that file's budget in the discipline test to the new count. The ratchet's `count >= budget` lower-bound proves you actually removed them.
- [ ] **Step 4:** Drive budgets to `0` for fully-converted files. Run the discipline test + `TestTokenDiscipline` + `TestDesignMDDrift|Freshness|LintSnapshot` → all PASS.
- [ ] **Step 5:** Commit `feat(ui): add rounded.xxs/xs tokens + radius-literal discipline ratchet`.

---

## Phase 1 — `sidebarTextScale` → density token (A5)

**Files:** `DESIGN.md` (`densities:`), `game_node_sidebar.go`, `design_density.gen.go` (regen).

`sidebarTextScale = 1.2` is a package const used at 30+ sites — it cannot scale per density today.

- [ ] **Step 1 (RED):** Add `TestSidebarTextScaleIsDensityToken` asserting `Profile().DensityValues().SidebarLabelScale > 0` and that all three density tiers define it. Run → FAIL (field undefined).
- [ ] **Step 2:** Add `sidebar-label-scale` to all three `densities:` tiers (Compact 1.1 / Comfortable 1.2 / Spacious 1.3 — Comfortable preserves today's 1.2). `make gen-design-tokens`.
- [ ] **Step 3:** Replace the `sidebarTextScale` const reads with `Profile().DensityValues().SidebarLabelScale` (keep a local `scale := Profile().DensityValues().SidebarLabelScale` at the top of each function to avoid 30 calls). Delete the const.
- [ ] **Step 4:** Run test → PASS; run sidebar tests + `TestDesignTokensFreshness` → PASS.
- [ ] **Step 5:** Commit `feat(ui): sidebar label scale is a density token`.

---

## Phase 2 — Sidebar + transport dimensions

**Files:** `DESIGN.md`, `game_node_sidebar.go`, `transport_zone.go`, drift want-map (for any spacing/geometry tokens).

Tokenize the inline literals the audit pinned. Prefer REUSE of existing `SpaceXS(3)/SM(6)/MD(10)` and the new `RadiusXXS/XS` before minting:
- `game_node_sidebar.go`: value-pill width 50 (lines 291/330/340/374) → new density `SidebarValuePillW`; swatch→label gap 6 (1304) → `SpaceSM`; badge pad 6/2 (1350/1351) → `SpaceSM`/new `SpaceXXS(2)`; dropdown gap 2 (321/368) → `SpaceXXS`.
- `transport_zone.go`: `chevSide=9` (1731) → geometry `subdiv-chevron-size`; chevron inset 3 (1732) → `SpaceXS`; `minW=48` (1813) → reuse touch-min (`densities.minTarget`/44 or new `transport-min-btn-w`); `RadiusMD+2` mobile group (1755) → geometry `transport-group-radius-mobile:10`; BPM gaps 4/2 (1621/1623) → `SpaceXS`/`SpaceXXS`.

**Decision:** add `spacing.xxs: 2` (additive) for the recurring 2px micro-gaps; add geometry `subdiv-chevron-size:9`, `transport-group-radius-mobile:10`.

- [ ] **Step 1 (RED):** `TestSidebarTransportTokensExist` — assert the new spacing/geometry/density tokens are non-zero. FAIL.
- [ ] **Step 2:** Add tokens to DESIGN.md (`spacing.xxs`, `geometry.subdiv-chevron-size`, `geometry.transport-group-radius-mobile`, `densities.sidebar-value-pill-w`); `make gen-design-tokens`; extend `wantSpacing`/geometry want-maps in `design_md_drift_test.go`.
- [ ] **Step 3:** Replace the listed literals with the tokens/`Space*`. Leave ±1 nudges and ratios alone.
- [ ] **Step 4:** Run sidebar + transport + token-guard tests → PASS.
- [ ] **Step 5:** Commit `feat(ui): tokenize sidebar + transport dimensions`.

---

## Phase 3 — Audio sticky bar (tab chrome) — re-styleable pills

**Files:** `DESIGN.md` (`densities:`), `audio_sticky_bar.go`, freshness guard.

The ~24 pill literals collapse into a SMALL shared **audio-chrome** token set (consolidation, not 24 tokens):
- `audio-pill-h` (button height; replaces the `-8`/`18`-min math) — density 18/22/26.
- `audio-pill-pad-x` (text pill horizontal pad; replaces the `+10/+12`) — 8/10/12.
- `audio-pill-gap` (inter-pill gap; replaces the recurring `3`/`2`) — 2/3/4.
- `audio-icon-pill-w` (fixed-width icon pills: close/freeze/expander/legend/reset — 18/20/24 family) — density 20/24/28.
- `audio-text-pill-min-w` (slope/tab min width — 28/44) — density 28/36/44.
- `audio-channel-pill-min-w` (72) — density 56/64/72.

- [ ] **Step 1 (RED):** `TestAudioStickyBarTokensExist` + `TestAudioStickyBarPillsFitInBar` (lay out the bar at 3 widths; assert every published pill rect is inside the bar rect and ≥ touch-min on mobile). FAIL on the missing token fields.
- [ ] **Step 2:** Add the 6 density tokens (all tiers) to DESIGN.md; `make gen-design-tokens`.
- [ ] **Step 3:** Replace the per-pill literals in `audio_sticky_bar.go` with `Profile().DensityValues().AudioPill*` reads. Keep `TextWidth(...)` content-fit math (that's correct, not a hardcoded size).
- [ ] **Step 4:** Run `TestAudioStickyBar*` (incl. existing `audio_sticky_bar_phase1_test.go`) + freshness → PASS.
- [ ] **Step 5:** Commit `feat(ui): tokenize audio sticky-bar pill sizing (re-styleable tab chrome)`.

---

## Phase 4 — Chain tab stage cards

**Files:** `DESIGN.md`, `chain_panel_zone.go`, freshness guard.

Chain already has many density tokens (`ChainStageColW`, `ChainMiniMeterW`, …). Remaining literals are small gaps/pads → reuse `Space*` + a couple of new tokens:
- content gap 8 (610/964) → new `chain-content-gap` density 6/8/10.
- stage-col pad 4/4 (916/917), chrome row pad 4 (927), row gaps 2 (922/963) → `SpaceXS`/`SpaceXXS`.
- badge pad 3/1 (875) → `SpaceXS`/keep-1 (1px = nudge).
- right margins 6 (928/683) → `SpaceSM`.

- [ ] **Step 1 (RED):** `TestChainTokensExist` + `TestChainStageCardsFitColumn` (stage cards stay within the stage column rect at 3 densities). FAIL.
- [ ] **Step 2:** Add `chain-content-gap` (all tiers); regen.
- [ ] **Step 3:** Replace literals with tokens; keep `chainBadgeInset` const + the 1px badge nudge.
- [ ] **Step 4:** Run chain tests (`chain_render_two_traces_test.go` + new) + freshness → PASS.
- [ ] **Step 5:** Commit `feat(ui): tokenize chain tab stage-card spacing`.

---

## Phase 5 — Synth + Sampler tabs (incl. profileOverrides)

**Files:** `DESIGN.md` (`densities:` + `profileOverrides:`), `synth_panel_zone.go`, `synth_preview_layer.go`, `sampler_panel_zone.go`.

- **profileOverrides** (layout-tied desktop/mobile divergence — these genuinely belong here, not densities): `synth-header-height` (desktop 48 / mobile 38), `synth-caption-max-w` (240/200).
- **densities**: `synth-section-pad-x`(8)→reuse `SpaceMD`; `synth-section-pad-y`(6)→`SpaceSM`; `synth-section-title-h`(16); `synth-overflow-chevron-w`(28); `synth-chip-state-w`(12); `synth-preview-target-w`(280); `synth-preview-min-w`(160); `synth-preview-min-graph-h`(18); `synth-preview-out-row-h`(20); `synth-preview-env-margin`(2)→`SpaceXXS`; `sampler-waveform-min-h`(36); `sampler-min-body-h`(48). Keep waveform % ratios (45/56/100) as proportional.

- [ ] **Step 1 (RED):** `TestSynthSamplerTokensExist` + `TestSynthHeaderHeightFollowsProfile` (desktop 48 vs mobile 38 via `Profile()`), + `TestSynthPreviewPlotsFitPane` (preview plots stay inside the pane at min height). FAIL.
- [ ] **Step 2:** Add tokens (densities + the 2 profileOverrides pairs); regen; extend `profileOverrides` want-map if guarded.
- [ ] **Step 3:** Replace literals; route `synthHeaderHeightDesktop/Mobile` through `Profile().IsMobile()` + the profileOverride field; keep ADSR/DSP/sample-count constants.
- [ ] **Step 4:** Run synth + sampler + preview tests (`synth_preview_layer_test.go`, `synth_param_display_test.go`, chip-strip tests) + alloc discipline (`eq_panel_draw_alloc_discipline_test.go` TabSynth) → PASS; re-snapshot `TestSynthLayoutFingerprintStable` if it moves.
- [ ] **Step 5:** Commit `feat(ui): tokenize synth/sampler tab sizing + header-height profileOverrides`.

---

## Phase 6 — Render content: shared label margin + strip heights

**Files:** `DESIGN.md`, `render_meters.go`, `render_spectrum.go`, `render_waveform.go`, `eq_panel_zone.go`, drift want-map.

- Consolidate the triplicated **28px dB/amplitude left margin** (`render_meters.go:175`, `render_spectrum.go:177`, `render_waveform.go:57`) into ONE `densities.audio-label-margin-w` (24/28/32). Highest-impact consolidation.
- Row heights: meters `header 12/14`, `footer 18` → `audio-readout-row-h`/`audio-label-row-h`; strip gap 4 (420/909) → `SpaceXS`/new `audio-strip-gap`; master pad 8 (421) → `SpaceSM`.
- spectrum `freq-label-min-h 14` (181) → `audio-label-row-h`.
- eq `hpfLPFSubStripH 22` desktop / `40` mobile (742/748) → density `eq-filter-strip-h` + profile, label gap `+6` (772) → `SpaceSM`; glow radius `16` (2207) → density `eq-handle-glow-r`.

- [ ] **Step 1 (RED):** `TestAudioRenderTokensExist` + `TestAudioLabelMarginShared` (all three render files resolve to the same `audio-label-margin-w`). FAIL.
- [ ] **Step 2:** Add tokens (densities + want-map for any spacing); regen.
- [ ] **Step 3:** Replace the literals; keep `0.48`/`0.5`/`60-100` ratios and the dB/Hz audio constants.
- [ ] **Step 4:** Run `render_meters_layout_test.go`, `render_spectrum_*_test.go`, `render_waveform_phase2_test.go`, `audio_panel_render_pixels_test.go`, the Phase-4-of-this-plan invariant tests + freshness → PASS.
- [ ] **Step 5:** Commit `feat(ui): consolidate audio render label-margin + strip heights into tokens`.

---

## Phase 7 — Close-out: verification + ratchet lock

**Files:** none new; verification.

- [ ] **Step 1:** Full fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/templates/ ./internal/assets/` — assert exactly the 8 pre-existing baseline failures, no 9th.
- [ ] **Step 2:** `make gen-design-tokens && git diff --exit-code src/go/internal/ui/design_*.gen.go` — no uncommitted gen drift.
- [ ] **Step 3:** Confirm the rounded-literal discipline budgets are all at their floor (ratchet can only tighten).
- [ ] **Step 4:** Commit `chore(ui): verify dimension-token restyleability pass`.

---

## Self-Review

**Spec coverage:** Phase 0 = A4 radii; Phase 1 = A5 sidebarTextScale; Phase 2 = A2 sidebar/transport; Phases 3-5 = the audio-panel **tab components** the user explicitly asked to make re-styleable (sticky bar, chain, synth, sampler); Phase 6 = A2 render offsets. A3 (viz alpha buckets) and the cushion A6 were addressed in the prior pass; not repeated.

**Out of scope (documented):** proportional ratios, DSP/audio constants, ±1 nudges, math constants — listed per phase as KEEP. Generic non-radius dimension scanning is infeasible (integers aren't syntactically distinguishable), so enforcement = radius ratchet (Phase 0) + drift/freshness token pins + per-tab layout-fits invariant tests (Phases 3-6).

**Type consistency:** density accessors use `Profile().DensityValues().XxxField` (PascalCase) generated from kebab-case YAML; new Go radius aliases `RadiusXXS`/`RadiusXS` parallel existing `RadiusSM/MD/LG/XL`. `SpaceXXS` parallels `SpaceXS/SM/MD`.

**Risk:** visual shift where a literal was retuned to its nearest token (e.g. 3px→4px swatch radius). Mitigated by choosing Comfortable-tier values equal to today's constants where a single value is being split into a density triple (e.g. sidebar-label-scale Comfortable=1.2, audio-label-margin Comfortable=28).
