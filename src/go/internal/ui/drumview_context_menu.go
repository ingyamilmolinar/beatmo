package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const templateSearchMobileInputID = "template-search"

// The row's instrument color, so the menu chrome reads as owned by the instrument.
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

		// Not a button, so button indices stay stable for tests and deferred taps.
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
		// Desktop: anchored to the row's kebab, which sits just above the bottom panel, so the menu
		// opens near the pointer. Falls back to the label when the kebab is not laid out.
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

		// Not a button, so button indices stay stable for tests and deferred taps.
		dv.contextMenuHeaderRect = image.Rect(
			dv.contextMenuRect.Min.X+SpaceMD, dv.contextMenuRect.Min.Y+SpaceMD,
			dv.contextMenuRect.Max.X-touchMinTargetPx, dv.contextMenuRect.Min.Y+SpaceMD+headerH-SpaceSM)

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

	// On mobile the viewport excludes the header, so it stays pinned while items scroll.
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

	// Desktop only (nil on mobile, where the sheet closes on an outside tap); appended only when
	// non-nil so itemBtnCount stays correct.
	dv.contextMenuCloseButton = newPopupCloseButton(func() { dv.closeContextMenuPortal() })
	if dv.contextMenuCloseButton != nil {
		dv.contextMenuCloseButton.SetRect(closeButtonRect(dv.contextMenuRect, SpaceXS))
		dv.contextMenuBtns = append(dv.contextMenuBtns, dv.contextMenuCloseButton)
	}
	dv.openContextMenuPortal()
}

