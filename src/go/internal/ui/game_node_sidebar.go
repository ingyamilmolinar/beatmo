package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ─── Sidebar sizing constants ──────────────────────────────────────────────
const (
	sidebarDefaultW = 260
	sidebarMinW     = 200
	sidebarMaxW     = 400
	sidebarBtnH     = 28 // unified button/row height
	sidebarIncBtnW  = 28 // inc/dec button width
	sidebarIncBtnH  = 24 // inc/dec button height
	sidebarGap      = 6
	sidebarPad      = 10
	sidebarHeaderH  = 36
	sidebarSectionH = 30 // collapsible section header height
	sidebarTabW     = 24 // collapsed expand-tab width
	sidebarInnerPad = 4  // inner padding for text within layout rects
	sidebarSwatchSz = 12 // instrument color swatch size in header
)

// sidebarLabelScale is the density-driven text-scale multiplier for sidebar
// labels (was a hardcoded 1.2 const). Comfortable preserves the historical
// 1.2; Compact tightens to 1.1, Spacious opens to 1.3. Read per-call so it
// re-derives every Layout/Draw, matching the other density tokens.
func sidebarLabelScale() float64 {
	return float64(Profile().DensityValues().SidebarLabelScale) / 1000
}

// sidebarScaledTextWidth returns the pixel width of s rendered at the given
// scale, using the same font metrics as DrawTextAtScale.
func sidebarScaledTextWidth(s string, scale float64) int {
	return int(float64(TextWidth(s)) * scale)
}

// NodeSidebar is a left-anchored panel for editing node properties.
// It replaces the floating popup and uses fixed sizes + scrolling.
type NodeSidebar struct {
	game *Game
	open bool
	node *uiNode

	// Geometry
	width     int  // current width (fixed; user-resize removed)
	collapsed bool // fully hidden (show expand tab only)

	// Sections — individually collapsible
	sectionOpen map[string]bool

	// Scroll — uses reusable ScrollBehavior with pixel-level mode (ItemHeight=1)
	scroll      *ScrollBehavior
	deferredTap DeferredTap // two-phase tap: prevents accidental toggles during scroll

	// Controls
	rects map[string]image.Rectangle
	btns  map[string]*Button
	anim  map[string]float64

	closedGuard int // frames to suppress tap-to-create after close

	// Dropdown state
	logicDropdownOpen  bool
	grooveDropdownOpen bool
}

// NewNodeSidebar creates a new sidebar attached to the game.
func NewNodeSidebar(g *Game) *NodeSidebar {
	return &NodeSidebar{
		game:        g,
		width:       sidebarDefaultW,
		sectionOpen: make(map[string]bool),
		scroll:      NewScrollBehavior(ScrollbarStyleForPlatform(), 1),
		rects:       make(map[string]image.Rectangle),
		btns:        make(map[string]*Button),
		anim:        make(map[string]float64),
	}
}

// Open activates the sidebar for the given node.
func (sb *NodeSidebar) Open(node *uiNode) {
	sb.open = true
	sb.node = node
	sb.collapsed = false
	sb.scroll.VS.First = 0
	sb.scroll.ResetTouch()
	sb.logicDropdownOpen = false
	sb.grooveDropdownOpen = false
	// Reset all sections to collapsed
	sb.sectionOpen = map[string]bool{}
	sb.deferredTap.Cancel()
	sb.rects = make(map[string]image.Rectangle)
	sb.btns = make(map[string]*Button)
	sb.anim = make(map[string]float64)
	sb.layout()
}

// Close deactivates the sidebar.
func (sb *NodeSidebar) Close() {
	sb.open = false
	sb.node = nil
	sb.closedGuard = 2
	sb.btns = make(map[string]*Button)
	sb.rects = make(map[string]image.Rectangle)
}

// IsOpen returns true if the sidebar is visible.
func (sb *NodeSidebar) IsOpen() bool { return sb.open }

// Node returns the currently-selected node, or nil.
func (sb *NodeSidebar) Node() *uiNode { return sb.node }

// ClosedGuard returns the current closed-guard frame counter.
func (sb *NodeSidebar) ClosedGuard() int { return sb.closedGuard }

// DecrementClosedGuard reduces the guard counter by one (called from game update).
func (sb *NodeSidebar) DecrementClosedGuard() {
	if sb.closedGuard > 0 {
		sb.closedGuard--
	}
}

// Hit reports whether a screen-space point lies within the sidebar panel.
func (sb *NodeSidebar) Hit(x, y int) bool {
	if sb.collapsed {
		r := sb.expandTabRect()
		return image.Pt(x, y).In(r)
	}
	if !sb.open || sb.node == nil {
		return false
	}
	if r, ok := sb.rects["panel"]; ok {
		return image.Pt(x, y).In(r)
	}
	return false
}

// ExpandAllSections opens all collapsible sections (for tests).
func (sb *NodeSidebar) ExpandAllSections() {
	for _, s := range []string{"vol", "pit", "dur", "logic", "groove", "aud", "move"} {
		sb.sectionOpen[s] = true
	}
}

// expandTabRect returns the rect for the collapsed expand tab.
func (sb *NodeSidebar) expandTabRect() image.Rectangle {
	gridH := sb.game.split.GridH(sb.game.winH)
	return image.Rect(0, 0, sidebarTabW, gridH)
}

// ─── Layout ────────────────────────────────────────────────────────────────

