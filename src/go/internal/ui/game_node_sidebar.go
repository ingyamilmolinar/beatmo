package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Sidebar sizing constants ──────────────────────────────────────────────
const (
	sidebarDefaultW  = 260
	sidebarMinW      = 200
	sidebarMaxW      = 400
	sidebarBtnH      = 28 // unified button/row height
	sidebarIncBtnW   = 28 // inc/dec button width
	sidebarIncBtnH   = 24 // inc/dec button height
	sidebarGap       = 6
	sidebarPad       = 10
	sidebarHeaderH   = 36
	sidebarSectionH  = 30 // collapsible section header height
	sidebarTextScale = 1.2
	sidebarResizeW   = 8  // resize grab zone on right edge
	sidebarTabW      = 24 // collapsed expand-tab width
	sidebarInnerPad  = 4  // inner padding for text within layout rects
	sidebarSwatchSz  = 12 // instrument color swatch size in header
)

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
	width     int  // current width (resizable)
	collapsed bool // fully hidden (show expand tab only)

	// Resize
	resizing    bool
	resizeDragX int

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

// resizeHandleRect returns the pill handle rect for the sidebar resize edge.
func (sb *NodeSidebar) resizeHandleRect() image.Rectangle {
	panel, ok := sb.rects["panel"]
	if !ok || panel.Empty() {
		return image.Rectangle{}
	}
	cx := panel.Max.X
	cy := panel.Dy() / 2
	return SplitterHandleRect(cx, cy, false) // vertical pill (sidebar is a vertical divider)
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
		y = sb.layoutSection(y, w, btnX, "vol", "Volume", true)
	}
	// Section: Pitch
	if showPitch {
		y = sb.layoutSection(y, w, btnX, "pit", "Pitch", true)
	}
	// Section: Duration
	if showDur {
		y = sb.layoutSection(y, w, btnX, "dur", "Duration", true)
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
		pillW := 50
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
			pillW := 50
			pillX := plusX - sidebarGap - pillW
			minusX := pillX - sidebarGap - sidebarIncBtnW
			btnY := y + (sidebarBtnH-sidebarIncBtnH)/2
			sb.rects["ln-"] = image.Rect(minusX, btnY, minusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			sb.rects["lnval"] = image.Rect(pillX, btnY, pillX+pillW, btnY+sidebarIncBtnH)
			sb.rects["ln+"] = image.Rect(plusX, btnY, plusX+sidebarIncBtnW, btnY+sidebarIncBtnH)
			y += sidebarBtnH + sidebarGap
		} else if kindHasP {
			plusX := w - sidebarPad - sidebarIncBtnW
			pillW := 50
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
		pillW := 50
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
			sb.btns[id] = NewButton(label, PopupButtonStyle, func() {
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
		sb.btns[id].TextScale = sidebarTextScale
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
	// Pitch +/-
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
	// Duration +/-
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
			{"None", ""},
			{"Trigger Every N", "every_n_triggers"},
			{"Skip Every N", "skip_every_n"},
			{"Probability", "probability"},
			{"If Prev Skipped", "trigger_if_prev_skipped"},
			{"If Prev Triggered", "trigger_if_prev_triggered"},
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
			{"None", ""}, {"Delay", "delay"}, {"Rush", "rush"},
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
	// Audible toggle
	mk("aud", "Audible", func() {
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
		}
	})
	// Move button
	mk("move", "Move Node", func() {
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
				sb.btns["close"] = NewButton("", PopupButtonStyle, func() {
					sb.Close()
				})
				sb.btns["close"].Icon = "close"
				sb.btns["close"].IconColor = colButtonBorder
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

	// 1. Resize drag on right edge via pill handle
	panelR := sb.rects["panel"]
	rightEdge := panelR.Max.X
	onScrollbar := sb.scroll.HasScroll() && image.Pt(x, y).In(sb.scroll.BarRect())
	if !onScrollbar && x >= rightEdge-sidebarResizeW && x <= rightEdge+sidebarResizeW {
		if pressed && !sb.resizing {
			handleR := sb.resizeHandleRect()
			if image.Pt(x, y).In(handleR.Inset(-SpaceSM)) {
				sb.resizing = true
				sb.resizeDragX = x
				return InputCaptured
			}
		}
	}
	if sb.resizing {
		if pressed {
			sb.width += x - sb.resizeDragX
			sb.resizeDragX = x
			if sb.width < sidebarMinW {
				sb.width = sidebarMinW
			}
			if sb.width > sidebarMaxW {
				sb.width = sidebarMaxW
			}
			return InputCaptured
		}
		sb.resizing = false
		return InputConsumed
	}

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
			if btn.Handle(x, y, pressed) {
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
	return sb.resizing || sb.scroll.TouchActive() || sb.scroll.Dragging()
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

	// Draw sections
	sepColor := WithAlpha(genColorSidebarSectionBg, genAlphaSidebarSection)
	textOffY := (sidebarBtnH - int(float64(TextHeight())*sidebarTextScale)) / 2
	if textOffY < 0 {
		textOffY = 0
	}
	secTextOffY := (sidebarSectionH - int(float64(TextHeight())*sidebarTextScale)) / 2
	if secTextOffY < 0 {
		secTextOffY = 0
	}

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
		sb.drawSectionHeader(dst, "sec-vol", "Volume", "vol", sepColor, secTextOffY)
		if sb.sectionOpen["vol"] {
			if r := sb.rects["vol-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				sidebarDrawTextColorAtScale(dst, "Vol", sidebarPad+2, y, colTextSecondary, sidebarTextScale)
				pct := int(math.Round(volVal * 100))
				sb.drawIncDecBtn(dst, "vol-", "\u2212") // −
				sb.drawValuePill(dst, "volval", fmt.Sprintf("%d%%", pct))
				sb.drawIncDecBtn(dst, "vol+", "+")
			}
		}
	}
	// Pitch section
	if showPitch {
		sb.drawSectionHeader(dst, "sec-pit", "Pitch", "pit", sepColor, secTextOffY)
		if sb.sectionOpen["pit"] {
			if r := sb.rects["pit-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				sidebarDrawTextColorAtScale(dst, "Pitch", sidebarPad+2, y, colTextSecondary, sidebarTextScale)
				sb.drawIncDecBtn(dst, "pit-", "\u2212") // −
				sb.drawValuePill(dst, "pitval", fmt.Sprintf("%+d", int(pitVal)))
				sb.drawIncDecBtn(dst, "pit+", "+")
			}
		}
	}
	// Duration section
	if showDur {
		sb.drawSectionHeader(dst, "sec-dur", "Duration", "dur", sepColor, secTextOffY)
		if sb.sectionOpen["dur"] {
			if r := sb.rects["dur-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				sidebarDrawTextColorAtScale(dst, "Dur", sidebarPad+2, y, colTextSecondary, sidebarTextScale)
				sb.drawIncDecBtn(dst, "dur-", "\u2212") // −
				sb.drawValuePill(dst, "durval", fmt.Sprintf("%.2fx", durVal))
				sb.drawIncDecBtn(dst, "dur+", "+")
			}
		}
	}
	// Logic section
	if showLogic {
		sb.drawSectionHeader(dst, "sec-logic", "Logic", "logic", sepColor, secTextOffY)
		if sb.sectionOpen["logic"] {
			if logicRect := sb.rects["logic"]; !logicRect.Empty() && sb.inViewport(logicRect) {
				sb.drawBtn(dst, "logic")
				yLogic := logicRect.Min.Y + textOffY
				DrawTextAtScale(dst, "Logic:", logicRect.Min.X+sidebarInnerPad, yLogic, sidebarTextScale)
				// Current value
				cur := "None"
				if haveNode {
					switch mn.Params.LogicKind {
					case "every_n_triggers":
						cur = "Every N"
					case "skip_every_n":
						cur = "Skip N"
					case "probability":
						cur = "Prob"
					case "trigger_if_prev_skipped":
						cur = "Prev Skip"
					case "trigger_if_prev_triggered":
						cur = "Prev Trig"
					}
				}
				curW := sidebarScaledTextWidth(cur, sidebarTextScale)
				curX := logicRect.Max.X - sidebarInnerPad - curW
				DrawTextAtScale(dst, cur, curX, yLogic, sidebarTextScale)
			}
			// Dropdown items
			if sb.logicDropdownOpen {
				for id := range sb.rects {
					if strings.HasPrefix(id, "logic:") {
						sb.drawBtn(dst, id)
					}
				}
			} else if haveNode {
				// Parameter controls — styled inc/dec with value pill
				switch mn.Params.LogicKind {
				case "every_n_triggers", "skip_every_n":
					if lnRect := sb.rects["ln-"]; !lnRect.Empty() && sb.inViewport(lnRect) {
						sidebarDrawTextColorAtScale(dst, "N", sidebarPad+2, lnRect.Min.Y+textOffY, colTextSecondary, sidebarTextScale)
						sb.drawIncDecBtn(dst, "ln-", "\u2212")
						sb.drawValuePill(dst, "lnval", fmt.Sprintf("%d", mn.Params.LogicN))
						sb.drawIncDecBtn(dst, "ln+", "+")
					}
				case "probability":
					if lpRect := sb.rects["lp-"]; !lpRect.Empty() && sb.inViewport(lpRect) {
						sidebarDrawTextColorAtScale(dst, "P", sidebarPad+2, lpRect.Min.Y+textOffY, colTextSecondary, sidebarTextScale)
						sb.drawIncDecBtn(dst, "lp-", "\u2212")
						sb.drawValuePill(dst, "lpval", fmt.Sprintf("%.0f%%", mn.Params.LogicP*100))
						sb.drawIncDecBtn(dst, "lp+", "+")
					}
				}
			}
		}
	}
	// Groove section
	if showGroove {
		sb.drawSectionHeader(dst, "sec-groove", "Groove", "groove", sepColor, secTextOffY)
		if sb.sectionOpen["groove"] {
			if grvRect := sb.rects["grv"]; !grvRect.Empty() && sb.inViewport(grvRect) {
				sb.drawBtn(dst, "grv")
				yGroove := grvRect.Min.Y + textOffY
				DrawTextAtScale(dst, "Groove:", grvRect.Min.X+sidebarInnerPad, yGroove, sidebarTextScale)
				if haveNode {
					curGroove := "None"
					switch strings.ToLower(mn.Params.GrooveKind) {
					case "delay":
						curGroove = "Delay"
					case "rush":
						curGroove = "Rush"
					}
					grvW := sidebarScaledTextWidth(curGroove, sidebarTextScale)
					grvX := grvRect.Max.X - sidebarInnerPad - grvW
					DrawTextAtScale(dst, curGroove, grvX, yGroove, sidebarTextScale)
				}
			}
			if sb.grooveDropdownOpen {
				for id := range sb.rects {
					if strings.HasPrefix(id, "groove:") {
						sb.drawBtn(dst, id)
					}
				}
			} else {
				if haveNode {
					if gpRect := sb.rects["gp-"]; !gpRect.Empty() && sb.inViewport(gpRect) {
						sidebarDrawTextColorAtScale(dst, "Pct", sidebarPad+2, gpRect.Min.Y+textOffY, colTextSecondary, sidebarTextScale)
						sb.drawIncDecBtn(dst, "gp-", "\u2212")
						sb.drawValuePill(dst, "gpval", fmt.Sprintf("%.0f%%", mn.Params.GroovePct*100))
						sb.drawIncDecBtn(dst, "gp+", "+")
					}
				}
			}
		}
	}
	// Audible section
	{
		sb.drawSectionHeader(dst, "sec-aud", "Audible", "aud", sepColor, secTextOffY)
		if sb.sectionOpen["aud"] {
			if audRect := sb.rects["aud"]; !audRect.Empty() && sb.inViewport(audRect) {
				PopupButtonStyle.DrawAnimated(dst, audRect, false, sb.anim["aud"])
				label := "Audible"
				switch nodeType {
				case model.NodeTypeSilent:
					label = "Silent"
				case model.NodeTypeMute:
					label = "Muted"
				}
				DrawTextAtScale(dst, label, audRect.Min.X+sidebarInnerPad, audRect.Min.Y+textOffY, sidebarTextScale)
			}
		}
	}
	// Move section (desktop only)
	if !Profile().IsMobile() {
		sb.drawSectionHeader(dst, "sec-move", "Move", "move", sepColor, secTextOffY)
		if sb.sectionOpen["move"] {
			if moveRect := sb.rects["move"]; !moveRect.Empty() && sb.inViewport(moveRect) {
				PopupButtonStyle.DrawAnimated(dst, moveRect, false, sb.anim["move"])
				DrawTextAtScale(dst, "Move Node", moveRect.Min.X+sidebarInnerPad, moveRect.Min.Y+textOffY, sidebarTextScale)
			}
		}
	}

	// Close button — draw with visible background for discoverability
	if btn, ok := sb.btns["close"]; ok && btn != nil {
		r := btn.Rect()
		if !r.Empty() {
			drawRect(dst, r, WithAlpha(genColorSidebarChipFill, genAlphaSidebarChip), true)
			drawRect(dst, r, WithAlpha(genColorSidebarChipBorder, genAlphaStrong), false)
			btn.Draw(dst)
		}
	}

	// Resize pill handle on right edge
	DrawSplitterHandle(dst, panel.Max.X, panel.Dy()/2, false, sb.resizing)

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
func (sb *NodeSidebar) drawHeader(dst *ebiten.Image) {
	g := sb.game
	headerRect := sb.rects["header"]
	if headerRect.Empty() {
		return
	}
	hx := headerRect.Min.X
	hy := headerRect.Min.Y + (sidebarHeaderH-int(float64(TextHeight())*sidebarTextScale))/2

	// Instrument color swatch (12×12 rounded square) + name only (no coordinates).
	swatchCol := color.Color(genColorSidebarSwatchFallback)
	label := "Node"

	if row, ok := g.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
		dr := g.drum.Rows[row]
		swatchCol = dr.Color
		label = dr.Name
	}

	swatchY := hy + (int(float64(TextHeight())*sidebarTextScale)-sidebarSwatchSz)/2
	swatchRect := image.Rect(hx, swatchY, hx+sidebarSwatchSz, swatchY+sidebarSwatchSz)
	drawRoundedRect(dst, swatchRect, swatchCol, 3, true)
	textX := hx + sidebarSwatchSz + 6

	DrawTextAtScale(dst, label, textX, hy, sidebarTextScale)
}

// drawSectionHeader draws a collapsible section header with filled triangle
// chevron and optional collapsed-state summary badge.
func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID, label, sectionID string, _ color.Color, textOffY int) {
	r, ok := sb.rects[rectID]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	// Subtle separator line above section
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colBorderSubtle, true)

	expanded := sb.sectionOpen[sectionID]

	// Filled triangle chevron: proportional to text height
	triH := int(float64(TextHeight()) * sidebarTextScale)
	if triH < 6 {
		triH = 6
	}
	triW := triH * 2 / 3 // slightly narrower than tall
	triX := r.Min.X + 2
	triCY := r.Min.Y + sidebarSectionH/2

	if expanded {
		// Down-pointing filled triangle (▾) in accent color
		sb.drawFilledTriangleDown(dst, triX, triCY-triH/2, triW, triH, colTextAccent)
	} else {
		// Right-pointing filled triangle (▸) in secondary color
		sb.drawFilledTriangleRight(dst, triX, triCY-triH/2, triW, triH, colTextSecondary)
	}

	// Label text after triangle
	labelX := triX + triW + 6
	labelCol := colTextPrimary
	if expanded {
		labelCol = colTextAccent
	}
	sidebarDrawTextColorAtScale(dst, label, labelX, r.Min.Y+textOffY, labelCol, sidebarTextScale)

	// Collapsed summary badge (right-aligned pill with non-default value)
	if !expanded {
		badge := sb.sectionBadgeText(sectionID)
		if badge != "" {
			badgePadX := 6
			badgePadY := 2
			bw := sidebarScaledTextWidth(badge, sidebarTextScale) + 2*badgePadX
			bh := int(float64(TextHeight())*sidebarTextScale) + 2*badgePadY
			bx := r.Max.X - bw - 2
			by := r.Min.Y + (sidebarSectionH-bh)/2
			pillR := image.Rect(bx, by, bx+bw, by+bh)
			drawRoundedRect(dst, pillR, colSurface2, RadiusMD/2, true)
			sidebarDrawTextColorAtScale(dst, badge, bx+badgePadX, by+badgePadY, colTextSecondary, sidebarTextScale)
		}
	}
}

// drawFilledTriangleRight draws a right-pointing filled triangle (play icon shape).
// Geometry lives in triangleRightRowSpan (sidebar_icons.go); this method is
// the thin pixel-emit wrapper around it.
func (sb *NodeSidebar) drawFilledTriangleRight(dst *ebiten.Image, x, y, w, h int, col color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	for row := 0; row < h; row++ {
		xs, xe := triangleRightRowSpan(x, y, w, h, row)
		if xe > xs {
			drawRect(dst, image.Rect(xs, y+row, xe, y+row+1), col, true)
		}
	}
}

// drawFilledTriangleDown draws a down-pointing filled triangle. Geometry
// lives in triangleDownRowSpan (sidebar_icons.go).
func (sb *NodeSidebar) drawFilledTriangleDown(dst *ebiten.Image, x, y, w, h int, col color.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	for row := 0; row < h; row++ {
		xs, xe := triangleDownRowSpan(x, y, w, h, row)
		if xe > xs {
			drawRect(dst, image.Rect(xs, y+row, xe, y+row+1), col, true)
		}
	}
}

// sectionBadgeText returns a summary string for a collapsed section with
// non-default values, or "" if the section has default values.
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
		if pct != 100 {
			return fmt.Sprintf("%d%%", pct)
		}
	case "pit":
		p := int(mn.Params.Pitch)
		if p != 0 {
			return fmt.Sprintf("%+d", p)
		}
	case "dur":
		if math.Abs(mn.Params.Duration-1.0) > 0.01 {
			return fmt.Sprintf("%.2fx", mn.Params.Duration)
		}
	case "logic":
		switch mn.Params.LogicKind {
		case "every_n_triggers":
			return fmt.Sprintf("Every %d", mn.Params.LogicN)
		case "skip_every_n":
			return fmt.Sprintf("Skip %d", mn.Params.LogicN)
		case "probability":
			return fmt.Sprintf("P %.0f%%", mn.Params.LogicP*100)
		case "trigger_if_prev_skipped":
			return "Prev Skip"
		case "trigger_if_prev_triggered":
			return "Prev Trig"
		}
	case "groove":
		kind := strings.ToLower(mn.Params.GrooveKind)
		if kind == "delay" || kind == "rush" {
			title := strings.ToUpper(kind[:1]) + kind[1:]
			return fmt.Sprintf("%s %.0f%%", title, mn.Params.GroovePct*100)
		}
	case "aud":
		switch mn.Type {
		case model.NodeTypeSilent:
			return "Silent"
		case model.NodeTypeMute:
			return "Muted"
		}
	}
	return ""
}

