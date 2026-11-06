# Logs Panel — Date/Time Picker + Button styling — Design

**Date:** 2026-07-20
**Status:** Approved (design)
**Author:** Claude (with ingyamilmolinar@gmail.com)
**Builds on:** `2026-07-19-logs-navigation-panel-design.md` (the shipped Logs panel)

## Goal

Two enhancements to the just-shipped Logs Navigation Panel:

1. **Every control reads as a real button.** The pagination prev/next, the close
   button, the category chips, and the At|Range toggle currently render as bare
   icons / custom pills. Route them all through the app's real `Button` widget
   (keycap chrome + press animation + hover), matching the transport/gear
   buttons and the beat-counter keycap direction.
2. **A calendar + time popup picker** replaces manual typing in the timestamp
   filter. Clicking a date/time field opens a popup: a month calendar picks the
   day, a time stepper picks the hour/minute. This upgrades the timestamp filter
   from **time-of-day** to **absolute date+time** (persisted notifications can be
   days old; live events are today-only — a real calendar is meaningful).

## Non-goals

- No change to the event ring, the notification store, the search box, the
  pagination logic, or the badge/toast.
- No seconds-level precision in the picker UI (minute precision; the row display
  keeps `HH:MM:SS.mmm`).
- No recurring/relative filters ("last hour"); absolute moments only.

---

## 1. Model change: time-of-day → absolute datetime

`logs_model.go` currently filters on seconds-of-day via `parseLogTime` /
`matchTimeAt` / `matchTimeRange` (string `HH:MM:SS`). Replace with absolute
moments:

```go
// logsTimeSel is one selected filter moment (unset = open-ended / no filter).
type logsTimeSel struct {
    set bool
    t   time.Time // minute precision (seconds/nanos zeroed)
}

// matchAtAbs: unset ⇒ true (no filter); set ⇒ at is within sel's MINUTE.
func matchAtAbs(at time.Time, sel logsTimeSel) bool

// matchRangeAbs: from.set ⇒ at >= from.t; to.set ⇒ at <= endOfMinute(to.t);
// both unset ⇒ true. Either bound optional (open-ended).
func matchRangeAbs(at time.Time, from, to logsTimeSel) bool
```

- `filterRows` signature changes: the timestamp params become
  `tsMode timestampMode, at, from, to logsTimeSel` (was three strings).
- `parseLogTime`, `matchTimeAt`, `matchTimeRange`, `atSecondsOfDay`, `timePrec`,
  `precNone/precMinute/precSecond` are **removed** (no more text parsing).
- `truncateToMinute(t) time.Time` and `endOfMinute(t) time.Time` helpers.
- Comparison uses `time.Time.Before/After/Equal` on absolute instants.

