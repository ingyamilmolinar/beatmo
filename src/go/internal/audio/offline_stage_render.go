//go:build !test && !js

package audio

import (
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Cross-platform stage renders (cmd/export_audio --stages,
// xplat_audio_stages.browser.test.js): the real offline mixer, against the
// LIVE engine configuration like RenderMasterMixOffline, with every stage of
// every channel captured so the browser's graph can be compared tap by tap.

// StageNote is one note of a stage render: Dur is the node's Duration factor
// (playedVoice's dur), Vol its volume (row × node). A Lane note is scheduled
// as an audio-lane voice (vid > 0), so StageStops can fade it.
type StageNote struct {
	Inst  instrument.ID
	At    int
	Pitch float64
	Vol   float64
	Dur   float64
	Lane  bool
}

// StageStop is a lane stop (a mute node): at sample At, fade the lane voices of
// Inst that started before it (mixer.stopLaneAtSample).
type StageStop struct {
	Inst instrument.ID
	At   int
}

// StageTaps is one channel's signal at each stage boundary, sample-aligned
// from the render's first sample: Voices = its voices summed (× headroom, the
// mixer's per-instrument buffer), Inserts = after its insert chain, EQ = after
// its EQ (what it adds to the master bus).
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

// captureStage appends block (starting at absolute sample at) to *dst, padding
// the silence since the channel was last active.
func captureStage(dst *[]float64, at int, block []float64) {
	for len(*dst) < at {
		*dst = append(*dst, 0)
	}
	*dst = append(*dst, block...)
}

// stageCaptureBlock records one channel's three taps for the block at `at`.
// Called by processBlock (Phase 2) only while a stage render runs.
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

// RenderStagesOffline renders notes (and lane stops) for totalSamples through
// the offline mixer at the engine rate and returns the master output (after
// the soft clip — what the device receives) and every channel's stage taps,
// each padded to totalSamples. DSP state starts from silence.
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
