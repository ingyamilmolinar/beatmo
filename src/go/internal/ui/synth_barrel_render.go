package ui

// A flat fade-stack of labels that scrolls with the wheel's sub-pixel offset; the soft edge dissolve keeps a
// glyph from hard-clipping at the lane edge.

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// RowH must match the wheel's detentPx so one step scrolls exactly one row; VisibleRows is forced odd.
type barrelLaneStyle struct {
	RowH        int
	VisibleRows int
	Accent      color.Color
	Emphasis    bool
}

// Plenty smooth for a short lane, without a per-pixel gradient's cost.
const barrelLaneFadeBands = 8

// barrelHairlineH is the accent hairline thickness in pixels.
const barrelHairlineH = 2

const synthLanePadX = 2

// Row i draws at centerY - i*rowH, so the larger value sits ABOVE centre as on a physical dial; the kinetic
// offset is ADDED, so a downward drag pulls the row above into centre and increases the value. Clipped to rect.
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

	// Drawn first so the centre label composites on top.
	hairlineY := centerY - barrelHairlineH/2 + offset
	hairline := image.Rect(rect.Min.X, hairlineY, rect.Max.X, hairlineY+barrelHairlineH)
	if hl := hairline.Intersect(rect); !hl.Empty() {
		drawRect(dst, hl, WithAlphaFromColor(style.Accent, AlphaStrong), true)
	}

	// Drawn BEFORE the labels: the ±1 row sits inside the fade band on compact densities, and a wash painted
	// over composited text crushed its contrast.
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
		// Caption, then Micro, then an ellipsis, so a wide string in the narrow resolution lane never overflows.
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
