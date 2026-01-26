package ui

import (
	"image"
)

// buildEQChannelMenu constructs the dropdown menu buttons for EQ channel selection.
// Supports scrolling when there are more items than fit in the visible area.
func (dv *DrumView) buildEQChannelMenu() {
	dv.eqChannelBtns = dv.eqChannelBtns[:0]

	if dv.eqChannelBtn == nil {
		return
	}
	base := dv.eqChannelBtn.Rect()
	btnH := dv.rowHeight()
	if btnH < 1 {
		btnH = 24
	}

	// Total items: 1 Master + len(Rows)
	total := 1 + len(dv.Rows)
	dv.eqChannelScroll.Total = total

	// Calculate available vertical space from button to screen bottom
	spaceDown := dv.Bounds.Max.Y - base.Max.Y
	maxVisDown := spaceDown / btnH
	if maxVisDown < 1 {
		maxVisDown = 1
	}

	// Constrain visible by: max constant, available space, AND total items
	visible := total
	if visible > eqChannelMenuMaxVisibleRows {
		visible = eqChannelMenuMaxVisibleRows
	}
	if visible > maxVisDown {
		visible = maxVisDown
	}
	if visible < 1 {
		visible = 1
	}
	dv.eqChannelScroll.Visible = visible
	dv.eqChannelScroll.Clamp()

	// Set up the view rectangle
	menuH := visible * btnH
	dv.eqChannelScroll.View = image.Rect(base.Min.X, base.Max.Y, base.Max.X, base.Max.Y+menuH)

	// Determine button width (narrower when scrollbar is visible)
	buttonMaxX := base.Max.X
	if dv.eqChannelScroll.HasScroll() {
		buttonMaxX -= eqChannelMenuScrollBarWidth
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}

	first := dv.eqChannelScroll.First
	for i := 0; i < visible && first+i < total; i++ {
		idx := first + i
		var label string
		var onClick func()

		if idx == 0 {
			// Master option
			label = "Master"
			onClick = func() {
				dv.setEQActiveChannel("main")
				dv.eqChannelOpen = false
				dv.eqChannelScroll.EndDrag()
				dv.eqChannelScroll.First = 0
			}
		} else {
			// Instrument row (idx-1 because idx=0 is Master)
			rowIdx := idx - 1
			if rowIdx >= len(dv.Rows) {
				continue
			}
			row := dv.Rows[rowIdx]
			label = row.Name
			if len(label) > 12 {
				label = label[:12] + "…"
			}
			instID := row.Instrument
			onClick = func() {
				dv.setEQActiveChannel(instID)
				dv.eqChannelOpen = false
				dv.eqChannelScroll.EndDrag()
				dv.eqChannelScroll.First = 0
			}
		}

		r := image.Rect(base.Min.X, base.Max.Y+i*btnH, buttonMaxX, base.Max.Y+(i+1)*btnH)
		btn := NewButton(label, DropdownStyle, onClick)
		btn.ConsumeOnPress = true
		btn.SetRect(insetRect(r, buttonPad))
		dv.eqChannelBtns = append(dv.eqChannelBtns, btn)
	}
}

// eqChannelMenuRect returns the bounding rectangle of the EQ channel dropdown menu viewport.
func (dv *DrumView) eqChannelMenuRect() image.Rectangle {
	if dv.eqChannelBtn == nil {
		return image.Rectangle{}
	}
	if !dv.eqChannelScroll.View.Empty() {
		return dv.eqChannelScroll.View
	}
	// Fallback for when scroll hasn't been set up yet
	base := dv.eqChannelBtn.Rect()
	menuH := len(dv.eqChannelBtns) * dv.rowHeight()
	return image.Rect(base.Min.X, base.Max.Y, base.Max.X, base.Max.Y+menuH)
}

// EQChannelMenuOverlay implements overlay handling for the EQ channel dropdown.
type EQChannelMenuOverlay struct {
	dv *DrumView
}

func (o *EQChannelMenuOverlay) ID() string { return "eq-channel-menu" }

func (o *EQChannelMenuOverlay) IsOpen() bool {
	return o.dv.eqChannelOpen
}

func (o *EQChannelMenuOverlay) ZIndex() int { return 210 }

func (o *EQChannelMenuOverlay) InputBounds() image.Rectangle {
	if !o.dv.eqChannelOpen || o.dv.eqChannelBtn == nil {
		return image.Rectangle{}
	}
	// Include both the trigger button and the dropdown menu.
	// This prevents the overlay from closing when the user clicks
	// on the button (which is outside the menu but still "valid").
	btn := o.dv.eqChannelBtn.Rect()
	menu := o.dv.eqChannelMenuRect()
	// Include scrollbar area in bounds when present
	if o.dv.eqChannelScroll.HasScroll() {
		menu.Max.X += eqChannelMenuScrollBarWidth
	}
	return image.Rect(
		min(btn.Min.X, menu.Min.X),
		min(btn.Min.Y, menu.Min.Y),
		max(btn.Max.X, menu.Max.X),
		max(btn.Max.Y, menu.Max.Y),
	)
}

func (o *EQChannelMenuOverlay) Capturing() bool {
	return o.dv.eqChannelScroll.dragging
}

func (o *EQChannelMenuOverlay) Close() {
	o.dv.eqChannelOpen = false
	o.dv.eqChannelScroll.EndDrag()
	o.dv.eqChannelScroll.First = 0
	SuppressClicksUntilMouseUp()
}

func (o *EQChannelMenuOverlay) HandleInput(x, y int, pressed bool) InputResult {
	// Point is within menu bounds - consume to prevent click-through
	// Actual button handling happens in drumview_update.go (matches subdiv pattern)
	if pressed && image.Pt(x, y).In(o.InputBounds()) {
		return InputConsumed
	}
	return InputIgnored
}

func (o *EQChannelMenuOverlay) HandleWheel(x, y, steps int) InputResult {
	if !o.dv.eqChannelOpen {
		return InputIgnored
	}

	if o.dv.eqChannelScroll.ScrollBy(-steps) {
		o.dv.buildEQChannelMenu()
	}
	return InputConsumed // Always consume when menu is open and cursor is over it
}
