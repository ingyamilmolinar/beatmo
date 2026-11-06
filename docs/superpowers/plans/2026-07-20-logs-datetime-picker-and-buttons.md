# Logs Date/Time Picker + Button styling — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`. NOTE: the working tree is edited concurrently by the user → execute with **inline edits + subagent reviews, no git commits** (per the standing never-commit rule); each task ends at a green, unstaged checkpoint. Snapshot dirty files before editing for clean per-task review diffs.

**Goal:** Make every Logs-panel control a real keycap/pill Button, and replace typed timestamp fields with a calendar+time popup picker (absolute date+time filtering).

**Architecture:** Additive-first. Add absolute-datetime matchers + calendar grid math + the picker component alongside the existing code (build stays green), do the button-styling pass, then flip the panel from typed time-boxes to field-buttons+picker and delete the obsolete string-parse/time-box path.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`, `-tags test -modfile=go.test.mod`), Ebiten v2, `internal/ui`, `internal/i18n`.

## Global Constraints

- **Bundled Go**, fast test cmd: from `src/go`: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./<pkg>`.
- **No commits.** Inline edits, unstaged; snapshot dirty files pre-edit for per-task diffs; subagent review per task.
- **DESIGN.md**: tokens only (no inline `color.RGBA`/`NRGBA`); glyphs via `IconID`+`DrawIcon` (no forbidden glyphs); Buttons via the app `Button` widget + `drawPillTabAt`/`Button.Draw`.
- **i18n**: every string an `i18n.Key` in `keys.go` + `AllKeys()` + `catalog_en.go` + `catalog_es.go` (parity guard).
- **Sun-first week**, minute precision, `logsNowFn` seam for deterministic "today".
- **Keep the build green every task** (additive ordering); never touch untracked user WIP.

---

## File Structure

- Create: `logs_datetime_grid.go` (pure grid math), `logs_datetime_picker.go` (picker component), `logs_datetime_picker_test.go`.
- Modify: `logs_model.go` (+`logsTimeSel`/`matchAtAbs`/`matchRangeAbs`, then remove old parsers + change `filterRows`), `logs_model_test.go`, `logs_component.go` (buttons + sel fields + picker wiring), `logs_component_time.go` (remove time-box UI; keep/add mode+field helpers), `logs_component_test.go`, `native_gesture.go` (drop time-box entries), `icons.go`+`drawing_icons.go` (`IconCalendar`), `keys.go`+`catalog_en.go`+`catalog_es.go`.

---

## Task 1: Absolute-datetime matchers (additive)

**Files:** `logs_model.go`, `logs_model_test.go`

**Interfaces produced:** `type logsTimeSel struct{ set bool; t time.Time }`; `truncateToMinute(time.Time) time.Time`; `endOfMinute(time.Time) time.Time`; `matchAtAbs(at time.Time, sel logsTimeSel) bool`; `matchRangeAbs(at time.Time, from, to logsTimeSel) bool`.