// layout recomputes all rects in screen space using fixed sizes.
func (sb *NodeSidebar) layout() {
	sb.rects = make(map[string]image.Rectangle)
	if !sb.open || sb.node == nil {
		return
	}
	g := sb.game
	gridH := g.split.GridH(g.winH)

	// Clamp width
	w := sb.width
	if w < sidebarMinW {
		w = sidebarMinW
	}
	if w > sidebarMaxW {
		w = sidebarMaxW
	}
	if w > g.winW {
		w = g.winW
	}
	sb.width = w

	// Panel rect: left-anchored, full grid height
	sb.rects["panel"] = image.Rect(0, 0, w, gridH)

	// Determine node type visibility
	nodeType := model.NodeTypeRegular
	if n, ok := g.graph.GetNodeByID(sb.node.ID); ok {
		nodeType = n.Type
	}
	isSilent := nodeType == model.NodeTypeSilent
	isMute := nodeType == model.NodeTypeMute
	showVol := !isSilent && !isMute
	showPitch := !isSilent && !isMute
	showDur := !isSilent && !isMute
	showLogic := !isSilent
	showGroove := !isSilent

	// Fixed header (NOT scrolled)
	closeSize := sidebarBtnH
	headerRect := image.Rect(sidebarPad, sidebarPad, w-sidebarPad, sidebarPad+sidebarHeaderH)
	sb.rects["header"] = headerRect
	sb.rects["close"] = image.Rect(w-sidebarPad-closeSize, sidebarPad, w-sidebarPad, sidebarPad+closeSize)

	// Content starts below header
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	viewportH := gridH - contentTop

	// Layout scrollable content
	y := contentTop - sb.scroll.VS.First
	btnX := w - sidebarPad - 2*sidebarBtnH - sidebarGap

	// Logic kind for parameter display
	kind := ""
	if mn, ok := g.graph.GetNodeByID(sb.node.ID); ok {
		kind = mn.Params.LogicKind
	}

	// Section: Volume
	if showVol {
		y = sb.layoutSection(y, w, btnX, "vol", i18n.T(i18n.KeyNodeSecVolume), true)
	}
	// Section: Pitch
	if showPitch {
		y = sb.layoutSection(y, w, btnX, "pit", i18n.T(i18n.KeyCapPitch), true)
	}
	// Section: Duration
	if showDur {
		y = sb.layoutSection(y, w, btnX, "dur", i18n.T(i18n.KeyNodeSecDuration), true)
	}
	// Section: Logic
	if showLogic {
		y = sb.layoutLogicSection(y, w, btnX, kind)
	}
	// Section: Groove
	if showGroove {
		y = sb.layoutGrooveSection(y, w, btnX)
	}
	// Section: Audible
	{
		sb.rects["sec-aud"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
		y += sidebarSectionH
		if sb.sectionOpen["aud"] {
			sb.rects["aud"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + sidebarGap
		}
	}
	// Section: Move (desktop only)
	if !Profile().IsMobile() {
		sb.rects["sec-move"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
		y += sidebarSectionH
		if sb.sectionOpen["move"] {
			sb.rects["move"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + sidebarGap
		}
	}

	// Compute scroll limits via ScrollBehavior
	contentH := y + sb.scroll.VS.First - contentTop
	sb.scroll.VS.Total = contentH
	sb.scroll.VS.Visible = viewportH
	sb.scroll.VS.View = image.Rect(w-sb.scroll.Style.Width, contentTop, w, contentTop+viewportH)
	sb.scroll.VS.Clamp()

	// Wire button objects
	sb.wireButtons()
}

// layoutSection lays out a simple +/- section (vol, pit, dur).
// Layout: [label] ... [−] [value pill] [+] right-aligned.
func (sb *NodeSidebar) layoutSection(y, w, btnX int, id, _ string, hasButtons bool) int {
	sb.rects["sec-"+id] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[id] {
		return y
	}
	if hasButtons {
		// Right-align: [−] [pill] [+]
		// +button at the far right
		plusX := w - sidebarPad - sidebarIncBtnW
		// pill between buttons
		pillW := Profile().DensityValues().SidebarValuePillW
		pillX := plusX - sidebarGap - pillW
		// −button to the left of pill
		minusX := pillX - sidebarGap - sidebarIncBtnW

		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2 // vertically center 24px buttons in 28px row
		sb.rects[id+"-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		sb.rects[id+"val"] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
		sb.rects[id+"+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
	}
	y += sidebarBtnH + sidebarGap
	return y
}

// layoutLogicSection lays out the logic section with dropdown and params.
func (sb *NodeSidebar) layoutLogicSection(y, w, btnX int, kind string) int {
	sb.rects["sec-logic"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen["logic"] {
		return y
	}
	// Logic selector button
	sb.rects["logic"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap

	if sb.logicDropdownOpen {
		opts := []string{"", "every_n_triggers", "skip_every_n", "probability", "trigger_if_prev_skipped", "trigger_if_prev_triggered"}
		for _, o := range opts {
			id := "logic:" + o
			sb.rects[id] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + 2
		}
		y += sidebarGap - 2
	} else {
		// Parameter adjusters — same [−] [pill] [+] layout as vol/pit/dur
		kindHasN := kind == "every_n_triggers" || kind == "skip_every_n"
		kindHasP := kind == "probability"
		if kindHasN {
			plusX := w - sidebarPad - sidebarIncBtnW
			pillW := Profile().DensityValues().SidebarValuePillW
			pillX := plusX - sidebarGap - pillW
			minusX := pillX - sidebarGap - sidebarIncBtnW
			btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
			sb.rects["ln-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			sb.rects["lnval"] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
			sb.rects["ln+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			y += sidebarBtnH + sidebarGap
		} else if kindHasP {
			plusX := w - sidebarPad - sidebarIncBtnW
			pillW := Profile().DensityValues().SidebarValuePillW
			pillX := plusX - sidebarGap - pillW
			minusX := pillX - sidebarGap - sidebarIncBtnW
			btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
			sb.rects["lp-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			sb.rects["lpval"] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
			sb.rects["lp+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			y += sidebarBtnH + sidebarGap
		}
	}
	return y
}

// layoutGrooveSection lays out the groove section with dropdown and pct.
func (sb *NodeSidebar) layoutGrooveSection(y, w, btnX int) int {
	sb.rects["sec-groove"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen["groove"] {
		return y
	}
	// Groove selector button
	sb.rects["grv"] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
	y += sidebarBtnH + sidebarGap

	if sb.grooveDropdownOpen {
		for _, kind := range []string{"", "delay", "rush"} {
			id := "groove:" + kind
			sb.rects[id] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarBtnH)
			y += sidebarBtnH + 2
		}
		y += sidebarGap - 2
	} else {
		// Pct adjusters — same [−] [pill] [+] layout
		plusX := w - sidebarPad - sidebarIncBtnW
		pillW := Profile().DensityValues().SidebarValuePillW
		pillX := plusX - sidebarGap - pillW
		minusX := pillX - sidebarGap - sidebarIncBtnW
		btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
		sb.rects["gp-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		sb.rects["gpval"] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
		sb.rects["gp+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
		y += sidebarBtnH + sidebarGap
	}
	return y
}

// ─── Button wiring ─────────────────────────────────────────────────────────

func (sb *NodeSidebar) wireButtons() {
	g := sb.game
	node := sb.node
	if node == nil {
		return
	}

	mk := func(id, label string, onClick func()) {
		r, ok := sb.rects[id]
		if !ok || r.Empty() {
			delete(sb.btns, id)
			return
		}
		if _, ok := sb.btns[id]; !ok {
			// Every node-sidebar control uses the shared menu keycap style
			// (DropdownStyle) so the node pop-up's buttons + dropdown lists read
			// identically to the instrument / context / subdivision menus. The
			// node's own color is layered on via tintBtnAccent (keycap face tint) and, for
			// dropdown lists, drawMenuRow's accent stripe.
			sb.btns[id] = NewButton(label, DropdownStyle, func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			})
		} else {
			sb.btns[id].Text = label
			sb.btns[id].OnClick = func() {
				g.enqueueUI(onClick)
				sb.anim[id] = 1
			}
		}
		sb.btns[id].SetRect(r)
		sb.btns[id].ConsumeOnPress = true
		sb.btns[id].TextScale = sidebarLabelScale()
		// +/− steppers use icon-based drawing (Button.Draw path) instead of the
		// legacy hand-drawn glyph so they get cushion + glow + rounded chrome.
		if strings.HasSuffix(id, "-") {
			sb.btns[id].Icon = "minus"
			sb.btns[id].Text = ""
			sb.btns[id].IconColor = colTextPrimary
		} else if strings.HasSuffix(id, "+") {
			sb.btns[id].Icon = "plus"
			sb.btns[id].Text = ""
			sb.btns[id].IconColor = colTextPrimary
		}
	}

	// Volume +/-
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
			emitNodeParamsChanged(node.ID, p)
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
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Pitch +/-
	mk("pit-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch -= 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk("pit+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Pitch += 1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Duration +/-
	mk("dur-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration -= 0.1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	mk("dur+", "+", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.Duration += 0.1
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Logic selector
	mk("logic", "", func() {
		sb.logicDropdownOpen = !sb.logicDropdownOpen
		if sb.logicDropdownOpen {
			sb.grooveDropdownOpen = false
		}
	})
	// Logic dropdown items
	if sb.logicDropdownOpen {
		opts := []struct{ label, kind string }{
			{i18n.T(i18n.KeyLogicNone), ""},
			{i18n.T(i18n.KeyLogicEveryN), "every_n_triggers"},
			{i18n.T(i18n.KeyLogicSkipN), "skip_every_n"},
			{i18n.T(i18n.KeyLogicProbability), "probability"},
			{i18n.T(i18n.KeyLogicIfPrevSkipped), "trigger_if_prev_skipped"},
			{i18n.T(i18n.KeyLogicIfPrevTriggered), "trigger_if_prev_triggered"},
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
					emitNodeParamsChanged(node.ID, p)
				}
				sb.logicDropdownOpen = false
				if g.engine != nil && g.engine.Predictor != nil {
					g.engine.Predictor.RebaseAt(g.elapsedBeats)
				}
			})
		}
	}
	// Logic N +/-
	mk("ln-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicN--
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
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
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	// Logic P +/-
	mk("lp-", "-", func() {
		if mn, ok := g.graph.GetNodeByID(node.ID); ok {
			p := mn.Params
			p.LogicP -= 0.1
			if p.LogicP < 0 {
				p.LogicP = 0
			}
			g.graph.SetNodeParams(node.ID, p)
			g.notifyPredictorNode(node.ID)
			emitNodeParamsChanged(node.ID, p)
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
			emitNodeParamsChanged(node.ID, p)
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.RebaseAt(g.elapsedBeats)
			}
		}
	})
	// Groove selector
	mk("grv", "", func() {
		sb.grooveDropdownOpen = !sb.grooveDropdownOpen
		if sb.grooveDropdownOpen {
			sb.logicDropdownOpen = false
		}
	})
	// Groove dropdown items
	if sb.grooveDropdownOpen {
		opts := []struct{ label, kind string }{
			{i18n.T(i18n.KeyLogicNone), ""}, {i18n.T(i18n.KeyGrooveDelay), "delay"}, {i18n.T(i18n.KeyGrooveRush), "rush"},
		}
		for _, o := range opts {
			kind := o.kind
			mk("groove:"+kind, o.label, func() {
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
					emitNodeParamsChanged(node.ID, p)
				}
				sb.grooveDropdownOpen = false
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
			emitNodeParamsChanged(node.ID, p)
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
			emitNodeParamsChanged(node.ID, p)
		}
	})
	// Audible toggle
	mk("aud", i18n.T(i18n.KeyNodeSecAudible), func() {
		if n, ok := g.graph.GetNodeByID(node.ID); ok {
			oldType := n.Type
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
			emitNodeTypeChanged(node.ID, oldType, n.Type)
		}
	})
	// Move button
	mk("move", i18n.T(i18n.KeyCapMoveNode), func() {
		g.moveMode = true
		g.movingNode = node
		sb.Close()
		g.moveSkipRelease = true
	})
	// Close button
	{
		r, ok := sb.rects["close"]
		if ok && !r.Empty() {
			if _, exists := sb.btns["close"]; !exists {
				sb.btns["close"] = NewButton("", DropdownStyle, func() {
					sb.Close()
				})
				sb.btns["close"].Icon = "close"
				sb.btns["close"].IconColor = closeIconColor()
			} else {
				sb.btns["close"].OnClick = func() {
					sb.Close()
				}
			}
			sb.btns["close"].SetRect(r)
			sb.btns["close"].ConsumeOnPress = true
		}
	}
}

// ─── InputHandler interface ────────────────────────────────────────────────

// InputBounds returns the sidebar rect (or expand tab if collapsed).
func (sb *NodeSidebar) InputBounds() image.Rectangle {
	if sb.collapsed {
		return sb.expandTabRect()
	}
	if r, ok := sb.rects["panel"]; ok {
		return r
	}
	return image.Rectangle{}
}

// ZIndex returns the z-order (above splitter and drumview).
func (sb *NodeSidebar) ZIndex() int { return 200 }

// HandleInput processes mouse/touch input within the sidebar.
// Follows the deferred-tap + scroll-commitment pattern from the instrument
// menu to prevent accidental section toggles during touch scrolling.
func (sb *NodeSidebar) HandleInput(x, y int, pressed bool) InputResult {
	if sb.collapsed {
		if pressed {
			sb.collapsed = false
			sb.open = true
		}
		return InputConsumed
	}
	if !sb.open || sb.node == nil {
		return InputIgnored
	}

	sb.layout()

	panelR := sb.rects["panel"]

	// 2. Scrollbar drag in-progress
	if sb.scroll.Dragging() {
		if pressed {
			sb.scroll.HandleDragTo(y)
		} else {
			sb.scroll.HandleDragEnd()
		}
		return InputCaptured
	}

	// 3. ScrollingCommitted → handle scroll move, cancel deferred tap
	if sb.scroll.ScrollingCommitted() {
		if pressed {
			sb.scroll.HandleTouchMove(x, y)
		} else {
			sb.deferredTap.Cancel()
			sb.scroll.HandleTouchEnd()
		}
		return InputCaptured
	}

	// 4. TouchActive but not yet committed (in dead zone) →
	//    process moves; on release fire deferred tap if no scroll committed
	if sb.scroll.TouchActive() {
		if pressed {
			sb.scroll.HandleTouchMove(x, y)
		} else {
			wasTap := !sb.scroll.ScrollingCommitted()
			sb.scroll.HandleTouchEnd()
			if wasTap {
				sb.deferredTap.End(sb.fireTapAt)
			} else {
				sb.deferredTap.Cancel()
			}
		}
		return InputCaptured
	}

	pt := image.Pt(x, y)

	// 5. Scrollbar thumb click (before touch begin to avoid intercepting it)
	if sb.scroll.HasScroll() {
		thumbRect := sb.scroll.ThumbRect()
		if pressed && pt.In(thumbRect) {
			sb.scroll.HandleDragStart(y)
			return InputCaptured
		}
	}

	// 6. Press in scroll viewport → begin deferred tap + touch scroll simultaneously
	//    Exclude scrollbar track area so track clicks reach step 11.
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	gridH := sb.game.split.GridH(sb.game.winH)
	viewport := image.Rect(panelR.Min.X, contentTop, panelR.Max.X, gridH)
	onScrollbarTrack := sb.scroll.HasScroll() && pt.In(sb.scroll.BarRect())
	if pressed && pt.In(viewport) && !onScrollbarTrack && !sb.scroll.TouchActive() && !sb.scroll.Dragging() {
		if sb.deferredTap.Begin(x, y) {
			sb.scroll.HandleTouchBegin(x, y)
			return InputCaptured
		}
	}

	// 7. Gate buttons + section headers during touch/deferred tap
	touchSuppressButtons := sb.scroll.TouchActive() || sb.deferredTap.Active()

	// 8. Hit-test buttons in z-order (only if not suppressed)
	if !touchSuppressButtons {
		order := sb.buttonOrder()
		for _, id := range order {
			btn, ok := sb.btns[id]
			if !ok {
				continue
			}
			r, ok := sb.rects[id]
			if !ok || r.Empty() {
				continue
			}
			btn.SetRect(r)
			if btn.HandleInputResult(x, y, pressed) != InputIgnored {
				return InputConsumed
			}
		}
	}

	// 9. Section header clicks (only if not suppressed by touch)
	if !touchSuppressButtons {
		sections := []string{"vol", "pit", "dur", "logic", "groove", "aud", "move"}
		for _, sec := range sections {
			r, ok := sb.rects["sec-"+sec]
			if !ok || r.Empty() {
				continue
			}
			if pressed && pt.In(r) {
				sb.toggleSection(sec)
				return InputConsumed
			}
		}
	}

	// 10. Close logic/groove dropdowns when clicking outside them
	if pressed && !touchSuppressButtons {
		sb.closeDropdownsOutside(x, y)
	}

	// 11. Scrollbar track click (not thumb) — jump scroll
	if pressed && sb.scroll.HasScroll() && pt.In(sb.scroll.BarRect()) {
		bar := sb.scroll.BarRect()
		trackH := bar.Dy()
		if trackH > 0 {
			frac := float64(y-bar.Min.Y) / float64(trackH)
			maxFirst := sb.scroll.VS.Total - sb.scroll.VS.Visible
			if maxFirst < 0 {
				maxFirst = 0
			}
			sb.scroll.VS.First = int(frac * float64(maxFirst))
			sb.scroll.VS.Clamp()
		}
		return InputConsumed
	}

	// 12. Release cleanup
	if !pressed {
		if sb.scroll.Dragging() {
			sb.scroll.HandleDragEnd()
		}
		if sb.scroll.TouchActive() {
			sb.scroll.HandleTouchEnd()
		}
	}

	// Consume any click inside the panel to prevent grid interaction
	return InputConsumed
}

// toggleSection toggles a section open/closed and cleans up related dropdowns.
func (sb *NodeSidebar) toggleSection(sec string) {
	sb.sectionOpen[sec] = !sb.sectionOpen[sec]
	if !sb.sectionOpen["logic"] {
		sb.logicDropdownOpen = false
	}
	if !sb.sectionOpen["groove"] {
		sb.grooveDropdownOpen = false
	}
}

// closeDropdownsOutside closes logic/groove dropdowns if click is outside them.
func (sb *NodeSidebar) closeDropdownsOutside(x, y int) {
	pt := image.Pt(x, y)
	if sb.logicDropdownOpen {
		inside := false
		if r, ok := sb.rects["logic"]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if strings.HasPrefix(id, "logic:") && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.logicDropdownOpen = false
		}
	}
	if sb.grooveDropdownOpen {
		inside := false
		if r, ok := sb.rects["grv"]; ok && pt.In(r) {
			inside = true
		}
		for id, r := range sb.rects {
			if strings.HasPrefix(id, "groove:") && pt.In(r) {
				inside = true
				break
			}
		}
		if !inside {
			sb.grooveDropdownOpen = false
		}
	}
}

// fireTapAt replays a deferred tap at stored coordinates — hit-tests buttons
// and section headers in z-order, fires the first match. Called by
// deferredTap.End() on touch release when scroll was NOT committed.
func (sb *NodeSidebar) fireTapAt(x, y int) {
	pt := image.Pt(x, y)

	// Close button (highest z-order)
	if btn, ok := sb.btns["close"]; ok && btn != nil {
		if r, ok := sb.rects["close"]; ok && pt.In(r) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}

	// Buttons in z-order
	order := sb.buttonOrder()
	for _, id := range order {
		if id == "close" {
			continue // already checked
		}
		btn, ok := sb.btns[id]
		if !ok || btn == nil {
			continue
		}
		r, ok := sb.rects[id]
		if !ok || r.Empty() {
			continue
		}
		if pt.In(r) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}

	// Section headers
	sections := []string{"vol", "pit", "dur", "logic", "groove", "aud", "move"}
	for _, sec := range sections {
		r, ok := sb.rects["sec-"+sec]
		if !ok || r.Empty() {
			continue
		}
		if pt.In(r) {
			sb.toggleSection(sec)
			return
		}
	}

	// Close dropdowns if tap was outside them
	sb.closeDropdownsOutside(x, y)
}

// HandleWheel scrolls content. steps comes from wheelScrollSteps() where
// positive = wheel up. ScrollBehavior.HandleWheel has the same convention,
// so we delegate directly.
func (sb *NodeSidebar) HandleWheel(_, _ int, steps int) InputResult {
	if !sb.open || sb.node == nil {
		return InputIgnored
	}
	sb.layout() // Ensure scroll limits are up-to-date
	sb.scroll.HandleWheel(steps * sidebarBtnH)
	return InputConsumed
}

// UpdateScroll applies per-frame momentum decay. Call from Game.Update()
// every frame (not from HandleInput, which only runs when the cursor is
// over the sidebar).
func (sb *NodeSidebar) UpdateScroll() {
	if !sb.open {
		return
	}
	sb.scroll.UpdateMomentum()
}

// Capturing reports if the sidebar is in an active drag.
func (sb *NodeSidebar) Capturing() bool {
	return sb.scroll.TouchActive() || sb.scroll.Dragging()
}

// buttonOrder returns button ids in z-order (highest first).
func (sb *NodeSidebar) buttonOrder() []string {
	order := []string{"close"}
	if sb.grooveDropdownOpen {
		for id := range sb.rects {
			if strings.HasPrefix(id, "groove:") {
				order = append(order, id)
			}
		}
	}
	if sb.logicDropdownOpen {
		for id := range sb.rects {
			if strings.HasPrefix(id, "logic:") {
				order = append(order, id)
			}
		}
	}
	order = append(order, "ln-", "ln+", "lp-", "lp+", "gp-", "gp+")
	order = append(order, "logic", "grv")
	order = append(order, "vol-", "vol+", "pit-", "pit+", "dur-", "dur+", "aud", "move")
	return order
}

// inViewport reports whether a rectangle is at least partially visible within
// the scrollable content area (below the fixed header and above the panel bottom).
func (sb *NodeSidebar) inViewport(r image.Rectangle) bool {
	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	gridH := sb.game.split.GridH(sb.game.winH)
	return r.Max.Y > contentTop && r.Min.Y < gridH
}

// ─── Draw ──────────────────────────────────────────────────────────────────

// Draw renders the sidebar onto the screen.
func (sb *NodeSidebar) Draw(dst *ebiten.Image) {
	if sb.collapsed {
		sb.drawExpandTab(dst)
		return
	}
	if !sb.open || sb.node == nil {
		return
	}

	sb.layout()
	g := sb.game
	panel := sb.rects["panel"]

	// Panel background with stronger shadow and elevated surface
	drawPanelShadow(dst, panel, 6)
	drawRoundedRect(dst, panel, colPanelBG, RadiusLG, true)
	drawRoundedRect(dst, panel, WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), RadiusLG, false)

	// Fixed header
	sb.drawHeader(dst)

	// Separator below header
	sepY := sidebarPad + sidebarHeaderH
	drawRect(dst, image.Rect(sidebarPad, sepY, panel.Max.X-sidebarPad, sepY+1), WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection), true)

	// Get node data
	mn, haveNode := g.graph.GetNodeByID(sb.node.ID)
	nodeType := model.NodeTypeRegular
	volVal := 1.0
	pitVal := 0.0
	durVal := 1.0
	if haveNode {
		nodeType = mn.Type
		volVal = mn.Params.Volume
		pitVal = mn.Params.Pitch
		durVal = mn.Params.Duration
	}

	contentTop := sidebarPad + sidebarHeaderH + sidebarGap
	gridH := g.split.GridH(g.winH)
	_, _ = contentTop, gridH // used by inViewport via sb receiver

	// Draw sections. Section headers now self-compute their text baseline via
	// StyledTextHeight; the legacy secTextOffY arg is retained for signature
	// compatibility but ignored by drawSectionHeader.
	sepColor := WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection)
	const secTextOffY = 0

	isSilent := nodeType == model.NodeTypeSilent
	isMute := nodeType == model.NodeTypeMute
	showVol := !isSilent && !isMute
	showPitch := !isSilent && !isMute
	showDur := !isSilent && !isMute
	showLogic := !isSilent
	showGroove := !isSilent

	_ = panel // used by layout; drawing uses sb.rects

	// Volume section
	if showVol {
		sb.drawSectionHeader(dst, "sec-vol", i18n.T(i18n.KeyNodeSecVolume), "vol", sepColor, secTextOffY)
		if sb.sectionOpen["vol"] {
			if r := sb.rects["vol-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyCapVol), sidebarPad+2, y, RoleCaption, colTextSecondary)
				pct := int(math.Round(volVal * 100))
				sb.drawBtn(dst, "vol-")
				sb.drawValuePill(dst, "volval", fmt.Sprintf("%d%%", pct))
				sb.drawBtn(dst, "vol+")
			}
		}
	}
	// Pitch section
	if showPitch {
		sb.drawSectionHeader(dst, "sec-pit", i18n.T(i18n.KeyCapPitch), "pit", sepColor, secTextOffY)
		if sb.sectionOpen["pit"] {
			if r := sb.rects["pit-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyCapPitch), sidebarPad+2, y, RoleCaption, colTextSecondary)
				sb.drawBtn(dst, "pit-")
				sb.drawValuePill(dst, "pitval", fmt.Sprintf("%+d", int(pitVal)))
				sb.drawBtn(dst, "pit+")
			}
		}
	}
	// Duration section
	if showDur {
		sb.drawSectionHeader(dst, "sec-dur", i18n.T(i18n.KeyNodeSecDuration), "dur", sepColor, secTextOffY)
		if sb.sectionOpen["dur"] {
			if r := sb.rects["dur-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + (sidebarBtnH-StyledTextHeight(RoleCaption))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyCapDur), sidebarPad+2, y, RoleCaption, colTextSecondary)
				sb.drawBtn(dst, "dur-")
				sb.drawValuePill(dst, "durval", fmt.Sprintf("%.2fx", durVal))
				sb.drawBtn(dst, "dur+")
			}
		}
	}
	// Logic section
	if showLogic {
		sb.drawSectionHeader(dst, "sec-logic", i18n.T(i18n.KeyNodeSecLogic), "logic", sepColor, secTextOffY)
		if sb.sectionOpen["logic"] {
			if logicRect := sb.rects["logic"]; !logicRect.Empty() && sb.inViewport(logicRect) {
				sb.drawBtn(dst, "logic")
				yLogic := logicRect.Min.Y + (sidebarBtnH-StyledTextHeight(RoleBody))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyNodeLogic), logicRect.Min.X+sidebarInnerPad, yLogic, RoleBody, colTextPrimary)
				// Current value
				cur := i18n.T(i18n.KeyLogicNone)
				if haveNode {
					switch mn.Params.LogicKind {
					case "every_n_triggers":
						cur = i18n.T(i18n.KeyLogicShortEveryN)
					case "skip_every_n":
						cur = i18n.T(i18n.KeyLogicShortSkipN)
					case "probability":
						cur = i18n.T(i18n.KeyLogicShortProb)
					case "trigger_if_prev_skipped":
						cur = i18n.T(i18n.KeyLogicShortPrevSkip)
					case "trigger_if_prev_triggered":
						cur = i18n.T(i18n.KeyLogicShortPrevTrig)
					}
				}
				curW := StyledTextWidth(cur, RoleBody)
				curX := logicRect.Max.X - sidebarInnerPad - curW
				DrawTextStyled(dst, cur, curX, yLogic, RoleBody, colTextPrimary)
			}
			// Dropdown items
			if sb.logicDropdownOpen {
				selID := ""
				if haveNode {
					selID = "logic:" + mn.Params.LogicKind
				}
				for id := range sb.rects {
					if strings.HasPrefix(id, "logic:") {
						sb.drawDropdownItem(dst, id, id == selID)
					}
				}
			} else if haveNode {
				// Parameter controls — styled inc/dec with value pill
				switch mn.Params.LogicKind {
				case "every_n_triggers", "skip_every_n":
					if lnRect := sb.rects["ln-"]; !lnRect.Empty() && sb.inViewport(lnRect) {
						DrawTextStyled(dst, "N", sidebarPad+2, lnRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, "ln-")
						sb.drawValuePill(dst, "lnval", fmt.Sprintf("%d", mn.Params.LogicN))
						sb.drawBtn(dst, "ln+")
					}
				case "probability":
					if lpRect := sb.rects["lp-"]; !lpRect.Empty() && sb.inViewport(lpRect) {
						DrawTextStyled(dst, "P", sidebarPad+2, lpRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, "lp-")
						sb.drawValuePill(dst, "lpval", fmt.Sprintf("%.0f%%", mn.Params.LogicP*100))
						sb.drawBtn(dst, "lp+")
					}
				}
			}
		}
	}
	// Groove section
	if showGroove {
		sb.drawSectionHeader(dst, "sec-groove", i18n.T(i18n.KeyNodeSecGroove), "groove", sepColor, secTextOffY)
		if sb.sectionOpen["groove"] {
			if grvRect := sb.rects["grv"]; !grvRect.Empty() && sb.inViewport(grvRect) {
				sb.drawBtn(dst, "grv")
				yGroove := grvRect.Min.Y + (sidebarBtnH-StyledTextHeight(RoleBody))/2
				DrawTextStyled(dst, i18n.T(i18n.KeyNodeGroove), grvRect.Min.X+sidebarInnerPad, yGroove, RoleBody, colTextPrimary)
				if haveNode {
					curGroove := i18n.T(i18n.KeyLogicNone)
					switch strings.ToLower(mn.Params.GrooveKind) {
					case "delay":
						curGroove = i18n.T(i18n.KeyGrooveDelay)
					case "rush":
						curGroove = i18n.T(i18n.KeyGrooveRush)
					}
					grvW := StyledTextWidth(curGroove, RoleBody)
					grvX := grvRect.Max.X - sidebarInnerPad - grvW
					DrawTextStyled(dst, curGroove, grvX, yGroove, RoleBody, colTextPrimary)
				}
			}
			if sb.grooveDropdownOpen {
				selID := ""
				if haveNode {
					selID = "groove:" + strings.ToLower(mn.Params.GrooveKind)
				}
				for id := range sb.rects {
					if strings.HasPrefix(id, "groove:") {
						sb.drawDropdownItem(dst, id, id == selID)
					}
				}
			} else {
				if haveNode {
					if gpRect := sb.rects["gp-"]; !gpRect.Empty() && sb.inViewport(gpRect) {
						DrawTextStyled(dst, i18n.T(i18n.KeyCapPct), sidebarPad+2, gpRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleCaption))/2, RoleCaption, colTextSecondary)
						sb.drawBtn(dst, "gp-")
						sb.drawValuePill(dst, "gpval", fmt.Sprintf("%.0f%%", mn.Params.GroovePct*100))
						sb.drawBtn(dst, "gp+")
					}
				}
			}
		}
	}
	// Audible section
	{
		sb.drawSectionHeader(dst, "sec-aud", i18n.T(i18n.KeyNodeSecAudible), "aud", sepColor, secTextOffY)
		if sb.sectionOpen["aud"] {
			if audRect := sb.rects["aud"]; !audRect.Empty() && sb.inViewport(audRect) {
				audStyle := DropdownStyle
				audStyle.Fill = blendColor(color.RGBAModel.Convert(DropdownStyle.Fill).(color.RGBA), color.RGBAModel.Convert(sb.nodeAccent()).(color.RGBA), sidebarBtnAccentTint)
				audStyle.DrawAnimated(dst, audRect, false, sb.anim["aud"])
				label := i18n.T(i18n.KeyNodeSecAudible)
				switch nodeType {
				case model.NodeTypeSilent:
					label = i18n.T(i18n.KeyAudSilent)
				case model.NodeTypeMute:
					label = i18n.T(i18n.KeyAudMuted)
				}
				DrawTextStyled(dst, label, audRect.Min.X+sidebarInnerPad, audRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleBody))/2, RoleBody, colTextPrimary)
			}
		}
	}
	// Move section (desktop only)
	if !Profile().IsMobile() {
		sb.drawSectionHeader(dst, "sec-move", i18n.T(i18n.KeyCapMove), "move", sepColor, secTextOffY)
		if sb.sectionOpen["move"] {
			if moveRect := sb.rects["move"]; !moveRect.Empty() && sb.inViewport(moveRect) {
				moveStyle := DropdownStyle
				moveStyle.Fill = blendColor(color.RGBAModel.Convert(DropdownStyle.Fill).(color.RGBA), color.RGBAModel.Convert(sb.nodeAccent()).(color.RGBA), sidebarBtnAccentTint)
				moveStyle.DrawAnimated(dst, moveRect, false, sb.anim["move"])
				DrawTextStyled(dst, i18n.T(i18n.KeyCapMoveNode), moveRect.Min.X+sidebarInnerPad, moveRect.Min.Y+(sidebarBtnH-StyledTextHeight(RoleBody))/2, RoleBody, colTextPrimary)
			}
		}
	}

	// Close button — draw with visible background for discoverability; its
	// outline carries the node's instrument color like every other control.
	if btn, ok := sb.btns["close"]; ok && btn != nil {
		r := btn.Rect()
		if !r.Empty() {
			drawRect(dst, r, WithAlpha(genColorSidebarChipFill, genAlphaSidebarChip), true)
			drawRect(dst, r, sb.nodeAccent(), false)
			sb.tintBtnAccent(btn)
			btn.Draw(dst)
		}
	}

	// Scrollbar via ScrollBehavior
	sb.scroll.Draw(dst)

	// Decay animations
	for k, v := range sb.anim {
		if v > 0 {
			v *= 0.85
			if v < 0.02 {
				v = 0
			}
			sb.anim[k] = v
		}
	}
}

// drawHeader renders the fixed header with instrument name and color swatch.
// nodeAccent returns the accent color for the node sidebar: the owning row's
// instrument color so the sidebar's header underline, active section stripes,
// and accent text all read as owned by the instrument the node belongs to.
// Falls back to the azure chrome accent when the node has no resolvable row.
func (sb *NodeSidebar) nodeAccent() color.Color {
	if sb.node != nil && sb.game != nil {
		if row, ok := sb.game.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(sb.game.drum.Rows) {
			if c := sb.game.drum.Rows[row].Color; c != nil {
				return c
			}
		}
	}
	return colAccent
}

func (sb *NodeSidebar) drawHeader(dst *ebiten.Image) {
	g := sb.game
	headerRect := sb.rects["header"]
	if headerRect.Empty() {
		return
	}

	// Shared "Neon Horizon" header band, tinted to the node's instrument color.
	drawMenuHeaderBandAccent(dst, headerRect, sb.nodeAccent())

	// Instrument swatch + name via the shared, unified menu-title treatment so
	// the node pop-up title matches the row context menu 100% (font/size/icon).
	swatchCol := color.Color(genColorSidebarSwatchFallback)
	label := i18n.T(i18n.KeyNodeTitle)
	if row, ok := g.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
		dr := g.drum.Rows[row]
		swatchCol = dr.Color
		label = dr.Name
	}
	titleRect := image.Rect(headerRect.Min.X+2, headerRect.Min.Y, headerRect.Max.X, headerRect.Min.Y+sidebarHeaderH)
	drawMenuTitle(dst, titleRect, label, swatchCol)
}

// drawSectionHeader draws a collapsible section header using the shared menu
// chrome (icon chevron + item background + active stripe) and an optional
// collapsed-state summary badge.
func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID, label, sectionID string, _ color.Color, _ int) {
	r, ok := sb.rects[rectID]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	expanded := sb.sectionOpen[sectionID]
	// No expand highlight: the chevron direction (down=open, right=closed) already
	// conveys section state, and the node-color fill/stripe + accent text read as a
	// distracting "selected" bar. Headers stay neutral; node color lives on the
	// buttons, header swatch, and dropdown stripes.
	chevR := image.Rect(r.Min.X+sidebarPad, r.Min.Y, r.Min.X+sidebarPad+IconSizeMD, r.Max.Y)
	var chevCol color.Color = colTextSecondary
	var labelCol color.Color = colTextPrimary
	drawMenuChevron(dst, chevR, expanded, chevCol)

	th := StyledTextHeight(RoleSectionHeader)
	labelX := chevR.Max.X + sidebarGap
	labelY := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, label, labelX, labelY, RoleSectionHeader, labelCol)

	sb.drawSectionBadge(dst, r, sectionID)
}

