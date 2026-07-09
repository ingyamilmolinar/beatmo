package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// contextMenuAccent returns the accent color for the open per-row context menu:
// the row's instrument color so the menu chrome (header underline + active item
// stripe + color dot) reads as owned by the instrument. Falls back to the
// instrument's registered color, then the global azure accent when no row is
// targeted.
func (dv *DrumView) contextMenuAccent() color.Color {
	if dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		if c := dv.Rows[dv.contextMenuRow].Color; c != nil {
			return c
		}
		if c := instColor(dv.Rows[dv.contextMenuRow].Instrument); c != nil {
			return c
		}
	}
	return colAccent
}

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
	dv.contextMenuCloseButton = nil
	dv.contextMenuItemIcons = make(map[*Button]string)

	rowH := touchMinTargetPx
	headerH := touchMinTargetPx // title row (instrument dot + name) — both platforms
	dividerH := 0               // group containers replace divider lines; gap is SpaceSM only
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
			btn.SetRect(insetRect(r, SpaceXS))
			if item.textColor != nil {
				btn.TextColor = item.textColor
			}
			dv.contextMenuBtns = append(dv.contextMenuBtns, btn)
			dv.contextMenuItemIcons[btn] = item.icon
			curY += rowH
		}
	} else {
		// Desktop: open the menu right where the user clicked — anchored to the
		// row's kebab (⋯) button, growing downward (flips up / clamps on-screen
		// via AnchorPopupRect). Because the kebab sits just above the bottom
		// panel, this lands the menu at the top of the bottom panel near the
		// finger. Falls back to the row label when the kebab isn't laid out
		// (partial-DrumView tests).
		menuW := 160
		menuH := headerH + nItems*rowH
		var anchor image.Rectangle
		if menus := dv.rowMenuBtns(); rowIdx < len(menus) && !menus[rowIdx].Rect().Empty() {
			anchor = menus[rowIdx].Rect()
		} else if rowIdx < len(dv.rowLabels()) {
			anchor = dv.rowLabels()[rowIdx].Rect()
		} else {
			anchor = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+100, dv.Bounds.Min.Y+dv.headerH+dv.rowHeight())
		}
		dv.contextMenuRect = AnchorPopupRect(dv.Bounds, anchor, menuW, menuH, PopupBelow)

		// Header rect (instrument dot + name title) tracked for drawing; not a
		// button so indices stay stable for tests / deferred-tap.
		dv.contextMenuHeaderRect = image.Rect(
			dv.contextMenuRect.Min.X+SpaceMD, dv.contextMenuRect.Min.Y+SpaceMD,
			dv.contextMenuRect.Max.X-touchMinTargetPx, dv.contextMenuRect.Min.Y+SpaceMD+headerH-SpaceSM)

		// Full-width item rows (icon + label drawn via shared geometry). Items
		// start below the header row.
		curY := dv.contextMenuRect.Min.Y + headerH
		for _, item := range items {
			if item.divider {
				continue // no extra pitch: uniform row height on desktop
			}
			r := image.Rect(dv.contextMenuRect.Min.X+SpaceMD, curY, dv.contextMenuRect.Max.X-SpaceMD, curY+rowH)
			btn := NewButton(item.label, item.style, item.onClick)
			btn.SetRect(insetRect(r, SpaceXS))
			if item.textColor != nil {
				btn.TextColor = item.textColor
			}
			dv.contextMenuBtns = append(dv.contextMenuBtns, btn)
			dv.contextMenuItemIcons[btn] = item.icon
			curY += rowH
		}
	}

	// Initialize scroll behavior for the context menu.
	// On mobile, the scroll viewport excludes the header so the header stays
	// pinned at the top while items scroll underneath.
	dv.contextMenuScroll = NewScrollBehavior(dropdownScrollbarStyle(), rowH)
	dv.contextMenuScroll.VS.Total = nItems
	// Both platforms reserve the header row at the top; items scroll beneath it.
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

	// Close button at the header's top-right — on both platforms now (the desktop
	// menu has a header row, so the × sits in the header, not over a menu item).
	closeR := closeButtonRect(dv.contextMenuRect, SpaceXS)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeContextMenuPortal() })
	closeB.Icon = "close"
	closeB.IconColor = closeIconColor()
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	dv.contextMenuCloseButton = closeB
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
	scrollPx := dv.contextMenuScroll.VS.First * dv.contextMenuScroll.ItemHeight

	// Rebuild non-close buttons with scroll offset. itemBtnCount is the number
	// of menu-item buttons (mobile appends a trailing close button; desktop
	// does not), so the guard works on both platforms.
	items := dv.contextMenuItems(dv.contextMenuRow)
	itemBtnCount := len(dv.contextMenuBtns) - 1 // trailing close button (both platforms)
	btnIdx := 0
	if Profile().UseBottomSheet {
		curY := dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx // skip header
		for _, item := range items {
			if item.divider {
				curY += SpaceSM // group gap (no divider line)
				continue
			}
			if btnIdx < itemBtnCount {
				r := image.Rect(dv.contextMenuRect.Min.X+SpaceMD, curY, dv.contextMenuRect.Max.X-SpaceMD, curY+rowH)
				dv.contextMenuBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
			}
			btnIdx++
			curY += rowH
		}
	} else {
		// Desktop: uniform row pitch (no divider gap), full-width rows — mirror
		// openContextMenu's desktop layout.
		curY := dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx // skip header
		for _, item := range items {
			if item.divider {
				continue
			}
			if btnIdx < itemBtnCount {
				r := image.Rect(dv.contextMenuRect.Min.X+SpaceMD, curY, dv.contextMenuRect.Max.X-SpaceMD, curY+rowH)
				dv.contextMenuBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
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
// The menu is identical on every platform: Rename, Color, Origin, Delete.
//
// "Instrument" is no longer a menu entry — tapping the instrument-name label
// opens the instrument picker directly on every platform (see row_rack_zone.go
// and the 2026-06-13 mobile-instrument-button design). "Color" opens the
// grouped Vice City color picker via OpenColorMenu (the same entry point used
// by scenes / JS exports). "Effects" is omitted: it has its own row button.
// Mute and Solo are omitted on all platforms — they are exposed in the row
// controls.
func (dv *DrumView) contextMenuItems(rowIdx int) []contextMenuItem {
	// Keycap style (same as the instrument / subdivision / template menus) so the
	// row ellipsis menu's filled cap face + press/hover animation match them — the
	// shared drawMenuRow renderer paints btn.Style on the cap, and DropdownStyle
	// fills it (the old transparent ContextMenuItemStyle left the cap face empty).
	// Delete still overrides to DisabledButtonStyle when disabled and keeps its
	// red label color.
	itemStyle := ButtonVisual(DropdownStyle)

	var items []contextMenuItem

	// Group 0: Identity
	items = append(items,
		contextMenuItem{label: i18n.T(i18n.KeyMenuRename), icon: "pencil", style: itemStyle, group: 0, onClick: func() {
			dv.closeContextMenuPortal()
			if rowIdx < len(dv.rowEditBtns()) {
				dv.rowEditBtns()[rowIdx].OnClick()
			}
		}},
	)
	items = append(items, contextMenuItem{divider: true})

	// Group 1: Appearance — open the grouped Vice City color picker.
	items = append(items,
		contextMenuItem{label: i18n.T(i18n.KeyMenuColor), icon: "color", style: itemStyle, group: 1, onClick: func() {
			dv.closeContextMenuPortal()
			dv.OpenColorMenu(rowIdx)
		}},
		contextMenuItem{divider: true},
	)

	// Group 2: Routing
	items = append(items,
		contextMenuItem{label: i18n.T(i18n.KeyMenuOrigin), icon: "target", style: itemStyle, group: 2, onClick: func() {
			dv.closeContextMenuPortal()
			dv.originReq = append(dv.originReq, rowIdx)
		}},
		contextMenuItem{divider: true},
	)

	// Group 3: Destructive — red text on transparent surface
	deleteItemStyle := itemStyle
	deleteTextColor := colDeleteText
	if len(dv.Rows) <= 1 {
		deleteItemStyle = DisabledButtonStyle
		deleteTextColor = colTextDisabled
	}
	items = append(items, contextMenuItem{label: i18n.T(i18n.KeyMenuDelete), icon: "trash", style: deleteItemStyle, textColor: deleteTextColor, group: 3, onClick: func() {
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
			if btn.HandleInputResult(mx, my, left) != InputIgnored {
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

// contextMenuCloseBtn returns the close (×) button by identity, or nil when the
// menu is closed. Tracked as a field rather than read from contextMenuBtns'
// tail so a future trailing button can never silently become "the close button".
func (dv *DrumView) contextMenuCloseBtn() *Button {
	return dv.contextMenuCloseButton
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
	// Backdrop scrim is painted by the OverlayPortal (PortalEntry.Scrim) so the
	// whole stack dims once; this overlay only draws its own surface.
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, dv.contextMenuRect)
	} else {
		drawPanel(dst, dv.contextMenuRect)
	}

	// Mobile: pill-shaped drag handle indicator at top of bottom sheet.
	if Profile().IsMobile() {
		drawSheetDragHandle(dst, dv.contextMenuRect)
	}

	// Draw header text (both platforms) with row color dot.
	if !dv.contextMenuHeaderRect.Empty() &&
		dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		row := dv.Rows[dv.contextMenuRow]
		// "Neon Horizon" header band across the full header height.
		headerBandRect := image.Rect(
			dv.contextMenuRect.Min.X, dv.contextMenuRect.Min.Y,
			dv.contextMenuRect.Max.X, dv.contextMenuRect.Min.Y+touchMinTargetPx,
		)
		drawMenuHeaderBandAccent(dst, headerBandRect, dv.contextMenuAccent())
		// Unified menu title: instrument swatch + name (shared drawMenuTitle).
		rowCol := row.Color
		if rowCol == nil {
			rowCol = instColor(row.Instrument)
		}
		drawMenuTitle(dst, dv.contextMenuHeaderRect, row.Name, rowCol)
	}

	// Hairline separators between groups (replaces the old group-container
	// boxes for the flat look). Walk items mirroring the layout pitch in
	// openContextMenu / rebuildContextMenuButtons.
	if dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		items := dv.contextMenuItems(dv.contextMenuRow)
		isMobile := Profile().IsMobile()
		scrollPx := 0
		if dv.contextMenuScroll != nil {
			scrollPx = dv.contextMenuScroll.VS.First * touchMinTargetPx
		}
		// Items start below the header row on both platforms.
		curY := dv.contextMenuRect.Min.Y + touchMinTargetPx - scrollPx
		dividerGap := SpaceSM
		if !isMobile {
			dividerGap = 0
		}
		headerBottom := dv.contextMenuRect.Min.Y + touchMinTargetPx
		for _, item := range items {
			if item.divider {
				if curY > headerBottom && curY < dv.contextMenuRect.Max.Y {
					drawMenuSeparator(dst, image.Rect(dv.contextMenuRect.Min.X, curY-1, dv.contextMenuRect.Max.X, curY+1))
				}
				curY += dividerGap
				continue
			}
			curY += touchMinTargetPx
		}
	}

	// drawContextMenuItem renders a single item button via the shared
	// drawMenuRow primitive — gaining keycap chrome + press/hover animation.
	// Per-item text color (e.g. red Delete) and icon mapping are preserved.
	drawContextMenuItem := func(i int, btn *Button) {
		if btn.Rect().Empty() {
			return
		}
		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		// Resolve the context menu's string icon name to an IconID, mirroring
		// the old drawContextMenuIcon mapping ("color" → IconCircle).
		var iconID IconID
		if ico := dv.contextMenuItemIcons[btn]; ico != "" {
			iconID = IconID(ico)
			if ico == "color" {
				iconID = IconCircle
			}
		}
		// Preserve per-item label color (e.g. red Delete).
		var labelCol color.Color // nil → colTextPrimary in drawMenuRow
		if btn.TextColor != nil {
			labelCol = btn.TextColor
		}
		drawMenuRow(dst, btn, MenuRowSpec{
			Accent:     dv.contextMenuAccent(),
			State:      state,
			IconID:     iconID,
			IconTint:   colMenuIcon,
			Label:      btn.Text,
			LabelColor: labelCol,
		})
	}

	// Draw buttons, clipping items to the item viewport so off-screen buttons
	// don't render over the header or outside the menu. The trailing close
	// button lives in the header area and draws via its own path (both
	// platforms).
	itemView := dv.contextMenuRect
	if dv.contextMenuScroll != nil {
		itemView = dv.contextMenuScroll.VS.View
	}
	for i, btn := range dv.contextMenuBtns {
		if btn == dv.contextMenuCloseButton {
			btn.Draw(dst) // close button (lives in header area)
			continue
		}
		r := btn.Rect()
		if r.Max.Y > itemView.Min.Y && r.Min.Y < itemView.Max.Y {
			drawContextMenuItem(i, btn)
		}
	}

	// Scrollbar (drawn on top of buttons)
	if dv.contextMenuScroll != nil {
		dv.contextMenuScroll.Draw(dst)
	}
}

// contextMenuItemGeom returns the leading-icon rect and the label x for menu
// item i — used by tests to assert icon + label geometry. Items use full-width
// button rects, so the small icon sits at the row's leading edge and the label
// starts to its right (via menuRowIconRect / menuRowLabelX).
func (dv *DrumView) contextMenuItemGeom(i int) (image.Rectangle, int) {
	if i < 0 || i >= len(dv.contextMenuBtns) {
		return image.Rectangle{}, 0
	}
	r := dv.contextMenuBtns[i].Rect()
	return menuRowIconRect(r), menuRowLabelX(r)
}

// handleOverflowMenuInput processes clicks on the overflow popup (Upload/Import/Export).
// Returns true if input was consumed.
func (dv *DrumView) handleOverflowMenuInput(mx, my int, left bool) bool {
	if !dv.IsOverflowMenuOpen() {
		return false
	}
	dv.configureOverflowScroll()
	popupRect := dv.overflowPopupRect()
	btns := dv.overflowPopupBtns(popupRect)
	closeBtn := btns[len(btns)-1]

	return dv.overflowMenuScroll.HandleInput(MenuScrollInput{
		Pt:        image.Pt(mx, my),
		Pressed:   left,
		PopupRect: popupRect,
		ItemView:  popupRect,
		// Buttons persist across frames; overflowPopupBtns only refreshes their rects each frame, so no Relayout.
		FireTapAt: dv.fireOverflowMenuTapAt,
		CloseBtn:  closeBtn,
		DesktopHit: func(pt image.Point, pressed bool) bool {
			for _, b := range btns {
				if b.HandleInputResult(pt.X, pt.Y, pressed) != InputIgnored {
					return true
				}
			}
			return false
		},
	})
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
			AnchorRect:   anchor,
			Bounds:       rackBounds,
			RowHeight:    dv.rowHeight(),
			CurrentColor: dv.rowColorAt(rowIdx),
			OnColorPick: func(c color.Color) {
				dv.SetRowColorManual(dv.colorMenuRow, c)
			},
			OnClose: func() {},
		})
		dv.colorWheelComp.Open()
		dv.colorWheelComp.ClearHold()
	}

	dv.buildColorMenu()
	dv.openColorWheelPortal()
}

// OverflowPopupRect returns the on-screen rectangle for the overflow popup
// (mobile bottom-sheet style or desktop dropdown). The result is only
// meaningful while IsOverflowMenuOpen() is true.
func (dv *DrumView) OverflowPopupRect() image.Rectangle { return dv.overflowPopupRect() }

// overflowPopupDefaultWidth is the minimum/baseline desktop popup width. The
// File page (short labels) sits at this width; the template page grows past it
// to fit the longest "Artist — Song" title (see overflowPopupWidth).
const overflowPopupDefaultWidth = 160

// overflowPopupWidth returns the desktop popup width sized to fit the widest
// row label. Labels start at menuRowLabelX (icon gutter) and need symmetric
// right padding plus room for the scrollbar; the result is clamped to a sane
// minimum and capped so the popup never exceeds the drum pane's width.
func (dv *DrumView) overflowPopupWidth(items []overflowItem) int {
	// Left inset reproduces drawOverflowMenu's geometry: each row rect is inset
	// by SpaceXS before menuRowLabelX adds the icon gutter.
	labelLeft := SpaceXS + (SpaceMD + menuRowIconSize() + SpaceSM)
	rightPad := SpaceMD + SpaceXS + dropdownScrollbarWidth()

	maxLabelW := 0
	for _, it := range items {
		if it.header {
			continue
		}
		if lw := StyledTextWidth(it.label, RoleBody); lw > maxLabelW {
			maxLabelW = lw
		}
	}

	w := labelLeft + maxLabelW + rightPad
	if w < overflowPopupDefaultWidth {
		w = overflowPopupDefaultWidth
	}
	// Never wider than the drum pane (less a small margin) so the popup can't
	// escape its bounds on narrow desktop windows.
	if maxW := dv.Bounds.Dx() - 2*SpaceMD; maxW > 0 && w > maxW {
		w = maxW
	}
	return w
}

// overflowPopupRect returns the rectangle for the overflow popup. On mobile
// (when LayoutProfile.UseBottomSheet is true) the popup is rendered as a
// full-width bottom sheet anchored to the bottom of the drum pane —
// DESIGN.md §"Mobile bottom sheets" mandates this for full-width modal
// menus. Desktop keeps the dropdown anchored to the kebab.
func (dv *DrumView) overflowPopupRect() image.Rectangle {
	if dv.overflowBtn() == nil {
		return image.Rectangle{}
	}
	items := dv.overflowItems()
	if Profile().UseBottomSheet {
		// Bottom sheet: full-width, anchored to drum-pane bottom.
		h := len(items) * touchMinTargetPx
		// Cap to drum pane height to avoid overflow above the toolbar.
		maxH := dv.Bounds.Dy() - touchMinTargetPx
		if h > maxH {
			h = maxH
		}
		if h < touchMinTargetPx {
			h = touchMinTargetPx
		}
		return image.Rect(dv.Bounds.Min.X, dv.Bounds.Max.Y-h, dv.Bounds.Max.X, dv.Bounds.Max.Y)
	}

	anchor := dv.overflowBtn().Rect()
	w := dv.overflowPopupWidth(items)
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
//
// iconID, when non-empty, is rendered as a small leading icon by
// drawOverflowMenu. active toggles the icon tint between colFollowActive
// (on, bright azure) and colIncDecIcon (off, neutral) — used by stateful
// entries such as the track/free follow toggle. Plain action entries
// (Upload, Import, Export) leave both fields zero-valued.
type overflowItem struct {
	label   string
	iconID  IconID
	active  bool
	header  bool // true → renders as a non-clickable subgroup header
	onClick func()
}

// overflowItems returns the full list of overflow menu entries.
// Header items (header==true) are non-clickable subgroup labels; they are
// rendered as dimmed section dividers and must NOT appear in the popup's
// button hit list (see overflowPopupBtns).
func (dv *DrumView) overflowItems() []overflowItem {
	if dv.overflowPage == 1 {
		items := []overflowItem{
			{label: i18n.T(i18n.KeyMenuTemplates), header: true},
			{label: i18n.T(i18n.KeyMenuBack), iconID: IconChevronLeft, onClick: func() {
				dv.overflowPage = 0
			}},
		}
		for _, tp := range assets.Templates() {
			tp := tp
			items = append(items, overflowItem{
				label:  tp.Display,
				iconID: IconTemplate,
				onClick: func() {
					dv.overflowPage = 0
					dv.closeOverflowMenu()
					if dv.onImport != nil {
						_ = dv.onImport(tp.Bytes, tp.Display)
					}
				},
			})
		}
		return items
	}
	items := []overflowItem{
		// File group — every action carries a semantic IconID so the
		// menu communicates meaning through iconography (Theme 5 of the
		// mobile UI consistency pass; DESIGN.md single-chrome-accent
		// invariant forbids per-item accent colors).
		{label: i18n.T(i18n.KeyMenuFile), header: true},
		{label: i18n.T(i18n.KeyMenuUpload), iconID: IconUpload, onClick: func() {
			dv.closeOverflowMenu()
			if dv.uploadBtn().OnClick != nil {
				dv.uploadBtn().OnClick()
			}
		}},
		{label: i18n.T(i18n.KeyMenuImport), iconID: IconImport, onClick: func() {
			dv.closeOverflowMenu()
			if dv.importBtn().OnClick != nil {
				dv.importBtn().OnClick()
			}
		}},
		{label: i18n.T(i18n.KeyMenuExport), iconID: IconExport, onClick: func() {
			dv.closeOverflowMenu()
			if dv.exportBtn().OnClick != nil {
				dv.exportBtn().OnClick()
			}
		}},
		{label: i18n.T(i18n.KeyMenuLoadTemplate), iconID: IconTemplate, onClick: func() {
			dv.overflowPage = 1
		}},
	}
	// Track/free follow toggle is exposed inline on both platforms:
	// desktop has the vertical strip left of the timeline; mobile has a
	// chip on the right edge of the timeline ruler header (Theme 2 of
	// the mobile UI consistency pass). No overflow entry needed.
	//
	// Window length +/− is exposed via inline timeline controls; the
	// previous mobile-only overflow duplicate was removed as redundant.
	// With both view-related entries gone, the empty "View" subgroup
	// header was removed as well.
	// Settings is reached exclusively via the gear button in the grid pane's
	// top-right corner on every platform (see gridHelpButtonRect). The overflow
	// menu intentionally carries no Settings entry to avoid duplicating it.
	return items
}

// rebuildOverflowBtns rebuilds the persisted overflow row buttons for the
// current page. Called on open and on page switch (item set changes). Button
// objects persist between rebuilds so their press-animation state survives
// across frames; only their rects are refreshed per frame (see overflowPopupBtns).
// Note: onClick closures are captured per-rebuild; the tp := tp shadowing in
// overflowItems() ensures each template closure binds its own template value.
func (dv *DrumView) rebuildOverflowBtns() {
	items := dv.overflowItems()
	itemStyle := ButtonVisual(DropdownStyle)
	btns := make([]*Button, 0, len(items)+1)
	for _, item := range items {
		if item.header {
			continue // headers are not buttons; rects are advanced in the accessor
		}
		btn := NewButton(item.label, itemStyle, item.onClick)
		btns = append(btns, btn)
	}
	// Close button at top-right (rect set in overflowPopupBtns each frame).
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeOverflowMenu() })
	closeB.Icon = "close"
	closeB.IconColor = closeIconColor()
	closeB.ConsumeOnPress = true
	btns = append(btns, closeB)
	dv.overflowBtns = btns
	dv.overflowBtnsPage = dv.overflowPage
}

