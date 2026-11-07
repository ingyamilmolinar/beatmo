package ui

import (
	"hash/crc32"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func (dv *DrumView) PlayPressed() bool {
	if dv.transportZone != nil {
		if dv.transportZone.PlayPressed() {
			dv.playPressed = false
			return true
		}
	}
	if dv.playPressed {
		dv.playPressed = false
		return true
	}
	return false
}

// SetPlaying updates the play button label to reflect playback state.
func (dv *DrumView) SetPlaying(p bool) {
	dv.isPlaying = p
	// The selector is disabled while playing, so close its menu before a stale pick slips through.
	if p && dv.subdivMenuComp != nil && dv.subdivMenuComp.IsOpen() {
		dv.subdivMenuComp.Close()
		dv.closeSubdivMenuPortal()
	}
	if dv.transportZone != nil {
		dv.transportZone.SetPlaying(p)
		return
	}
	dv.playBtn().SetToggled(p) // active state, as TransportZone.SetPlaying
	// DESIGN.md §0/§5: icon-only button — never raw Unicode in Text.
	dv.playBtn().Text = ""
	if p {
		dv.playBtn().Icon = string(IconPause)
	} else {
		dv.playBtn().Icon = string(IconPlay)
	}
}

func (dv *DrumView) StopPressed() bool {
	if dv.transportZone != nil {
		if dv.transportZone.StopPressed() {
			dv.stopPressed = false
			return true
		}
	}
	if dv.stopPressed {
		dv.stopPressed = false
		return true
	}
	return false
}

func (dv *DrumView) TriggerPlayPause() { dv.playPressed = true }

func (dv *DrumView) TriggerStop() { dv.stopPressed = true }

// RecordPressed returns true (once) if the record button was pressed.
func (dv *DrumView) RecordPressed() bool {
	if dv.transportZone != nil {
		return dv.transportZone.RecordPressed()
	}
	return false
}

// SetRecording updates the record button visual state.
func (dv *DrumView) SetRecording(rec bool) {
	if dv.transportZone != nil {
		dv.transportZone.SetRecording(rec)
	}
}

// IsRecording returns whether the UI is showing recording state.
func (dv *DrumView) IsRecording() bool {
	if dv.transportZone != nil {
		return dv.transportZone.IsRecording()
	}
	return false
}

func (dv *DrumView) BPM() int {
	if dv.transportZone != nil {
		return dv.transportZone.BPM()
	}
	return dv.bpm
}

func (dv *DrumView) SetBPM(b int) {
	if dv.transportZone != nil {
		prev := dv.BPM()
		dv.transportZone.SetBPM(b)
		dv.bpm = dv.transportZone.BPM()
		// Take the higher value, keeping both clamp errors and text-input errors.
		if dv.transportZone.BPMErrorAnim() > dv.bpmErrorAnim {
			dv.bpmErrorAnim = dv.transportZone.BPMErrorAnim()
		}
		dv.transportZone.bpmErrorAnim = dv.bpmErrorAnim
		dv.secPerBeat = 60.0 / float64(dv.bpm)
		if prev != dv.bpm {
			dv.logger.Debugf("[drumview] BPM set: %d -> %d", prev, dv.bpm)
			if dv.onStructuralMutation != nil {
				dv.onStructuralMutation("bpm-change")
			}
		}
		return
	}
	if b < 1 {
		dv.bpm = 1
		dv.bpmErrorAnim = 1
		return
	}
	if b > maxBPM {
		dv.bpm = maxBPM
		dv.bpmErrorAnim = 1
		return
	}
	prev := dv.bpm
	dv.logger.Debugf("[drumview] BPM set: %d -> %d", prev, b)
	dv.bpm = b
	dv.secPerBeat = 60.0 / float64(dv.bpm)
	if dv.bpmBox() != nil && !dv.bpmBox().Focused() {
		dv.bpmBox().SetText(strconv.Itoa(dv.bpm))
	}
	if prev != b && dv.onStructuralMutation != nil {
		dv.onStructuralMutation("bpm-change")
	}
}

func (dv *DrumView) OffsetChanged() bool {
	if dv.offsetChanged {
		dv.offsetChanged = false
		return true
	}
	return false
}

// TransportZone owns the state; true for DrumViews built without one (tests).
func (dv *DrumView) FollowPlayback() bool {
	if dv.transportZone != nil {
		return dv.transportZone.FollowPlayback()
	}
	return true
}

func (dv *DrumView) SetFollow(f bool) {
	if dv.transportZone != nil {
		dv.transportZone.SetFollow(f)
		return
	}
	// No TransportZone (rare test-only path) — log-only no-op.
}

func (dv *DrumView) syncTrackBtnVisual() {
	if dv.transportZone != nil {
		dv.transportZone.syncTrackBtnVisual()
	}
}

// Recenters to RibbonPlayheadFrac of the window, where the ribbon's cursor is pinned; 0.5 would
// put the cell flash somewhere the ribbon cursor is not.
func (dv *DrumView) TrackBeat(cur int) {
	if !dv.FollowPlayback() {
		return
	}
	frac := RuntimeProf().RibbonPlayheadFrac
	if frac <= 0 || frac >= 1 {
		frac = 0.70
	}
	// Ensure timeline extends far enough ahead of the playhead (beats) for clamping.
	units := float64(max1(dv.timelineUnitsPerBeat))
	length := float64(dv.Length)
	lengthBeats := length / units
	tb := int(math.Ceil(float64(cur)/units + lengthBeats))
	if tb > dv.timelineBeats {
		dv.timelineBeats = tb
	}
	// A tight (±1 cell) dead zone keeps the highlighted column locked to the ribbon cursor;
	// incremental sprite shifting makes per-tick offsets cheap.
	if dv.Length > 0 {
		const recenterDeadZone = 1
		targetCol := int(math.Round(float64(dv.Length) * frac))
		left := dv.Offset + targetCol - recenterDeadZone
		right := dv.Offset + targetCol + recenterDeadZone
		if left < dv.Offset {
			left = dv.Offset
		}
		if right > dv.Offset+dv.Length-1 {
			right = dv.Offset + dv.Length - 1
		}
		if left <= right && cur >= left && cur <= right {
			if runningUnderGoTest() && trackBeatForceRefreshUnderTest {
				// Tests force a refresh for visibility assertions; perf tests disable it to measure the real cadence.
				dv.offsetChanged = true
			}
			return
		}
	}
	floatCur := float64(cur)
	desiredF := floatCur - length*frac
	if desiredF < 0 {
		desiredF = 0
	}
	desired := int(math.Round(desiredF))
	// Clamped so the preview never goes blank as cur grows.
	maxOff := int(math.Round(float64(dv.timelineBeats)*units - length))
	if maxOff < 0 {
		maxOff = 0
	}
	if desired > maxOff {
		desired = maxOff
	}
	if dv.Offset != desired {
		dv.Offset = desired
		dv.offsetChanged = true
		dv.logger.Tracef("[drumview/track] cur=%d offset->%d", cur, dv.Offset)
	} else if runningUnderGoTest() && trackBeatForceRefreshUnderTest {
		dv.offsetChanged = true
	}
}

// Perf tests turn this off so the dead-zone short-circuit behaves as in the real binary.
var trackBeatForceRefreshUnderTest = true

func SetTrackBeatForceRefreshForTest(v bool) func() {
	prev := trackBeatForceRefreshUnderTest
	trackBeatForceRefreshUnderTest = v
	return func() { trackBeatForceRefreshUnderTest = prev }
}

func (dv *DrumView) SetLength(length int) {
	if length < 1 {
		length = 1
	}
	prev := dv.Length
	if length != dv.Length {
		dv.logger.Debugf("[drumview] length set: %d -> %d", dv.Length, length)
	}
	dv.Length = length
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
		r.CellTypes = make([]model.NodeType, dv.Length)
	}
	dv.SetBeatLength(dv.Length)
	if length != prev {
		// A Length change resizes every cell, so shift-and-fill would mix old- and new-pitch pixels.
		dv.invalidateRowCaches()
		dv.markAllRowsDirty()
	}
	dv.bgDirty = true
	if length != prev {
		emitLengthChange(length)
		if dv.onStructuralMutation != nil {
			dv.onStructuralMutation("length-change")
		}
	}
}

