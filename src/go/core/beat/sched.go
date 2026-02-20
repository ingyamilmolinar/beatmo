package beat

import (
	"math"
	"time"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type Scheduler struct {
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
	// Preserve fractional progress across BPM changes to avoid immediate
	// catch-up bursts. Map the elapsed fraction using the old spb into the
	// new spb so the next tick fires smoothly on the new timeline.
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
	s.running = true
	s.last = time.Time{}
	s.currentStep = 0
	if s.logger != nil {
		s.logger.Debugf("[SCHEDULER] Started")
	}
}

func (s *Scheduler) Stop() {
	s.running = false
	s.currentStep = 0
	s.last = time.Time{}
	if s.logger != nil {
		s.logger.Debugf("[SCHEDULER] Stopped")
	}
}

func (s *Scheduler) Tick() {
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
	if s.BPM <= 0 || s.last.IsZero() {
		return 0
	}
	spb := time.Minute / time.Duration(s.BPM)
	return float64(s.now().Sub(s.last)) / float64(spb)
}
