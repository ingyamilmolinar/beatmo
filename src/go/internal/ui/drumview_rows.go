package ui

import (
	"image"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// SetBounds is called from Game whenever the splitter moves or the window
// resizes; it invalidates the cached background so dimensions update next draw.
func (dv *DrumView) SetBounds(b image.Rectangle) {
	if dv.Bounds != b {
		dv.Bounds = b
		if dv.widgets != nil {
			dv.widgets.SetBounds(b)
			dv.refreshWidgetLayout()
			// Re-run layout after bounds change to keep controls anchored.
			dv.recalcButtons()
			dv.calcLayout()
		}
		dv.bgDirty = true
		dv.bgDirty = false
		// Invalidate row caches when bounds change (timeline width/height).
		dv.invalidateRowCaches()
		// Invalidate row controls cache when bounds change.
		dv.markRowControlsDirty()
	}
}

// AddRow appends a new drum row with default settings.
func (dv *DrumView) AddRow() {
	inst := "snare"
	name := "Snare"
	if len(dv.instOptions) > 0 {
		inst = dv.instOptions[0]
		name = strings.ToUpper(inst[:1]) + inst[1:]
	}
	idx := len(dv.Rows)
	baseCol := instColor(inst)
	uniq := dv.ensureUniqueColor(baseCol, idx)
	dv.Rows = append(dv.Rows, &DrumRow{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: uniq, Origin: model.InvalidNodeID, Node: nil, Volume: 1, EQGainsDB: make([]float64, len(eqBandDefs))})
	dv.logger.Infof("[DRUMVIEW] Row added index=%d instrument=%s name=%s", idx, inst, name)
	dv.added = append(dv.added, idx)
	dv.bgDirty = true
	dv.markRowControlsDirty()
	// Invalidate label caches since row names affect label width calculation.
	dv.invalidateLabelCaches()
	dv.activeSlider = -1
	dv.calcLayout()
	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
	// Ensure cache slices account for the new row and mark it dirty.
	dv.ensureRowCache()
	if len(dv.rowDirty) > 0 {
		idx := len(dv.rowDirty) - 1
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
	}
}

// DeleteRow removes the drum row at the given index.
func (dv *DrumView) DeleteRow(i int) {
	if i < 0 || i >= len(dv.Rows) || len(dv.Rows) <= 1 {
		return
	}
	dv.logger.Infof("[DRUMVIEW] Row deleted index=%d name=%s instrument=%s", i, dv.Rows[i].Name, dv.Rows[i].Instrument)
	// If the deleted row's instrument is the active EQ channel, reset to master
	if dv.eqActiveChannel == dv.Rows[i].Instrument {
		dv.setEQActiveChannel("main")
	}
	origin := dv.Rows[i].Origin
	dv.Rows = append(dv.Rows[:i], dv.Rows[i+1:]...)
	dv.deleted = append(dv.deleted, deletedRow{index: i, origin: origin})
	dv.bgDirty = true
	dv.markRowControlsDirty()
	// Invalidate label caches since row names affect label width calculation.
	dv.invalidateLabelCaches()
	dv.activeSlider = -1
	if dv.selRow >= len(dv.Rows) {
		dv.selRow = len(dv.Rows) - 1
	}
	dv.calcLayout()
	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
	// Reset caches so indexes realign.
	dv.rowCache = nil
	dv.rowDirty = nil
	dv.rowFullDirty = nil
	dv.rowFrame = nil
	dv.rowRepaint = nil
	dv.rowsLayer = nil
	dv.rowsLayerW, dv.rowsLayerH = 0, 0
	dv.rowsLayerOffset = 0
	dv.rowsLayerRowOff = 0
	dv.rowsLayerBaseX = 0
	dv.rowsLayerGen = 0
	dv.rowsLayerDirty = true
	dv.rowsLayerBytes = 0
	dv.rowsLayerFrame = 0
	dv.rowsRepaints = 0
}

func (dv *DrumView) toggleMute(idx int) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	r := dv.Rows[idx]
	r.Muted = !r.Muted
	if r.Muted {
		r.Solo = false
	}
	dv.markRowControlsDirty()
}

func (dv *DrumView) toggleSolo(idx int) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	r := dv.Rows[idx]
	r.Solo = !r.Solo
	if r.Solo {
		r.Muted = false
		for i, o := range dv.Rows {
			if i == idx {
				continue
			}
			o.Solo = false
			o.Muted = true
		}
	} else {
		anySolo := false
		for _, o := range dv.Rows {
			if o.Solo {
				anySolo = true
				break
			}
		}
		if !anySolo {
			for _, o := range dv.Rows {
				o.Muted = false
			}
		}
	}
	dv.markRowControlsDirty()
}

// ConsumeDeletedRows returns and clears the recently deleted rows info.
func (dv *DrumView) ConsumeDeletedRows() []deletedRow {
	rows := dv.deleted
	dv.deleted = nil
	return rows
}

// ConsumeAddedRows returns and clears indexes of newly added rows.
func (dv *DrumView) ConsumeAddedRows() []int {
	rows := dv.added
	dv.added = nil
	return rows
}

// ConsumeOriginRequests returns and clears indexes of rows requesting origin reassignment.
func (dv *DrumView) ConsumeOriginRequests() []int {
	rows := dv.originReq
	dv.originReq = nil
	return rows
}
