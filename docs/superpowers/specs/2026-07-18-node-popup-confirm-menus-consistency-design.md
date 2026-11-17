# Node Popup + Confirmation Menus — DESIGN.md consistency rework

**Date:** 2026-07-18
**Status:** Draft (awaiting spec review)
**Branch:** add-core-node-types

## Summary

Bring the node click-and-hold quick-action popup, the modal confirmation
dialogs, and the armed-mode banners into full DESIGN.md compliance and visual
consistency with the rest of the app's menus and buttons. The theme throughout
is **reuse existing infrastructure** — the shared keycap/`Button` chrome, the
`dvOverlayPortal` adapter, the icon system, the text-role API, design tokens,
and the `gridModeManager` ("mode") API — rather than adding new machinery.

Three families of chrome are currently inconsistent:

1. **Node quick-action popup** (`game_longpress_popup.go`) — a text-only,
   hand-drawn keycap strip with no leading icons; the only menu shaped unlike
   the icon-forward row/context menus.
2. **Modal confirmation dialogs** — two near-identical hand-drawn twins
   (`node_clipboard.go` "Replace node?", `grid_move_break_confirm.go` "Break the
   circuit?") with hardcoded geometry (320×140 / 120×32), the wrong title role
   (`RolePanelTitle` 21px vs. the shared `RoleSectionHeader` 16px), plain
   neutral confirm buttons, and — for paste — raw English strings. They diverge
   from the gold-standard shared-`Button` dialogs (`settings_overlay.go`,
   save-as).
3. **Armed-mode banners** — three separate implementations (Move = raw English
   UPPERCASE, Copy = i18n, Connect = raw English), all using the pixelated
   `DrawTextAt` bitmap path instead of the crisp `DrawTextStyled` role API.

## Goals

- The quick-action popup reads as a first-class menu: leading icons + labels on
  shared keycap chrome, instrument-color hover accent, destructive-red Delete.
- **One** shared `ConfirmDialog` component; both confirm twins become thin
  consumers. Titles use the shared menu-title role; the destructive action gets
  destructive-red; geometry is token-driven; all strings are i18n.
- **One** shared armed-mode banner driven by `gridModeManager`, covering
  Move/Copy/Connect uniformly with styled text + panel chrome.
- Net reduction in bespoke drawing/geometry/hit-test code.

## Non-goals

- **No portal-based input rewrite of the popup.** The slide-to-hover /
  release-to-activate gesture (`updateLongPressPopup`) stays hand-managed —
  explicitly out-of-scope per `2026-07-17-node-quick-action-modes-design.md`.
  This change upgrades the popup's **draw** path only.
- **No new confirmation before node Delete** — deletion stays immediate + undo
  (user decision, 2026-07-18).
- No change to mode semantics, `moveNode`/`pasteNodeFrom` behavior, keyboard
  clipboard flow, or the two-click row-delete confirm (already compliant).

## Existing infrastructure to reuse (the operative constraint)

| Need | Reuse (do NOT reinvent) |
|---|---|
| Overlay-panel chrome | `drawPanel` / `drawScrim` (`drawing.go`) |
| Keycap button chrome | `drawKeycapPanelButton` (`button_cushion.go:201`) — canonical raised keycap; already used by the popup |
| Real buttons in a dialog | `*Button` via `NewButtonKey(key, ComponentID, onClick)` + `SetTextRole` + `SetToggled` (see `settings_overlay.go`) |
| Modal portal + scrim + hit-index filter | `PortalEntry{Modal:true, Scrim:true}` + `portal().Open/Close` (`portal.go`) |
| Button→HitArea routing | `buttonHitAdapter` (`eq_panel_zone.go:1681`) — canonical `*Button`→`HitHandler`; per-button areas + catch-all-last (`settings_overlay.go:101`) |
| Keyboard seam | `portalEscapeHandler` / `portalEnterHandler` interfaces (`game_input_shortcuts.go:364/372`) — implement both on the dialog |
| Menu title band | `drawMenuTitle` (`menu_chrome.go:125`, `RoleSectionHeader` + optional swatch) |
| Close affordance | `closeButtonRect` / `closeIconColor` (`drawing.go`) |
| Icons | `drawIconByID` + `IconID` enum (`icons.go`); `IconTrash` exists |
| Text | `DrawTextStyled` + `StyledText{Width,Height}` with `RoleBody`/`RoleSectionHeader` |
| Tokens | `Token*()` accessors, `WithAlpha`, `Space*`/`Radius*`/`BtnHeight*`, `Profile().DensityValues()` |
| Mode state | `gridModeManager` (`grid_mode.go`): `Enter/Cancel/Commit/Active/Is/Subject`, `GridModeObserver.OnGridModeChanged` |
| i18n | `i18n.Key` catalog (en + es only) |

## Design

### A. Shared `ConfirmDialog` — new `confirm_dialog.go`

One reusable modal confirmation built as a real **`PortalOverlay`** in the
**gold-standard `settings_overlay.go` shape** (per-button `buttonHitAdapter` hit
areas + a catch-all-last), holding **two real `*Button`s** so it inherits keycap
chrome + hover/press animation + the canonical press lifecycle for free. This is
chosen over reusing `dvOverlayPortal` (which forces manual `pt.In()` hit-testing
and can't route Enter) precisely because it is cleaner and button-native.

```
type ConfirmDialog struct {
    id          string          // portal id
    titleKey    i18n.Key        // section-header title (a question)
    swatch      color.Color     // optional instrument accent; nil = none
    confirmBtn  *Button         // right; destructive or primary spec
    cancelBtn   *Button         // left; ComponentButtonSecondary
    closeBtn    *Button         // top-right close-X == cancel
    onConfirm   func()          // pluggable — caller owns mode/graph effects
    onCancel    func()          // pluggable — caller owns mode/graph effects
    rect        image.Rectangle
    closed      bool
}
```

- **Construction:** `newConfirmDialog(cfg confirmConfig) *ConfirmDialog` with
  `confirmConfig{id, titleKey, confirmKey, destructive bool, swatch, onConfirm,
  onCancel}`. `confirmBtn` spec = `ComponentButtonDestructive` when `destructive`
  else `ComponentButtonPrimary`; `cancelBtn` = `ComponentButtonSecondary`;
  `closeBtn` = secondary + `IconClose` + `closeIconColor()`. Labels via
  `NewButtonKey` (i18n) + `SetTextRole(RoleBody)`. `OnClick`s call
  `confirm()`/`cancel()` (which fire the pluggable callback then mark `closed`).
- **`Layout`:** centered over the grid content pane (reuse the twins' existing
  centering math), width/height derived from `StyledTextWidth(title)`, the two
  button widths, `SpaceMD` padding, and `BtnHeightMD` — **no** 320×140 / 120×32
  literals; clamped to the grid pane. `SetRect`s Cancel (left) / Confirm (right)
  on the bottom row and the close-X via `closeButtonRect`.
- **`HitAreas`:** one `buttonHitAdapter` area per button (Confirm, Cancel, close)
  + a full-panel catch-all handler registered **last** (à la
  `settingsPanelHandler`) so buttons win the equal-z tiebreak and no panel-bg tap
  falls through. Modal open filters out-of-panel taps at the `HitIndex`.
- **`Draw`:** `drawScrim` → `drawPanel` (surface-overlay, `rounded.md`, border,
  shadow) → `drawMenuTitle(title, swatch)` at `RoleSectionHeader` →
  `cancelBtn.Draw` / `confirmBtn.Draw` / `closeBtn.Draw`.
- **Keyboard (bonus fix):** implement `portalEnterHandler` → Confirm **and**
  `portalEscapeHandler` → Cancel (return `false` so `handleEscape`'s `CloseTop`
  removes the still-topmost entry, preserving the mid-stack contract at
  `grid_move_break_confirm.go:129-135`). This closes the **current gap** where
  Enter does nothing for these dialogs. The two ad-hoc Esc rungs at
  `game_input_shortcuts.go:411-420` are the **headless/no-portal** fallback
  (portal `Open` is a no-op when `g.drum.portal()==nil`); consolidate them into a
  single `confirmDialogActive()`-style check rather than one per dialog — keep a
  fallback for headless, but stop growing it per confirm.
- **Close:** `ShouldClose()` returns `closed` for the portal sweep; caller's
  `begin*` opens via `PortalEntry{Modal:true, Scrim:true, Overlay:dlg,
  OnClose: onCancel}`. Headless (no portal) still works via direct
  Confirm/Cancel (the `*Active()` + shortcut path is retained as today).
- **Click-outside dismissal (the shared pop-up contract):** the dialog owns
  **only** its panel + per-button hit areas (panel-rect catch-all last, exactly
  like `settingsPanelHandler`) — it does **NOT** register a full-screen backdrop.
  A press outside the panel therefore misses every area, and the existing tree
  dispatcher (`drumview_tree.go:636-651` `len(hits)==0 → portal.CloseTop()`)
  pops the entry and fires `OnClose` → the dialog's own `onCancel`. This is the
  same implicit route every menu/dropdown already uses, so click-outside works
  for free and consistently. **Do not add a scrim/backdrop hit area** — that
  would swallow the outside tap and break dismissal. Each pop-up thus owns its
  own **setup** (`begin*`/`Open`) and **cancellation** (`onCancel` via `OnClose`
  + `ShouldClose`); nothing external hand-manages its teardown.

**Two lifecycles, one component (via pluggable callbacks):**

- `grid_move_break_confirm.go` (break-circuit): the dialog runs **while Move
  mode stays armed behind it**. Keeps `breakConfirmModeObserver` (tears the
  dialog down if Move ends externally). `onConfirm` = `ConfirmBreakCircuit`
  (→ `moveNode` + `g.modes.Commit()`); `onCancel` = `CancelBreakCircuit`
  (dialog only — **Move stays armed** so the user retries). Config: `titleKey:
  KeyBreakCircuitTitle, confirmKey: KeyCapBreak, destructive: true`.
- `node_clipboard.go` (paste-overwrite): **no mode behind it** — Copy mode
  already `Commit()`ed before the confirm spawned (`game_input_touch.go:40`), and
  the Ctrl-V path is mode-independent. `onConfirm` = `ConfirmPasteOverwrite`,
  `onCancel` = `CancelPasteOverwrite`. Config: `titleKey: KeyConfirmReplaceNode,
  confirmKey: KeyCapOverwrite, destructive: true`.

Both consumers **delete** their `*ConfirmRect`/`*BtnRect` helpers, `open*Portal`,
`draw*Confirm`, and manual `inputFn` `pt.In()` blocks; they keep only their
`*PendingState`, `begin*`, `Confirm*`, `Cancel*`.

### B. Node quick-action popup — `game_longpress_popup.go` (draw upgrade only)

Input path (`updateLongPressPopup`, slide-hover/release-activate) is **unchanged**.
`drawLongPressPopup` upgrades:

- Keep `drawScrim` + `drawPanel` (already present). Re-measure `showLongPressPopup`
  panel/keycap sizing to fit an **icon + label** per keycap (icon on top or
  leading, label beneath/right), from density tokens.
- Each action drawn via `drawKeycapPanelButton` (the canonical shared keycap —
  identical chrome to every `*Button`) now carrying `drawIconByID(icon, ...)` +
  `DrawTextStyled(label, RoleBody)`:
  - Move → `IconMove`, Copy → `IconCopy`, Connect → `IconConnect`,
    Delete → `IconTrash`.
- Rest tint = `genColorPopupTextSecondary`; hover tint = instrument accent
  (`longPressPopupAccent()`, already present) applied to **icon + label**.
  Delete keycap keeps `colDeleteFill`/`colDeleteBorder` + dark-on-neon label.

> Note: we use the shared keycap **draw helper** rather than full `*Button`
> widgets because the slide-select gesture drives hover/activation by hand; the
> visual result is pixel-identical to a `*Button`, and this avoids the
> out-of-scope portal/input rewrite.

**Dismissal (pop-up contract):** the popup already owns its setup
(`showLongPressPopup`) and cancellation (`dismissLongPressPopup(reason)`), and
already dismisses on slide-release-outside-any-button (`updateLongPressPopup`
`default → dismiss`). We keep that gesture but also make a **press that lands
outside the panel rect** dismiss it (a plain tap-away closes it, matching the
"click-outside closes" contract), calling the same `dismissLongPressPopup`. No
portalization — it stays a grid layer with exclusive input — but its
open/cancel functions and outside-dismissal now match the portal pop-ups'
behavior.

### C. New icons — `icons.go` + draw funcs + tests

Add `IconMove` (four-way arrows), `IconCopy` (two offset rounded squares),
`IconConnect` (link / two dots joined). Antialiased vector via the existing
icon renderer, legible at keycap size (≥ `btn-sm`). Wire into the `IconID`
enum, the `drawIconByID` switch, individual `drawXxxIcon` funcs, and whatever
icon-enumeration test exists.

### D. Shared armed-mode banner — consolidate into `grid_pane_draw.go`

Replace the three bespoke banners (Move block, Copy block, `drawConnectMode`'s
banner) with **one** `drawGridModeBanner(dst)` reading `g.modes.Active()` /
`Subject()`:

- Chrome via `drawPanel` (consistent shadow/border), text via
  `DrawTextStyled(RoleBody)` (crisp), one i18n key per mode + a shared styled
  ESC-hint suffix (respecting `Profile().ShowEscHint`).
- Keys: `KeyBannerMoving` (new), `KeyBannerPasting` (exists), `KeyBannerConnecting`
  (new). Coordinate interpolation kept via `fmt.Sprintf` over the localized
  template. Connect mode's source-node outline stays in `drawConnectMode`; only
  its banner moves to the shared function.

### E. DESIGN.md + token cleanup

- Remove the **orphaned** `viz-confirm-green` / `viz-cancel-red` color tokens
  (zero runtime usages) from DESIGN.md YAML, `design_*.gen.go` (regen), and the
  `design_md_drift_test.go` want-map.
- Document `ConfirmDialog` under Components, the quick-action bar under the node
  pop-up section, and add `IconMove`/`IconCopy`/`IconConnect` to the Icon Map
  appendix.
- Update `token_discipline_test.go` budgets for any files whose inline-literal
  count changes (the confirm twins lose literals → ratchet down).

### F. i18n keys to add (en + es)

- `KeyConfirmReplaceNode` ("Replace node?" / "¿Reemplazar nodo?")
- `KeyCapOverwrite` ("Overwrite" / "Sobrescribir")
- `KeyBannerMoving`, `KeyBannerConnecting` (format templates with `(%d,%d)`)

## Mode-API integration (how it all fits `gridModeManager`)

- The **popup** is the entry point: Move/Copy/Connect call `g.modes.Enter(kind,
  node)`; Delete is immediate (not a mode). Unchanged.
- The **banner** is the visual read-out of `g.modes.Active()` — a pure function
  of mode state, no new state.
- **Confirms fire on the placement tap**, inside `handleArmedModeTap`
  (`game_input_touch.go`), *not* at `Enter`: Move → unhealable target →
  `beginBreakConfirm` (Move stays armed); Copy → occupied cell → `Commit()` then
  `beginPasteConfirm` (mode already gone). The rework does not touch this
  dispatcher.
- The **ConfirmDialog** consumers keep their `GridModeObserver` wiring: break-
  circuit's observer cancels the dialog when Move mode ends externally, and its
  Confirm calls `g.modes.Commit()`. The shared component **never touches the mode
  manager** — it only renders + routes Confirm/Cancel via pluggable callbacks —
  so the "only Enter/Cancel/Commit mutate mode state" discipline
  (`grid_mode_discipline_test.go`) is preserved.

## File-by-file change plan

| File | Change |
|---|---|
| `confirm_dialog.go` | **new** — shared `ConfirmDialog` as a `PortalOverlay` (per-button `buttonHitAdapter` + real `*Button`s; implements `portalEnterHandler`/`portalEscapeHandler`) |
| `node_clipboard.go` | delete hand-drawn rects/draw/portal/`pt.In`; use `ConfirmDialog` |
| `grid_move_break_confirm.go` | delete hand-drawn rects/draw/portal/`pt.In`; use `ConfirmDialog`; keep observer + Commit |
| `game_input_shortcuts.go` | dialog owns Enter/Esc centrally; consolidate the two headless Esc fallback rungs (411-420) into one `confirmDialogActive()` check |
| `game_longpress_popup.go` | draw upgrade: icon+label keycaps; re-measure sizing |
| `icons.go` (+ icon draw funcs) | add `IconMove`/`IconCopy`/`IconConnect` |
| `grid_pane_draw.go` | consolidate Move/Copy banners → `drawGridModeBanner` |
| `game_connect_mode.go` | route connect banner through shared banner |
| `i18n/keys.go`, `catalog_en.go`, `catalog_es.go` | add keys above |
| `DESIGN.md` + `design_*.gen.go` | remove orphaned tokens; doc updates; regen |
| `design_md_drift_test.go`, `token_discipline_test.go` | update guards |
| `scene_catalog.go` | add `paste_confirm_dialog`, `break_circuit_confirm`; remove temp scenes |

## Testing strategy

Functional tests that drive real `Game.Update` (per project convention), fast
`-tags test` path:

- **ConfirmDialog:** open → Confirm fires `onConfirm` + closes; Cancel/close-X /
  Esc fire `onCancel` + close; **a real press outside the panel cancels** (drives
  `Game.Update` so the tree `CloseTop` route runs → `OnClose` → `onCancel`) —
  fills the recon-noted test gap; button rects hit-test correctly; title uses
  `RoleSectionHeader`; destructive confirm uses the destructive spec. Exercised
  through both consumers (paste-overwrite occupied-cell paste; break-circuit,
  asserting Move stays armed after outside-cancel).
- **Popup:** hit-rects cover icon+label keycaps; slide-release still activates
  each action; Delete keycap uses destructive fill; hover tints icon+label to
  the instrument accent. Extend existing `longpress_popup*` tests.
- **Icons:** each new `IconID` returns true from `drawIconByID`; icon-enumeration
  test updated.
- **Banner:** one banner per armed mode via `g.modes`; styled (not bitmap) text.
- **Guards:** `design_md_drift`, `token_discipline`, and the design-md lint
  snapshot pass with the token removal + budget ratchet.
- **Screenshots:** before/after via the scene infra (`node_longpress_menu`,
  `paste_confirm_dialog`, `break_circuit_confirm`, `tmp_move_banner` → permanent).
- **No staging/commit.** (User convention.)

## Risks

- **New-icon legibility at keycap size.** Mitigate: reuse the antialiased icon
  renderer; verify via screenshots at desktop + mobile density; fall back to
  text-only for any icon that reads poorly (would return to the AskUserQuestion
  option B).
- **ConfirmDialog lifecycle regressions** (portal mid-stack close / Esc). Mitigate:
  the dialog's `HandleEscape()` returns `false` so `handleEscape`'s `CloseTop`
  removes the topmost entry (the twins' proven mid-stack contract), and `OnClose`
  clears pending state; keep the break-circuit `GridModeObserver` untouched.
- **Popup sizing on narrow mobile panes** with icon+label. Mitigate: keep the
  existing width-clamp logic; icons shrink with the keycap.
