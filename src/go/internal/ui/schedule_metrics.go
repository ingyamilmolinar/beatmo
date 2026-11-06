package ui

import (
	"math"
	"math/rand"
	"sort"
	"sync"
)

const (
	schedMaxSamples = 2048
	schedWarmupSec  = 0.5
	schedSmallLead  = 0.003 // 3ms
)

// ScheduleMetricsSnapshot is a point-in-time snapshot of the three-stage
// audio scheduling pipeline:
//
//	Stage A — sequencer-fire-late: how late seqScheduleTime fires a beat
//	          relative to its ideal wall-clock moment (real → scheduler).
//	Stage B — bridge latency: from audio request enqueue (req.enqAt) to
//	          audioLoop dispatch (scheduler → audio bridge).
//	Stage C — schedule lead: lead = when - audioNow, the cushion between
//	          dispatch and the buffer's scheduled fire time (audio bridge
//	          → audio output). Overdue events (lead < 0) accumulate as lag.
//
// For each stage we expose count, P50/P90/P99/max, and StdDev as a jitter
// proxy. Browser audio.js mirrors stage C only; stages A and B are Go-side
// since the sequencer + audio queue both live in the Go runtime even
// under WASM.
type ScheduleMetricsSnapshot struct {
	Count          int64 `json:"count"`
	Overdue        int64 `json:"overdue"`
	SmallLeadCount int64 `json:"smallLeadCount"`

	// Stage C — schedule lead (existing).
	MinLead    float64 `json:"minLead"`
	MaxLead    float64 `json:"maxLead"`
	AvgLead    float64 `json:"avgLead"`
	LeadP50    float64 `json:"leadP50"`
	LeadP90    float64 `json:"leadP90"`
	LeadP99    float64 `json:"leadP99"`
	LeadStdDev float64 `json:"leadStdDev"`
	AvgLag     float64 `json:"avgLag"`
	MaxLag     float64 `json:"maxLag"`
	LagP90     float64 `json:"lagP90"`
	LagP99     float64 `json:"lagP99"`

	// Stage B — bridge latency (enqAt → dispatch), seconds.
	BridgeCount  int64   `json:"bridgeCount"`
	BridgeAvg    float64 `json:"bridgeAvg"`
	BridgeMax    float64 `json:"bridgeMax"`
	BridgeP50    float64 `json:"bridgeP50"`
	BridgeP90    float64 `json:"bridgeP90"`
	BridgeP99    float64 `json:"bridgeP99"`
	BridgeStdDev float64 `json:"bridgeStdDev"`

	// Stage A — sequencer-fire-late (target wall-clock → fire), seconds.
	SeqFireCount  int64   `json:"seqFireCount"`
	SeqFireAvg    float64 `json:"seqFireAvg"`
	SeqFireMax    float64 `json:"seqFireMax"`
	SeqFireP50    float64 `json:"seqFireP50"`
	SeqFireP90    float64 `json:"seqFireP90"`
	SeqFireP99    float64 `json:"seqFireP99"`
	SeqFireStdDev float64 `json:"seqFireStdDev"`

	// Stage D — emission drift (idealWhen → sample emission), seconds.
	EmitCount  int64   `json:"emitCount"`
	EmitAvg    float64 `json:"emitAvg"`    // signed mean (systematic bias)
	EmitAbsMax float64 `json:"emitAbsMax"` // max |drift|
	EmitP99    float64 `json:"emitP99"`    // P99 of |drift|
	EmitStdDev float64 `json:"emitStdDev"` // stddev of |drift|

	// End-to-end latency = SeqFireLate + Bridge + max(0, -Lead).
	// Sampled by combining contemporaneous observations; we approximate
	// by summing per-stage P99s. Reported as a derived field so tests can
	// gate against a single worst-case ceiling.
	E2EP99 float64 `json:"e2eP99"`
}