// drawSectionBadge renders the collapsed-section summary badge (right-aligned,
// square sharp corners) showing the section's current value (default or not).
func (sb *NodeSidebar) drawSectionBadge(dst *ebiten.Image, r image.Rectangle, sectionID string) {
	if sb.sectionOpen[sectionID] {
		return
	}
	badge := sb.sectionBadgeText(sectionID)
	if badge == "" {
		return
	}
	badgePadX := 6
	badgePadY := 2
	bw := StyledTextWidth(badge, RoleCaption) + 2*badgePadX
	bh := StyledTextHeight(RoleCaption) + 2*badgePadY
	bx := r.Max.X - bw - 2
	by := r.Min.Y + (sidebarSectionH-bh)/2
	pillR := image.Rect(bx, by, bx+bw, by+bh)
	drawRect(dst, pillR, colSurface2, true)
	DrawTextStyled(dst, badge, bx+badgePadX, by+badgePadY, RoleCaption, colTextSecondary)
}

// sectionBadgeText returns a summary string of a collapsed section's current
// value. Every category surfaces its value — including when it is at the
// default — using the same formatting as updated values, so a glance at the
// collapsed sidebar shows each category's state (e.g. "100%", "+0", "None").
func (sb *NodeSidebar) sectionBadgeText(sectionID string) string {
	if sb.node == nil {
		return ""
	}
	g := sb.game
	mn, ok := g.graph.GetNodeByID(sb.node.ID)
	if !ok {
		return ""
	}
	switch sectionID {
	case "vol":
		pct := int(math.Round(mn.Params.Volume * 100))
		return fmt.Sprintf("%d%%", pct)
	case "pit":
		return fmt.Sprintf("%+d", int(mn.Params.Pitch))
	case "dur":
		return fmt.Sprintf("%.2fx", mn.Params.Duration)
	case "logic":
		switch mn.Params.LogicKind {
		case "every_n_triggers":
			return fmt.Sprintf("Every %d", mn.Params.LogicN)
		case "skip_every_n":
			return fmt.Sprintf("Skip %d", mn.Params.LogicN)
		case "probability":
			return fmt.Sprintf("P %.0f%%", mn.Params.LogicP*100)
		case "trigger_if_prev_skipped":
			return i18n.T(i18n.KeyLogicShortPrevSkip)
		case "trigger_if_prev_triggered":
			return i18n.T(i18n.KeyLogicShortPrevTrig)
		}
		return i18n.T(i18n.KeyLogicNone)
	case "groove":
		kind := strings.ToLower(mn.Params.GrooveKind)
		if kind == "delay" || kind == "rush" {
			title := strings.ToUpper(kind[:1]) + kind[1:]
			return fmt.Sprintf("%s %.0f%%", title, mn.Params.GroovePct*100)
		}
		return i18n.T(i18n.KeyLogicNone)
	case "aud":
		switch mn.Type {
		case model.NodeTypeSilent:
			return i18n.T(i18n.KeyAudSilent)
		case model.NodeTypeMute:
			return i18n.T(i18n.KeyAudMuted)
		}
		return i18n.T(i18n.KeyNodeSecAudible)
	}
	return ""
}

