# Node Pop-up + Node-Groups Menus Redesign

Date: 2026-07-16. Scope: the desktop node sidebar (node pop-up), the group
menu, the sidebar Groups section, and the mobile long-press popup — button
cleanliness (no color-filled faces), expansion/nesting legibility, and
typography smoothness. Baselines: `screenshots/nodemenus/desktop/*` (before),
`screenshots/nodemenus-after/desktop/*` (after).

## Part 1 — Critique

1. **Every button face was color-filled.** `tintBtnAccent` blended 40% of
   the node's instrument color into every stepper/selector/close keycap;
   the Audible/Move/Create/Add/Remove rows and group membership chips got
   the same 40% fill inline; the GroupMenu tinted Save and every merge chip
   and drew an `AlphaStrong` group-color ring around the whole panel. The
   sidebar read as a stack of orange blocks and the Groups section as a
   candy stack of colored bars — identity everywhere, hierarchy nowhere.
2. **Two text systems in one panel.** Panel labels rendered smooth embedded
   Inter (`DrawTextStyled`), but every `Button` caption went through the
   legacy bitmap `TextSprite` + `GeoM.Scale(1.2)` path — the visibly
   pixelated "Save / Disabled / Delete group / Drums" captions. The
   `TextScale = sidebarLabelScale()` assignment fed this on every sidebar +
   group-menu button; `sidebarScaledTextWidth` and the header truncation
   measured with the mismatched legacy metric.
3. **No nesting.** Dropdown option rows (Logic kinds, Groove kinds,
   Add/Remove-group candidates) laid out full-width at the same left edge
   as their trigger — an open dropdown read as flat sibling rows.
4. **Scrolled rows painted over the fixed header** — `inViewport` allowed
   partially-visible rows, and content drew after the header with no clip
   (visible in the `node_sidebar_groups` baseline: "Audible" over "Node").
5. Assorted: close button carried a colored outline; the Save primary
   action was indistinguishable from chrome except by fill color; the rule
   on/off toggle didn't use the app's latched-amber toggle language.

## Part 2 — Design

**Identity never fills a button face** (new DESIGN.md invariant text): in
identity menus the owning color appears only in highlight slots — header
underline, title/row swatches, active stripes, hover tints. Keycaps stay
the neutral chrome recipe; face fills remain reserved for semantic roles
(destructive red, latched amber, row-rack M/S/FX shading).

- Sidebar: all buttons neutral `DropdownStyle`; action rows share one
  `drawActionRow` helper; membership chips become flat menu rows (group
  swatch + label + drill-in chevron via `drawMenuRow`); close gets the
  standard hairline border; the node color stays on the header band,
  title swatch, and dropdown accent stripes.
- GroupMenu: neutral panel border; unified title (`drawMenuTitle` with the
  group swatch + `drawMenuHeaderBandAccent` underline); Save = neutral
  keycap with a gold accent label when armed; rule on/off = standard
  latched-amber toggle (`SetToggled`); merge chips = flat menu rows with
  candidate-group swatches; Delete keeps red text on neutral.

**Typography**: `Button.SetTextRole(role)` opt-in routes a button caption
through the styled text-role API (Inter at true pixel size); the group
menu + sidebar set `RoleBody` everywhere; header truncation and Save
sizing measure with `StyledTextWidth`. DESIGN.md § Typography now mandates
`SetTextRole` for new text-bearing buttons.

**Nesting**: option rows indent by `sidebarOptionIndent` (= `spacing.lg`);
DESIGN.md Menu-anatomy rows rule now names the one-level indent.

**Clipping**: sidebar scrolled content draws through a viewport `SubImage`;
the header/separator/close draw after it, so no row can paint over the
title band.

## Verification

New pins in `node_menus_redesign_test.go` (styled roles on both surfaces,
neutral Save face, option indent); `node_menu_instrument_color_test.go`
inverted to pin neutral faces. Full `internal/ui` fast suite green in
shards; design guards green; before/after captures reviewed at 4×.

## Non-goals

Group rename UI (no rename control exists — separate feature), badge/ring
grid overlays (data-canvas identity markers, intentionally colored),
long-press popup (already smooth + neutral/destructive), GroupMenu
scroll/collapse.
