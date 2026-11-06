# Notification System Redesign — Design

**Date:** 2026-06-10
**Status:** Approved (implementation pending)
**Branch:** add-core-node-types

## Problem

The current notification system is primitive: `notifyInfo` / `notifyError` push into a
short `dv.notifs` slice, and `layer_notifications.go` paints up to two toasts that float
in at the **top-right of the drum view** and fade after a few seconds (`ttl`). There is no
history, no persistence, and the toasts visually collide with the area above the beat/time
counter.

## Goals

1. Give notifications a **dedicated, bounded space** in the header band (above the
   timeline), not a floating overlay.
2. **Move the beat/time counter left**, next to the track/lock button, freeing the space to
   its right for the notification area.
3. **Unify the track/lock (follow) button** layout across desktop and mobile (left of the
   band on both).
4. Long messages must remain readable in the small space via a **slow leftward marquee**
   (single line, no wrap, bounded).
5. **Keep a persistent history** (across app restarts) and let the user open a **history
   popup** by clicking the notification area.

## Non-goals

- No new kebab/overflow button (the notification area itself is the click target).
- No new severity levels — **info + error only**, as today.
- The band does not pre-fill from persisted history on launch (it starts idle).

## Final band layout (both platforms)

```
[🔒 track/lock] [Beat N · m:ss] [ notification area ──────────────► ]   (chip strip / len± on the right as today)
```

- Track/lock button: square chip at the **left** of the band on both platforms (touch-min
  size on mobile). Replaces today's desktop left-vertical-strip and mobile right-edge-chip
  with one unified left chip.
- Counter pill: **left-anchored** immediately right of the track chip (was right-aligned).
- Notification area: fills the remaining width from the counter's right edge to the band's
  right limit (respecting the existing chip-strip right-clamp).

## Architecture (Approach 1: store + reuse TimelineZone band + portal popup)

### Components

| Component | Location | Responsibility |
|---|---|---|
| `notificationStore` | `internal/ui/notification_store.go` (new) | Single source of truth: bounded history ring (cap 100) + current live entry + marquee anim state. |
| `notifyInfo` / `notifyError` | `internal/ui/drumview_notifications.go` (existing funnel, 23 call sites) | Push into the store; signature unchanged so call sites don't change. |
| `TimelineZone` | `internal/ui/timeline_zone.go` (extended) | Draw the notification area + marquee in the band; register one hit area that opens the history popup. Already owns `drawBeatCounter`. |
| History popup | portal overlay id `notif-history` | Static menu-like list of history entries (newest first), errors red / info white, scrollable on overflow, closes on outside click. |
| `NotificationHistoryStore` | `internal/userprefs/store.go` + `store_js.go` + `store_notjs.go` | Persist/load the bounded history; mirrors the `AudioPanelStore` js/notjs split. |

### Data model

```go
type notification struct {
    text   string
    isErr  bool
    unixMs int64 // history timestamp
}
```

- The per-frame `ttl` field is **removed**. The band shows the **latest** entry persistently
  (no auto-fade) until a newer one replaces it.
- Store keeps a ring of the last **100** entries.
- On launch the band is **idle/empty**; persisted history is reachable only via the popup.

### Persistence (`userprefs`, gated)

- New `NotificationRecord{ Text string; IsErr bool; UnixMs int64 }`.
- New interface methods `SaveNotifications([]NotificationRecord) error` and
  `LoadNotifications() []NotificationRecord`.
- `store_js.go`: localStorage-backed. `store_notjs.go`: file-backed, atomic-rename via the
  `userprefs.persist` pool, coalescing latest-write-wins (same pattern as favorites).
- Bounded to 100 entries (drop-oldest).
- Gated by a new `UseNotificationsHistory` flag (parity-gate convention; flip false in tests
  to verify legacy handling). Round-trip test mirrors `audio_panel_state_test.go`.

### Marquee behavior (`timeline_zone.go`)

- Container = a surface chip like the counter pill (`colSurface1` fill, `colBorderSubtle`
  border), with a thin left accent stripe colored by severity. Clipped to `notifRect`.
- **Short message** (fits within `notifRect`): static, left-aligned.
- **Long message** (exceeds width): slow constant **leftward scroll**; when the tail is
  fully visible it pauses, then loops from the start. Offset is derived deterministically
  from the per-frame anim counter (`uiAnimFrame` / `AdvancePressAnim` pattern) so tests can
  assert exact positions.
- Text color: **white (`colTextPrimary`) for info, red (`colError`) for errors**.

### History popup (portal)

- Click anywhere in the notification area opens portal `notif-history`, anchored to the
  area.
- Rows newest-first: `timestamp + message`; errors red, info white. Scrollable (reuse the
  existing menu-scroll pattern) when it overflows. Portal default closes it on outside click.

### Removal

- Delete `layer_notifications.go` (floating top-right toast) and drop its tree registration.
  `ZNotifications` is retired or reused by the portal popup.

## Layout math (`drumview_layout.go`)

- Add `dv.notifRect`. Compute the band as: track chip (left) → counter pill (left-anchored,
  natural width) → `notifRect` (remaining width to the band's right limit).
- Unify the track button placement: one square chip at the band's left on both desktop and
  mobile. Remove the desktop left-vertical-strip and mobile right-edge-chip special cases.
- `beatCounterRect` becomes a left-anchored slot sized to the pill; the remainder becomes
  `notifRect`.

## Token / styling discipline

- Reuse existing tokens (`colSurface1`, `colBorderSubtle`, `colTextPrimary`, `colError`).
- If a new token is genuinely needed, follow the DESIGN.md drift-test checklist (update
  DESIGN.md YAML + Go constant + `design_md_drift_test.go` want-map) — avoid new inline
  `color.RGBA` literals (token-discipline ratchet).

## Testing (all automated; no manual steps)

Go fast-path (`-tags test`):
- `notification_store` ring-bounding + drop-oldest + ordering.
- Marquee offset math: deterministic from (text width, notifRect width, frame) — fits =
  static; overflow = scroll → pause-at-end → loop.
- Layout: `notifRect` non-empty, right of the counter, inside the band, no overlap with the
  track chip or timeline — on **both** profiles (`SetRuntimeProfileForTest` desktop + mobile).
- Track button unification: chip at band-left on both profiles.
- History popup: open on notif-area click, close on outside click (portal).
- Persistence round-trip: save → load equal, bounded to 100, gated by
  `UseNotificationsHistory` (mirror `audio_panel_state_test.go`).

Pixel:
- Notification area renders text in the correct severity color (drawRect/text-color count,
  mirroring `audio_panel_render_pixels_test.go`).

Browser:
- No new JS export expected (WASM persistence rides existing userprefs localStorage). Add a
  `wasm_bridge_smoke` catalogue line only if a new export appears.

## Open questions

None — design approved.
