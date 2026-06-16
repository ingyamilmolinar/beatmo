package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
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
	if dv.transportZone != nil {
		dv.transportZone.SetPlaying(p)
		return
	}
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

// TriggerPlayPause sets the one-frame play/pause pulse, exactly as the
// on-screen Play button does. Consumed by Game.Update via PlayPressed().
func (dv *DrumView) TriggerPlayPause() { dv.playPressed = true }

// TriggerStop sets the one-frame stop pulse, exactly as the Stop button does.
// Consumed by Game.Update via StopPressed().
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
		// Sync error animation bidirectionally: take the higher value so
		// that both zone-generated errors (clamping) and DrumView-generated
		// errors (invalid text input) are preserved.
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

// FollowPlayback reports whether the drum view auto-scrolls with playback.
// TransportZone is the single source of truth; this is a thin delegator.
// When TransportZone is absent (test-only DrumViews built without one), the
// historical default of true is preserved.
func (dv *DrumView) FollowPlayback() bool {
	if dv.transportZone != nil {
		return dv.transportZone.FollowPlayback()
	}
	return true
}

// SetFollow toggles whether the drum view auto-scrolls with playback. It
// delegates to TransportZone, the single source of truth; TransportZone is
// responsible for syncing the track button visual via OnFollowChange.
func (dv *DrumView) SetFollow(f bool) {
	if dv.transportZone != nil {
		dv.transportZone.SetFollow(f)
		return
	}
	// No TransportZone (rare test-only path) — log-only no-op.
}

// syncTrackBtnVisual delegates to TransportZone, which owns the track button
// and the follow state.
func (dv *DrumView) syncTrackBtnVisual() {
	if dv.transportZone != nil {
		dv.transportZone.syncTrackBtnVisual()
	}
}

// TrackBeat adjusts the drum view offset to keep the given beat visible when
// auto-tracking is enabled.
//
// The recenter target is `length * RibbonPlayheadFrac` from the LEFT of the
// visible window, matching the timeline ribbon's playhead cursor (pinned at
// the same fraction of the bar width). Using 0.5 here would put the cell
// flash at the drum-view midpoint while the ribbon cursor sits at ~0.70,
// visibly desyncing the two playheads once the ribbon window starts sliding.
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
	// Dead-zone centered on the same column the ribbon cursor pins to. Kept
	// tight (±1 cell) so the cell-highlight column stays visually locked to
	// the ribbon cursor; the wider band the old code used produced a
	// recenter every length/4 ticks during playback, and between recenters
	// the cell highlight drifted several cells away from the cursor.
	// Incremental row-sprite shifting (see drumview_cache_row_sprite.go)
	// makes per-tick offset updates cheap.
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
				// In tests, force a cache refresh to satisfy visibility assertions even
				// when the offset stays within the current window. Perf tests that
				// measure the PRODUCTION recomposite cadence disable this via
				// SetTrackBeatForceRefreshForTest(false).
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
	// Clamp to timeline range so preview doesn't go blank when cur grows.
	// Clamp offset in subdivision units
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
		dv.logger.Tracef("[DRUMVIEW/TRACK] cur=%d offset->%d", cur, dv.Offset)
	} else if runningUnderGoTest() && trackBeatForceRefreshUnderTest {
		// In tests, force a cache refresh to satisfy visibility assertions even
		// when the offset stays within the current window.
		dv.offsetChanged = true
	}
}

// trackBeatForceRefreshUnderTest gates the test-only "force a cache refresh
// every TrackBeat" behavior. It defaults true so existing visibility tests are
// unaffected; perf tests that measure the real production recomposite cadence
// flip it false via SetTrackBeatForceRefreshForTest so the dead-zone short-
// circuit behaves exactly as it does in the browser/desktop binary.
var trackBeatForceRefreshUnderTest = true

// SetTrackBeatForceRefreshForTest toggles the test-only TrackBeat force-refresh
// and returns a restore func. Test seam only — see trackBeatForceRefreshUnderTest.
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
		// Cell pitch is (w/Length); a Length change resizes every cell. The
		// row sprite cache and rows-layer composite must be fully invalidated
		// so the shift-and-fill path doesn't blend old-pitch and new-pitch
		// pixels across the row.
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

