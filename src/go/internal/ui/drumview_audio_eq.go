package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// sliderToGainDB maps EQ slider value (0..1) to gain in dB.
// Standard linear mapping: 0% = -12 dB, 50% = 0 dB (unity), 100% = +12 dB
func sliderToGainDB(v float64) float64 {
	// Linear mapping: v=0 -> -12dB, v=0.5 -> 0dB, v=1 -> +12dB
	gain := (v - 0.5) * 24
	return math.Round(gain*10) / 10
}

// gainDBToSlider is the inverse of sliderToGainDB.
func gainDBToSlider(g float64) float64 {
	// Inverse of linear: g = (v - 0.5) * 24, so v = g/24 + 0.5
	v := (g / 24) + 0.5
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return v
}

// activeEQChannel returns the current EQ channel (channel.Master from
// construction on; never zero).
func (dv *DrumView) activeEQChannel() channel.ID { return dv.eqActiveChannel }

func (dv *DrumView) analyzerSnapshot() audio.AnalyzerSnapshot {
	if dv.eqTestSnapshot != nil {
		return *dv.eqTestSnapshot
	}
	// Skip WebAudio FFT calls when EQ panel is not visible.
	if dv.eqPanelZone != nil && dv.eqPanelZone.rect.Dy() < 40 {
		return audio.AnalyzerSnapshot{}
	}
	return audio.ChannelAnalyzerSnapshot(dv.activeEQChannel())
}

func (dv *DrumView) preEQAnalyzerSnapshot() audio.AnalyzerSnapshot {
	if dv.eqTestPreEQSnapshot != nil {
		return *dv.eqTestPreEQSnapshot
	}
	return audio.PreEQAnalyzerSnapshot(dv.activeEQChannel())
}

// applyMasterEQ applies the master channel EQ from dv.eqBandGainsDB().
func (dv *DrumView) applyMasterEQ() {
	// masterEQBandState, NOT eqBandGainsDB: the latter is the panel's working
	// copy of the channel currently on screen, so with an instrument selected
	// this pushed that instrument's curve onto the MASTER bus.
	mGains, mMuted := dv.masterEQBandState()
	if len(mGains) != len(eqBandDefs) {
		return
	}
	bands := dv.buildFullEQBands(mGains, mMuted, dv.hpfEnabled, dv.hpfCutoffHz, dv.lpfEnabled, dv.lpfCutoffHz)
	// Persist for tests/exports and push to audio engine.
	dv.eqApplied = bands
	audio.SetChannelEQ(channel.Master, audio.SampleRate(), bands...)
	// Ensure analyzers stay in the chain so waveform/spectrum remain live.
	_ = audio.EnableChannelAnalyzer(channel.Master, 512)
	_ = audio.EnablePreEQAnalyzer(channel.Master, 512)
	_ = audio.EnableSynthAnalyzer(channel.Master, 512)
	dv.eqCurveDirty = true
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.curveDirty = true
	}
}

// applyRowEQ pushes the row's instrument EQ (rowEQ) to its channel: the
// compiled chain, or none when flat — a unity biquad chain is not a no-op, it
// differs from an absent one in the last bits, and a project that carries no
// `eq` block must import to a channel with NO EQ processor (export → import
// bit-exact). audio.ApplyEQSettings re-wires the channel's analyzers, which
// live in the same processor list.
func (dv *DrumView) applyRowEQ(row int) {
	if row < 0 || row >= len(dv.Rows) || dv.Rows[row].Instrument == "" {
		return
	}
	audio.ApplyEQSettings(dv.Rows[row].Instrument)
	dv.eqCurveDirty = true
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.curveDirty = true
	}
}

// clearRowEQ flattens the row's instrument EQ and applies it (no EQ
// processor at all).
func (dv *DrumView) clearRowEQ(row int) {
	if row < 0 || row >= len(dv.Rows) || dv.Rows[row].Instrument == "" {
		return
	}
	*rowEQ(dv.Rows[row]) = *audio.NewEQSettings()
	dv.applyRowEQ(row)
}

// applyEQ applies EQ to the currently active channel. For backwards
// compatibility, this function is called from existing code paths.
func (dv *DrumView) applyEQ() {
	ch := dv.activeEQChannel()
	if ch == channel.Master {
		dv.applyMasterEQ()
		return
	}
	// Find the row with this instrument ID
	for i, r := range dv.Rows {
		if channel.Of(r.Instrument) == ch {
			dv.applyRowEQ(i)
			return
		}
	}
	// Fallback to master if no matching row
	dv.applyMasterEQ()
}

