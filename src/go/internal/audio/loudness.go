package audio

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// masterLUFSIntegrator is the process-global K-weighted integrator the
// audio engine feeds from the master signal path. The analyzer service
// reads its current LUFS-S value via MasterLUFSShortTerm (passed as a
// callback through analyzer.Config so the analyzer package stays free
// of an audio import).
//
// Reset by audio.ResetLoudnessForTest in test isolation.
var (
	masterLUFSMu      sync.Mutex
	masterLUFSCurrent *LUFSIntegrator

	// masterLUFSBits holds the last published LUFS-S value as IEEE 754
	// bits so readers (analyzer goroutine) don't have to acquire
	// masterLUFSMu. Writer updates after each Process call.
	masterLUFSBits atomic.Uint64
)

// EnsureMasterLUFS lazily instantiates the master LUFS integrator at
// the given sample rate. Idempotent — repeated calls with the same
// rate reuse the existing integrator. Called by the engine init path.
func EnsureMasterLUFS(sampleRate float64) {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent != nil && masterLUFSCurrent.fs == sampleRate {
		return
	}
	masterLUFSCurrent = NewLUFSIntegrator(sampleRate)
	masterLUFSBits.Store(math.Float64bits(-120))
}

// FeedMasterLUFS pushes a block of master samples through the K-
// weighting filter + integrator. Called from the mixer master branch
// (engine_stop.go's master push site). No-op when the integrator
// hasn't been initialised (test stubs).
func FeedMasterLUFS(samples []float64) {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent == nil {
		return
	}
	masterLUFSCurrent.Process(samples)
	masterLUFSBits.Store(math.Float64bits(masterLUFSCurrent.LUFSShortTerm()))
}

// MasterLUFSShortTerm returns the most recently computed master LUFS-S
// in decibels. Safe to call from any goroutine — reads the published
// atomic bits without acquiring the integrator mutex. Returns -120
// (silence floor) before any audio has been fed.
func MasterLUFSShortTerm() float64 {
	bits := masterLUFSBits.Load()
	if bits == 0 {
		return -120
	}
	return math.Float64frombits(bits)
}

// ResetLoudnessForTest clears the integrator + published value. Tests
// call this at setup so cross-test bleed doesn't taint the LUFS
// measurement.
func ResetLoudnessForTest() {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent != nil {
		masterLUFSCurrent.Reset()
	}
	masterLUFSBits.Store(math.Float64bits(-120))
}

// ── Rolling 10-second clip-window tracker ────────────────────────────
//
// The Levels tab needs "clips in the last 10 seconds" instead of a
// monotonic session counter. Each channel reports its current clip
// count once per analyzer tick; we bucket the delta into 100 ms
// slots and sum the most-recent-10-second window.

const (
	clipWindowBuckets   = 100 // 100 × 100 ms = 10 s
	clipWindowBucketDur = 100 * time.Millisecond
)

var (
	clipWindowMu     sync.Mutex
	clipWindowSlots  [clipWindowBuckets]int
	clipWindowIdx    int
	clipWindowReset  time.Time
	clipWindowLastTS time.Time
	clipsLastCount   atomic.Int64
)

// PushClipDelta adds `delta` clip events to the current 100-ms slot
// and advances the ring when wall-clock time has crossed a bucket
// boundary. Called by the audio mixer's clip-count observer at each
// processTick (or by the analyzer service through a dispatcher hook
// in future).
func PushClipDelta(delta int) {
	if delta <= 0 {
		return
	}
	clipWindowMu.Lock()
	defer clipWindowMu.Unlock()
	now := time.Now()
	if clipWindowReset.IsZero() {
		clipWindowReset = now
		clipWindowLastTS = now
	}
	// Advance the ring by the number of buckets crossed since last
	// push. Zero out crossed buckets so they don't carry stale data.
	bucketsElapsed := int(now.Sub(clipWindowLastTS) / clipWindowBucketDur)
	if bucketsElapsed > 0 {
		if bucketsElapsed > clipWindowBuckets {
			bucketsElapsed = clipWindowBuckets
		}
		for i := 0; i < bucketsElapsed; i++ {
			clipWindowIdx = (clipWindowIdx + 1) % clipWindowBuckets
			clipWindowSlots[clipWindowIdx] = 0
		}
		clipWindowLastTS = now
	}
	clipWindowSlots[clipWindowIdx] += delta
	// Sum the whole ring and publish.
	var sum int
	for _, v := range clipWindowSlots {
		sum += v
	}
	clipsLastCount.Store(int64(sum))
}

