package beat

import (
	"log"
	"time"
)

type Scheduler struct {
	BPM         int
	now         func() time.Time
	last        time.Time
	OnTick      func(step int)
	running     bool
	currentStep int
	BeatLength  int
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		BPM:         120,
		now:         time.Now,
		currentStep: 0,
		BeatLength:  16,
	}
}

func (s *Scheduler) SetBPM(bpm int) {
	s.BPM = bpm
}

func (s *Scheduler) Start() {
	s.running = true
	s.last = time.Time{}
	s.currentStep = 0
	log.Printf("[SCHEDULER] Started")
}

func (s *Scheduler) Stop() {
	s.running = false
	s.currentStep = 0
	log.Printf("[SCHEDULER] Stopped")
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
