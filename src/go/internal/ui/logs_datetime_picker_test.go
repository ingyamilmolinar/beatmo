//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// setLogsNow pins logsNowFn for a deterministic calendar in tests.
func setLogsNow(t *testing.T, at time.Time) func() {
	t.Helper()
	prev := logsNowFn
	logsNowFn = func() time.Time { return at }
	return func() { logsNowFn = prev }
}

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

func TestPickerSetCommitsTruncatedMinute(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local))
	defer restore()
	var got struct {
		target logsPickTarget
		sel    logsTimeSel
	}
	p := newLogsDateTimePicker(func(tg logsPickTarget, s logsTimeSel) { got.target, got.sel = tg, s })
	p.openAt(pickFrom, logsTimeSel{}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600))
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
	got := logsTimeSel{set: true}
	p := newLogsDateTimePicker(func(_ logsPickTarget, s logsTimeSel) { got = s })
	p.openAt(pickAt, logsTimeSel{set: true, t: logsNowFn()}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600))
	p.commitClear()
	if got.set {
		t.Fatal("Clear must commit an unset selection")
	}
	if p.IsOpen() {
		t.Fatal("Clear should close the picker")
	}
}

func TestPickerHourMinuteWrap(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 23, 59, 0, 0, time.Local))
	defer restore()
	p := newLogsDateTimePicker(func(logsPickTarget, logsTimeSel) {})
	p.openAt(pickAt, logsTimeSel{}, image.Rectangle{}, image.Rect(0, 0, 800, 600))
	p.hourWheel.StepValue(+1) // 23 -> 0
	if p.sel.Hour() != 0 {
		t.Fatalf("hour wheel wrap: %d want 0", p.sel.Hour())
	}
	p.hourWheel.StepValue(-1) // 0 -> 23
	if p.sel.Hour() != 23 {
		t.Fatalf("hour wheel wrap back: %d want 23", p.sel.Hour())
	}
	p.minuteWheel.StepValue(+1) // 59 -> 0
	if p.sel.Minute() != 0 {
		t.Fatalf("minute wheel wrap: %d want 0", p.sel.Minute())
	}
}

// The HH and MM wheels scroll through the picker's own input path (the logs
// panel hands it pointer + wheel events), and Set commits what they show.
func TestPickerTimeWheelsDriveTheSelection(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 30, 0, 0, time.Local))
	defer restore()
	var got logsTimeSel
	p := newLogsDateTimePicker(func(_ logsPickTarget, s logsTimeSel) { got = s })
	p.openAt(pickAt, logsTimeSel{set: true, t: logsNowFn()}, image.Rect(10, 10, 60, 30), image.Rect(0, 0, 800, 600))
	p.layout()
	hx, hy := center(p.hourWheel.valRect)
	for i := 0; i < 2*wheelStepEventsPerNotch; i++ {
		p.HandleWheel(hx, hy, 1) // scroll DOWN = value up
	}
	if p.sel.Hour() != 12 {
		t.Fatalf("two notches on the hour wheel: %d want 12", p.sel.Hour())
	}
	mx, my := center(p.minuteWheel.valRect)
	for i := 0; i < wheelStepEventsPerNotch; i++ {
		p.HandleWheel(mx, my, -1)
	}
	if p.sel.Minute() != 29 {
		t.Fatalf("one notch up on the minute wheel: %d want 29", p.sel.Minute())
	}
	p.commitSet()
	if !got.set || got.t.Hour() != 12 || got.t.Minute() != 29 {
		t.Fatalf("Set committed %+v, want 12:29", got)
	}
}

func TestPickerMonthNav(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 0, 0, 0, time.Local))
	defer restore()
	p := newLogsDateTimePicker(func(logsPickTarget, logsTimeSel) {})
	p.openAt(pickAt, logsTimeSel{}, image.Rectangle{}, image.Rect(0, 0, 800, 600))
	p.prevMonth()
	if p.viewMonth != time.June || p.viewYear != 2026 {
		t.Fatalf("prevMonth: %v %d want June 2026", p.viewMonth, p.viewYear)
	}
	p.nextMonth()
	p.nextMonth() // July -> August
	if p.viewMonth != time.August {
		t.Fatalf("nextMonth: %v want August", p.viewMonth)
	}
	// Year rollover.
	p.viewMonth, p.viewYear = time.December, 2026
	p.nextMonth()
	if p.viewMonth != time.January || p.viewYear != 2027 {
		t.Fatalf("Dec->Jan rollover: %v %d want January 2027", p.viewMonth, p.viewYear)
	}
}

func TestPickerDrawSmoke(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 14, 30, 0, 0, time.Local))
	defer restore()
	p := newLogsDateTimePicker(func(logsPickTarget, logsTimeSel) {})
	p.openAt(pickAt, logsTimeSel{}, image.Rect(100, 40, 160, 60), image.Rect(0, 0, 900, 700))
	dst := ebiten.NewImage(900, 700)
	p.Draw(dst) // must not panic; populates dayRects
	if p.rect.Empty() {
		t.Fatal("picker rect not laid out after Draw")
	}
	// A press on a day cell selects that day.
	dr := p.dayRects[15]
	p.HandleInput(dr.Min.X+dr.Dx()/2, dr.Min.Y+dr.Dy()/2, true)
	want := gridDays(2026, time.July)[15]
	if p.sel.Day() != want.Day() || p.sel.Month() != want.Month() {
		t.Fatalf("day-cell click selected %v, want day %d", p.sel, want.Day())
	}
}

func TestIconCalendarDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconCalendar, image.Rect(0, 0, 24, 24), color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("drawIconByID false for IconCalendar")
	}
}

// A finger DRAG on a time wheel goes through the tree's hit handler: the press
// must capture (or the tree never delivers OnDrag), and the drag moves the
// value one detent per notch like every other wheel.
func TestPickerTimeWheelDragThroughTheHitHandler(t *testing.T) {
	restore := setLogsNow(t, time.Date(2026, 7, 20, 10, 30, 0, 0, time.Local))
	defer restore()
	p := newLogsDateTimePicker(func(logsPickTarget, logsTimeSel) {})
	p.openAt(pickAt, logsTimeSel{set: true, t: logsNowFn()}, image.Rect(10, 10, 60, 30), image.Rect(0, 0, 800, 600))
	p.layout()
	h := &compHitHandler{comp: p}
	x := (p.hourWheel.valRect.Min.X + p.hourWheel.valRect.Max.X) / 2
	y := p.hourWheel.valRect.Min.Y + 4
	if res := h.OnPress(x, y); res != InputCaptured {
		t.Fatalf("a press on the hour wheel must capture the gesture, got %v", res)
	}
	gap := p.hourWheel.tickGap()
	h.OnDrag(x, y+gap)   // finger DOWN one notch = value UP
	h.OnDrag(x, y+2*gap) // two notches
	h.OnRelease(x, y+2*gap)
	if p.sel.Hour() != 12 {
		t.Fatalf("dragging the hour wheel down two notches: %d want 12", p.sel.Hour())
	}
	if p.Capturing() {
		t.Fatal("release ends the capture")
	}
}
