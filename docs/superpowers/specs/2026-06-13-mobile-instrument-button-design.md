# Mobile instrument button + ellipsis menu parity

**Date:** 2026-06-13
**Branch:** add-core-node-types
**Status:** Implemented (uncommitted, in working tree)

## Problem

On mobile, tapping a drum row's instrument-name label opens the row *context
menu* (bottom sheet), and the user must then tap an "Instrument" entry to reach
the instrument picker. On desktop, tapping the label opens the instrument picker
directly. This is an unnecessary extra tap on mobile and an inconsistency across
platforms.

## Goal

Make the instrument-name label open the instrument picker on **all** platforms
(matching today's desktop behavior). The ellipsis (⋯) overflow button then owns
only the remaining row actions.

## Decisions

- **Label → instrument picker, everywhere.** The label's `OnClick` always opens
  the instrument picker, on mobile and desktop alike.
- **Ellipsis menu = Rename, Origin, Delete, on both platforms.** The mobile-only
  "Instrument" item is removed (the label now opens the picker). The desktop-only
  "Color" item is also removed — **color editing is dropped entirely** from the
  interactive UI. (Confirmed with the user after flagging that the menu was the
  only interactive color-access path; the hidden row swatch was never shown.)
- `openColorPickerForRow` / `OpenColorMenu` infrastructure is **kept** — the
  scene catalog and JS scene exports still call `OpenColorMenu`, so the picker
  itself is not removed, only its interactive menu entry.

## Changes

### 1. `internal/ui/row_rack_zone.go` — label `OnClick` (~line 662)

Remove the `Profile().IsMobile()` branch so the handler always calls
`z.callbacks.OnInstMenuOpen(idx)`:

```go
lbl.OnClick = func() {
    if z.callbacks.RenameRow != nil && z.callbacks.RenameRow() == idx {
        return // rename active for this row
    }
    if z.callbacks.OnInstMenuOpen != nil {
        z.callbacks.OnInstMenuOpen(idx)
    }
}
```

The instrument picker already renders as a bottom sheet on mobile via
`Profile().UseBottomSheet`, so no layout work is required.

### 2. `internal/ui/drumview_context_menu.go` — `contextMenuItems` (~line 242)

The function becomes platform-independent. Drop the mobile "Instrument" block
and the desktop "Color" block. Resulting menu (both platforms):

```
Rename
─────────
Origin
─────────
Delete  (red; disabled when only one row remains)
```

Update the function's doc comment to reflect the new, platform-uniform menu.

## Testing (TDD — functional, driving the real input loop)

Per project convention ([[feedback_functional_tests_over_isolation]]), tests
drive the real `Game.Update()` input loop and assert user-visible outcomes, not
isolated component state.

1. **Mobile: label tap opens the instrument picker** (and NOT the context menu).
   Force mobile profile, tap the row label rect, assert the instrument menu
   overlay is open and the context menu is closed.
2. **Desktop: label tap opens the instrument picker** (regression guard,
   unchanged behavior).
3. **Ellipsis menu contents = exactly {Rename, Origin, Delete} on mobile** —
   assert "Instrument" and "Color" are absent.
4. **Ellipsis menu contents = exactly {Rename, Origin, Delete} on desktop** —
   assert "Color" is absent (the dropped item) and "Instrument" remains absent.
5. **Delete still disabled when a single row remains** (guard the existing
   invariant survives the menu refactor).

Test files: extend `row_rack_zone_test.go` (label-click routing) and
`drumview_context_menu*_test.go` (menu contents). Use existing mobile/desktop
profile override helpers (`SetRuntimeProfileForTest` / layout-profile mobile
toggles already used by sibling tests).

## Out of scope

- Re-introducing a visible color swatch (explicitly declined).
- Any change to the instrument picker component itself.
- Mobile EQ-mode row-control visibility (unchanged).
