package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/utils"
)

// GuideKind classifies what the grid guide snapped to.
type GuideKind int

const (
	// GuideCell is a free grid cell — no circuit element within the snap radius.
	GuideCell GuideKind = iota
	// GuideNode is an existing node within the snap radius.
	GuideNode
	// GuideEdge is an interior cell on an existing edge within the snap radius.
	GuideEdge
)

// GuideTarget is the resolved commit target for the grid guide crosshair.
type GuideTarget struct {
	Kind GuideKind
	I, J int     // commit cell (always valid)
	Node *uiNode // set when Kind == GuideNode
	Edge *uiEdge // set when Kind == GuideEdge
	// GuideEdge only: a node here sits Num/Den of the way from Edge.A to Edge.B.
	Num, Den int
}

// Defined in screen pixels so snapping scales with zoom: coarse when zoomed out, exact when in.
func (g *Game) guideSnapRadiusWorld() float64 {
	if g.cam.Scale <= 0 {
		return 0
	}
	return float64(Profile().DensityValues().GuideSnapRadius) / g.cam.Scale
}

// The nearest node in the snap radius, else the best interior cell on the nearest edge, else the
// free cell.
func (g *Game) guideTargetAt(mx, my int) GuideTarget {
	wx := (float64(mx) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(my-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	radius := g.guideSnapRadiusWorld()

	var bestNode *uiNode
	bestNodeD := radius
	for _, n := range g.nodes {
		if d := math.Hypot(n.X-wx, n.Y-wy); d <= bestNodeD {
			bestNodeD, bestNode = d, n
		}
	}
	if bestNode != nil {
		return GuideTarget{Kind: GuideNode, I: bestNode.I, J: bestNode.J, Node: bestNode}
	}

	if t, ok := g.guideEdgeTarget(wx, wy, radius); ok {
		return t
	}

	_, _, i, j := g.grid.Snap(wx, wy)
	return GuideTarget{Kind: GuideCell, I: i, J: j}
}

// One scan per draw, shared by the crosshair and the label. g.frame only advances while playing,
// so drawGridPane resets the memo to -1 each pass. DRAW-ONLY: input runs before this frame's camera
// changes, so a memo taken there would be stale.
func (g *Game) guideTargetCached(mx, my int) GuideTarget {
	if g.guideMemoFrame == g.frame && g.guideMemoX == mx && g.guideMemoY == my {
		return g.guideMemo
	}
	t := g.guideTargetAt(mx, my)
	g.guideMemoFrame, g.guideMemoX, g.guideMemoY = g.frame, mx, my
	g.guideMemo = t
	return t
}

// Nearest edge by perpendicular distance (smaller span on ties), then the in-radius cell with the
// smallest reduced denominator (nearest on ties). Edges are axis-aligned.
func (g *Game) guideEdgeTarget(wx, wy, radius float64) (GuideTarget, bool) {
	unit := g.grid.Unit()
	if unit <= 0 {
		return GuideTarget{}, false
	}
	var best *uiEdge
	bestDist := radius
	bestSpan := 0
	for i := range g.edges {
		e := &g.edges[i]
		a, b := e.A, e.B
		var dist float64
		var span int
		switch {
		case a.I == b.I: // vertical
			span = abs(b.J - a.J)
			along := math.Min(math.Max(wy, math.Min(a.Y, b.Y)), math.Max(a.Y, b.Y))
			dist = math.Hypot(wx-a.X, wy-along)
		case a.J == b.J: // horizontal
			span = abs(b.I - a.I)
			along := math.Min(math.Max(wx, math.Min(a.X, b.X)), math.Max(a.X, b.X))
			dist = math.Hypot(wy-a.Y, wx-along)
		default:
			continue
		}
		if span < 2 { // adjacent nodes: no interior cells
			continue
		}
		if dist < bestDist || (dist == bestDist && (best == nil || span < bestSpan)) {
			bestDist, best, bestSpan = dist, e, span
		}
	}
	if best == nil {
		return GuideTarget{}, false
	}

	a, b := best.A, best.B
	vertical := a.I == b.I
	var aU, bU int
	var alongW float64
	if vertical {
		aU, bU, alongW = a.J, b.J, wy
	} else {
		aU, bU, alongW = a.I, b.I, wx
	}
	span := abs(bU - aU)
	dir := 1
	if bU < aU {
		dir = -1
	}
	loU, hiU := min(aU, bU)+1, max(aU, bU)-1
	alongU := alongW / unit
	radiusU := radius / unit
	kMin := max(int(math.Ceil(alongU-radiusU)), loU)
	kMax := min(int(math.Floor(alongU+radiusU)), hiU)
	if kMin > kMax {
		// No interior cell within radius along the axis: nearest interior cell.
		k := min(max(int(math.Round(alongU)), loU), hiU)
		kMin, kMax = k, k
	}
	bestK := kMin
	bestDen := span + 1
	bestOff := math.MaxFloat64
	for k := kMin; k <= kMax; k++ {
		off := (k - aU) * dir // 1..span-1 measured from A
		den := span / utils.GCD(off, span)
		d := math.Abs(float64(k) - alongU)
		if den < bestDen || (den == bestDen && d < bestOff) {
			bestDen, bestK, bestOff = den, k, d
		}
	}
	off := (bestK - aU) * dir
	gcd := utils.GCD(off, span)
	ci, cj := a.I, bestK
	if !vertical {
		ci, cj = bestK, a.J
	}
	if n := g.nodeAt(ci, cj); n != nil {
		// Cell already occupied (crossing circuit / imported shape).
		return GuideTarget{Kind: GuideNode, I: n.I, J: n.J, Node: n}, true
	}
	return GuideTarget{Kind: GuideEdge, I: ci, J: cj, Edge: best, Num: off / gcd, Den: span / gcd}, true
}

// Visual only: a stationary touch near a circuit element shows the crosshair and label for a
// precise tap; a move past the slop pans, and only a stationary tap places. Touch input only.
type guideAimState struct {
	active   bool
	prevLeft bool
	startX   int
	startY   int
}

// SetInputForTest forces the real touch override off, so tests arm this instead.
var guideAimTouchForTest bool

func SetGuideAimTouchForTest(v bool) func() {
	old := guideAimTouchForTest
	guideAimTouchForTest = v
	return func() { guideAimTouchForTest = old }
}

// Touch only; desktop pans on plain drags.
func guideAimTouchActive() bool { return touchOverrideActive || guideAimTouchForTest }

// Before pan gating.
func (g *Game) updateGuideAim(mx, my int, left bool) {
	prev := g.guideAim.prevLeft
	g.guideAim.prevLeft = left
	if !guideAimTouchActive() {
		g.guideAim.active = false
		return
	}
	if globalTouchState.ActiveTouchCount() >= 2 || g.modalOverlayActive() ||
		g.landscapeUnsupported() || g.modes.Active() != GridModeNone ||
		g.longPressPopup || g.sidebar.IsOpen() || g.groupMenu.IsOpen() {
		g.guideAim.active = false
		return
	}
	if left && !prev {
		// Visual only: placement happens solely on a stationary GestureTap, and a drag pans.
		if my >= gridTopOffset() && g.split.InGridPane(mx, my) && !g.blocksAt(mx, my) {
			if t := g.guideTargetAt(mx, my); t.Kind != GuideCell {
				g.guideAim = guideAimState{active: true, prevLeft: true, startX: mx, startY: my}
			}
		}
		return
	}
	if !g.guideAim.active {
		return
	}
	if left {
		// Past the slop it is a pan: drop the aim, place nothing.
		if abs(mx-g.guideAim.startX) >= tapMaxMovePx || abs(my-g.guideAim.startY) >= tapMaxMovePx {
			g.guideAim.active = false
		}
		return
	}
	// A stationary release is a GestureTap, which owns placement.
	g.guideAim.active = false
}

func (g *Game) commitGuideTarget(x, y int) {
	t := g.guideTargetAt(x, y)
	if t.Kind == GuideNode {
		g.selectNodeOpenSidebar(t.Node)
		g.groupMenu.Close()
		return
	}
	n := g.tryAddNode(t.I, t.J, model.NodeTypeRegular)
	if n != nil {
		if g.sel != nil && g.sel != n {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
	}
	// Don't open the sidebar for new nodes (consistent with mouse behavior).
	g.sidebar.Close()
	g.groupMenu.Close()
}
