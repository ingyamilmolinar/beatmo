package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

// activeEQChannel returns the current EQ channel ID, defaulting to "main".
func (dv *DrumView) activeEQChannel() string {
	if dv.eqActiveChannel == "" {
		return "main"
	}
	return dv.eqActiveChannel
}

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
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		return
	}
	bands := dv.buildFullEQBands(dv.eqBandGainsDB(), dv.eqBandMuted(), dv.hpfEnabled, dv.hpfCutoffHz, dv.lpfEnabled, dv.lpfCutoffHz)
	// Persist for tests/exports and push to audio engine.
	dv.eqApplied = bands
	audio.SetChannelEQ("main", audio.SampleRate(), bands...)
	// Ensure analyzers stay in the chain so waveform/spectrum remain live.
	_ = audio.EnableChannelAnalyzer("main", 512)
	_ = audio.EnablePreEQAnalyzer("main", 512)
	dv.eqCurveDirty = true
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.curveDirty = true
	}
}

// applyRowEQ applies EQ for a specific instrument row.
func (dv *DrumView) applyRowEQ(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	r := dv.Rows[row]
	if len(r.EQGainsDB) != len(eqBandDefs) {
		return
	}
	hpfHz := r.HPFCutoffHz
	if hpfHz <= 0 {
		hpfHz = 20
	}
	lpfHz := r.LPFCutoffHz
	if lpfHz <= 0 {
		lpfHz = 20000
	}
	bands := dv.buildFullEQBands(r.EQGainsDB, r.EQBandMuted, r.HPFEnabled, hpfHz, r.LPFEnabled, lpfHz)
	channelID := r.Instrument
	audio.SetChannelEQ(channelID, audio.SampleRate(), bands...)
	_ = audio.EnableChannelAnalyzer(channelID, 512)
	_ = audio.EnablePreEQAnalyzer(channelID, 512)
	dv.eqCurveDirty = true
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.curveDirty = true
	}
}

// buildEQBands constructs audio.EQBand slice from gains and muted arrays.
// Uses geometric mean for center frequency and constant Q=1.414 for 1-octave ISO bands.
func (dv *DrumView) buildEQBands(gains []float64, muted []bool) []audio.EQBand {
	bands := make([]audio.EQBand, 0, len(eqBandDefs))
	for i, def := range eqBandDefs {
		kind := audio.EQPeaking
		if i == 0 {
			kind = audio.EQLowShelf
		}
		if i == len(eqBandDefs)-1 {
			kind = audio.EQHighShelf
		}
		center := math.Sqrt(def.loHz * def.hiHz) // geometric mean
		q := 1.414                               // standard Q for 1-octave graphic EQ bands
		gain := 0.0
		if i < len(gains) {
			gain = gains[i]
		}
		isMuted := i < len(muted) && muted[i]
		bands = append(bands, audio.EQBand{
			Kind:   kind,
			Freq:   center,
			Q:      q,
			GainDB: gain,
			Muted:  isMuted,
		})
	}
	return bands
}

// applyEQ applies EQ to the currently active channel. For backwards
// compatibility, this function is called from existing code paths.
func (dv *DrumView) applyEQ() {
	ch := dv.activeEQChannel()
	if ch == "main" {
		dv.applyMasterEQ()
		return
	}
	// Find the row with this instrument ID
	for i, r := range dv.Rows {
		if r.Instrument == ch {
			dv.applyRowEQ(i)
			return
		}
	}
	// Fallback to master if no matching row
	dv.applyMasterEQ()
}

