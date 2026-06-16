//go:build !test && !js

package audio

import (
	"io"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// trigger is one scheduled audible hit: the predictor said row fires at absIdx,
// which maps deterministically to a sample offset via bpm/div.
type trigger struct {
	row, absIdx, sampleOffset int
}

// buildLoopCircuit builds a single-row loop of `n` adjacent regular nodes (all
// always-audible) and returns a ready predictor plus its loop length. Adjacent
// J positions mean one path cell per node — "fires every subdivision".
func buildLoopCircuit(t *testing.T, n int) (*engine.Predictor, int) {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	ids := make([]model.NodeID, n)
	for k := 0; k < n; k++ {
		ids[k] = graph.AddNode(0, k, model.NodeTypeRegular)
	}
	for k := 0; k < n; k++ {
		graph.Edges[[2]model.NodeID{ids[k], ids[(k+1)%n]}] = struct{}{}
	}
	graph.StartNodeID = ids[0]
	return finishPredictor(graph), len(idsToPath(graph))
}

// finishPredictor wires a graph into a predictor via the canonical path it uses
// in production (CalculateBeatRow → SetPaths → UpdateNode for every node).
func finishPredictor(graph *model.Graph) *engine.Predictor {
	pred := engine.NewPredictor(graph, nil)
	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	for id, node := range graph.Nodes {
		pred.UpdateNode(id, node)
	}
	return pred
}

func idsToPath(graph *model.Graph) []model.BeatInfo {
	path, _, _ := graph.CalculateBeatRow()
	return path
}

// buildComplexNodeRuleCircuit builds a single-row loop of many adjacent nodes
// carrying a mix of every logic kind (every_n_triggers, skip_every_n, fixed
// deterministic probability, trigger_if_prev_*). It drives ONE instrument, so
// the audio is trivial while the SCHEDULE is complex — the exact shape the user
// asked for. The predictor (deterministic, no RNG) owns which idx fire.
func buildComplexNodeRuleCircuit(t *testing.T) (*engine.Predictor, int, int) {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	const n = 12
	ids := make([]model.NodeID, n)
	for k := 0; k < n; k++ {
		ids[k] = graph.AddNode(0, k, model.NodeTypeRegular)
	}
	for k := 0; k < n; k++ {
		graph.Edges[[2]model.NodeID{ids[k], ids[(k+1)%n]}] = struct{}{}
	}
	graph.StartNodeID = ids[0]

	setLogic := func(idx int, kind string, ln int, lp float64) {
		node, ok := graph.GetNodeByID(ids[idx])
		if !ok {
			t.Fatalf("node %d missing", idx)
		}
		p := node.Params
		p.LogicKind = kind
		p.LogicN = ln
		p.LogicP = lp
		graph.SetNodeParams(ids[idx], p)
	}
	setLogic(2, "every_n_triggers", 2, 0)
	setLogic(4, "skip_every_n", 3, 0)
	setLogic(6, "probability", 0, 0.5) // deterministic hash roll, reproducible
	setLogic(8, "trigger_if_prev_triggered", 0, 0)
	setLogic(10, "trigger_if_prev_skipped", 0, 0)

	return finishPredictor(graph), 1, 4 // rows=1, div=4
}

// assertInterOnsetSilence checks the back half of every inter-onset gap is below
// `floor` — i.e. no spurious noise / denormals / leaked tails between hits.
func assertInterOnsetSilence(t *testing.T, x []float64, onsets []int, floor float64) {
	t.Helper()
	for i := 0; i+1 < len(onsets); i++ {
		a, b := onsets[i], onsets[i+1]
		start := a + (b-a)/2 // well past the ~8ms burst decay
		for j := start; j < b-32 && j < len(x); j++ {
			if math.Abs(x[j]) > floor {
				t.Fatalf("inter-onset noise at %d (between onsets %d..%d) = %g > %g", j, a, b, x[j], floor)
			}
		}
	}
}

// enumerateTriggers walks the predictor over [0, durSec) and returns every
// audible hit with its deterministic sample offset. time(idx) = (idx/div) *
// 60/bpm, matching game_sequencer_schedule.go's beatDtSec.
func enumerateTriggers(p *engine.Predictor, rows, div, bpm, sr int, durSec float64) []trigger {
	stepSec := 60.0 / float64(bpm) / float64(div)
	target := int(durSec/stepSec) + 1
	p.Ensure(target)
	var out []trigger
	for abs := 0; abs < target; abs++ {
		for r := 0; r < rows; r++ {
			if p.AudibleAt(r, abs) {
				off := int(float64(abs)*stepSec*float64(sr) + 0.5)
				out = append(out, trigger{r, abs, off})
			}
		}
	}
	return out
}

// renderCircuitOffline schedules `voiceFactory()` at each trigger's sample
// offset through the real desktop mixer and returns the captured master PCM.
// The buffer extends 50ms past the last trigger so the final burst completes.
func renderCircuitOffline(trigs []trigger, voiceFactory func() Voice, sr int) []float64 {
	ResetInstruments()
	m := newTestMixer()
	last := 0
	for _, tr := range trigs {
		m.Schedule("kick", voiceFactory(), tr.sampleOffset)
		if tr.sampleOffset > last {
			last = tr.sampleOffset
		}
	}
	return renderMixer(m, last+sr/20) // +50ms tail
}

// pinSampleRate fixes the package audio sample rate for a test so trigger
// offsets, the burst voice, the mixer and the capture all agree (and the golden
// hash is portable). Restores afterward.
func pinSampleRate(t *testing.T, sr int) {
	t.Helper()
	old := sampleRate
	sampleRate = sr
	t.Cleanup(func() { sampleRate = old })
}

// assertNoNaNOrClip fails if the master has any NaN/Inf or exceeds full scale.
func assertNoNaNOrClip(t *testing.T, x []float64) {
	t.Helper()
	for i, v := range x {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite sample at %d: %v", i, v)
		}
		if v > 1.0 || v < -1.0 {
			t.Fatalf("clipped sample at %d: %v", i, v)
		}
	}
}

