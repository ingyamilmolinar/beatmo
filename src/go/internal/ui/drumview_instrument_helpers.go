package ui

import (
	"image"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// instMenuHasScroll reports whether the instrument menu has more rows
// than fit in the current viewport. Drives whether the scrollbar
// gutter renders.
func (dv *DrumView) instMenuHasScroll() bool {
	return dv.instMenuScroll.HasScroll()
}

// instMenuThumbRect returns the screen-space rect of the scrollbar
// thumb for the open menu, or an empty rect when the menu is closed
// or has no scroll.
func (dv *DrumView) instMenuThumbRect() image.Rectangle {
	if !dv.IsInstMenuOpen() || !dv.instMenuHasScroll() || dv.instMenuScroll.View.Empty() {
		return image.Rect(0, 0, 0, 0)
	}
	return dv.instMenuScroll.ThumbRect(dropdownScrollbarWidth(), dv.rowHeight()/2)
}

// instDisplayLabel returns a user-facing name for an instrument ID,
// preferring catalog metadata when available. Uses a per-DrumView
// cache to avoid recomputing the label every frame.
func (dv *DrumView) instDisplayLabel(id string) string {
	if id == "" {
		return "(missing)"
	}
	if dv.instLabelCache != nil {
		if lbl, ok := dv.instLabelCache[id]; ok {
			return lbl
		}
	}
	lbl := dv.computeInstLabel(id)
	if dv.instLabelCache == nil {
		dv.instLabelCache = make(map[string]string)
	}
	dv.instLabelCache[id] = lbl
	return lbl
}

// computeInstLabel computes the display label for an instrument ID without
// caching. Delegates to audio.InstrumentDisplayName, the single source of
// truth: user override → catalog name → catalog basename → pretty-cased id.
func (dv *DrumView) computeInstLabel(id string) string {
	return audio.InstrumentDisplayName(id)
}

// matchInstrumentSearch reports whether the query q matches the
// instrument id by id, display label, or relPath. Case-insensitive
// substring match. Empty query always matches.
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

// syncInstMenuScrollFromComp mirrors the InstrumentMenuComponent's
// internal scroll state into dv.instMenuScroll so call sites that
// historically read the latter (drumview_notifications.go, tests in
// drumview_test.go) keep working without each one reaching through
// the component. Invoked at every state-mutating menu transition
// (Open, Refresh, post-Update). Cheap.
func (dv *DrumView) syncInstMenuScrollFromComp() {
	if dv == nil || dv.instMenuComp == nil {
		return
	}
	dv.instMenuScroll.View = dv.instMenuComp.ScrollView()
	first, visible, total := dv.instMenuComp.ScrollState()
	dv.instMenuScroll.First = first
	dv.instMenuScroll.Visible = visible
	dv.instMenuScroll.Total = total
	dv.instMenuFullRect = dv.instMenuComp.Bounds()
	mode := dv.instMenuComp.Mode()
	if mode == InstMenuModeCategories {
		dv.instMenuMode = instMenuModeCategories
	} else {
		dv.instMenuMode = instMenuModeInstruments
	}
	dv.instSearchBox = dv.instMenuComp.SearchBox()
	// Mirror category buttons too — drumview_notifications and a few
	// tests read dv.instCategoryBtns directly.
	dv.instCategoryBtns = append(dv.instCategoryBtns[:0], dv.instMenuComp.CategoryBtns()...)
}

// instMenuBtns returns the current set of instrument-menu buttons
// derived from InstrumentMenuComponent. Test code historically read
// dv.instMenuBtns as a field; the field has been retired and this
// method returns the equivalent shape: [Back?, ...categoryBtns or
// instBtns]. Order matches the legacy contract (Back first when in
// instruments mode and categories exist; otherwise the visible
// list at index 0).
//
// Callers must treat the slice as read-only — it is freshly allocated
// on each call and not retained between frames.
func (dv *DrumView) instMenuBtns() []*Button {
	if dv == nil || dv.instMenuComp == nil {
		return nil
	}
	comp := dv.instMenuComp
	out := make([]*Button, 0, len(comp.InstBtns())+len(comp.CategoryBtns())+1)
	if back := comp.BackBtn(); back != nil {
		out = append(out, back)
	}
	out = append(out, comp.InstBtns()...)
	out = append(out, comp.CategoryBtns()...)
	return out
}

// openInstMenuForRow opens the instrument selector menu for the
// given row, using the InstrumentMenuComponent. A row out of range
// is a silent no-op so callers can pass user-supplied indices
// without bounds checking.
func (dv *DrumView) openInstMenuForRow(rowIdx int) {
	if rowIdx < 0 || rowIdx >= len(dv.Rows) {
		return
	}

	dv.selRow = rowIdx
	dv.instMenuCameFromCategories = false
	dv.instMenuUserScrolled = false

	// Prime category based on the row's current instrument.
	if dv.instCatByID != nil {
		if cat, ok := dv.instCatByID[dv.Rows[rowIdx].Instrument]; ok {
			dv.instMenuActiveCat = cat
			if dv.instMenuActiveByRow == nil {
				dv.instMenuActiveByRow = map[int]string{}
			}
			dv.instMenuActiveByRow[rowIdx] = cat
		}
	}

	// Toggle: close if already open for this row.
	if dv.instMenuComp != nil && dv.instMenuComp.IsOpen() && dv.instMenuRow == rowIdx {
		dv.instMenuComp.Close()
		dv.closeInstMenuPortal()
		dv.instMenuScroll.EndDrag()
		return
	}

	// Close all other overlays for mutual exclusivity.
	dv.CloseAllPopups()

	dv.instMenuRow = rowIdx

	if dv.instMenuComp == nil {
		// Component should always be present after DrumView.New() — guard
		// for tests that build a partial DrumView. There's no legacy
		// fallback path anymore.
		return
	}

	var instOpts []InstrumentOption
	for _, id := range dv.instOptions {
		label := id
		if dv.instLabelCache != nil {
			if l, ok := dv.instLabelCache[id]; ok {
				label = l
			}
		}
		cat := ""
		if dv.instCatByID != nil {
			cat = dv.instCatByID[id]
		}
		instOpts = append(instOpts, InstrumentOption{
			ID:       id,
			Label:    label,
			Category: cat,
			Color:    dv.instrumentRowColor(id),
		})
	}

	vertBounds := dv.widgetRects[WidgetRack]
	if Profile().IsMobile() {
		vertBounds = dv.Bounds
	}
	anchorRect := image.Rectangle{}
	if rowIdx < len(dv.rowLabels()) {
		anchorRect = dv.rowLabels()[rowIdx].Rect()
	}
	dv.instMenuComp.SetProps(InstrumentMenuProps{
		AnchorRect:            anchorRect,
		VertBounds:            vertBounds,
		RowIndex:              rowIdx,
		CurrentInstrument:     dv.Rows[rowIdx].Instrument,
		Categories:            dv.instCategories,
		Instruments:           instOpts,
		RowHeight:             dv.rowHeight(),
		LabelWidth:            dv.labelW,
		ControlsWidth:         dv.controlsW,
		ForceCategories:       dv.instMenuForceCategories,
		Favorites:             Favorites(),
		ShowFavoritesCategory: dv.instMenuShowFavoritesCategory,
		ProjectPins:           dv.ProjectPins(),
		Style:                 DefaultMenuStyle(),
		OnSelect: func(instID string) {
			// SetInstrument invokes dv.onRowInstrumentChanged so
			// dependents (EQ panel, etc.) follow per CLAUDE.md
			// "Two-Tier Event Notification" rules.
			dv.SetInstrument(instID)
		},
		OnFavorite: func(instID string, fav bool) {
			// FavoritesStore.Set is invoked by the component itself;
			// this hook is a telemetry / future-integration seam.
			_ = instID
			_ = fav
		},
		OnClose: func() {
			dv.closeInstMenuPortal()
		},
		OnStateChanged: func() {
			// Fired from inside SetProps / Open / category click /
			// search / scroll. Mirror the component's state into
			// dv's bookkeeping fields so tests and other DrumView
			// methods see consistent state without reaching through
			// dv.instMenuComp accessors on every read.
			dv.syncInstMenuScrollFromComp()
		},
	})
	dv.instMenuDeferredTap.Cancel()
	dv.instMenuComp.Open()
	dv.openInstMenuPortal()
	dv.syncInstMenuScrollFromComp()
}
