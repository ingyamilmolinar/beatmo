package core

import (
	"time"
)

type tick struct {
	at      int
	id      NodeID
	port    PortName
	payload Payload
}

type Scheduler struct {
	grid        *Grid
	bpm         int
	res         int
	q           []tick
	clock       int
	OnPlayEvent func(Payload)
}

func NewScheduler(g *Grid, bpm, res int, cb func(Payload)) *Scheduler {
	return &Scheduler{grid: g, bpm: bpm, res: res, OnPlayEvent: cb}
}

func (s *Scheduler) Tick() {
	for i := 0; i < len(s.q); {
		if s.q[i].at > s.clock {
			i++
			continue
		}
		ev := s.q[i]
		s.q[i] = s.q[len(s.q)-1]
		s.q = s.q[:len(s.q)-1]

		node := s.grid.nodes[ev.id]
		out := node.OnEvent(ev.port, ev.payload)

		if s.OnPlayEvent != nil {
			s.OnPlayEvent(ev.payload)
		}

		for _, e := range s.grid.edges {
			if e.Source != ev.id {
				continue
			}
			if outs, ok := out[e.SrcOut]; ok {
				for _, p := range outs {
					s.q = append(s.q, tick{
						at:      s.clock + e.Delay,
						id:      e.Target,
						port:    e.TgtIn,
						payload: p,
					})
				}
			}
		}
	}
	s.clock++
}

func (s *Scheduler) TickDur() time.Duration {
	beat := 60.0 / float64(s.bpm)
	sec := beat / float64(s.res)
	return time.Duration(sec * float64(time.Second))
}
