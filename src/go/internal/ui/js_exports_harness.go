//go:build js && !test

package ui

import (
	"math"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
		wy := (y - float64(gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
		constZoomFactor := 1.05
		constSens := 0.1
		newScale := g.cam.Scale * math.Pow(constZoomFactor, delta*constSens)
		if newScale < 0.1 {
			newScale = 0.1
		} else if newScale > 10.0 {
			newScale = 10.0
		}
		g.cam.OffsetX = x - wx*newScale
		g.cam.OffsetY = y - float64(gridTopOffset()) - wy*newScale
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
		// On small screens, the direct draw path bypasses rowsLayer/stripes.
		if isSmallScreen() && g.drum.directDrawCount > 0 {
			// Direct draw path is active — no layer/stripe needed.
		} else if g.drum.rowsStripingEnabled && g.drum.rowsStripeCount > 1 {
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

	// setCamScale(s) – set camera scale directly.
	js.Global().Set("setCamScale", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		g.cam.Scale = args[0].Float()
		g.cam.Snap()
		return nil
	}))

	// setCamOffset(x, y) – set camera offset directly and snap to pixels.
	js.Global().Set("setCamOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		g.cam.OffsetX = args[0].Float()
		g.cam.OffsetY = args[1].Float()
		g.cam.Snap()
		return nil
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

	// gridToScreen(i,j) -> {x,y} screen coordinates for a grid position,
	// even if no node exists there. Useful for clicking empty grid cells.
	js.Global().Set("gridToScreen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		i := args[0].Int()
		j := args[1].Int()
		unitPx := g.grid.UnitPixels(g.cam.Scale)
		offX := math.Round(g.cam.OffsetX)
		offY := math.Round(g.cam.OffsetY)
		sx := offX + unitPx*float64(i)
		sy := offY + unitPx*float64(j) + float64(gridTopOffset())
		obj := js.Global().Get("Object").New()
		obj.Set("x", int(sx))
		obj.Set("y", int(sy))
		return obj
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
		g.sidebar.Open(n)
		return nil
	}))

	// centerCamera() resets the centered flag and re-runs Layout so the camera
	// re-centers on the current splitY. Useful after the demo circuit changes
	// splitY post-init.
	js.Global().Set("centerCamera", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.centered = false
		g.Layout(g.winW, g.winH)
		return nil
	}))

	// zoomBtnRects() retained for backward compatibility; grid zoom buttons
	// are disabled in this UI. Return an empty object to satisfy callers.
	js.Global().Set("zoomBtnRects", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.Global().Get("Object").New()
	}))

	// instrumentsList() -> string[] : returns the Go-side instrument ID list.
	js.Global().Set("instrumentsList", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ids := audio.Instruments()
		arr := js.Global().Get("Array").New(len(ids))
		for i, id := range ids {
			arr.SetIndex(i, id)
		}
		return arr
	}))

	// debugDrumLayout() -> object with all critical rendering state for mobile diagnosis.
	js.Global().Set("debugDrumLayout", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(nil)
		}
		dv := g.drum
		obj := js.Global().Get("Object").New()
		obj.Set("bounds", rectToJS(dv.Bounds))
		obj.Set("headerH", dv.headerH)
		obj.Set("eqH", dv.eqH)
		obj.Set("rowsAreaHeight", dv.rowsAreaHeight())
		obj.Set("visibleRows", dv.visibleRows())
		obj.Set("timelineRect", rectToJS(dv.timelineRect))
		obj.Set("rowHeight", dv.rowHeight())
		obj.Set("rowOffset", dv.rowOffset)
		obj.Set("rowsStripingEnabled", dv.rowsStripingEnabled)
		obj.Set("isSmallScreen", isSmallScreen())
		obj.Set("touchScreenWidth", touchScreenWidth)
		obj.Set("touchScreenHeight", touchScreenHeight)
		obj.Set("numRows", len(dv.Rows))
		obj.Set("numRowCache", len(dv.rowCache))
		obj.Set("rowsLayerExists", dv.rowsLayer != nil)
		if dv.rowsLayer != nil {
			obj.Set("rowsLayerW", dv.rowsLayerW)
			obj.Set("rowsLayerH", dv.rowsLayerH)
		}
		obj.Set("rowsLayerDirty", dv.rowsLayerDirty)
		obj.Set("rowCacheW", dv.rowCacheW)
		obj.Set("rowCacheH", dv.rowCacheH)
		obj.Set("rowsStripeCount", dv.rowsStripeCount)
		obj.Set("numRowsStripes", len(dv.rowsStripes))
		obj.Set("directDrawCount", int(dv.directDrawCount))
		obj.Set("directDrawCells", dv.directDrawCells)

		// Per-row detail
		rowsArr := js.Global().Get("Array").New(len(dv.Rows))
		for i := range dv.Rows {
			r := js.Global().Get("Object").New()
			if i < len(dv.rowDirty) {
				r.Set("dirty", dv.rowDirty[i])
			}
			if i < len(dv.rowFullDirty) {
				r.Set("fullDirty", dv.rowFullDirty[i])
			}
			r.Set("cacheExists", i < len(dv.rowCache) && dv.rowCache[i] != nil)
			r.Set("numSteps", len(dv.Rows[i].Steps))
			onCount := 0
			for _, s := range dv.Rows[i].Steps {
				if s {
					onCount++
				}
			}
			r.Set("stepsOn", onCount)
			if i < len(dv.rowsDrawnMask) {
				r.Set("drawn", dv.rowsDrawnMask[i])
			}
			rowsArr.SetIndex(i, r)
		}
		obj.Set("rows", rowsArr)
		return obj
	}))

	// debugDrumRender() -> object with per-frame render decision trace.
	// Call forceDraw() first, then immediately call debugDrumRender() to
	// inspect what happened in the most recent Draw().
	js.Global().Set("debugDrumRender", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(nil)
		}
		dv := g.drum
		obj := js.Global().Get("Object").New()
		obj.Set("frame", int(dv.frame))
		obj.Set("rowsLayerFrame", int(dv.rowsLayerFrame))
		obj.Set("rowsLayerDirty", dv.rowsLayerDirty)
		obj.Set("rowsLayerExists", dv.rowsLayer != nil)
		obj.Set("rowsStripingEnabled", dv.rowsStripingEnabled)
		obj.Set("rowsStripeCount", dv.rowsStripeCount)
		obj.Set("numRowsStripes", len(dv.rowsStripes))
		obj.Set("rowsRepaints", dv.rowsRepaints)
		obj.Set("rowsLayerBytes", dv.rowsLayerBytes)
		obj.Set("visibleRows", dv.visibleRows())
		obj.Set("rowsAreaHeight", dv.rowsAreaHeight())
		obj.Set("directDrawCount", int(dv.directDrawCount))
		obj.Set("directDrawCells", dv.directDrawCells)

		// Check how many visible rows were drawn
		vis := dv.visibleRows()
		drawn := 0
		for i := dv.rowOffset; i < dv.rowOffset+vis && i < len(dv.Rows); i++ {
			if i >= 0 && i < len(dv.rowsDrawnMask) && dv.rowsDrawnMask[i] {
				drawn++
			}
		}
		obj.Set("drawnRows", drawn)
		return obj
	}))
}
