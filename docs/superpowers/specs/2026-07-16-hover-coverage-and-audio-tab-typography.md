# Hover Coverage + Audio-Tab Typography

Date: 2026-07-16. Two workstreams: (1) hover highlights on every clickable,
via shared mechanisms only; (2) migrating all audio-tab text off the legacy
bitmap font. Baselines: `screenshots/audiotabs/desktop/*` (before) vs
`screenshots/audiotabs-after/desktop/*`.

## Part 1 — Hover coverage

**Root causes found.** The app had five independent hover mechanisms and
three systematic blind spots:
- **Portal overlays were hover-blind by construction**: portal input
  handlers run only on press/drag/release edges, and the tree's hover-glow
  overlay deliberately skips portal z — so NOTHING inside a popup (FX
  panel, instrument/template/context/subdiv menus, naming dialog) reacted
  to plain mouse-over.
- **Synth pipeline chips** were clickable tree citizens whose hit adapter
  didn't implement `glowTarget`, and whose draw had no hover branch.
- **Sidebar section headers and action rows** (and the collapsed expand
  tab) were hand-drawn with no hover; **grid nodes** had only a 1.2× size
  bump.

**Design: three sanctioned mechanisms, one spring** (now normative in
DESIGN.md "Hover lift"):
1. Tree controls → the existing `HitIndex` overlay; new clickables opt in
   by implementing `glowTarget` (synth chips now do; the selected chip is
   suppressed like latched buttons).
2. Portal overlays → new `portalHoverer` seam: `OverlayPortal.Update`
   forwards the plain cursor once per frame (desktop only); overlays
   refresh their buttons via the new `Button.UpdateHover`. Wired for the
   FX panel, overflow/template menu, context menu, naming dialog,
   instrument menu, and subdiv menu — their existing style hover deltas
   and `menuItemHover` row tints now actually fire.
3. Hand-drawn surfaces → new shared `hoverFade` animator (`hover_fade.go`,
   same `button-hover-fade` spring) + `drawButtonHoverGlow`. Wired for
   sidebar section headers, action rows, and the expand tab. Grid nodes
   reuse the firing bloom (`drawNodeBloom`) at `nodeHoverGlowIntensity`
   (0.35) with the same eased fade, so hover and trigger share one
   language.

New checklist item in DESIGN.md: every new clickable must show the lift.
Pins: `hover_coverage_test.go` (portal seam, sidebar header fade in/out,
node glow ease + fade-out retention, chip glowTarget).

## Part 2 — Audio-tab typography

**Root cause**: ~70 call sites across the tabs drew the bitmap debug font
via `DrawTextColorAtScale` at 0.6–1.6× — pixelated at every factor ≠ 1.
Worst: Levels (18 sites incl. the headline-upscaled big dB numbers), Chain
(11), Synth (~20).

**Design**: a new `micro` role (10 px Inter Regular, `RoleMicro`) joins the
typography scale for dense data-canvas labels; scaled captions map to
`micro`/`caption`, body readouts to `body`, the Levels headline to
`panel-title`. DESIGN.md now bans scale-factor text in the tabs ("Audio-tab
canvases use roles, not scale factors").

**Executed** (three parallel migrations): render_meters (all 18 sites,
`levelsAggRow.Scale` → `Role`); render_chain + chain_panel_zone +
chain_stage_card (all sites; the dead `chain*Scale` permille accessors and
`chain_density.go` deleted); render_waveform (9) + render_spectrum (5);
synth_panel_zone + concept_viz/focus_graph/full_note/preview_layer;
sampler_panel_zone; eq_panel_zone (band cells now caption→micro fallback
instead of dynamic bitmap shrink); audio_sticky_bar; audio_panel_legend;
knob_step. A styled-path test seam (`drawTextStyledHook`) mirrors the
legacy hook so text-observing tests keep working. Pinned metric tests
rewritten to pin roles instead of scale factors. Alloc budgets unchanged
(styled sprites are cached).

## Verification

Full `internal/ui` suite green in four shards except
`TestNodeLogicParamAdjustmentsApplied` / `TestNodeLogicParamCountsSkipDisabled`
— the volume-exactly-halved pair proven pre-existing at HEAD 98cfa5967 in a
clean worktree (2026-07-15), briefly fixed by the user's WIP, re-broken by
their continued node-groups edits during this session; no overlap with this
diff. Design guards green; before/after captures reviewed.
