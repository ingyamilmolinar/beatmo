package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// openContextMenu builds a context menu for row actions.
// On mobile, renders as a bottom sheet anchored to the bottom of the drum pane.
// On desktop, renders as a positioned popup near the row label.
func (dv *DrumView) openContextMenu(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		return
	}
	// Close all other popups for mutual exclusivity.
	dv.CloseAllPopups()

	dv.contextMenuRow = rowIdx
	dv.contextMenuBtns = dv.contextMenuBtns[:0]
	dv.contextMenuIcons = dv.contextMenuIcons[:0]

	rowH := touchMinTargetPx
	dividerH := 0 // group containers replace divider lines; gap is SpaceSM only
	items := dv.contextMenuItems(rowIdx)

	// Count non-divider items and total content height for scroll calculations
	nItems := 0
	nDividers := 0
	for _, item := range items {
		if item.divider {
			nDividers++
		} else {
			nItems++
		}
	}

	if Profile().UseBottomSheet {
		// Bottom sheet: full-width, anchored to bottom of drum pane.
		pad := 12
		menuW := dv.Bounds.Dx() - pad*2
		if menuW < 160 {
			menuW = dv.Bounds.Dx()
			pad = 0
		}
		// Calculate height: count items + dividers + header.
		headerH := touchMinTargetPx
		contentH := headerH
		for _, item := range items {
			if item.divider {
				contentH += dividerH + SpaceSM
			} else {
				contentH += rowH
			}
		}
		menuH := contentH + pad
		if menuH > dv.Bounds.Dy() {
			menuH = dv.Bounds.Dy()
		}
		mx := dv.Bounds.Min.X + pad
		my := dv.Bounds.Max.Y - menuH
		dv.contextMenuRect = image.Rect(mx, my, mx+menuW, dv.Bounds.Max.Y)

		// Header rect is tracked for drawing but NOT added to contextMenuBtns
		// so that button indices remain stable for tests and deferred-tap logic.
		dv.contextMenuHeaderRect = image.Rect(mx+SpaceMD, my+SpaceMD, mx+menuW-touchMinTargetPx, my+SpaceMD+headerH-SpaceSM)

		// Layout items below header.
		curY := my + headerH
		for _, item := range items {
			if item.divider {
				curY += dividerH + SpaceSM
				continue
			}
			r := image.Rect(mx+SpaceMD, curY, mx+menuW-SpaceMD, curY+rowH)
			btn := NewButton(item.label, item.style, item.onClick)
			btn.SetRect(insetRect(r, buttonPad))
			if item.textColor != nil {
				btn.TextColor = item.textColor
			}
			dv.contextMenuBtns = append(dv.contextMenuBtns, btn)
			dv.contextMenuIcons = append(dv.contextMenuIcons, item.icon)
			curY += rowH
		}
	} else {
		// Desktop: positioned popup next to row label.
		var anchor image.Rectangle
		if rowIdx < len(dv.rowLabels()) {
			anchor = dv.rowLabels()[rowIdx].Rect()
		} else {
			anchor = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+100, dv.Bounds.Min.Y+dv.headerH+dv.rowHeight())
		}
		menuW := 160
		dividerSpace := nDividers * (dividerH + SpaceSM)
		fullH := nItems*rowH + dividerSpace
		menuH := fullH
		if maxH := dv.Bounds.Dy(); menuH > maxH {
			menuH = maxH
		}
		mx := anchor.Max.X + 4
		my := anchor.Min.Y
		if mx+menuW > dv.Bounds.Max.X {
			mx = anchor.Min.X - menuW - 4
		}
		if mx < dv.Bounds.Min.X {
			mx = dv.Bounds.Min.X
		}
		if my+menuH > dv.Bounds.Max.Y {
			my = dv.Bounds.Max.Y - menuH
		}
		if my < dv.Bounds.Min.Y {
			my = dv.Bounds.Min.Y
		}
		dv.contextMenuRect = image.Rect(mx, my, mx+menuW, my+menuH)

		curY := my
		for _, item := range items {
			if item.divider {
				curY += dividerH + SpaceSM
				continue
			}
			r := image.Rect(mx, curY, mx+menuW, curY+rowH)
			btn := NewButton(item.label, item.style, item.onClick)
			btn.SetRect(insetRect(r, buttonPad))
			if item.textColor != nil {
				btn.TextColor = item.textColor
			}
			dv.contextMenuBtns = append(dv.contextMenuBtns, btn)
			dv.contextMenuIcons = append(dv.contextMenuIcons, item.icon)
			curY += rowH
		}
	}

	// Initialize scroll behavior for the context menu.
	// On mobile, the scroll viewport excludes the header so the header stays
	// pinned at the top while items scroll underneath.
	dv.contextMenuScroll = NewScrollBehavior(DropdownScrollbarStyle, rowH)
	dv.contextMenuScroll.VS.Total = nItems
	if Profile().UseBottomSheet {
		headerH := touchMinTargetPx
		itemViewTop := dv.contextMenuRect.Min.Y + headerH
		dv.contextMenuScroll.VS.View = image.Rect(
			dv.contextMenuRect.Min.X, itemViewTop,
			dv.contextMenuRect.Max.X, dv.contextMenuRect.Max.Y)
		visibleItems := (dv.contextMenuRect.Max.Y - itemViewTop) / rowH
		if visibleItems > nItems {
			visibleItems = nItems
		}
		if visibleItems < 1 {
			visibleItems = 1
		}
		dv.contextMenuScroll.VS.Visible = visibleItems
	} else {
		dv.contextMenuScroll.VS.View = dv.contextMenuRect
		visibleItems := dv.contextMenuRect.Dy() / rowH
		if visibleItems > nItems {
			visibleItems = nItems
		}
		if visibleItems < 1 {
			visibleItems = 1
		}
		dv.contextMenuScroll.VS.Visible = visibleItems
	}

	// Close button at top-right.
	closeR := closeButtonRect(dv.contextMenuRect, buttonPad)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeContextMenuPortal() })
	closeB.Icon = "close"
	closeB.IconColor = colButtonBorder
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	dv.contextMenuBtns = append(dv.contextMenuBtns, closeB)
	dv.openContextMenuPortal()
}

