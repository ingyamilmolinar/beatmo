package ui

import (
	"strconv"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestMuteNodeStopsAudioWithoutGating(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated: %+v", g.beatInfosByRow)
	}
	muteIdx := -1
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeID == mute.ID {
			muteIdx = i
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute index not found")
	}

	g.playing = true
	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	inst := g.drum.Rows[0].Instrument
	stops := 0
	audio.SetStopHook(func(id string) {
		if id == inst {
			stops++
		}
	})
	defer audio.SetStopHook(nil)

	g.seqScheduleBeat()
	if plays != 1 || stops != 0 {
		t.Fatalf("after regular beat: plays=%d stops=%d", plays, stops)
	}

	g.seqScheduleBeat()
	if stops != 1 {
		t.Fatalf("mute beat did not stop audio: stops=%d", stops)
	}
	if plays != 1 {
		t.Fatalf("mute beat should not play: plays=%d", plays)
	}

	// Next beat should resume playback immediately (no gate hold).
	g.seqScheduleBeat()
	if plays != 2 {
		t.Fatalf("tail beat should play after mute: plays=%d", plays)
	}

	if len(g.muteUntilByRow) == 0 || g.muteUntilByRow[0] > muteIdx+1 {
		t.Fatalf("unexpected mute gate extension: %v", g.muteUntilByRow)
	}

	g.ensurePredictions(muteIdx + 2)
	if g.engine != nil && g.engine.Predictor != nil {
		if !g.engine.Predictor.AudibleAt(0, muteIdx+1) {
			t.Fatalf("predictor muted following beat")
		}
	} else {
		if row := 0; row < len(g.predAudibleByRow) && muteIdx+1 < len(g.predAudibleByRow[row]) {
			if !g.predAudibleByRow[row][muteIdx+1] {
				t.Fatalf("legacy predictions muted following beat")
			}
		}
	}

	g.playing = false
	g.engine.Stop()
}

func TestMuteNodeHighlightRespectsLogic(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		n.Params.LogicKind = "every_n_triggers"
		n.Params.LogicN = 2
		g.graph.Nodes[mute.ID] = n
	}
	g.updateBeatInfos()

	g.playing = true
	g.SetPlayFunc(func(string, float64, ...float64) {})

	muteIdx := -1
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeID == mute.ID {
			muteIdx = i
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute index not found")
	}

	states := make([]bool, 0, 4)
	if len(g.seqNextIdxs) == 0 {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	steps := len(g.beatInfosByRow[0]) * 6
	for step := 0; step < steps && len(states) < 4; step++ {
		idx := g.seqNextIdxs[0]
		info := g.beatInfoAtRow(0, idx)
		g.seqScheduleBeat()
		select {
		case ev := <-g.hlCh:
			beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
			g.highlightBeat(ev.row, ev.idx, ev.info, beatDuration)
		default:
		}
		if info.NodeID == mute.ID {
			key := makeBeatKey(0, idx)
			_, highlighted := g.highlightedBeats[key]
			states = append(states, highlighted)
			delete(g.highlightedBeats, key)
		}
	}
	if len(states) < 4 {
		t.Fatalf("insufficient mute samples: %v", states)
	}
	expected := []bool{false, true, false, true}
	for i := range expected {
		if states[i] != expected[i] {
			t.Fatalf("mute highlight pattern mismatch got=%v want=%v", states, expected)
		}
	}
	g.playing = false
	g.engine.Stop()
}

func TestMuteNodeLogicEveryNTriggers(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		n.Params.LogicKind = "every_n_triggers"
		n.Params.LogicN = 2
		g.graph.Nodes[mute.ID] = n
	}
	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated: %+v", g.beatInfosByRow)
	}

	g.playing = true
	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	cycleLen := len(g.beatInfosByRow[0])
	if len(g.seqNextIdxs) == 0 {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	is := func(id model.NodeID) int {
		for i, bi := range g.beatInfosByRow[0] {
			if bi.NodeID == id {
				return i
			}
		}
		return -1
	}
	muteIdx := is(mute.ID)
	tailIdx := is(tail.ID)
	if muteIdx < 0 || tailIdx < 0 {
		t.Fatalf("missing mute or tail index: mute=%d tail=%d", muteIdx, tailIdx)
	}

	muteTriggers := make([]bool, 0, 4)
	steps := cycleLen * 6
	for step := 0; step < steps && len(muteTriggers) < 4; step++ {
		idx := g.seqNextIdxs[0]
		info := g.beatInfoAtRow(0, idx)
		g.seqScheduleBeat()
		select {
		case ev := <-g.hlCh:
			beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
			g.highlightBeat(ev.row, ev.idx, ev.info, beatDuration)
		default:
		}
		if info.NodeID == mute.ID {
			state := false
			if m, ok := g.lastTriggeredByRow[0]; ok {
				state = m[mute.ID]
			}
			muteTriggers = append(muteTriggers, state)
			key := makeBeatKey(0, idx)
			_, highlighted := g.highlightedBeats[key]
			if state && !highlighted {
				t.Fatalf("mute should highlight when triggered at idx %d", idx)
			}
			if !state && highlighted {
				t.Fatalf("mute highlight persisted when logic disabled at idx %d", idx)
			}
		}
	}
	if len(muteTriggers) < 4 {
		t.Fatalf("insufficient mute samples: %v", muteTriggers)
	}
	expected := []bool{false, true, false, true}
	for i := range expected {
		if muteTriggers[i] != expected[i] {
			t.Fatalf("mute trigger pattern mismatch got=%v want=%v", muteTriggers, expected)
		}
	}

	if g.nodeAnim[mute.ID] <= 0 {
		t.Fatalf("mute node animation not triggered")
	}
	g.playing = false
	g.engine.Stop()
}