// drawExpandTab draws the collapsed sidebar expand tab.
func (sb *NodeSidebar) drawExpandTab(dst *ebiten.Image) {
	r := sb.expandTabRect()
	drawRoundedRect(dst, r, WithAlpha(genColorSidebarBadgeBg, genAlphaSidebarChip), RadiusXXS, true)
	// Chevron-right at the unified icon-system size.
	dim := IconSizeMD
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	iconR := image.Rect(cx-dim/2, cy-dim/2, cx+dim/2, cy+dim/2)
	DrawIcon(dst, IconChevronRight, iconR, colTextSecondary)
}

// drawBtn draws a single button if it exists and is within the viewport.
func (sb *NodeSidebar) drawBtn(dst *ebiten.Image, id string) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	if r, ok := sb.rects[id]; ok && !sb.inViewport(r) {
		return
	}
	sb.tintBtnAccent(b)
	b.Draw(dst)
}

// sidebarBtnAccentTint is how strongly a node-sidebar button's keycap FACE is
// tinted toward the node's color. Low enough to keep the 3D keycap reading
// (shell + bevel) while carrying a clear node-color accent.
const sidebarBtnAccentTint = 0.4

// tintBtnAccent gives a node-sidebar button the node's color as a tint on its
// keycap FACE — the same way the rest of our buttons accent (a filled cap, like
// the latched/amber state), NOT a flat bright outline. The button keeps the
// standard 3D keycap frame (dark socket shell + bevel + default border) so it
// reads identically to the menu keycaps, just node-colored. Blends from the
// stable DropdownStyle.Fill base so repeated per-frame calls don't compound.
func (sb *NodeSidebar) tintBtnAccent(b *Button) {
	if b == nil {
		return
	}
	if bs, ok := b.Style.(ButtonStyle); ok {
		base := color.RGBAModel.Convert(DropdownStyle.Fill).(color.RGBA)
		acc := color.RGBAModel.Convert(sb.nodeAccent()).(color.RGBA)
		bs.Fill = blendColor(base, acc, sidebarBtnAccentTint)
		b.Style = bs
	}
}

