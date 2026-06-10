package analyzer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// instSlot holds per-instrument registration and ring buffer.
type instSlot struct {
	id   string
	name string
	ring *RingBuffer
}

// pendingCap holds a trigger notification waiting to be processed. The
// buffer is kept as []float32 — the same shared, immutable reference to
// the cached recipe voice buffer the audio thread already holds. The
// audio thread does NOT allocate or convert here; conversion to
// []float64 happens lazily in processTick, and only for triggers that
// are actually consumed (dropped triggers — overwritten before the next
// tick — never pay the conversion cost).
type pendingCap struct {
	instID string
	bufF32 []float32
}

// Service is the analyzer goroutine that ingests audio data from ring
// buffers, runs wave observers, and publishes state atomically for the
// UI to read.
type Service struct {
	cfg Config

	// Instrument slots (written before Run, read during Run).
	mu    sync.Mutex
	slots []instSlot

	// Master ring buffer.
	masterRing *RingBuffer

	// Atomic state for lock-free publish/read.
	state atomic.Pointer[State]

	// Pending capture trigger (atomic swap from audio thread).
	pending atomic.Pointer[pendingCap]

	// Frozen state — when true, capture is not replaced.
	frozen atomic.Bool

	// Detail channel ID.
	detailID atomic.Pointer[string]

	// Monotonic trigger index for capture ordering.
	triggerIdx atomic.Int64

	// Lifecycle.
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewService creates an analyzer service with the given config.
func NewService(cfg Config) *Service {
	if cfg.FFTSize <= 0 {
		cfg.FFTSize = 1024
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 2048
	}
	if cfg.MaxInstruments <= 0 {
		cfg.MaxInstruments = 32
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 44100
	}

	// Ring buffer capacity: 4x window size to handle bursty writes.
	ringCap := cfg.WindowSize * 4

	return &Service{
		cfg:        cfg,
		slots:      make([]instSlot, cfg.MaxInstruments),
		masterRing: NewRingBuffer(ringCap),
		stopCh:     make(chan struct{}),
	}
}

// RegisterInstrument registers an instrument at the given slot index.
// Must be called before Run or while the service is running (mutex-protected).
func (s *Service) RegisterInstrument(slot int, id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if slot < 0 || slot >= len(s.slots) {
		return
	}
	ringCap := s.cfg.WindowSize * 4
	s.slots[slot] = instSlot{
		id:   id,
		name: name,
		ring: NewRingBuffer(ringCap),
	}
}

// PushInstBuf pushes audio samples into an instrument's ring buffer.
// Called from the audio thread — memcopy only.
func (s *Service) PushInstBuf(slot int, buf []float64) {
	s.mu.Lock()
	sl := s.slots[slot]
	s.mu.Unlock()
	if sl.ring == nil {
		return
	}
	sl.ring.Write(buf)
}

// PushMasterBuf pushes audio samples into the master ring buffer.
// Called from the audio thread — memcopy only.
func (s *Service) PushMasterBuf(buf []float64) {
	s.masterRing.Write(buf)
}

// NotifyTrigger notifies the service of a capture trigger from the audio
// thread. rawBuf is captured BY REFERENCE — it points at the cached,
// immutable recipe-rendered voice buffer (see tryRecipeVoice; the same
// []float32 is shared with cVoice.buf via the same read-only contract).
// Conversion to []float64 is deferred until processTick consumes the
// trigger, so dropped triggers (overwritten before the next tick) skip
// the conversion entirely.
//
// Previously this allocated a ~80 KB []float64 per call; the synth-tab
// profile attributed 252 MB of cumulative allocations to this single
// function over 60 seconds.
func (s *Service) NotifyTrigger(instID string, rawBuf []float32) {
	idx := s.triggerIdx.Add(1)
	cap := &pendingCap{
		instID: instID,
		bufF32: rawBuf,
	}
	_ = idx
	s.pending.Store(cap)
}

// Freeze freezes the capture — subsequent triggers will not replace it.
func (s *Service) Freeze() {
	s.frozen.Store(true)
}

// Unfreeze unfreezes the capture — new triggers can replace it.
func (s *Service) Unfreeze() {
	s.frozen.Store(false)
}

// SetDetailChannel sets the instrument ID for detailed analysis (FFT +
// envelope). Pass "" to disable detail.
func (s *Service) SetDetailChannel(id string) {
	s.detailID.Store(&id)
}

// State returns the latest published state. Returns nil if no state has
// been published yet. Lock-free (atomic load).
func (s *Service) State() *State {
	return s.state.Load()
}

// Run blocks until Stop is called. It runs the analysis loop on a
// ticker-based interval.
func (s *Service) Run() {
	s.wg.Add(1)
	defer s.wg.Done()

	// Tick interval = WindowSize / SampleRate seconds.
	intervalSec := float64(s.cfg.WindowSize) / float64(s.cfg.SampleRate)
	interval := time.Duration(intervalSec * float64(time.Second))
	if interval < time.Millisecond {
		interval = time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Reusable working buffers (goroutine-local).
	masterBuf := make([]float64, s.cfg.WindowSize)
	instBuf := make([]float64, s.cfg.WindowSize)

	// Observers.
	peakRMS := wave.NewPeakRMSObserver()
	masterFFT := wave.NewFFTObserver(s.cfg.FFTSize)
	envelope := wave.NewEnvelopeObserver(5, 50) // 5ms attack, 50ms release

	// Track last capture for carry-forward.
	var lastCapture *CaptureBuffer

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			st := s.processTick(masterBuf, instBuf, peakRMS, masterFFT, envelope, lastCapture)
			if st.Capture != nil {
				lastCapture = st.Capture
			}
			s.state.Store(st)
		}
	}
}