func TestMuteNodeHighlight(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	g.updateBeatInfos()

	g.playing = true
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Advance one full cycle so the mute node fires once.
	cycleLen := len(g.beatInfosByRow[0])
	for i := 0; i < cycleLen; i++ {
		g.seqScheduleBeat()
	}
	// Drain highlight event for the mute node and apply.
	beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
	processed := false
	for {
		select {
		case ev := <-g.hlCh:
			g.highlightBeat(ev.row, ev.idx, ev.info, beatDuration)
			if ev.info.NodeID == mute.ID {
				processed = true
			}
		default:
			goto done
		}
	}
done:
	if !processed {
		t.Fatalf("mute highlight event not received")
	}
	if g.nodeAnim[mute.ID] <= 0 {
		t.Fatalf("mute node animation not activated")
	}
	muteIdx := -1
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeID == mute.ID {
			muteIdx = i
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute index not found")
	}
	key := makeBeatKey(0, muteIdx)
	if _, ok := g.highlightedBeats[key]; !ok {
		t.Fatalf("mute beat not highlighted; highlighted=%v", g.highlightedBeats)
	}
	if !isMuteHighlight(g.highlightedBeats[key]) {
		t.Fatalf("highlight should be tagged as mute: %v", g.highlightedBeats[key])
	}
}

type muteObservation struct {
	triggered bool
	highlight bool
	stopDelta int
}

func setupMuteLoopGame(t *testing.T, logicKind string, logicN int) (*Game, model.NodeID) {
	t.Helper()
	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	g.addEdge(start, mute)
	g.addEdge(mute, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}

	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		n.Params.LogicKind = logicKind
		n.Params.LogicN = logicN
		g.graph.SetNodeParams(mute.ID, n.Params)
	}

	g.updateBeatInfos()
	g.playing = true
	g.SetPlayFunc(func(string, float64, ...float64) {})

	return g, mute.ID
}

func collectMuteObservations(t *testing.T, g *Game, row int, muteID model.NodeID, want int, stops func() int) []muteObservation {
	t.Helper()
	if row < 0 || row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
		t.Fatalf("invalid beat infos for row %d", row)
	}
	if len(g.seqNextIdxs) == 0 {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
	observations := make([]muteObservation, 0, want)
	prevStops := stops()
	maxSteps := len(g.beatInfosByRow[row])*want*3 + want
	for step := 0; len(observations) < want && step < maxSteps; step++ {
		idx := g.seqNextIdxs[row]
		info := g.beatInfoAtRow(row, idx)
		g.seqScheduleBeat()
	drain:
		for {
			select {
			case ev := <-g.hlCh:
				g.highlightBeat(ev.row, ev.idx, ev.info, beatDuration)
			default:
				break drain
			}
		}

		if info.NodeID != muteID {
			continue
		}
		triggered := false
		if m, ok := g.lastTriggeredByRow[row]; ok {
			triggered = m[muteID]
		}
		g.highlightBeat(row, idx, info, beatDuration)
		key := makeBeatKey(row, idx)
		val, ok := g.highlightedBeats[key]
		highlight := ok && isMuteHighlight(val)
		currStops := stops()
		observations = append(observations, muteObservation{
			triggered: triggered,
			highlight: highlight,
			stopDelta: currStops - prevStops,
		})
		prevStops = currStops
	}
	if len(observations) != want {
		t.Fatalf("insufficient mute samples: got %d want %d", len(observations), want)
	}
	return observations
}