func (dv *DrumView) rebuildContextMenuButtons() {
	if dv.contextMenuScroll == nil || !dv.IsContextMenuOpen() {
		return
	}
	rowH := touchMinTargetPx
	scrollPx := dv.contextMenuScroll.VS.First * dv.contextMenuScroll.ItemHeight

	// The close × is a desktop-only trailing button, so discount it only when present.
	items := dv.contextMenuItems(dv.contextMenuRow)
	itemBtnCount := len(dv.contextMenuBtns)
	if dv.contextMenuCloseButton != nil {
		itemBtnCount-- // trailing close button (desktop only)
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

// The same on every platform. Instrument, Effects, Mute and Solo have their own row controls.
func (dv *DrumView) contextMenuItems(rowIdx int) []contextMenuItem {
	// DropdownStyle, so the cap face and press/hover animation match the other menus.
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

// Mobile uses the deferred-tap pattern to tell taps from scrolls.
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

	// Click-outside closing is the tree's job.
	if pt.In(dv.contextMenuRect) {
		return true
	}
	return false
}

// Tracked by identity, never read off contextMenuBtns' tail, so a new trailing button cannot
// silently become "the close button".
func (dv *DrumView) contextMenuCloseBtn() *Button {
	return dv.contextMenuCloseButton
}

// In reverse: the close button is appended last and overlaps the items.
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

func (dv *DrumView) drawContextMenu(dst *ebiten.Image) {
	if !dv.IsContextMenuOpen() {
		return
	}
	// The portal paints the scrim, so a stack dims once; the frame is the shared MenuCard.
	if dv.contextMenuCard == nil {
		dv.contextMenuCard = NewMenuCard(nil)
	}
	spec := MenuCardSpec{Accent: dv.contextMenuAccent(), BottomSheet: Profile().UseBottomSheet}
	if dv.contextMenuRow >= 0 && dv.contextMenuRow < len(dv.Rows) {
		row := dv.Rows[dv.contextMenuRow]
		rowCol := row.Color
		if rowCol == nil {
			rowCol = instColor(row.Instrument)
		}
		spec.Title, spec.Swatch = row.Name, rowCol
	}
	dv.contextMenuCard.SetCloseButton(dv.contextMenuCloseButton)
	dv.contextMenuCard.Layout(dv.contextMenuRect, spec)
	dv.contextMenuCard.Draw(dst)

	// Walks the items with the same pitch as openContextMenu.
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

	drawContextMenuItem := func(i int, btn *Button) {
		if btn.Rect().Empty() {
			return
		}
		state := menuItemRest
		if btn.hovered || btn.pressed || i == dv.contextKeyNav().Selected() {
			// One highlight shared by mouse and keyboard (see menu_keynav.go).
			state = menuItemHover
		}
		var iconID IconID
		if ico := dv.contextMenuItemIcons[btn]; ico != "" {
			iconID = IconID(ico)
			if ico == "color" {
				iconID = IconCircle
			}
		}
		// Preserve per-item label color (e.g. red Delete).
		var labelCol color.Color // nil → colTextPrimary in drawMenuRow
		if tc := btn.effectiveTextColor(); tc != nil {
			labelCol = tc
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

	// Clipped to the item viewport so off-screen buttons never draw over the header.
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

func (dv *DrumView) contextMenuItemGeom(i int) (image.Rectangle, int) {
	if i < 0 || i >= len(dv.contextMenuBtns) {
		return image.Rectangle{}, 0
	}
	r := dv.contextMenuBtns[i].Rect()
	return menuRowIconRect(r), menuRowLabelX(r)
}

func (dv *DrumView) handleOverflowMenuInput(mx, my int, left bool) bool {
	if !dv.IsOverflowMenuOpen() {
		return false
	}
	dv.configureOverflowScroll()
	popupRect := dv.overflowPopupRect()

	// Frame controls (close ×) win first.
	if dv.overflowCard != nil && dv.overflowCard.HandleInput(mx, my, left) != InputIgnored {
		return true
	}
	btns := dv.overflowPopupBtns(popupRect)

	// A press on the clear-× wipes the query; anywhere else in the band focuses the box.
	if left && dv.overflowPage == 1 && dv.templateSearchBox != nil &&
		!dv.templateSearchRect.Empty() && image.Pt(mx, my).In(dv.templateSearchRect) {
		if dv.templateSearchQuery != "" && menuSearchAdornmentsActive(dv.templateSearchBox) &&
			image.Pt(mx, my).In(menuSearchClearRect(dv.templateSearchBox)) {
			dv.setTemplateSearchQuery("")
			return true
		}
		dv.templateSearchBox.SetFocus(true)
		return true
	}

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

// In reverse: the close button is appended last and overlaps the items.
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

	// The swatch column has zero width on mobile.
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

// Meaningful only while IsOverflowMenuOpen().
func (dv *DrumView) OverflowPopupRect() image.Rectangle { return dv.overflowPopupRect() }

const overflowPopupDefaultWidth = 160

func (dv *DrumView) overflowPopupWidth(items []overflowItem) int {
	// Must match drawOverflowMenu: rows inset by SpaceXS, then the icon gutter.
	labelLeft := SpaceXS + (SpaceMD + menuRowIconSize() + SpaceSM)
	rightPad := SpaceMD + SpaceXS + dropdownScrollbarWidth()

	w := labelLeft + dv.overflowMaxLabelWidth(items) + rightPad
	if w < overflowPopupDefaultWidth {
		w = overflowPopupDefaultWidth
	}
	// Never wider than the drum pane, so it cannot escape a narrow window.
	if maxW := dv.Bounds.Dx() - 2*SpaceMD; maxW > 0 && w > maxW {
		w = maxW
	}
	return w
}

// Memoized on the item set and the font: overflowPopupWidth runs several times a frame over
// ~40 long template labels.
func (dv *DrumView) overflowMaxLabelWidth(items []overflowItem) int {
	key := dv.overflowItemsCacheKeyNow()
	fontGen := i18n.FontGeneration()
	if dv.overflowLabelWidthValid && dv.overflowLabelWidthKey == key && dv.overflowLabelWidthFont == fontGen {
		return dv.overflowLabelWidth
	}
	maxLabelW := 0
	for _, it := range items {
		if it.header {
			continue
		}
		if lw := StyledTextWidth(it.label, RoleBody); lw > maxLabelW {
			maxLabelW = lw
		}
	}
	dv.overflowLabelWidth = maxLabelW
	dv.overflowLabelWidthKey = key
	dv.overflowLabelWidthFont = fontGen
	dv.overflowLabelWidthValid = true
	return maxLabelW
}

func (dv *DrumView) overflowPopupRect() image.Rectangle {
	anchor := dv.overflowAnchorRect()
	if anchor.Empty() {
		return image.Rectangle{}
	}
	items := dv.overflowItems()

	// Drops beside its grid-pane button and clamps to the grid pane on every screen size, so it
	// never spills into the drum view (like the node pop-up, Settings and Logs).
	clampBox := dv.Bounds
	if dv.game != nil {
		g := dv.game
		clampBox = image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	}

	w := dv.overflowPopupWidth(items)
	h := len(items) * touchMinTargetPx
	x := anchor.Max.X - w
	y := anchor.Max.Y + 2
	if x < clampBox.Min.X {
		x = clampBox.Min.X
	}
	if x+w > clampBox.Max.X {
		x = clampBox.Max.X - w
	}
	if y+h > clampBox.Max.Y {
		y = anchor.Min.Y - h - 2 // flip above the trigger
	}
	// Clamp so the popup never escapes above the clamp box.
	if y < clampBox.Min.Y {
		y = clampBox.Min.Y
	}
	// Cap height so the popup does not overflow below the clamp box.
	if maxH := clampBox.Max.Y - y; h > maxH {
		h = maxH
		if h < touchMinTargetPx {
			h = touchMinTargetPx
		}
	}
	return image.Rect(x, y, x+w, y+h)
}

// Menus open adjacent to their trigger.
func (dv *DrumView) overflowAnchorRect() image.Rectangle {
	if r := dv.fileMenuAnchorRect(); !r.Empty() {
		return r
	}
	// Fallback until the grid folder button exists: drum-pane top-right.
	return image.Rect(dv.Bounds.Max.X-1, dv.Bounds.Min.Y, dv.Bounds.Max.X, dv.Bounds.Min.Y+1)
}

// Screen space (the grid pane's origin is (0,0)); empty when the button is absent.
func (dv *DrumView) fileMenuAnchorRect() image.Rectangle {
	if dv.game == nil {
		return image.Rectangle{}
	}
	return dv.game.gridFileButtonRect()
}

// Memoized on the query: overflowItems runs several times a frame.
func (dv *DrumView) templateSearchResults() []MenuSearchResult {
	q := strings.TrimSpace(dv.templateSearchQuery)
	if dv.templateSearchCacheValid && dv.templateSearchCacheQuery == q {
		return dv.templateSearchCache
	}
	tps := assets.Templates()
	items := make([]MenuSearchItem, 0, len(tps))
	for _, tp := range tps {
		items = append(items, MenuSearchItem{Key: tp.Genre, Label: tp.Display})
	}
	var searcher MenuSearcher
	dv.templateSearchCache = searcher.Search(q, items)
	dv.templateSearchCacheQuery = q
	dv.templateSearchCacheValid = true
	return dv.templateSearchCache
}

// invalidateTemplateSearch drops the memoized filter cache after a query change.
func (dv *DrumView) invalidateTemplateSearch() { dv.templateSearchCacheValid = false }

// Programmatic changes only: on the typing path SetText would jump the caret to the end.
func (dv *DrumView) setTemplateSearchQuery(q string) {
	if dv.templateSearchQuery == q {
		return
	}
	dv.templateSearchQuery = q
	dv.invalidateTemplateSearch()
	if dv.templateSearchBox != nil {
		dv.templateSearchBox.SetText(q)
	}
}

func (dv *DrumView) updateTemplateSearch() {
	if !dv.IsOverflowMenuOpen() || dv.overflowPage != 1 {
		return
	}
	dv.overflowPopupBtns(dv.overflowPopupRect()) // ensures box + rect exist
	if dv.templateSearchBox == nil {
		return
	}
	// Mobile: value comes from the native <input> overlay, not the ebiten kbd.
	if Profile().IsMobile() {
		if v, ok := mobileInputGetValue(templateSearchMobileInputID); ok && v != dv.templateSearchQuery {
			dv.setTemplateSearchQuery(v)
		}
	}
	dv.templateSearchBox.Update()
	if nv := dv.templateSearchBox.Value(); nv != dv.templateSearchQuery {
		dv.templateSearchQuery = nv
		dv.invalidateTemplateSearch()
		emitSearchChanged("template-menu", nv)
	}
}

// false on an empty query, so the universal Esc handler closes the menu.
func (dv *DrumView) handleTemplateSearchEscape() bool {
	if dv.overflowPage != 1 || strings.TrimSpace(dv.templateSearchQuery) == "" {
		return false
	}
	dv.setTemplateSearchQuery("")
	return true
}

// Only the LEADING header/search run is pinned; mid-list group headers scroll with the rows.
func overflowPinnedCount(items []overflowItem) int {
	n := 0
	for _, it := range items {
		if it.header || it.search {
			n++
			continue
		}
		break
	}
	return n
}

// active toggles a stateful entry's icon tint; plain actions leave iconID empty.
type overflowItem struct {
	label      string
	iconID     IconID
	active     bool   // row renders menuItemActive (stripe + tint) — e.g. the currently-loaded template
	header     bool   // true → non-clickable header; pinned title when leading, scrolling group header mid-list
	search     bool   // true → renders the pinned fuzzy-search box (no button)
	highlights []int  // fuzzy-match highlight rune indices for this row's label
	trail      string // optional right-aligned muted caption (e.g. "120 BPM")
	onClick    func()
}

// Everything the item model depends on: two calls with an equal key MUST produce an equal list.
// locale is included because the labels go through i18n.T.
type overflowItemsKey struct {
	page   int
	query  string
	loaded string
	locale i18n.Locale
}

// overflowItemsCacheKeyNow snapshots the current model state.
func (dv *DrumView) overflowItemsCacheKeyNow() overflowItemsKey {
	return overflowItemsKey{
		page:   dv.overflowPage,
		query:  dv.templateSearchQuery,
		loaded: dv.templateLoadedStem,
		locale: i18n.ActiveLocale(),
	}
}

// Memoized on overflowItemsKey: it runs about a dozen times a frame, and rebuilding the template
// page each time dropped the frame rate. The returned slice is shared: treat it as read-only.
func (dv *DrumView) overflowItems() []overflowItem {
	key := dv.overflowItemsCacheKeyNow()
	if dv.overflowItemsCacheValid && dv.overflowItemsCacheKey == key {
		return dv.overflowItemsCache
	}
	items := dv.buildOverflowItems()
	dv.overflowItemsCache = items
	dv.overflowItemsCacheKey = key
	dv.overflowItemsCacheValid = true
	return items
}

// The uncached body; call overflowItems().
func (dv *DrumView) buildOverflowItems() []overflowItem {
	if dv.overflowPage == 1 {
		items := []overflowItem{
			{label: i18n.T(i18n.KeyMenuTemplates), header: true},
			{search: true},
			{label: i18n.T(i18n.KeyMenuBack), iconID: IconChevronLeft, onClick: func() {
				dv.setTemplateSearchQuery("")
				dv.overflowPage = 0
			}},
		}
		byGenre := make(map[string]assets.Template, len(assets.Templates()))
		for _, tp := range assets.Templates() {
			byGenre[tp.Genre] = tp
		}
		// An active search flattens the results by score, so no group headers.
		grouped := strings.TrimSpace(dv.templateSearchQuery) == ""
		lastGroup := ""
		for _, r := range dv.templateSearchResults() {
			tp, ok := byGenre[r.Key]
			if !ok {
				continue
			}
			if grouped && tp.Group != "" && tp.Group != lastGroup {
				items = append(items, overflowItem{label: tp.Group, header: true})
				lastGroup = tp.Group
			}
			hl := r.Highlights
			trail := ""
			if tp.BPM > 0 {
				trail = strconv.Itoa(tp.BPM) + " BPM"
			}
			items = append(items, overflowItem{
				label:      tp.Display,
				iconID:     IconTemplate,
				highlights: hl,
				trail:      trail,
				active:     tp.Genre == dv.templateLoadedStem,
				onClick: func() {
					dv.templateLoadedStem = tp.Genre
					dv.setTemplateSearchQuery("")
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
		// Icons, never per-item accent colors: the design allows one chrome accent.
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
	// Follow, window length and Settings have their own controls, so the menu does not repeat them.
	return items
}

// Buttons persist across rebuilds so their press animation survives; only rects refresh per frame.
func (dv *DrumView) rebuildOverflowBtns() {
	items := dv.overflowItems()
	itemStyle := ButtonVisual(DropdownStyle)
	btns := make([]*Button, 0, len(items))
	for _, item := range items {
		if item.header || item.search {
			continue // headers are not buttons; rects are advanced in the accessor
		}
		btn := NewButton(item.label, itemStyle, item.onClick)
		btns = append(btns, btn)
	}
	// No close (×) button — the folder button toggles this menu closed.
	dv.overflowBtns = btns
	dv.overflowBtnsPage = dv.overflowPage
}

func (dv *DrumView) overflowPopupBtns(popupRect image.Rectangle) []*Button {
	dv.configureOverflowScroll()
	rowH := touchMinTargetPx
	items := dv.overflowItems()
	want := 0
	for _, item := range items {
		if !item.header && !item.search {
			want++
		}
	}
	if len(dv.overflowBtns) != want || dv.overflowBtnsPage != dv.overflowPage { // rebuild on count/page change
		dv.rebuildOverflowBtns()
	}
	pinned := overflowPinnedCount(items)
	bandH := pinned * rowH
	pinnedY := popupRect.Min.Y
	scrollY := popupRect.Min.Y + bandH - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	seen := 0
	for _, item := range items {
		if seen < pinned {
			r := image.Rect(popupRect.Min.X, pinnedY, popupRect.Max.X, pinnedY+rowH)
			pinnedY += rowH
			seen++
			if item.search {
				dv.templateSearchRect = r
				freshBox := dv.templateSearchBox == nil
				dv.templateSearchBox = ensureMenuSearchBox(dv.templateSearchBox, r, dv.textFields,
					TextFieldSpec{ID: templateSearchMobileInputID, Owner: "overflow-menu"})
				// Seed only a fresh box: resyncing every call would stomp a same-frame typed edit before
				// updateTemplateSearch reads it.
				if freshBox && dv.templateSearchBox.Value() != dv.templateSearchQuery {
					dv.templateSearchBox.SetText(dv.templateSearchQuery)
				}
			}
			continue
		}
		r := image.Rect(popupRect.Min.X, scrollY, popupRect.Max.X, scrollY+rowH)
		scrollY += rowH
		if item.header {
			continue // mid-list group header: occupies a row, owns no button
		}
		if btnIdx < len(dv.overflowBtns) {
			dv.overflowBtns[btnIdx].SetRect(insetRect(r, SpaceXS))
		}
		btnIdx++
	}
	return dv.overflowBtns
}

func (dv *DrumView) drawOverflowMenu(dst *ebiten.Image) {
	if !dv.IsOverflowMenuOpen() {
		return
	}
	popupRect := dv.overflowPopupRect()
	dv.configureOverflowScroll()
	// A floating card on every screen size: the File menu anchors in the grid pane, so no bottom sheet.
	items := dv.overflowItems()
	if dv.overflowCard == nil {
		dv.overflowCard = NewMenuCard(dv.closeOverflowMenu)
	}
	title := ""
	for _, it := range items {
		if it.header {
			title = it.label
			break
		}
	}
	dv.overflowCard.Layout(popupRect, MenuCardSpec{Title: title, IconID: IconFolder})
	dv.overflowCard.Draw(dst)
	rowH := touchMinTargetPx
	pinned := overflowPinnedCount(items)
	bandH := pinned * rowH
	// Refresh button rects + templateSearchRect/box for this frame.
	btns := dv.overflowPopupBtns(popupRect)

	// Scrolling rows are clipped to the region below the pinned band.
	scrollRegion := popupRect
	scrollRegion.Min.Y += bandH
	if scrollRegion.Min.Y > scrollRegion.Max.Y {
		scrollRegion.Min.Y = scrollRegion.Max.Y
	}
	clip := dst.SubImage(scrollRegion).(*ebiten.Image)

	// 1. Pinned band (fixed Y, unclipped chrome), offset below the nav panel.
	pinnedY := popupRect.Min.Y
	for i := 0; i < pinned && i < len(items); i++ {
		item := items[i]
		rowR := image.Rect(popupRect.Min.X, pinnedY, popupRect.Max.X, pinnedY+rowH)
		pinnedY += rowH
		if item.header {
			continue // drawn by the MenuCard title band above
		}
		if item.search {
			// A global menu takes the neutral chrome accent, never an instrument color.
			drawMenuHeaderBandAccent(dst, rowR, colAccent)
			drawMenuSearchBox(dst, dv.templateSearchBox)
		}
	}

	// Rows advance exactly like overflowPopupBtns so headers stay aligned with the buttons. Only rows
	// inside scrollRegion are drawn: the template page has ~40 rows for ~6 visible.
	scrollY := popupRect.Min.Y + bandH - dv.overflowMenuScroll.OffsetPx()
	btnIdx := 0
	// No trailing chrome button: the folder button toggles the menu closed.
	nActionBtns := len(btns)
	for i := pinned; i < len(items); i++ {
		item := items[i]
		rowR := image.Rect(popupRect.Min.X, scrollY, popupRect.Max.X, scrollY+rowH)
		scrollY += rowH
		if rowR.Min.Y >= scrollRegion.Max.Y {
			break
		}
		visible := rowR.Max.Y > scrollRegion.Min.Y
		if item.header {
			if visible {
				drawMenuGroupHeader(clip, rowR, item.label)
			}
			continue
		}
		if btnIdx >= nActionBtns {
			break
		}
		btn := btns[btnIdx]
		btnIdx++
		if !visible {
			continue // scrolled above the viewport
		}
		state := menuItemRest
		if item.active {
			// The currently-loaded template reads as the list's selection.
			state = menuItemActive
		} else if btn.hovered || btn.pressed || i == dv.overflowKeyNav().Selected() {
			// One highlight shared by mouse and keyboard (see menu_keynav.go).
			state = menuItemHover
		}
		var iconTint color.Color = colMenuIcon
		if item.active {
			iconTint = colFollowActive
		}
		drawMenuRow(clip, btn, MenuRowSpec{
			Accent:     colAccent,
			State:      state,
			IconID:     item.iconID,
			IconTint:   iconTint,
			Highlights: item.highlights,
			Label:      btn.Text,
			TrailLabel: item.trail,
		})
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

// Idempotent; Configure clamps rather than resets, so the scroll survives a page switch.
func (dv *DrumView) configureOverflowScroll() {
	if dv.overflowMenuScroll == nil {
		dv.overflowMenuScroll = NewMenuScroll(dropdownScrollbarStyle(), touchMinTargetPx)
	}
	items := dv.overflowItems()
	rowH := touchMinTargetPx
	popupRect := dv.overflowPopupRect()
	bandH := overflowPinnedCount(items) * rowH
	scrollRegion := popupRect
	scrollRegion.Min.Y += bandH
	if scrollRegion.Min.Y > scrollRegion.Max.Y {
		scrollRegion.Min.Y = scrollRegion.Max.Y
	}
	scrollCount := len(items) - overflowPinnedCount(items)
	visible := scrollRegion.Dy() / rowH
	if visible > scrollCount {
		visible = scrollCount
	}
	if visible < 1 {
		visible = 1
	}
	dv.overflowMenuScroll.Configure(scrollRegion, scrollCount, visible)
}

// OverflowMenuScrollForTest exposes the overflow menu's MenuScroll for tests.
func (dv *DrumView) OverflowMenuScrollForTest() *MenuScroll { return dv.overflowMenuScroll }

func (dv *DrumView) closeOverflowMenu() {
	dv.overflowPage = 0
	dv.overflowBtns = nil // force rebuild on next open so stale animation state is cleared
	dv.setTemplateSearchQuery("")
	if dv.templateSearchBox != nil {
		dv.templateSearchBox.SetFocus(false)
	}
	if dv.overflowMenuScroll != nil {
		dv.overflowMenuScroll.DeferredTap().Cancel()
	}
	dv.closeOverflowMenuPortal()
}

// Idempotent. Every caller gets the same mode-entry side effects.
func (dv *DrumView) setViewMode(target viewMode) {
	if dv.currentViewMode == target || !viewModeAvailable(target) {
		return
	}
	dv.currentViewMode = target
	emitViewMode(viewModeSlug(target))
	dv.resetTransientTabState()
	// The tree gate owns EQPanelZone visibility; viewModeRows keeps the previous tab state.
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
	// Derived from bottomNavModeList, so the selected segment cannot drift from the mapping.
	if dv.viewSwitchSegmented != nil {
		if idx := segmentIndexForViewMode(dv.currentViewMode); idx >= 0 {
			dv.viewSwitchSegmented.SetActive(idx)
		}
	}
	if dv.MobileEQMode() {
		if rs := dv.rowScroll(); rs != nil {
			rs.ResetTouch()
		}
	}
	// The rack's hit areas depend on MobileEQMode() but its rect does not change across the switch,
	// so force a relayout or Pads keeps the stale scroll-only hit areas.
	if dv.rowRackZone != nil {
		dv.rowRackZone.Invalidate()
	}
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()
	dv.markAllRowsDirty()
	dv.rowsLayerDirty = true
}

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

// MobileEQCollapsed returns the mobile EQ collapsed state for testing.
func (dv *DrumView) MobileEQCollapsed() bool { return dv.mobileEQCollapsed }

// SetMobileEQCollapsed sets the mobile EQ collapsed state for testing.
func (dv *DrumView) SetMobileEQCollapsed(v bool) { dv.mobileEQCollapsed = v }

// Derived from currentViewMode, so there is one source of truth for the mobile tab swap.
func (dv *DrumView) MobileEQMode() bool {
	return Profile().IsMobile() && dv.currentViewMode != viewModeRows
}

// The negation of MobileEQMode on mobile, deliberately the same predicate that gates the EQ panel.
func (dv *DrumView) gridPaneVisibleForToast() bool {
	return !dv.MobileEQMode()
}

// Routes through setViewMode so the tab state, nav and tree gate move together.
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

func (dv *DrumView) ContextMenuIconsForTest() []string {
	out := make([]string, len(dv.contextMenuBtns))
	for i, btn := range dv.contextMenuBtns {
		out[i] = dv.contextMenuItemIcons[btn]
	}
	return out
}

// Matched by icon identity, never a positional index: dividers are not buttons, so an index
// drifts onto a neighbour when an item is inserted.
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

func (dv *DrumView) SetOverflowMenuOpen(v bool) {
	if v {
		dv.openOverflowMenuPortal()
	} else {
		dv.closeOverflowMenuPortal()
	}
}

// RowMenuBtns returns the per-row kebab menu buttons (for testing).
func (dv *DrumView) RowMenuBtns() []*Button { return dv.rowMenuBtns() }

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

// Same as tapping Import, so the importing flag and notification behave identically. The mobile
// file-picker overlay calls it after stashing the file.
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

// The full width the timeline gives the grid, unlike timelineRect, which header chrome narrows.
func (dv *DrumView) CellGridRect() image.Rectangle { return dv.cellGridRect }

// ContextMenuScrollForTest returns the context menu scroll behavior for testing.
func (dv *DrumView) ContextMenuScrollForTest() *ScrollBehavior { return dv.contextMenuScroll }