// overflowPopupBtns returns the persisted overflow buttons with their rects
// refreshed for the current scroll offset and popup geometry. It rebuilds the
// button set if the count is stale (e.g. first call after open, or after a
// File↔Templates page switch where the item count changed). Safe to call every
// frame from draw and input.
func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	dv.configureOverflowScroll()
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	// Count non-header items; rebuild when the persisted set doesn't match
	// (covers first call and File↔Templates page switches with differing counts).
	want := 0
	for _, item := range items {
		if !item.header {
			want++
		}
	}
	if len(dv.overflowBtns) != want+1 || dv.overflowBtnsPage != dv.overflowPage { // +1 for the close button; also rebuild on page change
		dv.rebuildOverflowBtns()
	}
	// Refresh rects for the current scroll offset.
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	for _, item := range items {
		if item.header {
			curY += rowH
			continue
		}
		r := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		if btnIdx < len(dv.overflowBtns)-1 {
			dv.overflowBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
		}
		btnIdx++
		curY += rowH
	}
	// Close button rect (last element).
	if len(dv.overflowBtns) > 0 {
		dv.overflowBtns[len(dv.overflowBtns)-1].SetRect(closeButtonRect(popupRect, SpaceXS))
	}
	return dv.overflowBtns
}

// drawOverflowMenu renders the overflow popup.
//
// Header items are drawn as dimmed non-interactive section labels.
// Stateful items (those carrying an iconID) get a small leading icon
// overlay drawn after the button so it sits on top of the chrome. The icon
// tint flips with item.active — bright azure when active, neutral otherwise
// — mirroring the inline track-button's icon-tint state model.
func (dv *DrumView) drawOverflowMenu(dst *ebiten.Image) {
	if !dv.IsOverflowMenuOpen() {
		return
	}
	popupRect := dv.overflowPopupRect()
	dv.configureOverflowScroll()
	// Backdrop scrim painted by the OverlayPortal (PortalEntry.Scrim).
	// Mobile renders as a bottom sheet with the shared drag handle so the
	// File sheet's chrome matches the context menu and instrument picker.
	if Profile().UseBottomSheet {
		drawBottomSheetPanel(dst, popupRect)
		drawSheetDragHandle(dst, popupRect)
	} else {
		drawPanel(dst, popupRect)
	}
	// Clip scrolling rows to the popup so offset content can't paint outside.
	clip := dst.SubImage(popupRect).(*ebiten.Image)
	// Build buttons (headers excluded) then close button.
	btns := dv.overflowPopupBtns(popupRect)
	// Draw header rows and action-button rows by iterating items in order,
	// tracking which button corresponds to each non-header item.
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0                  // index into btns (excluding the trailing close button)
	nActionBtns := len(btns) - 1 // last button is always the close button
	for _, item := range items {
		rowR := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		curY += rowH
		if item.header {
			// Unified menu title (shared drawMenuTitle) — no instrument swatch
			// for this global menu. Inset to align with the menu's title gutter.
			titleRect := image.Rect(rowR.Min.X+SpaceMD, rowR.Min.Y, rowR.Max.X-SpaceMD, rowR.Max.Y)
			drawMenuTitle(clip, titleRect, item.label, nil)
			continue
		}
		if btnIdx >= nActionBtns {
			break
		}
		btn := btns[btnIdx]
		btnIdx++

		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		var iconTint color.Color = colMenuIcon
		if item.active {
			iconTint = colFollowActive
		}
		drawMenuRow(clip, btn, MenuRowSpec{
			Accent:   dv.contextMenuAccent(),
			State:    state,
			IconID:   item.iconID,
			IconTint: iconTint,
			Label:    btn.Text,
		})
	}
	// Close button + scrollbar draw on the unclipped dst (pinned chrome).
	if len(btns) > 0 {
		btns[len(btns)-1].Draw(dst)
	}
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.Draw(dst)
	}
}