// rebuildContextMenuButtons adjusts button positions based on current scroll offset.
// Called after scroll changes to reposition visible items.
func (dv *DrumView) rebuildContextMenuButtons() {
	if dv.contextMenuScroll == nil || !dv.IsContextMenuOpen() {
		return
	}
	rowH := touchMinTargetPx
	scrollPx := dv.contextMenuScroll.VS.First * rowH

	// Rebuild non-close buttons with scroll offset
	items := dv.contextMenuItems(dv.contextMenuRow)
	btnIdx := 0
	if Profile().UseBottomSheet {
		curY := dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx // skip header
		for _, item := range items {
			if item.divider {
				curY += SpaceSM // group gap (no divider line)
				continue
			}
			if btnIdx < len(dv.contextMenuBtns)-1 { // -1 for close button
				r := image.Rect(dv.contextMenuRect.Min.X+SpaceMD, curY, dv.contextMenuRect.Max.X-SpaceMD, curY+rowH)
				dv.contextMenuBtns[btnIdx].SetRect(insetRect(r, buttonPad))
			}
			btnIdx++
			curY += rowH
		}
	} else {
		curY := dv.contextMenuRect.Min.Y - scrollPx
		for _, item := range items {
			if item.divider {
				continue
			}
			if btnIdx < len(dv.contextMenuBtns)-1 { // -1 for close button
				r := image.Rect(dv.contextMenuRect.Min.X, curY, dv.contextMenuRect.Max.X, curY+rowH)
				dv.contextMenuBtns[btnIdx].SetRect(insetRect(r, buttonPad))
			}
			btnIdx++
			curY += rowH
		}
	}
}

// contextMenuItem defines a context menu entry. Set divider=true for a visual separator.
type contextMenuItem struct {
	label     string
	onClick   func()
	style     ButtonVisual
	divider   bool
	textColor color.Color // optional: override button text color (e.g. red for delete)
	icon      string      // optional: icon name drawn to left of label (e.g. "note", "pencil", "color", "mute", "solo", "fx", "target", "trash")
	group     int         // semantic group index (0=identity, 1=playback, 2=routing, 3=danger)
}

// contextMenuItems returns the grouped list of context menu entries for a row.
func (dv *DrumView) contextMenuItems(rowIdx int) []contextMenuItem {
	itemStyle := ButtonVisual(ContextMenuItemStyle)

	// Compute mute/solo style and label based on row state.
	muteStyle, muteLabel := itemStyle, "Mute"
	if rowIdx >= 0 && rowIdx < len(dv.Rows) && dv.Rows[rowIdx].Muted {
		muteStyle = ButtonVisual(MuteActiveStyle)
		muteLabel = "Muted"
	}
	soloStyle, soloLabel := itemStyle, "Solo"
	if rowIdx >= 0 && rowIdx < len(dv.Rows) && dv.Rows[rowIdx].Solo {
		soloStyle = ButtonVisual(SoloActiveStyle)
		soloLabel = "Soloed"
	}

	items := []contextMenuItem{
		// Group 0: Identity
		{label: "Instrument", icon: "note", style: itemStyle, group: 0, onClick: func() {
			dv.closeContextMenuPortal()
			dv.selRow = rowIdx
			dv.openInstMenuForRow(rowIdx)
		}},
		{label: "Rename", icon: "pencil", style: itemStyle, group: 0, onClick: func() {
			dv.closeContextMenuPortal()
			if rowIdx < len(dv.rowEditBtns()) {
				dv.rowEditBtns()[rowIdx].OnClick()
			}
		}},
		{label: "Color", icon: "color", style: itemStyle, group: 0, onClick: func() {
			dv.closeContextMenuPortal()
			dv.openColorPickerForRow(rowIdx)
		}},
		// Divider
		{divider: true},
		// Group 1: Playback
		{label: muteLabel, icon: "mute", style: muteStyle, group: 1, onClick: func() {
			dv.closeContextMenuPortal()
			dv.toggleMute(rowIdx)
		}},
		{label: soloLabel, icon: "solo", style: soloStyle, group: 1, onClick: func() {
			dv.closeContextMenuPortal()
			dv.toggleSolo(rowIdx)
		}},
		// Divider
		{divider: true},
		// Group 2: Effects & routing
		{label: "Effects", icon: "fx", style: itemStyle, group: 2, onClick: func() {
			dv.closeContextMenuPortal()
			dv.toggleFXPanel(rowIdx)
		}},
		{label: "Origin", icon: "target", style: itemStyle, group: 2, onClick: func() {
			dv.closeContextMenuPortal()
			dv.originReq = append(dv.originReq, rowIdx)
		}},
		// Divider
		{divider: true},
	}
	// Group 3: Destructive — red text on transparent surface
	deleteItemStyle := itemStyle
	deleteTextColor := colDeleteText
	if len(dv.Rows) <= 1 {
		deleteItemStyle = DisabledButtonStyle
		deleteTextColor = colTextDisabled
	}
	items = append(items, contextMenuItem{label: "Delete", icon: "trash", style: deleteItemStyle, textColor: deleteTextColor, group: 3, onClick: func() {
		dv.closeContextMenuPortal()
		if len(dv.Rows) > 1 {
			dv.DeleteRow(rowIdx)
		}
	}})
	return items
}

