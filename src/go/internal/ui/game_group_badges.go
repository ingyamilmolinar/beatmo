package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Colors ─────────────────────────────────────────────────────────────────

// groupColor deterministically maps a GroupID onto the shared custom palette
// (customPalette, theme.go — sourced from DESIGN.md's
// instrumentFallbackPalette: block, the same token-ratchet-safe cycle
// sampler_playhead.go's samplerPlayheadColor uses) so consecutive groups get
// visually distinct colors with no new color literals. Falls back to the
// accent token if the palette is somehow empty (defensive; never happens in
// production — buildCustomPalette always yields a non-empty slice).
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

// groupRingColor resolves the display color for a group-membership ring:
// groupColor(gid) at AlphaStrong when gid is the currently OPEN GroupMenu's
// group, else AlphaSubtle. Shared by both node-layer and fallback-debug-mode
// ring drawing paths (grid_pane_draw.go) so they can't diverge on which
// alpha bucket a given membership ring uses.
func (g *Game) groupRingColor(gid model.GroupID) color.NRGBA {
	alpha := uint8(AlphaSubtle)
	if g.groupMenu != nil && g.groupMenu.IsOpen() && g.groupMenu.GroupID() == gid {
		alpha = AlphaStrong
	}
	return WithAlpha(g.groupColor(gid), alpha)
}

// ─── Ring geometry (multi-membership) ──────────────────────────────────────

const (
	// groupRingBaseGrow / groupRingStepGrow: the Nth (0-indexed) concurrent
	// membership ring on a node grows groupRingBaseGrow+N*groupRingStepGrow
	// px past the node body — one radius step further out than the last, so
	// up to groupRingMaxCount rings nest concentrically without overlapping.
	groupRingBaseGrow = 6
	groupRingStepGrow = 4
	// groupRingMaxCount caps the number of concentric rings drawn per node;
	// a node in more memberships than this only shows its first 3 (ascending
	// GroupID).
	groupRingMaxCount = 3
)

// rebuildGroupMembershipScratch rebuilds g.groupMembershipScratch — NodeID ->
// ascending-GroupID list — from g.graph.AllGroups() (already GroupID-sorted),
// reusing the map (cleared, not reallocated) across frames. Called once per
// drawGridNodes invocation, not per node, mirroring the groupMenuMembersScratch
// hoisting pattern directly above it in grid_pane_draw.go.
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

// groupBadgePadX / groupBadgePadY size the badge pill's text padding.
// groupBadgeOffsetPx is how far outward (past the member bbox's top-right
// corner) the badge anchors. groupBadgeGapPx is the vertical clearance
// enforced between two collision-resolved (stacked) badges.
const (
	groupBadgePadX     = 8
	groupBadgePadY     = 4
	groupBadgeOffsetPx = 6
	groupBadgeGapPx    = 4
)

// groupBadgeRect is one computed badge placement for a group: its id (for
// hit-testing / color lookup) and its final screen-space rect (after
// collision resolution).
type groupBadgeRect struct {
	gid  model.GroupID
	rect image.Rectangle
}

// groupMemberScreenBBox returns the screen-space bounding box over every
// VISIBLE member of ids (nodes not resolvable via g.nodesByID — e.g. deleted
// out from under a stale group snapshot — are skipped). ok is false when no
// member resolved to a live node.
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

// groupMemberScreenRects returns the screen-space rect for every VISIBLE
// member of ids (mirroring groupMemberScreenBBox's node-resolution and
// Floor/Ceil conversion), each clipped to the grid-pane viewport
// [0,gridW]×[0,gridH]. A member with NO onscreen portion at all is omitted;
// a member that's only PARTIALLY offscreen keeps its visible sliver rather
// than being dropped or left unclipped — task-placement-v2's placeGroupMenu
// tests overlap against these per-member rects (not the coarse union bbox
// groupMemberScreenBBox returns), so an over-wide unclipped rect for a
// partly-offscreen member would needlessly starve every placement candidate
// of room it doesn't actually occupy onscreen.
//
// scratch is a reused, caller-owned backing slice (truncated via [:0], not
// reallocated) — mirrors the groupMenuMembersScratch/groupMembershipScratch
// per-frame-alloc-neutral pattern used elsewhere in this file. Pass nil on
// first use; store the returned slice back into the scratch field.
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

