package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logsDatePickerID identifies the stacked date/time-picker portal.
const logsDatePickerID = "logs-datetime"

// setTimeMode switches the timestamp filter between At and Range and re-applies.
func (c *LogsComponent) setTimeMode(m timestampMode) { c.tsMode = m; c.reapply() }

// The picker's onCommit.
func (c *LogsComponent) setTimeSel(target logsPickTarget, sel logsTimeSel) {
	switch target {
	case pickFrom:
		c.fromSel = sel
	case pickTo:
		c.toSel = sel
	default:
		c.atSel = sel
	}
	c.reapply()
}

// timeFieldLabel formats a field's value ("Any time" when unset).
func timeFieldLabel(sel logsTimeSel) string {
	if !sel.set {
		return i18n.T(i18n.KeyLogsAnyTime)
	}
	return sel.t.Format("2006-01-02 15:04")
}

// ensureTimeButtons lazily builds the At|Range toggle + the three field buttons.
func (c *LogsComponent) ensureTimeButtons() {
	if c.atModeBtn != nil {
		return
	}
	c.atModeBtn = NewButtonKey(i18n.KeyLogsTimeAt, ComponentButtonSecondary, func() { c.setTimeMode(tsModeAt) })
	c.rangeModeBtn = NewButtonKey(i18n.KeyLogsTimeRange, ComponentButtonSecondary, func() { c.setTimeMode(tsModeRange) })
	c.atFieldBtn = NewSpecButton("", ComponentButtonSecondary, func() { c.openPicker(pickAt) })
	c.fromFieldBtn = NewSpecButton("", ComponentButtonSecondary, func() { c.openPicker(pickFrom) })
	c.toFieldBtn = NewSpecButton("", ComponentButtonSecondary, func() { c.openPicker(pickTo) })
}

// ensurePicker lazily constructs the date/time picker bound to setTimeSel.
func (c *LogsComponent) ensurePicker() *logsDateTimePicker {
	if c.picker == nil {
		c.picker = newLogsDateTimePicker(c.setTimeSel)
	}
	return c.picker
}

// fieldSel / fieldButton map a target to its current selection / opener button.
func (c *LogsComponent) fieldSel(target logsPickTarget) logsTimeSel {
	switch target {
	case pickFrom:
		return c.fromSel
	case pickTo:
		return c.toSel
	default:
		return c.atSel
	}
}

func (c *LogsComponent) fieldButton(target logsPickTarget) *Button {
	switch target {
	case pickFrom:
		return c.fromFieldBtn
	case pickTo:
		return c.toFieldBtn
	default:
		return c.atFieldBtn
	}
}

func (c *LogsComponent) openPicker(target logsPickTarget) {
	if c.dv == nil || c.dv.tree == nil {
		return
	}
	p := c.ensurePicker()
	anchor := c.fieldButton(target).Rect()
	p.openAt(target, c.fieldSel(target), anchor, c.panelRect())
	c.dv.portal().Open(PortalEntry{
		ID:      logsDatePickerID,
		Overlay: &compPortalOverlay{comp: p, tag: logsDatePickerID, updateFn: p.Update},
		Modal:   true,
		Scrim:   true,
		Anchor:  anchor,
		OnClose: func() { p.Close() },
	})
}

// IsDatePickerOpen reports whether the stacked date/time picker portal is open.
func (c *LogsComponent) IsDatePickerOpen() bool {
	return c.dv != nil && c.dv.tree != nil && c.dv.portal().Has(logsDatePickerID)
}

func (c *LogsComponent) drawTimeControls(dst *ebiten.Image, r image.Rectangle, y, h int) {
	c.ensureTimeButtons()
	x := r.Min.X + r.Dx()/2 + SpaceSM
	maxX := r.Max.X - SpaceMD

	// drawPillTabAt renders btn.Text, so the localized label is set each frame.
	atLabel := i18n.T(i18n.KeyLogsTimeAt)
	rangeLabel := i18n.T(i18n.KeyLogsTimeRange)
	c.atModeBtn.Text = atLabel
	c.rangeModeBtn.Text = rangeLabel
	// Size to the caption role drawPillTabAt actually renders (not body TextWidth).
	aw := StyledTextWidth(atLabel, RoleCaption) + 2*SpaceSM
	rw := StyledTextWidth(rangeLabel, RoleCaption) + 2*SpaceSM
	c.atModeBtn.SetRect(image.Rect(x, y, x+aw, y+h))
	drawPillTabAt(dst, c.atModeBtn, c.tsMode == tsModeAt)
	x = c.atModeBtn.Rect().Max.X + SpaceXS
	c.rangeModeBtn.SetRect(image.Rect(x, y, x+rw, y+h))
	drawPillTabAt(dst, c.rangeModeBtn, c.tsMode == tsModeRange)
	x = c.rangeModeBtn.Rect().Max.X + SpaceSM

	if c.tsMode == tsModeRange {
		x = c.drawTimeField(dst, c.fromFieldBtn, timeFieldLabel(c.fromSel), x, y, h, maxX)
		c.drawTimeField(dst, c.toFieldBtn, timeFieldLabel(c.toSel), x, y, h, maxX)
		c.atFieldBtn.SetRect(image.Rectangle{})
	} else {
		c.drawTimeField(dst, c.atFieldBtn, timeFieldLabel(c.atSel), x, y, h, maxX)
		c.fromFieldBtn.SetRect(image.Rectangle{})
		c.toFieldBtn.SetRect(image.Rectangle{})
	}
}

// An off-strip button gets an empty rect so it cannot be hit.
func (c *LogsComponent) drawTimeField(dst *ebiten.Image, btn *Button, label string, x, y, h, maxX int) int {
	w := TextWidth(label) + 2*SpaceSM
	if x+w > maxX {
		btn.SetRect(image.Rectangle{})
		return x
	}
	btn.Text = label
	btn.SetRect(image.Rect(x, y, x+w, y+h))
	btn.Draw(dst)
	return x + w + SpaceXS
}

// Releases must reach the buttons too, or each fires only once.
func (c *LogsComponent) handleTimeInput(x, y int, pressed bool) bool {
	consumed := false
	for _, b := range []*Button{c.atModeBtn, c.rangeModeBtn, c.atFieldBtn, c.fromFieldBtn, c.toFieldBtn} {
		if b != nil && b.HandleInputResult(x, y, pressed) != InputIgnored {
			consumed = true
		}
	}
	return consumed
}
