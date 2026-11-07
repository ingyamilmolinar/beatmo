//go:build !test

package ui

// Real build only. Renders the live engine state through the offline mixer with nothing re-applied, and reads the
// notes from the live predictor, not the file, so a schedule regression cannot hide behind a file that round-trips
// its own bytes.

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Several bars at 120 BPM plus the earliest hits' tails.
const masterRenderSeconds = 4.0

// The documents are process-global, so an edit or Save As changes every later test's start.
func restoreLibraryCopies(t *testing.T, ids ...instrument.ID) {
	t.Helper()
	for _, id := range ids {
		audio.ResetInstrumentParams(id)
	}
}

// From the live predictor, mirroring songrender.BuildArrangement's semantics (AudibleAt gate, loop folding,
// duration/volume), which reads a parsed template instead of a Game.
func liveArrangement(t *testing.T, g *Game, seconds float64) ([]audio.SongRenderNote, int) {
	t.Helper()
	sr := audio.SampleRate()
	bpm := g.drum.BPM()
	if bpm <= 0 {
		bpm = 120
	}
	subdiv := currentMaxDiv()
	if subdiv <= 0 {
		subdiv = 32
	}
	// One grid unit: 60/(BPM·MaxDiv) s, the sequencer's clock.
	secPerStep := 60.0 / float64(bpm) / float64(subdiv)
	samplesPerStep := secPerStep * float64(sr)
	totalSamples := int(math.Round(seconds * float64(sr)))
	totalSteps := int(math.Ceil(seconds / secPerStep))

	pred := g.engine.Predictor
	if pred == nil {
		t.Fatal("live arrangement: engine predictor is nil")
	}
	pred.Ensure(totalSteps)

	nodes := make(map[model.NodeID]model.Node, len(g.graph.Nodes))
	for id, n := range g.graph.Nodes {
		nodes[id] = n
	}

	var notes []audio.SongRenderNote
	for row := 0; row < len(g.beatInfosByRow) && row < len(g.drum.Rows); row++ {
		inst := g.drum.Rows[row].Instrument
		if inst == "" {
			continue
		}
		rowVol := g.drum.Rows[row].Volume
		for abs := 0; abs < totalSteps; abs++ {
			if !pred.AudibleAt(row, abs) {
				continue
			}
			bi, ok := liveBeatInfoAt(g.beatInfosByRow[row], g.isLoopByRow[row], g.loopStartByRow[row], abs)
			if !ok {
				continue
			}
			node := nodes[bi.NodeID]
			dur := node.Params.Duration
			if dur <= 0 {
				dur = 1
			}
			dur *= audio.ConfigForInstrument(inst).DurationSec
			vol := node.Params.Volume
			if vol <= 0 {
				vol = 1
			}
			vol *= rowVol
			// The node's own groove, exactly as nodeNudgeSecondsAt applies it.
			nudge := 0.0
			switch node.Params.GrooveKind {
			case model.GrooveDelay:
				nudge = math.Min(1, math.Max(0, node.Params.GroovePct)) * secPerStep
			case model.GrooveRush:
				nudge = -math.Min(1, math.Max(0, node.Params.GroovePct)) * secPerStep
			}
			note := audio.SongRenderNote{
				InstID:   inst,
				AtSample: int(math.Max(0, math.Round(float64(abs)*samplesPerStep+nudge*float64(sr)))),
				Pitch:    node.Params.Pitch,
				Gain:     vol,
				DurSec:   dur,
			}
			// A chord sounds its whole voice stack, as dispatch and songrender do.
			layers := model.NodeLayers(node.Type, node.Params)
			if layers == nil {
				notes = append(notes, note)
			}
			for _, l := range layers {
				v := note
				v.Pitch += l.Semis
				v.Gain *= l.Gain
				notes = append(notes, v)
			}
		}
	}
	return notes, totalSamples
}

// The predictor's own index folding.
func liveBeatInfoAt(path []model.BeatInfo, isLoop bool, loopStart, abs int) (model.BeatInfo, bool) {
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
	return path[loopStart+((abs-loopStart)%loopLen)], true
}

// Through the real offline mixer and master chain.
func renderLiveMasterMix(t *testing.T, g *Game, seconds float64) []float64 {
	t.Helper()
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	notes, totalSamples := liveArrangement(t, g, seconds)
	return audio.RenderMasterMixOffline(notes, audio.SampleRate(), totalSamples)
}

// n == 0 means byte-identical.
func pcmIdentical(a, b []float64) (firstIdx int, firstDelta float64, n int) {
	firstIdx = -1
	if len(a) != len(b) {
		return 0, math.NaN(), max(len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			if firstIdx < 0 {
				firstIdx, firstDelta = i, b[i]-a[i]
			}
			n++
		}
	}
	return firstIdx, firstDelta, n
}

// Names where and how loudly they diverge.
func assertMasterPCMIdentical(t *testing.T, want, got []float64, what string) {
	t.Helper()
	if len(want) != len(got) {
		t.Errorf("%s: master PCM LENGTH %d -> %d", what, len(want), len(got))
		return
	}
	idx, delta, n := pcmIdentical(want, got)
	if n == 0 {
		return
	}
	var peak float64
	for i := range want {
		if d := math.Abs(got[i] - want[i]); d > peak {
			peak = d
		}
	}
	t.Errorf("%s: master PCM DIFFERS in %d/%d samples (%.3f%%); first at sample %d (%.1f ms) delta=%g, peak delta=%g",
		what, n, len(want), 100*float64(n)/float64(len(want)),
		idx, 1000*float64(idx)/float64(audio.SampleRate()), delta, peak)
}
