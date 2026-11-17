//go:build test

package ui

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// recSink records the lane's sink calls (vids, voice counts) on top of a
// fakeSink, so tests can check the per-cell voice-id blocks.
type recSink struct {
	*fakeSink
	schedVIDs []uint64
	replaces  [][3]uint64 // old, oldVoices, new
	cancelled []uint64
}

func (r *recSink) Schedule(vid uint64, n laneNote) {
	r.schedVIDs = append(r.schedVIDs, vid)
	r.fakeSink.Schedule(vid, n)
}
func (r *recSink) Replace(old uint64, oldVoices int, vid uint64, n laneNote) {
	r.replaces = append(r.replaces, [3]uint64{old, uint64(oldVoices), vid})
	r.fakeSink.Replace(old, oldVoices, vid, n)
}
func (r *recSink) Cancel(v []uint64) {
	r.cancelled = append(r.cancelled, v...)
	r.fakeSink.Cancel(v)
}

func triad() []model.Layer {
	return model.NodeLayers(model.NodeTypeChord, model.NodeParams{Chord: model.ChordSpec{Quality: "maj"}})
}

func newChordGridLane(t *testing.T, layers []model.Layer) (*audioLane, *gridSource, *recSink, *float64) {
	t.Helper()
	l, src, sink, now := newGridLane(t, 1)
	rec := &recSink{fakeSink: sink}
	l.sink = rec
	src.layers = [][]model.Layer{layers}
	return l, src, rec, now
}

// A chord cell is ONE lane entry whose voices own a contiguous block of
// voice ids: the next cell's block starts right after it.
func TestLaneChordCommitsOneBlockPerCell(t *testing.T) {
	l, _, rec, _ := newChordGridLane(t, triad())
	l.Tick()
	if len(rec.schedVIDs) < 2 {
		t.Fatalf("want several committed cells, got %v", rec.schedVIDs)
	}
	for i := 1; i < len(rec.schedVIDs); i++ {
		if d := rec.schedVIDs[i] - rec.schedVIDs[i-1]; d != 3 {
			t.Fatalf("vid blocks must be 3 apart for a triad, got %v", rec.schedVIDs)
		}
	}
}

// A regular note mints exactly one id per cell — the sequence is unchanged
// from before voice stacks existed.
func TestLaneSingleVoiceVIDsUnchanged(t *testing.T) {
	l, _, rec, _ := newChordGridLane(t, nil)
	l.Tick()
	for i, v := range rec.schedVIDs {
		if v != uint64(i+1) {
			t.Fatalf("single-voice vids must be 1,2,3…, got %v", rec.schedVIDs)
		}
	}
}

func TestLaneChordEditReplacesWholeChord(t *testing.T) {
	l, src, rec, now := newChordGridLane(t, triad())
	l.Tick()
	src.layers[0] = model.NodeLayers(model.NodeTypeChord, model.NodeParams{Chord: model.ChordSpec{Quality: "maj7"}})
	src.rowVol[0] = 0.9 // move the row key so the row reconciles
	*now += 0.004
	l.Tick()
	if len(rec.replaces) == 0 {
		t.Fatal("a chord edit must replace the committed chords")
	}
	for _, r := range rec.replaces {
		if r[1] != 3 {
			t.Fatalf("Replace must name the OLD chord's 3 voices, got %v", r)
		}
	}
	if next := rec.replaces[len(rec.replaces)-1][2]; next != rec.replaces[len(rec.replaces)-2][2]+4 {
		t.Fatalf("new 4-voice chords must take 4-id blocks: %v", rec.replaces)
	}
}

