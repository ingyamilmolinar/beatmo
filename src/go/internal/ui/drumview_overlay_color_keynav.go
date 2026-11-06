package ui

import "image/color"

// Keyboard ownership and navigation for the colour picker.
//
// The picker OWNS the keyboard while open (KeyboardClaimant, forwarded by
// compPortalOverlay), so handleGlobalShortcuts yields every gated grid
// shortcut. Blocking alone would not be enough — a surface that swallows the
// arrows must also DO something with them, so the cursor moves through the
// swatch grid with MenuKeyNav, the same component the note matrix and the list
// menus use, in its grid mode (Up/Down a whole row, Left/Right one swatch).

// ClaimsKeyboard reports whether the picker owns keyboard input.
//
// Without this the arrows fell through to the grid's arrow-pan: one press
// panned the camera under an open picker and did nothing inside it.
func (c *ColorWheelComponent) ClaimsKeyboard() bool {
	return c != nil && c.state.open
}

// colorWheelNavHost adapts the swatch grid to MenuKeyNavHost. Items are the
// palette in row-major order, which is exactly how rebuildWheel lays the cells
// out, so the flat index IS the swatch index.
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

// The grid never scrolls — the whole palette is always on screen — so there is
// nothing to reveal.
func (h colorWheelNavHost) MenuScrollBehavior() *ScrollBehavior { return nil }

// KeyNav is the picker's keyboard cursor, created on first use.
func (c *ColorWheelComponent) KeyNav() *MenuKeyNav {
	if c.keyNav == nil {
		c.keyNav = NewMenuKeyNav()
	}
	return c.keyNav
}

// UpdateKeyNav pumps the keyboard cursor for one frame. Driven from the
// portal's per-frame update so the arrows work wherever the pointer is.
func (c *ColorWheelComponent) UpdateKeyNav() {
	if !c.state.open {
		if c.keyNav != nil {
			c.keyNav.Reset()
		}
		return
	}
	c.KeyNav().Update(colorWheelNavHost{c: c})
}

// syncCursorToApplied places the cursor on the swatch currently applied, so the
// first arrow press moves relative to the current colour and Enter straight
// after opening confirms rather than changing anything.
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

// applySwatch is the ONE mutation both a swatch click and Enter go through, so
// pointer and keyboard cannot diverge.
//
// SELECTING APPLIES, RE-SELECTING CONFIRMS — the same rule the note matrix
// uses: a different colour is applied and the picker stays open; the colour
// already applied closes it.
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

// KeyCursorSwatch returns the palette index under the keyboard cursor, or -1.
// Draw uses it to render the focus ring.
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
