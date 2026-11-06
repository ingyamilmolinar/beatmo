# Logs Navigation Panel — Design

**Date:** 2026-07-19
**Status:** Approved (design), pending spec review → implementation plan
**Author:** Claude (with ingyamilmolinar@gmail.com)

## Goal

Replace the in-band notification message component (the marquee strip above the
timeline) and the "super basic" notification-history popup with:

1. A **Logs icon** in the grid pane, sitting immediately left of the settings
   gear.
2. A **Logs Navigation Panel** — a large, maximally-usable overlay for
   searching, filtering, and paginating through *all* kinds of logs: the full
   `hooks.Bus` event stream (every user action + lifecycle event) merged with
   user-facing notifications.

The panel must support: search-by-name (fuzzy), search-by-timestamp (exact time
**and** range), category filtering by **log prefix / event tag** (data-driven,
auto-including new kinds), and **pagination**.

## Non-goals

- No persistence of the full event stream (session-only, in-memory). User
  notifications keep their existing userprefs persistence.
- No change to the `hooks.Bus` publish path, the `eventlogger` INFO narrative
  format, the tag taxonomy, or the JSONL `eventstream` sink.
- No new logging levels or event kinds.

---

## 1. Data model & history (reuse core)

### Problem
`hooks.Bus` is fire-and-forget — no consumer retains events. `eventlogger`
already subscribes to every `hooks.Kind` and formats each into one human line,
but keeps only counters, not the events. Its per-kind formatters are unexported.

### Design
Give `internal/eventlogger` a bounded **in-memory ring buffer** alongside its
existing INFO writing. It becomes the single component that both *narrates* and
*retains* — maximal reuse, one subscription.

- New record type (in `eventlogger`):
  ```go
  type LogRecord struct {
      Seq   int64      // monotonic
      At    time.Time  // hooks.Event.At
      Kind  hooks.Kind
      Tag   string     // hooks.MetaFor(Kind).Tag  (the bracket prefix / category)
      Scope hooks.ActionScope
      Msg   string     // rendered via the existing per-kind formatter
      IsErr bool        // derived (error-ish kinds); notifications set this directly
  }
  ```
- `Msg` reuses eventlogger's existing formatter table. Expose exactly one new
  exported function `eventlogger.Format(e hooks.Event) string` that returns the
  message string (no bracket tag — tag lives in `LogRecord.Tag`). The formatter
  map stays private; `Format` is the only new surface.
- Ring cap ≈ **1000** entries (session-only). Verbose kinds respect the same
  `Verbose` gate the logger already applies.
- Global accessor mirroring the existing `NotificationHistory()` sink pattern
  (`notification_history_sink.go`):
  ```go
  func History() LogHistory            // never nil (no-op default)
  func SetLogHistory(h LogHistory)     // wired once at startup
  ```
  `LogHistory.Snapshot() []LogRecord` returns newest-first (copy). Thread-safe
  (RWMutex), same discipline as the notification sink.
- Wired once at startup where the logger is already created: `cmd/beatmo.go`
  and `internal/ui/js_bootstrap_wasm.go`.

### Notifications stay put
`notificationStore` (UI, cap 100, userprefs-persisted) is **unchanged**. It
continues to feed the badge + error toast. The Logs panel **merges at read
time**: `eventlogger.History().Snapshot()` (bus events) ∪ notifications, sorted
newest-first by timestamp. Notifications map to `LogRecord`-shaped rows with
`Tag = "info"` / `Tag = "error"`. No data migration; both stores keep their
current jobs.

### Category derivation (data-driven — no hardcoded buckets)
The set of filter categories is the **distinct `Tag` values present in the
merged stream** (computed each rebuild). Because every event's tag comes from
`hooks.MetaFor(kind).Tag` and notifications contribute `info`/`error`, a new
`hooks.Kind` with a new tag appears as a filter label **automatically** with no
UI code change. There is no static Graph/Transport/Audio grouping.

---

## 2. `LogsComponent` (new UI overlay)

Modeled end-to-end on `InstrumentMenuComponent`
(`drumview_overlay_inst_comp.go`). Implements `compOverlayable`, embeds
`overlayBase`, wrapped in `compPortalOverlay`, opened via
`dv.portal().Open(PortalEntry{ID:"logs", ..., Modal:true})` following
`openInstMenuPortal`. A large centered overlay (sized like `SettingsOverlay`),
`Scrim:false`, `Modal:true`.

### Layout (top → bottom)
```
┌ Logs ──────────────────────────────────────────── [×] ┐
│  [🔍  search name / tag           ]   [ At | Range ]   │  header
│  time: [HH:MM:SS]        (At)                          │
│  from: [HH:MM:SS]  to: [HH:MM:SS]   (Range)           │
│  ┌ categories (distinct tags, scrollable chip strip)┐ │
│  │ All  node  edge  play  row  eq  fx  synth  import │ │  filter chips
│  └───────────────────────────────────────────────────┘ │
├───────────────────────────────────────────────────────┤
│  12:04:35.812  [node]   add node (i2,j3)              │
│  12:04:33.104  [play]   play start                    │  paginated body
│  12:04:31.500  [error]  import failed: bad JSON       │  (one page of rows)
│  …                                                    │
├───────────────────────────────────────────────────────┤
│              ‹  Page 2 / 17  ›     go to [   ]         │  footer / pagination
└───────────────────────────────────────────────────────┘
```