**Tests** (`logs_model_test.go`, updating the removed matchers' tests):
- `matchAtAbs`: unset→true; same-minute→true; different-minute→false; different
  day same clock-time → false (proves date-awareness).
- `matchRangeAbs`: in-range; out-of-range; open-ended from; open-ended to;
  both-unset→true; a moment in the `to` minute is inclusive; inverted range→empty.
- `filterRows` datetime cases wired through the new params.

---

## 2. Date/Time Picker component (`logs_datetime_picker.go`)

A popup opened as a **stacked portal** (on top of the logs modal, which stays
open beneath it). Owned by `LogsComponent`; one reused instance.

**State:**
```go
type logsDateTimePicker struct {
    open      bool
    target    logsPickTarget // which field it edits: At | From | To
    viewYear  int
    viewMonth time.Month     // the month currently shown in the grid
    sel       time.Time      // the working selection (day + hour + minute)
    // Button objects (lazily built, reused):
    monthPrev, monthNext *Button
    dayBtns              [42]*Button
    hourDec, hourInc     *Button
    minDec, minInc       *Button
    setBtn, clearBtn, cancelBtn *Button
    rect image.Rectangle
}
```

**Layout (top→bottom), all clickable elements are real `Button`s:**
```
┌ Pick date & time ───────────────┐
│  ‹      July 2026        ›       │  month nav (keycap buttons)
│  Su Mo Tu We Th Fr Sa           │  weekday header (text)
│  30  1  2  3  4  5  6            │  6×7 day buttons; selected=accent,
│   7  8  9 10 11 12 13            │  today=subtle ring, other-month=dim
│  ...                            │
│        [−] 14 : 30 [+]          │  hour/min steppers (keycap buttons)
│   [ Clear ]  [ Cancel ] [ Set ] │  footer buttons
└─────────────────────────────────┘
```

**Grid math** (pure, tested — `logs_datetime_grid.go` or inline):
- `daysInMonth(year, month) int`
- `firstCellDate(year, month) time.Time` — the date shown in grid cell 0 (the
  Sunday on/before the 1st), so the 42 cells fill Sun-first weeks.
- `gridDays(year, month) [42]time.Time` — the 42 dates rendered.

**`nowFn` seam:** `var logsNowFn = func() time.Time { return time.Now() }` so
tests pin "today" and the default view month deterministically (mirrors
`nowUnixMilli`).

**Interactions:**
- Open (seeded): `openPicker(target, seed logsTimeSel)` — `viewYear/Month` and
  `sel` come from `seed.t` when set, else `logsNowFn()`.
- Day button → set `sel`'s date (keep hour/min); month arrows change
  `viewYear/Month` (rebuild day buttons).
- Hour/min steppers → adjust `sel` clock (wrap 0–23 / 0–59).
- **Set** → write `logsTimeSel{set:true, t:truncateToMinute(sel)}` back to the
  target field on `LogsComponent`, `reapply()`, close picker.
- **Clear** → write `logsTimeSel{set:false}`, `reapply()`, close.
- **Cancel** / click-outside / Esc → close, no change.

**Portal:** opened via `dv.portal().Open(PortalEntry{ID:"logs-datetime", Overlay:
&compPortalOverlay{comp: picker, tag:"logs-datetime"}, Modal:true, Scrim:true,
Anchor: fieldRect})`. `compOverlayable` methods on the picker. Anchored below the
clicked field, clamped into `dv.Bounds`. Closing the picker returns to the logs
panel (still open beneath).

---

## 3. Button styling pass (LogsComponent)

Replace bare-icon/custom draws with real `Button` widgets:

| Control | Before | After |
|---|---|---|
| Prev / Next page | `DrawIcon(IconChevronLeft/Right)` | `Button` (`ComponentButtonSecondary`, icon set), `Button.Draw` keycap chrome |
| Close | `DrawIcon(IconClose)` | `Button` with `IconClose` |
| Category chips | `drawLogsPill` (custom rect) | one `Button` per chip, rendered via `drawPillTabAt(btn, active)` (the audio-panel pill renderer with press springs); rebuilt when the tag set changes |
| At / Range toggle | `drawLogsPill` | two `Button`s via `drawPillTabAt(btn, active==mode)` |
| At / From / To fields | 3 `TextInput`s (typed) | button-styled value chips (keycap, value text + `IconCalendar`) that open the picker |

- Button objects held on `LogsComponent`, constructed lazily, rects set each
  `Draw`, `OnClick` bound to the existing handlers (page±, close, `toggleTag`,
  `setTimeMode`, `openPicker`).
- Input: prefer routing presses through the buttons' `HandleInputResult` in
  `HandleInput` (like other zones) so press animation + hover fire; keep the
  modal-consume fallback so nothing leaks to the background.
- `drawLogsPill` is removed (superseded by `drawPillTabAt` + `Button`).
- Hover: forward `HoverAt` to the buttons (the picker + panel implement
  `portalHoverer`) so desktop hover works.

---

## 4. Wiring & cleanup

- Remove `logs_component_time.go`'s `TextInput`-based controls
  (`ensureTimeBox`, `drawTimeControls`, `updateTimeBoxes`, `handleTimeInput`,
  `acceptTimeRune`, `boxVisible`, `hideTimeBox`) — replaced by the field-buttons
  + picker. Keep `setTimeMode`; add `setPickTarget`/`openPicker`.
- Remove the **mobile native-input wiring for the time boxes** in
  `native_gesture.go` (the `logs-search` name-box entry STAYS; the picker needs
  no keyboard). Drop `tsAtBox/tsFromBox/tsToBox` fields; add `atSel/fromSel/toSel
  logsTimeSel`.
- `LogsComponent.Update` still drives the name search box; the picker (when open)
  is driven by its portal `updateFn`.
- Field display text: `atSel`/`fromSel`/`toSel` formatted as
  `2006-01-02 15:04` (or a localized short form); empty label when unset (e.g.
  "Any time").

---

## 5. DESIGN.md compliance

- Tokens only: keycap/pill tokens (`pillCapFill*`, `pillText*`, button styles),
  `colAccent`, surfaces, text, `RadiusMD/LG`, `Space*`. No inline
  `color.RGBA`/`NRGBA`.
- New `IconCalendar IconID = "calendar"` + `drawCalendarIcon` (canvas + stroke
  helpers, mirror `drawFolderIcon`) + dispatch case + an icon-dispatch test.
- All glyphs via `IconID`; month-nav arrows use `IconChevronLeft/Right`,
  steppers use `IconMinus`/`IconPlus`. No forbidden glyphs.
- i18n keys (en + es), **option (a) full localization**:
  - `KeyLogsPickTitle` ("Pick date & time"), `KeyLogsSet`, `KeyLogsClear`,
    `KeyLogsAnyTime` (empty-field label). (`KeyCancel` already exists.)
  - Months: `KeyMonth1…KeyMonth12` (full names) — or reuse if the app already
    has month keys; add if not.
  - Weekday abbreviations: `KeyWeekdaySun…KeyWeekdaySat` (Su/Mo/… localized).
  - All added in `keys.go` + `AllKeys()` + `catalog_en.go` + `catalog_es.go`.

---

## 6. Testing

- **Pure**: `daysInMonth`, `firstCellDate`, `gridDays` (leap year, month
  boundaries, Sun-first alignment); `matchAtAbs`/`matchRangeAbs` (date-aware,
  inclusive `to`-minute, open-ended, inverted).
- **Functional** (drive real `Game.Update` / component):
  - Click an At field → picker portal opens (`portal().Has("logs-datetime")`),
    seeded to the field's value / `logsNowFn`.
  - Click a day + Set → field updates, filter re-applies, picker closes.
  - Clear → field open-ended; Cancel → no change.
  - Month prev/next changes the shown grid.
  - Hour/min steppers wrap correctly.
- **Button/aesthetics**: prev/next/close/chip/toggle are `Button`s and their
  click drives the action (open picker, page±, toggle); a render test asserts the
  keycap/pill path is taken (interceptor on `Button.Draw`/`drawPillTabAt`), not a
  bare `DrawIcon`.
- **Discipline**: token discipline, forbidden-glyph, i18n parity, design-lint.
- **Removal regressions**: no dangling refs to `parseLogTime`/`matchTimeAt`/
  `matchTimeRange`/`drawLogsPill`/`ensureTimeBox`/time-box native input.

## Open questions / assumptions

- Sun-first week (matches US/most locales); not making week-start locale-aware.
- Picker seed when the field is unset defaults to `logsNowFn()` truncated to the
  minute.
- The picker is modal-within-modal; only one picker open at a time (reused
  instance keyed by `target`).
