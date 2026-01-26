//go:build js && !test

package ui

import (
	"math"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func (g *Game) initJSGraphUI() {
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
		return rectToJS(r)
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
		g.scheduleSound(row, 0, info, inst, 1.0, pitch, dur, math.NaN(), false)
		return js.ValueOf(true)
	}))

	// nodeParams(i,j) -> {volume, pitch, duration, logicKind, logicN, logicP, type}
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
			n2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
			n3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
			g.addEdge(n0, n1)
			g.addEdge(n1, n2)
			g.addEdge(n2, n3)
			g.addEdge(n3, n0)
			g.pendingStartRow = -1
			g.start = n0
			g.graph.StartNodeID = n0.ID
			g.updateBeatInfos()
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
		return rectToJS(g.drum.rowVolSliders[row].Rect())
	}))

	// timelineRect() -> {x,y,w,h}
	js.Global().Set("timelineRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		return rectToJS(g.drum.timelineRect)
	}))

	// bpmBoxRect() -> {x,y,w,h}
	js.Global().Set("bpmBoxRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmBox == nil {
			return nil
		}
		return rectToJS(g.drum.bpmBox.Rect)
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
		return rectToJS(g.drum.Bounds)
	}))

	// scrollBarRect() / scrollThumbRect() -> {x,y,w,h}
	js.Global().Set("scrollBarRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		return rectToJS(g.drum.scrollBarRect())
	}))

	js.Global().Set("scrollThumbRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		return rectToJS(g.drum.scrollThumbRect())
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

	js.Global().Set("setRowInstrument", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 2 {
			return nil
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.drum.Rows) {
			return nil
		}
		id := args[1].String()
		prev := g.drum.selRow
		g.drum.selRow = row
		g.drum.SetInstrument(id)
		g.drum.selRow = prev
		return nil
	}))

	js.Global().Set("setRowStep", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 3 {
			return nil
		}
		row := args[0].Int()
		idx := args[1].Int()
		val := args[2].Bool()
		if row < 0 || row >= len(g.drum.Rows) {
			return nil
		}
		steps := g.drum.Rows[row].Steps
		if idx < 0 || idx >= len(steps) {
			return nil
		}
		steps[idx] = val
		g.drum.markRowDirty(row)
		return nil
	}))

	// rowLabelRect(row), rowEditBtnRect(row), rowColorBtnRect(row) -> {x,y,w,h}
	js.Global().Set("rowLabelRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowLabels) {
			return nil
		}
		return rectToJS(g.drum.rowLabels[i].Rect())
	}))

	js.Global().Set("rowEditBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowEditBtns) {
			return nil
		}
		return rectToJS(g.drum.rowEditBtns[i].Rect())
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
		return rectToJS(g.drum.rowColorBtns[i].Rect())
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
		return rectToJS(g.drum.rowMuteBtns[i].Rect())
	}))

	js.Global().Set("rowSoloBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		i := args[0].Int()
		if i < 0 || i >= len(g.drum.rowSoloBtns) {
			return nil
		}
		return rectToJS(g.drum.rowSoloBtns[i].Rect())
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
		return rectToJS(g.drum.colorWheelRect)
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
		return rectToJS(g.drum.subdivBtn.Rect())
	}))

	// openSubdivMenu()
	js.Global().Set("openSubdivMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		g.drum.subdivMenuOpen = true
		g.drum.buildSubdivMenu()
		return nil
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

	// applySubdivValue(val int) – simulate selecting a subdivision menu item.
	js.Global().Set("applySubdivValue", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || len(args) < 1 {
			return nil
		}
		target := args[0].Int()
		for _, btn := range g.drum.subdivMenuBtns {
			v, _ := strconv.Atoi(btn.Text)
			if v == target {
				if btn.OnClick != nil {
					btn.OnClick()
				}
				break
			}
		}
		return nil
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
		return rectToJS(g.drum.uploadBtn.Rect())
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
		return rectToJS(g.drum.renameBox.Rect)
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
		g.drum.renameBox.MaxLen = 32
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
		return js.ValueOf(g.AppliedBPM())
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
		// Pause auto-tracking briefly so seeking doesn't fight TrackBeat.
		g.state.SetSeekFreezeFrames(30)
		js.Global().Get("console").Call("log", "[WASM] clickTimelineAt", f, "-> Offset:", desired)
		return nil
	}))

	// ─────────────────────────────────────────────────────────────────────────
	// State verification exports for real input tests
	// ─────────────────────────────────────────────────────────────────────────

	// nodeMenuOpen() -> bool
	js.Global().Set("nodeMenuOpen", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.nodeMenuOpen)
	}))

	// nodeMenuNodeId() -> int (-1 if none)
	js.Global().Set("nodeMenuNodeId", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.nodeMenuNode == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(g.nodeMenuNode.ID))
	}))

	// selectedNodeId() -> int (-1 if none)
	js.Global().Set("selectedNodeId", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.sel == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(g.sel.ID))
	}))

	// playBtnRect() -> {x,y,w,h}
	js.Global().Set("playBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.playBtn == nil {
			return nil
		}
		return rectToJS(g.drum.playBtn.Rect())
	}))

	// stopBtnRect() -> {x,y,w,h}
	js.Global().Set("stopBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.stopBtn == nil {
			return nil
		}
		return rectToJS(g.drum.stopBtn.Rect())
	}))

	// bpmIncBtnRect() -> {x,y,w,h}
	js.Global().Set("bpmIncBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmIncBtn == nil {
			return nil
		}
		return rectToJS(g.drum.bpmIncBtn.Rect())
	}))

	// bpmDecBtnRect() -> {x,y,w,h}
	js.Global().Set("bpmDecBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.bpmDecBtn == nil {
			return nil
		}
		return rectToJS(g.drum.bpmDecBtn.Rect())
	}))

	// lenIncBtnRect() -> {x,y,w,h}
	js.Global().Set("lenIncBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.lenIncBtn == nil {
			return nil
		}
		return rectToJS(g.drum.lenIncBtn.Rect())
	}))

	// lenDecBtnRect() -> {x,y,w,h}
	js.Global().Set("lenDecBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.lenDecBtn == nil {
			return nil
		}
		return rectToJS(g.drum.lenDecBtn.Rect())
	}))

	// addRowBtnRect() -> {x,y,w,h}
	js.Global().Set("addRowBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.addRowBtn == nil {
			return nil
		}
		return rectToJS(g.drum.addRowBtn.Rect())
	}))

	// importBtnRect() -> {x,y,w,h}
	js.Global().Set("importBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.importBtn == nil {
			return nil
		}
		return rectToJS(g.drum.importBtn.Rect())
	}))

	// exportBtnRect() -> {x,y,w,h}
	js.Global().Set("exportBtnRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil || g.drum.exportBtn == nil {
			return nil
		}
		return rectToJS(g.drum.exportBtn.Rect())
	}))

	// instMenuOpen() -> bool
	js.Global().Set("instMenuOpenState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.drum.instMenuOpen)
	}))

	// colorMenuOpen() -> bool
	js.Global().Set("colorMenuOpenState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(false)
		}
		return js.ValueOf(g.drum.colorMenuOpen)
	}))

	// isPlaying() -> bool
	js.Global().Set("isPlaying", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.Playing())
	}))

	// closeNodeMenu() - explicitly close the node popup
	js.Global().Set("closeNodeMenu", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.nodeMenuOpen = false
		g.nodeMenuNode = nil
		return nil
	}))

	// ─────────────────────────────────────────────────────────────────────────
	// Debug exports for grid input bug testing (fastPath issue)
	// ─────────────────────────────────────────────────────────────────────────

	// getFastPath() -> bool - returns current fastPath status
	js.Global().Set("getFastPath", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.perfMode.FastPathEnabled())
	}))

	// setFastPath(enabled) - toggle fastPath for testing
	js.Global().Set("setFastPath", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		enabled := args[0].Bool()
		g.perfMode.SetFastPath(enabled)
		return nil
	}))

	// getLeftPrev() -> bool - returns g.leftPrev state
	js.Global().Set("getLeftPrev", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.leftPrev)
	}))

	// getPendingClick() -> bool - returns g.pendingClick state
	js.Global().Set("getPendingClick", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return js.ValueOf(g.pendingClick)
	}))

	// getClickNode() -> int - returns g.clickNode ID (-1 if nil)
	js.Global().Set("getClickNode", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.clickNode == nil {
			return js.ValueOf(-1)
		}
		return js.ValueOf(int(g.clickNode.ID))
	}))

	// debugGridInputState() -> object with all input state fields
	js.Global().Set("debugGridInputState", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		obj := js.Global().Get("Object").New()
		obj.Set("fastPath", g.perfMode.FastPathEnabled())
		obj.Set("leftPrev", g.leftPrev)
		obj.Set("pendingClick", g.pendingClick)
		if g.clickNode != nil {
			obj.Set("clickNodeId", int(g.clickNode.ID))
		} else {
			obj.Set("clickNodeId", -1)
		}
		obj.Set("clickI", g.clickI)
		obj.Set("clickJ", g.clickJ)
		obj.Set("nodeMenuOpen", g.nodeMenuOpen)
		if g.nodeMenuNode != nil {
			obj.Set("nodeMenuNodeId", int(g.nodeMenuNode.ID))
		} else {
			obj.Set("nodeMenuNodeId", -1)
		}
		if g.sel != nil {
			obj.Set("selectedNodeId", int(g.sel.ID))
		} else {
			obj.Set("selectedNodeId", -1)
		}
		obj.Set("camDragging", g.camDragging)
		obj.Set("camDragged", g.camDragged)
		return obj
	}))
}