// initOverflowScroll creates the overflow menu's shared scroll component.
func (dv *DrumView) initOverflowScroll() {
	dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, touchMinTargetPx)
	dv.configureOverflowScroll()
}

// configureOverflowScroll refreshes scroll geometry from the current page's
// items and popup rect. Cheap + idempotent; safe to call every frame. Preserves
// the scroll position across page switches (Configure clamps, not resets).
func (dv *DrumView) configureOverflowScroll() {
	if dv.overflowMenuScroll == nil {
		dv.overflowMenuScroll = NewMenuScroll(dropdownScrollbarStyle(), touchMinTargetPx)
	}
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	popupRect := dv.overflowPopupRect()
	visible := popupRect.Dy() / rowH
	if visible > len(items) {
		visible = len(items)
	}
	if visible < 1 {
		visible = 1
	}
	dv.overflowMenuScroll.Configure(popupRect, len(items), visible)
}

// OverflowMenuScrollForTest exposes the overflow menu's MenuScroll for tests.
func (dv *DrumView) OverflowMenuScrollForTest() *MenuScroll { return dv.overflowMenuScroll }

// closeOverflowMenu centralizes overflow menu teardown, including clearing
// file picker rects registered for mobile gesture-based file picking.
func (dv *DrumView) closeOverflowMenu() {
	dv.overflowPage = 0
	dv.overflowBtns = nil // force rebuild on next open so stale animation state is cleared
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
	dv.closeOverflowMenuPortal()
}