// handleContextMenuInput processes clicks on the context menu.
// Returns true if input was consumed.
//
// On mobile, uses the deferred-tap + scroll-commitment pattern (same as
// NodeSidebar.HandleInput) to disambiguate taps from scroll gestures.
func (dv *DrumView) handleContextMenuInput(mx, my int, left bool) bool {
	if !dv.IsContextMenuOpen() {
		return false
	}
	pt := image.Pt(mx, my)
	scroll := dv.contextMenuScroll

	if Profile().IsMobile() && scroll != nil {
		// 1. Scrollbar drag in-progress.
		if scroll.Dragging() {
			if left {
				scroll.HandleDragTo(my)
				dv.rebuildContextMenuButtons()
			} else {
				scroll.HandleDragEnd()
			}
			return true
		}

		// 2. ScrollingCommitted → handle move, cancel deferred tap on release.
		if scroll.ScrollingCommitted() {
			if left {
				if scroll.HandleTouchMove(mx, my) {
					dv.rebuildContextMenuButtons()
				}
			} else {
				dv.contextMenuDeferredTap.Cancel()
				scroll.HandleTouchEnd()
			}
			return true
		}

		// 3. TouchActive but not yet committed (in dead zone).
		if scroll.TouchActive() {
			if left {
				if scroll.HandleTouchMove(mx, my) {
					dv.rebuildContextMenuButtons()
				}
			} else {
				wasTap := !scroll.ScrollingCommitted()
				scroll.HandleTouchEnd()
				if wasTap {
					dv.contextMenuDeferredTap.End(dv.fireContextMenuTapAt)
				} else {
					dv.contextMenuDeferredTap.Cancel()
				}
			}
			return true
		}

		// 4. Scrollbar thumb click.
		if scroll.HasScroll() {
			thumbRect := scroll.ThumbRect()
			if left && pt.In(thumbRect) {
				scroll.HandleDragStart(my)
				return true
			}
		}

		// 5. Close button — highest z-order, check before item area.
		closeBtn := dv.contextMenuCloseBtn()
		if closeBtn != nil && left && pt.In(closeBtn.Rect()) {
			closeBtn.OnClick()
			return true
		}

		// 6. Press in item viewport → begin deferred tap + touch scroll.
		itemView := scroll.VS.View
		if left && pt.In(itemView) && !scroll.TouchActive() && !scroll.Dragging() {
			if dv.contextMenuDeferredTap.Begin(mx, my) {
				scroll.HandleTouchBegin(mx, my)
				return true
			}
		}

		// 7. Header area — consume without action.
		if pt.In(dv.contextMenuRect) && my < itemView.Min.Y {
			return true
		}
	} else {
		// Desktop: scrollbar drag support.
		if scroll != nil && scroll.Dragging() {
			if left {
				scroll.HandleDragTo(my)
				dv.rebuildContextMenuButtons()
			} else {
				scroll.HandleDragEnd()
			}
			return true
		}
		if scroll != nil && scroll.HasScroll() {
			thumbRect := scroll.ThumbRect()
			if left && pt.In(thumbRect) {
				scroll.HandleDragStart(my)
				return true
			}
		}
		// Desktop: immediate button handling.
		for _, btn := range dv.contextMenuBtns {
			if btn.Handle(mx, my, left) {
				return true
			}
		}
	}

	// Consume all input when open and inside the rect.
	// Click-outside closing is handled by the tree (drumview_tree.go:245-249).
	if pt.In(dv.contextMenuRect) {
		return true
	}
	return false
}

// contextMenuCloseBtn returns the close button (last in contextMenuBtns), or nil.
func (dv *DrumView) contextMenuCloseBtn() *Button {
	if len(dv.contextMenuBtns) == 0 {
		return nil
	}
	return dv.contextMenuBtns[len(dv.contextMenuBtns)-1]
}