// setEQActiveChannel switches the EQ view to the specified channel.
// Pass "main" for master or an instrument ID for per-row EQ.
func (dv *DrumView) setEQActiveChannel(id string) {
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

	// Update button text
	if dv.eqChannelBtn() != nil {
		if id == "" || id == "main" {
			dv.eqChannelBtn().Text = "Master"
		} else {
			// Find row name for this instrument
			label := id
			for _, r := range dv.Rows {
				if r.Instrument == id {
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
	} else if id == "" || id == "main" {
		// Same master channel: the zone IS the authoritative store.
		// Sync dB input texts in case bandGainsDB was modified externally
		// (e.g., import writes directly into the shared backing array).
		if dv.eqPanelZone != nil {
			muted = dv.eqPanelZone.bandMuted
			dv.eqPanelZone.syncAllDBInputTexts()
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

	// Enable analyzers for the selected channel.
	active := dv.activeEQChannel()
	_ = audio.EnableChannelAnalyzer(active, 512)
	_ = audio.EnablePreEQAnalyzer(active, 512)
	audio.SetAnalyzerEnabled(active, true)
	dv.eqCurveDirty = true
}

// saveZoneBandState copies the zone's current working bandGainsDB/bandMuted
// back to the appropriate backing store (masterGainsDB for "main", or the
// matching DrumRow for per-instrument channels).
func (dv *DrumView) saveZoneBandState(ch string) {
	if dv.eqPanelZone == nil {
		return
	}
	z := dv.eqPanelZone
	if ch == "" || ch == "main" {
		dv.ensureMasterBandState()
		copy(dv.masterGainsDB, z.bandGainsDB)
		copy(dv.masterMuted, z.bandMuted)
	} else {
		for j, r := range dv.Rows {
			if r.Instrument == ch {
				dv.ensureRowEQ(j)
				dv.ensureRowEQMuted(j)
				copy(r.EQGainsDB, z.bandGainsDB)
				copy(r.EQBandMuted, z.bandMuted)
				break
			}
		}
	}
}

// loadChannelBandState returns the gains and muted slices for the given
// channel from the backing store. Returns fresh copies for "main" so
// SyncBandState doesn't alias the master store.
func (dv *DrumView) loadChannelBandState(id string) (gains []float64, muted []bool) {
	if id == "" || id == "main" {
		dv.ensureMasterBandState()
		gains = dv.masterGainsDB
		muted = dv.masterMuted
	} else {
		for j, r := range dv.Rows {
			if r.Instrument == id {
				dv.ensureRowEQ(j)
				dv.ensureRowEQMuted(j)
				gains = r.EQGainsDB
				muted = r.EQBandMuted
				break
			}
		}
	}
	return
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

	if current == "main" && len(dv.Rows) > 0 {
		// Switch to first instrument
		dv.setEQActiveChannel(dv.Rows[0].Instrument)
		return
	}

	// Find current position and advance
	for i, r := range dv.Rows {
		if r.Instrument == current {
			if i+1 < len(dv.Rows) {
				dv.setEQActiveChannel(dv.Rows[i+1].Instrument)
			} else {
				dv.setEQActiveChannel("main")
			}
			return
		}
	}

	// Default to main if current not found
	dv.setEQActiveChannel("main")
}

// ensureRowEQ initializes EQ gains for a row if not already set.
func (dv *DrumView) ensureRowEQ(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	r := dv.Rows[row]
	if len(r.EQGainsDB) != len(eqBandDefs) {
		r.EQGainsDB = make([]float64, len(eqBandDefs))
	}
}

// ensureRowEQMuted initializes EQ mute state for a row if not already set.
func (dv *DrumView) ensureRowEQMuted(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	r := dv.Rows[row]
	if len(r.EQBandMuted) != len(eqBandDefs) {
		r.EQBandMuted = make([]bool, len(eqBandDefs))
	}
}

// activeHPFEnabled returns whether HPF is enabled for the active channel.
func (dv *DrumView) activeHPFEnabled() bool {
	ch := dv.activeEQChannel()
	if ch == "main" {
		return dv.hpfEnabled
	}
	for _, r := range dv.Rows {
		if r.Instrument == ch {
			return r.HPFEnabled
		}
	}
	return false
}

// activeHPFCutoffHz returns the HPF cutoff for the active channel.
func (dv *DrumView) activeHPFCutoffHz() float64 {
	ch := dv.activeEQChannel()
	if ch == "main" {
		return dv.hpfCutoffHz
	}
	for _, r := range dv.Rows {
		if r.Instrument == ch {
			if r.HPFCutoffHz <= 0 {
				return 20
			}
			return r.HPFCutoffHz
		}
	}
	return 20
}

// activeLPFEnabled returns whether LPF is enabled for the active channel.
func (dv *DrumView) activeLPFEnabled() bool {
	ch := dv.activeEQChannel()
	if ch == "main" {
		return dv.lpfEnabled
	}
	for _, r := range dv.Rows {
		if r.Instrument == ch {
			return r.LPFEnabled
		}
	}
	return false
}

// activeLPFCutoffHz returns the LPF cutoff for the active channel.
func (dv *DrumView) activeLPFCutoffHz() float64 {
	ch := dv.activeEQChannel()
	if ch == "main" {
		return dv.lpfCutoffHz
	}
	for _, r := range dv.Rows {
		if r.Instrument == ch {
			if r.LPFCutoffHz <= 0 {
				return 20000
			}
			return r.LPFCutoffHz
		}
	}
	return 20000
}

// setActiveHPF sets the HPF state for the active channel.
func (dv *DrumView) setActiveHPF(enabled bool, cutoffHz float64) {
	ch := dv.activeEQChannel()
	if ch == "main" {
		dv.hpfEnabled = enabled
		dv.hpfCutoffHz = cutoffHz
	} else {
		for _, r := range dv.Rows {
			if r.Instrument == ch {
				r.HPFEnabled = enabled
				r.HPFCutoffHz = cutoffHz
				break
			}
		}
	}
}

// setActiveLPF sets the LPF state for the active channel.
func (dv *DrumView) setActiveLPF(enabled bool, cutoffHz float64) {
	ch := dv.activeEQChannel()
	if ch == "main" {
		dv.lpfEnabled = enabled
		dv.lpfCutoffHz = cutoffHz
	} else {
		for _, r := range dv.Rows {
			if r.Instrument == ch {
				r.LPFEnabled = enabled
				r.LPFCutoffHz = cutoffHz
				break
			}
		}
	}
}

// toggleHPF toggles the high-pass filter for the active channel.
func (dv *DrumView) toggleHPF() {
	enabled := !dv.activeHPFEnabled()
	dv.setActiveHPF(enabled, dv.activeHPFCutoffHz())
	dv.applyEQ()
	dv.eqCurveDirty = true
}

// toggleLPF toggles the low-pass filter for the active channel.
func (dv *DrumView) toggleLPF() {
	enabled := !dv.activeLPFEnabled()
	dv.setActiveLPF(enabled, dv.activeLPFCutoffHz())
	dv.applyEQ()
	dv.eqCurveDirty = true
}

// buildFullEQBands constructs the full EQ band chain including HPF and LPF filters.
func (dv *DrumView) buildFullEQBands(gains []float64, muted []bool, hpfOn bool, hpfHz float64, lpfOn bool, lpfHz float64) []audio.EQBand {
	bands := dv.buildEQBands(gains, muted)
	if hpfOn && hpfHz > 20 {
		bands = append([]audio.EQBand{{Kind: audio.EQHighpass, Freq: hpfHz, Q: 0.707}}, bands...)
	}
	if lpfOn && lpfHz < 20000 {
		bands = append(bands, audio.EQBand{Kind: audio.EQLowpass, Freq: lpfHz, Q: 0.707})
	}
	return bands
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
	if ch == "main" {
		if band >= 0 && band < len(dv.eqBandMuted()) {
			dv.eqBandMuted()[band] = !dv.eqBandMuted()[band]
			dv.applyMasterEQ()
			dv.logger.Debugf("[drumview] EQ band %d mute toggled: %v", band, dv.eqBandMuted()[band])
		}
	} else {
		// Per-instrument EQ
		for j, r := range dv.Rows {
			if r.Instrument == ch {
				dv.ensureRowEQ(j)
				dv.ensureRowEQMuted(j)
				if band >= 0 && band < len(r.EQBandMuted) {
					r.EQBandMuted[band] = !r.EQBandMuted[band]
					dv.applyRowEQ(j)
					dv.logger.Debugf("[drumview] EQ band %d mute toggled for %s: %v", band, ch, r.EQBandMuted[band])
				}
				break
			}
		}
	}
}

// onRowInstrumentChanged is the single notification point for "row[r]'s
// instrument id changed from oldID to newID". Callers must invoke this
// after the row's Instrument and Name fields are updated. It runs
// synchronously on the UI goroutine, so the EQ panel observes the new
// value before the next Draw. Subscribers added here must not re-acquire
// seqMu (this is reachable from Game.Update under that lock).
func (dv *DrumView) onRowInstrumentChanged(row int, oldID, newID string) {
	if oldID == newID {
		return
	}
	if dv.eqActiveChannel == oldID && oldID != "" {
		dv.setEQActiveChannel(newID)
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
