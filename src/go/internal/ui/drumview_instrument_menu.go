package ui

import (
	"fmt"
	"image"
	"os"
	"path"
	"slices"
	"strings"
)

func (dv *DrumView) instMenuHasScroll() bool {
	return dv.instMenuScroll.HasScroll()
}

func (dv *DrumView) instMenuThumbRect() image.Rectangle {
	if !dv.instMenuOpen || !dv.instMenuHasScroll() || dv.instMenuScroll.View.Empty() {
		return image.Rect(0, 0, 0, 0)
	}
	return dv.instMenuScroll.ThumbRect(instMenuScrollBarWidth, dv.rowHeight()/2)
}

// instDisplayLabel returns a user-facing name for an instrument ID, preferring
// catalog metadata when available. Uses a cache to avoid expensive string ops
// every frame.
func (dv *DrumView) instDisplayLabel(id string) string {
	if id == "" {
		return "(missing)"
	}
	// Fast path: return cached label if available.
	if dv.instLabelCache != nil {
		if lbl, ok := dv.instLabelCache[id]; ok {
			return lbl
		}
	}
	// Compute label.
	lbl := dv.computeInstLabel(id)
	// Cache result.
	if dv.instLabelCache == nil {
		dv.instLabelCache = make(map[string]string)
	}
	dv.instLabelCache[id] = lbl
	return lbl
}

