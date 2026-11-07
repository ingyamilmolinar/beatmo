package ui

import (
	"hash/crc32"
	"image"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// samplerSource identifies where the sampler's working buffer came from.
type samplerSource int

const (
	samplerSourceSynth samplerSource = iota // captured from an instrument's rendered one-shot
	samplerSourceWAV
)

// On-screen left-to-right order.
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
)

// Free of UI dependencies, so the edit/capture/save behaviour is testable without a DrumView.
type samplerState struct {
	source        samplerSource
	captureID     instrument.ID // instrument id the buffer was captured from / Save overrides
	raw           []float32
	rawSampleRate int

	// Synth sources only; WAV PCM is fixed.
	srcSignature uint64

	// Reverse flips the working buffer in place, so the waveform and the bake reflect it directly; `reverse`
	// is the net flip parity, which the descriptor (re-rendered forward at trigger time) and the toggle visual need.
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

	// Mobile only; kept across Layout so the scroll position survives.
	knobGrid     *ControlGrid
	knobGridRect image.Rectangle
	btns         []*Button // header + control-row buttons, rebuilt each Layout

	dragHandle int // -1 none, 0 start handle, 1 end handle

	// -1 none, 0 start, 1 end; stays -1 on mobile.
	hoverHandle int

	// A transient line in the waveform card, e.g. a capture that returned no audio. Empty = none.
	status string

	// Reused scratch so the tab stays within its alloc budget; dispF32 carries the amplitude overlays so the trace
	// matches the baked audio.
	waveF64 []float64
	dispF32 []float32

	playheads samplerPlayheadTracker
}

// The working buffer with normalize → gain → fade applied through the same DSP functions BakeSample uses,
// so the trace matches the sound. Reverse is already in s.raw; trim is a shading overlay.
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
			audio.ApplyFades(s.dispF32, sr, audio.SampleFadeMs)
		}
		src = s.dispF32
	}
	for i, v := range src {
		s.waveF64[i] = float64(v)
	}
	return s.waveF64
}

// 0 when empty or the rate is unknown.
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

// Pitched like BakeSample resamples (higher plays shorter); 0 when nothing audible is loaded.
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

// Visible only while 0 ≤ elapsed ≤ duration. Always forward: a reversed sound still plays front to back
// through the kept region.
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

// Keeps the loaded buffer.
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
	s.hoverHandle = -1
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

// Gain is asymmetric (-24..+6 dB), so its zero sits at 0.8; Start/End are unipolar.
func samplerKnobBipolar(idx int) (zeroFrac float64, bipolar bool) {
	switch idx {
	case samplerKnobTranspose, samplerKnobDetune:
		return 0.5, true
	case samplerKnobGain:
		return (0 - samplerGainMinDB) / (samplerGainMaxDB - samplerGainMinDB), true
	}
	return 0, false
}

func (s *samplerState) edit() audio.SampleEdit {
	lo, hi := s.startFrac, s.endFrac
	if lo > hi {
		lo, hi = hi, lo
	}
	// Not Reverse: the buffer is already flipped, and setting it here would reverse twice.
	return audio.SampleEdit{
		StartFrac:      lo,
		EndFrac:        hi,
		TransposeSemis: s.transposeSemis,
		DetuneCents:    s.detuneCents,
		GainDB:         float32(s.gainDB),
		Fade:           s.fadeOn,
		Normalize:      s.normalize,
	}
}

// For the non-destructive path, which bakes the FORWARD source at trigger time and trims before reversing:
// [lo,hi] of reverse(P) is reverse(P[1-hi : 1-lo]), so a reversed edit mirrors the trim into the source frame.
func (s *samplerState) editDescriptor() audio.SampleEdit {
	e := s.edit()
	if s.reverse {
		e.StartFrac, e.EndFrac = 1-e.EndFrac, 1-e.StartFrac
		e.Reverse = true
	}
	return e
}

// The buffer must hold the raw source; a saved reverse is re-applied once so the waveform matches.
func (s *samplerState) loadEditDescriptor(e audio.SampleEdit) {
	// Un-mirror into the display frame, or a reversed+trimmed chop reopens with its handles on the wrong end.
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
	s.fadeOn = e.Fade
	if e.Reverse && !s.reverse {
		s.reverseBuffer() // flips raw for display AND sets s.reverse
	}
}

// A var so tests can drive the empty-capture path.
var samplerCaptureFn = audio.RenderInstrumentOneShot

// Ignores the saved edit, which the editor overlays itself (the edited render would apply it twice).
var samplerRawCaptureFn = audio.RenderInstrumentOneShotRaw

func SwapSamplerRawCaptureFnForTest(fn func(instrument.ID) ([]float32, int)) func(instrument.ID) ([]float32, int) {
	prev := samplerRawCaptureFn
	if fn != nil {
		samplerRawCaptureFn = fn
	}
	return prev
}

func SwapSamplerCaptureFnForTest(fn func(instrument.ID) ([]float32, int)) func(instrument.ID) ([]float32, int) {
	prev := samplerCaptureFn
	if fn != nil {
		samplerCaptureFn = fn
	}
	return prev
}

// No audio (the browser has not rendered yet) leaves the buffer empty with a status message, never a
// silent no-op.
func (s *samplerState) captureFromSynth(id instrument.ID) {
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

// captureID is set so a follow-up Save targets this instrument.
func (s *samplerState) loadFromInstrument(id instrument.ID, pcm []float32, sr int, src samplerSource) {
	s.raw = pcm
	s.rawSampleRate = sr
	s.captureID = id
	s.source = src
	s.status = ""
	s.reset()
}

// Keeps the in-progress edit (a re-render must not discard the chop); trim fractions stay valid. The render
// arrives forward, so a latched Reverse is re-applied, or the waveform would show forward while the audio plays reversed.
func (s *samplerState) recaptureRaw(pcm []float32, sr int) {
	s.raw = pcm
	s.rawSampleRate = sr
	s.source = samplerSourceSynth
	s.status = ""
	if s.reverse {
		audio.ReverseSample(s.raw)
	}
}

// Mirrors the trim ([s,e] → [1-e, 1-s]) so the kept region still covers the same audio. loadEditDescriptor
// computes its own handles and calls reverseBuffer directly.
func (s *samplerState) toggleReverse() {
	if len(s.raw) == 0 {
		return
	}
	s.startFrac, s.endFrac = 1-s.endFrac, 1-s.startFrac
	s.reverseBuffer()
}

// A no-op on an empty buffer.
func (s *samplerState) reverseBuffer() {
	if len(s.raw) == 0 {
		return
	}
	audio.ReverseSample(s.raw)
	s.reverse = !s.reverse // net flip parity, consumed only by editDescriptor()
}

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

// The seed tells same-named samples of different origins apart.
func userSampleID(displayName, seed string) instrument.ID {
	return instrument.UserSampleID(crc32.ChecksumIEEE([]byte(displayName + "|" + seed)))
}
