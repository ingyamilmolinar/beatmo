package analyzer

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// instSlot holds per-instrument registration and ring buffer.
type instSlot struct {
	id   channel.ID
	name string
	ring *RingBuffer
}

// The same shared, immutable voice buffer the audio thread holds: no allocation or conversion there. processTick
// converts only triggers it consumes, so overwritten ones never pay.
type pendingCap struct {
	instID channel.ID
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
	detailID atomic.Pointer[channel.ID]

	// Monotonic trigger index for capture ordering.
	triggerIdx atomic.Int64

	// Atomic so Reset can clear it from the UI goroutine when a circuit loads.
	lastCapture atomic.Pointer[CaptureBuffer]

	// Bumped by Reset: the Run loop drains the rings and resets ballistics on its next tick, so a freshly loaded, stopped
	// circuit reads flat instead of replaying the previous one.
	clearGen atomic.Int64

	// Lifecycle.
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewService creates an analyzer service with the given config.
func NewService(cfg Config) *Service {
	if cfg.FFTSize <= 0 {
		cfg.FFTSize = SpectrumFFTSize
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
func (s *Service) RegisterInstrument(slot int, id channel.ID, name string) {
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

// Audio thread: memcopy only.
func (s *Service) PushInstBuf(slot int, buf []float64) {
	// The mixer mints a slot per unique instrument with no cap, so a session cycling many instruments exceeds the array; an
	// out-of-range slot was never registered, so it is dropped.
	if slot < 0 || slot >= len(s.slots) {
		return
	}
	s.mu.Lock()
	sl := s.slots[slot]
	s.mu.Unlock()
	if sl.ring == nil {
		return
	}
	sl.ring.Write(buf)
}

// Audio thread: memcopy only.
func (s *Service) PushMasterBuf(buf []float64) {
	s.masterRing.Write(buf)
}

// rawBuf is captured BY REFERENCE (the cached, immutable voice buffer, read-only by contract); processTick converts
// only triggers it consumes, so the audio thread never allocates a per-trigger copy.
func (s *Service) NotifyTrigger(instID channel.ID, rawBuf []float32) {
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
func (s *Service) SetDetailChannel(id channel.ID) {
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

	// Track the clear generation so a Reset (bumped clearGen) drains rings and
	// resets observer state exactly once, on the consumer goroutine.
	var seenClearGen int64

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if g := s.clearGen.Load(); g != seenClearGen {
				seenClearGen = g
				// Drain any leftover pre-load samples (consumer-side, safe).
				s.masterRing.Reset()
				s.mu.Lock()
				for i := range s.slots {
					if s.slots[i].ring != nil {
						s.slots[i].ring.Reset()
					}
				}
				s.mu.Unlock()
				// Reset observer ballistics/hold state so the next tick seeds
				// fresh instead of decaying from the previous circuit.
				peakRMS = wave.NewPeakRMSObserver()
				masterFFT = wave.NewFFTObserver(s.cfg.FFTSize)
				envelope = wave.NewEnvelopeObserver(5, 50)
			}
			st := s.processTick(masterBuf, instBuf, peakRMS, masterFFT, envelope)
			s.state.Store(st)
		}
	}
}

// Publishes an empty state at once (readers see flat data the moment a load completes) and signals Run to drain the rings
// and reset observers on its next tick. Safe from the UI goroutine.
func (s *Service) Reset() {
	s.pending.Store(nil)
	s.lastCapture.Store(nil)
	s.clearGen.Add(1)
	s.state.Store(&State{
		Master:    ChannelMetrics{ID: channel.Master, Name: "Master"},
		Timestamp: time.Now().UnixNano(),
	})
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
) *State {
	now := time.Now().UnixNano()
	sr := s.cfg.SampleRate

	// --- Master channel ---
	masterN := s.masterRing.Read(masterBuf)
	masterSamples := masterBuf[:masterN]

	masterW := wave.Wave{
		Samples:    masterSamples,
		SampleRate: sr,
		Label:      channel.Master.String(),
	}

	var master ChannelMetrics
	master.ID = channel.Master
	master.Name = "Master"

	if masterN > 0 {
		prObs := peakRMS.Observe(masterW)
		master.PeakDB = prObs.PeakDB
		master.RMSDB = prObs.RMSDB
		master.TruePeakDB = prObs.PeakDB // equals PeakDB until ISP lands
		master.HeadroomDB = -prObs.PeakDB
		master.ClipCount = prObs.ClipCount
		master.Active = true
		// The audio package owns the K-weighted integrator, reached through the Config getter (nil-safe).
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
	var detailID channel.ID
	if p := s.detailID.Load(); p != nil {
		detailID = *p
	}

	var instruments []InstrumentMetrics
	var detail *ChannelMetrics

	for _, sl := range slotsCopy {
		if sl.ring == nil || sl.id.IsZero() {
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
				Label:      sl.id.String(),
			}
			prObs := peakRMS.Observe(w)
			im.PeakDB = prObs.PeakDB
			im.RMSDB = prObs.RMSDB
			im.ClipCount = prObs.ClipCount
			im.Active = true

			// If this is the detail channel, compute full analysis.
			if sl.id == detailID {
				// TruePeakDB is PeakDB (no oversampling); HeadroomDB = -PeakDB (a -3 dB peak is 3 dB of headroom, negative on clip).
				headroom := -im.PeakDB
				dm := &ChannelMetrics{
					ID:         sl.id,
					Name:       sl.name,
					PeakDB:     im.PeakDB,
					RMSDB:      im.RMSDB,
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

	// The first point the conversion is unavoidable (wave.Wave holds float64): bounded by the tick rate, not the trigger rate.
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
		// Remember for carry-forward across subsequent (quiet) ticks. Reset
		// clears this so a circuit load can't replay the previous capture.
		s.lastCapture.Store(capture)
	}

	// If frozen or no new trigger, carry forward the last capture.
	if capture == nil {
		capture = s.lastCapture.Load()
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
