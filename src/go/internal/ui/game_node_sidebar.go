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
	sidebarGap       = 6
	sidebarPad       = 10
	sidebarHeaderH   = 36
	sidebarSectionH  = 30 // collapsible section header height
	sidebarTextScale = 1.2
	sidebarResizeW   = 8  // resize grab zone on right edge
	sidebarTabW      = 24 // collapsed expand-tab width
	sidebarInnerPad  = 4  // inner padding for text within layout rects
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
	if !isSmallScreen() {
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
func (sb *NodeSidebar) layoutSection(y, w, btnX int, id, _ string, hasButtons bool) int {
	sb.rects["sec-"+id] = image.Rect(sidebarPad, y, w-sidebarPad, y+sidebarSectionH)
	y += sidebarSectionH
	if !sb.sectionOpen[id] {
		return y
	}
	if hasButtons {
		sb.rects[id+"-"] = image.Rect(btnX, y, btnX+sidebarBtnH, y+sidebarBtnH)
		sb.rects[id+"+"] = image.Rect(btnX+sidebarBtnH+sidebarGap, y, btnX+2*sidebarBtnH+sidebarGap, y+sidebarBtnH)
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
		// Parameter adjusters
		kindHasN := kind == "every_n_triggers" || kind == "skip_every_n"
		kindHasP := kind == "probability"
		if kindHasN {
			sb.rects["ln-"] = image.Rect(btnX, y, btnX+sidebarBtnH, y+sidebarBtnH)
			sb.rects["ln+"] = image.Rect(btnX+sidebarBtnH+sidebarGap, y, btnX+2*sidebarBtnH+sidebarGap, y+sidebarBtnH)
			y += sidebarBtnH + sidebarGap
		} else if kindHasP {
			sb.rects["lp-"] = image.Rect(btnX, y, btnX+sidebarBtnH, y+sidebarBtnH)
			sb.rects["lp+"] = image.Rect(btnX+sidebarBtnH+sidebarGap, y, btnX+2*sidebarBtnH+sidebarGap, y+sidebarBtnH)
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
		// Pct adjusters
		sb.rects["gp-"] = image.Rect(btnX, y, btnX+sidebarBtnH, y+sidebarBtnH)
		sb.rects["gp+"] = image.Rect(btnX+sidebarBtnH+sidebarGap, y, btnX+2*sidebarBtnH+sidebarGap, y+sidebarBtnH)
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

	// Panel background with shadow
	drawPanelShadow(dst, panel, 3)
	drawRoundedRect(dst, panel, color.NRGBA{40, 40, 40, 230}, RadiusMD, true)
	drawRoundedRect(dst, panel, color.NRGBA{90, 90, 90, 100}, RadiusMD, false)

	// Fixed header
	sb.drawHeader(dst)

	// Separator below header
	sepY := sidebarPad + sidebarHeaderH
	drawRect(dst, image.Rect(sidebarPad, sepY, panel.Max.X-sidebarPad, sepY+1), color.NRGBA{90, 90, 90, 100}, true)

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
	sepColor := color.NRGBA{90, 90, 90, 100}
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

	valColX := panel.Min.X + panel.Dx()*45/100

	// Volume section
	if showVol {
		sb.drawSectionHeader(dst, "sec-vol", "Volume", "vol", sepColor, secTextOffY)
		if sb.sectionOpen["vol"] {
			if r := sb.rects["vol-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				DrawTextAtScale(dst, "Vol", sidebarPad+2, y, sidebarTextScale)
				pct := int(math.Round(volVal * 100))
				DrawTextAtScale(dst, fmt.Sprintf("%d%%", pct), valColX, y, sidebarTextScale)
				sb.drawBtn(dst, "vol-")
				sb.drawBtn(dst, "vol+")
			}
		}
	}
	// Pitch section
	if showPitch {
		sb.drawSectionHeader(dst, "sec-pit", "Pitch", "pit", sepColor, secTextOffY)
		if sb.sectionOpen["pit"] {
			if r := sb.rects["pit-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				DrawTextAtScale(dst, "Pitch", sidebarPad+2, y, sidebarTextScale)
				DrawTextAtScale(dst, fmt.Sprintf("%+d", int(pitVal)), valColX, y, sidebarTextScale)
				sb.drawBtn(dst, "pit-")
				sb.drawBtn(dst, "pit+")
			}
		}
	}
	// Duration section
	if showDur {
		sb.drawSectionHeader(dst, "sec-dur", "Duration", "dur", sepColor, secTextOffY)
		if sb.sectionOpen["dur"] {
			if r := sb.rects["dur-"]; !r.Empty() && sb.inViewport(r) {
				y := r.Min.Y + textOffY
				DrawTextAtScale(dst, "Dur", sidebarPad+2, y, sidebarTextScale)
				DrawTextAtScale(dst, fmt.Sprintf("%.2fx", durVal), valColX, y, sidebarTextScale)
				sb.drawBtn(dst, "dur-")
				sb.drawBtn(dst, "dur+")
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
				// Parameter controls
				switch mn.Params.LogicKind {
				case "every_n_triggers", "skip_every_n":
					sb.drawBtn(dst, "ln-")
					sb.drawBtn(dst, "ln+")
					if lnRect := sb.rects["ln-"]; !lnRect.Empty() && sb.inViewport(lnRect) {
						DrawTextAtScale(dst, fmt.Sprintf("N: %d", mn.Params.LogicN), sidebarPad+2, lnRect.Min.Y+textOffY, sidebarTextScale)
					}
				case "probability":
					sb.drawBtn(dst, "lp-")
					sb.drawBtn(dst, "lp+")
					if lpRect := sb.rects["lp-"]; !lpRect.Empty() && sb.inViewport(lpRect) {
						DrawTextAtScale(dst, fmt.Sprintf("P: %.1f", mn.Params.LogicP), sidebarPad+2, lpRect.Min.Y+textOffY, sidebarTextScale)
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
				sb.drawBtn(dst, "gp-")
				sb.drawBtn(dst, "gp+")
				if haveNode {
					if gpRect := sb.rects["gp-"]; !gpRect.Empty() && sb.inViewport(gpRect) {
						DrawTextAtScale(dst, fmt.Sprintf("Pct: %.0f%%", mn.Params.GroovePct*100), sidebarPad+2, gpRect.Min.Y+textOffY, sidebarTextScale)
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
	if !isSmallScreen() {
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
			drawRect(dst, r, color.NRGBA{80, 80, 80, 200}, true)
			drawRect(dst, r, color.NRGBA{120, 120, 120, 180}, false)
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

// drawHeader renders the fixed header with instrument info + grid coordinates.
func (sb *NodeSidebar) drawHeader(dst *ebiten.Image) {
	g := sb.game
	headerRect := sb.rects["header"]
	if headerRect.Empty() {
		return
	}
	hx := headerRect.Min.X
	hy := headerRect.Min.Y + (sidebarHeaderH-int(float64(TextHeight())*sidebarTextScale))/2

	// Always draw a color swatch and append grid coordinates (i,j).
	swatchSz := 14
	swatchCol := color.Color(color.RGBA{180, 180, 180, 255}) // default grey
	label := "Node"

	if row, ok := g.nodeRows[sb.node.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
		dr := g.drum.Rows[row]
		swatchCol = dr.Color
		label = dr.Name
	}

	swatchRect := image.Rect(hx, hy+1, hx+swatchSz, hy+1+swatchSz)
	drawRect(dst, swatchRect, swatchCol, true)
	textX := hx + swatchSz + 6

	coordStr := fmt.Sprintf("%s (%d,%d)", label, sb.node.I, sb.node.J)
	DrawTextAtScale(dst, coordStr, textX, hy, sidebarTextScale)
}

// drawSectionHeader draws a collapsible section header.
func (sb *NodeSidebar) drawSectionHeader(dst *ebiten.Image, rectID, label, sectionID string, sepColor color.Color, textOffY int) {
	r, ok := sb.rects[rectID]
	if !ok || r.Empty() || !sb.inViewport(r) {
		return
	}
	// Separator line above
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), sepColor, true)

	// Chevron
	chevron := ">"
	if sb.sectionOpen[sectionID] {
		chevron = "v"
	}
	DrawTextAtScale(dst, chevron+" "+label, r.Min.X+2, r.Min.Y+textOffY, sidebarTextScale)
}

// drawExpandTab draws the collapsed sidebar expand tab.
func (sb *NodeSidebar) drawExpandTab(dst *ebiten.Image) {
	r := sb.expandTabRect()
	drawRoundedRect(dst, r, color.NRGBA{40, 40, 40, 200}, 4, true)
	// Draw >> icon
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	DrawTextAt(dst, ">>", cx-TextWidth(">"), cy-TextHeight()/2)
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
