package ui

import (
	"image"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func (dv *DrumView) instMenuHasScroll() bool {
	return dv.instMenuScroll.HasScroll()
}

func (dv *DrumView) instMenuThumbRect() image.Rectangle {
	if !dv.IsInstMenuOpen() || !dv.instMenuHasScroll() || dv.instMenuScroll.View.Empty() {
		return image.Rect(0, 0, 0, 0)
	}
	return dv.instMenuScroll.ThumbRect(dropdownScrollbarWidth(), dv.rowHeight()/2)
}

func (dv *DrumView) instDisplayLabel(id instrument.ID) string {
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
		dv.instLabelCache = make(map[instrument.ID]string)
	}
	dv.instLabelCache[id] = lbl
	return lbl
}

func (dv *DrumView) computeInstLabel(id instrument.ID) string {
	return audio.InstrumentDisplayName(id)
}

func (dv *DrumView) matchInstrumentSearch(id instrument.ID, q string) bool {
	if q == "" {
		return true
	}
	idLower := strings.ToLower(string(id))
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

// Mirrors the component's scroll into dv.instMenuScroll for the callers that read it.
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
	dv.instCategoryBtns = append(dv.instCategoryBtns[:0], dv.instMenuComp.CategoryBtns()...)
}

// Freshly allocated each call: [Back?, categories or instruments]. Read-only.
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

// An out-of-range row is a silent no-op.
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
				dv.instMenuActiveByRow = map[int]audio.Category{}
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
		// Only a partially built DrumView (tests) lacks the component.
		return
	}

	var instOpts []InstrumentOption
	for _, id := range dv.instOptions {
		// instDisplayLabel, not the cache: it fills a miss.
		label := dv.instDisplayLabel(id)
		cat := audio.CatNone
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
		TextFields:            dv.textFields,
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
		OnSelect: func(instID instrument.ID) {
			// Never drops the row's settings: it may ask first.
			dv.pickInstrument(rowIdx, instID)
		},
		RowInstrument: func() instrument.ID {
			if rowIdx >= 0 && rowIdx < len(dv.Rows) {
				return dv.Rows[rowIdx].Instrument
			}
			return ""
		},
		OnFavorite: func(instID instrument.ID, fav bool) {
			// The component writes FavoritesStore itself.
			_ = instID
			_ = fav
		},
		OnClose: func() {
			dv.closeInstMenuPortal()
		},
		OnStateChanged: func() {
			dv.syncInstMenuScrollFromComp()
		},
	})
	dv.instMenuDeferredTap.Cancel()
	dv.instMenuComp.Open()
	dv.openInstMenuPortal()
	dv.syncInstMenuScrollFromComp()
}
