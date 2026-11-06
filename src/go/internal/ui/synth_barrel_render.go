package ui

// synth_barrel_render.go — the MODERN FLAT lane renderer for an armed synth
// control's BarrelWheel. This is deliberately NOT the skeuomorphic cylinder
// barrel popup (no gradient-shaded cylinder body, no ridge/rail chrome, no
// specular highlight, no keycap bevel): just a short fade-stack of text
// labels that scrolls with the wheel's sub-pixel offset, framed by a thin
// accent hairline under the center value and a soft top/bottom dissolve so
// the stack doesn't hard-clip mid-glyph at the lane edges.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// barrelLaneStyle configures one lane's flat render pass. RowH is the
// pixel pitch between adjacent detent labels (matches the wheel's
// detentPx so a full step scrolls exactly one row); VisibleRows is how many
// labels to draw total (center + N each side — forced odd). Accent tints
// the center row (when Emphasis) and always tints the hairline marker.
type barrelLaneStyle struct {
	RowH        int
	VisibleRows int
	Accent      color.Color
	Emphasis    bool
}

// barrelLaneFadeBands is the number of gradient strips used for the top/
// bottom edge dissolve. The lane is short (a handful of rows), so this is
// plenty smooth without the allocation cost of a true per-pixel gradient.
const barrelLaneFadeBands = 8

// barrelHairlineH is the accent hairline thickness in pixels.
const barrelHairlineH = 2

// synthLanePadX is the horizontal inset labels are centered/measured within,
// so a lane's text never touches its own edge (let alone a neighbor's).
const synthLanePadX = 2

// drawBarrelLane renders w's current fade stack — VisibleRows labels sourced
// from b.labelAt(-n..+n), vertically centered in rect and shifted by
// w.OffsetPx() for sub-pixel kinetic motion — plus the center hairline and
// the edge fade. Labels whose binding returns "" (past the value/enum
// range) are skipped entirely.
//
// DIRECTION (both signs matter, and they are a pair — see
// synth_barrel_direction_test.go):
//
//   - Row i draws at centerY MINUS i*rowH, so labelAt(+1) — the LARGER value,
//     and the COARSER resolution rung — sits ABOVE the center selector, the
//     way a physical dial reads.
//   - The kinetic offset is ADDED, so the stack translates with the pointer:
//     BarrelWheel.DragTo accumulates the raw pointer dy, so a downward drag
//     grows offsetPx and slides every label down by the same pixel count,
//     pulling the higher rows above the center down into it. That is also why
//     a downward drag/scroll INCREASES the value: advance() crosses a detent
//     into stepBy(+1) at exactly the moment the row above reaches center.
//
// Everything this function draws is clipped to rect (via SubImage): this is
// what keeps one lane's stack from bleeding into its neighbor AND from
// bleeding upward into the card's header band above it — VisibleRows is
// sized for the IDEAL case where the body has room for every row; a shorter
// body (narrow section, small density) simply clips the outer rows instead
// of painting over content outside the lane.
func drawBarrelLane(dst *ebiten.Image, rect image.Rectangle, w *BarrelWheel, b barrelBinding, style barrelLaneStyle) {
	if dst == nil || rect.Empty() || w == nil || b == nil {
		return
	}
	clip, ok := dst.SubImage(rect).(*ebiten.Image)
	if !ok || clip == nil {
		return
	}
	dst = clip

	rows := style.VisibleRows
	if rows < 1 {
		rows = 1
	}
	if rows%2 == 0 {
		rows++ // VisibleRows is center + N each side — always odd.
	}
	half := rows / 2
	rowH := style.RowH
	if rowH < 1 {
		rowH = 1
	}

	centerY := rect.Min.Y + rect.Dy()/2
	offset := int(w.OffsetPx())

	// Center hairline: a thin accent bar across the full lane width, drawn
	// first so the center label composites on top of it. Always present —
	// only the label's own tint is conditional on style.Emphasis.
	hairlineY := centerY - barrelHairlineH/2 + offset
	hairline := image.Rect(rect.Min.X, hairlineY, rect.Max.X, hairlineY+barrelHairlineH)
	if hl := hairline.Intersect(rect); !hl.Empty() {
		drawRect(dst, hl, WithAlphaFromColor(style.Accent, AlphaStrong), true)
	}

	// Soft top/bottom fade toward the card surface so the stack dissolves
	// at the lane edges rather than hard-clipping a half-visible glyph.
	// Drawn BEFORE the label loop below (not after): the ±1 neighbor row
	// sits close enough to the lane edge, on the comfortable/compact
	// densities, to fall inside this band's reach. Painting the gradient
	// first and the labels on top keeps every drawn label at its full
	// intended alpha — a wash painted over already-composited text was
	// silently crushing the ±1 row's contrast down near the background
	// on desktop (fine on the taller mobile/spacious lane, where ±1 sits
	// past the band, but not here) even though its alpha bucket was
	// correct. The truly-clipped outermost row (if any) is still hard-cut
	// by the rect SubImage clip regardless of draw order, so this loses
	// no dissolve behavior that mattered.
	fadeH := rowH
	if fadeH > rect.Dy()/2 {
		fadeH = rect.Dy() / 2
	}
	if fadeH > 0 {
		transparent := WithAlpha(colSurface2, 0)
		top := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+fadeH)
		fillVerticalGradient(dst, top, colSurface2, transparent, barrelLaneFadeBands)
		bot := image.Rect(rect.Min.X, rect.Max.Y-fadeH, rect.Max.X, rect.Max.Y)
		fillVerticalGradient(dst, bot, transparent, colSurface2, barrelLaneFadeBands)
	}

	maxTextW := rect.Dx() - 2*synthLanePadX
	for i := -half; i <= half; i++ {
		label := b.labelAt(i)
		if label == "" {
			continue
		}
		// Fix C: fit the label within its own lane — RoleCaption first,
		// falling back to the narrower RoleMicro, then truncating with an
		// ellipsis as a last resort — so a wide string like "±0.001s" in a
		// narrow resolution lane can never overflow into the value lane.
		role := RoleCaption
		if StyledTextWidth(label, role) > maxTextW {
			role = RoleMicro
		}
		fitted := truncCaptionStyled(label, maxTextW, role)
		if fitted == "" {
			continue // lane too narrow even for an ellipsis-only fit — skip
		}
		col := color.Color(WithAlpha(colTextSecondary, barrelNeighborAlpha(i)))
		if i == 0 && style.Emphasis {
			col = style.Accent
		} else if i == 0 {
			col = colTextPrimary
		}
		y := centerY - i*rowH + offset - StyledTextHeight(role)/2
		tw := StyledTextWidth(fitted, role)
		x := rect.Min.X + (rect.Dx()-tw)/2
		DrawTextStyled(dst, fitted, x, y, role, col)
	}
}

// barrelNeighborAlpha returns the text alpha for a non-center row, falling
// off with distance from center so the fade stack reads as receding.
func barrelNeighborAlpha(offset int) uint8 {
	d := offset
	if d < 0 {
		d = -d
	}
	switch d {
	case 1:
		return AlphaStrong
	case 2:
		return AlphaMedium
	default:
		return AlphaSubtle
	}
}
