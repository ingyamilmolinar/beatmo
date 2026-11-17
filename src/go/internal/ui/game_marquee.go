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

// handleMarquee owns the Shift+drag-from-empty-space gesture. handleEditor's
// shift branch calls this from exactly two spots: (1) the arm site, only
// reached when left==true, linkDrag/marquee are both inactive, shift is held,
// and the press landed on empty grid space (nodeAtScreen == nil) — so arming
// here is unconditional, no press-edge check needed (link-drag claims
// Shift+press ON a node before this ever runs); (2) the continue/release
// site, reached on every subsequent frame while g.marquee.active is true.
// Always returns true (kept bool for symmetry with other gesture handlers
// and so callers read as "consumed").
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
	// Reuse instead of duplicate: marquee-selecting the EXACT same node set
	// (order-independent) an existing group already contains reopens THAT
	// group instead of creating a new one. Subset/superset selections are
	// NOT matches — they fall through to CreateGroup below.
	if gid, ok := g.findGroupWithExactNodeSet(ids); ok {
		// openGroupMenuForBadge's provisional-preserving reopen rule applies
		// identically here: re-marqueeing the same set as the still-open
		// provisional group keeps it provisional (openAt's same-id guard
		// skips the discard-and-recreate path); any other match (committed,
		// or a DIFFERENT still-open provisional group) reopens non-
		// provisional — closing it must not delete it.
		g.openGroupMenuForBadge(gid, x, y)
		g.logger.Debugf("[group] marquee reused existing group=%d nodes=%d", gid, len(ids))
		return true
	}
	gid, err := g.graph.CreateGroup("", ids)
	if err != nil {
		g.logger.Debugf("[group] marquee create failed: %v", err)
		return true
	}
	// The group is provisional until the user presses Save in the menu: no
	// emitGroupCreated here (moved to GroupMenu.save()) so an abandoned
	// marquee selection never produces an undo step. See
	// GroupMenu.OpenAtProvisional / Close for the full discard lifecycle.
	g.groupMenu.OpenAtProvisional(gid, x, y)
	g.logger.Debugf("[group] marquee created group=%d nodes=%d (provisional)", gid, len(ids))
	return true
}

// findGroupWithExactNodeSet returns the lowest GroupID whose NodeIDs form the
// EXACT same set as ids (order-independent, same cardinality) — used by
// handleMarquee's release path to reuse an existing group instead of
// duplicating it. g.graph.AllGroups() is already ascending-GroupID sorted, so
// the first match found is the lowest id; subset/superset selections never
// match (the length check rejects them outright).
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
	// Translucent accent fill + 1px border, tokens only (token discipline
	// ratchet: no inline color literals). Same accent token family used
	// elsewhere for selection/focus overlays.
	fill := WithAlpha(TokenAccent(), AlphaFaint)
	border := WithAlpha(TokenAccent(), AlphaStrong)
	drawRect(dst, r, fill, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), border, true)
	drawRect(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), border, true)
}