// fireContextMenuTapAt finds the context menu button at (x, y) and fires it.
// Iterates in reverse: the close button is appended last and has highest
// z-order, so it must be checked before menu items it spatially overlaps.
func (dv *DrumView) fireContextMenuTapAt(x, y int) {
	pt := image.Pt(x, y)
	for i := len(dv.contextMenuBtns) - 1; i >= 0; i-- {
		btn := dv.contextMenuBtns[i]
		if pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
}

// drawContextMenu renders the context menu popup.
// On mobile, draws as a bottom sheet with group dividers and drag handle.
func (dv *DrumView) drawContextMenu(dst *ebiten.Image) {
	if !dv.IsContextMenuOpen() {
		return
	}
	drawScrim(dst)
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, dv.contextMenuRect)
	} else {
		drawPanel(dst, dv.contextMenuRect)
	}

	// Mobile: pill-shaped drag handle indicator at top of bottom sheet.
	if Profile().IsMobile() {
		handleW := 36
		handleH := 4
		hx := dv.contextMenuRect.Min.X + dv.contextMenuRect.Dx()/2 - handleW/2
		hy := dv.contextMenuRect.Min.Y + 8
		drawRoundedRect(dst, image.Rect(hx, hy, hx+handleW, hy+handleH),
			color.NRGBA{255, 255, 255, 40}, handleH/2, true)
	}

	// Draw header text (mobile bottom sheet) with row color dot.
	if Profile().IsMobile() && !dv.contextMenuHeaderRect.Empty() &&
		dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		row := dv.Rows[dv.contextMenuRow]
		// Draw row color dot before row name.
		dotSize := 8
		dotX := dv.contextMenuHeaderRect.Min.X
		dotY := dv.contextMenuHeaderRect.Min.Y + (dv.contextMenuHeaderRect.Dy()-dotSize)/2
		rowCol := row.Color
		if rowCol == nil {
			rowCol = instColor(row.Instrument)
		}
		drawRoundedRect(dst, image.Rect(dotX, dotY, dotX+dotSize, dotY+dotSize),
			rowCol, dotSize/2, true)
		// Draw row name offset to right of dot.
		textX := dotX + dotSize + SpaceMD
		DrawTextAt(dst, row.Name,
			textX,
			dv.contextMenuHeaderRect.Min.Y+(dv.contextMenuHeaderRect.Dy()-TextHeight())/2)
	}

	// Draw group container backgrounds behind items.
	if dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		items := dv.contextMenuItems(dv.contextMenuRow)
		isMobile := Profile().IsMobile()
		scrollPx := 0
		if dv.contextMenuScroll != nil {
			scrollPx = dv.contextMenuScroll.VS.First * touchMinTargetPx
		}
		var curY, itemViewTop int
		if isMobile {
			curY = dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx
			itemViewTop = dv.contextMenuRect.Min.Y + touchMinTargetPx
		} else {
			curY = dv.contextMenuRect.Min.Y
			itemViewTop = dv.contextMenuRect.Min.Y
		}

		// Walk items to compute group Y extents and draw group containers.
		groupStartY := curY
		lastGroup := -1
		pad := SpaceSM // 4px internal group padding
		insetX := SpaceSM
		radius := RadiusSM
		if isMobile {
			radius = RadiusMD
			insetX = SpaceSM
		}
		menuLeft := dv.contextMenuRect.Min.X + insetX
		menuRight := dv.contextMenuRect.Max.X - insetX

		drawGroupBG := func(startY, endY, group int) {
			if startY >= endY {
				return
			}
			gr := image.Rect(menuLeft, startY-pad, menuRight, endY+pad)
			// Clip to viewport
			if gr.Min.Y < itemViewTop {
				gr.Min.Y = itemViewTop
			}
			if gr.Max.Y > dv.contextMenuRect.Max.Y {
				gr.Max.Y = dv.contextMenuRect.Max.Y
			}
			if gr.Empty() {
				return
			}
			bg := colMenuGroupBG
			if group == 3 {
				bg = colMenuGroupDeleteBG
			}
			drawRoundedRect(dst, gr, bg, radius, true)
		}

		for _, item := range items {
			if item.divider {
				// Emit previous group
				if lastGroup >= 0 {
					drawGroupBG(groupStartY, curY, lastGroup)
				}
				curY += SpaceSM // group gap
				groupStartY = curY
				lastGroup = -1
			} else {
				if lastGroup < 0 {
					lastGroup = item.group
					groupStartY = curY
				}
				curY += touchMinTargetPx
			}
		}
		// Emit final group
		if lastGroup >= 0 {
			drawGroupBG(groupStartY, curY, lastGroup)
		}
	}

	// Draw buttons, clipping to the item viewport so off-screen buttons
	// don't render over the header or outside the menu.
	if dv.contextMenuScroll != nil && Profile().IsMobile() {
		itemView := dv.contextMenuScroll.VS.View
		for i, btn := range dv.contextMenuBtns {
			// Always draw the close button (last, lives in header area).
			if i == len(dv.contextMenuBtns)-1 {
				btn.Draw(dst)
				continue
			}
			r := btn.Rect()
			if r.Max.Y > itemView.Min.Y && r.Min.Y < itemView.Max.Y {
				btn.Draw(dst)
			}
		}
	} else {
		for _, btn := range dv.contextMenuBtns {
			btn.Draw(dst)
		}
	}

	// Draw context menu icons to the left of each button's text.
	iconSize := IconSizeSM
	if Profile().IsMobile() {
		iconSize = IconSizeMD
	}
	iconCol := colMenuIcon
	for i, btn := range dv.contextMenuBtns {
		// Skip close button (last) and buttons without icons.
		if i >= len(dv.contextMenuIcons) {
			break
		}
		ico := dv.contextMenuIcons[i]
		if ico == "" {
			continue
		}
		r := btn.Rect()
		if r.Empty() {
			continue
		}
		// Position icon at left edge of button, vertically centered.
		ix := r.Min.X + SpaceMD
		iy := r.Min.Y + (r.Dy()-iconSize)/2
		iconR := image.Rect(ix, iy, ix+iconSize, iy+iconSize)
		drawContextMenuIcon(dst, iconR, ico, iconCol)
	}

	// Scrollbar (drawn on top of buttons)
	if dv.contextMenuScroll != nil {
		dv.contextMenuScroll.Draw(dst)
	}
}