// setViewMode transitions to target. Idempotent: a no-op if already in
// target. Performs full mode-entry side effects (popup close, scroll
// reset, layout invalidation) so any caller — toolbar button, segmented
// control, EQ peek tap — sees identical state afterward.
func (dv *DrumView) setViewMode(target viewMode) {
	if dv.currentViewMode == target {
		return
	}
	dv.currentViewMode = target
	emitViewMode(viewModeSlug(target))
	// Tear down transient per-tab state on every switch (open Save-As
	// modal, in-flight pointer capture) so no departing tab can swallow
	// input destined for the new view. Single chokepoint — see
	// resetTransientTabState.
	dv.resetTransientTabState()
	// EQPanelZone visibility is owned by the tree gate in drumview_ctor.go,
	// which derives from currentViewMode. We only need to sync the active
	// audio sub-tab here; viewModeRows preserves the prior tabState (covered
	// by view_mode_segmented_test.go).
	switch target {
	case viewModeRows:
		// no audio sub-tab change
	case viewModeEQ:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabEQ)
		}
	case viewModeWave:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabWave)
		}
	case viewModeSpectrum:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabSpectrum)
		}
	case viewModeMeters:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabMeters)
		}
	case viewModeChain:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabScope)
		}
	case viewModeSynth:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabSynth)
		}
	case viewModeSampler:
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.tabState.SetActiveTab(TabSampler)
		}
	}
	dv.syncViewSwitchIcon()
	// Sync segmented control's active index with the new mode. Derived from the
	// canonical bottomNavModeList (same list the click handler uses) so the
	// selected segment can never drift from the segment→mode mapping.
	if dv.viewSwitchSegmented != nil {
		if idx := segmentIndexForViewMode(dv.currentViewMode); idx >= 0 {
			dv.viewSwitchSegmented.SetActive(idx)
		}
	}
	// All popups/portals are now torn down unconditionally by
	// resetTransientTabState above (the single chokepoint), so no departing
	// tab can leak an overlay into the new view — including when entering
	// Pads, where MobileEQMode() is false. Only the mobile-EQ scroll reset
	// stays gated to this branch.
	if dv.MobileEQMode() {
		if rs := dv.rowScroll(); rs != nil {
			rs.ResetTouch()
		}
	}
	// The row rack's hit-area *content* depends on MobileEQMode(): while a
	// panel tab owns the mobile screen the rack is hidden (vis=0) and
	// publishes only its `row-rack-scroll` catch-all. recalcButtons' republish
	// gate only fires on NeedsLayout()/rect change, but the rack rect is
	// identical across the mobile panel↔Pads transition and nothing else marks
	// the zone dirty — so without forcing a relayout here the tree keeps
	// serving the stale scroll-only snapshot and the mute/solo/FX/label
	// controls are unreachable after returning to Pads. This mirrors the
	// "RegisterZoneVisible gates BOTH Draw AND HitAreas" discipline: the rack's
	// HitArea content depends on a predicate, so the predicate flip must
	// invalidate it. Regression: row_rack_input_alive_after_panel_test.go.
	if dv.rowRackZone != nil {
		dv.rowRackZone.Invalidate()
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
		if dv.MobileEQMode() {
			dv.eqToggleMobile().Text = "Rows"
		} else {
			dv.eqToggleMobile().Text = "EQ"
		}
	}
}