// scheduleMetrics tracks the three-stage audio scheduling pipeline on
// desktop and under WASM. The browser's scheduleMetrics object in
// audio.js mirrors Stage C only; stages A and B are owned here because
// the sequencer goroutine and the audio dispatch loop both live in Go
// even when targeting wasm.
type scheduleMetrics struct {
	mu sync.Mutex

	// Stage C — lead/lag (existing).
	startAt        float64
	count          int64
	overdue        int64
	smallLeadCount int64
	minLead        float64
	maxLead        float64
	sumLead        float64
	sumLeadSq      float64
	sumLag         float64
	maxLag         float64
	leadSamples    []float64
	lagSamples     []float64

	// Stage B — bridge latency.
	bridgeCount   int64
	bridgeSum     float64
	bridgeSumSq   float64
	bridgeMax     float64
	bridgeSamples []float64

	// Stage A — sequencer-fire-late.
	seqFireCount   int64
	seqFireSum     float64
	seqFireSumSq   float64
	seqFireMax     float64
	seqFireSamples []float64

	// Stage D — emission drift (idealWhen → actual sample emission).
	emitCount    int64
	emitSumSig   float64 // signed sum, for systematic-bias mean
	emitSumAbs   float64 // sum of |drift|, for stddev
	emitSumSqAbs float64 // sum of squares of |drift|, for stddev
	emitAbsMax   float64
	emitSamples  []float64 // |drift| reservoir for percentile

	// sorted is Snapshot's scratch copy of one reservoir: each reservoir is
	// sorted ONCE per Snapshot and every quantile read off it. Snapshot runs
	// every frame (parityTimingCheck) and the audio lane keeps the lead
	// reservoir full (~2000 observations/s under edits): a copy + sort per
	// quantile was three sorts of 2048 samples per stage per frame.
	sorted []float64

	// rev counts mutations; Snapshot returns snap while snapRev == rev. A
	// frame with no new observation (most of them) re-sorted every reservoir
	// for an identical snapshot.
	rev      uint64
	snapRev  uint64
	haveSnap bool
	snap     ScheduleMetricsSnapshot
}

