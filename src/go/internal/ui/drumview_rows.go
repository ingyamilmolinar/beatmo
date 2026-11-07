package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func anySoloActive(rows []*DrumRow) bool {
	for _, r := range rows {
		if r != nil && r.Solo {
			return true
		}
	}
	return false
}

// The one audibility predicate: not muted, and nothing soloed or this row soloed.
func rowAudible(r *DrumRow, anySolo bool) bool {
	return r != nil && !r.Muted && (!anySolo || r.Solo)
}

// Rows without an instrument id are skipped: no channel to key on.
func audibleInstrumentIDs(rows []*DrumRow) map[channel.ID]bool {
	anySolo := anySoloActive(rows)
	out := make(map[channel.ID]bool, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		if rowAudible(r, anySolo) {
			out[channel.Of(r.Instrument)] = true
		}
	}
	return out
}

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
		// Targets the graph's beat length, so Length can grow back on a wider orientation.
		targetLen := dv.Length
		if dv.Graph != nil {
			if gl := dv.Graph.BeatLength(); gl > targetLen {
				targetLen = gl
			}
		}
		// Mobile caps at the platform default unless the user changed the length.
		if p := Profile(); p.IsMobile() && !dv.userAdjustedLength {
			if db := p.DefaultTimelineBeats; db > 0 && dv.timelineUnitsPerBeat > 0 {
				cap := db * dv.timelineUnitsPerBeat
				if targetLen > cap {
					targetLen = cap
				}
			}
		}
		if clamped := dv.clampLength(targetLen); clamped != dv.Length {
			oldLen := dv.Length
			dv.Length = clamped
			for _, r := range dv.Rows {
				newSteps := make([]bool, dv.Length)
				newTypes := make([]model.NodeType, dv.Length)
				n := oldLen
				if dv.Length < n {
					n = dv.Length
				}
				copy(newSteps[:n], r.Steps[:n])
				copy(newTypes[:n], r.CellTypes[:n])
				r.Steps = newSteps
				r.CellTypes = newTypes
			}
			dv.markAllRowsDirty()
		}
		dv.bgDirty = true
		// Invalidate row caches when bounds change (timeline width/height).
		dv.invalidateRowCaches()
		// Invalidate row controls cache when bounds change.
		dv.markRowControlsDirty()
	}
}

// AddRow appends a new drum row with default settings.
func (dv *DrumView) AddRow() {
	// The group defers the snapshot to the end, so it captures the finished document.
	beginUndoGroup("add row")
	defer endUndoGroup()
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-add")
	}
	inst := roster.Snare
	if len(dv.instOptions) > 0 {
		inst = dv.instOptions[0]
	}
	name := dv.computeInstLabel(inst)
	idx := len(dv.Rows)
	// By row index, independent of the instrument.
	rowColor := seriesColorAt(idx)
	dv.Rows = append(dv.Rows, &DrumRow{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: rowColor, Origin: model.InvalidNodeID, Node: nil, Volume: 0.5})
	dv.logger.Debugf("[drumview] row added index=%d instrument=%s name=%s", idx, inst, name)
	emitRowAdded(idx, inst, name)
	dv.added = append(dv.added, idx)
	dv.bgDirty = true
	dv.markRowControlsDirty()
	// Invalidate label caches since row names affect label width calculation.
	dv.invalidateLabelCaches()
	if dv.rowVolGroup() != nil {
		dv.rowVolGroup().Release()
	}
	dv.calcLayout()
	maxOff := len(dv.Rows) - dv.visibleRows()
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
	// The group defers the snapshot past the splice: emitRowDeleted fires before it. Cascaded deletes
	// nest and stay one step.
	beginUndoGroup("delete row")
	defer endUndoGroup()
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-delete")
	}
	dv.logger.Debugf("[drumview] row deleted index=%d name=%s instrument=%s", i, dv.Rows[i].Name, dv.Rows[i].Instrument)
	emitRowDeleted(i)
	// If the deleted row's instrument is the active EQ channel, reset to master
	if dv.eqActiveChannel == channel.Of(dv.Rows[i].Instrument) {
		dv.setEQActiveChannel(channel.Master)
	}
	origin := dv.Rows[i].Origin
	dv.Rows = append(dv.Rows[:i], dv.Rows[i+1:]...)
	if dv.onRowRemoved != nil {
		dv.onRowRemoved(i, origin)
	}
	dv.deleted = append(dv.deleted, deletedRow{index: i, origin: origin})
	dv.bgDirty = true
	dv.markRowControlsDirty()
	// Invalidate label caches since row names affect label width calculation.
	dv.invalidateLabelCaches()
	if dv.rowVolGroup() != nil {
		dv.rowVolGroup().Release()
	}
	if dv.selRow >= len(dv.Rows) {
		dv.selRow = len(dv.Rows) - 1
	}
	dv.calcLayout()
	maxOff := len(dv.Rows) - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
	// Reset caches so indexes realign.
	if dv.timelineZone != nil {
		dv.timelineZone.ResetAfterDelete()
	}
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
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-mute-toggle")
	}
	r := dv.Rows[idx]
	r.Muted = !r.Muted
	if r.Muted {
		r.Solo = false
	}
	dv.markRowControlsDirty()
	emitRowMute(idx, r.Muted)
	dv.onAudibleSetChanged()
	dv.reconcileChannelWithAudibleSet()
}

func (dv *DrumView) toggleSolo(idx int) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-solo-toggle")
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
	emitRowSolo(idx, r.Solo)
	dv.onAudibleSetChanged()
	dv.reconcileChannelWithAudibleSet()
}

// More than one distinct instrument audible: back to Master. Exactly one audible row: focus its
// instrument so its tabs are editable. Same predicate as the timeline's dimming.
func (dv *DrumView) reconcileChannelWithAudibleSet() {
	if len(dv.Rows) < 2 {
		return
	}
	// Revert to Master when more than one distinct instrument is audible.
	if len(audibleInstrumentIDs(dv.Rows)) > 1 {
		if ch := dv.activeEQChannel(); !ch.IsMaster() {
			dv.selectAudioChannel(channel.Master)
		}
		return
	}
	// Otherwise focus the sole audible row's instrument, if there is exactly one.
	anySolo := anySoloActive(dv.Rows)
	count := 0
	var sole *DrumRow
	for _, r := range dv.Rows {
		if rowAudible(r, anySolo) {
			count++
			sole = r
		}
	}
	if count == 1 && sole != nil && sole.Instrument != "" {
		dv.selectAudioChannel(channel.Of(sole.Instrument))
	}
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

func (dv *DrumView) MarkRowFired(row int) {
	if row < 0 {
		return
	}
	if row >= len(dv.rowFireDecay) {
		grown := make([]float64, row+1)
		copy(grown, dv.rowFireDecay)
		dv.rowFireDecay = grown
	}
	dv.rowFireDecay[row] = 1.0
}

func (dv *DrumView) RowFireIntensity(row int) float64 {
	if row < 0 || row >= len(dv.rowFireDecay) {
		return 0
	}
	return dv.rowFireDecay[row]
}
