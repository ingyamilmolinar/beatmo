// Package scopeexport provides a continuous flight-recorder service that
// captures audio pipeline data at all 6 stages for every instrument and
// the master bus, writing JSONL snapshots for automated verification and
// human diagnosis.
//
// Activate via CLI flag (-scope-export) or env var (SCOPE_EXPORT=1).
// Each JSONL line is a self-contained snapshot with per-channel metrics,
// downsampled waveforms, and FFT peaks.
//
// Analysis with jq:
//
//	# Latest snapshot (pretty-printed)
//	tail -1 scope_export.jsonl | jq .
//
//	# Kick peak_db at synth stage over time
//	jq -r '.channels[] | select(.id=="kick") | .stages.synth.peak_db' scope_export.jsonl
//
//	# Master RMS trend
//	jq -r '[.tick, .master.stages.master.rms_db] | @tsv' scope_export.jsonl
//
//	# Find clipping events
//	jq 'select([.channels[].stages[].clip_count] | add > 0)' scope_export.jsonl
//
//	# Compare pre-EQ vs post-EQ
//	jq -r '.channels[] | select(.id=="kick") | [.stages.antipop.peak_db, .stages.eq.peak_db] | @tsv' scope_export.jsonl
//
//	# Feed last 5 snapshots to LLM
//	tail -5 scope_export.jsonl | jq -s .
package scopeexport

import (
	"sync"
	"time"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// stageKeyNames maps scope.Stage to the JSON key used in snapshots.
var stageKeyNames = [6]string{"synth", "antipop", "insertfx", "eq", "sends", "master"}

// Config holds configuration for the export service.
type Config struct {
	SampleRate   int
	Interval     time.Duration // default 2s
	OutputPath   string        // default "scope_export.jsonl"
	FFTSize      int           // default 1024
	WaveformBins int           // default 64
	FFTTopN      int           // default 16

	// Callbacks avoid circular import with audio package.
	BPMFunc         func() int
	InstrumentsFunc func() []string
	LookupMeta      func(id string) (name, kind string, ok bool)
	ChannelVolume   func(id string) float64
	ChannelPan      func(id string) float64
	MainVolume      func() float64
}

// ringBuf is a mutex-protected append buffer for audio samples.
type ringBuf struct {
	mu  sync.Mutex
	buf []float64
}

func (r *ringBuf) push(samples []float64) {
	r.mu.Lock()
	r.buf = append(r.buf, samples...)
	r.mu.Unlock()
}

func (r *ringBuf) drain() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) == 0 {
		return nil
	}
	out := make([]float64, len(r.buf))
	copy(out, r.buf)
	r.buf = r.buf[:0]
	return out
}

// stageRings holds ring buffers for all instruments at a single pipeline stage.
type stageRings struct {
	mu    sync.Mutex
	rings map[string]*ringBuf
}

func (sr *stageRings) getOrCreate(id string) *ringBuf {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if sr.rings == nil {
		sr.rings = make(map[string]*ringBuf)
	}
	rb, ok := sr.rings[id]
	if !ok {
		rb = &ringBuf{}
		sr.rings[id] = rb
	}
	return rb
}

func (sr *stageRings) drainAll() map[string][]float64 {
	sr.mu.Lock()
	ids := make([]string, 0, len(sr.rings))
	rbs := make([]*ringBuf, 0, len(sr.rings))
	for id, rb := range sr.rings {
		ids = append(ids, id)
		rbs = append(rbs, rb)
	}
	sr.mu.Unlock()

	result := make(map[string][]float64)
	for i, rb := range rbs {
		if samples := rb.drain(); len(samples) > 0 {
			result[ids[i]] = samples
		}
	}
	return result
}

// Service captures audio pipeline data and writes JSONL snapshots.
type Service struct {
	cfg     Config
	stages  [6]stageRings
	stopCh  chan struct{}
	wg      sync.WaitGroup
	tickNum int64
	startAt time.Time
	buffer  serviceBuffer // in-memory JSONL sink (WASM)
}

