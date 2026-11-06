package fingerprint

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// bundle.go — every artifact derived from one wave, computed at most once.
//
// Descriptors are cheap lookups over a Bundle rather than self-contained
// analyzers, because the battery shares expensive artifacts: one STFT feeds
// the frame tracks, the modulation spectra and several spectral descriptors;
// the envelope segmentation feeds every temporal descriptor. Recomputing
// those per descriptor would make a 150-row report quadratically slower than
// it needs to be, and the cepstral HNR path alone is ~1.8M operations.
//
// Go's call graph is the dependency graph here, and memoization makes each
// artifact once-only, so no separate dependency-resolution engine is needed.
// Adding a new analysis is one accessor.
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

	// Test-only instrumentation proving the memoization actually holds.
	// Written under the same guards as the artifacts they count.
	fingerprintCalls int
	segmentCalls     int
	stftCalls        int
}

type stftKey struct{ size, hop int }

// STFTFrames is a magnitude spectrogram. Frames[i] is one frame's linear
// magnitude spectrum with BinHz between bins, and HopSec elapses between
// consecutive frames.
type STFTFrames struct {
	Frames [][]float64
	BinHz  float64
	HopSec float64
}

// NewBundle prepares an analysis bundle over w. Nothing is computed until an
// accessor is called.
//
// A Bundle is intended for single-goroutine use. The sync guards make the
// memoization safe if it is shared, but the test counters are not guarded
// beyond that and no accessor is designed for contention.
func NewBundle(w wave.Wave, label string) *Bundle {
	return &Bundle{w: w, label: label, stfts: make(map[stftKey]*STFTFrames)}
}

// Wave returns the bundle's source waveform.
func (b *Bundle) Wave() wave.Wave { return b.w }

// Label returns the bundle's label (a reference id or instrument id).
func (b *Bundle) Label() string { return b.label }

// SampleRate returns the source sample rate, defaulting to 44100 when the
// wave carries none — matching every other analyzer in this package.
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

// STFT returns the magnitude spectrogram at the given frame size and hop,
// computed once per distinct (size, hop) pair.
//
// A wave shorter than one full frame yields zero frames rather than a
// zero-padded pseudo-frame: padding a 2-sample clip out to 2048 would report
// a spectrum that is almost entirely window taper, which is a fabricated
// measurement rather than a short one.
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

// Piano returns the struck-string analysis, computed once.
//
// AnalyzePiano needs a fundamental, which it takes from the already-memoized
// Fingerprint rather than re-detecting: running two independent F0 detections
// over one wave is how a report ends up quoting two different pitches for the
// same signal.
func (b *Bundle) Piano() PianoMetrics {
	b.pianoOnce.Do(func() { b.piano = AnalyzePiano(b.w, b.Fingerprint().F0Hz) })
	return b.piano
}

// WindAttack returns the onset-trajectory analysis, computed once.
func (b *Bundle) WindAttack() WindAttack {
	b.windOnce.Do(func() { b.wind = ComputeWindAttack(b.w) })
	return b.wind
}
