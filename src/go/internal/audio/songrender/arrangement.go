package songrender

import (
	"fmt"
	"io"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type NoteEvent struct {
	Row        int
	InstID     instrument.ID
	AbsStep    int
	AtSample   int
	PitchSemis float64
	Volume     float64
	DurSec     float64
}

type Arrangement struct {
	BPM, Subdiv, Bars, SampleRate int
	Instruments                   []InstrumentSpec
	Notes                         []NoteEvent
	TotalSamples                  int
}

// beatInfoAt returns the BeatInfo at absolute step abs for a row path, honoring
// the row's loop (mirrors the predictor's own index folding).
func beatInfoAt(path []model.BeatInfo, isLoop bool, loopStart, abs int) (model.BeatInfo, bool) {
	if len(path) == 0 {
		return model.BeatInfo{}, false
	}
	if abs < len(path) {
		return path[abs], true
	}
	if !isLoop {
		return model.BeatInfo{}, false
	}
	loopLen := len(path) - loopStart
	if loopLen <= 0 {
		return model.BeatInfo{}, false
	}
	idx := loopStart + ((abs - loopStart) % loopLen)
	return path[idx], true
}

// StepSeconds is the duration of one path step: a template's subdiv is grid
// units per BEAT (the engine's MaxDiv), and the sequencer advances one unit per
// step. This read "units per bar" until 2026-10-02 (60/BPM * 4/Subdiv), which
// rendered every template at a quarter of its tempo.
func StepSeconds(bpm, subdiv int) float64 {
	return 60.0 / float64(bpm) / float64(subdiv)
}

// NOTE: the engine predictor caps its sliding window at ~4096 steps; renders where
// bars*4*Subdiv exceeds that would silently drop the earliest notes. All current
// uses (2-8 bars) are far under the cap.
func BuildArrangement(pt ParsedTemplate, bars, sampleRate int) (Arrangement, error) {
	rp, err := BuildGraph(pt)
	if err != nil {
		return Arrangement{}, err
	}
	// A bar is four beats of Subdiv units each.
	totalSteps := bars * 4 * pt.Subdiv
	if totalSteps <= 0 {
		return Arrangement{}, fmt.Errorf("songrender: non-positive totalSteps")
	}

	pred := engine.NewPredictor(rp.Graph, game_log.New(io.Discard, game_log.LevelError))
	pred.SetPaths(rp.Paths, rp.IsLoop, rp.LoopStart, rp.Nodes)
	pred.Ensure(totalSteps)

	secPerStep := StepSeconds(pt.BPM, pt.Subdiv)
	samplesPerStep := secPerStep * float64(sampleRate)

	arr := Arrangement{
		BPM: pt.BPM, Subdiv: pt.Subdiv, Bars: bars, SampleRate: sampleRate,
		Instruments:  pt.Instruments,
		TotalSamples: int(math.Round(float64(totalSteps) * samplesPerStep)),
	}
	for row := 0; row < len(rp.Paths); row++ {
		inst := pt.Instruments[row]
		for abs := 0; abs < totalSteps; abs++ {
			if !pred.AudibleAt(row, abs) {
				continue
			}
			bi, ok := beatInfoAt(rp.Paths[row], rp.IsLoop[row], rp.LoopStart[row], abs)
			if !ok {
				continue
			}
			node := rp.Nodes[bi.NodeID]
			dur := node.Params.Duration
			if dur <= 0 {
				dur = 1.0
			}
			dur *= audio.ConfigForInstrument(inst.ID).DurationSec
			vol := node.Params.Volume
			if vol <= 0 {
				vol = 1.0
			}
			vol *= inst.Volume
			at := float64(abs)*samplesPerStep +
				node.Params.GrooveKind.Seconds(node.Params.GroovePct, secPerStep)*float64(sampleRate)
			ev := NoteEvent{
				Row: row, InstID: inst.ID, AbsStep: abs,
				AtSample:   int(math.Max(0, math.Round(at))),
				PitchSemis: node.Params.Pitch,
				Volume:     vol,
				DurSec:     dur,
			}
			layers := model.NodeLayers(node.Type, node.Params)
			if layers == nil {
				arr.Notes = append(arr.Notes, ev)
			}
			for _, l := range layers { // a chord: one note per voice
				v := ev
				v.PitchSemis += l.Semis
				v.Volume *= l.Gain
				arr.Notes = append(arr.Notes, v)
			}
		}
	}
	return arr, nil
}