// TestBurstVoiceLandsAtOffset: a deterministic burst scheduled at a known
// sample offset produces audible energy starting at (≈) that offset and is
// silent before it — the foundational guarantee the harness relies on.
func TestBurstVoiceLandsAtOffset(t *testing.T) {
	ResetInstruments()
	t.Cleanup(func() { ResetInstruments() })

	off := sampleRate / 100 // 10ms in
	m := newTestMixer()
	m.Schedule("kick", newBurstVoice(), off)
	got := renderMixer(m, sampleRate/20) // 50ms

	_, peak := argmaxAbs(got)
	if peak < 0.01 {
		t.Fatalf("burst not audible: peak=%g", peak)
	}

	onsets := detectOnsets(got, 1e-3)
	if len(onsets) != 1 {
		t.Fatalf("got %d onsets, want exactly 1: %v", len(onsets), onsets)
	}
	if onsets[0] < off || onsets[0] > off+64 {
		t.Fatalf("onset at sample %d, want in [%d,%d]", onsets[0], off, off+64)
	}
	for i := 0; i < off; i++ {
		if math.Abs(got[i]) > 1e-6 {
			t.Fatalf("pre-onset energy at %d = %g", i, got[i])
		}
	}
}

// TestEnumerateTriggersMapsAndIsDeterministic: the predictor is the ground
// truth for WHICH subdivisions fire (CLAUDE.md: "Predictor as source of
// truth"). The harness must (a) map each audible idx to the exact sample offset
// idx*stepSamples, (b) return a non-empty, strictly-increasing schedule, and
// (c) be byte-for-byte reproducible. We do NOT assert a hand-guessed count —
// the grid layout owns that; the predictor owns the schedule.
func TestEnumerateTriggersMapsAndIsDeterministic(t *testing.T) {
	pred, _ := buildLoopCircuit(t, 4)
	const div, bpm, sr = 4, 120, 48000
	trigs := enumerateTriggers(pred, 1, div, bpm, sr, 1.0)
	if len(trigs) == 0 {
		t.Fatal("no triggers enumerated")
	}
	step := float64(sr) * 60.0 / float64(bpm) / float64(div) // samples/subdivision
	for i, tr := range trigs {
		want := int(float64(tr.absIdx)*step + 0.5)
		if tr.sampleOffset != want {
			t.Fatalf("trigger %d idx=%d offset=%d, want %d", i, tr.absIdx, tr.sampleOffset, want)
		}
		if i > 0 && tr.sampleOffset <= trigs[i-1].sampleOffset {
			t.Fatalf("offsets not strictly increasing at %d: %d <= %d", i, tr.sampleOffset, trigs[i-1].sampleOffset)
		}
	}
	// Determinism: a fresh predictor for the same circuit yields the identical schedule.
	pred2, _ := buildLoopCircuit(t, 4)
	trigs2 := enumerateTriggers(pred2, 1, div, bpm, sr, 1.0)
	if len(trigs2) != len(trigs) {
		t.Fatalf("non-deterministic count: %d vs %d", len(trigs2), len(trigs))
	}
	for i := range trigs {
		if trigs[i] != trigs2[i] {
			t.Fatalf("non-deterministic trigger %d: %+v vs %+v", i, trigs[i], trigs2[i])
		}
	}
}