// drawDropdownItem renders one Logic/Groove dropdown LIST row through the shared
// drawMenuRow primitive (same as every other menu list): background accent +
// keycap chrome + label. The node's own color (nodeAccent — SSOT DrumRow.Color)
// is the accent, so the selected row shows a node-color stripe and a hovered row
// a node-color tint. The clip guard keeps off-viewport rows from drawing.
func (sb *NodeSidebar) drawDropdownItem(dst *ebiten.Image, id string, selected bool) {
	b := sb.btns[id]
	if b == nil {
		return
	}
	r, ok := sb.rects[id]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	state := menuItemRest
	if selected {
		state = menuItemActive
	} else if b.hovered || b.pressed {
		state = menuItemHover
	}
	drawMenuRow(dst, b, MenuRowSpec{
		Accent: sb.nodeAccent(),
		State:  state,
		Label:  b.Text,
	})
}


// drawValuePill draws a centered value READOUT between the inc/dec stepper
// keys. In the mechanical-keycap language the steppers are raised keys, so the
// readout reads as a RECESSED display window sunk into the panel: a fill
// darker than the panel + an inner top shadow (the tactile opposite of a
// keycap's top specular), then the border and centered colTextPrimary text.
func (sb *NodeSidebar) drawValuePill(dst *ebiten.Image, key, value string) {
	r, ok := sb.rects[key]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	drawRecessedPill(dst, r, value)
}

// drawRecessedPill paints a value READOUT as a recessed display well: a fill
// darker than the panel + an inner top shadow (the tactile opposite of a
// keycap's top specular), the border, then centered colTextPrimary text. Pure
// (no sidebar state) so it is unit-testable and reusable for any readout.
func drawRecessedPill(dst *ebiten.Image, r image.Rectangle, value string) {
	drawRoundedRect(dst, r, adjustColor(colSurface2, -22), RadiusSM, true)
	drawRect(dst, image.Rect(r.Min.X+2, r.Min.Y+1, r.Max.X-2, r.Min.Y+1+genGeomButtonInnerShadowPx),
		WithAlphaFromColor(color.Black, 56), true)
	drawRoundedRect(dst, r, colBorderMedium, RadiusSM, false)

	tw := StyledTextWidth(value, RoleBody)
	th := StyledTextHeight(RoleBody)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	DrawTextStyled(dst, value, tx, ty, RoleBody, colTextPrimary)
}
