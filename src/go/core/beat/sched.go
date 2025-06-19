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
	step   int
	OnBeat func(step int)
}

func NewScheduler(m *model.Graph) *Scheduler {
	return &Scheduler{
		Model: m,
		BPM:   120,
		now:   time.Now,
		step:  0,
	}
}

// Reset clears the internal timer and restarts playback from step zero.
func (s *Scheduler) Reset() {
	s.last = time.Time{}
	s.step = 0
}

func (s *Scheduler) Tick() {
	if s.BPM <= 0 {
		return
	}
	if len(s.Model.Row) == 0 {
		return
	}
	spb := time.Minute / time.Duration(s.BPM)
	if s.last.IsZero() {
		s.last = s.now()
		if s.Model.Row[s.step] && s.OnBeat != nil {
			s.OnBeat(s.step)
		}
		return
	}
	if s.now().Sub(s.last) < spb {
		return
	}
	s.last = s.now()

	s.step = (s.step + 1) % len(s.Model.Row)
	if s.Model.Row[s.step] && s.OnBeat != nil {
		s.OnBeat(s.step)
	}
}