// Observe records a Stage C scheduling event (audio dispatch → fire).
// audioNow is the current audio clock; when is the scheduled playback
// time. Both in seconds.
func (m *scheduleMetrics) Observe(audioNow, when float64) {
	lead := when - audioNow
	if math.IsInf(lead, 0) || math.IsNaN(lead) {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++

	if m.startAt == 0 {
		m.startAt = audioNow
	}
	if audioNow-m.startAt < schedWarmupSec {
		return
	}

	if m.count == 0 {
		m.minLead = lead
		m.maxLead = lead
	} else {
		if lead < m.minLead {
			m.minLead = lead
		}
		if lead > m.maxLead {
			m.maxLead = lead
		}
	}
	m.count++
	m.sumLead += lead
	m.sumLeadSq += lead * lead

	if lead < 0 {
		m.overdue++
		lag := -lead
		m.sumLag += lag
		if lag > m.maxLag {
			m.maxLag = lag
		}
		pushSample(&m.lagSamples, lag)
	} else if lead < schedSmallLead {
		m.smallLeadCount++
	}

	pushSample(&m.leadSamples, lead)
}

// ObserveBridge records a Stage B observation. dtSec is the elapsed
// wall-clock seconds between sequencer enqueue (req.enqAt) and the
// audioLoop dispatch site.
func (m *scheduleMetrics) ObserveBridge(dtSec float64) {
	if math.IsNaN(dtSec) || math.IsInf(dtSec, 0) || dtSec < 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.bridgeCount++
	m.bridgeSum += dtSec
	m.bridgeSumSq += dtSec * dtSec
	if dtSec > m.bridgeMax {
		m.bridgeMax = dtSec
	}
	pushSample(&m.bridgeSamples, dtSec)
}

// ObserveSeqFireLate records a Stage A observation. lateSec is the
// wall-clock seconds by which seqScheduleTime fired a beat after the
// beat's ideal target moment. Always non-negative because seqScheduleTime
// only fires once target ≥ idx.
func (m *scheduleMetrics) ObserveSeqFireLate(lateSec float64) {
	if math.IsNaN(lateSec) || math.IsInf(lateSec, 0) {
		return
	}
	if lateSec < 0 {
		lateSec = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.seqFireCount++
	m.seqFireSum += lateSec
	m.seqFireSumSq += lateSec * lateSec
	if lateSec > m.seqFireMax {
		m.seqFireMax = lateSec
	}
	pushSample(&m.seqFireSamples, lateSec)
}

// ObserveEmitDrift records a Stage D observation: driftSec is the signed
// wall-clock seconds between a beat's ideal scheduled moment and the actual
// audio sample emission (emittedAt - idealWhen). Signed drift feeds the
// systematic-bias mean; |drift| feeds the tail/max/stddev so symmetric
// jitter cannot cancel out.
func (m *scheduleMetrics) ObserveEmitDrift(driftSec float64) {
	if math.IsNaN(driftSec) || math.IsInf(driftSec, 0) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	a := math.Abs(driftSec)
	m.emitCount++
	m.emitSumSig += driftSec
	m.emitSumAbs += a
	m.emitSumSqAbs += a * a
	if a > m.emitAbsMax {
		m.emitAbsMax = a
	}
	pushSample(&m.emitSamples, a)
}

// ResetPeaks zeroes only the per-stage session-monotonic MAX fields
// (seqFireMax, maxLag, bridgeMax, emitAbsMax) — counts, sums, and the
// percentile reservoirs are left untouched so mean/P99/stddev keep
// integrating over their existing window.
//
// Called from bumpParityGen (game_mutation_gate.go) on every structural
// mutation (node/edge/rule/group/BPM/length/insert-fx — every path that
// routes through the mutation gate) so a spike latched by an edit-induced
// scheduling stall — either recorded just before the edit, or slipping
// through DURING the edit before the grace-window observation guards in
// game_sequencer_schedule.go / game_audio_loop.go take effect — can never
// survive to breach evalTimingStage's absMax dropout check (which fires
// regardless of MinSamples) after grace expires. This is the deterministic
// half of the C1 fix; the grace-window observation guards are the
// preventive half. See design §3.4 ("universal suppression across all
// stages") in docs/superpowers/specs/2026-07-10-parity-timing-sync-design.md.
func (m *scheduleMetrics) ResetPeaks() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.maxLag = 0
	m.bridgeMax = 0
	m.seqFireMax = 0
	m.emitAbsMax = 0
}

// Reset clears all collected data.
func (m *scheduleMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.startAt = 0
	m.count = 0
	m.overdue = 0
	m.smallLeadCount = 0
	m.minLead = 0
	m.maxLead = 0
	m.sumLead = 0
	m.sumLeadSq = 0
	m.sumLag = 0
	m.maxLag = 0
	m.leadSamples = m.leadSamples[:0]
	m.lagSamples = m.lagSamples[:0]

	m.bridgeCount = 0
	m.bridgeSum = 0
	m.bridgeSumSq = 0
	m.bridgeMax = 0
	m.bridgeSamples = m.bridgeSamples[:0]

	m.seqFireCount = 0
	m.seqFireSum = 0
	m.seqFireSumSq = 0
	m.seqFireMax = 0
	m.seqFireSamples = m.seqFireSamples[:0]

	m.emitCount = 0
	m.emitSumSig = 0
	m.emitSumAbs = 0
	m.emitSumSqAbs = 0
	m.emitAbsMax = 0
	m.emitSamples = m.emitSamples[:0]
}

// Snapshot returns a point-in-time snapshot. Safe to call concurrently.
func (m *scheduleMetrics) Snapshot() ScheduleMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.haveSnap && m.snapRev == m.rev {
		return m.snap
	}

	s := ScheduleMetricsSnapshot{
		Count:          m.count,
		Overdue:        m.overdue,
		SmallLeadCount: m.smallLeadCount,
		MaxLag:         m.maxLag,
		BridgeCount:    m.bridgeCount,
		BridgeMax:      m.bridgeMax,
		SeqFireCount:   m.seqFireCount,
		SeqFireMax:     m.seqFireMax,
		EmitCount:      m.emitCount,
		EmitAbsMax:     m.emitAbsMax,
	}

	if m.count == 0 {
		s.MinLead = math.NaN()
		s.MaxLead = math.NaN()
		s.AvgLead = math.NaN()
		s.LeadP50 = math.NaN()
		s.LeadP90 = math.NaN()
		s.LeadP99 = math.NaN()
		s.LeadStdDev = math.NaN()
		s.LagP90 = math.NaN()
		s.LagP99 = math.NaN()
	} else {
		s.MinLead = m.minLead
		s.MaxLead = m.maxLead
		s.AvgLead = m.sumLead / float64(m.count)
		s.LeadStdDev = stddev(m.sumLead, m.sumLeadSq, m.count)
		// Lag statistics are normalized over ALL dispatches in the window, not
		// just the overdue ones. sumLag/len(lagSamples) is the conditional
		// "mean lateness of the late beats" — under that reading a SINGLE 4ms
		// blip in a 65-dispatch window reported AvgLag=LagP99=4ms and breached
		// the 0.5ms MeanMax bound written for sustained drift (300 BPM bench
		// crash, 2026-07-13). On-time dispatches contribute lag 0; the isolated
		// blip belongs to MaxLag (dropout), which the emit cross-check governs.
		s.AvgLag = m.sumLag / float64(m.count)
		lead := m.sortSamples(m.leadSamples)
		s.LeadP50 = sortedQuantile(lead, 0.50)
		s.LeadP90 = sortedQuantile(lead, 0.90)
		s.LeadP99 = sortedQuantile(lead, 0.99)
		lag := m.sortSamples(m.lagSamples)
		s.LagP90 = lagQuantileAll(lag, m.overdue, m.count, 0.90)
		s.LagP99 = lagQuantileAll(lag, m.overdue, m.count, 0.99)
	}

	if m.bridgeCount > 0 {
		s.BridgeAvg = m.bridgeSum / float64(m.bridgeCount)
		s.BridgeStdDev = stddev(m.bridgeSum, m.bridgeSumSq, m.bridgeCount)
		bridge := m.sortSamples(m.bridgeSamples)
		s.BridgeP50 = sortedQuantile(bridge, 0.50)
		s.BridgeP90 = sortedQuantile(bridge, 0.90)
		s.BridgeP99 = sortedQuantile(bridge, 0.99)
	} else {
		s.BridgeAvg = math.NaN()
		s.BridgeStdDev = math.NaN()
		s.BridgeP50 = math.NaN()
		s.BridgeP90 = math.NaN()
		s.BridgeP99 = math.NaN()
	}

	if m.seqFireCount > 0 {
		s.SeqFireAvg = m.seqFireSum / float64(m.seqFireCount)
		s.SeqFireStdDev = stddev(m.seqFireSum, m.seqFireSumSq, m.seqFireCount)
		seq := m.sortSamples(m.seqFireSamples)
		s.SeqFireP50 = sortedQuantile(seq, 0.50)
		s.SeqFireP90 = sortedQuantile(seq, 0.90)
		s.SeqFireP99 = sortedQuantile(seq, 0.99)
	} else {
		s.SeqFireAvg = math.NaN()
		s.SeqFireStdDev = math.NaN()
		s.SeqFireP50 = math.NaN()
		s.SeqFireP90 = math.NaN()
		s.SeqFireP99 = math.NaN()
	}

	if m.emitCount > 0 {
		s.EmitAvg = m.emitSumSig / float64(m.emitCount)
		s.EmitStdDev = stddev(m.emitSumAbs, m.emitSumSqAbs, m.emitCount)
		s.EmitP99 = sortedQuantile(m.sortSamples(m.emitSamples), 0.99)
	} else {
		s.EmitAvg = math.NaN()
		s.EmitStdDev = math.NaN()
		s.EmitP99 = math.NaN()
	}

	// End-to-end P99: sum the per-stage P99s plus any overdue lag. This
	// over-estimates (independent stages don't share their P99 sample)
	// but is conservative for ceiling gating.
	if m.seqFireCount > 0 && m.bridgeCount > 0 && m.count > 0 {
		s.E2EP99 = s.SeqFireP99 + s.BridgeP99
		if !math.IsNaN(s.LagP99) {
			s.E2EP99 += s.LagP99
		}
	} else {
		s.E2EP99 = math.NaN()
	}

	m.snap, m.snapRev, m.haveSnap = s, m.rev, true
	return s
}

