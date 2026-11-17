package ui

import (
	"hash/fnv"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// laneKind is what an audio-lane cell does.
type laneKind uint8

const (
	laneSound laneKind = iota + 1 // a voice to play
	laneStop                      // a mute node: StopLaneAt(Inst, Time)
)

// laneNote is one cell's sound as the audio lane commits it.
type laneNote struct {
	Row, Abs        int
	Kind            laneKind
	Inst            instrument.ID
	Vol, Pitch, Dur float64
	Time            float64 // audio-clock start: grid + nudge + AudioOffsetSec
	Grid            float64 // grid + nudge, no offset (the probe's "predicted time")
	Hash            uint64  // identity: any field (or the voice) changing ⇒ different Hash
	node            uint64  // the node the cell plays (part of Hash)
	// Layers is the cell's voice stack (a chord node); nil = one voice at
	// (Pitch, Vol). Voice k sounds at Pitch+Layers[k].Semis, Vol×Layers[k].Gain
	// and owns voice id base+k (the lane mints a contiguous block per cell).
	Layers []model.Layer
}

// voiceCount is how many engine voices the note occupies (≥ 1).
func (n *laneNote) voiceCount() int {
	if len(n.Layers) == 0 {
		return 1
	}
	return len(n.Layers)
}

// voiceAt is voice k's pitch and volume. A single-voice note returns its own
// fields untouched (bit-identical to before voice stacks existed).
func (n *laneNote) voiceAt(k int) (pitch, vol float64) {
	if len(n.Layers) == 0 {
		return n.Pitch, n.Vol
	}
	l := n.Layers[k]
	return n.Pitch + l.Semis, n.Vol * l.Gain
}

// laneVoiceIDs lists the voice ids a committed note owns.
func laneVoiceIDs(base uint64, n *laneNote) []uint64 {
	out := make([]uint64, n.voiceCount())
	for k := range out {
		out[k] = base + uint64(k)
	}
	return out
}

// laneCtx is computed once per lane tick, not per note.
type laneCtx struct {
	snap    *seqPathSnapshot
	bpm     int
	div     int
	anySolo bool
	offset  float64
}

func (s *playbackSlate) laneContext() (laneCtx, bool) {
	g := s.g
	snap := g.seqPathSnapshot()
	if snap == nil || g.drum == nil || g.engine == nil || g.engine.Predictor == nil {
		return laneCtx{}, false
	}
	bpm := g.state.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	div := g.grid.MaxDiv()
	if bpm <= 0 || div <= 0 {
		return laneCtx{}, false
	}
	anySolo := false
	for _, r := range g.drum.Rows {
		if r.Solo {
			anySolo = true
			break
		}
	}
	return laneCtx{snap: snap, bpm: bpm, div: div, anySolo: anySolo, offset: RuntimeProf().AudioOffsetSec}, true
}

// laneNoteAt is the audio lane's PURE note function: what cell (row, abs)
// sounds like under current state. No counters, gates or side effects — the
// lane may evaluate the same cell any number of times. Audibility comes from
// the predictor (which owns the stateful logic walk); volume / pitch /
// duration from the same evalNodeParamsOnly the legacy sequencer uses, plus
// the custom-logic decision the predictor recorded for the cell.
func (s *playbackSlate) laneNoteAt(c *laneCtx, row, abs int) (laneNote, bool) {
	g := s.g
	if row < 0 || row >= len(g.drum.Rows) {
		return laneNote{}, false
	}
	info := seqBeatInfoAtRow(c.snap, row, abs)
	inst := g.drum.Rows[row].Instrument
	pred := g.engine.Predictor
	var n laneNote
	switch info.NodeType {
	case model.NodeTypeMute:
		if !pred.TriggeredAt(row, abs) {
			return laneNote{}, false
		}
		n.Kind = laneStop
	case model.NodeTypeRegular, model.NodeTypeChord:
		if !pred.AudibleAt(row, abs) {
			return laneNote{}, false
		}
		r := g.drum.Rows[row]
		if r.Muted || (c.anySolo && !r.Solo) {
			return laneNote{}, false
		}
		if !g.drum.IsInstrumentAvailable(inst) && g.playFn == nil && g.scheduleHook == nil {
			return laneNote{}, false
		}
		vol, pitch, dur := g.evalNodeParamsOnly(row, abs, info)
		if dec, ok := pred.DecisionAt(row, abs); ok {
			vol, pitch, dur = applyNodeDecision(vol, pitch, dur, dec)
		}
		if vol < 0 {
			vol = 0
		}
		n.Kind, n.Vol, n.Pitch, n.Dur = laneSound, vol, pitch, dur
		n.Layers = s.nodeLayersAt(info)
	default:
		return laneNote{}, false
	}
	grid := g.state.AudioTimeOfStep(abs, c.div, c.bpm) + g.nodeNudgeSecondsAt(row, abs, info)
	n.Row, n.Abs, n.Inst, n.Grid, n.Time = row, abs, inst, grid, grid+c.offset
	n.node = uint64(info.NodeID)
	n.Hash = laneNoteHash(&n, n.node, audio.VoiceRevision(inst))
	return n, true
}

func laneNoteHash(n *laneNote, node, voiceRev uint64) uint64 {
	h := fnv.New64a()
	var b [8]byte
	w := func(x uint64) {
		for i := range b {
			b[i] = byte(x >> (8 * i))
		}
		_, _ = h.Write(b[:])
	}
	_, _ = h.Write([]byte(n.Inst))
	w(uint64(n.Kind))
	w(node)
	w(voiceRev)
	w(math.Float64bits(n.Vol))
	w(math.Float64bits(n.Pitch))
	w(math.Float64bits(n.Dur))
	w(math.Float64bits(math.Round(n.Time*1e6) / 1e6)) // µs: float noise must not churn recommits
	for _, l := range n.Layers {
		w(math.Float64bits(l.Semis))
		w(math.Float64bits(l.Gain))
	}
	return h.Sum64()
}
