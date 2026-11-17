//go:build !test

package ui

// Deterministic master-mix harness for the export → import audio-identity
// tests (export_import_master_audio_identity_test.go).
//
// WHY A REAL-BUILD FILE. Under `-tags test` the synth renders through a stub
// and NewEQProcessor returns a pass-through gain (see internal/audio/stub.go),
// so a "byte-identical audio" assertion there proves nothing about the shipped
// sound. These tests therefore run only in the real build — `make test-real`.
//
// WHAT IT RENDERS. renderLiveMasterMix drives the SAME offline mixer the
// songrender path uses (audio.RenderMasterMixOffline), but against whatever the
// engine currently holds: channel volume/pan, insert-effect chains, per-channel
// EQ, delay/reverb sends, master EQ, compressor and limiter. Nothing is
// re-applied from a spec first, so the render is a faithful readout of live
// engine state — which is exactly the thing an export must be able to
// reconstruct.
//
// WHERE THE NOTES COME FROM. liveArrangement walks the LIVE engine predictor
// (g.engine.Predictor, the same buffers the sequencer schedules from), not the
// project file. Both halves of a round-trip therefore derive their schedule
// independently of the JSON, so a note-schedule regression cannot hide behind a
// file that round-trips its own bytes.

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// masterRenderSeconds is the default audio window compared by the identity
// tests. Long enough to cover several bars at 120 BPM plus the reverb/delay
// tail of the earliest hits, short enough to keep the suite quick.
const masterRenderSeconds = 4.0

// restoreLibraryCopies returns each instrument's project document to its
// library copy. The documents are PROCESS-GLOBAL, so a test that edits or
// Saves-As an instrument silently changes the starting state of every later
// test in the package. Call it when setting up.
func restoreLibraryCopies(t *testing.T, ids ...instrument.ID) {
	t.Helper()
	for _, id := range ids {
		audio.ResetInstrumentParams(id)
	}
}

// liveArrangement returns the note schedule the sequencer would emit for the
// first `seconds` of playback, read from the live engine predictor.
//
// It mirrors songrender.BuildArrangement's semantics deliberately — same
// AudibleAt gate, same loop folding, same duration/volume composition — so the
// offline render lines up with what songrender produces for the same circuit.
// It is a separate implementation only because BuildArrangement consumes a
// parsed template while this must read the live Game.
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
	// One step is one grid unit: 60/(BPM*MaxDiv) seconds — the same clock as
	// songrender.StepSeconds and the sequencer. (This read the subdivision as
	// units per BAR until 2026-10-02, so "the first N seconds" covered a
	// quarter of the music it claimed to.)
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
			// A chord node sounds its whole voice stack (model.NodeLayers), as
			// the lane / legacy dispatch and songrender do.
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

// liveBeatInfoAt folds an absolute step into a row's path, honoring the row's
// loop — the predictor's own index folding, mirrored so the schedule matches.
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

// renderLiveMasterMix renders `seconds` of the game's circuit through the real
// offline mixer and master chain, returning the captured master PCM.
func renderLiveMasterMix(t *testing.T, g *Game, seconds float64) []float64 {
	t.Helper()
	game_log.TailOnFailure(t) // the capture lines, only if this test fails
	notes, totalSamples := liveArrangement(t, g, seconds)
	return audio.RenderMasterMixOffline(notes, audio.SampleRate(), totalSamples)
}

// pcmIdentical reports the index and magnitude of the first mismatch, plus the
// total number of differing samples. n == 0 means byte-identical.
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

// assertMasterPCMIdentical fails with a diagnostic that names where the two
// renders diverge and how loud the divergence is — "differs" alone gives a
// reader nothing to work with.
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