// computeInstLabel computes the display label for an instrument ID without caching.
func (dv *DrumView) computeInstLabel(id string) string {
	if dv.instMeta != nil {
		if meta, ok := dv.instMeta[id]; ok {
			if meta.Name != "" {
				if strings.EqualFold(meta.Name, id) {
					return meta.Name
				}
				return fmt.Sprintf("%s (%s)", meta.Name, id)
			}
			if meta.RelPath != "" {
				base := path.Base(meta.RelPath)
				base = strings.TrimSuffix(base, path.Ext(base))
				if base != "" {
					return strings.ToUpper(base[:1]) + base[1:]
				}
			}
		}
	}
	if len(id) == 1 {
		return strings.ToUpper(id)
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func (dv *DrumView) matchInstrumentSearch(id, q string) bool {
	if q == "" {
		return true
	}
	idLower := strings.ToLower(id)
	if strings.Contains(idLower, q) {
		return true
	}
	label := strings.ToLower(dv.instDisplayLabel(id))
	if strings.Contains(label, q) {
		return true
	}
	if dv.instMeta != nil {
		if meta, ok := dv.instMeta[id]; ok {
			if meta.RelPath != "" && strings.Contains(strings.ToLower(meta.RelPath), q) {
				return true
			}
		}
	}
	return false
}

// buildInstMenu rebuilds the instrument dropdown buttons for the selected row.
func (dv *DrumView) buildInstMenu() {
	dv.instMenuBtns = dv.instMenuBtns[:0]
	if dv.instMenuRow < 0 || dv.instMenuRow >= len(dv.rowLabels) {
		dv.instMenuScroll.View = image.Rect(0, 0, 0, 0)
		return
	}
	if dv.instMenuMode == instMenuModeUnset {
		dv.instMenuMode = instMenuModeCategories
	}
	if dv.instMenuMode == instMenuModeCategories && len(dv.instCategories) == 0 {
		// Gracefully fall back to instruments when there are no catalog
		// categories so the dropdown is never empty.
		dv.instMenuMode = instMenuModeInstruments
	}
	curInst := ""
	if dv.instMenuRow >= 0 && dv.instMenuRow < len(dv.Rows) {
		curInst = dv.Rows[dv.instMenuRow].Instrument
	}
	// Default active category to the row instrument’s category when unset.
	if dv.instMenuActiveCat == "" && curInst != "" {
		if cat, ok := dv.instCatByID[curInst]; ok {
			dv.instMenuActiveCat = cat
		}
	}
	base := dv.rowLabels[dv.instMenuRow].Rect()
	host := dv.widgetRects[WidgetRack]
	if host.Empty() {
		host = image.Rect(dv.Bounds.Min.X, dv.Bounds.Min.Y+dv.headerH, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
	}
	vertBounds := host
	if vertBounds.Empty() {
		vertBounds = dv.Bounds
	}
	// Widen popup to fit long labels; keep within rack.
	minMenuW := dv.labelW + dv.controlsW/2
	if minMenuW < 260 {
		minMenuW = 260
	}
	if minMenuW > host.Dx() {
		minMenuW = host.Dx()
	}
	if base.Dx() < minMenuW {
		base = image.Rect(base.Min.X, base.Min.Y, base.Min.X+minMenuW, base.Max.Y)
	}
	rowH := dv.rowHeight()
	if rowH < 1 {
		rowH = 1
	}
	catCount := len(dv.instCategories)
	// Decide direction using overall bounds to prevent collapse in short rack views.
	spaceDown := dv.Bounds.Max.Y - base.Max.Y
	spaceUp := base.Min.Y - dv.Bounds.Min.Y
	openUp := spaceDown < spaceUp

	filtered := dv.instOptions
	listStartY := 0
	vis := instMenuMaxVisibleRows
	if vis < 1 {
		vis = 1
	}
	if dv.instSearchBox == nil {
		dv.instSearchBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
		dv.instSearchBox.MaxLen = 40
		dv.instSearchBox.SetText(dv.instSearch)
	}
	showBack := dv.instMenuMode == instMenuModeInstruments && len(dv.instCategories) > 0
	startY := 0
	if dv.instMenuMode == instMenuModeInstruments {
		if dv.instMenuActiveCat != "" {
			out := filtered[:0]
			for _, id := range filtered {
				if dv.instCatByID[id] == dv.instMenuActiveCat {
					out = append(out, id)
				}
			}
			filtered = out
		}
		if q := strings.TrimSpace(strings.ToLower(dv.instSearch)); q != "" {
			out := filtered[:0]
			for _, id := range filtered {
				if dv.matchInstrumentSearch(id, q) {
					out = append(out, id)
				}
			}
			filtered = out
		}
		extraRows := 1 // search row
		if showBack {
			extraRows++ // back row
		}
		// Cap by available entries and host height (extra rows + visible instruments must fit the rack).
		maxVisHost := vertBounds.Dy()/rowH - extraRows
		if maxVisHost < 1 {
			maxVisHost = 1
		}
		wantMinVis := 2
		if vis > maxVisHost {
			vis = maxVisHost
		}
		if vis > len(filtered) {
			vis = len(filtered)
		}
		if vis < wantMinVis && maxVisHost >= wantMinVis && len(filtered) >= wantMinVis {
			vis = wantMinVis
		}
		if vis > instMenuMaxVisibleRows {
			vis = instMenuMaxVisibleRows
		}
		if vis < 1 {
			vis = 1
		}
		totalH := (vis + extraRows) * rowH
		startY = base.Max.Y
		if openUp {
			startY = base.Min.Y - totalH
		}
		if startY < vertBounds.Min.Y {
			startY = vertBounds.Min.Y
		}
		if startY+totalH > vertBounds.Max.Y {
			startY = vertBounds.Max.Y - totalH
		}
		searchY := startY
		if showBack {
			searchY += rowH
		}
		searchRect := image.Rect(base.Min.X, searchY, base.Max.X, searchY+rowH)
		dv.instSearchRect = searchRect
		dv.instSearchBox.Rect = insetRect(searchRect, buttonPad)
		listStartY = searchY + rowH
		dv.instMenuScroll.Total = len(filtered)
		dv.instMenuScroll.Visible = vis
		if len(filtered) == 0 {
			emptyView := image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+rowH)
			dv.instMenuScroll.View = emptyView
			dv.instMenuFullRect = image.Rect(base.Min.X, startY, base.Max.X, startY+rowH*2)
			placeholder := NewButton("No matches", DisabledButtonStyle, nil)
			placeholder.SetRect(insetRect(emptyView, buttonPad))
			dv.instMenuBtns = append(dv.instMenuBtns, placeholder)
			return
		}
		dv.instMenuScroll.View = image.Rect(base.Min.X, listStartY, base.Max.X, listStartY+vis*rowH)
		dv.instMenuFullRect = image.Rect(base.Min.X, startY, base.Max.X, listStartY+vis*rowH)
	} else { // categories mode
		if vis > catCount {
			vis = catCount
		}
		if vis < 1 {
			vis = 1
		}
		// Respect rack height so the popup does not overlap other widgets.
		maxVisHost := vertBounds.Dy() / rowH
		if maxVisHost < 1 {
			maxVisHost = 1
		}
		if vis > maxVisHost {
			vis = maxVisHost
		}
		if vis > instMenuMaxVisibleRows {
			vis = instMenuMaxVisibleRows
		}
		totalH := vis * rowH
		startY = base.Max.Y
		if openUp {
			startY = base.Min.Y - totalH
		}
		if startY < host.Min.Y {
			startY = host.Min.Y
		}
		if startY+totalH > host.Max.Y {
			startY = host.Max.Y - totalH
		}
		dv.instMenuScroll.Total = catCount
		dv.instMenuScroll.Visible = vis
		dv.instMenuScroll.View = image.Rect(base.Min.X, startY, base.Max.X, startY+totalH)
		dv.instMenuFullRect = dv.instMenuScroll.View
	}
	// Clamp popup to rack vertically; shift view together.
	if !vertBounds.Empty() {
		shiftY := 0
		if dv.instMenuFullRect.Min.Y < vertBounds.Min.Y {
			shiftY = vertBounds.Min.Y - dv.instMenuFullRect.Min.Y
		} else if dv.instMenuFullRect.Max.Y > vertBounds.Max.Y {
			shiftY = vertBounds.Max.Y - dv.instMenuFullRect.Max.Y
		}
		if shiftY != 0 {
			dv.instMenuFullRect = dv.instMenuFullRect.Add(image.Pt(0, shiftY))
			dv.instMenuScroll.View = dv.instMenuScroll.View.Add(image.Pt(0, shiftY))
			if dv.instMenuMode == instMenuModeInstruments {
				dv.instSearchRect = dv.instSearchRect.Add(image.Pt(0, shiftY))
			}
		}
	}
	// Bias viewport to show the most recently added instrument when present, unless the user already scrolled.
	if !dv.instMenuUserScrolled && dv.instMenuMode == instMenuModeInstruments && dv.instMenuLastAdded != "" {
		if debugInst := os.Getenv("TUNKUL_DEBUG_INST") == "1"; debugInst {
			fmt.Printf("[instMenu/bias] lastAdded=%s idx=%d in=%v\n", dv.instMenuLastAdded, slices.Index(filtered, dv.instMenuLastAdded), slices.Contains(filtered, dv.instMenuLastAdded))
		}
		if idx := slices.Index(filtered, dv.instMenuLastAdded); idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			dv.instMenuScroll.First = first
		}
	}
	// Bias categories view to keep the active category visible when opening.
	if !dv.instMenuUserScrolled && dv.instMenuMode == instMenuModeCategories && dv.instMenuActiveCat != "" {
		idx := -1
		if dv.instMenuActiveCat != fallbackInstCategory {
			idx = slices.Index(dv.instCategories, dv.instMenuActiveCat)
		}
		if os.Getenv("TUNKUL_DEBUG_INST") == "1" {
			fmt.Printf("[instMenu/cat-bias] active=%q idx=%d vis=%d\n", dv.instMenuActiveCat, idx, vis)
		}
		if idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			dv.instMenuScroll.First = first
		}
	}
	dv.instMenuScroll.Clamp()
	// Bias viewport to keep the current instrument visible unless the user scrolled manually.
	if !dv.instMenuUserScrolled && curInst != "" && dv.instMenuLastAdded == "" && dv.instMenuMode == instMenuModeInstruments {
		if idx := slices.Index(filtered, curInst); idx >= 0 {
			first := idx - vis + 1
			if first < 0 {
				first = 0
			}
			dv.instMenuScroll.First = first
			dv.instMenuScroll.Clamp()
		}
	}
	if dv.instMenuMode == instMenuModeInstruments {
		dv.instMenuLastAdded = ""
	}
	hasScroll := dv.instMenuHasScroll()
	buttonMaxX := base.Max.X
	if hasScroll {
		buttonMaxX -= instMenuScrollBarWidth
		if buttonMaxX <= base.Min.X {
			buttonMaxX = base.Min.X + 1
		}
	}
	dv.instCategoryBtns = dv.instCategoryBtns[:0]
	dv.instMenuBtns = dv.instMenuBtns[:0]
	if dv.instMenuMode == instMenuModeCategories {
		if os.Getenv("TUNKUL_DEBUG_INST") == "1" {
			fmt.Printf("[instMenu/cats] first=%d vis=%d total=%d active=%q scrolled=%v cats=%v\n", dv.instMenuScroll.First, vis, dv.instMenuScroll.Total, dv.instMenuActiveCat, dv.instMenuUserScrolled, dv.instCategories)
		}
		start := dv.instMenuScroll.First
		for i := 0; i < vis && start+i < len(dv.instCategories); i++ {
			cat := dv.instCategories[start+i]
			r := image.Rect(base.Min.X, startY+i*dv.rowHeight(), base.Max.X, startY+(i+1)*dv.rowHeight())
			btnCat := cat
			btn := NewButton(btnCat, DropdownStyle, func() {
				dv.instMenuActiveCat = btnCat
				dv.instMenuMode = instMenuModeInstruments
				dv.instMenuScroll.First = 0
				dv.instMenuCameFromCategories = true
				dv.instMenuUserScrolled = false
				dv.buildInstMenu()
				SuppressClicksUntilMouseUp()
			})
			if btnCat == dv.instMenuActiveCat {
				btn.Style = PopupButtonStyle
			}
			btn.SetRect(insetRect(r, buttonPad))
			dv.instCategoryBtns = append(dv.instCategoryBtns, btn)
		}
	} else { // instruments mode
		instStartY := listStartY
		if showBack {
			backRect := image.Rect(base.Min.X, startY, base.Max.X, startY+dv.rowHeight())
			backBtn := NewButton("Back", DropdownStyle, func() {
				dv.instMenuMode = instMenuModeCategories
				dv.instMenuScroll.First = 0
				dv.instMenuCameFromCategories = false
				dv.instMenuUserScrolled = false
				dv.buildInstMenu()
				SuppressClicksUntilMouseUp()
			})
			backBtn.SetRect(insetRect(backRect, buttonPad))
			dv.instMenuBtns = append(dv.instMenuBtns, backBtn)
		}
		// Search box row is rendered separately in renderRowControlOverlay.
		// Search row is drawn/handled separately; reserve its height.
		for i := 0; i < vis && dv.instMenuScroll.First+i < len(filtered); i++ {
			id := filtered[dv.instMenuScroll.First+i]
			var r image.Rectangle
			r = image.Rect(base.Min.X, instStartY+i*dv.rowHeight(), buttonMaxX, instStartY+(i+1)*dv.rowHeight())
			if r.Min.Y < vertBounds.Min.Y {
				r = image.Rect(r.Min.X, vertBounds.Min.Y, r.Max.X, vertBounds.Min.Y+dv.rowHeight())
			}
			if r.Max.Y > vertBounds.Max.Y {
				r = image.Rect(r.Min.X, vertBounds.Max.Y-dv.rowHeight(), r.Max.X, vertBounds.Max.Y)
			}
			optID := id
			label := dv.instDisplayLabel(id)
			btn := NewButton(label, DropdownStyle, func() {
				dv.SetInstrument(optID)
				dv.instMenuOpen = false
				// Close component as well to keep state in sync
				if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() {
					dv.instMenuComp.Close()
				}
			})
			btn.SetRect(insetRect(r, buttonPad))
			if os.Getenv("TUNKUL_DEBUG_INST") == "1" {
				fmt.Printf("[instMenu/btn] %s rect=%v\n", label, btn.Rect())
			}
			dv.instMenuBtns = append(dv.instMenuBtns, btn)
		}
	}
	if os.Getenv("TUNKUL_DEBUG_INST") == "1" && dv.instMenuMode == instMenuModeInstruments {
		var labels []string
		for _, b := range dv.instMenuBtns {
			labels = append(labels, b.Text)
		}
		fmt.Printf("[instMenu/build] first=%d vis=%d total=%d labels=%v\n", dv.instMenuScroll.First, dv.instMenuScroll.Visible, dv.instMenuScroll.Total, labels)
	}
}