// wrapOverflowBtn lets tests access the overflow button.
func (dv *DrumView) OverflowBtn() *Button { return dv.overflowBtn() }

// MobileEQCollapsed returns the mobile EQ collapsed state for testing.
func (dv *DrumView) MobileEQCollapsed() bool { return dv.mobileEQCollapsed }

// SetMobileEQCollapsed sets the mobile EQ collapsed state for testing.
func (dv *DrumView) SetMobileEQCollapsed(v bool) { dv.mobileEQCollapsed = v }

// MobileEQMode reports whether the mobile audio panel currently owns the
// drum view (i.e. the user is on EQ/Wave/Spec/Mtr/Scope rather than Pads).
// Derived from currentViewMode + the mobile profile so there is exactly
// one source of truth for the mobile tab swap.
func (dv *DrumView) MobileEQMode() bool {
	return Profile().IsMobile() && dv.currentViewMode != viewModeRows
}

// SetMobileEQMode is the legacy entry point used by tests, screenshot
// scenes, and the Game-level uistate setter. It now routes through the
// canonical setViewMode so the segmented control, tab state, and tree
// visibility gate update in lockstep — avoiding the parallel-state bug
// where SetMobileEQMode flipped a flag without telling the tab system.
func (dv *DrumView) SetMobileEQMode(v bool) {
	if v {
		dv.setViewMode(viewModeEQ)
	} else {
		dv.setViewMode(viewModeRows)
	}
}

