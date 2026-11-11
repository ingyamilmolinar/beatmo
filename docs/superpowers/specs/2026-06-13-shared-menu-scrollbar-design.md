# Shared `MenuScroll` helper — scrollbar reuse across all list menus

Date: 2026-06-13
Branch: add-core-node-types
Status: Approved (design), pre-implementation

## Problem

The "Load template" (overflow) menu does not scroll. When the template list
overflows the popup (the popup is height-clamped to the drum pane), lower items
are unreachable.

Root cause: the overflow menu's `ScrollBehavior` is **half-wired**.

- `wheelFn` calls `overflowScroll.HandleWheel(steps)` so scroll *state* updates,
  but...
- `overflowPopupBtns` and `drawOverflowMenu` lay items out from `popupRect.Min.Y`
  with **no scroll offset** (`VS.First * rowH`) applied — content never moves.
- `handleOverflowMenuInput` has **no scrollbar-drag handling** and **no
  touch-scroll** on mobile (only deferred-tap).

Net: the scrollbar is cosmetic/dead.

## Background — what is already shared

A reusable, correct scroll stack already exists and is **not** the problem:

- `VerticalScroller` (`scroll.go`) — item-level scroll state + thumb geometry.
- `ScrollBehavior` (`scroll_behavior.go`) — wheel / thumb-drag / touch-momentum.
- `ScrollbarStyle` (`scrollbar_style.go`) — `DefaultScrollbarStyle`,
  `MobileScrollbarStyle`, `DropdownScrollbarStyle`.
- `TouchScroller` (`touch_scroll.go`) — pixel momentum + direction lock.
- `DeferredTap` (`deferred_tap.go`) — tap vs scroll disambiguation.

What is **not** shared is the per-menu glue, hand-rolled (and mis-rolled) in each
menu. The four parts:

1. **Configure** geometry: `VS.View`, `VS.Total`, `VS.Visible`, `ItemHeight`.
2. **Input state machine**: desktop thumb-drag + immediate button hit; mobile
   deferred-tap + touch-scroll-commit + thumb-drag.
3. **Offset application**: position items at `base − VS.First*rowH`.
4. **Draw** the scrollbar (already a single `sb.Draw(dst)` call everywhere).

Current state per menu:

