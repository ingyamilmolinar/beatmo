package ui

import "github.com/hajimehoshi/ebiten/v2"

// EQPeekLayer paints the mobile EQ peek sparkline strip just above the
// bottom action bar. Repaints the strip's surface first so any earlier
// widget bg painted into this region is replaced — the peek strip is the
// canonical owner of these pixels.
//
// Z = ZEQPeek (60) — above background, below transport.
type EQPeekLayer struct {
	dv *DrumView
}

func newEQPeekLayer(dv *DrumView) *EQPeekLayer { return &EQPeekLayer{dv: dv} }

func (l *EQPeekLayer) ID() string    { return "eq-peek" }
func (l *EQPeekLayer) ZIndex() int   { return ZEQPeek }
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
