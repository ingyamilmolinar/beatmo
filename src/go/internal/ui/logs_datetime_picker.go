package ui

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logsPickTarget names which timestamp field the picker edits.
type logsPickTarget int

const (
	pickAt logsPickTarget = iota
	pickFrom
	pickTo
)

// monthKey / weekdayKey map to the localized calendar labels.
func monthKey(m time.Month) i18n.Key {
	keys := [...]i18n.Key{
		i18n.KeyMonth1, i18n.KeyMonth2, i18n.KeyMonth3, i18n.KeyMonth4, i18n.KeyMonth5, i18n.KeyMonth6,
		i18n.KeyMonth7, i18n.KeyMonth8, i18n.KeyMonth9, i18n.KeyMonth10, i18n.KeyMonth11, i18n.KeyMonth12,
	}
	i := int(m) - 1
	if i < 0 || i > 11 {
		return i18n.KeyMonth1
	}
	return keys[i]
}

func weekdayKey(wd time.Weekday) i18n.Key {
	keys := [...]i18n.Key{
		i18n.KeyWeekdaySun, i18n.KeyWeekdayMon, i18n.KeyWeekdayTue, i18n.KeyWeekdayWed,
		i18n.KeyWeekdayThu, i18n.KeyWeekdayFri, i18n.KeyWeekdaySat,
	}
	return keys[int(wd)%7]
}

// The calendar days are flat tappable cells; every other control is a real keycap Button.
type logsDateTimePicker struct {
	overlayBase
	onCommit func(logsPickTarget, logsTimeSel)

	open      bool
	target    logsPickTarget
	viewYear  int
	viewMonth time.Month
	sel       time.Time // working selection (day + hour + minute)

	anchor      image.Rectangle // field rect to drop the panel below
	clampBounds image.Rectangle // parent bounds for clamping
	rect        image.Rectangle // computed panel rect (set in layout)
	dayRects    [42]image.Rectangle

	monthPrev, monthNext        *Button
	setBtn, clearBtn, cancelBtn *Button
	// Wrapping clock fields.
	hourWheel, minuteWheel ValueWheel
	hourModel, minuteModel *NumberModel
	ctrls                  []*Button // cached control-button list (no per-call alloc)
}

// Clear commits an unset selection.
func newLogsDateTimePicker(onCommit func(logsPickTarget, logsTimeSel)) *logsDateTimePicker {
	p := &logsDateTimePicker{overlayBase: newOverlayBase(), onCommit: onCommit}
	p.monthPrev = NewSpecButton("", ComponentButtonSecondary, p.prevMonth)
	p.monthPrev.Icon = string(IconChevronLeft)
	p.monthNext = NewSpecButton("", ComponentButtonSecondary, p.nextMonth)
	p.monthNext.Icon = string(IconChevronRight)
	p.setBtn = NewButtonKey(i18n.KeyLogsSet, ComponentButtonSecondary, p.commitSet)
	p.clearBtn = NewButtonKey(i18n.KeyLogsClear, ComponentButtonSecondary, p.commitClear)
	p.cancelBtn = NewButtonKey(i18n.KeyCancel, ComponentButtonSecondary, p.Close)
	p.ctrls = []*Button{
		p.monthPrev, p.monthNext,
		p.setBtn, p.clearBtn, p.cancelBtn,
	}
	return p
}

// openAt opens the picker for target, seeded from seed (or logsNowFn when unset).
func (p *logsDateTimePicker) openAt(target logsPickTarget, seed logsTimeSel, anchor, bounds image.Rectangle) {
	p.target = target
	base := logsNowFn()
	if seed.set {
		base = seed.t
	}
	p.sel = truncateToMinute(base)
	p.viewYear, p.viewMonth = p.sel.Year(), p.sel.Month()
	p.anchor, p.clampBounds = anchor, bounds
	p.open = true
	p.bindTimeWheels()
}

// clockFormat renders a clock field as two digits.
func clockFormat(v, _ float64) string { return fmt.Sprintf("%02d", int(v)) }

