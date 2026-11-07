package ui

import "github.com/hajimehoshi/ebiten/v2"

// It repaints its surface first: the peek strip owns these pixels.
type EQPeekLayer struct {
	dv *DrumView
}

func newEQPeekLayer(dv *DrumView) *EQPeekLayer { return &EQPeekLayer{dv: dv} }

func (l *EQPeekLayer) ID() string  { return "eq-peek" }
func (l *EQPeekLayer) ZIndex() int { return ZEQPeek }
func (l *EQPeekLayer) Visible() bool {
	return !l.dv.eqPeekRect.Empty() && l.dv.eqPanelZone != nil
}

func (l *EQPeekLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	drawRect(dst, dv.eqPeekRect, colBGBottom, true)
	// 1 sample per ~4 px width — balances detail vs CPU.
	n := dv.eqPeekRect.Dx() / 4
	if n < 8 {
		n = 8
	}
	samples := dv.eqPanelZone.SampleCurve(n)
	if len(samples) >= 2 {
		drawSparklineInRect(dst, dv.eqPeekRect, samples,
			WithAlpha(genColorPrimary, genAlphaSubtle))
	}
}