// ClipsLastWindow returns the rolling 10-second clip count. Safe to
// call from any goroutine.
func ClipsLastWindow() int {
	// Cheap "decay" path: if no PushClipDelta in >100 ms, advance the
	// ring by the elapsed buckets so an idle session doesn't keep
	// showing the last-recorded clip count forever.
	clipWindowMu.Lock()
	if !clipWindowLastTS.IsZero() {
		bucketsElapsed := int(time.Since(clipWindowLastTS) / clipWindowBucketDur)
		if bucketsElapsed > 0 {
			if bucketsElapsed > clipWindowBuckets {
				bucketsElapsed = clipWindowBuckets
			}
			for i := 0; i < bucketsElapsed; i++ {
				clipWindowIdx = (clipWindowIdx + 1) % clipWindowBuckets
				clipWindowSlots[clipWindowIdx] = 0
			}
			clipWindowLastTS = time.Now()
			var sum int
			for _, v := range clipWindowSlots {
				sum += v
			}
			clipsLastCount.Store(int64(sum))
		}
	}
	clipWindowMu.Unlock()
	return int(clipsLastCount.Load())
}

// ResetClipsWindow zeros the rolling clip-count ring. Called from the
// UI when the user clicks the Levels-tab Clear Clips pill to
// acknowledge clipping events. Tests also call this in setup so
// cross-test state never bleeds.
func ResetClipsWindow() {
	clipWindowMu.Lock()
	defer clipWindowMu.Unlock()
	for i := range clipWindowSlots {
		clipWindowSlots[i] = 0
	}
	clipWindowIdx = 0
	clipWindowReset = time.Time{}
	clipWindowLastTS = time.Time{}
	clipsLastCount.Store(0)
}

// loudness.go — minimal ITU-R BS.1770-style loudness primitives used by
// the Levels tab's LUFS-S readout. Two pieces:
//
//   1. KWeightingFilter — a cascade of two biquads (pre-filter +
//      RLB high-pass) that approximates the BS.1770 K-weighting
//      response. The pre-filter is a shelf around 1681 Hz; the RLB
//      filter is a 38 Hz high-pass. Coefficients are the canonical
//      48 kHz values from the standard; for other sample rates we
//      re-derive them via bilinear transform.
//
//   2. LUFSIntegrator — keeps a sliding 3-second window of squared
//      K-weighted samples and converts the running mean-square to
//      LUFS via `-0.691 + 10*log10(ms)`. 3 s = LUFS Short-Term per
//      the BS.1770 spec.
//
// Both types are pure DSP — no UI, no goroutines, no allocations on
// the hot path after construction. Designed to be called from the
// analyzer service.go's master-channel branch.

// KWeightingFilter holds the per-channel biquad state for the two
// cascaded K-weighting filters. Create one per channel; reset on
// transport stop so a paused state doesn't bleed into the next play.
type KWeightingFilter struct {
	// Pre-filter (high-frequency shelf around 1681 Hz, +4 dB).
	preB0, preB1, preB2 float64
	preA1, preA2        float64
	preZ1, preZ2        float64
	// RLB filter (38 Hz high-pass).
	hpB0, hpB1, hpB2 float64
	hpA1, hpA2       float64
	hpZ1, hpZ2       float64
}