- [ ] **Step 1 — failing tests.** Add to `logs_model_test.go`:
```go
func TestMatchAtAbs(t *testing.T) {
	base := time.Date(2026, 7, 19, 14, 30, 20, 0, time.Local)
	if !matchAtAbs(base, logsTimeSel{}) {
		t.Fatal("unset = no filter (match)")
	}
	sel := logsTimeSel{set: true, t: time.Date(2026, 7, 19, 14, 30, 0, 0, time.Local)}
	if !matchAtAbs(base, sel) {
		t.Fatal("same minute should match")
	}
	if matchAtAbs(base.Add(2*time.Minute), sel) {
		t.Fatal("different minute must not match")
	}
	if matchAtAbs(base.AddDate(0, 0, -1), sel) {
		t.Fatal("same clock time, different DAY must not match (date-aware)")
	}
}

func TestMatchRangeAbs(t *testing.T) {
	d := func(h, m int) time.Time { return time.Date(2026, 7, 19, h, m, 0, 0, time.Local) }
	from := logsTimeSel{set: true, t: d(12, 0)}
	to := logsTimeSel{set: true, t: d(13, 0)}
	if !matchRangeAbs(d(12, 30), from, to) {
		t.Fatal("in-range")
	}
	if matchRangeAbs(d(14, 0), from, to) {
		t.Fatal("out-of-range")
	}
	if !matchRangeAbs(d(13, 0).Add(59*time.Second), from, to) {
		t.Fatal("a moment in the `to` minute is inclusive")
	}
	if !matchRangeAbs(d(9, 0), from, logsTimeSel{}) {
		t.Fatal("open-ended to (since)")
	}
	if !matchRangeAbs(d(9, 0), logsTimeSel{}, to) {
		t.Fatal("open-ended from (until)")
	}
	if !matchRangeAbs(d(9, 9), logsTimeSel{}, logsTimeSel{}) {
		t.Fatal("both unset = no filter")
	}
}
```
- [ ] **Step 2 — run, expect FAIL** (`-run 'TestMatchAtAbs|TestMatchRangeAbs'`): undefined.
- [ ] **Step 3 — implement** in `logs_model.go`:
```go
type logsTimeSel struct {
	set bool
	t   time.Time
}

func truncateToMinute(t time.Time) time.Time { return t.Truncate(time.Minute) }
func endOfMinute(t time.Time) time.Time      { return truncateToMinute(t).Add(time.Minute - time.Nanosecond) }

// matchAtAbs: unset ⇒ no filter (true); set ⇒ at falls within sel's minute.
func matchAtAbs(at time.Time, sel logsTimeSel) bool {
	if !sel.set {
		return true
	}
	m := truncateToMinute(sel.t)
	return !at.Before(m) && !at.After(endOfMinute(sel.t))
}

// matchRangeAbs: from.set ⇒ at >= from-minute-start; to.set ⇒ at <= to-minute-end.
func matchRangeAbs(at time.Time, from, to logsTimeSel) bool {
	if from.set && at.Before(truncateToMinute(from.t)) {
		return false
	}
	if to.set && at.After(endOfMinute(to.t)) {
		return false
	}
	return true
}
```
- [ ] **Step 4 — run, expect PASS.**
- [ ] **Step 5 — checkpoint:** `go test ./internal/ui/ -run 'TestMatchAtAbs|TestMatchRangeAbs'`; leave unstaged.

---

## Task 2: Calendar grid math (additive, pure)

**Files:** Create `logs_datetime_grid.go`, add tests to `logs_datetime_picker_test.go`.

**Interfaces produced:** `var logsNowFn = func() time.Time { return time.Now() }`; `daysInMonth(year int, m time.Month) int`; `firstCellDate(year int, m time.Month) time.Time` (Sunday on/before the 1st, at 00:00 local); `gridDays(year int, m time.Month) [42]time.Time`.

- [ ] **Step 1 — failing tests** (`//go:build test` file `logs_datetime_picker_test.go`):
```go
func TestDaysInMonth(t *testing.T) {
	if daysInMonth(2026, time.February) != 28 {
		t.Fatal("Feb 2026 = 28")
	}
	if daysInMonth(2024, time.February) != 29 {
		t.Fatal("Feb 2024 leap = 29")
	}
	if daysInMonth(2026, time.July) != 31 {
		t.Fatal("Jul = 31")
	}
}

func TestGridDaysSunFirstAndCoversMonth(t *testing.T) {
	g := gridDays(2026, time.July) // Jul 1 2026 is a Wednesday
	if g[0].Weekday() != time.Sunday {
		t.Fatalf("cell 0 must be a Sunday, got %v", g[0].Weekday())
	}
	// The 1st of the month must appear in the first week.
	found := -1
	for i, d := range g {
		if d.Year() == 2026 && d.Month() == time.July && d.Day() == 1 {
			found = i
		}
	}
	if found < 0 || found > 6 {
		t.Fatalf("Jul 1 should be in the first week, at cell %d", found)
	}
	if len(g) != 42 {
		t.Fatalf("grid must be 42 cells, got %d", len(g))
	}
}
```
- [ ] **Step 2 — run, expect FAIL.**
- [ ] **Step 3 — implement** `logs_datetime_grid.go`:
```go
package ui

import "time"

// logsNowFn returns "now"; overridable in tests for a deterministic calendar.
var logsNowFn = func() time.Time { return time.Now() }

func daysInMonth(year int, m time.Month) int {
	// Day 0 of next month == last day of this month.
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// firstCellDate is the Sunday on/before the 1st of (year, m), at local midnight.
func firstCellDate(year int, m time.Month) time.Time {
	first := time.Date(year, m, 1, 0, 0, 0, 0, time.Local)
	return first.AddDate(0, 0, -int(first.Weekday())) // Sunday==0
}

// gridDays returns the 42 Sun-first calendar cells covering (year, m).
func gridDays(year int, m time.Month) [42]time.Time {
	start := firstCellDate(year, m)
	var g [42]time.Time
	for i := 0; i < 42; i++ {
		g[i] = start.AddDate(0, 0, i)
	}
	return g
}
```
- [ ] **Step 4 — run, expect PASS.**
- [ ] **Step 5 — checkpoint** + unstaged.

