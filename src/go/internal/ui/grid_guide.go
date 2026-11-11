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
	// Num/Den is the reduced fraction of the A→B span at the commit cell
	// (GuideEdge only): a node placed here sits Num/Den of the way from
	// Edge.A to Edge.B.
	Num, Den int
}

// guideSnapRadiusWorld converts the screen-pixel snap radius (density token)
// to world units at the current zoom. Screen-space definition makes snapping
// zoom-proportional: zoomed out the radius spans many cells (coarse musical
// snapping), zoomed in it spans less than one cell (exact placement).
func (g *Game) guideSnapRadiusWorld() float64 {
	if g.cam.Scale <= 0 {
		return 0
	}
	return float64(Profile().DensityValues().GuideSnapRadius) / g.cam.Scale
}

// guideTargetAt resolves the guide target for a screen-space cursor position:
// the nearest node within the snap radius, else the best interior cell on the
// nearest edge within the radius, else the free cell under the cursor.
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

// guideTargetCached memoizes guideTargetAt per (draw pass, cursor) so the
// crosshair layer and the cursor label share one node/edge scan per draw.
// The memo key is g.frame, but g.frame only advances while Playing() (see
// game_update.go), so it cannot by itself detect "a new draw pass happened
// while stopped". drawGridPane resets g.guideMemoFrame to -1 at the start of
// every draw pass (before any drawGrid* method — including this one's callers
// — runs), which forces the first guideTargetCached call each pass to
// recompute regardless of whether g.frame changed; the second call within the
// same pass then hits the cache since g.frame is stable across both calls.
// g.frame is a monotonically non-decreasing int64 that starts at 0, so -1 is
// never a value it legitimately takes — it's safe as a "not computed this
// pass" sentinel.
// DRAW-ONLY: input paths must keep calling guideTargetAt directly — they run
// during Update, before camera pan/zoom mutate the transform for this frame,
// so a memo taken there would go stale by draw time.
func (g *Game) guideTargetCached(mx, my int) GuideTarget {
	if g.guideMemoFrame == g.frame && g.guideMemoX == mx && g.guideMemoY == my {
		return g.guideMemo
	}
	t := g.guideTargetAt(mx, my)
	g.guideMemoFrame, g.guideMemoX, g.guideMemoY = g.frame, mx, my
	g.guideMemo = t
	return t
}

// guideEdgeTarget finds the nearest edge within radius (perpendicular distance
// to the axis-aligned segment; ties broken by smaller span) and picks the
// interior cell whose reduced fraction of the span has the smallest
// denominator among the cells within radius of the cursor's along-axis
// position (nearest wins denominator ties). Edges are orthogonal-only, so all
// geometry is axis-aligned.
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

// guideAimState tracks the mobile aiming crosshair: a touch that starts within
// the guide snap radius of a node/edge shows the aim crosshair + coordinate
// label while it stays stationary, so a precise tap can be lined up. It is
// visual only — it neither suppresses camera pan nor places a node; the moment
// the finger moves past the tap slop the aim clears and the drag pans, and
// placement happens solely on a stationary tap (handleTapInGrid). Desktop mouse
// input never arms it (touch-originated input only).
type guideAimState struct {
	active   bool
	prevLeft bool
	startX   int
	startY   int
}

// guideAimTouchForTest stands in for touchOverrideActive in functional tests:
// SetInputForTest forces the real touch override off (updateTouchOverride),
// so tests arm this seam instead.
var guideAimTouchForTest bool

// SetGuideAimTouchForTest simulates touch-originated input for the aiming
// gesture. Returns a restore func.
func SetGuideAimTouchForTest(v bool) func() {
	old := guideAimTouchForTest
	guideAimTouchForTest = v
	return func() { guideAimTouchForTest = old }
}

// guideAimTouchActive reports whether this frame's pointer input comes from a
// touch (the aiming gesture is touch-only; desktop pans on plain drags).
func guideAimTouchActive() bool { return touchOverrideActive || guideAimTouchForTest }

// updateGuideAim runs once per frame from Game.Update, before pan gating.
// Press edge near a circuit element arms aiming; movement past the tap slop
// marks it a drag; release commits the guide target (a short tap instead
// resolves through GestureTap → handleTapInGrid on the same frame).
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
		// Press edge: arm the visual aim (crosshair + coordinate label) when the
		// press lands on a circuit element, so a user lining up a precise tap can
		// see where the node would land. This is VISUAL ONLY — placement happens
		// exclusively on a stationary GestureTap (handleTapInGrid). A drag never
		// places; it pans (the camera owns pan, gated by its tap-slop deadzone).
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
		// Moved past the tap slop → this gesture is a pan, not a precise tap.
		// Drop the visual aim so the crosshair/label stops tracking; the camera
		// takes over the pan. Nothing is placed.
		if abs(mx-g.guideAim.startX) >= tapMaxMovePx || abs(my-g.guideAim.startY) >= tapMaxMovePx {
			g.guideAim.active = false
		}
		return
	}
	// Release edge: clear. A stationary release is a tap — GestureTap →
	// handleTapInGrid owns placement; there is nothing to commit here.
	g.guideAim.active = false
}

// commitGuideTarget places/selects at the guide target — shared by the aiming
// gesture's release and handleTapInGrid's empty-space branch.
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
