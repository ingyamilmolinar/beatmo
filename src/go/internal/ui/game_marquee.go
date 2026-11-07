package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

const marqueeMinDragPx = 4

// marqueeRect returns the normalized screen-space selection rectangle.
func (g *Game) marqueeRect() image.Rectangle {
	return image.Rect(g.marquee.startX, g.marquee.startY, g.marquee.curX, g.marquee.curY).Canon()
}

// Called from two spots in handleEditor's shift branch: the arm site (empty space, nothing active)
// and the continue/release site. Link-drag claims Shift+press on a node first.
func (g *Game) handleMarquee(left bool, x, y int) bool {
	if !g.marquee.active {
		g.marquee = marqueeDrag{active: true, startX: x, startY: y, curX: x, curY: y}
		return true
	}
	if left {
		g.marquee.curX, g.marquee.curY = x, y
		return true
	}
	// Release: select + create group.
	r := g.marqueeRect()
	g.marquee = marqueeDrag{}
	if r.Dx() < marqueeMinDragPx && r.Dy() < marqueeMinDragPx {
		return true // shift+click on empty space stays a no-op
	}
	ids := g.nodesInScreenRect(r)
	if len(ids) == 0 {
		return true
	}
	// Exactly the same node set as an existing group reopens it; subsets and supersets create a new one.
	if gid, ok := g.findGroupWithExactNodeSet(ids); ok {
		// Re-marqueeing the open provisional group keeps it provisional; any other match reopens committed.
		g.openGroupMenuForBadge(gid, x, y)
		g.logger.Debugf("[group] marquee reused existing group=%d nodes=%d", gid, len(ids))
		return true
	}
	gid, err := g.graph.CreateGroup("", ids)
	if err != nil {
		g.logger.Debugf("[group] marquee create failed: %v", err)
		return true
	}
	// Provisional until Save, so an abandoned selection never leaves an undo step.
	g.groupMenu.OpenAtProvisional(gid, x, y)
	g.logger.Debugf("[group] marquee created group=%d nodes=%d (provisional)", gid, len(ids))
	return true
}

// The lowest matching id (AllGroups is sorted); a length mismatch rejects subsets and supersets.
func (g *Game) findGroupWithExactNodeSet(ids []model.NodeID) (model.GroupID, bool) {
	want := make(map[model.NodeID]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	for _, grp := range g.graph.AllGroups() {
		if len(grp.NodeIDs) != len(want) {
			continue
		}
		match := true
		for _, m := range grp.NodeIDs {
			if !want[m] {
				match = false
				break
			}
		}
		if match {
			return grp.ID, true
		}
	}
	return 0, false
}

// nodesInScreenRect returns IDs of visible nodes whose screen rect intersects r.
func (g *Game) nodesInScreenRect(r image.Rectangle) []model.NodeID {
	var out []model.NodeID
	for _, n := range g.nodes {
		if mn, ok := g.graph.GetNodeByID(n.ID); !ok || mn.Type == model.NodeTypeInvisible {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		nr := image.Rect(int(x1), int(y1), int(x2), int(y2))
		if nr.Overlaps(r) {
			out = append(out, n.ID)
		}
	}
	return out
}

// drawGridMarquee renders the live selection rectangle (GZMarquee layer).
func (g *Game) drawGridMarquee(dst *ebiten.Image) {
	if !g.marquee.active {
		return
	}
	r := g.marqueeRect()
	if r.Empty() {
		return
	}
	// Tokens only: no inline color literals.
	fill := WithAlpha(TokenAccent(), AlphaFaint)
	border := WithAlpha(TokenAccent(), AlphaStrong)
	drawRect(dst, r, fill, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), border, true)
}
