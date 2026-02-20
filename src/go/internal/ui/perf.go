package ui

import (
	"runtime"
	"sync/atomic"
	"time"
)

// PerfStats is a snapshot of recent performance metrics gathered in Game.
// Values are approximate and reset periodically.
type PerfStats struct {
	Frames      int64   // number of Update() calls sampled
	FPSAvg      float64 // frames per second over the sample period
	UpdateAvgMS float64 // average Update() duration in ms
	UpdateMaxMS float64 // max Update() duration in ms

	DrawAvgMS float64 // average Draw() duration in ms
	DrawMaxMS float64 // max Draw() duration in ms

	AudioEnq     int64   // number of audio schedule enqueues
	AudioDeq     int64   // number of audio dispatches to engine/JS
	AudioQLatAvg float64 // average enqueue→dispatch latency in ms
	AudioQLatMax float64 // max enqueue→dispatch latency in ms

	AudioCallAvg float64 // average per-event duration of audio.Play* dispatch in ms (batched calls amortized)
	AudioCallMax float64 // max per-event duration of audio.Play* dispatch in ms

	HeapAllocKB uint64 // current heap allocation in KB
	HeapSysKB   uint64 // total heap obtained from OS in KB
	HeapObjects uint64 // live heap objects
	Goroutines  int    // number of live goroutines

	SchedMetrics ScheduleMetricsSnapshot // per-event audio scheduling lead/lag
}

type perfCounters struct {
	// frame/update
	frames   int64
	updSumNS int64
	updMaxNS int64
	started  time.Time
	nextLog  time.Time

	// audio queue
	aEnq       int64
	aDeq       int64
	aQLatSumNS int64
	aQLatMaxNS int64

	// audio call (bridge)
	aCallSumNS int64
	aCallMaxNS int64

	// draw
	drawSumNS int64
	drawMaxNS int64
}

const (
	perfUpdateWarmupFrames = 3
	perfDrawWarmupFrames   = 6
	perfAudioWarmupCount   = 3
)

func (p *perfCounters) reset() {
	atomic.StoreInt64(&p.frames, 0)
	atomic.StoreInt64(&p.updSumNS, 0)
	atomic.StoreInt64(&p.updMaxNS, 0)
	atomic.StoreInt64(&p.aEnq, 0)
	atomic.StoreInt64(&p.aDeq, 0)
	atomic.StoreInt64(&p.aQLatSumNS, 0)
	atomic.StoreInt64(&p.aQLatMaxNS, 0)
	atomic.StoreInt64(&p.aCallSumNS, 0)
	atomic.StoreInt64(&p.aCallMaxNS, 0)
	p.started = time.Now()
}

func (p *perfCounters) onUpdate(d time.Duration) {
	frames := atomic.LoadInt64(&p.frames)
	if frames == 0 {
		p.started = time.Now()
	}
	atomic.AddInt64(&p.frames, 1)
	if frames < perfUpdateWarmupFrames {
		return
	}
	atomic.AddInt64(&p.updSumNS, int64(d))
	for {
		max := atomic.LoadInt64(&p.updMaxNS)
		if int64(d) <= max {
			break
		}
		if atomic.CompareAndSwapInt64(&p.updMaxNS, max, int64(d)) {
			break
		}
	}
}

func (p *perfCounters) onDraw(d time.Duration) {
	if atomic.LoadInt64(&p.frames) < perfDrawWarmupFrames {
		return
	}
	atomic.AddInt64(&p.drawSumNS, int64(d))
	for {
		max := atomic.LoadInt64(&p.drawMaxNS)
		if int64(d) <= max {
			break
		}
		if atomic.CompareAndSwapInt64(&p.drawMaxNS, max, int64(d)) {
			break
		}
	}
}

func (p *perfCounters) onAudioEnq() { atomic.AddInt64(&p.aEnq, 1) }

func (p *perfCounters) onAudioDeq(qLatency, callDur time.Duration) {
	deq := atomic.AddInt64(&p.aDeq, 1)
	if deq <= perfAudioWarmupCount {
		return
	}
	atomic.AddInt64(&p.aQLatSumNS, int64(qLatency))
	for {
		max := atomic.LoadInt64(&p.aQLatMaxNS)
		if int64(qLatency) <= max {
			break
		}
		if atomic.CompareAndSwapInt64(&p.aQLatMaxNS, max, int64(qLatency)) {
			break
		}
	}
	atomic.AddInt64(&p.aCallSumNS, int64(callDur))
	for {
		max := atomic.LoadInt64(&p.aCallMaxNS)
		if int64(callDur) <= max {
			break
		}
		if atomic.CompareAndSwapInt64(&p.aCallMaxNS, max, int64(callDur)) {
			break
		}
	}
}

func (p *perfCounters) snapshot() PerfStats {
	elapsed := time.Since(p.started).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	frames := atomic.LoadInt64(&p.frames)
	updSum := atomic.LoadInt64(&p.updSumNS)
	updMax := atomic.LoadInt64(&p.updMaxNS)
	drwSum := atomic.LoadInt64(&p.drawSumNS)
	drwMax := atomic.LoadInt64(&p.drawMaxNS)
	aEnq := atomic.LoadInt64(&p.aEnq)
	aDeq := atomic.LoadInt64(&p.aDeq)
	aQLatSum := atomic.LoadInt64(&p.aQLatSumNS)
	aQLatMax := atomic.LoadInt64(&p.aQLatMaxNS)
	aCallSum := atomic.LoadInt64(&p.aCallSumNS)
	aCallMax := atomic.LoadInt64(&p.aCallMaxNS)

	fps := float64(frames) / elapsed
	activeElapsed := float64(updSum+drwSum) / 1e9
	if activeElapsed > 0 {
		fps = float64(frames) / activeElapsed
	}
	updAvgMS := 0.0
	if frames > 0 {
		updAvgMS = (float64(updSum) / float64(frames)) / 1e6
	}
	updMaxMS := float64(updMax) / 1e6
	drwAvgMS := 0.0
	if frames > 0 {
		drwAvgMS = (float64(drwSum) / float64(frames)) / 1e6
	}
	drwMaxMS := float64(drwMax) / 1e6
	qLatAvgMS := 0.0
	if aDeq > 0 {
		qLatAvgMS = (float64(aQLatSum) / float64(aDeq)) / 1e6
	}
	qLatMaxMS := float64(aQLatMax) / 1e6
	callAvgMS := 0.0
	if aDeq > 0 {
		callAvgMS = (float64(aCallSum) / float64(aDeq)) / 1e6
	}
	callMaxMS := float64(aCallMax) / 1e6

	s := PerfStats{
		Frames:       frames,
		FPSAvg:       fps,
		UpdateAvgMS:  updAvgMS,
		UpdateMaxMS:  updMaxMS,
		DrawAvgMS:    drwAvgMS,
		DrawMaxMS:    drwMaxMS,
		AudioEnq:     aEnq,
		AudioDeq:     aDeq,
		AudioQLatAvg: qLatAvgMS,
		AudioQLatMax: qLatMaxMS,
		AudioCallAvg: callAvgMS,
		AudioCallMax: callMaxMS,
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.HeapAllocKB = ms.Alloc / 1024
	s.HeapSysKB = ms.HeapSys / 1024
	s.HeapObjects = ms.HeapObjects
	s.Goroutines = runtime.NumGoroutine()
	return s
}
