# Node Pop-up List + List-Button Unification

**Date:** 2026-06-28
**Status:** Approved (design)
**Scope:** `src/go/internal/ui/game_node_sidebar.go`

## Problem

The previous batch unified all overlay list menus (instrument, row context, subdivision, template/overflow) on a shared `drawMenuRow` primitive (`menu_chrome.go`) that owns styling + keycap animation, and aligned them on `DropdownStyle`. The **node pop-up menu** (`NodeSidebar`, `game_node_sidebar.go`) was deliberately left out of that pass.

The node pop-up already renders its buttons via the keycap `Button.Draw()` path, persists them, and threads the node's color (`nodeAccent()`) through every border/stripe. But two things diverge from the now-unified menus:

1. Its **Logic/Groove dropdown lists** do not route through `drawMenuRow`. `drawDropdownItem` draws the button, then overlays a selected-state highlight via `drawMenuItemBackgroundAccent` after the fact.
2. Its buttons use `PopupButtonStyle` (`ComponentButtonSecondary`), while the menu lists use `DropdownStyle` (`ComponentButtonDropdown`) — so even the list items look different from the other menus.

Goal: make the node pop-up's **lists** render identically to the other menus (via `drawMenuRow`), accented by the node's own color, with the list-backing buttons on `DropdownStyle`. Keep every other node control on its role-specific spec.

## Decisions (from brainstorming)

- **Restyle scope:** lists + list-backing buttons. Route node Logic/Groove dropdown lists through `drawMenuRow` (node-color active stripe), and switch those list items to `DropdownStyle`. Keep steppers (±), the Logic/Groove selector buttons, Audible/Move, and Close on their current role-specific specs (they already use the keycap path + `nodeAccent` borders).
- **No new abstraction (targeted):** rendering is already unified at two seams — `Button.Draw()` (all keycap buttons) and `drawMenuRow()` (all list rows). Route the node lists through the existing `drawMenuRow` seam; introduce no new component layer.

## Node color — single source of truth

`DrumRow.Color` (`drumview.go:142`) is the canonical storage. `NodeSidebar.nodeAccent()` (`game_node_sidebar.go:1283`) resolves a node's color via `game.nodeRows[node.ID]` → `drum.Rows[row].Color`, falling back to `colAccent` when the node has no row. This is the existing single seam (no duplicated logic) and is what every node-menu accent already uses. The unified lists must keep using `nodeAccent()` — passed as `drawMenuRow`'s `Accent` — so node lists are accented by the node's own color, not the global `colAccent`.

## Design

Two changes, both in `game_node_sidebar.go`:

### 1. `drawDropdownItem(dst, id, selected)` → route through `drawMenuRow`

Replace the current `drawBtn(dst, id)` + post-hoc selected overlay with a single `drawMenuRow` call:

```go
func (sb *NodeSidebar) drawDropdownItem(dst *ebiten.Image, id string, selected bool) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	r, ok := sb.rects[id]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	state := menuItemRest
	if selected {
		state = menuItemActive
	} else if b.hovered || b.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, b, MenuRowSpec{
		Accent: sb.nodeAccent(), // node-color stripe/tint (SSOT = DrumRow.Color)
		State:  state,
		Label:  b.Text,
	})
}
```

The node color now shows through `drawMenuRow`'s accent stripe (active) and hover tint — identical to the other menus. The previous per-item `tintBtnBorder` is dropped for list items (the stripe is the indicator now, matching the menus). The `inViewport` clip guard is preserved for the scrolling sidebar.

### 2. `wireButtons` → list-backing buttons use `DropdownStyle`

The `logic:*` and `groove:*` dropdown-item buttons are currently created with `PopupButtonStyle`. Create them with `DropdownStyle` so `drawMenuRow`'s `btn.Draw()` paints the same cap face as the other menus. All other node buttons (steppers, `logic`/`grv` selectors, `aud`, `move`, `close`) keep their existing specs.

The discriminator is the button id prefix: ids beginning with `logic:` or `groove:` are dropdown list items. `wireButtons` chooses `DropdownStyle` for those and `PopupButtonStyle` (or the existing role spec) for the rest.

## Out of scope

- Steppers (`ComponentButtonStepper`, shared app-wide with BPM/length steppers) — unchanged.
- Logic/Groove **selector** buttons (the header that opens the dropdown) — unchanged (role: selector).
- Audible / Move action buttons (`ComponentButtonSecondary` + `nodeAccent` border) — unchanged.
- Close button — unchanged.
- Value pills (`drawValuePill` / `drawRecessedPill`) — bespoke recessed readouts, not buttons; unchanged.
- Section headers (`drawSectionHeader`, chevron + `drawMenuItemBackgroundAccent`) — headers, not list rows; consistent with the menus' header treatment; unchanged.
- No new shared component layer.

## Testing

- Existing node-sidebar tests stay green.
- New test (`game_node_sidebar` test file): open the Logic (and/or Groove) dropdown, then assert (a) dropdown-item buttons are `DropdownStyle` (not `PopupButtonStyle`), and (b) the selected item draws the `nodeAccent` active stripe via `drawMenuRow` — using the existing drawRect-interception / state pattern the repo already uses for menu-chrome tests, or asserting the row state path. Confirm a non-selected hovered item reads `menuItemHover`.
- Animation persistence: dropdown-item `pressDepth` advances across draws (buttons are already persisted in `sb.btns`).
- `go vet -tags test -modfile=go.test.mod ./internal/ui` exits 0.
- Design-token guards (`TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot`) pass — the change introduces no inline color literals (it uses `nodeAccent()` + existing tokens).

## Risks / notes

- `nodeAccent()` must remain the accent source so node lists stay node-colored (not global `colAccent`). The fallback to `colAccent` for row-less nodes is acceptable (pre-existing behavior).
- External churn on this branch is touching the `audio` package / instrument-label store; verify with scoped test runs + `go vet -tags test`, not the full (churn-disrupted) suite.
- Git commit intentionally skipped (user forbids staging/commits); work left uncommitted for review.