// setEQActiveChannel switches the EQ view to the specified channel.
// Pass channel.Master or an instrument's channel for per-row EQ.
func (dv *DrumView) setEQActiveChannel(id channel.ID) {
	if id.IsZero() {
		return // names no channel
	}
	prev := dv.activeEQChannel()
	channelChanged := prev != id

	if channelChanged {
		// Disable analyzers on the previous channel to save FFT CPU.
		audio.SetAnalyzerEnabled(prev, false)
		// Save the zone's working copy back to the appropriate store before
		// switching, so master gains are not lost when switching to an instrument.
		dv.saveZoneBandState(prev)
	}

	dv.eqActiveChannel = id

	// Keep the panel zone's channel in lockstep so Wave/Spectrum (resolveChannel)
	// and the Synth tab (synthTabActiveInstrument reads ActiveChannel()) follow
	// every programmatic channel change, not just dropdown clicks.
	if dv.eqPanelZone != nil {
		// Through the zone's single writer, never the field: it re-baselines
		// the Spectrum readout, and the anchor only rises, so a switch that
		// skips it measures the new channel against the OLD one's loudest
		// moment. This path carries every PROGRAMMATIC switch — the
		// auto-focus when a mute leaves one audible row, renames, import,
		// the JS bridge — which is most of them.
		dv.eqPanelZone.setActiveChannelID(id)
	}

	// Update button text
	if dv.eqChannelBtn() != nil {
		if id.IsMaster() {
			dv.eqChannelBtn().Text = "Master"
		} else {
			// Find row name for this instrument
			label := id.String()
			for _, r := range dv.Rows {
				if channel.Of(r.Instrument) == id {
					label = r.Name
					break
				}
			}
			// Truncate to fit button width using pixel-based metrics
			if dv.eqChannelBtn() != nil && !dv.eqChannelBtn().Rect().Empty() {
				maxW := dv.eqChannelBtn().Rect().Dx() - 8
				if maxW > 0 {
					label = clipTextToWidth(label, maxW)
				}
			}
			dv.eqChannelBtn().Text = label
		}
	}

	// Load gains/muted from the target channel's backing store and sync the
	// zone's working arrays so curve/handles update correctly.
	// For same-channel re-select (e.g., after external row data changes),
	// master reads from the zone's live data; instruments reload from the row.
	var gains []float64
	var muted []bool
	if channelChanged {
		gains, muted = dv.loadChannelBandState(id)
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.SyncBandState(gains, muted)
		}
	} else if id.IsMaster() {
		// Same master channel: the zone IS the authoritative store. The dB
		// readouts derive live from bandGainsDB, so an external write into the
		// shared backing array shows up on the next Draw with no sync needed.
		if dv.eqPanelZone != nil {
			muted = dv.eqPanelZone.bandMuted
		}
	} else {
		// Same instrument channel: reload from the row (external code may
		// have modified row.EQGainsDB or row.EQBandMuted directly).
		gains, muted = dv.loadChannelBandState(id)
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.SyncBandState(gains, muted)
		}
	}

	// Update mute buttons to reflect the channel's current mute state
	for i, btn := range dv.eqMuteBtns() {
		if btn == nil {
			continue
		}
		isMuted := i < len(muted) && muted[i]
		if isMuted {
			btn.Style = EQMuteButtonActiveStyle
		} else {
			btn.Style = EQMuteButtonStyle
		}
	}

	// Sync HPF/LPF button styles for the selected channel
	dv.syncFilterButtonStyles()

	// Enable analyzers for the selected channel. SynthAnalyzer is required
	// for the Chn tab's Synth/AntiPop traces to render — without it the
	// per-instrument scope state's TapA goes inactive on WASM. See
	// chain_tab_per_instrument_test.go.
	active := dv.activeEQChannel()
	_ = audio.EnableChannelAnalyzer(active, 512)
	_ = audio.EnablePreEQAnalyzer(active, 512)
	_ = audio.EnableSynthAnalyzer(active, 512)
	audio.SetAnalyzerEnabled(active, true)
	dv.eqCurveDirty = true
}