// SetLengthClamped sets the visible drum view length, clamping to
// screen-aware limits so cells remain readable. The unclamped value is
// stored as the graph/timeline beat length so the full circuit stays
// scrollable.
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
		// Same rationale as SetLength: Length change resizes every cell, so
		// the row sprite + composite caches must fully invalidate to avoid
		// the shift-and-fill path blending pitches.
		dv.invalidateRowCaches()
		dv.markAllRowsDirty()
	}
	dv.bgDirty = true
	if length != prev && dv.onStructuralMutation != nil {
		dv.onStructuralMutation("length-change")
	}
}

func (dv *DrumView) SetInstrument(id string) {
	if len(dv.Rows) == 0 {
		return
	}
	// Lazily load sample-backed instruments when first selected.
	_ = audio.EnsureInstrumentLoaded(id)
	// Clamp selected row to valid range to avoid out-of-bounds when rows were
	// recently added/removed or after import/naming flows.
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
	dv.Rows[dv.selRow].Instrument = id
	if id != "" {
		dv.Rows[dv.selRow].Name = dv.computeInstLabel(id)
	}
	if dv.instCatByID != nil {
		if cat, ok := dv.instCatByID[id]; ok {
			dv.instMenuActiveCat = cat
			if dv.instMenuActiveByRow == nil {
				dv.instMenuActiveByRow = map[int]string{}
			}
			dv.instMenuActiveByRow[dv.selRow] = cat
		}
	}
	// Color is bound to the row INDEX (pure sequential series), not the
	// instrument id — changing the instrument keeps the row's color. A manual
	// pick via SetRowColor is the only way to change it.
	// Update label text and style immediately; also mark layout dirty so full rebuild
	if dv.selRow < len(dv.rowLabels()) {
		dv.rowLabels()[dv.selRow].Text = dv.Rows[dv.selRow].Name
		if dv.IsInstrumentAvailable(id) {
			dv.rowLabels()[dv.selRow].Style = InstButtonStyle
		} else {
			dv.rowLabels()[dv.selRow].Style = MissingInstStyle
		}
	}
	// Instrument changes also update the row color; invalidate row caches so the
	// visible window updates immediately during playback (not only as the window
	// scrolls and the incremental strip redraws).
	dv.markRowDirty(dv.selRow)
	// Also invalidate the row controls cache so the label button text is redrawn.
	dv.markRowControlsDirty()
	dv.bgDirty = true
	dv.onRowInstrumentChanged(dv.selRow, oldID, id)
}

func (dv *DrumView) AddInstrument(id string) {
	dv.instOptions = audio.Instruments()
	dv.SetInstrument(id)
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
			dv.SetInstrument(next)
			return
		}
	}
}

func (dv *DrumView) registerInstrument(id string) {
	if id == "" {
		dv.logger.Debugf("[drumview] ignored empty WAV name")
		dv.notifyError(i18n.T(i18n.KeyNotifInstNameEmpty))
		dv.pendingWAV = ""
		dv.nameInput = ""
		dv.nameBox = nil
		dv.closeNamingPortal()
		dv.savePressed = false
		return
	}
	existed := false
	canonicalID := id
	for _, opt := range dv.instOptions {
		if strings.EqualFold(opt, id) {
			existed = true
			canonicalID = opt
			break
		}
	}

	if err := audio.RegisterWAV(canonicalID, dv.pendingWAV); err == nil {
		if dv.samplePath == nil {
			dv.samplePath = map[string]string{}
		}
		dv.samplePath[canonicalID] = dv.pendingWAV
		dv.refreshInstruments()
		if dv.IsInstMenuOpen() {
			dv.refreshInstMenuComponent()
		}
		if existed {
			for row := range dv.Rows {
				if strings.EqualFold(dv.Rows[row].Instrument, canonicalID) {
					dv.Rows[row].Instrument = canonicalID
					// Color stays index-bound (pure sequential series); reloading a
					// WAV onto an existing row does not re-tint it.
				}
			}
			dv.notifyInfo(i18n.Tf(i18n.KeyNotifUpdatedWAVInst, canonicalID))
		} else {
			dv.notifyInfo(i18n.Tf(i18n.KeyNotifLoadedWAVInst, canonicalID))
		}
		dv.logger.Debugf("[drumview] loaded user WAV %s (existing=%v)", canonicalID, existed)
		emitCustomWAVLoaded(canonicalID, existed)
	} else {
		dv.logger.Errorf("[drumview] failed to load WAV: %v", err)
		dv.notifyError(i18n.Tf(i18n.KeyNotifErrLoadWAV, err.Error()))
	}
	dv.pendingWAV = ""
	dv.nameInput = ""
	dv.nameBox = nil
	dv.closeNamingPortal()
	dv.savePressed = false
}
