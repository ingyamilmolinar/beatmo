package ui

import (
	"image"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// updateNodeMenuRects recomputes the popup rectangles around the selected node.
func (g *Game) updateNodeMenuRects() {
	g.nodeMenuRects = map[string]image.Rectangle{}
	if !g.nodeMenuOpen || g.nodeMenuNode == nil {
		return
	}
	x1, y1, x2, _ := g.nodeScreenRect(g.nodeMenuNode)
	// Initial placement to the right of the node; final clamping/adjustment
	// happens after panel size is known.
	px := int(x2) + 8
	py := int(y1)
	// Compact controls sizing
	panelW := 220
	bW, bH := 18, 16
	gap := 4
	pad := 6
	// Start at content origin
	rowY := py + pad
	// Right-aligned +/- clusters for numeric rows
	btnX := px + panelW - (2*bW + gap + pad)
	nodeType := model.NodeTypeRegular
	if n, ok := g.graph.GetNodeByID(g.nodeMenuNode.ID); ok {
		nodeType = n.Type
	}
	isSilent := (nodeType == model.NodeTypeSilent)
	isMute := (nodeType == model.NodeTypeMute)
	// Hide most rows when Silent; for Mute nodes we still expose logic and
	// groove controls but omit volume/pitch/duration (they do not play audio).
	showVol := !(isSilent || isMute)
	showPitch := !(isSilent || isMute)
	showGroove := !isSilent
	showDuration := !(isSilent || isMute)
	showLogic := !isSilent
	if !showGroove {
		g.nodeGrooveOpen = false
	}
	if !showLogic {
		g.nodeLogicOpen = false
	}
	// Volume row
	if showVol {
		g.nodeMenuRects["vol-"] = image.Rect(btnX, rowY, btnX+bW, rowY+bH)
		g.nodeMenuRects["vol+"] = image.Rect(btnX+bW+gap, rowY, btnX+2*bW+gap, rowY+bH)
		rowY += bH + gap
	} else {
		g.nodeMenuRects["vol-"] = image.Rectangle{}
		g.nodeMenuRects["vol+"] = image.Rectangle{}
	}
	// Pitch row
	if showPitch {
		g.nodeMenuRects["pit-"] = image.Rect(btnX, rowY, btnX+bW, rowY+bH)
		g.nodeMenuRects["pit+"] = image.Rect(btnX+bW+gap, rowY, btnX+2*bW+gap, rowY+bH)
		rowY += bH + gap
	} else {
		g.nodeMenuRects["pit-"] = image.Rectangle{}
		g.nodeMenuRects["pit+"] = image.Rectangle{}
	}
	// Duration row
	if showDuration {
		g.nodeMenuRects["dur-"] = image.Rect(btnX, rowY, btnX+bW, rowY+bH)
		g.nodeMenuRects["dur+"] = image.Rect(btnX+bW+gap, rowY, btnX+2*bW+gap, rowY+bH)
		rowY += bH + gap
	} else {
		g.nodeMenuRects["dur-"] = image.Rectangle{}
		g.nodeMenuRects["dur+"] = image.Rectangle{}
	}
	// Logic row: selector button aligned to the right cluster (like +/-)
	logicBtnX := btnX
	if showLogic {
		g.nodeMenuRects["logic"] = image.Rect(logicBtnX, rowY, logicBtnX+2*bW+gap, rowY+bH)
	} else {
		g.nodeMenuRects["logic"] = image.Rectangle{}
	}
	// If open, place dropdown items below logic and advance rowY accordingly
	if showLogic && g.nodeLogicOpen {
		opts := []struct{ label, kind string }{
			{"None", ""},
			{"Trigger Every N", "every_n_triggers"},
			{"Skip Every N", "skip_every_n"},
			{"Probability", "probability"},
			{"Trigger If Prev Skipped", "trigger_if_prev_skipped"},
			{"Trigger If Prev Triggered", "trigger_if_prev_triggered"},
		}
		base := g.nodeMenuRects["logic"]
		sideX := px + panelW + gap
		for i, o := range opts {
			id := "logic:" + o.kind
			y0 := base.Min.Y + i*(bH+2)
			y1 := y0 + bH
			g.nodeMenuRects[id] = image.Rect(sideX, y0, sideX+180, y1)
		}
	}
	// Parameter adjusters for logic kind (mutually exclusive placement)
	lnRectMinus := image.Rect(btnX, rowY+bH+gap, btnX+bW, rowY+bH+gap+bH)
	lnRectPlus := image.Rect(btnX+bW+gap, rowY+bH+gap, btnX+2*bW+gap, rowY+bH+gap+bH)
	// Detect current kind for this node to expose only the appropriate controls
	kind := ""
	if g.nodeMenuNode != nil {
		if mn, ok := g.graph.GetNodeByID(g.nodeMenuNode.ID); ok {
			kind = mn.Params.LogicKind
		}
	}
	if showLogic && (kind == "every_n_triggers" || kind == "skip_every_n") {
		g.nodeMenuRects["ln-"] = lnRectMinus
		g.nodeMenuRects["ln+"] = lnRectPlus
		g.nodeMenuRects["lp-"] = image.Rectangle{}
		g.nodeMenuRects["lp+"] = image.Rectangle{}
	} else if showLogic && kind == "probability" {
		g.nodeMenuRects["ln-"] = image.Rectangle{}
		g.nodeMenuRects["ln+"] = image.Rectangle{}
		g.nodeMenuRects["lp-"] = lnRectMinus
		g.nodeMenuRects["lp+"] = lnRectPlus
	} else {
		// No parameters
		g.nodeMenuRects["ln-"] = image.Rectangle{}
		g.nodeMenuRects["ln+"] = image.Rectangle{}
		g.nodeMenuRects["lp-"] = image.Rectangle{}
		g.nodeMenuRects["lp+"] = image.Rectangle{}
	}
	// Advance after parameter row slot
	if showLogic {
		rowY += 2*bH + gap
	}
	// Groove row: selector + percentage +/- (compact width)
	if showGroove {
		g.nodeMenuRects["grv"] = image.Rect(btnX, rowY, btnX+2*bW+gap, rowY+bH)
		rowY += bH + gap
		g.nodeMenuRects["gp-"] = image.Rect(btnX, rowY, btnX+bW, rowY+bH)
		g.nodeMenuRects["gp+"] = image.Rect(btnX+bW+gap, rowY, btnX+2*bW+gap, rowY+bH)
		rowY += bH + gap
	} else {
		g.nodeMenuRects["grv"] = image.Rectangle{}
		g.nodeMenuRects["gp-"] = image.Rectangle{}
		g.nodeMenuRects["gp+"] = image.Rectangle{}
	}
	// Audible toggle button at bottom-right, aligned cluster width
	g.nodeMenuRects["aud"] = image.Rect(btnX, rowY, btnX+2*bW+gap, rowY+bH)
	rowY += bH + gap
	// Compute final panel rect to contain all content
	panelH := (rowY - py) + pad
	// Record initial panel rect
	g.nodeMenuRects["panel"] = image.Rect(px, py, px+panelW, py+panelH)

	// Clamp panel fully inside the top grid pane (screen-space: y in [0, split.Y])
	newPx := px
	newPy := py
	maxX := g.winW - panelW
	maxY := g.split.Y - panelH
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	// If the default right-of-node placement overflows, prefer left-of-node.
	if newPx > maxX {
		left := int(x1) - 8 - panelW
		if left >= 0 {
			newPx = left
		}
	}
	if newPx < 0 {
		newPx = 0
	}
	if newPx > maxX {
		newPx = maxX
	}
	// Vertically align to node top but keep fully visible.
	if newPy < 0 {
		newPy = 0
	}
	if newPy > maxY {
		newPy = maxY
	}
	// Apply translation delta to all rects if placement changed.
	if newPx != px || newPy != py {
		dx := newPx - px
		dy := newPy - py
		for id, r := range g.nodeMenuRects {
			if r.Empty() {
				continue
			}
			g.nodeMenuRects[id] = image.Rect(r.Min.X+dx, r.Min.Y+dy, r.Max.X+dx, r.Max.Y+dy)
		}
	}
	// Final clamp for panel height: if the panel cannot fit, clamp its rect to the top pane bounds
	if r, ok := g.nodeMenuRects["panel"]; ok {
		if r.Dy() > g.split.Y {
			g.nodeMenuRects["panel"] = image.Rect(newPx, 0, newPx+panelW, g.split.Y)
		} else {
			// ensure within bounds
			if r.Min.Y < 0 || r.Max.Y > g.split.Y || r.Min.X < 0 || r.Max.X > g.winW {
				minY := r.Min.Y
				if minY < 0 {
					minY = 0
				}
				maxY := r.Max.Y
				if maxY > g.split.Y {
					maxY = g.split.Y
				}
				minX := r.Min.X
				if minX < 0 {
					minX = 0
				}
				maxX := r.Max.X
				if maxX > g.winW {
					maxX = g.winW
				}
				g.nodeMenuRects["panel"] = image.Rect(minX, minY, maxX, maxY)
			}
		}
	}

	// Ensure button objects exist with decoupled handlers.
	if g.nodeMenuBtns == nil {
		g.nodeMenuBtns = map[string]*Button{}
	}
	node := g.nodeMenuNode
	mk := func(id, label string, onClick func()) {
		r := g.nodeMenuRects[id]
		if r.Empty() {
			delete(g.nodeMenuBtns, id)
			return
		}
		if _, ok := g.nodeMenuBtns[id]; !ok {
			g.nodeMenuBtns[id] = NewButton(label, PopupButtonStyle, func() { g.enqueueUI(onClick); g.nodeMenuAnim[id] = 1 })
		} else {
			g.nodeMenuBtns[id].OnClick = func() { g.enqueueUI(onClick); g.nodeMenuAnim[id] = 1 }
		}
		g.nodeMenuBtns[id].SetRect(r)
		g.nodeMenuBtns[id].ConsumeOnPress = true
	}
	// Wire actions: remove UI clamps; allow free adjustment.
	mk("vol-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume -= 0.10
			if p.Volume < 0 {
				p.Volume = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("vol+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			if p.Volume == 0 {
				p.Volume = 1
			}
			p.Volume += 0.10
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("pit-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch -= 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("pit+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch += 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("dur-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration -= 0.1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("dur+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration += 0.1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	// Logic controls
	mk("logic", "LOG", func() { g.nodeLogicOpen = !g.nodeLogicOpen })
	// Menu items (created when open). Provide handlers.
	if g.nodeLogicOpen {
		opts := []struct{ label, kind string }{
			{"None", ""},
			{"Trigger Every N", "every_n_triggers"},
			{"Skip Every N", "skip_every_n"},
			{"Probability", "probability"},
			{"Trigger If Prev Skipped", "trigger_if_prev_skipped"},
			{"Trigger If Prev Triggered", "trigger_if_prev_triggered"},
		}
		for _, o := range opts {
			kind := o.kind
			mk("logic:"+kind, o.label, func() {
				if mn, ok := g.graph.GetNodeByID(node.ID); ok {
					p := mn.Params
					p.LogicKind = kind
					if kind == "every_n_triggers" || kind == "skip_every_n" {
						if p.LogicN <= 0 {
							p.LogicN = 2
						}
					}
					if kind == "probability" {
						if p.LogicP <= 0 {
							p.LogicP = 0.5
						}
					}
					g.graph.SetNodeParams(node.ID, p)
					g.notifyPredictorNode(node.ID)
				}
				g.nodeLogicOpen = false
				if g.engine != nil && g.engine.Predictor != nil {
					g.engine.Predictor.RebaseAt(g.elapsedBeats)
				}
			})
		}
	}
	// Parameter adjusters: no clamps
	mk("ln-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicN--
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk("ln+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicN++
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk("lp-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicP -= 0.1
			if p.LogicP < 0 {
				p.LogicP = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk("lp+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicP += 0.1
			if p.LogicP > 1 {
				p.LogicP = 1
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	mk("grv", "GRV", func() { g.nodeGrooveOpen = !g.nodeGrooveOpen })
	if g.nodeGrooveOpen {
		opts := []struct{ label, kind string }{
			{"None", ""}, {"Delay", "delay"}, {"Rush", "rush"},
		}
		base := g.nodeMenuRects["grv"]
		sideX := px + panelW + gap
		for i, o := range opts {
			id := "groove:" + o.kind
			y0 := base.Min.Y + i*(bH+2)
			y1 := y0 + bH
			r := image.Rect(sideX, y0, sideX+140, y1)
			g.nodeMenuRects[id] = r
			kind := o.kind
			mk(id, o.label, func() {
				if mn, ok := g.graph.GetNodeByID(node.ID); ok {
					p := mn.Params
					p.GrooveKind = kind
					if p.GroovePct < 0 {
						p.GroovePct = 0
					}
					if p.GroovePct > 1 {
						p.GroovePct = 1
					}
					g.graph.SetNodeParams(node.ID, p)
					g.notifyPredictorNode(node.ID)
				}
				g.nodeGrooveOpen = false
			})
		}
	}
	// Groove percentage +/-
	mk("gp-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.GroovePct -= 0.05
			if p.GroovePct < 0 {
				p.GroovePct = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("gp+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.GroovePct += 0.05
			if p.GroovePct > 1 {
				p.GroovePct = 1
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
		}
	})
	mk("aud", "AUD", func() {
		if n, ok := g.graph.GetNodeByID(node.ID); ok {
			switch n.Type {
			case model.NodeTypeSilent:
				n.Type = model.NodeTypeMute
			case model.NodeTypeMute:
				n.Type = model.NodeTypeRegular
			default:
				n.Type = model.NodeTypeSilent
			}
			g.graph.Nodes[node.ID] = n
			g.cacheNode(node.ID)
			g.notifyPredictorNode(node.ID)
			g.updateBeatInfos()
			// Refresh popup layout so rows hide/show immediately when toggling
			// between Regular/Silent/Mute without requiring another hover/drag.
			g.updateNodeMenuRects()
		}
	})
}