// ContextMenuOpen returns whether the context menu is open (for testing).
func (dv *DrumView) ContextMenuOpen() bool { return dv.IsContextMenuOpen() }

// ContextMenuBtns returns the context menu buttons (for testing).
func (dv *DrumView) ContextMenuBtns() []*Button { return dv.contextMenuBtns }

// ContextMenuIconsForTest returns the leading-icon name per button, in
// ContextMenuBtns order (for testing). Derived from the identity-keyed icon map;
// buttons without an icon (e.g. the close button) yield "".
func (dv *DrumView) ContextMenuIconsForTest() []string {
	out := make([]string, len(dv.contextMenuBtns))
	for i, btn := range dv.contextMenuBtns {
		out[i] = dv.contextMenuItemIcons[btn]
	}
	return out
}

// contextMenuRenameBtn returns the open context menu's "Rename" item button,
// identified by its leading icon ("pencil") rather than a positional index.
//
// Why identity-match instead of contextMenuBtns[N]: the items list interleaves
// dividers, but only non-divider items become buttons — so a hard-coded index
// silently drifts onto a neighbouring control whenever an item is inserted
// ahead of Rename. That is exactly how the mobile rename native-input TRIGGER
// ended up registered on the Color item's rect, making a tap on "Color" open
// the rename input. The icon is looked up by button identity (not a parallel
// slice indexed in lockstep), so the match holds regardless of menu ordering or
// slice drift. Returns nil when the menu is closed/absent.
func (dv *DrumView) contextMenuRenameBtn() *Button {
	for _, btn := range dv.contextMenuBtns {
		if dv.contextMenuItemIcons[btn] == "pencil" {
			return btn
		}
	}
	return nil
}

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

