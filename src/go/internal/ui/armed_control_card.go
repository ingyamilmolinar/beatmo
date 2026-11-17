package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// armed_control_card.go — the ARMED control's grown card, shared by the Synth
// and Sampler tabs so an armed card is pixel-identical on both at equal size.
//
// Layout mirrors ArmedControlEditor.SetCardRect exactly: a header band one
// wheel row tall carrying the param name, then the body split into an optional
// left resolution lane and a wider right value lane with a gutter between them.
// `r` is the GROWN rect the caller's grow/reflow pass already assigned.
//
// This bypasses the per-card ParamCard sprite cache on purpose: the lanes
// animate every frame, and a cached sprite would freeze mid-scroll.
func drawArmedControlCard(dst *ebiten.Image, r image.Rectangle, name string,
	e *ArmedControlEditor, valB, resB barrelBinding) {
	if e == nil || r.Empty() {
		return
	}
	rad := RadiusControl
	// Flat-tonal armed card: one filled body plus the accent bar marking it as
	// the armed control. No shell/bevel/rim.
	capR := r
	drawFlatButtonBody(dst, capR, TokenSurface3(), rad)
	drawFlatSelectionBar(dst, capR, colConceptStroke, selectionEdgeBottom, 1)

	e.SetCardRect(capR)

	dv2 := Profile().DensityValues()
	label := truncCaptionStyled(name, capR.Dx()-2*SpaceXS, RoleMicro)
	nameY := capR.Min.Y + (dv2.SynthWheelRowH-StyledTextHeight(RoleMicro))/2
	DrawTextStyled(dst, label, capR.Min.X+SpaceXS, nameY, RoleMicro, TokenTextSecondary())

	style := barrelLaneStyle{
		RowH:        dv2.SynthWheelRowH,
		VisibleRows: dv2.SynthWheelVisibleRows,
		Accent:      colConceptStroke,
		Emphasis:    true,
	}
	if vr := e.ValueLaneRect(); !vr.Empty() {
		drawBarrelLane(dst, vr, e.value(), valB, style)
	}
	if rr := e.ResLaneRect(); !rr.Empty() && resB != nil {
		drawBarrelLane(dst, rr, e.res(), resB, style)
	}
	// A subtle 1px hairline in the gutter between the two lanes so their label
	// stacks read as two distinct columns rather than merging.
	if gap := e.LaneGapRect(); !gap.Empty() {
		divX := gap.Min.X + gap.Dx()/2
		div := image.Rect(divX, gap.Min.Y+SpaceXS, divX+1, gap.Max.Y-SpaceXS)
		drawRect(dst, div, WithAlphaFromColor(TokenBorderMedium(), AlphaSubtle), true)
	}
}