// bindTimeWheels seeds the HH / MM wheels from the working selection.
func (p *logsDateTimePicker) bindTimeWheels() {
	p.hourModel = &NumberModel{Min: 0, Max: 23, Value: float64(p.sel.Hour()), Wrap: true, Format: clockFormat}
	p.minuteModel = &NumberModel{Min: 0, Max: 59, Value: float64(p.sel.Minute()), Wrap: true, Format: clockFormat}
	p.hourWheel.Bind(WheelBinding{Model: p.hourModel, OnChange: func() { p.setHour(int(p.hourModel.Value)) }})
	p.minuteWheel.Bind(WheelBinding{Model: p.minuteModel, OnChange: func() { p.setMinute(int(p.minuteModel.Value)) }})
}

func (p *logsDateTimePicker) IsOpen() bool { return p.open }
func (p *logsDateTimePicker) Close()       { p.open = false }

// --- selection mutation ---

func (p *logsDateTimePicker) selectDay(d time.Time) {
	p.sel = time.Date(d.Year(), d.Month(), d.Day(), p.sel.Hour(), p.sel.Minute(), 0, 0, time.Local)
	p.viewYear, p.viewMonth = d.Year(), d.Month()
}

func (p *logsDateTimePicker) setHour(h int) {
	p.sel = time.Date(p.sel.Year(), p.sel.Month(), p.sel.Day(), (h%24+24)%24, p.sel.Minute(), 0, 0, time.Local)
}

func (p *logsDateTimePicker) setMinute(m int) {
	p.sel = time.Date(p.sel.Year(), p.sel.Month(), p.sel.Day(), p.sel.Hour(), (m%60+60)%60, 0, 0, time.Local)
}

func (p *logsDateTimePicker) prevMonth() {
	t := time.Date(p.viewYear, p.viewMonth, 1, 0, 0, 0, 0, time.Local).AddDate(0, -1, 0)
	p.viewYear, p.viewMonth = t.Year(), t.Month()
}

func (p *logsDateTimePicker) nextMonth() {
	t := time.Date(p.viewYear, p.viewMonth, 1, 0, 0, 0, 0, time.Local).AddDate(0, 1, 0)
	p.viewYear, p.viewMonth = t.Year(), t.Month()
}

func (p *logsDateTimePicker) commitSet() {
	if p.onCommit != nil {
		p.onCommit(p.target, logsTimeSel{set: true, t: truncateToMinute(p.sel)})
	}
	p.open = false
}

func (p *logsDateTimePicker) commitClear() {
	if p.onCommit != nil {
		p.onCommit(p.target, logsTimeSel{})
	}
	p.open = false
}

// --- geometry ---

const (
	logsPickCell = 30 // day-cell + control-button size
	logsPickPad  = SpaceSM
)