// NewKWeightingFilter constructs a filter for the given sample rate.
// Falls back to the canonical 48 kHz coefficients (ITU-R BS.1770-4
// Annex 1) when sampleRate matches, and re-derives via bilinear
// transform for other rates.
func NewKWeightingFilter(sampleRate float64) *KWeightingFilter {
	f := &KWeightingFilter{}
	if sampleRate == 48000 {
		// Canonical 48 kHz coefficients from BS.1770-4.
		f.preB0 = 1.53512485958697
		f.preB1 = -2.69169618940638
		f.preB2 = 1.19839281085285
		f.preA1 = -1.69065929318241
		f.preA2 = 0.73248077421585
		f.hpB0 = 1.0
		f.hpB1 = -2.0
		f.hpB2 = 1.0
		f.hpA1 = -1.99004745483398
		f.hpA2 = 0.99007225036621
		return f
	}
	// Re-derive via bilinear transform from the analog prototypes.
	// Pre-filter: shelving filter centered at 1681.974 Hz, +4 dB.
	const preFc = 1681.974450955533
	const preGain = 3.999843853973347
	const preQ = 0.7071752369554196
	preB0, preB1, preB2, preA1, preA2 := highShelfBiquad(preFc, preGain, preQ, sampleRate)
	f.preB0, f.preB1, f.preB2, f.preA1, f.preA2 = preB0, preB1, preB2, preA1, preA2

	// RLB: 2nd-order high-pass at 38.135 Hz.
	const hpFc = 38.13547087602542
	const hpQ = 0.5003270373238773
	hpB0, hpB1, hpB2, hpA1, hpA2 := highPassBiquad(hpFc, hpQ, sampleRate)
	f.hpB0, f.hpB1, f.hpB2, f.hpA1, f.hpA2 = hpB0, hpB1, hpB2, hpA1, hpA2
	return f
}

// Process advances the filter by one sample and returns the K-weighted
// output. Maintains direct-form-II transposed state.
func (f *KWeightingFilter) Process(x float64) float64 {
	// Pre-filter (shelf).
	y1 := f.preB0*x + f.preZ1
	f.preZ1 = f.preB1*x - f.preA1*y1 + f.preZ2
	f.preZ2 = f.preB2*x - f.preA2*y1

	// RLB (high-pass).
	y2 := f.hpB0*y1 + f.hpZ1
	f.hpZ1 = f.hpB1*y1 - f.hpA1*y2 + f.hpZ2
	f.hpZ2 = f.hpB2*y1 - f.hpA2*y2
	return y2
}

// Reset zeroes the filter state (call on transport stop).
func (f *KWeightingFilter) Reset() {
	f.preZ1, f.preZ2 = 0, 0
	f.hpZ1, f.hpZ2 = 0, 0
}

// highShelfBiquad returns RBJ-cookbook biquad coefficients for a
// 2nd-order high-shelf at fc, gain dB, Q. Returned as (b0, b1, b2,
// a1, a2) with a0 normalised to 1.
func highShelfBiquad(fc, gainDB, q, fs float64) (float64, float64, float64, float64, float64) {
	A := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * fc / fs
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW / (2 * q)
	twoSqrtAa := 2 * math.Sqrt(A) * alpha

	b0 := A * ((A + 1) + (A-1)*cosW + twoSqrtAa)
	b1 := -2 * A * ((A - 1) + (A+1)*cosW)
	b2 := A * ((A + 1) + (A-1)*cosW - twoSqrtAa)
	a0 := (A + 1) - (A-1)*cosW + twoSqrtAa
	a1 := 2 * ((A - 1) - (A+1)*cosW)
	a2 := (A + 1) - (A-1)*cosW - twoSqrtAa

	return b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0
}

