//go:build js && !test

package ui

import (
	"math"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func (g *Game) initJSHarness() {
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

	// visibleRows() -> int
	js.Global().Set("visibleRows", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.visibleRows())
	}))

	// drumRowCount() -> int
	js.Global().Set("drumRowCount", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(len(g.drum.Rows))
	}))

	// rowsContentVisibleCount() -> int : counts visible rows with any content drawn
	js.Global().Set("rowsContentVisibleCount", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		vis := g.drum.visibleRows()
		cnt := 0
		for i := g.drum.rowOffset; i < g.drum.rowOffset+vis && i < len(g.drum.Rows); i++ {
			if i >= 0 && i < len(g.drum.rowsDrawnMask) && g.drum.rowsDrawnMask[i] {
				cnt++
			}
		}
		return js.ValueOf(cnt)
	}))

	// uiLayoutOk() – sanity check to catch catastrophic UI layout regressions.
	js.Global().Set("uiLayoutOk", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g == nil || g.drum == nil {
			return js.ValueOf(false)
		}
		ok := true
		if g.drum.Bounds.Dx() <= 0 || g.drum.Bounds.Dy() <= 0 {
			ok = false
		}
		if g.drum.timelineRect.Dx() <= 0 || g.drum.timelineRect.Dy() <= 0 {
			ok = false
		}
		// Require at least one drawable rows representation.
		if g.drum.rowsStripingEnabled && g.drum.rowsStripeCount > 1 {
			if len(g.drum.rowsStripes) == 0 {
				ok = false
			}
		} else {
			if g.drum.rowsLayer == nil {
				ok = false
			}
		}
		return js.ValueOf(ok)
	}))

	// buildPerfRect(rows, side) builds 'rows' disjoint 1-rectangle loops
	// each with edges of length 'side' grid units to stress scheduling.
	js.Global().Set("buildPerfRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		start := time.Now()
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
		if !g.perf.started.IsZero() {
			g.perf.started = g.perf.started.Add(time.Since(start))
		}
		return nil
	}))

	// setNodeLogicCallbackGrid(i, j, kind) – attach a built-in NodeLogic callback
	// for browser harnesses. Kinds: "param_boost", "param_drop", "disable_even", "none".
	js.Global().Set("setNodeLogicCallbackGrid", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 3 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		kind := args[2].String()
		node := g.nodeAt(i, j)
		if node == nil {
			return nil
		}
		if kind == "" || kind == "none" {
			g.graph.SetNodeLogic(node.ID, nil)
			return nil
		}
		var logic model.NodeLogic
		switch kind {
		case "param_boost":
			logic = func(ctx model.NodeContext) model.NodeDecision {
				return model.NodeDecision{VolumeMul: 0.5, PitchDelta: 3, DurationMul: 2}
			}
		case "param_drop":
			logic = func(ctx model.NodeContext) model.NodeDecision {
				return model.NodeDecision{VolumeMul: 0.25, PitchDelta: -2, DurationMul: 0.5}
			}
		case "disable_even":
			logic = func(ctx model.NodeContext) model.NodeDecision {
				if ctx.TriggerCount%2 == 0 {
					disabled := false
					return model.NodeDecision{Enabled: &disabled}
				}
				return model.NodeDecision{}
			}
		default:
			return nil
		}
		g.graph.SetNodeLogic(node.ID, logic)
		return nil
	}))

	js.Global().Set("buildLine", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		targetRow := 0
		if len(args) > 0 {
			targetRow = args[0].Int()
		}
		length := 4
		if len(args) > 1 && args[1].Int() > 1 {
			length = args[1].Int()
		}
		start := g.grid.MaxDiv()
		if len(args) > 2 {
			start = args[2].Int()
		}
		step := g.grid.MaxDiv()
		if step <= 0 {
			step = 1
		}
		for len(g.drum.Rows) <= targetRow {
			g.drum.AddRow()
		}
		g.pendingStartRow = targetRow
		prev := g.tryAddNode(start, targetRow, model.NodeTypeRegular)
		g.pendingStartRow = -1
		last := prev
		for i := 1; i < length; i++ {
			next := g.tryAddNode(start+i*step, targetRow, model.NodeTypeRegular)
			g.addEdge(last, next)
			last = next
		}
		g.updateBeatInfos()
		return js.ValueOf(step)
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
}