### Search-by-name (fuzzy)
- Reuse `ensureMenuSearchBox` / `drawMenuSearchBox` + `TextInput`.
- Filter with `MenuSearcher` / `FuzzyMatch` over each record's searchable text =
  `Tag + " " + Msg`. Because `Tag` is included, typing a category name (e.g.
  `node`) filters to that category. Fuzzy highlight positions render via
  `drawMenuRow`'s `Highlights`.

### Search-by-timestamp (exact + range)
A small segmented `At | Range` toggle (reuse the segmented-control / pill
pattern already used elsewhere; two Buttons).
- **At**: one `TextInput` accepting `HH:MM` or `HH:MM:SS`. Matches records whose
  timestamp equals that value at the entered precision (minute if seconds
  omitted, else exact second).
- **Range**: `From` + `To` `TextInput`s (same format). Matches
  `From ≤ At ≤ To`. Either bound may be empty (open-ended: From-only = "since",
  To-only = "until").
- Time parse helper is timezone-local, tolerant (`H:MM` etc.), returns "no
  filter" on empty/unparseable input (never errors the list). Unit-tested in Go.

### Category filter chips (data-driven)
- Distinct `Tag`s from the merged snapshot, sorted (stable: `error`/`info`
  pinned first for visibility, then alphabetical), preceded by an `All` chip.
- Horizontally-scrollable chip strip (reuse `MenuScroll`/button-row idiom) so an
  arbitrary number of tags fits without overflow.
- Multi-toggle: clicking chips selects a subset; `All` clears the subset. Active
  chips use the primary sunset-gold accent; inactive use `colSurface3`.
- New tags appear here automatically on the next rebuild.

### Rows
`drawMenuRow(btn, MenuRowSpec{ IconID: iconForTag(tag), IconTint: tagTint,
Label: Msg, Highlights: hl, LabelColor: (colError if IsErr else colTextPrimary),
TrailLabel: HH:MM:SS.mmm })`. Timestamp right-aligned muted (`colTextDim`);
error rows render label in `colError`. `iconForTag` maps a handful of common
tags to existing `IconID`s (play→IconPlay, import→IconImport, fx→IconFx, …) with
a generic fallback icon; unknown tags get the fallback — no per-tag maintenance
required for correctness.

### Pagination (primary navigation)
- Page size = number of body rows that fit the current panel height
  (recomputed on layout). Pagination is the primary navigator; **no vertical
  scrollbar** in the body.
- Footer: `‹` (`IconChevronLeft`) `Page X / M` `›` (`IconChevronRight`) + a
  "go to page" `TextInput` (numeric). Clamp to `[1, M]`.
- Applying any search/timestamp/category filter recomputes `M` from the filtered
  set and resets to page 1.
- Pure pagination math (`pageSlice(total, pageSize, page)`, `pageCount`)
  extracted as a small pure function, unit-tested in Go independent of rendering.

### State
```go
type logsState struct {
    nameQuery   string
    tsMode      timestampMode        // At | Range
    tsAt, tsFrom, tsTo string
    activeTags  map[string]bool      // empty ⇒ All
    page        int
    // derived each rebuild:
    filtered    []LogRecord
    distinctTags []string
    pageCount   int
}
```
Rebuild pipeline (pure, testable): `snapshot → applyName → applyTimestamp →
applyCategory → distinctTags(before category) → paginate`.

---

## 3. Logs icon (next to the gear)

Mirror `gridHelpBtn` exactly (see `game_new.go:149`, `grid_pane_draw.go:1333`,
`game_input_shortcuts.go:296`, `grid_help_input_handler.go`,
`game_update.go:419`).

- New field `gridLogsBtn *Button` (`game_struct.go`), constructed with
  `ComponentButtonSecondary` and `Icon = string(IconLogs)`, callback
  `g.toggleLogsPanel`.
- New `IconLogs IconID = "logs"` in `icons.go` + `drawLogsIcon` (a document /
  stacked-lines glyph) in the icon renderer.
- `gridLogsButtonRect()`: same top edge, one slot left of the gear —
  `x = gearRect.Min.X - gap - sz` (sz=30, gap≈SpaceXS). Refreshed each frame in
  the draw path and in `game_layout.go` alongside the gear rect.
- `gridLogsInputHandler` (sibling of `gridHelpInputHandler`) registered in
  `game_update.go` at z ≈ 300 (equal priority band; the two rects don't
  overlap). Sets a `gridLogsCapturing` guard like the gear so the just-opened
  overlay isn't immediately closed by click-outside.
