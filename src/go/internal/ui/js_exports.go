//go:build js && !test

package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	"image"
	"math"
	"strconv"
	"strings"
	"syscall/js"
)

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	js.Global().Set("startPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		js.Global().Get("console").Call("log", "[WASM] startPlay() called")
		g.drum.playPressed = true
		return nil
	}))
	js.Global().Set("stopPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.drum.stopPressed = true
		return nil
	}))

	// setSimpleDraw(bool) – reduce rendering complexity for perf (web).
	js.Global().Set("setSimpleDraw", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		g.simpleDraw = args[0].Bool()
		return nil
	}))

	// gridCacheInfo() -> { tileReady: bool, cacheReady: bool, simpleDraw: bool }
	js.Global().Set("gridCacheInfo", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("tileReady", g.gridTile != nil)
		obj.Set("cacheReady", g.gridCache != nil && g.gridCacheW > 0)
		obj.Set("simpleDraw", g.simpleDraw)
		return obj
	}))

	// forceDraw() – render one frame into an offscreen image to build caches.
	js.Global().Set("forceDraw", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		w, h := g.winW, g.winH
		if w <= 0 {
			w = 800
		}
		if h <= 0 {
			h = 600
		}
		img := ebiten.NewImage(w, h)
		g.Draw(img)
		return nil
	}))

	// togglePlay – alias of startPlay for clarity in tests (toggles play/pause)
	js.Global().Set("togglePlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.drum.playPressed = true
		return nil
	}))

	// zoomAt(x, y, delta) applies a zoom centered at screen coords (x,y).
	js.Global().Set("zoomAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		x := args[0].Float()
		y := args[1].Float()
		delta := args[2].Float()
		wx := (x - g.cam.OffsetX) / g.cam.Scale
		// Account for the transport bar offset in screen space
		wy := (y - float64(topOffset) - g.cam.OffsetY) / g.cam.Scale
		constZoomFactor := 1.05
		constSens := 0.1
		newScale := g.cam.Scale * math.Pow(constZoomFactor, delta*constSens)
		if newScale < 0.1 {
			newScale = 0.1
		} else if newScale > 10.0 {
			newScale = 10.0
		}
		g.cam.OffsetX = x - wx*newScale
		g.cam.OffsetY = y - float64(topOffset) - wy*newScale
		g.cam.Scale = newScale
		return nil
	}))

	// setFollow(bool)
	js.Global().Set("setFollow", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		g.drum.follow = args[0].Bool()
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
		if g.engine != nil && g.engine.Predictor != nil {
			return js.ValueOf(g.engine.Predictor.VisibleAt(row, abs))
		}
		g.predMu.RLock()
		defer g.predMu.RUnlock()
		if row < len(g.predVisibleByRow) && abs < len(g.predVisibleByRow[row]) {
			return js.ValueOf(g.predVisibleByRow[row][abs])
		}
		return js.ValueOf(false)
	}))

	// audibleAt(row, abs) -> bool
	js.Global().Set("audibleAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		if g.engine != nil && g.engine.Predictor != nil {
			return js.ValueOf(g.engine.Predictor.AudibleAt(row, abs))
		}
		// fall back to UI prediction snapshot when engine predictor is unavailable
		g.predMu.RLock()
		defer g.predMu.RUnlock()
		if row < len(g.predAudibleByRow) && abs < len(g.predAudibleByRow[row]) {
			return js.ValueOf(g.predAudibleByRow[row][abs])
		}
		return js.ValueOf(false)
	}))
	js.Global().Set("triggeredAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		row := args[0].Int()
		abs := args[1].Int()
		info := g.beatInfoAtRow(row, abs)
		if info.NodeType != model.NodeTypeRegular && info.NodeType != model.NodeTypeMute {
			return js.ValueOf(false)
		}
		g.ensurePredictions(abs + 1)
		if g.engine != nil && g.engine.Predictor != nil {
			return js.ValueOf(g.engine.Predictor.TriggeredAt(row, abs))
		}
		g.predMu.RLock()
		defer g.predMu.RUnlock()
		if row < len(g.predTriggeredByRow) && abs < len(g.predTriggeredByRow[row]) {
			return js.ValueOf(g.predTriggeredByRow[row][abs])
		}
		return js.ValueOf(false)
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
		// fallback to UI predictions
		g.predMu.RLock()
		defer g.predMu.RUnlock()
		for i := 0; i < count; i++ {
			idx := start + i
			val := 0
			if row < len(g.predAudibleByRow) && idx >= 0 && idx < len(g.predAudibleByRow[row]) && g.predAudibleByRow[row][idx] {
				val = 1
			}
			arr.Call("push", val)
		}
		return arr
	}))

	// Graph mutation helpers for e2e tests
	js.Global().Set("addNode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return js.ValueOf(-1)
		}
		i := args[0].Int()
		j := args[1].Int()
		t := args[2].String()
		nt := model.NodeTypeRegular
		switch t {
		case "invisible":
			nt = model.NodeTypeInvisible
		case "silent":
			nt = model.NodeTypeSilent
		case "mute":
			nt = model.NodeTypeMute
		}
		n := g.tryAddNode(i, j, nt)
		return js.ValueOf(int(n.ID))
	}))
	js.Global().Set("addEdgeGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			return nil
		}
		i1 := args[0].Int()
		j1 := args[1].Int()
		i2 := args[2].Int()
		j2 := args[3].Int()
		a := g.nodeAt(i1, j1)
		b := g.nodeAt(i2, j2)
		if a != nil && b != nil {
			g.addEdge(a, b)
		}
		return nil
	}))
	js.Global().Set("deleteEdgeGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			return nil
		}
		i1 := args[0].Int()
		j1 := args[1].Int()
		i2 := args[2].Int()
		j2 := args[3].Int()
		a := g.nodeAt(i1, j1)
		b := g.nodeAt(i2, j2)
		if a != nil && b != nil {
			g.deleteEdge(a, b)
		}
		return nil
	}))
	js.Global().Set("deleteNodeGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n != nil {
			g.deleteNode(n)
		}
		return nil
	}))
	js.Global().Set("updateBeatInfosJS", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.updateBeatInfos()
		return nil
	}))

	// setOrigin(row, i, j) – set a drum row origin to the node at grid (i,j).
	js.Global().Set("setOrigin", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 || g.drum == nil {
			return nil
		}
		row := args[0].Int()
		i := args[1].Int()
		j := args[2].Int()
		if row < 0 || row >= len(g.drum.Rows) {
			return nil
		}
		n := g.nodeAt(i, j)
		if n == nil {
			return nil
		}
		g.drum.Rows[row].Origin = n.ID
		g.drum.Rows[row].Node = n
		if row == 0 {
			g.start = n
			g.graph.StartNodeID = n.ID
		}
		g.updateBeatInfos()
		return nil
	}))

	// setNodeLogicGrid(i,j, kind, n, p)
	js.Global().Set("setNodeLogicGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		kind := args[2].String()
		n := 0
		if len(args) > 3 {
			n = args[3].Int()
		}
		p := 0.0
		if len(args) > 4 {
			p = args[4].Float()
		}
		node := g.nodeAt(i, j)
		if node == nil {
			return nil
		}
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			ps := mn.Params
			ps.LogicKind = kind
			if kind == "every_n_triggers" || kind == "skip_every_n" {
				ps.LogicN = n
			}
			if kind == "probability" {
				ps.LogicP = p
			}
			g.graph.SetNodeParams(node.ID, ps)
		}
		return nil
	}))
	// setBPM(n) sets the DrumView BPM directly for perf tests.
	js.Global().Set("setBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		b := args[0].Int()
		if b < 1 {
			b = 1
		}
		g.drum.SetBPM(b)
		return nil
	}))

	// commitBPM(n) simulates typing a number and pressing Enter in the BPM box.
	js.Global().Set("commitBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmBox == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		b := args[0].Int()
		if b < 1 {
			b = 1
		}
		g.drum.bpmBox.focused = true
		g.drum.bpmBox.SetText(strconv.Itoa(b))
		g.drum.SetBPM(b)
		g.drum.bpmBox.focused = false
		return nil
	}))
	// perfStats() -> object with recent perf metrics.
	js.Global().Set("perfStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		s := g.PerfSnapshot()
		obj := js.Global().Get("Object").New()
		obj.Set("frames", int(s.Frames))
		obj.Set("fpsAvg", s.FPSAvg)
		obj.Set("updateAvgMS", s.UpdateAvgMS)
		obj.Set("updateMaxMS", s.UpdateMaxMS)
		obj.Set("drawAvgMS", s.DrawAvgMS)
		obj.Set("drawMaxMS", s.DrawMaxMS)
		obj.Set("audioEnq", int(s.AudioEnq))
		obj.Set("audioDeq", int(s.AudioDeq))
		obj.Set("audioQLatAvg", s.AudioQLatAvg)
		obj.Set("audioQLatMax", s.AudioQLatMax)
		obj.Set("audioCallAvg", s.AudioCallAvg)
		obj.Set("audioCallMax", s.AudioCallMax)
		return obj
	}))
	// buildPerfRect(rows, side) builds 'rows' disjoint 1-rectangle loops
	// each with edges of length 'side' grid units to stress scheduling.
	js.Global().Set("buildPerfRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		rows := 1
		side := 1
		if len(args) > 0 {
			rows = args[0].Int()
		}
		if len(args) > 1 {
			side = args[1].Int()
		}
		if rows < 1 {
			rows = 1
		}
		if side < 1 {
			side = 1
		}
		// Ensure enough rows exist in the drum view
		for len(g.drum.Rows) < rows {
			g.drum.AddRow()
		}
		for r := 0; r < rows; r++ {
			g.pendingStartRow = r
			baseX := r * (side + 2)
			baseY := 0
			n0 := g.tryAddNode(baseX, baseY, model.NodeTypeRegular)
			n1 := g.tryAddNode(baseX+side, baseY, model.NodeTypeRegular)
			n2 := g.tryAddNode(baseX+side, baseY+side, model.NodeTypeRegular)
			n3 := g.tryAddNode(baseX, baseY+side, model.NodeTypeRegular)
			g.addEdge(n0, n1)
			g.addEdge(n1, n2)
			g.addEdge(n2, n3)
			g.addEdge(n3, n0)
			g.pendingStartRow = -1
		}
		g.updateBeatInfos()
		return nil
	}))
	js.Global().Set("incrementBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		b := g.drum.BPM() + 1
		g.drum.SetBPM(b)
		js.Global().Get("console").Call("log", "[WASM] incrementBPM ->", b)
		return nil
	}))
	js.Global().Set("currentBeat", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		cb := g.currentBeat()
		js.Global().Get("console").Call("log", "[WASM] currentBeat()=", cb)
		return js.ValueOf(cb)
	}))

	// camScale() -> float, camOffset() -> {x,y}
	js.Global().Set("camScale", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.cam.Scale)
	}))
	js.Global().Set("camOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("x", g.cam.OffsetX)
		obj.Set("y", g.cam.OffsetY)
		return obj
	}))

	// panBy(dx, dy) – adjust camera offset directly and snap to pixels.
	js.Global().Set("panBy", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		dx := args[0].Float()
		dy := args[1].Float()
		g.cam.OffsetX += dx
		g.cam.OffsetY += dy
		g.cam.Snap()
		return nil
	}))

	// nodeRect(i,j) -> {x,y,w,h} for a node at grid coordinates. nil if none.
	js.Global().Set("nodeRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return nil
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		obj := js.Global().Get("Object").New()
		obj.Set("x", int(x1))
		obj.Set("y", int(y1))
		obj.Set("w", int(x2-x1))
		obj.Set("h", int(y2-y1))
		return obj
	}))

	// nodeIdAt(i,j) -> id or -1 if none
	js.Global().Set("nodeIdAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(-1)
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(n.ID))
	}))

	// nodeHighlightedAt(i,j) -> bool using last drawn highlight state
	js.Global().Set("nodeHighlightedAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return js.ValueOf(false)
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.lastNodeHLHas(n.ID))
	}))

	// openNodeMenu(i,j) -> opens the node popup for the node at grid coords.
	js.Global().Set("openNodeMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return nil
		}
		g.sel = n
		n.Selected = true
		g.nodeMenuOpen = true
		g.nodeMenuNode = n
		return nil
	}))

	// zoomBtnRects() retained for backward compatibility; grid zoom buttons
	// are disabled in this UI. Return an empty object to satisfy callers.
	js.Global().Set("zoomBtnRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.Global().Get("Object").New()
	}))

	// nodeMenuRect(id) -> {x,y,w,h} for popup control id; "panel" for full panel.
	js.Global().Set("nodeMenuRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		id := args[0].String()
		g.updateNodeMenuRects()
		r, ok := g.nodeMenuRects[id]
		if !ok {
			return nil
		}
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// triggerOnce(i,j,pitch,dur) – test helper to schedule one audio + highlight for a grid node.
	js.Global().Set("triggerOnce", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 4 {
			return js.ValueOf(false)
		}
		i := args[0].Int()
		j := args[1].Int()
		pitch := args[2].Float()
		dur := args[3].Float()
		n := g.nodeAt(i, j)
		if n == nil {
			return js.ValueOf(false)
		}
		row, ok := g.nodeRows[n.ID]
		if !ok || row < 0 || row >= len(g.drum.Rows) {
			return js.ValueOf(false)
		}
		info := model.BeatInfo{NodeID: n.ID, NodeType: model.NodeTypeRegular, I: i, J: j}
		inst := g.drum.Rows[row].Instrument
		g.scheduleSound(row, 0, info, inst, 1.0, pitch, dur, math.NaN())
		return js.ValueOf(true)
	}))

	// nodeParams(i,j) -> {volume, pitch, duration, logicKind, logicN, logicP, skipN, type}
	js.Global().Set("nodeParams", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		n := g.nodeAt(i, j)
		if n == nil {
			return nil
		}
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			return nil
		}
		obj := js.Global().Get("Object").New()
		obj.Set("volume", mn.Params.Volume)
		obj.Set("pitch", mn.Params.Pitch)
		obj.Set("duration", mn.Params.Duration)
		obj.Set("logicKind", mn.Params.LogicKind)
		obj.Set("logicN", mn.Params.LogicN)
		obj.Set("logicP", mn.Params.LogicP)
		obj.Set("skipN", mn.Params.SkipEveryN)
		obj.Set("type", int(mn.Type))
		return obj
	}))

	// nodeMenuAction(id) -> applies the same action as clicking a popup button
	js.Global().Set("nodeMenuAction", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		if g.nodeMenuNode == nil {
			return nil
		}
		id := args[0].String()
		node := g.nodeMenuNode
		mn, ok := g.graph.GetNodeByID(node.ID)
		if !ok {
			return nil
		}
		switch id {
		case "vol-":
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume -= 0.1
			if p.Volume < 0 {
				p.Volume = 0
			}
			g.graph.SetNodeParams(node.ID, p)
		case "vol+":
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume += 0.1
			g.graph.SetNodeParams(node.ID, p)
		case "pit-":
			p := mn.Params
			p.Pitch -= 1
			if p.Pitch < -24 {
				p.Pitch = -24
			}
			g.graph.SetNodeParams(node.ID, p)
		case "pit+":
			p := mn.Params
			p.Pitch += 1
			if p.Pitch > 24 {
				p.Pitch = 24
			}
			g.graph.SetNodeParams(node.ID, p)
		case "dur-":
			p := mn.Params
			if p.Duration == 0 {
				p.Duration = 1
			}
			p.Duration -= 0.1
			if p.Duration < 0.25 {
				p.Duration = 0.25
			}
			g.graph.SetNodeParams(node.ID, p)
		case "dur+":
			p := mn.Params
			if p.Duration == 0 {
				p.Duration = 1
			}
			p.Duration += 0.1
			if p.Duration > 4 {
				p.Duration = 4
			}
			g.graph.SetNodeParams(node.ID, p)
		case "ln-":
			p := mn.Params
			if p.LogicN > 0 {
				p.LogicN--
			}
			if p.LogicN < 1 {
				p.LogicN = 1
			}
			g.graph.SetNodeParams(node.ID, p)
		case "ln+":
			p := mn.Params
			p.LogicN++
			if p.LogicN > 64 {
				p.LogicN = 64
			}
			g.graph.SetNodeParams(node.ID, p)
		case "lp-":
			p := mn.Params
			p.LogicP -= 0.1
			if p.LogicP < 0 {
				p.LogicP = 0
			}
			g.graph.SetNodeParams(node.ID, p)
		case "lp+":
			p := mn.Params
			p.LogicP += 0.1
			if p.LogicP > 1 {
				p.LogicP = 1
			}
			g.graph.SetNodeParams(node.ID, p)
		case "aud":
			n := mn
			switch n.Type {
			case model.NodeTypeSilent:
				n.Type = model.NodeTypeMute
			case model.NodeTypeMute:
				n.Type = model.NodeTypeRegular
			default:
				n.Type = model.NodeTypeSilent
			}
			g.graph.Nodes[node.ID] = n
			g.updateBeatInfos()
		}
		g.notifyPredictorNode(node.ID)
		return nil
	}))

	// nodeActionAt(i,j,id) -> applies an action to the node at grid coords
	js.Global().Set("nodeActionAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		id := args[2].String()
		n := g.nodeAt(i, j)
		if n == nil {
			return nil
		}
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			return nil
		}
		switch id {
		case "vol-":
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume -= 0.1
			if p.Volume < 0 {
				p.Volume = 0
			}
			g.graph.SetNodeParams(n.ID, p)
		case "vol+":
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume += 0.1
			g.graph.SetNodeParams(n.ID, p)
		case "pit-":
			p := mn.Params
			p.Pitch -= 1
			if p.Pitch < -24 {
				p.Pitch = -24
			}
			g.graph.SetNodeParams(n.ID, p)
		case "pit+":
			p := mn.Params
			p.Pitch += 1
			if p.Pitch > 24 {
				p.Pitch = 24
			}
			g.graph.SetNodeParams(n.ID, p)
		case "dur-":
			p := mn.Params
			if p.Duration == 0 {
				p.Duration = 1
			}
			p.Duration -= 0.1
			if p.Duration < 0.25 {
				p.Duration = 0.25
			}
			g.graph.SetNodeParams(n.ID, p)
		case "dur+":
			p := mn.Params
			if p.Duration == 0 {
				p.Duration = 1
			}
			p.Duration += 0.1
			if p.Duration > 4 {
				p.Duration = 4
			}
			g.graph.SetNodeParams(n.ID, p)
		case "ln-":
			p := mn.Params
			if p.LogicN > 0 {
				p.LogicN--
			}
			if p.LogicN < 1 {
				p.LogicN = 1
			}
			g.graph.SetNodeParams(n.ID, p)
		case "ln+":
			p := mn.Params
			p.LogicN++
			if p.LogicN > 64 {
				p.LogicN = 64
			}
			g.graph.SetNodeParams(n.ID, p)
		case "lp-":
			p := mn.Params
			p.LogicP -= 0.1
			if p.LogicP < 0 {
				p.LogicP = 0
			}
			g.graph.SetNodeParams(n.ID, p)
		case "lp+":
			p := mn.Params
			p.LogicP += 0.1
			if p.LogicP > 1 {
				p.LogicP = 1
			}
			g.graph.SetNodeParams(n.ID, p)
		case "aud":
			n2 := mn
			switch n2.Type {
			case model.NodeTypeSilent:
				n2.Type = model.NodeTypeMute
			case model.NodeTypeMute:
				n2.Type = model.NodeTypeRegular
			default:
				n2.Type = model.NodeTypeSilent
			}
			g.graph.Nodes[n.ID] = n2
			g.updateBeatInfos()
		}
		g.notifyPredictorNode(n.ID)
		return nil
	}))
	// ensureDefaultPath builds a minimal single-edge path if the graph is empty
	// so browser tests can start playback without interacting with the editor.
	js.Global().Set("ensureDefaultPath", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(g.nodes) == 0 {
			g.pendingStartRow = 0
			n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
			n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
			g.addEdge(n0, n1)
			g.pendingStartRow = -1
			g.start = n0
			g.graph.StartNodeID = n0.ID
			g.updateBeatInfos()
		}
		return nil
	}))
	js.Global().Set("setRowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		row := args[0].Int()
		vol := args[1].Float()
		if row >= 0 && row < len(g.drum.Rows) {
			if vol < 0 {
				vol = 0
			}
			if vol > 1 {
				vol = 1
			}
			g.drum.Rows[row].Volume = vol
			if row < len(g.drum.rowVolSliders) {
				g.drum.rowVolSliders[row].Value = vol
			}
		}
		return nil
	}))

	// Export the current state as a JSON string.
	js.Global().Set("exportJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf("")
		}
		if data, err := g.drum.exportBytes(); err == nil {
			return js.ValueOf(string(data))
		}
		return js.ValueOf("")
	}))
	// Import a JSON string to rebuild the state.
	js.Global().Set("importJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		txt := args[0].String()
		_ = g.Import([]byte(txt))
		return nil
	}))

	// sliderRect(row) -> {x, y, w, h}
	// Exposes the on-screen rectangle for a drum row's volume slider so
	// browser tests can simulate pointer interaction precisely.
	js.Global().Set("sliderRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.drum.rowVolSliders) {
			return nil
		}
		r := g.drum.rowVolSliders[row].Rect()
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	// timelineRect() -> {x,y,w,h}
	js.Global().Set("timelineRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.timelineRect
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// bpmBoxRect() -> {x,y,w,h}
	js.Global().Set("bpmBoxRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmBox == nil {
			return nil
		}
		r := g.drum.bpmBox.Rect
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// splitY() -> int
	js.Global().Set("splitY", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.split == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.split.Y)
	}))
	// setSplitY(y)
	js.Global().Set("setSplitY", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.split == nil || len(args) < 1 {
			return nil
		}
		y := args[0].Int()
		g.split.Y = y
		g.split.userSet = true
		if g.winH > 0 {
			g.split.ratio = float64(g.split.Y) / float64(g.winH)
		}
		return nil
	}))

	// drumBounds() -> {x,y,w,h}
	js.Global().Set("drumBounds", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.Bounds
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// scrollBarRect() / scrollThumbRect() -> {x,y,w,h}
	js.Global().Set("scrollBarRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.scrollBarRect()
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	js.Global().Set("scrollThumbRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.scrollThumbRect()
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// rowOffset() -> int (vertical scroll)
	js.Global().Set("rowOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.rowOffset)
	}))
	// setRowOffset(n)
	js.Global().Set("setRowOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		off := args[0].Int()
		if off < 0 {
			off = 0
		}
		max := len(g.drum.Rows) - g.drum.visibleRows()
		if max < 0 {
			max = 0
		}
		if off > max {
			off = max
		}
		g.drum.rowOffset = off
		return nil
	}))

	// addDrumRow(); totalRows() -> int
	js.Global().Set("addDrumRow", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum != nil {
			g.drum.AddRow()
		}
		return nil
	}))
	js.Global().Set("totalRows", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(len(g.drum.Rows))
	}))

	// rowLabelRect(row), rowEditBtnRect(row), rowColorBtnRect(row) -> {x,y,w,h}
	btnRect := func(r image.Rectangle) js.Value {
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}
	js.Global().Set("rowLabelRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowLabels) {
			return nil
		}
		return btnRect(g.drum.rowLabels[i].Rect())
	}))
	js.Global().Set("rowEditBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowEditBtns) {
			return nil
		}
		return btnRect(g.drum.rowEditBtns[i].Rect())
	}))
	// rowLabelText(row) -> string
	js.Global().Set("rowLabelText", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf("")
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowLabels) {
			return js.ValueOf("")
		}
		return js.ValueOf(g.drum.rowLabels[i].Text)
	}))
	js.Global().Set("rowColorBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowColorBtns) {
			return nil
		}
		return btnRect(g.drum.rowColorBtns[i].Rect())
	}))

	// rowMuteBtnRect(row), rowSoloBtnRect(row)
	js.Global().Set("rowMuteBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowMuteBtns) {
			return nil
		}
		return btnRect(g.drum.rowMuteBtns[i].Rect())
	}))
	js.Global().Set("rowSoloBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowSoloBtns) {
			return nil
		}
		return btnRect(g.drum.rowSoloBtns[i].Rect())
	}))

	// rowMuted(row) / rowSoloed(row)
	js.Global().Set("rowMuted", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf(false)
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.drum.Rows[i].Muted)
	}))
	js.Global().Set("rowSoloed", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf(false)
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.drum.Rows[i].Solo)
	}))

	// toggleMute(row), toggleSolo(row)
	js.Global().Set("toggleMute", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return nil
		}
		g.drum.toggleMute(i)
		return nil
	}))
	js.Global().Set("toggleSolo", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return nil
		}
		g.drum.toggleSolo(i)
		return nil
	}))

	// rowInstrument(row) -> string
	js.Global().Set("rowInstrument", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf("")
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return js.ValueOf("")
		}
		return js.ValueOf(g.drum.Rows[i].Instrument)
	}))

	// instOptions() -> []string
	js.Global().Set("instOptions", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arr := js.Global().Get("Array").New()
		if g.drum == nil {
			return arr
		}
		for _, id := range g.drum.instOptions {
			arr.Call("push", id)
		}
		return arr
	}))

	// openInstMenu(row)
	js.Global().Set("openInstMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return nil
		}
		g.drum.selRow = i
		g.drum.instMenuRow = i
		g.drum.instMenuOpen = true
		g.drum.buildInstMenu()
		return nil
	}))
	// instMenuItemRects() -> [{id,x,y,w,h}]
	js.Global().Set("instMenuItemRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arr := js.Global().Get("Array").New()
		if g.drum == nil {
			return arr
		}
		for idx := range g.drum.instMenuBtns {
			btn := g.drum.instMenuBtns[idx]
			if idx >= 0 && idx < len(g.drum.instOptions) {
				id := g.drum.instOptions[idx]
				obj := js.Global().Get("Object").New()
				r := btn.Rect()
				obj.Set("id", id)
				obj.Set("x", r.Min.X)
				obj.Set("y", r.Min.Y)
				obj.Set("w", r.Dx())
				obj.Set("h", r.Dy())
				arr.Call("push", obj)
			}
		}
		return arr
	}))

	// rowColor(row) -> hex string like #RRGGBBAA
	js.Global().Set("rowColor", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return js.ValueOf("")
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return js.ValueOf("")
		}
		return js.ValueOf(g.drum.colorKey(g.drum.Rows[i].Color))
	}))

	// Hex input removed; wheel used instead

	// colorWheelRect() -> {x,y,w,h}
	js.Global().Set("colorWheelRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.colorWheelRect
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))

	// pickColorAtWheel(row, fx, fy) – opens wheel if needed and picks color at fractional coords (0..1)
	js.Global().Set("pickColorAtWheel", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 3 {
			return nil
		}
		row := args[0].Int()
		fx := args[1].Float()
		fy := args[2].Float()
		if row < 0 || row >= len(g.drum.Rows) {
			return nil
		}
		g.drum.selRow = row
		g.drum.colorMenuRow = row
		g.drum.colorMenuOpen = true
		g.drum.buildColorMenu()
		r := g.drum.colorWheelRect
		if r.Dx() <= 0 || r.Dy() <= 0 {
			return nil
		}
		if fx < 0 {
			fx = 0
		}
		if fx > 1 {
			fx = 1
		}
		if fy < 0 {
			fy = 0
		}
		if fy > 1 {
			fy = 1
		}
		x := int(float64(r.Min.X) + fx*float64(r.Dx()))
		y := int(float64(r.Min.Y) + fy*float64(r.Dy()))
		col := g.drum.pickColorFromWheel(x, y)
		g.drum.SetRowColor(row, col)
		g.drum.colorMenuOpen = false
		return nil
	}))

	// openColorMenu(row)
	js.Global().Set("openColorMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.Rows) {
			return nil
		}
		g.drum.selRow = i
		g.drum.colorMenuRow = i
		g.drum.colorMenuOpen = true
		g.drum.buildColorMenu()
		return nil
	}))

	// subdivBtnRect() -> {x,y,w,h}
	js.Global().Set("subdivBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.subdivBtn == nil {
			return nil
		}
		return btnRect(g.drum.subdivBtn.Rect())
	}))
	// subdivMenuItemRects() -> [{val,x,y,w,h}]
	js.Global().Set("subdivMenuItemRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arr := js.Global().Get("Array").New()
		if g.drum == nil {
			return arr
		}
		for _, btn := range g.drum.subdivMenuBtns {
			obj := js.Global().Get("Object").New()
			r := btn.Rect()
			obj.Set("x", r.Min.X)
			obj.Set("y", r.Min.Y)
			obj.Set("w", r.Dx())
			obj.Set("h", r.Dy())
			// Button text holds the value, parse to int
			v, _ := strconv.Atoi(btn.Text)
			obj.Set("val", v)
			arr.Call("push", obj)
		}
		return arr
	}))
	// timelineUnitsPerBeat() -> int
	js.Global().Set("timelineUnitsPerBeat", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.timelineUnitsPerBeat)
	}))

	// uploadBtnRect() -> {x,y,w,h}
	js.Global().Set("uploadBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.uploadBtn == nil {
			return nil
		}
		return btnRect(g.drum.uploadBtn.Rect())
	}))
	// isUploading() -> bool
	js.Global().Set("isUploading", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.drum.uploading)
	}))

	// renameBoxRect() -> {x,y,w,h} when active; commitRename(name)
	js.Global().Set("renameBoxRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.renameBox == nil {
			return nil
		}
		r := g.drum.renameBox.Rect
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	js.Global().Set("commitRename", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.renameBox == nil || len(args) < 1 {
			return nil
		}
		name := args[0].String()
		name = strings.TrimSpace(name)
		if name != "" && g.drum.renameRow >= 0 && g.drum.renameRow < len(g.drum.Rows) {
			oldID := g.drum.Rows[g.drum.renameRow].Instrument
			newID := strings.ToLower(name)
			audio.RenameInstrument(oldID, newID)
			if g.drum.samplePath != nil {
				if p, ok := g.drum.samplePath[oldID]; ok {
					g.drum.samplePath[newID] = p
					delete(g.drum.samplePath, oldID)
				}
			}
			g.drum.Rows[g.drum.renameRow].Instrument = newID
			g.drum.Rows[g.drum.renameRow].Name = name
			g.drum.rowLabels[g.drum.renameRow].Text = name
			customColors[newID] = g.drum.Rows[g.drum.renameRow].Color
			g.drum.refreshInstruments()
		}
		g.drum.renameBox = nil
		g.drum.renameRow = -1
		return nil
	}))
	// openRenameBox(row)
	js.Global().Set("openRenameBox", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowLabels) {
			return nil
		}
		r := g.drum.rowLabels[i].Rect()
		g.drum.renameRow = i
		g.drum.renameBox = NewTextInput(r, BPMBoxStyle)
		g.drum.renameBox.SetText(g.drum.Rows[i].Name)
		g.drum.renameBox.focused = true
		g.drum.renameBox.anim = 1
		g.drum.renameHold = false
		g.drum.instMenuOpen = false
		g.drum.colorMenuOpen = false
		return nil
	}))

	// getBPM() -> int
	js.Global().Set("getBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.BPM())
	}))

	// getEngineBPM() -> int
	js.Global().Set("getEngineBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.engine == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.engine.BPM())
	}))

	// getAppliedBPM() -> int
	js.Global().Set("getAppliedBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.appliedBPM)
	}))
	// drumOffset() -> int
	js.Global().Set("drumOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.Offset)
	}))
	// drumLength() -> int
	js.Global().Set("drumLength", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.Length)
	}))
	// rowVolume(row) -> float
	js.Global().Set("rowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0.0)
		}
		if len(args) < 1 {
			return js.ValueOf(0.0)
		}
		r := args[0].Int()
		if r < 0 || r >= len(g.drum.Rows) {
			return js.ValueOf(0.0)
		}
		return js.ValueOf(g.drum.Rows[r].Volume)
	}))
	js.Global().Set("mainVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(audio.MainVolume())
	}))
	js.Global().Set("setMainVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		v := args[0].Float()
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		audio.SetMainVolume(v)
		if g.drum != nil && g.drum.mainVolSlider != nil {
			g.drum.mainVolSlider.Value = v
		}
		return nil
	}))
	// setRowVolume(row, value)
	js.Global().Set("setRowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 2 {
			return nil
		}
		r := args[0].Int()
		v := args[1].Float()
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if r < 0 || r >= len(g.drum.Rows) {
			return nil
		}
		g.drum.Rows[r].Volume = v
		if r < len(g.drum.rowVolSliders) {
			g.drum.rowVolSliders[r].Value = v
		}
		return nil
	}))
	// timelineBeats() -> int
	js.Global().Set("timelineBeats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.timelineBeats)
	}))
	// setTimelineBeats(n)
	js.Global().Set("setTimelineBeats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		n := args[0].Int()
		if n < g.drum.Length {
			n = g.drum.Length
		}
		g.drum.timelineBeats = n
		return nil
	}))
	// setDrumLength(n)
	js.Global().Set("setDrumLength", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		n := args[0].Int()
		if n < 1 {
			n = 1
		}
		g.drum.SetLength(n)
		return nil
	}))
	// clickTimelineAt(frac) simulates a click on the timeline at a fractional
	// position [0,1], updating the drum offset using the same centering logic
	// as the UI.
	js.Global().Set("clickTimelineAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		f := args[0].Float()
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		total := g.drum.timelineBeats
		if total < g.drum.Length {
			total = g.drum.Length
		}
		center := int(f * float64(total))
		desired := center - g.drum.Length/2
		maxOff := g.drum.timelineBeats - g.drum.Length
		if maxOff < 0 {
			maxOff = 0
		}
		if desired < 0 {
			desired = 0
		}
		if desired > maxOff {
			desired = maxOff
		}
		g.drum.Offset = desired
		js.Global().Get("console").Call("log", "[WASM] clickTimelineAt", f, "-> Offset:", desired)
		return nil
	}))
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
