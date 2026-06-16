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
			btn.SetRect(insetRect(r, SpaceXS))
			if item.textColor != nil {
				btn.TextColor = item.textColor
			}
			dv.contextMenuBtns = append(dv.contextMenuBtns, btn)
			dv.contextMenuIcons = append(dv.contextMenuIcons, item.icon)
			curY += rowH
		}
	} else {
		// Desktop: positioned popup next to row label, placed via the shared
		// AnchorPopupRect primitive (flip + clamp) so a bottom-row anchor never
		// runs the menu off-screen. Uniform row pitch: dividers add NO extra
		// gap here (visual grouping comes from the group-container backgrounds
		// in drawContextMenu), so every row has the same height.
		var anchor image.Rectangle
		if rowIdx < len(dv.rowLabels()) {
			anchor = dv.rowLabels()[rowIdx].Rect()
		} else {
			anchor = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+100, dv.Bounds.Min.Y+dv.headerH+dv.rowHeight())
		}
		menuW := 160
		menuH := nItems * rowH
		dv.contextMenuRect = AnchorPopupRect(dv.Bounds, anchor, menuW, menuH, PopupRight)

		// Label column starts to the right of the leading icon so labels are
		// left-aligned next to their icons (icon at SpaceMD, label at
		// icon-right + SpaceSM).
		iconSize := IconSizeSM
		labelLeft := dv.contextMenuRect.Min.X + SpaceMD + iconSize + SpaceSM
		curY := dv.contextMenuRect.Min.Y
		for _, item := range items {
			if item.divider {
				continue // no extra pitch: uniform row height on desktop
			}
			r := image.Rect(labelLeft, curY, dv.contextMenuRect.Max.X, curY+rowH)
			btn := NewButton(item.label, item.style, item.onClick)
			btn.SetRect(insetRect(r, SpaceXS))
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

	// Close button at top-right — bottom-sheet (mobile) only. On desktop the
	// first menu row sits at the very top, so a corner × would straddle the
	// rounded corner and collide with the Rename row; desktop dismissal is via
	// click-outside + Esc (popup_click_outside_test.go), which already work.
	if Profile().UseBottomSheet {
		closeR := closeButtonRect(dv.contextMenuRect, SpaceXS)
		closeB := NewButton("", PopupButtonStyle, func() { dv.closeContextMenuPortal() })
		closeB.Icon = "close"
		closeB.IconColor = colButtonBorder
		closeB.SetRect(closeR)
		closeB.ConsumeOnPress = true
		dv.contextMenuBtns = append(dv.contextMenuBtns, closeB)
	}
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
	itemBtnCount := len(dv.contextMenuBtns)
	if Profile().UseBottomSheet {
		itemBtnCount-- // trailing close button
	}
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
		// Desktop: uniform row pitch (no divider gap), labels left-aligned to
		// the right of the icon column — mirror openContextMenu's desktop layout.
		iconSize := IconSizeSM
		labelLeft := dv.contextMenuRect.Min.X + SpaceMD + iconSize + SpaceSM
		curY := dv.contextMenuRect.Min.Y - scrollPx
		for _, item := range items {
			if item.divider {
				continue
			}
			if btnIdx < itemBtnCount {
				r := image.Rect(labelLeft, curY, dv.contextMenuRect.Max.X, curY+rowH)
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
	itemStyle := ButtonVisual(ContextMenuItemStyle)

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
	// Backdrop scrim is painted by the OverlayPortal (PortalEntry.Scrim) so the
	// whole stack dims once; this overlay only draws its own surface.
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
			WithAlpha(genColorBorder, genAlphaScrollbarThumb), handleH/2, true)
	}

	// Draw header text (mobile bottom sheet) with row color dot.
	if Profile().IsMobile() && !dv.contextMenuHeaderRect.Empty() &&
		dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		row := dv.Rows[dv.contextMenuRow]
		// "Neon Horizon" header band across the full header height.
		headerBandRect := image.Rect(
			dv.contextMenuRect.Min.X, dv.contextMenuRect.Min.Y,
			dv.contextMenuRect.Max.X, dv.contextMenuRect.Min.Y+touchMinTargetPx,
		)
		drawMenuHeaderBandAccent(dst, headerBandRect, dv.contextMenuAccent())
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
		// Draw row name offset to right of dot using RolePanelTitle.
		textX := dotX + dotSize + SpaceMD
		th := StyledTextHeight(RolePanelTitle)
		DrawTextStyled(dst, row.Name,
			textX,
			dv.contextMenuHeaderRect.Min.Y+(dv.contextMenuHeaderRect.Dy()-th)/2,
			RolePanelTitle, colTextPrimary)
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

		// Desktop uses uniform row pitch (no divider gap); mobile keeps the
		// SpaceSM group gap between groups. Must mirror the layout walk in
		// openContextMenu / rebuildContextMenuButtons.
		dividerGap := SpaceSM
		if !isMobile {
			dividerGap = 0
		}
		for _, item := range items {
			if item.divider {
				// Emit previous group
				if lastGroup >= 0 {
					drawGroupBG(groupStartY, curY, lastGroup)
				}
				curY += dividerGap // group gap
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

	// drawContextMenuItem renders a single item button with the Vice City
	// menu treatment: drawMenuItemBackground for the hover/rest state,
	// leading icon, and label via DrawTextStyled(RoleBody).
	iconSize := IconSizeSM
	if Profile().IsMobile() {
		iconSize = IconSizeMD
	}
	iconCol := colMenuIcon
	drawContextMenuItem := func(i int, btn *Button) {
		r := btn.Rect()
		if r.Empty() {
			return
		}
		// Per-item hover background.
		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		drawMenuItemBackgroundAccent(dst, r, state, dv.contextMenuAccent())

		// Icon at the menu's left edge (SpaceMD inset), vertically centered.
		if i < len(dv.contextMenuIcons) {
			ico := dv.contextMenuIcons[i]
			if ico != "" {
				ix := r.Min.X + SpaceMD
				if !Profile().UseBottomSheet {
					ix = dv.contextMenuRect.Min.X + SpaceMD
				}
				iy := r.Min.Y + (r.Dy()-iconSize)/2
				iconR := image.Rect(ix, iy, ix+iconSize, iy+iconSize)
				drawContextMenuIcon(dst, iconR, ico, iconCol)
			}
		}

		// Draw button chrome (shadow/glow) without text — null Text temporarily.
		saved := btn.Text
		btn.Text = ""
		btn.Draw(dst)
		btn.Text = saved

		// Label via RoleBody; keep the existing per-item text color (e.g. red for Delete).
		var labelCol color.Color = colTextPrimary
		if btn.TextColor != nil {
			labelCol = btn.TextColor
		}
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(dst, saved, r.Min.X, ty, RoleBody, labelCol)
	}

	// Draw buttons, clipping to the item viewport so off-screen buttons
	// don't render over the header or outside the menu.
	if dv.contextMenuScroll != nil && Profile().IsMobile() {
		itemView := dv.contextMenuScroll.VS.View
		for i, btn := range dv.contextMenuBtns {
			// Always draw the close button (last, lives in header area) via its own path.
			if i == len(dv.contextMenuBtns)-1 {
				btn.Draw(dst)
				continue
			}
			r := btn.Rect()
			if r.Max.Y > itemView.Min.Y && r.Min.Y < itemView.Max.Y {
				drawContextMenuItem(i, btn)
			}
		}
	} else {
		for i, btn := range dv.contextMenuBtns {
			drawContextMenuItem(i, btn)
		}
	}

	// Scrollbar (drawn on top of buttons)
	if dv.contextMenuScroll != nil {
		dv.contextMenuScroll.Draw(dst)
	}
}

// drawContextMenuIcon delegates to the shared icon registry so the same
// glyphs render everywhere (toolbar, row controls, context menu).
func drawContextMenuIcon(dst *ebiten.Image, r image.Rectangle, icon string, col color.Color) {
	if r.Empty() {
		return
	}
	id := IconID(icon)
	if icon == "color" {
		id = IconCircle
	}
	DrawIcon(dst, id, r, col)
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
		// Buttons are rebuilt every frame from the live offset, so no Relayout.
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
			AnchorRect: anchor,
			Bounds:     rackBounds,
			RowHeight:  dv.rowHeight(),
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
	w := 160
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
				iconID: IconRows,
				onClick: func() {
					dv.overflowPage = 0
					dv.closeOverflowMenu()
					if dv.onImport != nil {
						_ = dv.onImport(tp.Bytes)
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
		{label: i18n.T(i18n.KeyMenuLoadTemplate), iconID: IconRows, onClick: func() {
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
	items = append(items, overflowItem{
		label:  i18n.T(i18n.KeySettingsTitle),
		iconID: IconSettings,
		onClick: func() {
			dv.closeOverflowMenu()
			dv.openSettingsOverlay()
		},
	})
	return items
}

// overflowPopupBtns builds the buttons for the overflow popup.
// Header items (item.header==true) are rendered separately by drawOverflowMenu
// as dimmed section labels; they are NOT included in the returned button list
// so taps on them don't fire any action.
func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	dv.configureOverflowScroll()
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	btns := make([]*Button, 0, len(items)+1)
	itemStyle := ButtonVisual(DropdownStyle)
	curY := popupRect.Min.Y - dv.overflowMenuScroll.OffsetPx() // scroll offset
	for _, item := range items {
		if item.header {
			// Headers occupy vertical space but are not buttons.
			curY += rowH
			continue
		}
		r := image.Rect(popupRect.Min.X, curY, popupRect.Max.X, curY+rowH)
		btn := NewButton(item.label, itemStyle, item.onClick)
		btn.SetRect(insetRect(r, SpaceXS))
		btns = append(btns, btn)
		curY += rowH
	}
	// Close button at top-right
	closeR := closeButtonRect(popupRect, SpaceXS)
	closeB := NewButton("", PopupButtonStyle, func() { dv.closeOverflowMenu() })
	closeB.Icon = "close"
	closeB.IconColor = colButtonBorder
	closeB.SetRect(closeR)
	closeB.ConsumeOnPress = true
	btns = append(btns, closeB)
	return btns
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
	drawPanel(dst, popupRect)
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
			// Subgroup header: RoleCaption + colTextSecondary, no fill, no hit area.
			tx := rowR.Min.X + SpaceMD
			ty := rowR.Min.Y + (rowR.Dy()-StyledTextHeight(RoleCaption))/2
			DrawTextStyled(clip, item.label, tx, ty, RoleCaption, colTextSecondary)
			continue
		}
		if btnIdx >= nActionBtns {
			break
		}
		btn := btns[btnIdx]
		btnIdx++

		// Per-item hover background.
		state := menuItemRest
		if btn.hovered || btn.pressed {
			state = menuItemHover
		}
		drawMenuItemBackgroundAccent(clip, btn.Rect(), state, dv.contextMenuAccent())

		// Draw leading icon (from item.iconID).
		if item.iconID != "" {
			r := btn.Rect()
			// Square icon at the leading edge, sized to the row height,
			// inset to leave the label text room to the right of it.
			side := r.Dy() - 2*SpaceXS
			if side < 0 {
				side = 0
			}
			iconR := image.Rect(
				r.Min.X+SpaceSM,
				r.Min.Y+(r.Dy()-side)/2,
				r.Min.X+SpaceSM+side,
				r.Min.Y+(r.Dy()-side)/2+side,
			)
			tint := colIncDecIcon
			if item.active {
				tint = colFollowActive
			}
			DrawIcon(clip, item.iconID, iconR, tint)
		}

		// Draw button chrome (shadow/glow) without text — null Text temporarily.
		saved := btn.Text
		btn.Text = ""
		btn.Draw(clip)
		btn.Text = saved

		// Label via RoleBody.
		r := btn.Rect()
		th := StyledTextHeight(RoleBody)
		ty := r.Min.Y + (r.Dy()-th)/2
		DrawTextStyled(clip, saved, r.Min.X, ty, RoleBody, colTextPrimary)
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
		dv.overflowMenuScroll = NewMenuScroll(DropdownScrollbarStyle, touchMinTargetPx)
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
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
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
	dv.configureOverflowScroll()
	offset := dv.overflowMenuScroll.OffsetPx()
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
	closeR := closeButtonRect(popupRect, SpaceXS)
	for i, item := range dv.overflowItems() {
		fileID, ok := idByLabel[item.label]
		if !ok {
			continue
		}
		y0 := popupRect.Min.Y + i*rowH - offset
		r := insetRect(image.Rect(popupRect.Min.X, y0, popupRect.Max.X, y0+rowH), SpaceXS)
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

// setViewMode transitions to target. Idempotent: a no-op if already in
// target. Performs full mode-entry side effects (popup close, scroll
// reset, layout invalidation) so any caller — toolbar button, segmented
// control, EQ peek tap — sees identical state afterward.
func (dv *DrumView) setViewMode(target viewMode) {
	if dv.currentViewMode == target {
		return
	}
	dv.currentViewMode = target
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
	// Sync segmented control's active index with the new mode.
	if dv.viewSwitchSegmented != nil {
		switch dv.currentViewMode {
		case viewModeRows:
			dv.viewSwitchSegmented.SetActive(0)
		case viewModeEQ:
			dv.viewSwitchSegmented.SetActive(1)
		case viewModeWave:
			dv.viewSwitchSegmented.SetActive(2)
		case viewModeSpectrum:
			dv.viewSwitchSegmented.SetActive(3)
		case viewModeMeters:
			dv.viewSwitchSegmented.SetActive(4)
		case viewModeChain:
			dv.viewSwitchSegmented.SetActive(5)
		case viewModeSynth:
			dv.viewSwitchSegmented.SetActive(6)
		case viewModeSampler:
			dv.viewSwitchSegmented.SetActive(7)
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
		dv.tree.Portal().Close("context-menu")
	}
}

// OpenOverflowMenu opens the mobile overflow (Upload/Import/Export) menu.
func (dv *DrumView) OpenOverflowMenu() { dv.openOverflowMenuPortal() }

// CloseOverflowMenu closes the mobile overflow menu.
func (dv *DrumView) CloseOverflowMenu() { dv.closeOverflowMenu() }

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