func TestLaneChordCancelCancelsEveryVoice(t *testing.T) {
	l, src, rec, now := newChordGridLane(t, triad())
	l.Tick()
	first := rec.schedVIDs[len(rec.schedVIDs)-1]
	src.rowMute[0] = true
	*now += 0.004
	l.Tick()
	l.flushCancels()
	want := map[uint64]bool{first: false, first + 1: false, first + 2: false}
	for _, v := range rec.cancelled {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for v, ok := range want {
		if !ok {
			t.Fatalf("voice %d of the last chord was not cancelled (cancelled %v)", v, rec.cancelled)
		}
	}
}

// flushLaneOps expands a chord note into one BatchParam per voice — same
// start, own voice id, pitch = root + layer semis, vol = vol × layer gain.
func TestFlushLaneOpsExpandsChordVoices(t *testing.T) {
	g := newLaneCircuitGame(t)
	var got []audio.BatchParam
	recordEngine(t, nil) // installs cancel hooks; the batch hook below replaces its own
	t.Cleanup(audio.SetPlayBatchHookForTest(func(b []audio.BatchParam) { got = append(got, b...) }))
	n := laneNote{Row: 0, Abs: 1, Kind: laneSound, Inst: roster.PianoGrand, Vol: 0.8, Pitch: 2, Dur: 1, Time: 2, Layers: triad()}
	g.laneSink.Schedule(10, n)
	g.flushLaneOps()
	if len(got) != 3 {
		t.Fatalf("want 3 voices, got %d", len(got))
	}
	g3 := 1 / math.Sqrt(3)
	for k, want := range []float64{2, 6, 9} {
		p := got[k]
		if p.VoiceID != uint64(10+k) || p.Pitch != want || math.Abs(p.Vol-0.8*g3) > 1e-12 || p.When != 2 || p.Row != 0 || p.Abs != 1 {
			t.Fatalf("voice %d = %+v", k, p)
		}
	}
}

// A single-voice note reaches the engine exactly as before (voice id, pitch,
// volume untouched — no gain multiply).
func TestFlushLaneOpsSingleVoiceUnchanged(t *testing.T) {
	g := newLaneCircuitGame(t)
	var got []audio.BatchParam
	recordEngine(t, nil) // installs cancel hooks; the batch hook below replaces its own
	t.Cleanup(audio.SetPlayBatchHookForTest(func(b []audio.BatchParam) { got = append(got, b...) }))
	g.laneSink.Schedule(7, laneNote{Row: 0, Abs: 1, Kind: laneSound, Inst: roster.Kick, Vol: 0.3, Pitch: -1, Dur: 1, Time: 2})
	g.flushLaneOps()
	if len(got) != 1 || got[0].VoiceID != 7 || got[0].Vol != 0.3 || got[0].Pitch != -1 {
		t.Fatalf("single voice changed: %+v", got)
	}
}

// Review Focus 2: replacing a chord is all-or-nothing. If the engine refuses
// ANY old voice (it started), none of the replacement voices play — never
// the old chord and the new one together. If it refuses none, all play.
func TestLaneChordReplaceRefusedDropsWholeChord(t *testing.T) {
	g := newLaneCircuitGame(t)
	log := recordEngine(t, map[uint64]bool{21: true}) // only the MIDDLE old voice started
	n := laneNote{Row: 0, Abs: 1, Kind: laneSound, Inst: roster.PianoGrand, Vol: 1, Dur: 1, Time: 2, Layers: triad()}
	g.laneSink.Replace(20, 3, 30, n) // refused: 21 started
	g.laneSink.Replace(40, 3, 50, n) // accepted
	g.flushLaneOps()
	got := strings.Join(*log, " | ")
	want := "cancel[20 21 22 40 41 42] | play[50,51,52]"
	if got != want {
		t.Fatalf("engine calls:\n got  %s\n want %s", got, want)
	}
}

// laneNoteAt carries the chord node's layers; the hash moves when the chord
// changes (or the lane would never re-commit an edited chord).
func TestLaneNoteAtCarriesChordLayers(t *testing.T) {
	g := newLaneCircuitGame(t)
	id := chordTestNodeAt(t, g, 2)
	c, ok := g.playbackSlate.laneContext()
	if !ok {
		t.Fatal("no lane context")
	}
	abs := 2
	before, ok := g.playbackSlate.laneNoteAt(&c, 0, abs)
	if !ok || len(before.Layers) != 3 {
		t.Fatalf("chord cell note = %+v (ok=%v), want 3 layers", before, ok)
	}
	n, _ := g.graph.GetNodeByID(id)
	p := n.Params
	p.Chord = model.ChordSpec{Quality: "maj7"}
	g.graph.SetNodeParams(id, p)
	g.notifyPredictorNode(id)
	c, _ = g.playbackSlate.laneContext()
	after, _ := g.playbackSlate.laneNoteAt(&c, 0, abs)
	if len(after.Layers) != 4 || after.Hash == before.Hash {
		t.Fatalf("edited chord: layers=%d hash moved=%v", len(after.Layers), after.Hash != before.Hash)
	}
}

// chordTestNodeAt turns the lane circuit's node at grid (i, 0) into a major
// chord and returns its id.
func chordTestNodeAt(t *testing.T, g *Game, i int) model.NodeID {
	t.Helper()
	ui := g.nodeAt(i, 0)
	if ui == nil {
		t.Fatalf("no node at (%d,0)", i)
	}
	n, _ := g.graph.GetNodeByID(ui.ID)
	n.Type = model.NodeTypeChord
	g.graph.Nodes[ui.ID] = n
	p := n.Params
	p.Chord = model.ChordSpec{Quality: "maj"}
	g.graph.SetNodeParams(ui.ID, p)
	g.notifyPredictorNode(ui.ID)
	g.updateBeatInfos()
	return ui.ID
}

// The legacy dispatch path (lane off) plays every voice of a chord, each
// with the cell's row/abs; a regular cell still dispatches exactly one.
func TestLegacyDispatchPlaysEveryChordVoice(t *testing.T) {
	g := newLaneCircuitGame(t)
	chordTestNodeAt(t, g, 2)
	pitches := map[[2]int][]float64{}
	captureLegacyDispatch(t, g, 16, func(row, abs int, inst string, vol, pitch float64) {
		pitches[[2]int{row, abs}] = append(pitches[[2]int{row, abs}], pitch)
	})
	chordCells, singles := 0, 0
	for key, ps := range pitches {
		switch len(ps) {
		case 3:
			chordCells++
			// node 2 has Pitch 3 in the lane circuit: root 3 → 3, 7, 10
			if ps[0] != 3 || ps[1] != 7 || ps[2] != 10 {
				t.Fatalf("cell %v chord pitches = %v, want [3 7 10]", key, ps)
			}
		case 1:
			singles++
		default:
			t.Fatalf("cell %v dispatched %d voices", key, len(ps))
		}
	}
	if chordCells == 0 || singles == 0 {
		t.Fatalf("vacuous: chord cells=%d single cells=%d", chordCells, singles)
	}
}