// drawContextMenuIcon draws a small geometric icon for context menu items.
func drawContextMenuIcon(dst *ebiten.Image, r image.Rectangle, icon string, col color.Color) {
	if r.Empty() {
		return
	}
	switch icon {
	case "note":
		// Musical note: small filled circle + vertical line
		dim := r.Dy()
		cx := r.Min.X + dim/2
		dotR := dim / 4
		if dotR < 2 {
			dotR = 2
		}
		// Note head (bottom-left)
		dotY := r.Max.Y - dotR
		drawRoundedRect(dst, image.Rect(cx-dotR, dotY-dotR, cx+dotR, dotY+dotR), col, dotR, true)
		// Stem (vertical line up from note head)
		stemX := cx + dotR - 1
		drawRect(dst, image.Rect(stemX, r.Min.Y+2, stemX+1, dotY), col, true)
	case "pencil":
		drawPencilIcon(dst, r, col)
	case "color":
		// Filled circle with the instrument color would need the color passed.
		// Use a generic circle icon.
		cx := (r.Min.X + r.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		rad := minI(r.Dx(), r.Dy()) / 3
		if rad < 2 {
			rad = 2
		}
		drawRoundedRect(dst, image.Rect(cx-rad, cy-rad, cx+rad, cy+rad), col, rad, true)
	case "mute":
		// Speaker-off: speaker body + diagonal strike
		dim := minI(r.Dx(), r.Dy())
		inner := insetRect(r, dim/5)
		cx := (inner.Min.X + inner.Max.X) / 2
		cy := (inner.Min.Y + inner.Max.Y) / 2
		bodyW := dim / 5
		bodyH := dim / 4
		drawRect(dst, image.Rect(cx-bodyW, cy-bodyH, cx, cy+bodyH), col, true)
		// Cone
		drawRect(dst, image.Rect(cx, cy-bodyH*3/2, cx+bodyW+1, cy+bodyH*3/2), col, true)
		// Diagonal strike
		thick := maxI(dim/10, 1)
		steps := max1(inner.Dx())
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps)
			x := inner.Min.X + int(t*float64(inner.Dx()))
			y := inner.Min.Y + int(t*float64(inner.Dy()))
			drawRect(dst, image.Rect(x, y, x+thick, y+thick), col, true)
		}
	case "solo":
		// Headphone: arc + ear cups
		dim := minI(r.Dx(), r.Dy())
		inner := insetRect(r, dim/5)
		cy := (inner.Min.Y + inner.Max.Y) / 2
		thick := maxI(dim/8, 1)
		// Headband arc (top half)
		drawRect(dst, image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+thick), col, true)
		// Left arm
		drawRect(dst, image.Rect(inner.Min.X, inner.Min.Y, inner.Min.X+thick, cy), col, true)
		// Right arm
		drawRect(dst, image.Rect(inner.Max.X-thick, inner.Min.Y, inner.Max.X, cy), col, true)
		// Ear cups
		cupW := dim / 4
		cupH := dim / 3
		drawRect(dst, image.Rect(inner.Min.X, cy, inner.Min.X+cupW, cy+cupH), col, true)
		drawRect(dst, image.Rect(inner.Max.X-cupW, cy, inner.Max.X, cy+cupH), col, true)
	case "fx":
		// Sparkle/star: 4 points
		cx := (r.Min.X + r.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		arm := minI(r.Dx(), r.Dy()) / 3
		thick := maxI(arm/3, 1)
		// Vertical
		drawRect(dst, image.Rect(cx-thick/2, cy-arm, cx+thick/2+1, cy+arm), col, true)
		// Horizontal
		drawRect(dst, image.Rect(cx-arm, cy-thick/2, cx+arm, cy+thick/2+1), col, true)
	case "target":
		// Crosshair: circle + cross
		cx := (r.Min.X + r.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		rad := minI(r.Dx(), r.Dy()) / 3
		thick := maxI(rad/3, 1)
		// Circle (stroked)
		drawRoundedRect(dst, image.Rect(cx-rad, cy-rad, cx+rad, cy+rad), col, rad, false)
		// Cross
		drawRect(dst, image.Rect(cx-thick/2, cy-rad-1, cx+thick/2+1, cy+rad+1), col, true)
		drawRect(dst, image.Rect(cx-rad-1, cy-thick/2, cx+rad+1, cy+thick/2+1), col, true)
	case "trash":
		// Trash: rectangle body with lid
		dim := minI(r.Dx(), r.Dy())
		inner := insetRect(r, dim/5)
		if inner.Empty() {
			return
		}
		// Lid (top bar, slightly wider)
		lidH := maxI(inner.Dy()/6, 1)
		drawRect(dst, image.Rect(inner.Min.X-1, inner.Min.Y, inner.Max.X+1, inner.Min.Y+lidH), col, true)
		// Handle (small rect centered on lid)
		handleW := inner.Dx() / 3
		hx := (inner.Min.X + inner.Max.X) / 2
		drawRect(dst, image.Rect(hx-handleW/2, inner.Min.Y-lidH, hx+handleW/2, inner.Min.Y), col, true)
		// Body (stroked rect below lid)
		bodyTop := inner.Min.Y + lidH + 1
		drawRect(dst, image.Rect(inner.Min.X+1, bodyTop, inner.Max.X-1, inner.Max.Y), col, false)
	}
}

