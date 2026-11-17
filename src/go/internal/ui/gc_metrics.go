package ui

import (
	"math"
	"runtime/metrics"
	"sync"
)

// gcMetrics accumulates garbage-collector statistics over a measurement window
// that is INDEPENDENT of perfCounters.reset() (which the PERF_LOG path fires
// every 2 s). GC pause percentiles and cycle counts therefore span the whole
// bench run, not just the last 2 s window — the whole point of GC tuning is the
// worst-case pause across the run, which a 2 s window would clip.
//
// All pause data is read via runtime/metrics, which does NOT stop the world —
// unlike runtime.ReadMemStats. This matters: the instrument must not create the
// very pauses it measures. Heap high-water is likewise sampled through
// runtime/metrics.
//
// The zero value is usable. The first sample()/snapshot() lazily captures a
// baseline, so perfStats() outside bench mode reports GC-since-first-frame.
// reset() re-baselines (called at bench start and from resetPerfStats()).
type gcMetrics struct {
	mu         sync.Mutex
	started    bool
	buckets    []float64 // pause histogram bucket bounds (stable within a run)
	baseCounts []uint64  // pause histogram counts at window start (baseline)
	baseCycles uint64    // GC cycle count at window start
	baseForced uint64    // forced-GC count at window start
	heapHigh   uint64    // peak live-heap bytes observed during the window
}

const (
	gcPausesMetric  = "/gc/pauses:seconds"
	gcCyclesMetric  = "/gc/cycles/total:gc-cycles"
	gcHeapObjMetric = "/memory/classes/heap/objects:bytes"
)

// gcSnapshot is the windowed GC result surfaced into PerfStats.
type gcSnapshot struct {
	Pauses     int64   // number of STW pauses in the window
	Cycles     int64   // GC cycles in the window
	Forced     int64   // forced runtime.GC() calls in the window
	PauseP50MS float64 // approx median STW pause (ms; bucket-resolution)
	PauseP99MS float64 // approx p99 STW pause (ms; bucket-resolution)
	PauseMaxMS float64 // approx max STW pause (ms; bucket-resolution)
	HeapHighKB uint64  // peak live heap during the window (KB)
}

func readGCHistogram() (counts []uint64, buckets []float64, ok bool) {
	s := []metrics.Sample{{Name: gcPausesMetric}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindFloat64Histogram {
		return nil, nil, false
	}
	h := s[0].Value.Float64Histogram()
	// Copy — the returned slices are only valid until the next metrics.Read.
	counts = make([]uint64, len(h.Counts))
	copy(counts, h.Counts)
	buckets = make([]float64, len(h.Buckets))
	copy(buckets, h.Buckets)
	return counts, buckets, true
}

func readGCUint(name string) (uint64, bool) {
	s := []metrics.Sample{{Name: name}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0, false
	}
	return s[0].Value.Uint64(), true
}

// reset re-baselines the window: subsequent snapshots report GC activity since
// this call. Non-STW.
func (m *gcMetrics) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetLocked()
}

func (m *gcMetrics) resetLocked() {
	if counts, buckets, ok := readGCHistogram(); ok {
		m.buckets = buckets
		m.baseCounts = counts
	}
	if c, ok := readGCUint(gcCyclesMetric); ok {
		m.baseCycles = c
	}
	m.baseForced = ForcedGCRunsForTest()
	m.heapHigh = 0
	m.started = true
}

// sample updates the heap high-water mark. Cheap and non-STW; call from the
// per-frame heap probe (throttled).
func (m *gcMetrics) sample() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		m.resetLocked()
	}
	if v, ok := readGCUint(gcHeapObjMetric); ok && v > m.heapHigh {
		m.heapHigh = v
	}
}

// snapshot returns windowed GC stats since the last reset (or first touch).
func (m *gcMetrics) snapshot() gcSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		m.resetLocked()
		return gcSnapshot{}
	}
	if v, ok := readGCUint(gcHeapObjMetric); ok && v > m.heapHigh {
		m.heapHigh = v
	}
	var snap gcSnapshot
	snap.Forced = int64(ForcedGCRunsForTest() - m.baseForced)
	if c, ok := readGCUint(gcCyclesMetric); ok && c >= m.baseCycles {
		snap.Cycles = int64(c - m.baseCycles)
	}
	snap.HeapHighKB = m.heapHigh / 1024
	if counts, buckets, ok := readGCHistogram(); ok &&
		len(m.baseCounts) == len(counts) && len(buckets) == len(m.buckets) {
		delta := make([]uint64, len(counts))
		for i := range counts {
			if counts[i] >= m.baseCounts[i] {
				delta[i] = counts[i] - m.baseCounts[i]
			}
		}
		n, p50, p99, mx := histAggregate(delta, buckets)
		snap.Pauses = n
		snap.PauseP50MS = p50 * 1000
		snap.PauseP99MS = p99 * 1000
		snap.PauseMaxMS = mx * 1000
	}
	return snap
}

// histAggregate returns the count and approximate p50/p99/max (in the
// histogram's native unit — seconds here) from bucket counts + bounds.
// Percentiles use the "value at or below" convention: the representative for a
// bucket is its upper edge; a +Inf top edge is clamped to the bucket's lower
// edge (a conservative floor on the true max).
func histAggregate(counts []uint64, buckets []float64) (n int64, p50, p99, max float64) {
	var total uint64
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0, 0, 0, 0
	}
	n = int64(total)
	edge := func(i int) float64 {
		hi := buckets[i+1]
		if math.IsInf(hi, 1) {
			lo := buckets[i]
			if math.IsInf(lo, -1) {
				return 0
			}
			return lo // clamp +Inf upper to the bucket's lower edge
		}
		if hi < 0 {
			return 0
		}
		return hi
	}
	for i := len(counts) - 1; i >= 0; i-- {
		if counts[i] > 0 {
			max = edge(i)
			break
		}
	}
	t50 := float64(total) * 0.50
	t99 := float64(total) * 0.99
	var cum float64
	var got50 bool
	for i := range counts {
		cum += float64(counts[i])
		if !got50 && cum >= t50 {
			p50 = edge(i)
			got50 = true
		}
		if cum >= t99 {
			p99 = edge(i)
			break
		}
	}
	return n, p50, p99, max
}