func TestMuteNodeSkipEveryNLoopAudioHighlight(t *testing.T) {
	cases := []struct {
		n    int
		want []bool
	}{
		{n: 2, want: []bool{true, false, true, false}},
		{n: 3, want: []bool{true, true, false, true, true, false}},
		{n: 4, want: []bool{true, true, true, false}},
	}
	for _, tc := range cases {
		t.Run("N="+strconv.Itoa(tc.n), func(t *testing.T) {
			g, muteID := setupMuteLoopGame(t, "skip_every_n", tc.n)
			defer func() {
				g.playing = false
				if g.engine != nil {
					g.engine.Stop()
				}
			}()

			inst := g.drum.Rows[0].Instrument
			stops := 0
			audio.SetStopHook(func(id string) {
				if id == inst {
					stops++
				}
			})
			defer audio.SetStopHook(nil)

			obs := collectMuteObservations(t, g, 0, muteID, len(tc.want), func() int { return stops })
			expectedStops := 0
			for i, wantTrig := range tc.want {
				if obs[i].triggered != wantTrig {
					t.Fatalf("trigger mismatch at %d: got=%v want=%v", i, obs[i].triggered, wantTrig)
				}
				if obs[i].highlight != wantTrig {
					t.Fatalf("highlight mismatch at %d: got=%v want=%v", i, obs[i].highlight, wantTrig)
				}
				if wantTrig {
					expectedStops++
					if obs[i].stopDelta != 1 {
						t.Fatalf("stop delta mismatch at %d: got=%d want=1", i, obs[i].stopDelta)
					}
				} else if obs[i].stopDelta != 0 {
					t.Fatalf("expected no stop at %d, got delta=%d", i, obs[i].stopDelta)
				}
			}
			if stops != expectedStops {
				t.Fatalf("total stop count mismatch: got=%d want=%d", stops, expectedStops)
			}
		})
	}
}

func TestMuteNodeEveryNLoopAudioHighlight(t *testing.T) {
	cases := []struct {
		n    int
		want []bool
	}{
		{n: 2, want: []bool{false, true, false, true}},
		{n: 3, want: []bool{false, false, true, false, false, true}},
		{n: 4, want: []bool{false, false, false, true}},
	}
	for _, tc := range cases {
		t.Run("N="+strconv.Itoa(tc.n), func(t *testing.T) {
			g, muteID := setupMuteLoopGame(t, "every_n_triggers", tc.n)
			defer func() {
				g.playing = false
				if g.engine != nil {
					g.engine.Stop()
				}
			}()

			inst := g.drum.Rows[0].Instrument
			stops := 0
			audio.SetStopHook(func(id string) {
				if id == inst {
					stops++
				}
			})
			defer audio.SetStopHook(nil)

			obs := collectMuteObservations(t, g, 0, muteID, len(tc.want), func() int { return stops })
			expectedStops := 0
			for i, wantTrig := range tc.want {
				if obs[i].triggered != wantTrig {
					t.Fatalf("trigger mismatch at %d: got=%v want=%v", i, obs[i].triggered, wantTrig)
				}
				if obs[i].highlight != wantTrig {
					t.Fatalf("highlight mismatch at %d: got=%v want=%v", i, obs[i].highlight, wantTrig)
				}
				if wantTrig {
					expectedStops++
					if obs[i].stopDelta != 1 {
						t.Fatalf("stop delta mismatch at %d: got=%d want=1", i, obs[i].stopDelta)
					}
				} else if obs[i].stopDelta != 0 {
					t.Fatalf("expected no stop at %d, got delta=%d", i, obs[i].stopDelta)
				}
			}
			if stops != expectedStops {
				t.Fatalf("total stop count mismatch: got=%d want=%d", stops, expectedStops)
			}
		})
	}
}

func TestMuteNodeDrumViewPredictionConsistency(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}

	g.updateBeatInfos()
	g.refreshDrumRow()

	if g.engine == nil || g.engine.Predictor == nil {
		t.Fatalf("predictor missing")
	}

	initial := append([]bool(nil), g.drum.Rows[0].Steps...)
	for i, on := range initial {
		abs := g.drum.Offset + i
		bi := g.beatInfoAtRow(0, abs)
		var want bool
		if bi.NodeType == model.NodeTypeMute {
			want = g.engine.Predictor.TriggeredAt(0, abs)
		} else {
			want = g.engine.Predictor.VisibleAt(0, abs)
		}
		if want != on {
			t.Fatalf("predictor mismatch at abs=%d before playback: got=%v want=%v node=%v", abs, want, on, bi.NodeType)
		}
	}

	g.playing = true
	for i := 0; i < 3; i++ {
		g.seqScheduleBeat()
	}
	g.playing = false
	g.refreshDrumRow()

	for i, on := range g.drum.Rows[0].Steps {
		abs := g.drum.Offset + i
		bi := g.beatInfoAtRow(0, abs)
		var want bool
		if bi.NodeType == model.NodeTypeMute {
			want = g.engine.Predictor.TriggeredAt(0, abs)
		} else {
			want = g.engine.Predictor.VisibleAt(0, abs)
		}
		if want != on {
			t.Fatalf("predictor mismatch at abs=%d after playback: got=%v want=%v node=%v", abs, want, on, bi.NodeType)
		}
		if abs < len(initial) {
			if on != initial[abs] {
				t.Fatalf("history mismatch at abs=%d: got=%v want=%v", abs, on, initial[abs])
			}
		}
	}
}