func (p *logsDateTimePicker) layout() {
	rowH := TextHeight() + SpaceSM
	gridW := 7 * logsPickCell
	w := gridW + 2*logsPickPad
	// SpaceSM keeps the wheels clear of the last calendar row.
	timeGap := SpaceSM
	timeH := logsTimeWheelHeight()
	h := logsPickPad + rowH + logsPickCell + rowH + 6*logsPickCell + timeGap + timeH + rowH + logsPickPad*2
	x := p.anchor.Min.X
	y := p.anchor.Max.Y + SpaceXS
	r := image.Rect(x, y, x+w, y+h)
	// Clamp into bounds.
	if r.Max.X > p.clampBounds.Max.X-SpaceSM {
		r = r.Sub(image.Pt(r.Max.X-(p.clampBounds.Max.X-SpaceSM), 0))
	}
	if r.Min.X < p.clampBounds.Min.X+SpaceSM {
		r = r.Add(image.Pt(p.clampBounds.Min.X+SpaceSM-r.Min.X, 0))
	}
	if r.Max.Y > p.clampBounds.Max.Y-SpaceSM {
		// Drop above the field instead.
		r = r.Sub(image.Pt(0, r.Min.Y-(p.anchor.Min.Y-SpaceXS-r.Dy())))
	}
	if r.Min.Y < p.clampBounds.Min.Y+SpaceSM {
		r = r.Add(image.Pt(0, p.clampBounds.Min.Y+SpaceSM-r.Min.Y))
	}
	p.rect = r

	gx := r.Min.X + logsPickPad
	// month nav row.
	mnY := r.Min.Y + logsPickPad + rowH
	p.monthPrev.SetRect(image.Rect(gx, mnY, gx+logsPickCell, mnY+logsPickCell))
	p.monthNext.SetRect(image.Rect(gx+gridW-logsPickCell, mnY, gx+gridW, mnY+logsPickCell))
	// day grid.
	gridTop := mnY + logsPickCell + rowH
	for i := 0; i < 42; i++ {
		col := i % 7
		row := i / 7
		cx := gx + col*logsPickCell
		cy := gridTop + row*logsPickCell
		p.dayRects[i] = image.Rect(cx, cy, cx+logsPickCell, cy+logsPickCell)
	}
	// time wheels: [ HH ] : [ MM ], colon gap in the middle.
	tY := gridTop + 6*logsPickCell + timeGap
	colonW := logsPickCell / 2
	wheelW := (gridW - colonW) / 2
	p.hourWheel.Layout(image.Rect(gx, tY, gx+wheelW, tY+timeH))
	p.minuteWheel.Layout(image.Rect(gx+wheelW+colonW, tY, gx+gridW, tY+timeH))
	// footer.
	fY := tY + timeH + SpaceXS
	fW := (gridW - 2*SpaceXS) / 3
	p.clearBtn.SetRect(image.Rect(gx, fY, gx+fW, fY+logsPickCell))
	p.cancelBtn.SetRect(image.Rect(gx+fW+SpaceXS, fY, gx+2*fW+SpaceXS, fY+logsPickCell))
	p.setBtn.SetRect(image.Rect(gx+2*fW+2*SpaceXS, fY, gx+3*fW+2*SpaceXS, fY+logsPickCell))
}

func (p *logsDateTimePicker) InputBounds() image.Rectangle { return p.rect }
func (p *logsDateTimePicker) Capturing() bool {
	return p.hourWheel.IsDraggingAny() || p.minuteWheel.IsDraggingAny()
}
func (p *logsDateTimePicker) Update() {}

const logsTimeWheelRows = 3

func logsTimeWheelHeight() int {
	gap := Profile().DensityValues().MobileWheelTickGap
	if gap <= 0 {
		gap = 34
	}
	return logsTimeWheelRows * gap
}

// timeWheelAt returns the HH / MM wheel under (x, y), or nil.
func (p *logsDateTimePicker) timeWheelAt(x, y int) *ValueWheel {
	pt := image.Pt(x, y)
	if pt.In(p.hourWheel.valRect) {
		return &p.hourWheel
	}
	if pt.In(p.minuteWheel.valRect) {
		return &p.minuteWheel
	}
	return nil
}

// HandleWheel scrolls the HH / MM wheel under the pointer; swallows the rest.
func (p *logsDateTimePicker) HandleWheel(x, y, steps int) InputResult {
	if w := p.timeWheelAt(x, y); w != nil && p.hourModel != nil {
		w.HandleWheel(x, y, steps)
	}
	return InputConsumed
}

func (p *logsDateTimePicker) HandleInput(x, y int, pressed bool) InputResult {
	if p.hourModel != nil {
		for _, w := range []*ValueWheel{&p.hourWheel, &p.minuteWheel} {
			if w.IsDraggingAny() {
				w.HandleInput(x, y, pressed)
				return InputConsumed
			}
		}
		if pressed {
			if w := p.timeWheelAt(x, y); w != nil {
				w.HandleInput(x, y, true)
				return InputCaptured // the tree keeps delivering the drag to us
			}
		}
	}
	for _, b := range p.controlButtons() {
		if b.HandleInputResult(x, y, pressed) != InputIgnored {
			return InputConsumed
		}
	}
	if pressed {
		days := gridDays(p.viewYear, p.viewMonth)
		for i, dr := range p.dayRects {
			if image.Pt(x, y).In(dr) {
				p.selectDay(days[i])
				return InputConsumed
			}
		}
	}
	return InputConsumed
}