// Stop signals the service to stop and waits for it to finish.
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// processTick runs one analysis cycle and returns the new State.
func (s *Service) processTick(
	masterBuf, instBuf []float64,
	peakRMS wave.Observer,
	masterFFT wave.Observer,
	envelope wave.Observer,
	lastCapture *CaptureBuffer,
) *State {
	now := time.Now().UnixNano()
	sr := s.cfg.SampleRate

	// --- Master channel ---
	masterN := s.masterRing.Read(masterBuf)
	masterSamples := masterBuf[:masterN]

	masterW := wave.Wave{
		Samples:    masterSamples,
		SampleRate: sr,
		Label:      "master",
	}

	var master ChannelMetrics
	master.ID = "master"
	master.Name = "Master"

	if masterN > 0 {
		prObs := peakRMS.Observe(masterW)
		master.PeakDB = prObs.PeakDB
		master.RMSDB = prObs.RMSDB
		master.TruePeakDB = prObs.PeakDB // equals PeakDB until ISP lands
		master.HeadroomDB = -prObs.PeakDB
		master.ClipCount = prObs.ClipCount
		master.Active = true
		// Phase 2 audio-panel redesign: pull LUFS-S from the audio
		// package's K-weighted integrator (audio/loudness.go) via the
		// Config getter callback. Nil-safe.
		if s.cfg.MasterLUFSGetter != nil {
			master.LUFSShortTermDB = s.cfg.MasterLUFSGetter()
		}

		// Copy waveform.
		master.Waveform = make([]float64, masterN)
		copy(master.Waveform, masterSamples)

		// FFT on master.
		fftObs := masterFFT.Observe(masterW)
		master.FFTBins = fftObs.Bins
		master.FreqBins = fftObs.FreqBins

		// Envelope on master.
		envObs := envelope.Observe(masterW)
		master.Envelope = envObs.Envelope
	}

	// --- Instruments ---
	s.mu.Lock()
	slotsCopy := make([]instSlot, len(s.slots))
	copy(slotsCopy, s.slots)
	s.mu.Unlock()

	// Determine detail channel ID.
	var detailID string
	if p := s.detailID.Load(); p != nil {
		detailID = *p
	}

	var instruments []InstrumentMetrics
	var detail *ChannelMetrics

	for _, sl := range slotsCopy {
		if sl.ring == nil || sl.id == "" {
			continue
		}

		n := sl.ring.Read(instBuf)
		samples := instBuf[:n]

		im := InstrumentMetrics{
			ID:   sl.id,
			Name: sl.name,
		}

		if n > 0 {
			w := wave.Wave{
				Samples:    samples,
				SampleRate: sr,
				Label:      sl.id,
			}
			prObs := peakRMS.Observe(w)
			im.PeakDB = prObs.PeakDB
			im.RMSDB = prObs.RMSDB
			im.ClipCount = prObs.ClipCount
			im.Active = true

			// If this is the detail channel, compute full analysis.
			if sl.id == detailID {
				// Derived fields. TruePeakDB equals PeakDB until ISP
				// oversampling lands; HeadroomDB = 0 - PeakDB (so a -3 dB
				// peak yields 3 dB of headroom; negative on clip).
				headroom := -im.PeakDB
				dm := &ChannelMetrics{
					ID:         sl.id,
					Name:       sl.name,
					PeakDB:     im.PeakDB,
					RMSDB:     im.RMSDB,
					TruePeakDB: im.PeakDB,
					HeadroomDB: headroom,
					ClipCount:  im.ClipCount,
					Active:     true,
				}
				dm.Waveform = make([]float64, n)
				copy(dm.Waveform, samples)

				fftObs := masterFFT.Observe(w)
				dm.FFTBins = fftObs.Bins
				dm.FreqBins = fftObs.FreqBins

				envObs := envelope.Observe(w)
				dm.Envelope = envObs.Envelope

				detail = dm
			}
		}

		instruments = append(instruments, im)
	}

	// --- Capture ---
	var capture *CaptureBuffer

	// Check for pending trigger (atomic swap to nil). Convert the cached
	// float32 buffer to float64 lazily here — this is the first point in
	// the trigger pipeline where the conversion is unavoidable (wave.Wave
	// holds []float64). Allocation rate is bounded by the processTick rate
	// rather than the trigger rate; dropped triggers never pay this cost.
	if pc := s.pending.Swap(nil); pc != nil && !s.frozen.Load() {
		samples := make([]float64, len(pc.bufF32))
		for i, v := range pc.bufF32 {
			samples[i] = float64(v)
		}
		capture = &CaptureBuffer{
			InstID: pc.instID,
			Wave: wave.Wave{
				Samples:    samples,
				SampleRate: sr,
			},
			TriggerIdx: s.triggerIdx.Load(),
		}
	}

	// If frozen or no new trigger, carry forward last capture.
	if capture == nil && lastCapture != nil {
		capture = lastCapture
	}

	var clipsLastWindow int
	if s.cfg.ClipsLastWindowGetter != nil {
		clipsLastWindow = s.cfg.ClipsLastWindowGetter()
	}
	return &State{
		Instruments:     instruments,
		Master:          master,
		Detail:          detail,
		Capture:         capture,
		Timestamp:       now,
		ClipsLastWindow: clipsLastWindow,
	}
}