// OpenContextMenu opens the row context menu programmatically.
// Used by the screenshot harness, scene catalog, and tests.
func (dv *DrumView) OpenContextMenu(rowIdx int) { dv.openContextMenu(rowIdx) }

// CloseContextMenu closes the row context menu portal.
func (dv *DrumView) CloseContextMenu() {
	if dv.tree != nil {
		dv.portal().Close("context-menu")
	}
}

// OpenOverflowMenu opens the mobile overflow (Upload/Import/Export) menu.
func (dv *DrumView) OpenOverflowMenu() { dv.openOverflowMenuPortal() }

// CloseOverflowMenu closes the mobile overflow menu.
func (dv *DrumView) CloseOverflowMenu() { dv.closeOverflowMenu() }

// StartOverflowImport runs the same action as tapping the "Import" overflow
// menu item: it closes the menu and fires the Import button's handler (which
// drives the platform file picker / pending-pick consumption). It is the
// JS→Go entry point used by the mobile real-input file-picker overlay, whose
// change handler has already stashed the picked file via _fpConsumePending.
// Mirrors the overflowItems() "Import" onClick so the importing flag and the
// "Loaded <file>" notification behave identically to a direct button press.
func (dv *DrumView) StartOverflowImport() {
	dv.closeOverflowMenu()
	if b := dv.importBtn(); b != nil && b.OnClick != nil {
		b.OnClick()
	}
}

// StartOverflowUpload is the Upload (WAV) counterpart of StartOverflowImport.
func (dv *DrumView) StartOverflowUpload() {
	dv.closeOverflowMenu()
	if b := dv.uploadBtn(); b != nil && b.OnClick != nil {
		b.OnClick()
	}
}

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
