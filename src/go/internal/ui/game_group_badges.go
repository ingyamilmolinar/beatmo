package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Colors ─────────────────────────────────────────────────────────────────

// From the shared palette, so groups get distinct colors with no new color literals.
func (g *Game) groupColor(gid model.GroupID) color.RGBA {
	n := len(customPalette)
	if n == 0 {
		return TokenAccent()
	}
	idx := int(gid) % n
	if idx < 0 {
		idx += n
	}
	return color.RGBAModel.Convert(customPalette[idx]).(color.RGBA)
}

// Shared by both ring paths so they cannot disagree on the alpha.
func (g *Game) groupRingColor(gid model.GroupID) color.NRGBA {
	alpha := uint8(AlphaSubtle)
	if g.groupMenu != nil && g.groupMenu.IsOpen() && g.groupMenu.GroupID() == gid {
		alpha = AlphaStrong
	}
	return WithAlpha(g.groupColor(gid), alpha)
}

// ─── Ring geometry (multi-membership) ──────────────────────────────────────

const (
	// Each ring one step further out, so up to groupRingMaxCount rings nest without overlap.
	groupRingBaseGrow = 6
	groupRingStepGrow = 4
	groupRingMaxCount = 3
)

// Once per drawGridNodes call; the map is cleared, not reallocated.
func (g *Game) rebuildGroupMembershipScratch() map[model.NodeID][]model.GroupID {
	scratch := g.groupMembershipScratch
	if scratch == nil {
		scratch = make(map[model.NodeID][]model.GroupID)
	} else {
		for k := range scratch {
			delete(scratch, k)
		}
	}
	for _, grp := range g.graph.AllGroups() {
		for _, nid := range grp.NodeIDs {
			scratch[nid] = append(scratch[nid], grp.ID)
		}
	}
	g.groupMembershipScratch = scratch
	return scratch
}

// ─── Badges ─────────────────────────────────────────────────────────────────

const (
	groupBadgePadX     = 8
	groupBadgePadY     = 4
	groupBadgeOffsetPx = 6
	groupBadgeGapPx    = 4
)

type groupBadgeRect struct {
	gid  model.GroupID
	rect image.Rectangle
}

// Members that no longer resolve (a stale group snapshot) are skipped.
func (g *Game) groupMemberScreenBBox(ids []model.NodeID) (bbox image.Rectangle, ok bool) {
	for _, id := range ids {
		n := g.nodesByID[id]
		if n == nil {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		r := image.Rect(int(math.Floor(x1)), int(math.Floor(y1)), int(math.Ceil(x2)), int(math.Ceil(y2)))
		if !ok {
			bbox, ok = r, true
			continue
		}
		bbox = bbox.Union(r)
	}
	return bbox, ok
}

// Each member clipped to the viewport, keeping a partly offscreen member's visible sliver:
// placeGroupMenu tests overlap against these rects. scratch is reused across calls.
func (g *Game) groupMemberScreenRects(ids []model.NodeID, scratch []image.Rectangle) []image.Rectangle {
	gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
	viewport := image.Rect(0, 0, gridW, gridH)
	out := scratch[:0]
	for _, id := range ids {
		n := g.nodesByID[id]
		if n == nil {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		r := image.Rect(int(math.Floor(x1)), int(math.Floor(y1)), int(math.Ceil(x2)), int(math.Ceil(y2)))
		r = r.Intersect(viewport)
		if r.Empty() {
			continue
		}
		out = append(out, r)
	}
	return out
}

// The single source for badge placement: draw and hit-testing both call it (input runs before
// Draw). Ascending GroupID; groups fully off-screen are skipped.
func (g *Game) groupBadgeRects() []groupBadgeRect {
	groups := g.graph.AllGroups()
	if len(groups) == 0 {
		return nil
	}
	gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
	viewport := image.Rect(0, 0, gridW, gridH)
	reserved := g.gridTopButtonsReservedRect() // clear the whole gear+folder+logs cluster

	out := make([]groupBadgeRect, 0, len(groups))
	for _, grp := range groups {
		bbox, ok := g.groupMemberScreenBBox(grp.NodeIDs)
		if !ok || !bbox.Overlaps(viewport) {
			continue
		}
		label := grp.Name
		w := TextWidth(label) + groupBadgePadX*2
		h := StyledTextHeight(RoleCaption) + groupBadgePadY*2

		x2 := bbox.Max.X + groupBadgeOffsetPx
		y1 := bbox.Min.Y - groupBadgeOffsetPx - h
		rect := image.Rect(x2-w, y1, x2, y1+h)

		// Deterministic: ascending gid order.
		for moved := true; moved; {
			moved = false
			for _, placed := range out {
				if rect.Overlaps(placed.rect) {
					dy := placed.rect.Max.Y + groupBadgeGapPx - rect.Min.Y
					rect = rect.Add(image.Pt(0, dy))
					moved = true
				}
			}
		}
		// Keep the badge clear of the settings gear's protected top-right zone.
		rect = avoidReservedRect(rect, reserved, gridW, gridH)
		out = append(out, groupBadgeRect{gid: grp.ID, rect: rect})
	}
	return out
}

func (g *Game) groupBadgeAt(x, y int) (model.GroupID, bool) {
	pt := image.Pt(x, y)
	for _, b := range g.groupBadgeRects() {
		if pt.In(b.rect) {
			return b.gid, true
		}
	}
	return 0, false
}

// Desktop only: touch has no hover, and a tapped badge opens its menu, which strengthens the rings.
func (g *Game) hoveredGroupBadge() (model.GroupID, bool) {
	x, y := cursorPosition()
	return g.groupBadgeAt(x, y)
}

// The open menu's group wins over the hovered badge's. Hoist it once per frame.
func (g *Game) groupExternalHighlightGid() (model.GroupID, bool) {
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		return g.groupMenu.GroupID(), true
	}
	return g.hoveredGroupBadge()
}

func (g *Game) groupBadgeFillColor(gid model.GroupID) color.NRGBA {
	gc := g.groupColor(gid)
	if hovered, ok := g.hoveredGroupBadge(); ok && hovered == gid {
		return WithAlpha(gc, AlphaStrong)
	}
	return WithAlpha(gc, AlphaMedium)
}

// Keeps an already-open provisional menu provisional when gid is the same group; otherwise a
// stray badge click commits an unsaved group with no undo step. Every badge-click path calls this.
func (g *Game) openGroupMenuForBadge(gid model.GroupID, x, y int) {
	if g.groupMenu.IsOpen() && g.groupMenu.provisional && g.groupMenu.GroupID() == gid {
		g.groupMenu.OpenAtProvisional(gid, x, y)
		return
	}
	g.groupMenu.OpenAt(gid, x, y)
}

func (g *Game) drawGridGroupBadges(dst *ebiten.Image) {
	for _, b := range g.groupBadgeRects() {
		grp, ok := g.graph.Group(b.gid)
		if !ok {
			continue
		}
		gc := g.groupColor(b.gid)
		badgeRad := fitRadius(RadiusControl, b.rect)
		drawRoundedRect(dst, b.rect, g.groupBadgeFillColor(b.gid), badgeRad, true)
		drawRoundedRect(dst, b.rect, WithAlpha(gc, AlphaStrong), badgeRad, false)
		ty := b.rect.Min.Y + (b.rect.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, grp.Name, b.rect.Min.X+groupBadgePadX, ty, RoleCaption, colTextPrimary)
	}
}
