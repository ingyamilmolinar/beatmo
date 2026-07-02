package ui

import (
	"fmt"
	"hash/crc32"
	"image"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// samplerSource identifies where the sampler's working buffer came from.
type samplerSource int

const (
	samplerSourceSynth samplerSource = iota // captured from an instrument's rendered one-shot
	samplerSourceWAV                        // loaded from a WAV file (Phase 2)
)

// Knob indices for the Sampler control row. The order is the on-screen
// left-to-right order and the index used by samplerKnobHitAdapter.
const (
	samplerKnobStart = iota
	samplerKnobEnd
	samplerKnobTranspose
	samplerKnobDetune
	samplerKnobGain
	samplerKnobCount
)

// Pitch / gain knob ranges (see setKnob/knobValue).
const (
	samplerTransposeRange = 24.0  // +/- semitones at knob extremes
	samplerDetuneRange    = 100.0 // +/- cents at knob extremes
	samplerGainMinDB      = -24.0
	samplerGainMaxDB      = 6.0
	samplerFadeMs         = 5.0 // fade length applied when the Fade toggle is on
)

// samplerState is the canonical model for the Sampler tab. The UI knobs and
// trim handles derive from these fields every Layout; the baked buffer is
// produced by audio.BakeSample. It is self-contained (no UI dependencies in
// its logic) so the edit/capture/save behavior is testable without a DrumView.
type samplerState struct {
	source        samplerSource
	captureID     string // instrument id the buffer was captured from / Save overrides
	raw           []float32
	rawSampleRate int

	// srcSignature is the synth-signal fingerprint of the source instrument at
	// the moment raw was last captured (synth sources only). ensureSamplerLoaded
	// re-renders the one-shot whenever the live signature diverges from this, so
	// the waveform tracks synth edits in real time without a manual Preview or
	// tab switch. Zero / unused for WAV sources (their PCM is fixed).
	srcSignature uint64

	// Edit params (canonical). Knobs and handles read/write these.
	//
	// Note on reverse: the Reverse control flips the working buffer in place
	// (reverseBuffer), so the visible waveform and the baked audio both reflect
	// it directly — and the playback highlight always sweeps forward (it has no
	// direction flag to honour). `reverse` tracks the NET flip parity: it feeds
	// editDescriptor() (the non-destructive synth path re-renders the recipe
	// forward at trigger time, so the descriptor must carry reversal
	// declaratively) AND drives the Reverse toggle's latched on/off visual, so
	// the button reads its reversed state like every other toggle.
	startFrac      float64
	endFrac        float64
	transposeSemis float64
	detuneCents    float64
	gainDB         float64
	normalize      bool
	fadeOn         bool
	reverse        bool

	// Layout rects (rebuilt each Layout by buildSamplerTab).
	headerRect      image.Rectangle
	waveformRect    image.Rectangle
	startHandleRect image.Rectangle
	endHandleRect   image.Rectangle
	metaRect        image.Rectangle // length/sample-rate readout strip

	knobs          []*Knob                           // samplerKnobCount knobs, reused across Layout
	knobStepBadges []*KnobStepBadge                  // index-aligned with knobs; one badge per knob
	knobCells      [samplerKnobCount]image.Rectangle // full per-knob cell (caption/label/badge center here, decoupled from the dial)
	readoutRects   [samplerKnobCount]image.Rectangle // caption hit-rects; populated each Draw

	// knobGrid scrolls the single-column knob list on mobile (desktop keeps the
	// side-by-side one-row layout and never uses it). Persisted across Layout so
	// the scroll position survives re-layout — mirrors the Synth tab's per-section
	// grids. knobGridRect is the area it lays the cells inside, reused by the
	// scroll body catch-all hit area.
	knobGrid     *ControlGrid
	knobGridRect image.Rectangle
	btns         []*Button // header + control-row buttons, rebuilt each Layout

	dragHandle int // -1 none, 0 start handle, 1 end handle

	// status is a transient one-line message shown in the waveform card —
	// e.g. when a synth capture returned no audio (the silent-no-op fix).
	// Empty means "no message".
	status string

	// waveF64 is a reusable float32→float64 scratch buffer for the waveform
	// trace renderer. Owned here (not re-allocated per Draw) so the Sampler
	// tab honours the audio-panel per-tab alloc budget. dispF32 is its
	// float32 sibling: waveFloat64 applies the amplitude overlays
	// (normalize/gain/fade) into it before widening, so the rendered trace
	// reflects the same button state the baked audio does.
	waveF64 []float64
	dispF32 []float32

	// playheads tracks the in-flight preview/playback lines. Each trigger gets
	// its own line so overlapping voices render as separate coloured lines.
	playheads samplerPlayheadTracker
}

// waveFloat64 returns the RENDERED signal as float64 for the trace renderer:
// the working buffer with the length-preserving amplitude overlays applied —
// normalize → gain → fade, the same transforms (and the same DSP functions)
// BakeSample applies to produce the audio. The Reverse order-edit already lives
// in s.raw, and trim is drawn as a shading overlay rather than truncating the
// trace, so the displayed waveform reflects every toggle button exactly as the
// sound does. Both backing arrays are cached so the per-frame Draw path
// allocates nothing once the buffer length is stable; with no overlay active it
// widens s.raw directly (the original fast path).
func (s *samplerState) waveFloat64() []float64 {
	n := len(s.raw)
	if cap(s.waveF64) < n {
		s.waveF64 = make([]float64, n)
	}
	s.waveF64 = s.waveF64[:n]

	src := s.raw
	if s.normalize || s.gainDB != 0 || s.fadeOn {
		if cap(s.dispF32) < n {
			s.dispF32 = make([]float32, n)
		}
		s.dispF32 = s.dispF32[:n]
		copy(s.dispF32, s.raw) // never mutate the source-of-truth buffer
		if s.normalize {
			audio.NormalizePeak(s.dispF32)
		}
		if s.gainDB != 0 {
			audio.ApplyGainDB(s.dispF32, float32(s.gainDB))
		}
		if s.fadeOn {
			sr := s.rawSampleRate
			if sr <= 0 {
				sr = audio.SampleRate()
			}
			audio.ApplyFades(s.dispF32, sr, samplerFadeMs, samplerFadeMs)
		}
		src = s.dispF32
	}
	for i, v := range src {
		s.waveF64[i] = float64(v)
	}
	return s.waveF64
}

// lengthSeconds returns the captured buffer's duration in seconds (0 when
// empty or the sample rate is unknown).
func (s *samplerState) lengthSeconds() float64 {
	sr := s.rawSampleRate
	if sr <= 0 || len(s.raw) == 0 {
		return 0
	}
	return float64(len(s.raw)) / float64(sr)
}

// trimSeconds returns the duration of the kept [start,end] region in seconds.
func (s *samplerState) trimSeconds() float64 {
	lo, hi := s.startFrac, s.endFrac
	if lo > hi {
		lo, hi = hi, lo
	}
	return s.lengthSeconds() * (hi - lo)
}

// audibleDurationSeconds returns how long the current edit's kept region takes
// to play, in seconds: the trimmed length scaled by the pitch ratio (a higher
// transpose/detune plays faster and shortens it, matching how BakeSample
// resamples). It is derived from current state, so it self-corrects whenever
// the trim handles, pitch knobs, or loaded sample change. Returns 0 when
// nothing audible is loaded — the playhead reads this to know its sweep length.
func (s *samplerState) audibleDurationSeconds() float64 {
	trim := s.trimSeconds()
	if trim <= 0 {
		return 0
	}
	ratio := math.Pow(2, (s.transposeSemis+s.detuneCents/100.0)/12.0)
	if ratio <= 0 {
		return trim
	}
	return trim / ratio
}

// samplerPlayheadFrac maps elapsed playback time to a [0,1] x-fraction over the
// waveform for the preview playhead line, and reports whether the line should
// be drawn at all. The line is visible only while playback is in progress
// (0 <= elapsed <= duration); a non-positive duration is never visible. lo/hi
// are the trim fractions (ordered here defensively so the line always lands in
// the kept region). The sweep is ALWAYS forward (lo→hi, left→right) regardless
// of whether the buffer was reversed — a reversed sound still plays front-to-
// back through the kept region, so the highlight must never run right→left.
func samplerPlayheadFrac(elapsedSec, durationSec, lo, hi float64) (float64, bool) {
	if durationSec <= 0 {
		return 0, false
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	p := elapsedSec / durationSec
	if p < 0 || p > 1 {
		return 0, false
	}
	return lo + p*(hi-lo), true
}

// reset restores default edit params (full untrimmed buffer, no pitch shift,
// unity gain). It preserves the loaded raw buffer.
func (s *samplerState) reset() {
	s.startFrac = 0
	s.endFrac = 1
	s.transposeSemis = 0
	s.detuneCents = 0
	s.gainDB = 0
	s.normalize = false
	s.fadeOn = false
	s.reverse = false
	s.dragHandle = -1
}

func clampUnit(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// setKnob maps a normalized [0,1] knob value to the corresponding edit param.
func (s *samplerState) setKnob(idx int, v float64) {
	v = clampUnit(v)
	switch idx {
	case samplerKnobStart:
		s.startFrac = v
	case samplerKnobEnd:
		s.endFrac = v
	case samplerKnobTranspose:
		s.transposeSemis = v*2*samplerTransposeRange - samplerTransposeRange
	case samplerKnobDetune:
		s.detuneCents = v*2*samplerDetuneRange - samplerDetuneRange
	case samplerKnobGain:
		s.gainDB = samplerGainMinDB + v*(samplerGainMaxDB-samplerGainMinDB)
	}
}

// knobValue is the inverse of setKnob: the normalized [0,1] value that
// represents the current edit param for the given knob.
func (s *samplerState) knobValue(idx int) float64 {
	switch idx {
	case samplerKnobStart:
		return clampUnit(s.startFrac)
	case samplerKnobEnd:
		return clampUnit(s.endFrac)
	case samplerKnobTranspose:
		return clampUnit((s.transposeSemis + samplerTransposeRange) / (2 * samplerTransposeRange))
	case samplerKnobDetune:
		return clampUnit((s.detuneCents + samplerDetuneRange) / (2 * samplerDetuneRange))
	case samplerKnobGain:
		return clampUnit((s.gainDB - samplerGainMinDB) / (samplerGainMaxDB - samplerGainMinDB))
	}
	return 0
}

// samplerKnobScale returns the real-unit domain for a sampler knob so the
// endless drag, step badge, and numeric editor operate in real units. The
// normalized knob Value still maps through setKnob/knobValue.
func samplerKnobScale(idx int) KnobScale {
	switch idx {
	case samplerKnobStart, samplerKnobEnd:
		return KnobScale{Min: 0, Max: 100, Unit: "%"}
	case samplerKnobTranspose:
		return KnobScale{Min: -samplerTransposeRange, Max: samplerTransposeRange, Unit: "st"}
	case samplerKnobDetune:
		return KnobScale{Min: -samplerDetuneRange, Max: samplerDetuneRange, Unit: "cents"}
	case samplerKnobGain:
		return KnobScale{Min: samplerGainMinDB, Max: samplerGainMaxDB, Unit: "dB"}
	}
	return KnobScale{Min: 0, Max: 1}
}

// samplerStepPrefName is the persistence key for a sampler knob's step rung.
func samplerStepPrefName(idx int) string {
	switch idx {
	case samplerKnobStart:
		return "sampler.start"
	case samplerKnobEnd:
		return "sampler.end"
	case samplerKnobTranspose:
		return "sampler.transpose"
	case samplerKnobDetune:
		return "sampler.detune"
	case samplerKnobGain:
		return "sampler.gain"
	}
	return "sampler.unknown"
}

// samplerKnobBipolar reports whether a sampler knob's range straddles zero
// and, if so, the normalized [0,1] position of that zero. Transpose/Detune are
// symmetric (±range → 0.5); Gain is asymmetric (-24..+6 dB → 0 dB at 0.8);
// Start/End are unipolar position fractions. Drives Knob.Bipolar/ZeroFrac so
// the neutral value reads as a centred (un-filled) arc.
func samplerKnobBipolar(idx int) (zeroFrac float64, bipolar bool) {
	switch idx {
	case samplerKnobTranspose, samplerKnobDetune:
		return 0.5, true
	case samplerKnobGain:
		return (0 - samplerGainMinDB) / (samplerGainMaxDB - samplerGainMinDB), true
	}
	return 0, false
}

// edit snapshots the current params into an audio.SampleEdit, ordering the
// trim fractions so start <= end.
func (s *samplerState) edit() audio.SampleEdit {
	lo, hi := s.startFrac, s.endFrac
	if lo > hi {
		lo, hi = hi, lo
	}
	fade := 0.0
	if s.fadeOn {
		fade = samplerFadeMs
	}
	// Reverse is intentionally NOT set here: reverseBuffer already flipped the
	// working buffer in place, so the kept region (and thus the bake) is reversed
	// at the source. Setting Reverse here too would double-reverse and cancel it.
	return audio.SampleEdit{
		StartFrac:      lo,
		EndFrac:        hi,
		TransposeSemis: s.transposeSemis,
		DetuneCents:    s.detuneCents,
		GainDB:         float32(s.gainDB),
		FadeInMs:       fade,
		FadeOutMs:      fade,
		Normalize:      s.normalize,
	}
}

// editDescriptor snapshots the current params for the NON-DESTRUCTIVE path,
// where the FORWARD source (recipe render, or a user sample's pristine PCM) is
// baked at trigger time. It is edit() plus a declarative Reverse — the
// working-buffer flip that edit() relies on cannot travel with a descriptor
// applied to a fresh forward render.
//
// Trim is expressed over the DISPLAYED working buffer, which is physically
// reversed when s.reverse is set; but BakeSample applies the descriptor to the
// forward source and trims BEFORE it reverses (trim → reverse). Selecting
// [lo,hi] of reverse(P) equals reverse(P[1-hi : 1-lo]), so when reverse is on we
// must MIRROR the trim fractions into the source frame — otherwise the audible
// trim lands on the wrong (mirror-image) region while the UI/preview show the
// selected one. loadEditDescriptor performs the inverse mirror on load.
func (s *samplerState) editDescriptor() audio.SampleEdit {
	e := s.edit()
	if s.reverse {
		e.StartFrac, e.EndFrac = 1-e.EndFrac, 1-e.StartFrac
		e.Reverse = true
	}
	return e
}

// loadEditDescriptor maps a saved SampleEdit back into the editor's
// knob-backing fields (the inverse of editDescriptor), so reopening the
// Sampler on a descriptor-bearing instrument shows the saved trim/pitch/gain.
// The working buffer must hold the RAW (un-edited) source render; a saved
// reverse is re-applied to it once so the displayed waveform matches.
func (s *samplerState) loadEditDescriptor(e audio.SampleEdit) {
	// Inverse of editDescriptor's source-frame mirror: a reversed descriptor
	// stores trim in the forward-source frame, so un-mirror it back to the
	// display frame the handles live in (1-EndFrac .. 1-StartFrac). Without this,
	// reopening a reversed+trimmed chop would show the handles on the wrong end.
	if e.Reverse {
		s.startFrac, s.endFrac = 1-e.EndFrac, 1-e.StartFrac
	} else {
		s.startFrac = e.StartFrac
		s.endFrac = e.EndFrac
	}
	s.transposeSemis = e.TransposeSemis
	s.detuneCents = e.DetuneCents
	s.gainDB = float64(e.GainDB)
	s.normalize = e.Normalize
	s.fadeOn = e.FadeInMs > 0 || e.FadeOutMs > 0
	if e.Reverse && !s.reverse {
		s.reverseBuffer() // flips raw for display AND sets s.reverse
	}
}

// samplerEditPersistFn persists a just-saved sample-edit descriptor through
// the registered SampleEditSaveSink (sampler_edit_sink.go); a nil sink makes
// it a no-op so the in-memory flow works without a store.
var samplerEditPersistFn = persistSampleEdit

// samplerEditDeleteFn removes the persisted descriptor on Reset, mirroring
// samplerEditPersistFn.
var samplerEditDeleteFn = deletePersistedSampleEdit

// samplerCaptureFn renders an instrument's one-shot PCM. Defaults to the
// platform audio.RenderInstrumentOneShot; swappable in tests so the
// empty-capture feedback path can be exercised without a real renderer.
var samplerCaptureFn = audio.RenderInstrumentOneShot

// samplerRawCaptureFn renders an instrument's one-shot IGNORING any saved
// sample-edit descriptor. ensureSamplerLoaded uses it for descriptor-bearing
// instruments so the editor shows the un-edited source waveform and overlays
// the saved edit itself (capturing the edited render would double-apply it).
var samplerRawCaptureFn = audio.RenderInstrumentOneShotRaw

// SwapSamplerRawCaptureFnForTest installs a test fake for the raw synth-capture
// renderer and returns the previous value, mirroring SwapSamplerCaptureFnForTest.
func SwapSamplerRawCaptureFnForTest(fn func(string) ([]float32, int)) func(string) ([]float32, int) {
	prev := samplerRawCaptureFn
	if fn != nil {
		samplerRawCaptureFn = fn
	}
	return prev
}

// SwapSamplerCaptureFnForTest installs a test fake for the synth-capture
// renderer and returns the previous value.
func SwapSamplerCaptureFnForTest(fn func(string) ([]float32, int)) func(string) ([]float32, int) {
	prev := samplerCaptureFn
	if fn != nil {
		samplerCaptureFn = fn
	}
	return prev
}

// captureFromSynth renders the given instrument's one-shot into the working
// buffer and resets the edit params to the full untrimmed region. When the
// renderer returns no audio (e.g. the browser synth hasn't rendered yet) the
// buffer is left empty and a status message is set so the UI can explain the
// no-op instead of silently doing nothing.
func (s *samplerState) captureFromSynth(id string) {
	pcm, sr := samplerCaptureFn(id)
	if len(pcm) == 0 {
		s.status = "Couldn't capture — play the sound once, then try again"
		return
	}
	s.raw = pcm
	s.rawSampleRate = sr
	s.captureID = id
	s.source = samplerSourceSynth
	s.status = ""
	s.reset()
}

// loadFromInstrument installs an instrument's audio as the working buffer and
// resets the edit params. Used by the dropdown-driven auto-load: synth-backed
// instruments arrive as a freshly-rendered one-shot (src=synth), user samples
// (WAV-loaded or previously saved) arrive straight from the canonical PCM store
// (src=WAV). captureID is set so a follow-up Save overrides this instrument.
func (s *samplerState) loadFromInstrument(id string, pcm []float32, sr int, src samplerSource) {
	s.raw = pcm
	s.rawSampleRate = sr
	s.captureID = id
	s.source = src
	s.status = ""
	s.reset()
}

// recaptureRaw swaps in a freshly-rendered one-shot for the SAME instrument
// while deliberately preserving the user's in-progress edit (trim handles,
// pitch, gain, toggles). Used when the source synth's signal changed and the
// waveform must track it live — unlike loadFromInstrument (a new selection),
// re-rendering the current sound must not discard the chop the user is shaping.
// Trim handles stay valid because start/end are fractions of the buffer length.
//
// The fresh render arrives FORWARD; the Reverse button is the source of truth
// for order, so when it is latched we re-apply the flip to keep the working
// buffer (and thus the displayed trace) consistent with the button — otherwise
// the audio (editDescriptor().Reverse) would play reversed while the waveform
// showed it forward.
func (s *samplerState) recaptureRaw(pcm []float32, sr int) {
	s.raw = pcm
	s.rawSampleRate = sr
	s.source = samplerSourceSynth
	s.status = ""
	if s.reverse {
		audio.ReverseSample(s.raw)
	}
}

// reverseBuffer flips the working buffer in place — the Reverse control is an
// ACTION (one click reverses the signal), not a stateful toggle. Because the
// reversal lives in the buffer, the visible waveform flips, the baked audio
// matches what is shown, and the playback highlight keeps sweeping forward
// (it has no direction flag). A no-op on an empty buffer. Note: a later live
// re-capture from a synth source (recaptureRaw) renders the new sound forward,
// so a manual reversal is not re-applied across a synth edit — the user re-
// clicks if they still want it reversed.
func (s *samplerState) reverseBuffer() {
	if len(s.raw) == 0 {
		return
	}
	audio.ReverseSample(s.raw)
	s.reverse = !s.reverse // net flip parity, consumed only by editDescriptor()
}

// loadPCM installs an externally-decoded buffer (WAV import, Phase 2).
func (s *samplerState) loadPCM(pcm []float32, sr int) {
	s.raw = pcm
	s.rawSampleRate = sr
	s.source = samplerSourceWAV
	s.status = ""
	s.reset()
}

// hasBuffer reports whether a working buffer is loaded.
func (s *samplerState) hasBuffer() bool { return len(s.raw) > 0 }

// bake renders the current edit into a final, self-contained PCM buffer.
func (s *samplerState) bake() ([]float32, int) {
	sr := s.rawSampleRate
	if sr <= 0 {
		sr = audio.SampleRate()
	}
	return audio.BakeSample(s.raw, sr, s.edit()), sr
}

// save overrides the captured instrument in place (same id), mirroring the
// Synth tab's "Save". No-op without a capture id.
//
// BOTH sources — NON-DESTRUCTIVE: the edit is stored as a per-instrument
// descriptor (audio.SetSampleEdit) and is ALREADY live by the time Save is
// pressed (commitSamplerEdit applied it per gesture). Save's job is durability,
// not application:
//
//   - SYNTH source: persist the descriptor; the recipe binding is KEPT so the
//     synth stays the source of truth and the dispatcher re-applies the edit to
//     the fresh recipe render at trigger time. Baking PCM here (the old behavior)
//     silently converted the instrument into a dead sample.
//   - WAV / user-sample source: persist the descriptor AND the pristine source
//     PCM (PersistUserSample) so the chop reproduces across sessions. Playback is
//     already driven live by reapplyUserSampleEdit; we do NOT bake destructively,
//     so the edit stays reversible and the pristine remains the source of truth.
func (s *samplerState) save() {
	if s.captureID == "" || !s.hasBuffer() {
		return
	}
	audio.SetSampleEdit(s.captureID, s.editDescriptor())
	samplerEditPersistFn(s.captureID)
	frames := 0
	if rec, ok := audio.UserSamplePCM(s.captureID); ok {
		audio.PersistUserSample(s.captureID)
		frames = len(rec.PCM)
	}
	hooks.PublishKind(hooks.EventSampleSaved, hooks.SamplePayload{
		SampleID: s.captureID,
		SourceID: s.captureID,
		Frames:   frames,
	})
}

// saveAs bakes the current edit and registers it as a brand-new instrument,
// returning its id. The new instrument becomes the capture target so a
// follow-up Save overrides it. No-op (returns "") without a buffer.
func (s *samplerState) saveAs(displayName string) string {
	if !s.hasBuffer() {
		return ""
	}
	baked, sr := s.bake()
	id := samplerUserID(displayName, s.captureID)
	source := s.captureID
	audio.SaveUserSample(id, baked, sr)
	hooks.PublishKind(hooks.EventSampleCreated, hooks.SamplePayload{
		SampleID:    id,
		SourceID:    source,
		DisplayName: displayName,
		Frames:      len(baked),
	})
	s.captureID = id
	return id
}

// samplerUserID derives a stable user-sample instrument id from the display
// name and source id, so re-saving the same name is idempotent.
func samplerUserID(displayName, base string) string {
	sum := crc32.ChecksumIEEE([]byte(displayName + "|" + base))
	return fmt.Sprintf("user.sample.%08x", sum)
}