// stddev returns the sample stddev given running sum, sum-of-squares, and n.
// Returns NaN for n < 2.
func stddev(sum, sumSq float64, n int64) float64 {
	if n < 2 {
		return math.NaN()
	}
	nf := float64(n)
	mean := sum / nf
	v := sumSq/nf - mean*mean
	if v < 0 {
		// Numerical noise on near-constant samples.
		return 0
	}
	return math.Sqrt(v)
}

// pushSample appends to the ring buffer. When full, randomly replaces an
// existing entry (reservoir sampling), matching the browser's pushSample().
func pushSample(buf *[]float64, v float64) {
	if len(*buf) < schedMaxSamples {
		*buf = append(*buf, v)
	} else {
		(*buf)[rand.Intn(len(*buf))] = v //nolint:gosec // perf sampling, not crypto
	}
}

// lagPercentileAll returns the q-quantile of the lag distribution over ALL
// count dispatches, where the (count-overdue) on-time dispatches have lag 0
// and the overdue tail is approximated by the lagSamples reservoir. Zero when
// the overdue fraction is at most (1-q): the q-quantile dispatch was on-time.
// sortedLag is the lag reservoir sorted ascending.
func lagQuantileAll(sortedLag []float64, overdue, count int64, q float64) float64 {
	if count <= 0 || overdue <= 0 || len(sortedLag) == 0 {
		return 0
	}
	tail := (1 - q) * float64(count) // dispatches strictly above the quantile
	if float64(overdue) <= tail {
		return 0
	}
	return sortedQuantile(sortedLag, 1-tail/float64(overdue))
}

// sortSamples copies a reservoir into m.sorted and sorts it (caller holds
// m.mu). The result is valid until the next sortSamples call.
func (m *scheduleMetrics) sortSamples(samples []float64) []float64 {
	m.sorted = append(m.sorted[:0], samples...)
	sort.Float64s(m.sorted)
	return m.sorted
}

// percentileSample returns the qth percentile from a sample buffer.
// Returns NaN if empty. Matches the browser's percentile() function.
func percentileSample(samples []float64, q float64) float64 {
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)
	return sortedQuantile(sorted, q)
}

// sortedQuantile is percentileSample over an already-sorted buffer.
func sortedQuantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	idx := int(math.Floor(q * float64(n-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