// selectAudioChannel is the single chokepoint for "select this channel across
// every audio-panel tab". It updates the EQ/zone channel state
// (setEQActiveChannel, which also syncs eqPanelZone.activeChannel for the
// Wave/Spectrum/Synth tabs), the analyzer detail channel (Wave/Spectrum data),
// the scope instrument (Chain tab on desktop), and the chain zone's instrument
// id (Chain tab on WASM/test where ScopeService is nil). The channel dropdown's
// OnChannelChange callback, the WASM setEQChannel bridge, and the solo/mute
// auto-select all route through here so the dropdown stays the single source of
// truth. Pass channel.Master or an instrument's channel; the zero ID is ignored.
func (dv *DrumView) selectAudioChannel(id channel.ID) {
	if id.IsZero() {
		return
	}
	dv.setEQActiveChannel(id)
	// Set the analyzer detail channel so Wave/Spectrum tabs show the selected
	// instrument's data (nil under -tags test / WASM).
	if svc := audio.AnalyzerService(); svc != nil {
		if id.IsMaster() {
			svc.SetDetailChannel(channel.ID{}) // no per-row detail: Master is the state's own
		} else {
			svc.SetDetailChannel(id)
		}
	}
	// Drive the Chain (Scope) tab from the same selector. Desktop goes through
	// the audio scope service; WASM and stubbed test builds (ScopeService nil)
	// rely on the zone-local instrumentID written below.
	if svc := audio.ScopeService(); svc != nil {
		svc.SetInstrument(id)
	}
	if dv.eqPanelZone != nil {
		if z := dv.eqPanelZone.chainZone; z != nil {
			z.instrumentID = id
		}
	}
}

// saveZoneBandState copies the zone's current working bandGainsDB/bandMuted
// back to the appropriate backing store (masterGainsDB for "main", or the
// matching DrumRow for per-instrument channels).
func (dv *DrumView) saveZoneBandState(ch channel.ID) {
	if dv.eqPanelZone == nil {
		return
	}
	z := dv.eqPanelZone
	if ch.IsMaster() {
		dv.ensureMasterBandState()
		copy(dv.masterGainsDB, z.bandGainsDB)
		copy(dv.masterMuted, z.bandMuted)
	} else {
		if inst, ok := ch.Instrument(); ok {
			eq := audio.EQSettingsOf(inst)
			copy(eq.GainsDB, z.bandGainsDB)
			copy(eq.BandMuted, z.bandMuted)
		}
	}
}

// loadChannelBandState returns the gains and muted slices for the given
// channel from the backing store. Returns fresh copies for "main" so
// SyncBandState doesn't alias the master store.
func (dv *DrumView) loadChannelBandState(id channel.ID) (gains []float64, muted []bool) {
	if id.IsMaster() {
		dv.ensureMasterBandState()
		gains = dv.masterGainsDB
		muted = dv.masterMuted
	} else {
		if inst, ok := id.Instrument(); ok {
			eq := audio.EQSettingsOf(inst)
			gains, muted = eq.GainsDB, eq.BandMuted
		}
	}
	return
}

// masterEQBandState returns the MASTER channel's authoritative band gains and
// mute flags.
//
// dv.eqBandGainsDB() is NOT that: it is `eqPanelZone.bandGainsDB`, the panel's
// working copy of whatever channel is currently on screen. Reading it as "the
// master EQ" silently substitutes the selected instrument's curve the moment
// the user clicks the channel pill — which is how a project exported `"eq":
// null` (and how applyMasterEQ pushed an instrument's curve onto the master
// bus) after a single click that changed nothing about the master.
//
// The split follows setEQActiveChannel's own rule: while Master is displayed
// the zone IS the store (live edits land there and are flushed to masterGainsDB
// by saveZoneBandState on the next channel switch); otherwise the zone belongs
// to an instrument and dv.masterGainsDB/masterMuted are authoritative.
//
// The returned slices alias the backing store — callers may read them or write
// through them, but must not retain them across a channel switch.
func (dv *DrumView) masterEQBandState() (gains []float64, muted []bool) {
	if ch := dv.activeEQChannel(); ch.IsMaster() {
		return dv.eqBandGainsDB(), dv.eqBandMuted()
	}
	dv.ensureMasterBandState()
	return dv.masterGainsDB, dv.masterMuted
}

// setMasterEQBandState writes the master channel's band gains and mute flags to
// whichever store is authoritative right now (see masterEQBandState), keeping
// the zone working copy in step when Master is the channel on screen so the
// curve and dB readouts update without a re-select.
func (dv *DrumView) setMasterEQBandState(gains []float64, muted []bool) {
	dv.ensureMasterBandState()
	copy(dv.masterGainsDB, gains)
	copy(dv.masterMuted, muted)
	if ch := dv.activeEQChannel(); ch.IsMaster() {
		copy(dv.eqBandGainsDB(), gains)
		copy(dv.eqBandMuted(), muted)
	}
}

