package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// 0 → −12 dB, 0.5 → 0 dB (unity), 1 → +12 dB.
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
	// masterEQBandState, not eqBandGainsDB: that is the working copy of the channel on screen,
	// which may be an instrument.
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

// Flat means NO processor, not a unity chain: a unity biquad chain differs in the last bits,
// and a project without an `eq` block must import bit-exact.
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

func (dv *DrumView) clearRowEQ(row int) {
	if row < 0 || row >= len(dv.Rows) || dv.Rows[row].Instrument == "" {
		return
	}
	*rowEQ(dv.Rows[row]) = *audio.NewEQSettings()
	dv.applyRowEQ(row)
}

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

func (dv *DrumView) setEQActiveChannel(id channel.ID) {
	if id.IsZero() {
		return // names no channel
	}
	prev := dv.activeEQChannel()
	channelChanged := prev != id

	if channelChanged {
		// Disable analyzers on the previous channel to save FFT CPU.
		audio.SetAnalyzerEnabled(prev, false)
		// Flush the working copy first, or switching away from Master loses its gains.
		dv.saveZoneBandState(prev)
	}

	dv.eqActiveChannel = id

	// The zone's channel must follow every programmatic switch, not just dropdown clicks:
	// Wave, Spectrum and the Synth tab read it.
	if dv.eqPanelZone != nil {
		// Through the zone's setter, never the field: it re-baselines the Spectrum readout, whose
		// anchor only rises, so skipping it measures the new channel against the old one's peak.
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

	var gains []float64
	var muted []bool
	if channelChanged {
		gains, muted = dv.loadChannelBandState(id)
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.SyncBandState(gains, muted)
		}
	} else if id.IsMaster() {
		// Same master channel: the zone is the authoritative store, so nothing needs syncing.
		if dv.eqPanelZone != nil {
			muted = dv.eqPanelZone.bandMuted
		}
	} else {
		// External code may have written the row's EQ directly.
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

	// The Synth analyzer is required too: without it the Chain tab's Synth/AntiPop traces go
	// dark on WASM.
	active := dv.activeEQChannel()
	_ = audio.EnableChannelAnalyzer(active, 512)
	_ = audio.EnablePreEQAnalyzer(active, 512)
	_ = audio.EnableSynthAnalyzer(active, 512)
	audio.SetAnalyzerEnabled(active, true)
	dv.eqCurveDirty = true
}

// The single chokepoint for selecting a channel across every audio-panel tab, so the dropdown
// stays the one source of truth. The zero ID is ignored.
func (dv *DrumView) selectAudioChannel(id channel.ID) {
	if id.IsZero() {
		return
	}
	dv.setEQActiveChannel(id)
	if svc := audio.AnalyzerService(); svc != nil {
		if id.IsMaster() {
			svc.SetDetailChannel(channel.ID{}) // no per-row detail: Master is the state's own
		} else {
			svc.SetDetailChannel(id)
		}
	}
	// WASM and stubbed builds have no scope service and rely on the zone-local instrumentID below.
	if svc := audio.ScopeService(); svc != nil {
		svc.SetInstrument(id)
	}
	if dv.eqPanelZone != nil {
		if z := dv.eqPanelZone.chainZone; z != nil {
			z.instrumentID = id
		}
	}
}

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

// Master returns fresh copies so SyncBandState never aliases the master store.
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

// Never eqBandGainsDB, the working copy of whichever channel is on screen. While Master is
// shown the zone IS the store; otherwise masterGainsDB is. The slices alias the store: do not
// keep them across a channel switch.
func (dv *DrumView) masterEQBandState() (gains []float64, muted []bool) {
	if ch := dv.activeEQChannel(); ch.IsMaster() {
		return dv.eqBandGainsDB(), dv.eqBandMuted()
	}
	dv.ensureMasterBandState()
	return dv.masterGainsDB, dv.masterMuted
}

func (dv *DrumView) setMasterEQBandState(gains []float64, muted []bool) {
	dv.ensureMasterBandState()
	copy(dv.masterGainsDB, gains)
	copy(dv.masterMuted, muted)
	if ch := dv.activeEQChannel(); ch.IsMaster() {
		copy(dv.eqBandGainsDB(), gains)
		copy(dv.eqBandMuted(), muted)
	}
}

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

// nil while the master is on screen (its filters live on the DrumView).
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

// Every mute/solo path must call this, directly rather than through hooks.Bus, so the EQ
// panel sees the change before the next Draw. Must not re-acquire seqMu.
func (dv *DrumView) onAudibleSetChanged() {
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.InvalidateSpectrumReadout()
	}
}

// Every site that changes a row's instrument id must call this. Synchronous and reachable
// under seqMu, so subscribers must not re-acquire it.
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
