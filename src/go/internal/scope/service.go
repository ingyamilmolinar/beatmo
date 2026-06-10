package scope

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// ringBuf is a simple mutex-protected append buffer used by the scope
// service to collect samples pushed from the audio thread.
//
// lastPeak / lastRMS hold the latest batch statistics as float64 bits
// stored atomically — populated in push() so UI consumers can read
// per-stage stats without taking the buf mutex. Both default to NaN
// (Float64bits(0) == 0), which LatestPeak() maps to -Inf for "silent"
// rendering. Each push() overwrites them with stats for that batch.
type ringBuf struct {
	mu       sync.Mutex
	buf      []float64
	id       string // instrument ID of last push
	lastPeak atomic.Uint64
	lastRMS  atomic.Uint64
	hasData  atomic.Bool
}

// push appends samples and records the instrument ID. Also computes
// the batch's peak/RMS and stores them atomically so consumers can
// read per-stage stats without locking the buf mutex.
func (r *ringBuf) push(id string, samples []float64) {
	r.mu.Lock()
	r.buf = append(r.buf, samples...)
	r.id = id
	r.mu.Unlock()
	if len(samples) == 0 {
		return
	}
	var peak, sumSq float64
	for _, s := range samples {
		if s < 0 {
			s = -s
		}
		if s > peak {
			peak = s
		}
		sumSq += s * s
	}
	rms := math.Sqrt(sumSq / float64(len(samples)))
	r.lastPeak.Store(math.Float64bits(peak))
	r.lastRMS.Store(math.Float64bits(rms))
	r.hasData.Store(true)
}

// stats returns the latest batch's peak/RMS as linear amplitudes, plus
// whether any data has ever been pushed. Reads atomic fields only —
// safe from any goroutine without taking the buf mutex.
func (r *ringBuf) stats() (peak, rms float64, has bool) {
	if !r.hasData.Load() {
		return 0, 0, false
	}
	return math.Float64frombits(r.lastPeak.Load()), math.Float64frombits(r.lastRMS.Load()), true
}

// drain returns up to the last maxSamples samples, clears the buffer,
// and returns the instrument ID. If the buffer is empty the returned
// slice is nil. The returned slice is retained by TapData.Samples →
// atomic.Pointer[State]; external State() readers may keep their copy of
// State past the next tick, so the slice cannot be pooled/recycled
// without a lifetime hazard.
func (r *ringBuf) drain(maxSamples int) ([]float64, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.buf) == 0 {
		return nil, r.id
	}

	src := r.buf
	if len(src) > maxSamples {
		src = src[len(src)-maxSamples:]
	}

	out := make([]float64, len(src))
	copy(out, src)

	id := r.id
	r.buf = r.buf[:0]
	return out, id
}

// clear discards any buffered samples without allocating. Used by tick()
// for stages that are not currently tapped — those rings still receive
// pushes from the audio thread (the mixer pushes unconditionally to keep
// the synth/insertFX/EQ/sends/master stages live for tap switching) but
// their contents are never read. Before this clear() was added the tick
// loop called drain on every stage every cycle, allocating a maxSamples
// slice (~172 KB) only to discard it — observed at 508 MB cumulative in
// the synth-tab profile.
func (r *ringBuf) clear() {
	r.mu.Lock()
	r.buf = r.buf[:0]
	r.id = ""
	r.mu.Unlock()
}