// ensureMasterBandState initializes masterGainsDB and masterMuted if needed,
// seeding from the zone's current values when first created.
func (dv *DrumView) ensureMasterBandState() {
	n := len(eqBandDefs)
	if len(dv.masterGainsDB) != n {
		dv.masterGainsDB = make([]float64, n)
		if dv.eqPanelZone != nil && len(dv.eqPanelZone.bandGainsDB) == n {
			copy(dv.masterGainsDB, dv.eqPanelZone.bandGainsDB)
		}
	}
	if len(dv.masterMuted) != n {
		dv.masterMuted = make([]bool, n)
		if dv.eqPanelZone != nil && len(dv.eqPanelZone.bandMuted) == n {
			copy(dv.masterMuted, dv.eqPanelZone.bandMuted)
		}
	}
}

// cycleEQChannel advances to the next EQ channel in sequence:
// Master -> Instrument1 -> Instrument2 -> ... -> Master
func (dv *DrumView) cycleEQChannel() {
	current := dv.activeEQChannel()

	if current == channel.Master && len(dv.Rows) > 0 {
		// Switch to first instrument
		dv.setEQActiveChannel(channel.Of(dv.Rows[0].Instrument))
		return
	}

	// Find current position and advance
	for i, r := range dv.Rows {
		if channel.Of(r.Instrument) == current {
			if i+1 < len(dv.Rows) {
				dv.setEQActiveChannel(channel.Of(dv.Rows[i+1].Instrument))
			} else {
				dv.setEQActiveChannel(channel.Master)
			}
			return
		}
	}

	// Default to main if current not found
	dv.setEQActiveChannel(channel.Master)
}

// activeEQ is the EQ settings of the active instrument channel, or nil when
// the master (whose filters live on the DrumView) is on screen.
func (dv *DrumView) activeEQ() *audio.EQSettings {
	if inst, ok := dv.activeEQChannel().Instrument(); ok {
		return audio.EQSettingsOf(inst)
	}
	return nil
}

// activeHPFEnabled returns whether HPF is enabled for the active channel.
func (dv *DrumView) activeHPFEnabled() bool {
	if eq := dv.activeEQ(); eq != nil {
		return eq.HPF
	}
	return dv.hpfEnabled
}

// activeHPFCutoffHz returns the HPF cutoff for the active channel.
func (dv *DrumView) activeHPFCutoffHz() float64 {
	if eq := dv.activeEQ(); eq != nil {
		if eq.HPFHz <= 0 {
			return audio.EQHPFDefaultHz
		}
		return eq.HPFHz
	}
	return dv.hpfCutoffHz
}

// activeLPFEnabled returns whether LPF is enabled for the active channel.
func (dv *DrumView) activeLPFEnabled() bool {
	if eq := dv.activeEQ(); eq != nil {
		return eq.LPF
	}
	return dv.lpfEnabled
}

// activeLPFCutoffHz returns the LPF cutoff for the active channel.
func (dv *DrumView) activeLPFCutoffHz() float64 {
	if eq := dv.activeEQ(); eq != nil {
		if eq.LPFHz <= 0 {
			return audio.EQLPFDefaultHz
		}
		return eq.LPFHz
	}
	return dv.lpfCutoffHz
}

// setActiveHPF sets the HPF state for the active channel.
func (dv *DrumView) setActiveHPF(enabled bool, cutoffHz float64) {
	if eq := dv.activeEQ(); eq != nil {
		eq.HPF, eq.HPFHz = enabled, cutoffHz
		return
	}
	dv.hpfEnabled, dv.hpfCutoffHz = enabled, cutoffHz
}

// setActiveLPF sets the LPF state for the active channel.
func (dv *DrumView) setActiveLPF(enabled bool, cutoffHz float64) {
	if eq := dv.activeEQ(); eq != nil {
		eq.LPF, eq.LPFHz = enabled, cutoffHz
		return
	}
	dv.lpfEnabled, dv.lpfCutoffHz = enabled, cutoffHz
}

// toggleHPF toggles the high-pass filter for the active channel.
func (dv *DrumView) toggleHPF() {
	enabled := !dv.activeHPFEnabled()
	dv.setActiveHPF(enabled, dv.activeHPFCutoffHz())
	dv.applyEQ()
	dv.eqCurveDirty = true
	dv.commitEQFilter(hooks.FilterHPF)
}

