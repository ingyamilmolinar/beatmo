package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Audio is written live per change; the emit and the single undo step happen once here, on release.

// No-op without a pending edit, so filter-handle releases are ignored.
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

// One undo step per synth-config gesture; the live write already happened at the edit site.
func (dv *DrumView) commitInstrumentParams(instID instrument.ID) {
	if instID == "" {
		return
	}
	// Recipe names the library instrument the document came from (its Origin).
	d, _ := audio.InstrumentDoc(instID)
	emitInstrumentParamsCommitted(instID, d.Origin)
	dv.recordUndoStep(hooks.EventInstrumentParamsCommitted)
}

// Announces nothing and records no undo step: those belong to the gesture (commitSamplerEdit),
// while this makes every change audible on the next trigger.
func (dv *DrumView) applySamplerEditLive() {
	s := &dv.sampler
	if s.captureID == "" || !s.hasBuffer() {
		return
	}
	audio.ApplySampleEditLive(s.captureID, s.editDescriptor())
}

// Ends a gesture: applies (idempotent), announces once, records one undo step. The announcement
// is here, not in the audio setter, which already stored the value live and would skip it.
func (dv *DrumView) commitSamplerEdit() {
	s := &dv.sampler
	if s.captureID == "" || !s.hasBuffer() {
		return
	}
	audio.ApplySampleEditLive(s.captureID, s.editDescriptor())
	emitSampleEditChanged(s.captureID)
	dv.recordUndoStep(hooks.EventSampleEditChanged)
}

func (dv *DrumView) commitEQFilter(filter hooks.EQFilter) {
	ch := dv.activeEQChannel()
	var enabled bool
	var cutoff float64
	switch filter {
	case hooks.FilterHPF:
		enabled, cutoff = dv.activeHPFEnabled(), dv.activeHPFCutoffHz()
	case hooks.FilterLPF:
		enabled, cutoff = dv.activeLPFEnabled(), dv.activeLPFCutoffHz()
	default:
		return
	}
	emitEQFilterToggled(ch, filter, enabled, cutoff)
	dv.recordUndoStep(hooks.EventEQFilterToggled)
}

// A band mute is a band edit, so it reuses EventEQBandChange.
func (dv *DrumView) commitEQBandMute(band int) {
	ch := dv.activeEQChannel()
	gainDB := 0.0
	if band >= 0 && band < len(dv.eqBandGainsDB()) {
		gainDB = dv.eqBandGainsDB()[band]
	}
	emitEQBandChange(ch, band, gainDB)
	dv.recordUndoStep(hooks.EventEQBandChange)
}

func (dv *DrumView) commitBPM() {
	emitBPMChange(dv.bpm)
}
