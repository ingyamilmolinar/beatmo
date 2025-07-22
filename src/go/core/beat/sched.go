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
		// For the first tick, ensure it fires immediately
		s.last = now.Add(-spb) // Set last to be one beat duration in the past
	}

	if now.Sub(s.last) < spb {
		return
	}

	s.last = now

	if s.OnTick != nil {
		s.OnTick(s.currentStep)
	}

	s.currentStep = (s.currentStep + 1) % s.BeatLength
}