// groupBadgeRects computes, deterministically (ascending GroupID — the order
// g.graph.AllGroups() already returns), the screen-space rect for every
// group's floating badge: anchored a few px outward from the top-right
// corner of the group's member bounding box, pushed straight down below any
// already-placed badge it would otherwise overlap (repeated until clear).
// Groups whose member bbox is fully outside the grid pane viewport are
// skipped entirely (no clamp-to-edge — brief §3).
//
// THIS IS THE SINGLE SOURCE for badge placement: both the draw layer
// (drawGridGroupBadges) and hit-testing (groupBadgeAt) call this, so the two
// can never diverge — computing rects only in the draw pass would be a frame
// too late for the input path, which runs before Draw each frame.
func (g *Game) groupBadgeRects() []groupBadgeRect {
	groups := g.graph.AllGroups()
	if len(groups) == 0 {
		return nil
	}
	gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
	viewport := image.Rect(0, 0, gridW, gridH)
	reserved := g.gridHelpReservedRect()

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

		// Non-overlap: push straight down below any already-placed badge
		// this candidate collides with, repeating until clear of all of
		// them (deterministic — processed in ascending gid order).
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

// groupBadgeAt returns the GroupID whose badge rect contains (x, y), if any.
// Shared hit-test entry point for both the desktop click-release path
// (game_input_editor.go's handleEditor) and the touch path
// (game_input_touch.go's handleTapInGrid).
func (g *Game) groupBadgeAt(x, y int) (model.GroupID, bool) {
	pt := image.Pt(x, y)
	for _, b := range g.groupBadgeRects() {
		if pt.In(b.rect) {
			return b.gid, true
		}
	}
	return 0, false
}

// hoveredGroupBadge returns the GroupID whose badge rect contains the current
// cursor position (task-enabled-status brief, Part B) — desktop hover only.
// Touch has no hover state: tapping a badge opens its menu via
// openGroupMenuForBadge, which already strengthens the member rings (see
// groupRingColor), so touch is covered by that click path instead.
func (g *Game) hoveredGroupBadge() (model.GroupID, bool) {
	x, y := cursorPosition()
	return g.groupBadgeAt(x, y)
}

// groupExternalHighlightGid resolves the GroupID (if any) whose members get
// the Part-B external highlight ring this frame: the OPEN GroupMenu's group
// takes priority — mirroring groupRingColor's existing "open menu draws
// strong" precedent — falling back to the hovered badge's group. Callers
// (drawGridNodes) hoist this ONCE per frame, not per node.
func (g *Game) groupExternalHighlightGid() (model.GroupID, bool) {
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		return g.groupMenu.GroupID(), true
	}
	return g.hoveredGroupBadge()
}

// groupBadgeFillColor resolves a badge's fill color: the group's own color at
// AlphaStrong while its badge is hovered (Part B's "brighten one alpha
// bucket" affordance), else the baseline AlphaMedium. Shared by
// drawGridGroupBadges so the draw pass and any test/inspection code can't
// diverge on which bucket a hovered badge uses.
func (g *Game) groupBadgeFillColor(gid model.GroupID) color.NRGBA {
	gc := g.groupColor(gid)
	if hovered, ok := g.hoveredGroupBadge(); ok && hovered == gid {
		return WithAlpha(gc, AlphaStrong)
	}
	return WithAlpha(gc, AlphaMedium)
}

// openGroupMenuForBadge opens gid's menu in response to a badge click,
// preserving an already-open PROVISIONAL menu's provisional-ness when gid is
// that SAME group (OpenAtProvisional) — otherwise a badge click landing, by
// coincidence, on the still-unsaved group whose own menu is already open
// would silently flip provisional=false with no Save ever having run,
// leaking an uncommitted "Group N" with no undo step for its creation.
// Mirrors handleMarquee's identical reuse decision (game_marquee.go). Every
// badge-click call site (desktop release path, desktop shift-press guard,
// touch path) must go through this so the rule can't drift between them.
func (g *Game) openGroupMenuForBadge(gid model.GroupID, x, y int) {
	if g.groupMenu.IsOpen() && g.groupMenu.provisional && g.groupMenu.GroupID() == gid {
		g.groupMenu.OpenAtProvisional(gid, x, y)
		return
	}
	g.groupMenu.OpenAt(gid, x, y)
}

// drawGridGroupBadges renders every group's floating badge: a small
// rounded-rect pill (mirroring drawGridCoordBadge's pill-plus-label idiom —
// grid_pane_draw.go) filled with the group's color at AlphaMedium, bordered
// at AlphaStrong, labeled with the group's own Name (user data — no i18n key
// needed). Registered as the "grid-group-badges" GridTree layer at
// GZGroupBadges (grid_tree.go).
func (g *Game) drawGridGroupBadges(dst *ebiten.Image) {
	for _, b := range g.groupBadgeRects() {
		grp, ok := g.graph.Group(b.gid)
		if !ok {
			continue
		}
		gc := g.groupColor(b.gid)
		drawRoundedRect(dst, b.rect, g.groupBadgeFillColor(b.gid), RadiusSM, true)
		drawRoundedRect(dst, b.rect, WithAlpha(gc, AlphaStrong), RadiusSM, false)
		ty := b.rect.Min.Y + (b.rect.Dy()-StyledTextHeight(RoleCaption))/2
		DrawTextStyled(dst, grp.Name, b.rect.Min.X+groupBadgePadX, ty, RoleCaption, colTextPrimary)
	}
}