// sidebarDrawTextColorAtScale draws text at the given position with color and scale.
func sidebarDrawTextColorAtScale(dst *ebiten.Image, s string, x, y int, col color.Color, scale float64) {
	spr := TextSprite(s)
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x), float64(y))
	r, g, b, a := col.RGBA()
	if a > 0 {
		fa := float64(a) / 0xffff
		op.ColorScale.Scale(float32(float64(r)/0xffff/fa), float32(float64(g)/0xffff/fa), float32(float64(b)/0xffff/fa), float32(fa))
	}
	dst.DrawImage(spr, &op)
}

// drawExpandTab draws the collapsed sidebar expand tab.
func (sb *NodeSidebar) drawExpandTab(dst *ebiten.Image) {
	r := sb.expandTabRect()
	drawRoundedRect(dst, r, WithAlpha(genColorSidebarBadgeBg, genAlphaSidebarChip), 4, true)
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
	b.Draw(dst)
}

// drawIncDecBtn draws a 28x24 increment/decrement button as a rounded
// rectangle: colSurface2 fill, colBorderMedium border, 6px radius.
// Press state darkens fill by -20. Label in colTextSecondary, centered.
func (sb *NodeSidebar) drawIncDecBtn(dst *ebiten.Image, key, label string) {
	r, ok := sb.rects[key]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	pressed := false
	if b := sb.btns[key]; b != nil {
		pressed = b.pressed
	}
	fill := color.Color(colSurface2)
	if pressed {
		fill = adjustColor(fill, -20)
	}
	drawRoundedRect(dst, r, fill, 6, true)
	drawRoundedRect(dst, r, colBorderMedium, 6, false)

	tw := sidebarScaledTextWidth(label, sidebarTextScale)
	th := int(float64(TextHeight()) * sidebarTextScale)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	sidebarDrawTextColorAtScale(dst, label, tx, ty, colTextSecondary, sidebarTextScale)
}

// drawValuePill draws a centered value display pill between inc/dec buttons:
// colSurface2 fill, 6px radius, colTextPrimary text centered.
func (sb *NodeSidebar) drawValuePill(dst *ebiten.Image, key, value string) {
	r, ok := sb.rects[key]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	drawRoundedRect(dst, r, colSurface2, 6, true)

	tw := sidebarScaledTextWidth(value, sidebarTextScale)
	th := int(float64(TextHeight()) * sidebarTextScale)
	tx := r.Min.X + (r.Dx()-tw)/2
	ty := r.Min.Y + (r.Dy()-th)/2
	sidebarDrawTextColorAtScale(dst, value, tx, ty, colTextPrimary, sidebarTextScale)
}
