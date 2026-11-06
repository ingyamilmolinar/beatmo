// Package soundview turns a rendered note (mono PCM) into the data the Synth
// tab's graphs draw: loudness over time, the attack, pitch over time, tone
// colour, harmonics, tone colour over time, tone vs noise, and a wave
// close-up (spec docs/superpowers/specs/2026-10-24-synth-graphs-and-devtools-design.md §4.4).
//
// It only MEASURES audio it is handed — the exact buffer the instrument plays
// (audio.RenderDocForGraph). It never models a voice: that is what made the
// old graphs lie (a pure-Go approximation that ignored every drum knob).
//
// Pure Go, no cgo, no Ebiten, wasm-safe; imports only internal/wave (the
// shared FFT). Deliberately NOT internal/audio/fingerprint (analysis tooling,
// large, kept out of shipped binaries): agreement with its descriptors is
// pinned by a real-build test instead.
//
// Every view is computed lazily, once (sync.Once), so an Analysis may be built
// on a worker goroutine, have the views it needs warmed there, and then be
// read from the UI goroutine.
package soundview

import (
	"math"
	"sync"
	"sync/atomic"
)

// FloorDB is the level floor every dB series is clamped to.
const FloorDB = -72.0

// AudibleDropDB is how far below its loudest moment a sound counts as gone.
const AudibleDropDB = 40.0

// Need names a set of analyses (Warm computes exactly those).
type Need uint8

const (
	NeedEnvelope  Need = 1 << iota // loudness over time, audible length, peak
	NeedOnsets                     // onsets + attack
	NeedPitch                      // pitch over time
	NeedSTFT                       // spectrum, centroid, spectrogram, tone/noise
	NeedHarmonics                  // the harmonic ladder (needs pitch)
	NeedAll       = NeedEnvelope | NeedOnsets | NeedPitch | NeedSTFT | NeedHarmonics
)

// Analysis is one rendered note and its lazily computed views.
type Analysis struct {
	pcm      []float32
	sr       int
	voiceSec float64

	done  atomic.Uint32          // Need bits computed so far
	yield atomic.Pointer[func()] // set while Warm runs: the long loops call it

	envOnce sync.Once
	env     Envelope
	peakDB  float64
	audible float64

	atkOnce sync.Once
	hold    []float64 // 1 ms block peak, 10 ms peak-hold
	attack  float64
	onsets  []float64
	onset   OnsetCurve
	clickDB float64

	pitchOnce sync.Once
	pitch     PitchTrack

	stftOnce sync.Once
	spec     Spectrum
	centroid float64
	sg       Spectrogram
	noise    NoiseSplit

	harmOnce sync.Once
	harm     Harmonics
	harmOK   bool
}

// New wraps a rendered note: pcm at sample rate sr, which the platform sizes
// to voiceSec (the note's played length; 0 = unknown, the buffer's length).
// The slice is read, never written or retained beyond the Analysis.
func New(pcm []float32, sr int, voiceSec float64) *Analysis {
	if sr <= 0 {
		sr = 48000
	}
	if voiceSec <= 0 {
		voiceSec = float64(len(pcm)) / float64(sr)
	}
	return &Analysis{pcm: pcm, sr: sr, voiceSec: voiceSec}
}

// SampleRate is the PCM's sample rate.
func (a *Analysis) SampleRate() int { return a.sr }

// VoiceSec is the note's played length on this platform.
func (a *Analysis) VoiceSec() float64 { return a.voiceSec }

// DurationSec is the buffer's length.
func (a *Analysis) DurationSec() float64 { return float64(len(a.pcm)) / float64(a.sr) }

// PCM is the analysed buffer (read-only).
func (a *Analysis) PCM() []float32 { return a.pcm }

// Warm computes the analyses need names now (on the caller's goroutine), so
// later reads are lock-free cache hits. yield, when non-nil, runs between
// analyses AND every few frames inside the long loops (the STFT, the pitch
// track): the browser runs this on the page's one thread and hands it back to
// the frame loop there.
func (a *Analysis) Warm(need Need, yield func()) {
	if yield != nil {
		a.yield.Store(&yield)
		defer a.yield.Store(nil)
	}
	steps := []struct {
		n Need
		f func()
	}{
		{NeedEnvelope, func() { a.Envelope() }},
		{NeedOnsets, func() { a.Onsets() }},
		{NeedPitch, func() { a.Pitch() }},
		{NeedSTFT, func() { a.Spectrum() }},
		{NeedHarmonics, func() { a.Harmonics() }},
	}
	for _, s := range steps {
		if need&s.n == 0 {
			continue
		}
		s.f()
		if yield != nil {
			yield()
		}
	}
}

// Has reports whether every analysis need names is already computed.
func (a *Analysis) Has(need Need) bool { return Need(a.done.Load())&need == need }

func (a *Analysis) markDone(n Need) {
	for {
		old := a.done.Load()
		if a.done.CompareAndSwap(old, old|uint32(n)) {
			return
		}
	}
}

// yieldEvery hands the thread back every `every` iterations of a long loop
// while Warm runs with a yield (no-op otherwise). The steps are sized so a
// stretch stays near 3 ms on WASM (TestWarmYieldsOftenAndComputesOnlyWhatIsNeeded).
func (a *Analysis) yieldEvery(i, every int) {
	if i%every != every-1 {
		return
	}
	if f := a.yield.Load(); f != nil {
		(*f)()
	}
}

func toDB(v float64) float64 {
	if v <= 1e-12 {
		return FloorDB
	}
	return math.Max(FloorDB, 20*math.Log10(v))
}

func powDB(p float64) float64 {
	if p <= 1e-24 {
		return FloorDB
	}
	return math.Max(FloorDB, 10*math.Log10(p))
}

// analysedTailSec is how far past the audible end the spectral and pitch
// analyses still look (a decaying tail's last frames; the views' time axes
// stop at 1.15 × the audible length).
const analysedTailSec = 0.1

// analysedLen is how much of the buffer the spectral and pitch analyses read:
// up to the note's audible end plus a short tail, never the silence after it.
func (a *Analysis) analysedLen() int {
	aud := a.AudibleSec()
	if aud <= 0 {
		return len(a.pcm)
	}
	return min(len(a.pcm), int((aud+analysedTailSec)*float64(a.sr)))
}