// handleOverflowMenuInput processes clicks on the overflow popup (Upload/Import/Export).
// Returns true if input was consumed.
func (dv *DrumView) handleOverflowMenuInput(mx, my int, left bool) bool {
	if !dv.IsOverflowMenuOpen() {
		return false
	}
	// Build popup rect anchored below the overflow button.
	popupRect := dv.overflowPopupRect()
	pt := image.Pt(mx, my)

	// On mobile, use deferred tap pattern.
	if Profile().IsMobile() {
		if dv.overflowDeferredTap.Active() {
			if !left {
				dv.overflowDeferredTap.End(dv.fireOverflowMenuTapAt)
			}
			return true
		}
		if left && pt.In(popupRect) {
			if dv.overflowDeferredTap.Begin(mx, my) {
				return true
			}
		}
	} else {
		// Desktop: immediate button handling.
		popupBtns := dv.overflowPopupBtns(popupRect)
		for _, btn := range popupBtns {
			if btn.Handle(mx, my, left) {
				return true
			}
		}
	}

	// Consume all input when open and inside the popup rect.
	// Click-outside closing is handled by the tree (drumview_tree.go:245-249).
	if pt.In(popupRect) {
		return true
	}
	return false
}

// fireOverflowMenuTapAt finds the overflow menu button at (x, y) and fires it.
// Iterates in reverse: the close button is appended last and has highest z-order.
func (dv *DrumView) fireOverflowMenuTapAt(x, y int) {
	pt := image.Pt(x, y)
	popupRect := dv.overflowPopupRect()
	popupBtns := dv.overflowPopupBtns(popupRect)
	for i := len(popupBtns) - 1; i >= 0; i-- {
		btn := popupBtns[i]
		if pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
}

// openColorPickerForRow opens the color wheel picker for the given row,
// using a fallback anchor rect when the swatch button is hidden (mobile).
func (dv *DrumView) openColorPickerForRow(rowIdx int) {
	dv.selRow = rowIdx

	// Toggle if already open for this row.
	if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() && dv.colorMenuRow == rowIdx {
		dv.colorWheelComp.Close()
		return
	}

	// Close all other overlays for mutual exclusivity.
	dv.CloseAllPopups()

	dv.colorMenuRow = rowIdx

	// Determine anchor: prefer the color swatch button, fall back to row label
	// (on mobile the swatch column has zero width).
	anchor := image.Rectangle{}
	if rowIdx < len(dv.rowColorBtns()) {
		anchor = dv.rowColorBtns()[rowIdx].Rect()
	}
	if anchor.Empty() && rowIdx < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[rowIdx].Rect()
	}

	if dv.colorWheelComp != nil {
		rackBounds := dv.widgetRects[WidgetRack]
		if rackBounds.Empty() {
			rackBounds = dv.Bounds
		}
		dv.colorWheelComp.SetProps(ColorWheelProps{
			AnchorRect: anchor,
			Bounds:     rackBounds,
			RowHeight:  dv.rowHeight(),
			OnColorPick: func(c color.Color) {
				dv.SetRowColor(dv.colorMenuRow, c)
			},
			OnClose: func() {},
		})
		dv.colorWheelComp.Open()
		dv.colorWheelComp.ClearHold()
	}

	dv.buildColorMenu()
	dv.openColorWheelPortal()
}