// Service captures triggered waveforms from pipeline tap points and
// publishes State atomically for lock-free UI reads.
type Service struct {
	cfg    Config
	tapA   atomic.Int32
	tapB   atomic.Int32
	instID atomic.Pointer[string]
	frozen atomic.Bool
	rings  [stageCount]ringBuf
	state  atomic.Pointer[State]
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewService creates a scope service with the given config.
func NewService(cfg Config) *Service {
	if cfg.MaxWindowMs <= 0 {
		cfg.MaxWindowMs = 500
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 44100
	}

	s := &Service{
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	s.tapA.Store(-1)
	s.tapB.Store(-1)
	return s
}

// SetTapA sets the tap point for tap A.
func (s *Service) SetTapA(stage Stage) { s.tapA.Store(int32(stage)) }

// SetTapB sets the tap point for tap B.
func (s *Service) SetTapB(stage Stage) { s.tapB.Store(int32(stage)) }

// TapA returns the current stage for tap A, or -1 if unset.
func (s *Service) TapA() Stage { return Stage(s.tapA.Load()) }

// TapB returns the current stage for tap B, or -1 if unset.
func (s *Service) TapB() Stage { return Stage(s.tapB.Load()) }

// ClearTapA unsets tap A.
func (s *Service) ClearTapA() { s.tapA.Store(-1) }

// ClearTapB unsets tap B.
func (s *Service) ClearTapB() { s.tapB.Store(-1) }

// SetInstrument sets the instrument ID to scope.
func (s *Service) SetInstrument(id string) { s.instID.Store(&id) }

// Instrument returns the current scoped instrument ID.
func (s *Service) Instrument() string {
	p := s.instID.Load()
	if p == nil {
		return ""
	}
	return *p
}

// Freeze prevents new state from being published.
func (s *Service) Freeze() { s.frozen.Store(true) }

// Unfreeze allows new state to be published.
func (s *Service) Unfreeze() { s.frozen.Store(false) }

// IsFrozen returns whether the service is frozen.
func (s *Service) IsFrozen() bool { return s.frozen.Load() }

// PushSamples is called from the audio thread to push samples for a
// given pipeline stage and instrument.
func (s *Service) PushSamples(stage Stage, instID string, samples []float64) {
	if stage < 0 || int(stage) >= int(stageCount) {
		return
	}
	s.rings[stage].push(instID, samples)
}

// State returns the latest published state. Returns nil if no state
// has been published yet. Lock-free (atomic load).
func (s *Service) State() *State {
	return s.state.Load()
}

// LatestPeak returns the most recent batch's peak/RMS for the named
// stage as dB values. Stages that have never received any samples
// return -Inf for both. Phase 3 UI consumes this to paint a per-stage
// signal-flow display (every stage gets a mini-meter, not only the
// currently-tapped pair). Lock-free atomic read; safe from the UI
// goroutine while the audio thread is pushing.
func (s *Service) LatestPeak(stage Stage) (peakDB, rmsDB float64) {
	if int(stage) < 0 || int(stage) >= int(stageCount) {
		return math.Inf(-1), math.Inf(-1)
	}
	peak, rms, has := s.rings[stage].stats()
	if !has {
		return math.Inf(-1), math.Inf(-1)
	}
	return linearToDB(peak), linearToDB(rms)
}

// linearToDB converts a linear amplitude to dB. Zero / negative maps
// to -Inf so callers can distinguish silence from a quantised floor.
func linearToDB(v float64) float64 {
	if v <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(v)
}

// Run blocks until Stop is called. It runs a ticker-based loop that
// drains ring buffers and publishes scope state.
func (s *Service) Run() {
	s.wg.Add(1)
	defer s.wg.Done()

	// Tick at ~30 Hz — enough for smooth UI updates.
	interval := 33 * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

// Stop signals the service to stop and waits for it to finish.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// maxSamples returns the maximum number of samples to keep based on
// the configured window and sample rate.
func (s *Service) maxSamples() int {
	return s.cfg.SampleRate * s.cfg.MaxWindowMs / 1000
}

// isMasterStage returns true for stages that carry the composite mix
// rather than per-instrument signals (Sends, Master).
func isMasterStage(s Stage) bool {
	return s == StageSends || s == StageMaster
}

// tick runs one processing cycle.
func (s *Service) tick() {
	max := s.maxSamples()

	tapAStage := Stage(s.tapA.Load())
	tapBStage := Stage(s.tapB.Load())

	instID := s.Instrument()

	// Load previous state for carry-forward when a tap has no new data.
	prev := s.state.Load()

	// If frozen, clear all rings to prevent unbounded growth but
	// don't update published state. clear() is allocation-free.
	if s.frozen.Load() {
		for i := 0; i < int(stageCount); i++ {
			s.rings[i].clear()
		}
		return
	}

	var tapA *TapData
	var tapB *TapData

	// Process tap A.
	if tapAStage >= 0 && int(tapAStage) < int(stageCount) {
		samples, id := s.rings[tapAStage].drain(max)
		// Master-path stages always pass the instrument filter because
		// they contain the composite mix, not per-instrument signals.
		if len(samples) > 0 && (isMasterStage(tapAStage) || instID == "" || id == instID) {
			td := buildTapData(tapAStage, id, samples)
			tapA = &td
		}
	}

	// Process tap B.
	if tapBStage >= 0 && int(tapBStage) < int(stageCount) {
		samples, id := s.rings[tapBStage].drain(max)
		if len(samples) > 0 && (isMasterStage(tapBStage) || instID == "" || id == instID) {
			td := buildTapData(tapBStage, id, samples)
			tapB = &td
		}
	}

	// Clear unused rings to prevent unbounded growth. clear() is allocation-
	// free; the previous code path called drain() which always allocated a
	// maxSamples-sized slice (~172 KB) just to discard it.
	for i := 0; i < int(stageCount); i++ {
		stage := Stage(i)
		if stage == tapAStage || stage == tapBStage {
			continue
		}
		s.rings[i].clear()
	}

	// Publish new state, carrying forward previous tap data when no
	// new samples arrived for a tap this tick. The stage-match guard
	// prevents carrying stale data when the user switches tap points.
	if tapAStage >= 0 || tapBStage >= 0 {
		st := &State{
			Timestamp: time.Now().UnixNano(),
		}
		if tapA != nil {
			st.TapA = *tapA
		} else if prev != nil && prev.TapA.Active && prev.TapA.Stage == tapAStage {
			st.TapA = prev.TapA
		}
		if tapB != nil {
			st.TapB = *tapB
		} else if prev != nil && prev.TapB.Active && prev.TapB.Stage == tapBStage {
			st.TapB = prev.TapB
		}
		s.state.Store(st)
	}
}

// buildTapData computes peak, RMS, and dB values for a set of samples.
func buildTapData(stage Stage, instID string, samples []float64) TapData {
	var peak float64
	var sumSq float64
	for _, s := range samples {
		abs := math.Abs(s)
		if abs > peak {
			peak = abs
		}
		sumSq += s * s
	}

	rms := math.Sqrt(sumSq / float64(len(samples)))

	peakDB := -math.Inf(1)
	if peak > 0 {
		peakDB = 20 * math.Log10(peak)
	}

	rmsDB := -math.Inf(1)
	if rms > 0 {
		rmsDB = 20 * math.Log10(rms)
	}

	return TapData{
		Stage:   stage,
		InstID:  instID,
		Samples: samples,
		PeakDB:  peakDB,
		RMSDB:   rmsDB,
		Active:  true,
	}
}