func (p *logsDateTimePicker) controlButtons() []*Button { return p.ctrls }

// HoverAt forwards hover to the control buttons and the time wheels.
func (p *logsDateTimePicker) HoverAt(mx, my int) {
	for _, b := range p.controlButtons() {
		b.UpdateHover(mx, my)
	}
	if p.hourModel != nil {
		p.hourWheel.HoverAt(mx, my)
		p.minuteWheel.HoverAt(mx, my)
	}
}

func (p *logsDateTimePicker) Draw(dst *ebiten.Image) {
	p.layout()
	r := p.rect
	drawRoundedRect(dst, r, colSurface2, RadiusContainer, true)
	drawRoundedRect(dst, r, colBorderSubtle, RadiusContainer, false)

	// Title.
	DrawTextColorAt(dst, i18n.T(i18n.KeyLogsPickTitle), r.Min.X+logsPickPad, r.Min.Y+SpaceXS, colTextPrimary)

	// Month nav + "Month Year".
	p.monthPrev.Draw(dst)
	p.monthNext.Draw(dst)
	label := fmt.Sprintf("%s %d", i18n.T(monthKey(p.viewMonth)), p.viewYear)
	mnY := p.monthPrev.Rect().Min.Y
	DrawTextColorAt(dst, label, r.Min.X+r.Dx()/2-TextWidth(label)/2, mnY+(logsPickCell-TextHeight())/2, colTextPrimary)

	// Weekday header.
	gx := r.Min.X + logsPickPad
	wdY := mnY + logsPickCell
	for i := 0; i < 7; i++ {
		lbl := i18n.T(weekdayKey(time.Weekday(i)))
		cx := gx + i*logsPickCell
		DrawTextColorAt(dst, lbl, cx+(logsPickCell-TextWidth(lbl))/2, wdY, colTextSecondary)
	}

	// Day cells (flat; selected = accent fill, today = accent ring, other-month = dim).
	days := gridDays(p.viewYear, p.viewMonth)
	today := truncateToMinute(logsNowFn())
	for i, d := range days {
		cell := p.dayRects[i]
		selected := d.Year() == p.sel.Year() && d.YearDay() == p.sel.YearDay()
		inMonth := d.Month() == p.viewMonth
		isToday := d.Year() == today.Year() && d.YearDay() == today.YearDay()
		var txtCol color.Color = colTextPrimary
		if !inMonth {
			txtCol = colTextSecondary
		}
		if selected {
			drawRoundedRect(dst, cell.Inset(2), colAccent, fitRadius(RadiusControl, cell.Inset(2)), true)
			txtCol = pillTextActive
		} else if isToday {
			drawRoundedRect(dst, cell.Inset(2), colAccent, fitRadius(RadiusControl, cell.Inset(2)), false)
		}
		num := fmt.Sprintf("%d", d.Day())
		DrawTextColorAt(dst, num, cell.Min.X+(logsPickCell-TextWidth(num))/2, cell.Min.Y+(logsPickCell-TextHeight())/2, txtCol)
	}

	// Time wheels: [ HH ] : [ MM ].
	if p.hourModel != nil {
		p.hourWheel.DrawBody(dst)
		p.minuteWheel.DrawBody(dst)
		hr, mr := p.hourWheel.valRect, p.minuteWheel.valRect
		colonX := hr.Max.X + (mr.Min.X-hr.Max.X-TextWidth(":"))/2
		DrawTextColorAt(dst, ":", colonX, hr.Min.Y+(hr.Dy()-TextHeight())/2, colTextPrimary)
	}

	// Footer buttons.
	p.clearBtn.Draw(dst)
	p.cancelBtn.Draw(dst)
	p.setBtn.Draw(dst)
}