// TestRenderCircuitOnsetsMatchSchedule: the rendered master has EXACTLY one
// transient per predicted trigger, each aligned to the trigger's sample offset,
// with no NaN/clip. Dropped/doubled/mistimed voices fail this.
func TestRenderCircuitOnsetsMatchSchedule(t *testing.T) {
	pinSampleRate(t, 48000)
	pred, _ := buildLoopCircuit(t, 4)
	trigs := enumerateTriggers(pred, 1, 4, 120, sampleRate, 2.0)
	master := renderCircuitOffline(trigs, newBurstVoice, sampleRate)

	assertNoNaNOrClip(t, master)
	onsets := detectOnsets(master, 1e-3)
	if len(onsets) != len(trigs) {
		t.Fatalf("detected %d onsets, want %d (dropped/doubled trigger)", len(onsets), len(trigs))
	}
	for i := range trigs {
		d := onsets[i] - trigs[i].sampleOffset
		if d < 0 || d > 64 { // anti-pop fade ramps energy up a few samples after onset
			t.Fatalf("onset %d at %d, trigger at %d (delta %d, mistimed)", i, onsets[i], trigs[i].sampleOffset, d)
		}
	}
}

// TestComplexCircuitSimpleAudioByteExact is the headline deterministic harness:
// a complex node-rule schedule drives one simple instrument; the rendered
// master must match the predictor schedule exactly (no dropped/doubled/mistimed
// hits), be silent between hits (no noise/denormals), never clip or go NaN, and
// hash-pin so any future signal regression is caught byte-for-byte.
func TestComplexCircuitSimpleAudioByteExact(t *testing.T) {
	pinSampleRate(t, 48000)
	pred, rows, div := buildComplexNodeRuleCircuit(t)
	trigs := enumerateTriggers(pred, rows, div, 120, sampleRate, 4.0)
	if len(trigs) < 20 {
		t.Fatalf("complex circuit produced only %d triggers; want a busy schedule", len(trigs))
	}
	master := renderCircuitOffline(trigs, newBurstVoice, sampleRate)

	// (a) Structural: audio matches predictor exactly.
	assertNoNaNOrClip(t, master)
	onsets := detectOnsets(master, 1e-3)
	if len(onsets) != len(trigs) {
		t.Fatalf("onsets=%d triggers=%d (audio diverges from schedule)", len(onsets), len(trigs))
	}
	for i := range trigs {
		d := onsets[i] - trigs[i].sampleOffset
		if d < 0 || d > 64 {
			t.Fatalf("onset %d at %d, trigger at %d (delta %d, mistimed)", i, onsets[i], trigs[i].sampleOffset, d)
		}
	}
	// (b) Silence between hits: no spurious noise / denormal leakage.
	assertInterOnsetSilence(t, master, onsets, 1e-4)

	// (c) Byte-exact golden — any signal regression flips the hash.
	got := hashFloat64PCM(master)
	t.Logf("master sha256=%s samples=%d triggers=%d onsets=%d", got, len(master), len(trigs), len(onsets))
	const golden = "e89d9fc235a5376a2385cf992cbdc9430355a682cb0ff91ad66e50e1849964f1"
	if golden != "" && got != golden {
		t.Fatalf("master hash drift: got %s want %s", got, golden)
	}
}
