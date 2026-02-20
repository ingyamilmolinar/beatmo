package ui

import (
	"strconv"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func scheduleAbsForMuteTest(g *Game, abs int) {
	// Ensure per-row counters exist when tests drive scheduling directly.
	if g != nil && g.drum != nil {
		if len(g.seqNextIdxs) != len(g.drum.Rows) {
			g.seqNextIdxs = make([]int, len(g.drum.Rows))
		}
		if len(g.nextBeatIdxs) != len(g.drum.Rows) {
			g.nextBeatIdxs = make([]int, len(g.drum.Rows))
		}
	}
	setPlayStartForAbs(g, abs)
	g.seqScheduleTime()
	for {
		select {
		case ev := <-g.hlCh:
			g.applySequencerHighlight(ev.row, ev.idx, ev.info)
		default:
			return
		}
	}
}

func TestMuteNodeStopsAudioWithoutGating(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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

	g.SetPlaying(true)
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

	scheduleAbsForMuteTest(g, 0)
	if plays != 1 || stops != 0 {
		t.Fatalf("after regular beat: plays=%d stops=%d", plays, stops)
	}

	scheduleAbsForMuteTest(g, 1)
	if stops != 1 {
		t.Fatalf("mute beat did not stop audio: stops=%d", stops)
	}
	if plays != 1 {
		t.Fatalf("mute beat should not play: plays=%d", plays)
	}

	// Next beat should resume playback immediately (no gate hold).
	scheduleAbsForMuteTest(g, 2)
	if plays != 2 {
		t.Fatalf("tail beat should play after mute: plays=%d", plays)
	}

	if len(g.muteUntilByRow) == 0 || g.muteUntilByRow[0] > muteIdx+1 {
		t.Fatalf("unexpected mute gate extension: %v", g.muteUntilByRow)
	}

	g.engine.Predictor.Ensure(muteIdx + 2)
	if !g.engine.Predictor.AudibleAt(0, muteIdx+1) {
		t.Fatalf("predictor muted following beat")
	}
}

func TestMuteNodeHighlightRespectsLogic(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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

	g.SetPlaying(true)
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
	maxSteps := len(g.beatInfosByRow[0])*16 + 32
	for abs := 0; abs < maxSteps && len(states) < 4; abs++ {
		info := g.beatInfoAtRow(0, abs)
		scheduleAbsForMuteTest(g, abs)
		if info.NodeID == mute.ID {
			key := makeBeatKey(0, abs)
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
}

func TestMuteNodeLogicEveryNTriggers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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

	g.SetPlaying(true)
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
	maxSteps := cycleLen*16 + 32
	for abs := 0; abs < maxSteps && len(muteTriggers) < 4; abs++ {
		info := g.beatInfoAtRow(0, abs)
		scheduleAbsForMuteTest(g, abs)
		if info.NodeID == mute.ID {
			state, _ := g.lastTriggeredForTest(0, mute.ID)
			muteTriggers = append(muteTriggers, state)
			key := makeBeatKey(0, abs)
			_, highlighted := g.highlightedBeats[key]
			if state && !highlighted {
				t.Fatalf("mute should highlight when triggered at idx %d", abs)
			}
			if !state && highlighted {
				t.Fatalf("mute highlight persisted when logic disabled at idx %d", abs)
			}
			delete(g.highlightedBeats, key)
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
}

func TestMuteNodeHighlight(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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

	g.SetPlaying(true)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Advance until we schedule the mute node once.
	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)
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
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

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
	g.SetPlaying(true)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	return g, mute.ID
}

func collectMuteObservations(t *testing.T, g *Game, row int, muteID model.NodeID, want int, stops func() int) []muteObservation {
	t.Helper()
	if row < 0 || row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
		t.Fatalf("invalid beat infos for row %d", row)
	}
	observations := make([]muteObservation, 0, want)
	prevStops := stops()
	maxSteps := len(g.beatInfosByRow[row])*want*8 + 32
	for abs := 0; len(observations) < want && abs < maxSteps; abs++ {
		info := g.beatInfoAtRow(row, abs)
		scheduleAbsForMuteTest(g, abs)
		if info.NodeID != muteID {
			continue
		}
		triggered, _ := g.lastTriggeredForTest(row, muteID)
		key := makeBeatKey(row, abs)
		val, ok := g.highlightedBeats[key]
		highlight := ok && isMuteHighlight(val)
		currStops := stops()
		observations = append(observations, muteObservation{
			triggered: triggered,
			highlight: highlight,
			stopDelta: currStops - prevStops,
		})
		prevStops = currStops
		delete(g.highlightedBeats, key)
	}
	if len(observations) != want {
		t.Fatalf("insufficient mute samples: got %d want %d", len(observations), want)
	}
	return observations
}

func TestMuteNodeSkipEveryNLogicLoopAudioHighlight(t *testing.T) {
	assertDefaultParityState(t)
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
				stopPlaybackForTest(g)
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
	assertDefaultParityState(t)
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
				stopPlaybackForTest(g)
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
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)
	scheduleAbsForMuteTest(g, 2)
	pressStop(t, g.drum)
	_ = g.Update()
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
