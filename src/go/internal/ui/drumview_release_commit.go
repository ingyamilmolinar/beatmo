package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

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

// commitInstrumentParams emits + records a single instrument synth-param change.
// It is the ONE reusable commit point for every synth-config edit that applies
// live to audio (knob drag release, stage enable/disable toggle, numeric value
// entry). The live audio.SetInstrumentParam runs at the edit site; this records
// exactly one undo step for the whole gesture. No-op without an instrument.
func (dv *DrumView) commitInstrumentParams(instID string) {
	if instID == "" {
		return
	}
	recipe := audio.RecipeForInstrument(instID)
	emitInstrumentParamsCommitted(instID, recipe)
	dv.recordUndoStep(hooks.EventInstrumentParamsCommitted)
}

// commitSamplerEdit applies the Sampler tab's staged edit live to the exported
// document (audio.SetSampleEdit) and records ONE undo step per gesture. It is
// the reusable commit point for every sampler edit gesture (trim handles, knobs,
// reverse/normalize/fade toggles, numeric entry) — the analog of
// commitInstrumentParams for synth and commitEQBand for EQ.
//
// BOTH sources apply live through the SAME non-destructive descriptor: the edit
// is an audio.SampleEdit the export round-trips, so each gesture is
// snapshot-undoable. For a synth the dispatcher applies the descriptor to the
// fresh recipe render at trigger time; for a WAV / user sample (no recipe)
// audio.SetSampleEdit re-derives the playable PCM from the stored pristine
// source and re-registers it (reapplyUserSampleEdit), so the chop updates in
// real time without waiting for Save. The WAV's pristine PCM is seeded into the
// user-sample store by ensureSamplerLoaded, which the descriptor decorates. No-op
// without a captured buffer or capture id.
func (dv *DrumView) commitSamplerEdit() {
	s := &dv.sampler
	if s.captureID == "" || !s.hasBuffer() {
		return
	}
	audio.SetSampleEdit(s.captureID, s.editDescriptor())
	dv.recordUndoStep(hooks.EventSampleEditChanged)
}

// commitEQFilter emits + records a single HPF/LPF enable toggle for the active
// EQ channel. filter is "hpf" or "lpf". Called once by toggleHPF/toggleLPF
// after the live applyEQ. Mirrors commitEQBand.
func (dv *DrumView) commitEQFilter(filter string) {
	ch := dv.activeEQChannel()
	var enabled bool
	var cutoff float64
	switch filter {
	case "hpf":
		enabled, cutoff = dv.activeHPFEnabled(), dv.activeHPFCutoffHz()
	case "lpf":
		enabled, cutoff = dv.activeLPFEnabled(), dv.activeLPFCutoffHz()
	default:
		return
	}
	emitEQFilterToggled(ch, filter, enabled, cutoff)
	dv.recordUndoStep(hooks.EventEQFilterToggled)
}

// commitEQBandMute emits + records a single per-band EQ mute toggle. A band
// mute is a per-band EQ modification, so it reuses EventEQBandChange for the
// undo step + event (there is no dedicated mute kind). Called once by
// toggleEQBandMute after the live applyMasterEQ/applyRowEQ.
func (dv *DrumView) commitEQBandMute(band int) {
	ch := dv.activeEQChannel()
	gainDB := 0.0
	if band >= 0 && band < len(dv.eqBandGainsDB()) {
		gainDB = dv.eqBandGainsDB()[band]
	}
	emitEQBandChange(ch, band, gainDB)
	dv.recordUndoStep(hooks.EventEQBandChange)
}
