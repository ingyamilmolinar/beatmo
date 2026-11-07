package ui

import "image/color"

// The picker owns the keyboard while open; blocking alone is not enough, so the arrows move a
// MenuKeyNav cursor through the grid (Up/Down a row, Left/Right a swatch).

// Otherwise the arrows fall through to the grid and pan the camera under the open picker.
func (c *ColorWheelComponent) ClaimsKeyboard() bool {
	return c != nil && c.state.open
}

// Row-major, exactly as rebuildWheel lays out the cells, so the flat index is the swatch index.
type colorWheelNavHost struct{ c *ColorWheelComponent }

func (h colorWheelNavHost) MenuGridCols() int { return colorSwatchCols }

func (h colorWheelNavHost) MenuItemCount() int {
	n := len(h.c.swatches())
	if max := colorSwatchCols * colorSwatchRows; n > max {
		n = max
	}
	return n
}

// Every swatch is a choice, so all items are selectable.
func (h colorWheelNavHost) MenuItemSelectable(i int) bool {
	return i >= 0 && i < h.MenuItemCount()
}

func (h colorWheelNavHost) MenuActivate(i int) {
	if !h.MenuItemSelectable(i) {
		return
	}
	h.c.applySwatch(h.c.swatches()[i].RGBA)
}

// The whole palette is always on screen.
func (h colorWheelNavHost) MenuScrollBehavior() *ScrollBehavior { return nil }

// KeyNav is the picker's keyboard cursor, created on first use.
func (c *ColorWheelComponent) KeyNav() *MenuKeyNav {
	if c.keyNav == nil {
		c.keyNav = NewMenuKeyNav()
	}
	return c.keyNav
}

// Driven from the portal's per-frame update, so the arrows work wherever the pointer is.
func (c *ColorWheelComponent) UpdateKeyNav() {
	if !c.state.open {
		if c.keyNav != nil {
			c.keyNav.Reset()
		}
		return
	}
	c.KeyNav().Update(colorWheelNavHost{c: c})
}

// So the first arrow moves from the current colour and an immediate Enter changes nothing.
func (c *ColorWheelComponent) syncCursorToApplied() {
	cur := c.currentColor()
	if cur == nil {
		return
	}
	nav := c.KeyNav()
	host := colorWheelNavHost{c: c}
	nav.BindHost(host)
	for i, s := range c.swatches() {
		if i >= host.MenuItemCount() {
			break
		}
		if colorsEqualRGB(cur, s.RGBA) {
			nav.SetSelected(i)
			return
		}
	}
}

// The one mutation for click and Enter. A new colour applies and keeps the picker open; the
// applied one closes it.
func (c *ColorWheelComponent) applySwatch(col color.RGBA) {
	if colorsEqualRGB(c.currentColor(), col) {
		c.Close()
		return
	}
	c.state.applied = col
	if c.props.OnColorPick != nil {
		c.props.OnColorPick(col)
	}
	// Keep the cursor on what was just applied so Enter confirms it.
	c.syncCursorToApplied()
}

func (c *ColorWheelComponent) KeyCursorSwatch() int {
	if !c.state.open || c.keyNav == nil {
		return -1
	}
	sel := c.keyNav.Selected()
	if sel < 0 || sel >= (colorWheelNavHost{c: c}).MenuItemCount() {
		return -1
	}
	return sel
}
