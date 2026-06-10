package ui

import "github.com/ingyamilmolinar/beatmo/internal/hooks"

// Deterministic release-commit points for continuous controls. The live
// audio.Set* calls run per-frame during the drag so the sound updates live;
// the user-facing event emit + the single undo step are recorded ONCE here,
// at drag release. This keeps undo history one-step-per-gesture and the event
// stream free of per-frame churn.

// commitEQBand emits + records the EQ band change once, at slider/curve
// release (curve-handle drag-up or dB text-input commit). No-op when no
// pending edit exists, so filter-handle releases (which don't touch the band
// gains) are silently ignored.
func (dv *DrumView) commitEQBand() {
	if !dv.eqPendingDirty {
		return
	}
	dv.eqPendingDirty = false
	emitEQBandChange(dv.eqPendingChannel, dv.eqPendingBand, dv.eqPendingGainDB)
	dv.recordUndoStep(hooks.EventEQBandChange)
}

// commitMainVolume emits + records the master-volume change once, at release.
func (dv *DrumView) commitMainVolume() {
	if !dv.mainVolPendingDirty {
		return
	}
	dv.mainVolPendingDirty = false
	emitMasterVolumeChange(dv.mainVolPending)
	dv.recordUndoStep(hooks.EventMasterVolumeChange)
}

// commitRowVolume emits + records a row-volume change once, at popup release.
func (dv *DrumView) commitRowVolume(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	emitRowVolume(row, dv.Rows[row].Volume)
	dv.recordUndoStep(hooks.EventRowVolume)
}