// highPassBiquad returns RBJ-cookbook biquad coefficients for a
// 2nd-order high-pass at fc, Q. Returned as (b0, b1, b2, a1, a2)
// with a0 normalised to 1.
func highPassBiquad(fc, q, fs float64) (float64, float64, float64, float64, float64) {
	w0 := 2 * math.Pi * fc / fs
	cosW := math.Cos(w0)
	sinW := math.Sin(w0)
	alpha := sinW / (2 * q)

	b0 := (1 + cosW) / 2
	b1 := -(1 + cosW)
	b2 := (1 + cosW) / 2
	a0 := 1 + alpha
	a1 := -2 * cosW
	a2 := 1 - alpha

	return b0 / a0, b1 / a0, b2 / a0, a1 / a0, a2 / a0
}

// LUFSIntegrator maintains a 3-second sliding window of K-weighted
// mean-square energy and converts it to LUFS-S on demand. Single-
// goroutine — the analyzer service ticks this in its master branch.
//
// Implementation: a circular buffer of recent block mean-squares,
// each block representing ~100 ms of audio. 30 blocks = ~3 s. The
// integrator sums blocks for the running mean.
type LUFSIntegrator struct {
	filter   *KWeightingFilter
	fs       float64
	blockN   int       // samples per block (≈100 ms)
	bufSize  int       // ring of block mean-squares
	ringSum  []float64 // per-block sums
	ringIdx  int       // next write index
	filled   bool      // ring has wrapped at least once
	curSum   float64   // running sum within the current block
	curCount int       // samples accumulated in the current block
}

// NewLUFSIntegrator builds an integrator at sampleRate, with a default
// 3-second window. Returns nil for non-positive rate so callers can
// guard the wiring without panicking on stub builds.
func NewLUFSIntegrator(sampleRate float64) *LUFSIntegrator {
	if sampleRate <= 0 {
		return nil
	}
	blockN := int(sampleRate / 10) // 100 ms blocks
	if blockN < 64 {
		blockN = 64
	}
	bufSize := 30 // 30 × 100 ms = 3 s
	return &LUFSIntegrator{
		filter:  NewKWeightingFilter(sampleRate),
		fs:      sampleRate,
		blockN:  blockN,
		bufSize: bufSize,
		ringSum: make([]float64, bufSize),
	}
}

// Process feeds a slice of samples through the K-weighting filter and
// accumulates the mean-square into the rolling block buffer.
func (l *LUFSIntegrator) Process(samples []float64) {
	if l == nil || len(samples) == 0 {
		return
	}
	for _, x := range samples {
		y := l.filter.Process(x)
		l.curSum += y * y
		l.curCount++
		if l.curCount >= l.blockN {
			// Block complete: store its mean-square and advance ring.
			ms := l.curSum / float64(l.curCount)
			l.ringSum[l.ringIdx] = ms
			l.ringIdx++
			if l.ringIdx >= l.bufSize {
				l.ringIdx = 0
				l.filled = true
			}
			l.curSum = 0
			l.curCount = 0
		}
	}
}

// LUFSShortTerm returns the current short-term loudness in LUFS
// (decibel-relative-to-full-scale on the K-weighted scale). Returns
// -120 (effective silence floor) when the integrator hasn't yet seen
// enough audio to fill at least one block.
func (l *LUFSIntegrator) LUFSShortTerm() float64 {
	if l == nil {
		return -120
	}
	var sum float64
	var n int
	if l.filled {
		for _, v := range l.ringSum {
			sum += v
		}
		n = l.bufSize
	} else {
		for i := 0; i < l.ringIdx; i++ {
			sum += l.ringSum[i]
		}
		n = l.ringIdx
	}
	if n == 0 || sum <= 0 {
		return -120
	}
	ms := sum / float64(n)
	if ms <= 0 {
		return -120
	}
	return -0.691 + 10*math.Log10(ms)
}

// Reset clears the integrator state (call on transport stop).
func (l *LUFSIntegrator) Reset() {
	if l == nil {
		return
	}
	l.filter.Reset()
	for i := range l.ringSum {
		l.ringSum[i] = 0
	}
	l.ringIdx = 0
	l.filled = false
	l.curSum = 0
	l.curCount = 0
}
