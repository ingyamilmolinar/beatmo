package songrender

import (
	"fmt"
	"io"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type NoteEvent struct {
	Row        int
	InstID     string
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

// NOTE: the engine predictor caps its sliding window at ~4096 steps; renders where
// bars*Subdiv exceeds that would silently drop the earliest notes. All current uses
// (2-8 bars) are far under the cap.
func BuildArrangement(pt ParsedTemplate, bars, sampleRate int) (Arrangement, error) {
	rp, err := BuildGraph(pt)
	if err != nil {
		return Arrangement{}, err
	}
	totalSteps := bars * pt.Subdiv
	if totalSteps <= 0 {
		return Arrangement{}, fmt.Errorf("songrender: non-positive totalSteps")
	}

	pred := engine.NewPredictor(rp.Graph, game_log.New(io.Discard, game_log.LevelError))
	pred.SetPaths(rp.Paths, rp.IsLoop, rp.LoopStart, rp.Nodes)
	pred.Ensure(totalSteps)

	// seconds per step = 60/BPM * 4/Subdiv (a beat = Subdiv/4 steps in 4/4).
	secPerStep := 60.0 / float64(pt.BPM) * 4.0 / float64(pt.Subdiv)
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
			arr.Notes = append(arr.Notes, NoteEvent{
				Row: row, InstID: inst.ID, AbsStep: abs,
				AtSample:   int(math.Round(float64(abs) * samplesPerStep)),
				PitchSemis: node.Params.Pitch,
				Volume:     vol,
				DurSec:     dur,
			})
		}
	}
	return arr, nil
}