---

## Task 3: i18n keys (months, weekdays, picker labels)

**Files:** `keys.go`, `catalog_en.go`, `catalog_es.go`.

**Keys:** `KeyLogsPickTitle`, `KeyLogsSet`, `KeyLogsClear`, `KeyLogsAnyTime`; `KeyMonth1..KeyMonth12`; `KeyWeekdaySun..KeyWeekdaySat`. (`KeyCancel` reused.)

- [ ] **Step 1 — declare consts** in `keys.go` (after the existing `KeyLogs*` block):
```go
	KeyLogsPickTitle Key = "logs.pick_title"
	KeyLogsSet       Key = "logs.set"
	KeyLogsClear     Key = "logs.clear"
	KeyLogsAnyTime   Key = "logs.any_time"
	KeyMonth1        Key = "cal.month.1"
	// … KeyMonth2..KeyMonth12 (cal.month.2 … cal.month.12)
	KeyWeekdaySun    Key = "cal.wd.sun"
	// … KeyWeekdayMon..KeyWeekdaySat (cal.wd.mon … cal.wd.sat)
```
- [ ] **Step 2 — add to `AllKeys()`** (all 4 + 12 + 7 keys).
- [ ] **Step 3 — English** (`catalog_en.go`): "Pick date & time"/"Set"/"Clear"/"Any time"; months "January".."December"; weekdays "Su".."Sa".
- [ ] **Step 4 — Spanish** (`catalog_es.go`): "Elegir fecha y hora"/"Aplicar"/"Borrar"/"Cualquier hora"; months "enero".."diciembre"; weekdays "Do"/"Lu"/"Ma"/"Mi"/"Ju"/"Vi"/"Sa".
- [ ] **Step 5 — run** `go test ./internal/i18n/` → PASS (parity). Checkpoint.

> Helpers to add near the picker (Task 5): `monthKey(m time.Month) i18n.Key` and `weekdayKey(time.Weekday) i18n.Key` mapping to the above.

---

## Task 4: `IconCalendar`

**Files:** `icons.go`, `drawing_icons.go`, test in `logs_datetime_picker_test.go`.

- [ ] **Step 1 — failing dispatch test:**
```go
func TestIconCalendarDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconCalendar, image.Rect(0, 0, 24, 24), color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("drawIconByID false for IconCalendar")
	}
}
```
- [ ] **Step 2 — run FAIL.**
- [ ] **Step 3 — add** `IconCalendar IconID = "calendar"` (const block), a `case IconCalendar: drawCalendarIcon(...)` in the DrawIcon switch, and in `drawing_icons.go`:
```go
// Calendar: a box with a top binding + a header rule (the date-picker glyph).
var drawCalendarIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeOpenRoundedRect(dst, c, 4, 5, 16, 15, float32(IconCornerRadius), "", col) // body
	iconStrokeLine(dst, c, 4, 9, 20, 9, col)                                            // header rule
	iconStrokeLine(dst, c, 8, 3, 8, 6, col)                                             // left binding
	iconStrokeLine(dst, c, 16, 3, 16, 6, col)                                           // right binding
}
```
- [ ] **Step 4 — run PASS.** Checkpoint.

---

## Task 5: Date/Time picker component

**Files:** Create `logs_datetime_picker.go`; tests in `logs_datetime_picker_test.go`.

**Interfaces produced:** `type logsPickTarget int` (`pickAt`/`pickFrom`/`pickTo`); `type logsDateTimePicker struct{…}`; `newLogsDateTimePicker(onCommit func(logsPickTarget, logsTimeSel)) *logsDateTimePicker`; methods `openAt(target, seed logsTimeSel)`, `IsOpen()`, plus `compOverlayable` (`Close/InputBounds/HandleInput/HandleWheel/Capturing/Draw`) + `Update()`. Buttons: month prev/next, 42 day buttons, hour/min ±, Set/Clear/Cancel.

