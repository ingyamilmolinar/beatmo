package beat

import (
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

type Scheduler struct {
	Model  *model.Graph
	BPM    int
	now    func() time.Time
	last   time.Time
	OnBeat func(step int)
}

// Reset clears internal timers so the next Tick starts a new cycle.
func (s *Scheduler) Reset() { s.last = time.Time{} }

func NewScheduler(m *model.Graph) *Scheduler {
	return &Scheduler{
		Model: m,
		BPM:   120,
		now:   time.Now,
	}
}

func (s *Scheduler) Tick() {
	if s.BPM <= 0 {
		return
	}
	spb := time.Minute / time.Duration(s.BPM)
	if s.last.IsZero() {
		s.last = s.now()
		for i, on := range s.Model.Row {
			if on && s.OnBeat != nil {
				s.OnBeat(i)
			}
		}
		return
	}
	if s.now().Sub(s.last) < spb {
		return
	}
	s.last = s.now()

	for i, on := range s.Model.Row {
		if on && s.OnBeat != nil {
			s.OnBeat(i)
		}
	}
}
