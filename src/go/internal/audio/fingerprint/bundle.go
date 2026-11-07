package fingerprint

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Every artefact derived from one wave, computed at most once: one STFT feeds the frame tracks, modulation spectra and
// spectral descriptors, one envelope segmentation every temporal one, so per-descriptor recomputation would make a
// report quadratically slower. Go's call graph is the dependency graph; a new analysis is one accessor.
type Bundle struct {
	w     wave.Wave
	label string

	fpOnce sync.Once
	fp     Fingerprint

	segOnce sync.Once
	seg     EnvelopeSegments

	stftMu sync.Mutex
	stfts  map[stftKey]*STFTFrames

	snareOnce sync.Once
	snare     SnareMetrics

	pianoOnce sync.Once
	piano     PianoMetrics

	windOnce sync.Once
	wind     WindAttack

	tracksOnce sync.Once
	tracks     *Tracks

	modesOnce sync.Once
	modes     *ModeSet

	// Written under the same guards as the artefacts they count.
	fingerprintCalls int
	segmentCalls     int
	stftCalls        int
}

type stftKey struct{ size, hop int }

// Frames[i] is one frame's linear magnitude spectrum, BinHz apart, HopSec between frames.
type STFTFrames struct {
	Frames [][]float64
	BinHz  float64
	HopSec float64
}

// Nothing is computed until an accessor is called. Meant for one goroutine: the guards make sharing safe, but the
// test counters are not guarded and nothing is built for contention.
func NewBundle(w wave.Wave, label string) *Bundle {
	return &Bundle{w: w, label: label, stfts: make(map[stftKey]*STFTFrames)}
}

// Wave returns the bundle's source waveform.
func (b *Bundle) Wave() wave.Wave { return b.w }

// Label returns the bundle's label (a reference id or instrument id).
func (b *Bundle) Label() string { return b.label }

// 44100 when the wave carries none, like every analyzer here.
func (b *Bundle) SampleRate() int {
	if b.w.SampleRate <= 0 {
		return 44100
	}
	return b.w.SampleRate
}

// Fingerprint returns the classic whole-wave Fingerprint, computed once.
func (b *Bundle) Fingerprint() Fingerprint {
	b.fpOnce.Do(func() {
		b.fp = FromWave(b.w, b.label)
		b.fingerprintCalls++
	})
	return b.fp
}

// Segments returns the general-purpose ADSR segmentation, computed once.
func (b *Bundle) Segments() EnvelopeSegments {
	b.segOnce.Do(func() {
		b.seg = AnalyzeEnvelopeSegments(b.w)
		b.segmentCalls++
	})
	return b.seg
}

// Once per (size, hop). A wave shorter than one frame yields no frames: padding would report a spectrum that is
// almost all window taper, a fabricated measurement.
func (b *Bundle) STFT(size, hop int) *STFTFrames {
	if size < 2 {
		size = 2048
	}
	if hop < 1 {
		hop = size / 4
	}
	key := stftKey{size, hop}

	b.stftMu.Lock()
	defer b.stftMu.Unlock()
	if got, ok := b.stfts[key]; ok {
		return got
	}

	sr := b.SampleRate()
	out := &STFTFrames{
		BinHz:  float64(sr) / float64(size),
		HopSec: float64(hop) / float64(sr),
	}
	for start := 0; start+size <= len(b.w.Samples); start += hop {
		seg := wave.Wave{Samples: b.w.Samples[start : start+size], SampleRate: sr}
		mag, _ := wave.MagnitudeSpectrum(seg, size, wave.WindowHann)
		out.Frames = append(out.Frames, mag)
	}
	b.stfts[key] = out
	b.stftCalls++
	return out
}

// Snare returns the percussive-body analysis, computed once.
func (b *Bundle) Snare() SnareMetrics {
	b.snareOnce.Do(func() { b.snare = AnalyzeSnare(b.w) })
	return b.snare
}

// Takes the fundamental from the memoised Fingerprint: two independent F0 detections let a report quote two pitches
// for one signal.
func (b *Bundle) Piano() PianoMetrics {
	b.pianoOnce.Do(func() { b.piano = AnalyzePiano(b.w, b.Fingerprint().F0Hz) })
	return b.piano
}

// WindAttack returns the onset-trajectory analysis, computed once.
func (b *Bundle) WindAttack() WindAttack {
	b.windOnce.Do(func() { b.wind = ComputeWindAttack(b.w) })
	return b.wind
}