- [ ] **Step 1 — failing behavior test** (drives the picker directly):
```go
func TestPickerSetCommitsTruncatedMinute(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local))
	defer restore()
	var got struct {
		target logsPickTarget
		sel    logsTimeSel
	}
	p := newLogsDateTimePicker(func(tg logsPickTarget, s logsTimeSel) { got.target, got.sel = tg, s })
	p.openAt(pickFrom, logsTimeSel{})
	if !p.IsOpen() {
		t.Fatal("picker should be open")
	}
	p.selectDay(time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local))
	p.setHour(9)
	p.setMinute(45)
	p.commitSet()
	if got.target != pickFrom || !got.sel.set {
		t.Fatalf("commit target/set wrong: %+v", got)
	}
	if !got.sel.t.Equal(time.Date(2026, 7, 15, 9, 45, 0, 0, time.Local)) {
		t.Fatalf("committed moment = %v, want 2026-07-15 09:45", got.sel.t)
	}
	if p.IsOpen() {
		t.Fatal("Set should close the picker")
	}
}

func TestPickerClearCommitsUnset(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local))
	defer restore()
	var got logsTimeSel
	got.set = true
	p := newLogsDateTimePicker(func(_ logsPickTarget, s logsTimeSel) { got = s })
	p.openAt(pickAt, logsTimeSel{set: true, t: logsNowFn()})
	p.commitClear()
	if got.set {
		t.Fatal("Clear must commit an unset selection")
	}
}
```
Add the test seam helper (in the test file):
```go
func setLogsNow(t *testing.T, at time.Time) func() {
	prev := logsNowFn
	logsNowFn = func() time.Time { return at }
	return func() { logsNowFn = prev }
}
```
- [ ] **Step 2 — run FAIL.**
- [ ] **Step 3 — implement** `logs_datetime_picker.go`: the struct + `newLogsDateTimePicker` + `openAt` (seed `viewYear/viewMonth/sel` from seed or `logsNowFn`) + `selectDay`/`setHour`/`setMinute` (wrap 0–23 / 0–59) + `commitSet` (`onCommit(target, logsTimeSel{set:true, t:truncateToMinute(sel)})` then close) + `commitClear` (`onCommit(target, logsTimeSel{})` then close) + `IsOpen/Close`. Layout/draw/hit (below) build the Buttons and route clicks to these methods. Use `gridDays(viewYear, viewMonth)` for the 42 day buttons; month prev/next adjust `viewYear/viewMonth`. All clickable elements are `Button`s (`NewSpecButton`/`NewButtonKey`), drawn via `Button.Draw`; the selected day + active states use accent (`drawPillTabAt` for the day cells is acceptable, or `Button` active style). `Draw` paints a `colSurface2`/`colBorderSubtle` `RadiusLG` panel + `KeyLogsPickTitle` + weekday header (`weekdayKey`) + grid + `HH:MM` with ± steppers + footer (Set/Clear/Cancel). `HandleInput` routes to each button's `HandleInputResult`; consume all in-panel presses. `panelRect` anchored/clamped in `dv.Bounds` (pass bounds via a field set on open, or a `boundsFn`). Portal open is wired in Task 7.
- [ ] **Step 4 — run PASS** (behavior tests don't need Draw). Checkpoint (token discipline).

---

## Task 6: Button styling pass on the panel

**Files:** `logs_component.go` (+ remove `drawLogsPill`), `logs_component_test.go`.

- [ ] **Step 1 — failing render/behavior test:** assert prev/next/close are `*Button`s whose click drives paging/close, and chips/toggle render via `drawPillTabAt` (intercept `drawPillTabAt`/`Button.Draw`, assert called; assert NO bare `DrawIcon(IconChevron*)` for the footer). Concretely, drive: open panel, `Draw`, click prev/next button rects → page changes; click a chip button → tag toggles.
- [ ] **Step 2 — run FAIL.**
- [ ] **Step 3 — implement:** add `Button` fields (`prevBtn,nextBtn,closeBtn *Button`; `atModeBtn,rangeModeBtn *Button`; `chipBtns []*Button` + parallel `chipKeys []string`). Construct lazily; in `Draw` set rects + draw via `Button.Draw` (prev/next/close) and `drawPillTabAt` (chips/toggle, `active` per state); bind `OnClick`. Rebuild `chipBtns` when `distinctTags` changes. `HandleInput` routes presses to these buttons' `HandleInputResult` first (so press anim fires), then the consume fallback. Delete `drawLogsPill` + `drawOneChip`'s manual rect fill (now via button). Add `HoverAt` forwarding to the buttons.
- [ ] **Step 4 — run PASS** (+ `TestTokenDiscipline`, `TestDesignMDDrift`). Checkpoint.

---

## Task 7: Flip fields → picker; remove typed time-boxes (integration)

**Files:** `logs_component.go`, `logs_component_time.go`, `logs_model.go` (remove old parsers + change `filterRows`), `native_gesture.go`, `drumview_logs_panel.go` (picker portal id/open).

- [ ] **Step 1 — failing functional test** (in `logs_component_test.go`): open panel; click the At field-button → `dv.portal().Has("logs-datetime")` true; drive the picker's `selectDay`+`commitSet`; assert `c.atSel.set` and the filtered rows reflect the chosen minute; Clear → open-ended.
- [ ] **Step 2 — run FAIL.**
- [ ] **Step 3 — implement:**
  - Replace `tsAt/tsFrom/tsTo string` with `atSel, fromSel, toSel logsTimeSel`; drop `tsAtBox/tsFromBox/tsToBox`.
  - `filterRows` new signature `filterRows(rows, name, activeTags, tsMode, at, from, to logsTimeSel)`; its timestamp branch calls `matchAtAbs`/`matchRangeAbs`. Update `reapply`/`rebuild` call sites.
  - Remove old `parseLogTime/matchTimeAt/matchTimeRange/atSecondsOfDay/timePrec` from `logs_model.go` and their now-obsolete tests.
  - `logs_component_time.go`: remove `ensureTimeBox/drawTimeControls/updateTimeBoxes/handleTimeInput/acceptTimeRune/boxVisible/hideTimeBox`; keep `setTimeMode`; add `setTimeSel(target, logsTimeSel)` (writes the right field + `reapply`), `openPicker(target)` (seed from the field, call `dv` to open the picker portal), and `drawTimeFieldButtons` (draws At-value / From/To value chips with `IconCalendar`, storing their rects/buttons).
  - `logs_component.go` `Draw`: draw the mode toggle (Task 6 buttons) + the field-buttons (via `drawTimeFieldButtons`); `Update` no longer drives time boxes; `HandleInput` routes field-button presses → `openPicker`.
  - `drumview_logs_panel.go`: add `logsDatePickerID = "logs-datetime"`; `openLogsDatePicker(target, seed, anchor)` opens the picker portal (`compPortalOverlay`, Modal:true, Scrim:true, Anchor:anchor); the picker's `onCommit` calls `c.setTimeSel`.
  - `native_gesture.go`: remove the (now-nonexistent) time-box entries if any remain; the `logs-search` name entry stays. (Time boxes were never given a native entry beyond search — verify none reference `tsAtBox` etc.)
  - The picker instance lives on `LogsComponent` (`picker *logsDateTimePicker`), lazily created with `onCommit = c.setTimeSel`.
- [ ] **Step 4 — build + run** the new functional test + all `Logs*` tests → PASS. Checkpoint.

---

## Task 8: Functional sweep + discipline + verification

**Files:** `logs_component_test.go`, `logs_datetime_picker_test.go`.

- [ ] **Step 1 — add functional tests:** picker seeds from an already-set field; month prev/next changes the shown grid month; hour/min steppers wrap (23→0, 0→23; 59→0); Cancel leaves the field unchanged; a picked From+To range filters rows across two days (proves date-awareness end-to-end).
- [ ] **Step 2 — run** the full logs test set: `go test ./internal/ui/ -run 'Logs|Picker|MatchAtAbs|MatchRangeAbs|DaysInMonth|GridDays|IconCalendar|GridTopRightIcons'` → PASS.
- [ ] **Step 3 — discipline:** `go test ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift|TestDesignMDLintSnapshot|TestNoUppercaseInfoTags'`; `go test ./internal/i18n/`; gofmt -l my files (empty).
- [ ] **Step 4 — full suite** `go test ./internal/ui/` (background; long). Confirm the ONLY failures are the pre-existing user-WIP overflow tests (`TestSynthMobileOverflowMenuHasSaveAsReset`, `TestSamplerMobileOverflowMenuHasSaveAsReset`) — any NEW failure is investigated for causation before completion.
- [ ] **Step 5 — checkpoint**, leave unstaged.

---

## Self-Review

**Spec coverage:** §1 model→Task 1,7; §2 picker→Task 2,5,7; §3 button pass→Task 6; §4 wiring/cleanup→Task 7; §5 DESIGN/i18n/icon→Task 3,4,6; §6 testing→Tasks 1,2,5,6,7,8. ✓
**Placeholder scan:** month/weekday key bodies are enumerated by pattern (KeyMonth2..12, KeyWeekdayMon..Sat) — the implementer fills the obvious sequential values; no logic placeholders. ✓
**Type consistency:** `logsTimeSel`, `matchAtAbs`/`matchRangeAbs`, `logsPickTarget`(pickAt/From/To), `logsDateTimePicker`, `logsNowFn`, `gridDays` consistent across tasks. `filterRows` signature changes exactly once (Task 7) with all call sites updated same task.
**Ordering:** additive Tasks 1–6 keep the build green; Task 7 is the single breaking integration (old parsers + time-boxes removed together with the new call sites). ✓
