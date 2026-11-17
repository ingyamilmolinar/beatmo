//go:build test

package ui

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

var lastLaneEditName string

// applyRandomLaneEdit performs one random edit through the SAME entry point
// the UI (or JS bridge) uses, followed by whatever reconciliation that
// entry point's caller performs. Deliberately includes edits that bypass
// every invalidation funnel (row volume / instrument written directly).
func applyRandomLaneEdit(t *testing.T, g *Game, rng *rand.Rand) {
	t.Helper()
	var regular []model.NodeID
	for id, n := range g.graph.Nodes {
		if n.Type.Fires() && id != g.graph.StartNodeID {
			regular = append(regular, id)
		}
	}
	sort.Slice(regular, func(i, j int) bool { return regular[i] < regular[j] }) // deterministic under the seed
	pick := func() (model.NodeID, bool) {
		if len(regular) == 0 {
			return 0, false
		}
		return regular[rng.Intn(len(regular))], true
	}
	inst := g.drum.Rows[0].Instrument
	switch k := rng.Intn(15); k {
	case 0, 1, 2: // node volume / pitch / logic
		id, ok := pick()
		if !ok {
			return
		}
		p := g.graph.Nodes[id].Params
		switch k {
		case 0:
			p.Volume = []float64{0.4, 0.8, 1.2}[rng.Intn(3)]
			lastLaneEditName = "node-volume"
		case 1:
			p.Pitch = float64(rng.Intn(7) - 3)
			lastLaneEditName = "node-pitch"
		case 2:
			if p.LogicKind == model.LogicNone {
				p.LogicKind, p.LogicP = model.LogicProbability, 0.3
			} else {
				p.LogicKind, p.LogicP = model.LogicNone, 0
			}
			lastLaneEditName = "node-logic"
		}
		g.graph.SetNodeParams(id, p)
	case 3: // node type → mute (sidebar path: direct write + notify + paths)
		id, ok := pick()
		if !ok {
			return
		}
		m := g.graph.Nodes[id]
		m.Type = model.NodeTypeMute
		g.graph.Nodes[id] = m
		g.notifyPredictorNode(id)
		g.updateBeatInfos()
		lastLaneEditName = "node-type"
	case 4: // move a node off the loop and back (path change)
		id, ok := pick()
		if !ok {
			return
		}
		if n := g.nodeByID(id); n != nil {
			g.moveNode(n, n.I, n.J+2)
			g.updateBeatInfos()
		}
		lastLaneEditName = "node-move"
	case 5: // delete a node (breaks the loop)
		id, ok := pick()
		if !ok {
			return
		}
		if n := g.nodeByID(id); n != nil {
			g.deleteNodeInternal(n, true)
		}
		lastLaneEditName = "node-delete"
	case 6:
		g.drum.Rows[0].Volume = []float64{0.3, 0.6, 1.0}[rng.Intn(3)]
		lastLaneEditName = "row-volume (direct write, no funnel)"
	case 7:
		g.drum.toggleMute(0)
		lastLaneEditName = "row-mute"
	case 8:
		g.drum.toggleSolo(0)
		lastLaneEditName = "row-solo"
	case 9: // BPM change as the sequencer applies it (re-anchor + applied BPM)
		prev := g.state.AppliedBPM()
		if prev <= 0 {
			prev = g.drum.BPM()
		}
		b := 90 + rng.Intn(90)
		g.drum.SetBPM(b)
		g.state.AnchorForBPMChange(prev, time.Now(), audio.Now())
		g.state.SetAppliedBPM(b)
		lastLaneEditName = "bpm"
	case 10:
		g.Seek(rng.Intn(16))
		lastLaneEditName = "seek"
	case 11:
		audio.ApplySampleEditLive(inst, audio.SampleEdit{StartFrac: 0, EndFrac: 1, GainDB: float32(-rng.Intn(12))})
		lastLaneEditName = "sample-edit"
	case 12:
		g.drum.Rows[0].Instrument = []instrument.ID{roster.Snare, roster.Kick, roster.Clap}[rng.Intn(3)]
		lastLaneEditName = "instrument (direct write, no funnel)"
	case 13: // node type regular ↔ chord (sidebar path: direct write + notify + paths)
		id, ok := pick()
		if !ok {
			return
		}
		m := g.graph.Nodes[id]
		if m.Type == model.NodeTypeChord {
			m.Type = model.NodeTypeRegular
		} else {
			m.Type = model.NodeTypeChord
		}
		g.graph.Nodes[id] = m
		g.notifyPredictorNode(id)
		g.updateBeatInfos()
		lastLaneEditName = "node-type chord"
	case 14: // chord quality / inversion / spread (the chord builder's edits)
		id, ok := pick()
		if !ok {
			return
		}
		p := g.graph.Nodes[id].Params
		p.Chord = model.NormalizeChord(model.ChordSpec{
			Quality:   []string{"maj", "m7", "9", "sus4"}[rng.Intn(4)],
			Inversion: rng.Intn(4),
			Spread:    rng.Intn(model.ChordSpreadCount),
		})
		g.graph.SetNodeParams(id, p)
		g.notifyPredictorNode(id)
		lastLaneEditName = "node-chord-spec"
	}
}

