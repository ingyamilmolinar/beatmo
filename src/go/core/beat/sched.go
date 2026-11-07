package beat

import (
	"math"
	"sync"
	"time"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Tick runs on the engine loop's goroutine while UI/BPM goroutines call Start/Stop/SetBPM and read Progress; mu guards
// the mutable fields, and the ones set before the loop starts are read without it.
type Scheduler struct {
	mu          sync.Mutex
	BPM         int
	now         func() time.Time
	last        time.Time
	OnTick      func(step int)
	running     bool
	currentStep int
	BeatLength  int
	logger      *game_log.Logger
}

func NewScheduler(logger *game_log.Logger) *Scheduler {
	return &Scheduler{
		BPM:         120,
		now:         time.Now,
		currentStep: 0,
		BeatLength:  16,
		logger:      logger,
	}
}

func (s *Scheduler) SetBPM(bpm int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// The elapsed fraction maps from the old spb into the new, so a BPM change never causes a catch-up burst.
	old := s.BPM
	if bpm <= 0 {
		s.BPM = bpm
		return
	}
	if !s.last.IsZero() && old > 0 && old != bpm {
		now := s.now()
		oldSpb := time.Minute / time.Duration(old)
		newSpb := time.Minute / time.Duration(bpm)
		elapsed := now.Sub(s.last)
		// Fraction of the current beat with old BPM; keep it within [0,1).
		frac := float64(elapsed) / float64(oldSpb)
		frac -= math.Floor(frac)
		// Rebase last so that now - last corresponds to the same fraction of new beat.
		adj := time.Duration(frac * float64(newSpb))
		s.last = now.Add(-adj)
	}
	s.BPM = bpm
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = true
	s.last = time.Time{}
	s.currentStep = 0
	if s.logger != nil {
		s.logger.Debugf("[scheduler] Started")
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.currentStep = 0
	s.last = time.Time{}
	if s.logger != nil {
		s.logger.Debugf("[scheduler] Stopped")
	}
}

func (s *Scheduler) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.BPM <= 0 {
		return
	}

	spb := time.Minute / time.Duration(s.BPM)
	now := s.now()

	if s.last.IsZero() {
		// Fire immediately on the first call
		s.last = now.Add(-spb)
	}

	for now.Sub(s.last) >= spb {
		s.last = s.last.Add(spb)
		if s.OnTick != nil {
			s.OnTick(s.currentStep)
		}
		s.currentStep = (s.currentStep + 1) % s.BeatLength
	}
}

// Progress returns the fraction of the current beat that has elapsed.
func (s *Scheduler) Progress() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.BPM <= 0 || s.last.IsZero() {
		return 0
	}
	spb := time.Minute / time.Duration(s.BPM)
	return float64(s.now().Sub(s.last)) / float64(spb)
}

// The run loop and SetBPM mutate the exported BPM field, so concurrent readers use this.
func (s *Scheduler) CurrentBPM() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.BPM
}