// syncInstMenuCompScroll syncs the component's scroll state from the legacy state.
// This ensures the component's thumb position matches after legacy wheel handling.
func (dv *DrumView) syncInstMenuCompScroll() {
	if dv.instMenuComp == nil || !dv.instMenuComp.IsOpen() {
		return
	}
	dv.instMenuComp.SetScrollFirst(dv.instMenuScroll.First)
}

// syncInstMenuBtnsFromComp syncs the legacy instMenuBtns from the component's buttons.
// This ensures tests that access dv.instMenuBtns get the component's actual buttons.
func (dv *DrumView) syncInstMenuBtnsFromComp() {
	if dv.instMenuComp == nil {
		return
	}
	// Build the legacy buttons list from the component's buttons
	dv.instMenuBtns = dv.instMenuBtns[:0]
	if backBtn := dv.instMenuComp.BackBtn(); backBtn != nil {
		dv.instMenuBtns = append(dv.instMenuBtns, backBtn)
	}
	for _, btn := range dv.instMenuComp.InstBtns() {
		dv.instMenuBtns = append(dv.instMenuBtns, btn)
	}
	// Sync category buttons separately for tests that access them directly
	dv.instCategoryBtns = dv.instCategoryBtns[:0]
	for _, btn := range dv.instMenuComp.CategoryBtns() {
		dv.instCategoryBtns = append(dv.instCategoryBtns, btn)
		// Also add to instMenuBtns for compatibility
		dv.instMenuBtns = append(dv.instMenuBtns, btn)
	}
	// Sync scroll state for legacy access
	dv.instMenuScroll.View = dv.instMenuComp.ScrollView()
	first, visible, total := dv.instMenuComp.ScrollState()
	dv.instMenuScroll.First = first
	dv.instMenuScroll.Visible = visible
	dv.instMenuScroll.Total = total
	// Sync fullRect for legacy access
	dv.instMenuFullRect = dv.instMenuComp.Bounds()
	// Sync mode for legacy access
	mode := dv.instMenuComp.Mode()
	if mode == InstMenuModeCategories {
		dv.instMenuMode = instMenuModeCategories
	} else {
		dv.instMenuMode = instMenuModeInstruments
	}
	// Sync search box for legacy access
	dv.instSearchBox = dv.instMenuComp.SearchBox()
}