// NewService creates a new export service with the given config.
func NewService(cfg Config) *Service {
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 44100
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Second
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = "scope_export.jsonl"
	}
	if cfg.FFTSize <= 0 {
		cfg.FFTSize = 1024
	}
	if cfg.WaveformBins <= 0 {
		cfg.WaveformBins = 64
	}
	if cfg.FFTTopN <= 0 {
		cfg.FFTTopN = 16
	}
	return &Service{
		cfg:     cfg,
		stopCh:  make(chan struct{}),
		startAt: time.Now(),
	}
}

// PushSamples is called from the audio thread to push samples for a
// pipeline stage and instrument. Lock-free on the fast path when the
// ring buffer already exists.
func (s *Service) PushSamples(stage scope.Stage, id string, samples []float64) {
	if int(stage) < 0 || int(stage) >= len(s.stages) {
		return
	}
	rb := s.stages[stage].getOrCreate(id)
	rb.push(samples)
}

// Run blocks until Stop is called, writing JSONL snapshots at the
// configured interval.
func (s *Service) Run() {
	s.wg.Add(1)
	defer s.wg.Done()

	w, err := openWriter(s.cfg.OutputPath)
	if err != nil {
		return
	}
	defer w.close()

	// Pre-allocate observers (goroutine-local, no contention).
	peakObs := wave.NewPeakRMSObserver()
	fftObs := wave.NewFFTObserver(s.cfg.FFTSize)
	zcObs := wave.NewZeroCrossingObserver()

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tickNum++
			snap := s.buildSnapshot(peakObs, fftObs, zcObs)
			_ = w.writeLine(snap)
		}
	}
}

// Stop signals the service to stop and waits for completion.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// buildSnapshot drains all ring buffers and assembles a complete snapshot.
func (s *Service) buildSnapshot(peakObs, fftObs, zcObs wave.Observer) *Snapshot {
	now := time.Now()
	snap := &Snapshot{
		TS:         now.UTC().Format(time.RFC3339Nano),
		Tick:       s.tickNum,
		ElapsedMs:  now.Sub(s.startAt).Milliseconds(),
		SampleRate: s.cfg.SampleRate,
	}

	if s.cfg.BPMFunc != nil {
		snap.BPM = s.cfg.BPMFunc()
	}

	// Drain all stages.
	var drainedStages [6]map[string][]float64
	for i := 0; i < 6; i++ {
		drainedStages[i] = s.stages[i].drainAll()
	}

	// Collect all unique instrument IDs (excluding "master").
	instSet := make(map[string]struct{})
	for _, stage := range drainedStages {
		for id := range stage {
			if id != "master" {
				instSet[id] = struct{}{}
			}
		}
	}

	// Build channel snapshots.
	for id := range instSet {
		ch := ChannelSnap{
			ID:     id,
			Stages: make(map[string]*StageMetrics),
		}
		if s.cfg.LookupMeta != nil {
			if name, kind, ok := s.cfg.LookupMeta(id); ok {
				ch.Name = name
				ch.Kind = kind
			}
		}
		if s.cfg.ChannelVolume != nil {
			ch.Volume = s.cfg.ChannelVolume(id)
		}
		if s.cfg.ChannelPan != nil {
			ch.Pan = s.cfg.ChannelPan(id)
		}

		// Per-instrument stages: synth (0), antipop (1), insertfx (2), eq (3).
		for stageIdx := 0; stageIdx <= 3; stageIdx++ {
			if samples, ok := drainedStages[stageIdx][id]; ok {
				metrics := computeStageMetrics(samples, s.cfg.SampleRate, s.cfg.WaveformBins, s.cfg.FFTTopN, peakObs, fftObs, zcObs)
				ch.Stages[stageKeyNames[stageIdx]] = metrics
			}
		}

		snap.Channels = append(snap.Channels, ch)
	}

	// Build master snapshot.
	snap.Master = MasterSnap{
		Stages: make(map[string]*StageMetrics),
	}
	if s.cfg.MainVolume != nil {
		snap.Master.Volume = s.cfg.MainVolume()
	}
	// Master stages: sends (4), master (5).
	for stageIdx := 4; stageIdx <= 5; stageIdx++ {
		if samples, ok := drainedStages[stageIdx]["master"]; ok {
			metrics := computeStageMetrics(samples, s.cfg.SampleRate, s.cfg.WaveformBins, s.cfg.FFTTopN, peakObs, fftObs, zcObs)
			snap.Master.Stages[stageKeyNames[stageIdx]] = metrics
		}
	}

	return snap
}