// toggleLPF toggles the low-pass filter for the active channel.
func (dv *DrumView) toggleLPF() {
	enabled := !dv.activeLPFEnabled()
	dv.setActiveLPF(enabled, dv.activeLPFCutoffHz())
	dv.applyEQ()
	dv.eqCurveDirty = true
	dv.commitEQFilter(hooks.FilterLPF)
}

// buildFullEQBands compiles an EQ (the master's, or one drawn) into its band
// chain, exactly as an instrument's settings compile (audio.EQSettings.Bands).
func (dv *DrumView) buildFullEQBands(gains []float64, muted []bool, hpfOn bool, hpfHz float64, lpfOn bool, lpfHz float64) []audio.EQBand {
	s := audio.EQSettings{GainsDB: gains, BandMuted: muted, HPF: hpfOn, HPFHz: hpfHz, LPF: lpfOn, LPFHz: lpfHz}
	return s.Bands()
}

// syncFilterButtonStyles updates the HPF/LPF button appearance for the active channel.
func (dv *DrumView) syncFilterButtonStyles() {
	if dv.hpfBtn() != nil {
		if dv.activeHPFEnabled() {
			dv.hpfBtn().Style = EQFilterButtonActiveStyle
		} else {
			dv.hpfBtn().Style = InstButtonStyle
		}
	}
	if dv.lpfBtn() != nil {
		if dv.activeLPFEnabled() {
			dv.lpfBtn().Style = EQFilterButtonActiveStyle
		} else {
			dv.lpfBtn().Style = InstButtonStyle
		}
	}
}

// toggleEQBandMute toggles the mute state for a specific EQ band on the active channel.
// This is called via OnClick callback to ensure single-toggle behavior.
func (dv *DrumView) toggleEQBandMute(band int) {
	ch := dv.activeEQChannel()
	changed := false
	if ch == channel.Master {
		if band >= 0 && band < len(dv.eqBandMuted()) {
			dv.eqBandMuted()[band] = !dv.eqBandMuted()[band]
			dv.applyMasterEQ()
			dv.logger.Debugf("[drumview] EQ band %d mute toggled: %v", band, dv.eqBandMuted()[band])
			changed = true
		}
	} else {
		// Per-instrument EQ
		if eq := dv.activeEQ(); eq != nil && band >= 0 && band < len(eq.BandMuted) {
			eq.BandMuted[band] = !eq.BandMuted[band]
			if inst, ok := ch.Instrument(); ok {
				audio.ApplyEQSettings(inst)
			}
			dv.eqCurveDirty = true
			dv.logger.Debugf("[drumview] EQ band %d mute toggled for %s: %v", band, ch, eq.BandMuted[band])
			changed = true
		}
	}
	if changed {
		dv.commitEQBandMute(band)
	}
}

// onRowInstrumentChanged is the single notification point for "row[r]'s
// instrument id changed from oldID to newID". Callers must invoke this
// onAudibleSetChanged is the sync notification for "which rows are audible
// just changed" — a mute or a solo. Direct call, not hooks.Bus: the EQ panel
// must observe it before the next Draw, and a worker-pool handler cannot
// promise that (CLAUDE.md, Event Notification).
//
// Every mute/solo path must call this. A mute that skips it leaves the
// Spectrum tab describing a row that is no longer in the mix, for the length
// of the banner dwell and the peak-hold.
//
// Reachable from Game.Update under seqMu, so it must not re-acquire it.
func (dv *DrumView) onAudibleSetChanged() {
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.InvalidateSpectrumReadout()
	}
}

// after the row's Instrument and Name fields are updated. It runs
// synchronously on the UI goroutine, so the EQ panel observes the new
// value before the next Draw. Subscribers added here must not re-acquire
// seqMu (this is reachable from Game.Update under that lock).
func (dv *DrumView) onRowInstrumentChanged(row int, oldID, newID instrument.ID) {
	if oldID == newID {
		return
	}
	if dv.eqActiveChannel == channel.Of(oldID) && oldID != "" {
		dv.setEQActiveChannel(channel.Of(newID))
		if dv.eqPanelZone != nil && dv.eqPanelZone.ChannelDropdownOpen() {
			dv.eqPanelZone.CloseChannelDropdown()
		}
	}
	name := ""
	if row >= 0 && row < len(dv.Rows) {
		name = dv.Rows[row].Name
	}
	emitRowInstrumentChange(row, oldID, newID, name)
}