// The unclamped value stays the timeline length, so the whole circuit remains scrollable.
func (dv *DrumView) SetLengthClamped(length int) {
	if length < 1 {
		length = 1
	}
	// Store the unclamped value so the full circuit is scrollable.
	dv.SetBeatLength(length)
	// Clamp the visible window.
	length = dv.clampLength(length)
	prev := dv.Length
	if length != dv.Length {
		dv.logger.Debugf("[drumview] length set (clamped): %d -> %d", dv.Length, length)
	}
	dv.Length = length
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
		r.CellTypes = make([]model.NodeType, dv.Length)
	}
	if length != prev {
		// A Length change resizes every cell, so shift-and-fill would mix old- and new-pitch pixels.
		dv.invalidateRowCaches()
		dv.markAllRowsDirty()
	}
	dv.bgDirty = true
	if length != prev && dv.onStructuralMutation != nil {
		dv.onStructuralMutation("length-change")
	}
}

// rowUsesInstrument reports whether any row other than skip plays id.
func (dv *DrumView) rowUsesInstrument(id instrument.ID, skip int) bool {
	for i, r := range dv.Rows {
		if i != skip && r != nil && r.Instrument == id {
			return true
		}
	}
	return false
}

func (dv *DrumView) SetInstrument(id instrument.ID) {
	if len(dv.Rows) == 0 {
		return
	}
	// Lazily load sample-backed instruments when first selected.
	_ = audio.EnsureInstrumentLoaded(id)
	if dv.selRow < 0 || dv.selRow >= len(dv.Rows) {
		if len(dv.Rows) == 0 {
			return
		}
		dv.selRow = len(dv.Rows) - 1
	}
	dv.logger.Debugf("[drumview] instrument set row=%d id=%s", dv.selRow, id)
	oldID := dv.Rows[dv.selRow].Instrument
	if oldID != id && dv.onStructuralMutation != nil {
		dv.onStructuralMutation("instrument-change")
	}
	// A pick copies the library entry: when no other row uses id, a copy edited by rows that have
	// since moved off it starts over from the library.
	if oldID != id && id != "" && !dv.rowUsesInstrument(id, dv.selRow) && audio.InstrumentDocModified(id) {
		audio.ResetInstrumentParams(id)
	}
	dv.Rows[dv.selRow].Instrument = id
	if id != "" {
		dv.Rows[dv.selRow].Name = dv.computeInstLabel(id)
	}
	if dv.instCatByID != nil {
		if cat, ok := dv.instCatByID[id]; ok {
			dv.instMenuActiveCat = cat
			if dv.instMenuActiveByRow == nil {
				dv.instMenuActiveByRow = map[int]audio.Category{}
			}
			dv.instMenuActiveByRow[dv.selRow] = cat
		}
	}
	// Color belongs to the row index, not the instrument, so a swap keeps it.
	if dv.selRow < len(dv.rowLabels()) {
		dv.rowLabels()[dv.selRow].Text = dv.Rows[dv.selRow].Name
		if dv.IsInstrumentAvailable(id) {
			dv.rowLabels()[dv.selRow].Style = InstButtonStyle
		} else {
			dv.rowLabels()[dv.selRow].Style = MissingInstStyle
		}
	}
	// So the visible window updates during playback, not only as it scrolls.
	dv.markRowDirty(dv.selRow)
	// Also invalidate the row controls cache so the label button text is redrawn.
	dv.markRowControlsDirty()
	dv.bgDirty = true
	// Warm the new instrument's voices off the hot path, or the first notes after a switch render cold.
	if oldID != id && id != "" {
		audio.WarmInstrument(id, audio.DefaultWarmPitches)
	}
	dv.onRowInstrumentChanged(dv.selRow, oldID, id)
}