// overflowPopupRect returns the rectangle for the overflow popup.
func (dv *DrumView) overflowPopupRect() image.Rectangle {
	if dv.overflowBtn() == nil {
		return image.Rectangle{}
	}
	anchor := dv.overflowBtn().Rect()
	w := 160
	items := dv.overflowItems()
	h := len(items) * touchMinTargetPx
	x := anchor.Max.X - w
	y := anchor.Max.Y + 2
	if x < dv.Bounds.Min.X {
		x = dv.Bounds.Min.X
	}
	if x+w > dv.Bounds.Max.X {
		x = dv.Bounds.Max.X - w
	}
	if y+h > dv.Bounds.Max.Y {
		y = anchor.Min.Y - h - 2
	}
	// Clamp so the popup never escapes above the drum pane.
	// Without this, on adaptive portrait layouts where the drum pane starts
	// near the bottom (e.g. Bounds.Min.Y ≈ 600), the flipped popup lands in
	// the grid area, and InputDispatcher skips the drum view (InputBounds()
	// returns dv.Bounds), so clicks create unwanted grid nodes.
	if y < dv.Bounds.Min.Y {
		y = dv.Bounds.Min.Y
	}
	// Cap height so the popup does not overflow below the drum pane.
	if maxH := dv.Bounds.Max.Y - y; h > maxH {
		h = maxH
		if h < touchMinTargetPx {
			h = touchMinTargetPx
		}
	}
	return image.Rect(x, y, x+w, y+h)
}

// overflowItem defines a menu entry for the overflow popup.
type overflowItem struct {
	label   string
	onClick func()
}

// overflowItems returns the full list of overflow menu entries.
func (dv *DrumView) overflowItems() []overflowItem {
	var items []overflowItem
	// File operations (view mode toggle removed — viewSwitchBtn in toolbar handles it)
	items = append(items,
		overflowItem{"Upload", func() {
			dv.closeOverflowMenu()
			if dv.uploadBtn().OnClick != nil {
				dv.uploadBtn().OnClick()
			}
		}},
		overflowItem{"Import", func() {
			dv.closeOverflowMenu()
			if dv.importBtn().OnClick != nil {
				dv.importBtn().OnClick()
			}
		}},
		overflowItem{"Export", func() {
			dv.closeOverflowMenu()
			if dv.exportBtn().OnClick != nil {
				dv.exportBtn().OnClick()
			}
		}},
	)
	return items
}

// overflowPopupBtns builds the buttons for the overflow popup.
func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	btns := make([]*Button, 0, len(items)+1)
	itemStyle := ButtonVisual(DropdownStyle)
	for i, item := range items {
		y0 := popupRect.Min.Y + i*rowH
		r := image.Rect(popupRect.Min.X, y0, popupRect.Max.X, y0+rowH)
		btn := NewButton(item.label, itemStyle, item.onClick)
		btn.SetRect(insetRect(r, buttonPad))
		btns = append(btns, btn)
	}
	// Close button at top-right
	closeR := closeButtonRect(popupRect, buttonPad)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeOverflowMenu() })
	closeB.Icon = "close"
	closeB.IconColor = colButtonBorder
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	btns = append(btns, closeB)
	return btns
}

// drawOverflowMenu renders the overflow popup.
func (dv *DrumView) drawOverflowMenu(dst *ebiten.Image) {
	if !dv.IsOverflowMenuOpen() {
		return
	}
	popupRect := dv.overflowPopupRect()
	drawScrim(dst)
	drawPanel(dst, popupRect)
	for _, btn := range dv.overflowPopupBtns(popupRect) {
		btn.Draw(dst)
	}
	if dv.overflowScroll != nil {
		dv.overflowScroll.Draw(dst)
	}
}

// initOverflowScroll initializes scroll behavior for the overflow menu.
func (dv *DrumView) initOverflowScroll() {
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	popupRect := dv.overflowPopupRect()
	visibleItems := popupRect.Dy() / rowH
	if visibleItems > len(items) {
		visibleItems = len(items)
	}
	dv.overflowScroll = NewScrollBehavior(DropdownScrollbarStyle, rowH)
	dv.overflowScroll.VS.Total = len(items)
	dv.overflowScroll.VS.Visible = visibleItems
	dv.overflowScroll.VS.View = popupRect
}

// closeOverflowMenu centralizes overflow menu teardown, including clearing
// file picker rects registered for mobile gesture-based file picking.
func (dv *DrumView) closeOverflowMenu() {
	dv.overflowDeferredTap.Cancel()
	filePickerClearRects()
	dv.closeOverflowMenuPortal()
}

// registerFilePickerRects computes the Import and Upload button positions from
// the overflow popup and registers them as file picker rects for the mobile
// gesture-based system. This allows touchend inside these buttons to open the
// file picker synchronously within the trusted gesture handler.
func (dv *DrumView) registerFilePickerRects() {
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		return
	}
	rowH := touchMinTargetPx
	// Locate Upload and Import by iterating overflowItems() so that the
	// indices stay correct when items are added or reordered.
	// (Previously hardcoded indices 0 and 1 broke when Track/Len+/Len-/EQ
	// were prepended, pushing Upload to index 4 and Import to index 5.)
	accept := map[string]string{
		"Upload": ".wav",
		"Import": "application/json,.json",
	}
	idByLabel := map[string]string{
		"Upload": "upload",
		"Import": "import",
	}
	closeR := closeButtonRect(popupRect, buttonPad)
	for i, item := range dv.overflowItems() {
		fileID, ok := idByLabel[item.label]
		if !ok {
			continue
		}
		y0 := popupRect.Min.Y + i*rowH
		r := insetRect(image.Rect(popupRect.Min.X, y0, popupRect.Max.X, y0+rowH), buttonPad)
		// Crop to avoid overlap with close button so tapping close
		// doesn't also trigger the file picker.
		if r.Max.Y > closeR.Min.Y && r.Min.Y < closeR.Max.Y {
			if r.Max.X > closeR.Min.X {
				r.Max.X = closeR.Min.X - SpaceSM
			}
		}
		filePickerRegisterRect(fileID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), accept[item.label])
	}
}