- `toggleLogsPanel()` mirrors `toggleSettingsOverlay()`: portal `Has("logs")`
  → close, else open.

### Unread badge
- The icon shows a small count bubble = number of new events + notifications
  since the panel was last opened (`sinceSeq` watermark). Bubble fill turns
  `colError` when any unread record `IsErr`; otherwise the accent/surface color.
- Opening the panel sets the watermark to the newest seq → clears the badge.
- Count drawn via a small rounded bubble at the icon's top-right corner using
  tokens (no inline literals).

### Error-only toast
- The removed in-band area is replaced (for *alerting*) by a minimal transient
  toast, drawn top-of-grid, `colError`, that fires **only** for error
  notifications (`notifyError*`) — not routine info events.
- Auto-fades using frame-count timing (reuse the marquee-style frame math
  approach; the marquee *scroll* helper is deleted but the frame-countdown idea
  is trivial). No input, no portal — a passive draw with a TTL. Tapping it (or
  the badge) opens the Logs panel.

---

## 4. Removal of the in-band notification component

- `timeline_zone.go`: remove `drawNotifArea`, its `Draw` call site, the
  `NotifRect`/`NotifLatest`/`OnNotifClick` callback fields, the
  `"timeline-notif"` hit-area registration, and `notifClickAdapter`.
- `drumview_layout.go`: remove the `dv.notifRect` carving (lines ~648-685); the
  band reclaims that width for the beat counter/readout.
- `drumview_ctor.go`: remove the `NotifRect`/`NotifLatest`/`OnNotifClick`
  bindings.
- Delete `notification_marquee.go` (marquee no longer used).
- Delete `drumview_notif_history.go` + `openNotifHistoryPortal` /
  `computeNotifHistoryRect` / `drawNotifHistory` / `IsNotifHistoryOpen` (old
  popup replaced by the Logs panel).
- **Keep**: `notification_store.go`, `notification_history_sink.go`,
  `notification_legacy.go`, and the `notifyInfo*/notifyError*` push APIs — they
  feed the badge, the error toast, and the Logs-panel merge.

---

## 5. DESIGN.md compliance

- Tokens only: `colSurface2/3`, `colBorderSubtle`, `RadiusLG`/`RadiusMD`,
  `Space*`, `colTextPrimary/Mid/Dim`, `colError`, primary accent for active
  chips / focused inputs. Zero inline `color.RGBA` / `color.NRGBA` (respect
  `token_discipline_test.go` ratchet; new files enter the budget map with
  review, but prefer `Token*()` accessors).
- Icons via `IconID` + `DrawIcon` only. Page nav uses existing
  `IconChevronLeft/Right`; close uses `IconClose`; search uses `IconSearch`;
  new `IconLogs` added. No forbidden glyphs.
- i18n: new keys in `keys.go` + `catalog_en.go` + `catalog_es.go`:
  `KeyLogs`, `KeyLogsSearchName`, `KeyLogsTimeAt`, `KeyLogsTimeRange`,
  `KeyLogsFrom`, `KeyLogsTo`, `KeyLogsAllCategories`, `KeyLogsPage`,
  `KeyLogsGoToPage`, `KeyNoLogs`. Row messages come from `eventlogger.Format`
  (already localized where the underlying formatter is) + translated
  notification `display()`.

---

## 6. Testing (functional-first, per repo convention)

Go fast-path tests driving real `Game.Update` (ebitenstub), plus pure-unit
tests for the extractable logic:

- `eventlogger` ring: records retained, cap enforced (oldest evicted), verbose
  gate respected, `Snapshot()` newest-first + copy-safety, `Format` output per
  kind. (`internal/eventlogger`)
- Pure logic: `pageSlice`/`pageCount`; timestamp parse + At/Range match;
  category distinct-set + filter; name fuzzy filter includes tag text.
- Functional (open panel via logs icon → assert portal `Has("logs")`; type in
  search → list filters; set At/Range → list filters; toggle chips → list
  filters + new tag auto-appears; pagination next/prev/jump + clamp; filter
  resets to page 1).
- Badge: increments on new event/notification, red on error, clears on open.
- Error toast: fires on `notifyError`, NOT on info events, auto-dismisses;
  tapping opens the panel.
- Gear protected-zone tests (`game_settings_gear_zone_test.go`) widened to cover
  both the gear and the new logs icon (both topmost, both avoided by menus).
- Discipline: token discipline + forbidden-glyph + zone-input-isolation
  (logs overlay is a modal portal — follows `compOverlayable`, no catch-all
  leak).
- Removal regressions: in-band notif area gone (no `notifRect`), timeline band
  reflows; no dangling references to deleted popup.

---

## Open questions / assumptions

- Timestamp inputs are timezone-local, second precision; sub-second only in the
  row `TrailLabel` display, not in the filter. (Assumed adequate.)
- Merge is read-time in the UI; if event volume ever makes per-open merge cost
  visible, the merge can move behind a cached snapshot invalidated on push.
  (Not needed at cap 1000 + 100.)