func (dv *DrumView) AddInstrument(id instrument.ID) {
	dv.instOptions = audio.Instruments()
	dv.pickInstrument(dv.selRow, id)
	// If this is a known custom sample id, try to re-register from cache.
	if dv.samplePath != nil {
		if p, ok := dv.samplePath[id]; ok && p != "" && !dv.IsInstrumentAvailable(id) {
			_ = audio.RegisterWAV(id, p)
			dv.refreshInstruments()
		}
	}
}

func (dv *DrumView) CycleInstrument() {
	if len(dv.instOptions) == 0 || len(dv.Rows) == 0 {
		return
	}
	cur := dv.Rows[dv.selRow].Instrument
	for i, id := range dv.instOptions {
		if id == cur {
			next := dv.instOptions[(i+1)%len(dv.instOptions)]
			dv.pickInstrument(dv.selRow, next)
			return
		}
	}
}

// Derived from the name alone, so a new file under the same name updates that instrument; never
// a built-in's id ("Kick.wav" is its own instrument).
func wavInstrumentID(name string) instrument.ID {
	return instrument.UserSampleID(crc32.ChecksumIEEE([]byte("wav|" + strings.ToLower(name))))
}

func (dv *DrumView) registerInstrument(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		dv.logger.Debugf("[drumview] ignored empty WAV name")
		dv.notifyErrorKey(i18n.KeyNotifInstNameEmpty)
		dv.pendingWAV = ""
		dv.nameInput = ""
		dv.nameBox = nil
		dv.closeNamingPortal()
		dv.savePressed = false
		return
	}
	id := wavInstrumentID(name)
	existed := slices.Contains(dv.instOptions, id)

	if err := audio.RegisterWAV(id, dv.pendingWAV); err == nil {
		if dv.samplePath == nil {
			dv.samplePath = map[instrument.ID]string{}
		}
		dv.samplePath[id] = dv.pendingWAV
		dv.applyInstrumentDisplayName(id, name)
		dv.refreshInstruments()
		if dv.IsInstMenuOpen() {
			dv.refreshInstMenuComponent()
		}
		if existed {
			dv.notifyInfoKey(i18n.KeyNotifUpdatedWAVInst, name)
		} else {
			dv.notifyInfoKey(i18n.KeyNotifLoadedWAVInst, name)
		}
		dv.logger.Debugf("[drumview] loaded user WAV %s as %s (existing=%v)", name, id, existed)
		emitCustomWAVLoaded(id, existed)
	} else {
		dv.logger.Errorf("[drumview] failed to load WAV: %v", err)
		dv.notifyErrorKey(i18n.KeyNotifErrLoadWAV, err.Error())
	}
	dv.pendingWAV = ""
	dv.nameInput = ""
	dv.nameBox = nil
	dv.closeNamingPortal()
	dv.savePressed = false
}
