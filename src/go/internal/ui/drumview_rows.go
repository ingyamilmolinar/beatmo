package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// anySoloActive reports whether any row is soloed. Soloing changes the
// audibility rule from "everything not muted" to "only soloed rows".
func anySoloActive(rows []*DrumRow) bool {
	for _, r := range rows {
		if r != nil && r.Solo {
			return true
		}
	}
	return false
}

// rowAudible is the canonical per-row audibility predicate used across the
// engine: a row is audible when it is not muted AND (nothing is soloed OR it
// is itself soloed). Pass anySolo from anySoloActive(rows).
func rowAudible(r *DrumRow, anySolo bool) bool {
	return r != nil && !r.Muted && (!anySolo || r.Solo)
}

// audibleInstrumentIDs returns the set of instrument IDs that are currently
// audible given the rows' solo/mute state. Rows with an empty Instrument ID
// are skipped (no channel to key on).
func audibleInstrumentIDs(rows []*DrumRow) map[string]bool {
	anySolo := anySoloActive(rows)
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		if rowAudible(r, anySolo) {
			out[r.Instrument] = true
		}
	}
	return out
}

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
		// Re-clamp Length after bounds change so cells stay readable.
		// Use the graph's authoritative beat length as the target so that
		// Length can expand back when rotating to a wider orientation.
		targetLen := dv.Length
		if dv.Graph != nil {
			if gl := dv.Graph.BeatLength(); gl > targetLen {
				targetLen = gl
			}
		}
		// On mobile, cap the target to the platform default (e.g. 8 beats)
		// unless the user has manually adjusted the length via +/- buttons.
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
	// One atomic undo step. The group also defers the undo snapshot to the END
	// of this method, so it captures the post-mutation document regardless of
	// where inside the body the emit (and its recordUndo tap) fires.
	beginUndoGroup("add row")
	defer endUndoGroup()
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-add")
	}
	inst := "snare"
	if len(dv.instOptions) > 0 {
		inst = dv.instOptions[0]
	}
	name := dv.computeInstLabel(inst)
	idx := len(dv.Rows)
	// Pure sequential by row index: row N takes the Nth color of the canonical
	// instrument series (wrapping). Independent of the instrument id.
	rowColor := seriesColorAt(idx)
	dv.Rows = append(dv.Rows, &DrumRow{Name: name, Instrument: inst, Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length), Color: rowColor, Origin: model.InvalidNodeID, Node: nil, Volume: 0.5, EQGainsDB: make([]float64, len(eqBandDefs))})
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
	// One atomic undo step whose snapshot is captured at the END of this method.
	// emitRowDeleted (below) fires BEFORE the row is spliced out, so a direct
	// recordUndo would snapshot the pre-delete document and miss the change; the
	// group defers the capture to endGroup, after the splice. Cascaded deletes
	// (deleteNodeInternal → DeleteRow) nest under the outer group and stay one step.
	beginUndoGroup("delete row")
	defer endUndoGroup()
	if dv.onStructuralMutation != nil {
		dv.onStructuralMutation("row-delete")
	}
	dv.logger.Debugf("[drumview] row deleted index=%d name=%s instrument=%s", i, dv.Rows[i].Name, dv.Rows[i].Instrument)
	emitRowDeleted(i)
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
	dv.reconcileChannelWithAudibleSet()
}

// reconcileChannelWithAudibleSet keeps the audio-panel channel dropdown in
// step with what is actually audible after a solo/mute toggle. A row is audible
// iff !Muted && (!anySolo || Solo) -- the same predicate the timeline uses to
// dim rows.
//
//   - More than one distinct INSTRUMENT audible -> no single focus exists, so
//     revert the selector to Master (only when it currently points at an
//     instrument; a redundant re-select of Master is skipped). Counting by
//     distinct instrument (audibleInstrumentIDs) means several audible rows of
//     the SAME instrument still count as one focus and do not force Master.
//   - Otherwise, exactly one audible ROW (solo one, or mute all-but-one) ->
//     focus that instrument so its EQ/Synth/Sampler are directly editable.
//
// It is a no-op for single-row projects (nothing to disambiguate) and when the
// sole-audible row has no instrument id. Routes through selectAudioChannel so
// every tab follows the dropdown.
func (dv *DrumView) reconcileChannelWithAudibleSet() {
	if len(dv.Rows) < 2 {
		return
	}
	// Revert to Master when more than one distinct instrument is audible.
	if len(audibleInstrumentIDs(dv.Rows)) > 1 {
		if ch := dv.activeEQChannel(); ch != "" && ch != "main" {
			dv.selectAudioChannel("main")
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
		dv.selectAudioChannel(sole.Instrument)
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

// MarkRowFired snaps the row's now-playing tint intensity to 1.0. The
// intensity decays each frame in decayAnims so the row glows briefly when
// the sequencer fires its audible step. Out-of-range rows are ignored.
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

// RowFireIntensity returns the current decayed tint intensity in [0,1] for the
// given row. Test-only / draw-time accessor.
func (dv *DrumView) RowFireIntensity(row int) float64 {
	if row < 0 || row >= len(dv.rowFireDecay) {
		return 0
	}
	return dv.rowFireDecay[row]
}
