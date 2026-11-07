package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared by the Synth and Sampler tabs so an armed card is pixel-identical; mirrors SetCardRect's layout.
// Bypasses the ParamCard sprite cache on purpose: the lanes animate every frame.
func drawArmedControlCard(dst *ebiten.Image, r image.Rectangle, name string,
	e *ArmedControlEditor, valB, resB barrelBinding) {
	if e == nil || r.Empty() {
		return
	}
	rad := RadiusControl
	// Flat-tonal: one filled body plus the accent bar marking it armed.
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
	// A hairline in the gutter so the two label stacks read as separate columns.
	if gap := e.LaneGapRect(); !gap.Empty() {
		divX := gap.Min.X + gap.Dx()/2
		div := image.Rect(divX, gap.Min.Y+SpaceXS, divX+1, gap.Max.Y-SpaceXS)
		drawRect(dst, div, WithAlphaFromColor(TokenBorderMedium(), AlphaSubtle), true)
	}
}