| Menu                | Configure | Input (#2) | Offset (#3) | Draw | Status |
|---------------------|-----------|------------|-------------|------|--------|
| Context menu        | ✅        | ✅         | ✅          | ✅   | works (reference impl) |
| Instrument menu     | ✅        | ✅ bespoke | ✅          | ✅   | works (complex) |
| Overflow / template | ✅        | ❌ wheel-only | ❌       | ✅   | **broken** |
| Subdivision         | ❌        | ❌         | ❌          | ❌   | no scroll |

## Goal

1. Fix the Load-template menu so it actually scrolls (wheel + thumb-drag +
   mobile touch-scroll).
2. Extract the repeated glue into one shared, tested helper so every list menu
   uses the same correct scroll mechanics — eliminating the duplication that let
   the overflow menu drift into a half-wired state.

Non-goals: rewriting the low-level scroll stack; changing scroll feel/visuals;
re-architecting the instrument menu's content layout.

## Design — `MenuScroll`

New file `internal/ui/menu_scroll.go`. `MenuScroll` bundles the two primitives
every list menu already needs together and owns the one correct input state
machine.

```go
type MenuScroll struct {
    sb  *ScrollBehavior // existing shared scroll engine
    tap DeferredTap     // existing shared tap/scroll disambiguator
}
```

### API

```go
func NewMenuScroll(style ScrollbarStyle, itemHeight int) *MenuScroll

// Configure sets the scroll viewport + counts for the current layout.
// itemHeight stays as set at construction unless overridden.
func (m *MenuScroll) Configure(view image.Rectangle, total, visible int)

// OffsetPx is the pixel offset to subtract from item base-Y during layout/draw.
func (m *MenuScroll) OffsetPx() int            // VS.First * ItemHeight

func (m *MenuScroll) HandleWheel(steps int) bool
func (m *MenuScroll) Draw(dst *ebiten.Image)
func (m *MenuScroll) HasScroll() bool
func (m *MenuScroll) ScrollBehavior() *ScrollBehavior // escape hatch for bespoke menus

// HandleInput runs the complete desktop+mobile scroll input state machine,
// lifted verbatim (behavior-preserving) from handleContextMenuInput.
func (m *MenuScroll) HandleInput(in MenuScrollInput) (handled bool)

// TapActive reports an in-flight mobile deferred tap (caller maps to
// InputCaptured vs InputConsumed for the portal).
func (m *MenuScroll) TapActive() bool
```

### `MenuScrollInput`

Callbacks keep each menu's content bespoke while the mechanics are shared:

```go
type MenuScrollInput struct {
    Pt         image.Point
    Pressed    bool
    PopupRect  image.Rectangle   // whole popup: consume taps inside
    ItemView   image.Rectangle   // scroll viewport (excludes pinned header)
    Relayout   func()            // re-apply OffsetPx() to buttons after scroll
    FireTapAt  func(x, y int)    // mobile: fire the button under (x,y)
    DesktopHit func(pt image.Point, pressed bool) bool // desktop immediate hit
    CloseBtn   *Button           // optional; checked before items on mobile
    HeaderTop  int               // optional; Y below a pinned header (consume above)
}
```

The state machine mirrors `handleContextMenuInput` exactly:

- **Mobile**: (1) scrollbar drag in progress → `HandleDragTo`/`HandleDragEnd` +
  `Relayout`; (2) scroll committed → `HandleTouchMove`+`Relayout` / on release
  cancel tap + `HandleTouchEnd`; (3) touch active but in dead zone →
  move/commit-or-tap on release; (4) thumb click → `HandleDragStart`;
  (5) close button; (6) press in `ItemView` → begin deferred tap +
  `HandleTouchBegin`; (7) header area → consume.
- **Desktop**: thumb-drag (start/to/end + `Relayout`) then `DesktopHit`.
- Finally: consume any tap inside `PopupRect`.

## Migration

### Overflow / Load-template (fixes the bug)
- `initOverflowScroll` → build/configure a `MenuScroll`.
- `overflowPopupBtns` + `drawOverflowMenu`: subtract `OffsetPx()` from row Y;
  clip rows to the viewport; the close button stays pinned (not offset).
- `handleOverflowMenuInput` → delegate to `MenuScroll.HandleInput` with a
  `Relayout` that rebuilds button rects and a `FireTapAt` (existing
  `fireOverflowMenuTapAt`). Keep file-picker-rect registration working
  (recompute against offset rows).
- `registerFilePickerRects` must account for the scroll offset.

### Subdivision (shares primitives; immediate-tap content-input stays)
- Add `*MenuScroll` to `SubdivMenuComponent`.
- `rebuildButtons`: cap card height to `screenBounds` (so it can overflow),
  configure `MenuScroll`, apply `OffsetPx()` to item rects, clip to card.
- `HandleInput` (component): call `MenuScroll.HandleScrollbarDrag` + `HandleWheel`
  before the existing immediate button iteration. The full deferred-tap
  `HandleInput` is **not** adopted here — subdiv is a tiny ≤8-option list whose
  immediate-tap path works and is covered by tests; switching it to deferred-tap
  for no overflow benefit is needless regression risk.
- `Draw`: add `menuScroll.Draw(dst)`.
- `buildSubdivMenu` mirror keeps reading `Buttons()` (already offset rects).

`MenuScroll.HandleScrollbarDrag(pt, pressed, relayout)` is the shared primitive
for menus that keep bespoke tap handling: it owns only thumb start/continue/end.

### Context menu (behavior-preserving)
- Replace the body of `handleContextMenuInput` with a `MenuScroll.HandleInput`
  call (it is the reference implementation; net behavior identical).
- `rebuildContextMenuButtons` uses `OffsetPx()` instead of inline
  `VS.First * rowH`.
- Keep the pinned mobile header + group-container draw logic.

### Instrument menu (shares primitives; content-input stays bespoke)
- Hold a `*MenuScroll`; use `OffsetPx()` / `HandleWheel` / `Draw` / `HasScroll`.
- Its bespoke content-input (favorites star clicks, search row, breadcrumb
  pagination, page chips, auto-scroll-to-last-added) **stays** — its `VS.View`
  math is woven through that chrome and it already works. Forcing it through the
  generic `HandleInput` risks regressing a working 1930-line menu for little
  gain. Verified-correct via existing tests + an offset-correctness assertion.

## Testing (TDD, functional-first)

Per `feedback_functional_tests_over_isolation`: reproduce the bug with a
functional test driving the **real** input loop FIRST.

1. **Failing test** (`template_menu_scroll_test.go` or similar): open
   Load-template with enough templates to overflow a clamped popup; scroll via
   wheel and via thumb-drag; assert (a) a previously off-screen template's button
   rect moves into the viewport, and (b) a tap on it fires the correct
   `onImport`. Must FAIL on current code.
2. `menu_scroll_test.go`: `MenuScroll` unit tests — `OffsetPx`, `HandleWheel`,
   `Configure`, `HasScroll`, and `HandleInput` transitions (desktop drag, mobile
   deferred-tap vs scroll-commit).
3. Subdivision: new test with many options asserting overflow scroll works;
   existing subdiv tests stay green.
4. Context + instrument menus: existing tests stay green (behavior-preserving).
5. Full fast suite `cd src/go && ../../.tools/go/bin/go test -tags test
   -modfile=go.test.mod ./internal/ui`, diffed against the branch baseline (this
   branch carries known pre-existing failures — see
   `project_add_core_node_types_preexisting_failures`).

## Constraints / project rules honored

- **No staging or committing** (`feedback_no_staging_or_committing`): all work,
  including this doc, stays in the working tree. Not committed.
- **Shared widget reuse** (`feedback_shared_widget_reuse`): all menus already use
  `*Button`; `MenuScroll` reuses `ScrollBehavior`/`DeferredTap`, no bespoke
  scroll chrome.
- **Functional tests over isolation**: bug reproduced via the real input loop
  before the fix.

## Risks

- Context-menu migration must be exactly behavior-preserving (it's the reference
  impl). Mitigated by lifting the state machine verbatim + keeping its tests.
- Instrument-menu scope is deliberately limited to primitive-sharing to avoid
  regressing a complex working menu.
- File-picker-rect registration on the overflow menu must track the new offset.