// cycleViewMode toggles between Rows and Audio (EQ/Wave) views.
func (dv *DrumView) cycleViewMode() {
	if dv.currentViewMode == viewModeRows {
		dv.currentViewMode = viewModeAudio
		dv.mobileEQMode = true
	} else {
		dv.currentViewMode = viewModeRows
		dv.mobileEQMode = false
	}
	dv.syncViewSwitchIcon()
	if dv.mobileEQMode {
		dv.CloseAllPopups()
		dv.rowScroll().ResetTouch()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	dv.markAllRowsDirty()
	dv.rowsLayerDirty = true
}

// syncViewSwitchIcon updates the view switch button icon to show the OTHER mode.
// In Rows mode → show "audio" icon (to switch to Audio).
// In Audio mode → show "rows" icon (to switch back to Rows).
func (dv *DrumView) syncViewSwitchIcon() {
	if dv.viewSwitchBtn() == nil {
		return
	}
	if dv.currentViewMode == viewModeRows {
		dv.viewSwitchBtn().Icon = "audio"
	} else {
		dv.viewSwitchBtn().Icon = "rows"
	}
	// Keep legacy button in sync.
	if dv.eqToggleMobile() != nil {
		if dv.mobileEQMode {
			dv.eqToggleMobile().Text = "Rows"
		} else {
			dv.eqToggleMobile().Text = "EQ"
		}
	}
}

// wrapOverflowBtn lets tests access the overflow button.
func (dv *DrumView) OverflowBtn() *Button { return dv.overflowBtn() }

// EqToggleMobileBtn lets tests access the mobile EQ toggle button.
func (dv *DrumView) EqToggleMobileBtn() *Button { return dv.eqToggleMobile() }

// MobileEQCollapsed returns the mobile EQ collapsed state for testing.
func (dv *DrumView) MobileEQCollapsed() bool { return dv.mobileEQCollapsed }

// SetMobileEQCollapsed sets the mobile EQ collapsed state for testing.
func (dv *DrumView) SetMobileEQCollapsed(v bool) { dv.mobileEQCollapsed = v }

// MobileEQMode returns whether the mobile EQ mode is active (for testing).
func (dv *DrumView) MobileEQMode() bool { return dv.mobileEQMode }

// SetMobileEQMode sets the mobile EQ mode (for testing).
func (dv *DrumView) SetMobileEQMode(v bool) { dv.mobileEQMode = v }

// ContextMenuOpen returns whether the context menu is open (for testing).
func (dv *DrumView) ContextMenuOpen() bool { return dv.IsContextMenuOpen() }

// ContextMenuBtns returns the context menu buttons (for testing).
func (dv *DrumView) ContextMenuBtns() []*Button { return dv.contextMenuBtns }

// ContextMenuRect returns the context menu rect (for testing).
func (dv *DrumView) ContextMenuRectVal() image.Rectangle { return dv.contextMenuRect }

// OverflowMenuOpen returns whether the overflow menu is open (for testing).
func (dv *DrumView) OverflowMenuOpen() bool { return dv.IsOverflowMenuOpen() }

// SetOverflowMenuOpen sets the overflow menu open state (for testing).
// Opens/closes the overflow menu portal for test compatibility.
func (dv *DrumView) SetOverflowMenuOpen(v bool) {
	if v {
		dv.openOverflowMenuPortal()
	} else {
		dv.closeOverflowMenuPortal()
	}
}

// RowMenuBtns returns the per-row kebab menu buttons (for testing).
func (dv *DrumView) RowMenuBtns() []*Button { return dv.rowMenuBtns() }

// OpenContextMenuForTest opens the context menu for a specific row (for testing).
func (dv *DrumView) OpenContextMenuForTest(rowIdx int) { dv.openContextMenu(rowIdx) }

// ContextMenuItemsForTest returns context menu items for testing.
func (dv *DrumView) ContextMenuItemsForTest(rowIdx int) []contextMenuItem {
	return dv.contextMenuItems(rowIdx)
}

// OverflowItemsForTest returns overflow menu items for testing.
func (dv *DrumView) OverflowItemsForTest() []overflowItem {
	return dv.overflowItems()
}

// TrackBtnForTest returns the track button for testing.
func (dv *DrumView) TrackBtnForTest() *Button { return dv.trackBtn() }

// TimelineRectForTest returns the timeline rect for testing.
func (dv *DrumView) TimelineRectForTest() image.Rectangle { return dv.timelineRect }

// ContextMenuScrollForTest returns the context menu scroll behavior for testing.
func (dv *DrumView) ContextMenuScrollForTest() *ScrollBehavior { return dv.contextMenuScroll }
