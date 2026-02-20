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

// ScheduleMetricsSnapshot is a point-in-time snapshot of audio scheduling
// lead/lag statistics, matching the browser's scheduleMetrics.snapshot().
type ScheduleMetricsSnapshot struct {
	Count          int64   `json:"count"`
	Overdue        int64   `json:"overdue"`
	SmallLeadCount int64   `json:"smallLeadCount"`
	MinLead        float64 `json:"minLead"` // seconds; NaN if no data
	MaxLead        float64 `json:"maxLead"` // seconds
	AvgLead        float64 `json:"avgLead"` // seconds
	AvgLag         float64 `json:"avgLag"`  // seconds
	MaxLag         float64 `json:"maxLag"`  // seconds
	LeadP90        float64 `json:"leadP90"` // seconds; NaN if no data
	LeadP99        float64 `json:"leadP99"` // seconds
	LagP90         float64 `json:"lagP90"`  // seconds; NaN if no data
	LagP99         float64 `json:"lagP99"`  // seconds
}

// scheduleMetrics tracks per-event audio scheduling lead/lag on desktop,
// mirroring the browser's scheduleMetrics object in audio.js.
//
// "Lead" = when - audioNow (positive = scheduled ahead of time).
// "Lag"  = audioNow - when (positive only when overdue).
type scheduleMetrics struct {
	mu             sync.Mutex
	startAt        float64 // first observation audioNow; 0 = unset
	count          int64
	overdue        int64
	smallLeadCount int64
	minLead        float64
	maxLead        float64
	sumLead        float64
	sumLag         float64
	maxLag         float64
	leadSamples    []float64
	lagSamples     []float64
}

// Observe records a single scheduling event. audioNow is the current audio
// clock, when is the scheduled playback time. Both in seconds.
func (m *scheduleMetrics) Observe(audioNow, when float64) {
	lead := when - audioNow
	if math.IsInf(lead, 0) || math.IsNaN(lead) {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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

// Reset clears all collected data.
func (m *scheduleMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startAt = 0
	m.count = 0
	m.overdue = 0
	m.smallLeadCount = 0
	m.minLead = 0
	m.maxLead = 0
	m.sumLead = 0
	m.sumLag = 0
	m.maxLag = 0
	m.leadSamples = m.leadSamples[:0]
	m.lagSamples = m.lagSamples[:0]
}

// Snapshot returns a point-in-time snapshot. Safe to call concurrently.
func (m *scheduleMetrics) Snapshot() ScheduleMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := ScheduleMetricsSnapshot{
		Count:          m.count,
		Overdue:        m.overdue,
		SmallLeadCount: m.smallLeadCount,
		MaxLag:         m.maxLag,
	}

	if m.count == 0 {
		s.MinLead = math.NaN()
		s.MaxLead = math.NaN()
		s.AvgLead = math.NaN()
		s.LeadP90 = math.NaN()
		s.LeadP99 = math.NaN()
		s.LagP90 = math.NaN()
		s.LagP99 = math.NaN()
		return s
	}

	s.MinLead = m.minLead
	s.MaxLead = m.maxLead
	s.AvgLead = m.sumLead / float64(m.count)

	if len(m.lagSamples) > 0 {
		s.AvgLag = m.sumLag / float64(len(m.lagSamples))
	}

	s.LeadP90 = percentileSample(m.leadSamples, 0.90)
	s.LeadP99 = percentileSample(m.leadSamples, 0.99)
	s.LagP90 = percentileSample(m.lagSamples, 0.90)
	s.LagP99 = percentileSample(m.lagSamples, 0.99)

	return s
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

// percentileSample returns the qth percentile from a sample buffer.
// Returns NaN if empty. Matches the browser's percentile() function.
func percentileSample(samples []float64, q float64) float64 {
	n := len(samples)
	if n == 0 {
		return math.NaN()
	}
	// Sort a copy to avoid mutating the ring buffer.
	sorted := make([]float64, n)
	copy(sorted, samples)
	sort.Float64s(sorted)
	idx := int(math.Floor(q * float64(n-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}
