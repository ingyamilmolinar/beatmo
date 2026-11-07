//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Against the LIVE configuration, every stage of every channel captured so the browser's graph can be compared tap by
// tap.

// A Lane note is an audio-lane voice (vid > 0), so StageStops can fade it.
type StageNote struct {
	Inst  instrument.ID
	At    int
	Pitch float64
	Vol   float64
	Dur   float64
	Lane  bool
}

// A mute node: lane voices of Inst that started before At fade.
type StageStop struct {
	Inst instrument.ID
	At   int
}

// Sample-aligned from the render's first sample: Voices (× headroom), after Inserts, after EQ (what reaches the master).
type StageTaps struct {
	Voices, Inserts, EQ []float64
}

var stageCapture struct {
	sync.Mutex
	on   bool
	taps map[channel.ID]*StageTaps
}

// stageCaptureOn is the mixer's lock-free check (zero cost outside a render).
var stageCaptureOn atomic.Bool

// Pads the silence since the channel was last active.
func captureStage(dst *[]float64, at int, block []float64) {
	for len(*dst) < at {
		*dst = append(*dst, 0)
	}
	*dst = append(*dst, block...)
}

// Called from processBlock only while a stage render runs.
func stageCaptureBlock(id channel.ID, at int, voices, inserts, eq []float64) {
	stageCapture.Lock()
	defer stageCapture.Unlock()
	if !stageCapture.on {
		return
	}
	t := stageCapture.taps[id]
	if t == nil {
		t = &StageTaps{}
		stageCapture.taps[id] = t
	}
	captureStage(&t.Voices, at, voices)
	captureStage(&t.Inserts, at, inserts)
	captureStage(&t.EQ, at, eq)
}

// The master is after the soft clip, what the device receives; every tap padded to totalSamples. DSP starts from silence.
func RenderStagesOffline(notes []StageNote, stops []StageStop, totalSamples int) ([]float64, map[instrument.ID]StageTaps) {
	ResetDSPState()
	m := newTestMixer()
	vid := uint64(0)
	for _, n := range notes {
		v := playedVoiceOrigin(n.Inst, n.Pitch, n.Dur, sampleRate, originPrewarm)
		if v == nil {
			continue
		}
		sv := &scaledVoice{v: v, gain: n.Vol}
		if n.Lane {
			vid++
			m.ScheduleLane(channel.Of(n.Inst), sv, n.At, -1, -1, vid)
		} else {
			m.Schedule(channel.Of(n.Inst), sv, n.At, -1, -1)
		}
	}
	for _, s := range stops {
		m.stopLaneAtSample(channel.Of(s.Inst), s.At)
	}
	stageCapture.Lock()
	stageCapture.on, stageCapture.taps = true, map[channel.ID]*StageTaps{}
	stageCapture.Unlock()
	stageCaptureOn.Store(true)
	master := renderMixer(m, totalSamples)
	stageCaptureOn.Store(false)
	stageCapture.Lock()
	stageCapture.on = false
	taps := stageCapture.taps
	stageCapture.taps = nil
	stageCapture.Unlock()
	out := make(map[instrument.ID]StageTaps, len(taps))
	pad := func(x []float64) []float64 {
		for len(x) < totalSamples {
			x = append(x, 0)
		}
		return x[:totalSamples]
	}
	for id, t := range taps {
		if inst, ok := id.Instrument(); ok {
			out[inst] = StageTaps{Voices: pad(t.Voices), Inserts: pad(t.Inserts), EQ: pad(t.EQ)}
		}
	}
	return master, out
}
