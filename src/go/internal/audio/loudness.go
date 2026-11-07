package audio

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Fed from the master path; the analyzer reads LUFS-S through a callback in analyzer.Config, so it imports no audio.
var (
	masterLUFSMu      sync.Mutex
	masterLUFSCurrent *LUFSIntegrator

	// Published as IEEE 754 bits so the analyzer goroutine reads without the mutex.
	masterLUFSBits atomic.Uint64
)

// Idempotent.
func EnsureMasterLUFS(sampleRate float64) {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent != nil && masterLUFSCurrent.fs == sampleRate {
		return
	}
	masterLUFSCurrent = NewLUFSIntegrator(sampleRate)
	masterLUFSBits.Store(math.Float64bits(-120))
}

// A no-op before the integrator exists (test stubs).
func FeedMasterLUFS(samples []float64) {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent == nil {
		return
	}
	masterLUFSCurrent.Process(samples)
	masterLUFSBits.Store(math.Float64bits(masterLUFSCurrent.LUFSShortTerm()))
}

// Lock-free. -120 (silence floor) before any audio.
func MasterLUFSShortTerm() float64 {
	bits := masterLUFSBits.Load()
	if bits == 0 {
		return -120
	}
	return math.Float64frombits(bits)
}

func ResetLoudnessForTest() {
	masterLUFSMu.Lock()
	defer masterLUFSMu.Unlock()
	if masterLUFSCurrent != nil {
		masterLUFSCurrent.Reset()
	}
	masterLUFSBits.Store(math.Float64bits(-120))
}

// The Levels tab shows clips in the last 10 seconds, not a session counter: per-tick deltas bucketed into 100 ms slots.

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

// Advances the ring when wall-clock time crosses a bucket boundary.
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
	// Crossed buckets are zeroed so they carry no stale data.
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

// Safe from any goroutine.
func ClipsLastWindow() int {
	// With no push for >100 ms the ring still advances, so an idle session does not show its last clips forever.
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

// The Levels tab's Clear Clips.
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

// BS.1770-style loudness for the Levels tab's LUFS-S: K-weighting then a 3 s mean square (Short-Term). Pure DSP, no
// allocation on the hot path.

// Reset on transport stop so a paused state does not bleed into the next play.
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

// The canonical 48 kHz coefficients (BS.1770-4 Annex 1) at 48 kHz, a bilinear re-derivation at other rates.
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
	// Pre-filter: a +4 dB shelf at 1681.974 Hz.
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

// Direct form II transposed.
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

// RBJ cookbook, a0 normalised to 1.
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

// RBJ cookbook, a0 normalised to 1.
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

// 30 blocks of ~100 ms = the 3 s Short-Term window. Single goroutine: the analyzer's master branch.
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

// nil for a non-positive rate, so stub builds can guard the wiring.
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

// -120 (silence floor) before one block has filled.
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