func equalU64(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalNoteMaps(a, b map[[2]int]uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// The lane only reconciles a row when its fingerprint (or the global one)
// moves. This property test proves the fingerprints never miss a change that
// alters what would sound: for random edits from every mutation class —
// including ones that bypass every invalidation funnel — if no key moved,
// the notes over the next 2 s must be identical.
func TestLaneKeysCoverEveryEditClass(t *testing.T) {
	var g *Game
	var src *gameLaneSource
	fresh := func() {
		// A fresh circuit every 25 edits: cumulative deletes / mutes would
		// otherwise degrade it to silence and make the property vacuous.
		g = newLaneCircuitGame(t)
		inst := g.drum.Rows[0].Instrument
		t.Cleanup(func() { audio.ClearSampleEdit(inst) })
		src = g.playbackSlate.newGameLaneSource()
	}
	rng := rand.New(rand.NewSource(42))
	snapshot := func() (map[[2]int]uint64, uint64, []uint64) {
		src.refresh()
		notes := map[[2]int]uint64{}
		if lo, hi, ok := src.Steps(src.Now(), src.Now()+2); ok {
			for r := 0; r < src.Rows(); r++ {
				for a := lo; a <= hi; a++ {
					if n, ok := src.NoteAt(r, a); ok {
						notes[[2]int{r, a}] = n.Hash
					}
				}
			}
		}
		keys := make([]uint64, src.Rows())
		for r := range keys {
			keys[r] = src.RowKey(r)
		}
		return notes, src.GlobalKey(), keys
	}
	compared, changed := 0, 0
	for i := 0; i < 400; i++ {
		if i%25 == 0 {
			fresh()
		}
		n0, g0, k0 := snapshot()
		applyRandomLaneEdit(t, g, rng)
		n1, g1, k1 := snapshot()
		compared += len(n0)
		if !equalNoteMaps(n0, n1) {
			changed++
		}
		if g0 == g1 && equalU64(k0, k1) && !equalNoteMaps(n0, n1) {
			t.Fatalf("edit #%d (%s) changed the notes but no lane key moved", i, lastLaneEditName)
		}
	}
	// Non-vacuity: the property only means something if the windows held
	// notes and the edits really changed them.
	if compared < 400 || changed < 40 {
		t.Fatalf("property test is vacuous: %d notes compared, %d edits changed notes", compared, changed)
	}
}

// Rows are keyed by INDEX: deleting a row shifts the next one into its
// index. Every committed future note of the deleted row must go, and every
// future note left must belong to the row now at its index.
func TestLaneRowDeleteShiftsCommittedNotes(t *testing.T) {
	g := newLaneCircuitGame(t)
	g.drum.AddRow()
	g.drum.Rows[1].Instrument = roster.Clap // distinguishable from row 0's instrument
	g.updateBeatInfos()
	now := laneTestNow(g)
	sink := newFakeSink(now)
	lane := enableAudioLaneForTest(t, g, sink)
	g.laneSrc.refresh()
	lane.Tick()
	deleted := g.drum.Rows[0].Instrument
	before := 0
	for _, n := range sink.sched {
		if n.Row == 0 && n.Inst == deleted && n.Time > *now+0.008 {
			before++
		}
	}
	if before == 0 {
		t.Fatal("setup: row 0 committed no future notes")
	}
	g.drum.DeleteRow(0)
	g.updateBeatInfos()
	g.laneSrc.refresh()
	lane.Tick()
	for vid, n := range sink.sched {
		if n.Time <= *now+0.008 {
			continue
		}
		if n.Inst == deleted {
			t.Fatalf("vid %d: a future note of the DELETED row survived: %+v", vid, n)
		}
		if n.Row >= len(g.drum.Rows) || n.Inst != g.drum.Rows[n.Row].Instrument {
			t.Fatalf("vid %d: future note does not match the row now at index %d: %+v", vid, n.Row, n)
		}
	}
}

// enableAudioLaneForTest installs an audio lane wired to the Game's lane
// source and the given sink, with the lane switched on for this test only.
func enableAudioLaneForTest(t *testing.T, g *Game, sink audioSink) *audioLane {
	t.Helper()
	p := RuntimeProf()
	if g.laneSrc == nil {
		g.laneSrc = g.playbackSlate.newGameLaneSource()
	}
	g.audioLane = newAudioLane(g.laneSrc, sink, laneConfig{Horizon: p.AudioHorizonSec, Guard: p.CancelGuardSec,
		LateTol: p.LateToleranceSec, FullReconcile: p.LaneFullReconcileSec, VoiceRecommitMin: p.LaneVoiceRecommitMinSec,
		VoiceNear: p.LaneVoiceNearSec})
	g.laneOn.Store(true)
	t.Cleanup(func() { g.laneOn.Store(false) })
	return g.audioLane
}

var _ = fmt.Sprintf
