//go:build js && !test

package ui

import (
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) initJSTimelinePredictor() {
	// rowWindow(row) -> snapshot of Steps for the requested row.
	js.Global().Set("rowWindow", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf(nil)
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.drum.Rows) {
			return js.ValueOf(nil)
		}
		steps := g.drum.Rows[row].Steps
		arr := js.Global().Get("Array").New(len(steps))
		for i, v := range steps {
			arr.SetIndex(i, v)
		}
		return arr
	}))

	// rowCacheOffset(row) -> int (diagnostics for tests)
	js.Global().Set("rowCacheOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf(-1)
		}
		row := args[0].Int()
		g.drum.ensureRowCache()
		if row < 0 || row >= len(g.drum.rowCacheOff) {
			return js.ValueOf(-1)
		}
		return js.ValueOf(g.drum.rowCacheOff[row])
	}))

	// togglePlay – alias of startPlay for clarity in tests (toggles play/pause)
	js.Global().Set("togglePlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.drum.playPressed = true
		return nil
	}))

	// dumpRowState(row) -> diagnostic snapshot of history/predictor state.
	js.Global().Set("dumpRowState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(nil)
		}
		row := args[0].Int()
		state := g.rowStateSnapshot(row)
		obj := js.Global().Get("Object").New()
		obj.Set("nextBeatIdx", state.NextBeatIdx)
		obj.Set("frozenUpTo", state.FrozenUpTo)
		toJSBoolArray := func(src []bool) js.Value {
			if src == nil {
				return js.ValueOf(nil)
			}
			arr := js.Global().Get("Array").New(len(src))
			for i, v := range src {
				arr.SetIndex(i, v)
			}
			return arr
		}

		obj.Set("timelineOffset", state.Timeline.Offset)
		obj.Set("timelinePast", boolSliceToJS(state.Timeline.Past))
		obj.Set("timelinePastMask", boolSliceToJS(state.Timeline.PastMask))
		obj.Set("timelinePastTypes", nodeTypeSliceToJS(state.Timeline.PastTypes))
		obj.Set("timelinePresent", boolSliceToJS(state.Timeline.Present))
		obj.Set("timelineFuture", boolSliceToJS(state.Timeline.Future))
		obj.Set("predictorVisible", toJSBoolArray(state.PredictorVisible))
		obj.Set("predictorAudible", toJSBoolArray(state.PredictorAudible))
		obj.Set("predictorTriggered", toJSBoolArray(state.PredictorTriggered))
		obj.Set("windowOffset", state.Window.Offset)
		obj.Set("windowSteps", boolSliceToJS(state.Window.Steps))
		obj.Set("windowTypes", nodeTypeSliceToJS(state.Window.Types))
		obj.Set("transport", transportSnapshotToJS(state.Transport))
		return obj
	}))

	// transportSnapshot() -> object containing transport-related counters.
	js.Global().Set("transportSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return transportSnapshotToJS(g.transportSnapshot())
	}))

	// dumpTimelineSegments(row) -> {offset, past, pastMask, present, future}
	js.Global().Set("dumpTimelineSegments", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(nil)
		}
		row := args[0].Int()
		seg := g.TimelineSegments(row)
		obj := js.Global().Get("Object").New()
		obj.Set("offset", seg.Offset)
		obj.Set("past", boolSliceToJS(seg.Past))
		obj.Set("pastMask", boolSliceToJS(seg.PastMask))
		obj.Set("pastTypes", nodeTypeSliceToJS(seg.PastTypes))
		obj.Set("present", boolSliceToJS(seg.Present))
		obj.Set("future", boolSliceToJS(seg.Future))
		return obj
	}))

	// timelineCommittedRange(row) -> [start, end, ok]
	js.Global().Set("timelineCommittedRange", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(nil)
		}
		row := args[0].Int()
		start, end, ok := g.timelineCommittedRange(row)
		arr := js.Global().Get("Array").New(3)
		arr.SetIndex(0, start)
		arr.SetIndex(1, end)
		arr.SetIndex(2, ok)
		return arr
	}))

	// setFollow(bool)
	js.Global().Set("setFollow", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		if g.drum != nil {
			f := args[0].Bool()
			g.drum.SetFollow(f)
			if g.simpleDraw {
				g.simpleDrawSavedFollow = f
				g.simpleDrawSavedFollowValid = true
			}
		}
		return nil
	}))

	// gridSubdiv() -> int (MaxDiv)
	js.Global().Set("gridSubdiv", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.grid == nil {
			return js.ValueOf(1)
		}
		return js.ValueOf(g.grid.MaxDiv())
	}))

	// nextBeatIdxs() -> []int
	js.Global().Set("nextBeatIdxs", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arr := js.Global().Get("Array").New()
		for i := 0; i < len(g.nextBeatIdxs); i++ {
			arr.Call("push", g.nextBeatIdxs[i])
		}
		return arr
	}))

	// rowSteps(row) -> []int (0/1)
	js.Global().Set("rowSteps", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 || g.drum == nil {
			return nil
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.drum.Rows) {
			return nil
		}
		steps := g.drum.Rows[row].Steps
		arr := js.Global().Get("Array").New()
		for i := 0; i < len(steps); i++ {
			if steps[i] {
				arr.Call("push", 1)
			} else {
				arr.Call("push", 0)
			}
		}
		return arr
	}))

	// recentSchedulerMismatches() -> [{row, abs, kind, nodeType, scheduled, slate, source, offset, length, inWindow, rowMuted, anySolo, missing, expected, actual, when, force, detail}]
	js.Global().Set("recentSchedulerMismatches", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		m := g.ParityMismatchSnapshot()
		arr := js.Global().Get("Array").New(len(m))
		for i, e := range m {
			obj := js.Global().Get("Object").New()
			obj.Set("row", e.Row)
			obj.Set("abs", e.Abs)
			obj.Set("kind", e.Kind)
			obj.Set("nodeType", int(e.NodeType))
			obj.Set("scheduled", e.Scheduled)
			obj.Set("slate", e.Slate)
			obj.Set("source", e.Source)
			obj.Set("offset", e.Offset)
			obj.Set("length", e.Length)
			obj.Set("inWindow", e.InWindow)
			obj.Set("rowMuted", e.RowMuted)
			obj.Set("anySolo", e.AnySolo)
			obj.Set("missing", e.Missing)
			obj.Set("expected", e.Expected)
			obj.Set("actual", e.Actual)
			obj.Set("when", e.When)
			obj.Set("force", e.Force)
			obj.Set("detail", e.Detail)
			arr.SetIndex(i, obj)
		}
		return arr
	}))

	// clearSchedulerMismatches() -> nil
	js.Global().Set("clearSchedulerMismatches", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.ClearParityMismatches()
		return nil
	}))

	// nodeSuccessorsGrid(i,j) -> array of successors with coordinates.
	js.Global().Set("nodeSuccessorsGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(nil)
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return js.ValueOf(nil)
		}
		arr := js.Global().Get("Array").New()
		for edge := range g.graph.Edges {
			if edge[0] != n.ID {
				continue
			}
			if to := g.nodeByID(edge[1]); to != nil {
				obj := js.Global().Get("Object").New()
				obj.Set("nodeId", int(to.ID))
				obj.Set("i", to.I)
				obj.Set("j", to.J)
				arr.Call("push", obj)
			}
		}
		return arr
	}))

	js.Global().Set("rowBeatCount", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(0)
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.beatInfosByRow) {
			return js.ValueOf(0)
		}
		return js.ValueOf(len(g.beatInfosByRow[row]))
	}))

	js.Global().Set("beatInfoAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		row := args[0].Int()
		idx := args[1].Int()
		info := g.beatInfoAtRow(row, idx)
		typ := "invisible"
		switch info.NodeType {
		case model.NodeTypeRegular:
			typ = "regular"
		case model.NodeTypeSilent:
			typ = "silent"
		case model.NodeTypeMute:
			typ = "mute"
		}
		obj := js.Global().Get("Object").New()
		obj.Set("type", typ)
		obj.Set("nodeId", int(info.NodeID))
		obj.Set("i", info.I)
		obj.Set("j", info.J)
		return obj
	}))

	js.Global().Set("nodeAnimValue", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(0)
		}
		id := model.NodeID(args[0].Int())
		return js.ValueOf(g.nodeAnimGet(id))
	}))

	// nodeHighlightUntilValue(nodeId) -> {start: number, end: number, ok: bool, now: number}
	js.Global().Set("nodeHighlightUntilValue", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		if len(args) < 1 {
			obj.Set("start", 0)
			obj.Set("end", 0)
			obj.Set("ok", false)
			obj.Set("now", audio.Now())
			return obj
		}
		id := model.NodeID(args[0].Int())
		start, end, ok := g.nodeHighlightUntil(id)
		obj.Set("start", start)
		obj.Set("end", end)
		obj.Set("ok", ok)
		obj.Set("now", audio.Now())
		return obj
	}))

	// hasHighlight(row, abs) -> bool
	js.Global().Set("hasHighlight", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		return js.ValueOf(g.hasHighlight(row, abs))
	}))

	// hasRealtimeHighlight(row, abs) -> bool using Game.highlightedBeats
	js.Global().Set("hasRealtimeHighlight", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		return js.ValueOf(g.hasHighlight(row, abs))
	}))

	// hasAnyRowHighlight(row) -> bool — true if any highlight exists for the row.
	js.Global().Set("hasAnyRowHighlight", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		return js.ValueOf(g.hasAnyRowHighlight(row))
	}))

	// ensure(need)
	js.Global().Set("ensure", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		need := args[0].Int()
		if g.engine != nil && g.engine.Predictor != nil {
			g.engine.Predictor.Ensure(need)
		}
		return nil
	}))

	// visibleAt(row, abs) -> bool
	js.Global().Set("visibleAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		if row < 0 || abs < 0 {
			return js.ValueOf(false)
		}
		if g.engine == nil || g.engine.Predictor == nil {
			return js.ValueOf(false)
		}
		g.engine.Predictor.Ensure(abs + 1)
		return js.ValueOf(g.engine.Predictor.VisibleAt(row, abs))
	}))

	// audibleAt(row, abs) -> bool
	js.Global().Set("audibleAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		if row < 0 || abs < 0 {
			return js.ValueOf(false)
		}
		if g.engine == nil || g.engine.Predictor == nil {
			return js.ValueOf(false)
		}
		g.engine.Predictor.Ensure(abs + 1)
		return js.ValueOf(g.engine.Predictor.AudibleAt(row, abs))
	}))

	js.Global().Set("triggeredAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		if row < 0 || abs < 0 {
			return js.ValueOf(false)
		}
		info := g.beatInfoAtRow(row, abs)
		if info.NodeType != model.NodeTypeRegular && info.NodeType != model.NodeTypeMute {
			return js.ValueOf(false)
		}
		if g.engine == nil || g.engine.Predictor == nil {
			return js.ValueOf(false)
		}
		g.engine.Predictor.Ensure(abs + 1)
		return js.ValueOf(g.engine.Predictor.TriggeredAt(row, abs))
	}))

	// predictorAudibleSnapshot(row, start, count) -> []int(0/1)
	js.Global().Set("predictorAudibleSnapshot", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return js.Global().Get("Array").New()
		}
		row := args[0].Int()
		start := args[1].Int()
		count := args[2].Int()
		if count < 0 {
			count = 0
		}
		arr := js.Global().Get("Array").New()
		if g.engine != nil && g.engine.Predictor != nil {
			need := start + count
			if need > 0 {
				g.engine.Predictor.Ensure(need)
			}
			for i := 0; i < count; i++ {
				v := g.engine.Predictor.AudibleAt(row, start+i)
				if v {
					arr.Call("push", 1)
				} else {
					arr.Call("push", 0)
				}
			}
			return arr
		}
		return arr
	}))
}
