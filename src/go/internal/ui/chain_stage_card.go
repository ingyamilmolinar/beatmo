package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Pure renderers: no state, no mutation.

// A no-op for empty samples; low alpha so it never competes with the label.
func drawChainStageMiniWaveform(dst *ebiten.Image, rect image.Rectangle, samples []float64, isA bool) {
	if rect.Dx() < 6 || rect.Dy() < 3 || len(samples) == 0 {
		return
	}
	col := genColorVizScopeTraceA
	if !isA {
		col = genColorVizScopeTraceB
	}
	traceCol := WithAlpha(col, AlphaStrong)

	// Max absolute amplitude per column, so transients stay visible.
	w := rect.Dx()
	step := float64(len(samples)) / float64(w)
	midY := rect.Min.Y + rect.Dy()/2
	half := float64(rect.Dy()) / 2.0
	prevY := midY
	for c := 0; c < w; c++ {
		lo := int(float64(c) * step)
		hi := int(float64(c+1) * step)
		if hi > len(samples) {
			hi = len(samples)
		}
		if lo >= hi {
			continue
		}
		// Take signed peak (largest abs value) to preserve polarity.
		var peakV float64
		for i := lo; i < hi; i++ {
			v := samples[i]
			if v > peakV || -v > peakV {
				if v < 0 {
					peakV = -v
					peakV = -peakV
				} else {
					peakV = v
				}
			}
		}
		// Clamp to [-1, 1].
		if peakV > 1 {
			peakV = 1
		} else if peakV < -1 {
			peakV = -1
		}
		y := midY - int(peakV*half)
		x := rect.Min.X + c
		// A tall slice between samples so the trace reads continuous.
		y0, y1 := prevY, y
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		drawRect(dst, image.Rect(x, y0, x+1, y1+1), traceCol, true)
		prevY = y
	}
}

// Nothing at 0; the hover tooltip lists the effects.
func drawChainFXBadgeCount(dst *ebiten.Image, btnRect image.Rectangle, count int) {
	if btnRect.Empty() || count <= 0 {
		return
	}
	label := fmt.Sprintf("+%d", count)
	tw := StyledTextWidth(label, RoleCaption)
	th := StyledTextHeight(RoleCaption)
	pad := 2
	badgeW := tw + 2*pad
	badgeH := th + pad
	bx := btnRect.Min.X + 1
	by := btnRect.Min.Y + 1
	bgR := image.Rect(bx, by, bx+badgeW, by+badgeH)
	drawRect(dst, bgR, WithAlpha(colVizMids, AlphaStrong), true)
	DrawTextStyled(dst, label, bx+pad, by+pad/2, RoleCaption, colTextPrimary)
}

// 0 when neither tap is on an instrument stage.
func chainInsertFXCountForActiveRow(state *scope.State) int {
	if state == nil {
		return 0
	}
	var ch channel.ID
	if state.TapA.Active && !state.TapA.InstID.IsZero() {
		ch = state.TapA.InstID
	} else if state.TapB.Active && !state.TapB.InstID.IsZero() {
		ch = state.TapB.InstID
	}
	inst, ok := ch.Instrument()
	if !ok {
		return 0
	}
	slots := audio.GetInsertEffects(inst)
	n := 0
	for _, s := range slots {
		if s.Enabled {
			n++
		}
	}
	return n
}

func (z *ChainPanelZone) drawChainStageCardDecorations(dst *ebiten.Image, btn *Button, stageIdx int, isATap, isBTap bool) {
	if btn == nil || btn.Rect().Empty() {
		return
	}
	stages := scope.AllStages()

	var state *scope.State
	if z.callbacks.ScopeState != nil {
		state = z.callbacks.ScopeState()
	}

	// Only on a tap whose trace is visible.
	var sampSlice []float64
	traceIsA := true
	if isATap && z.showTapA && state != nil && state.TapA.Active && len(state.TapA.Samples) > 0 {
		sampSlice = state.TapA.Samples
		traceIsA = true
	} else if isBTap && z.showTapB && state != nil && state.TapB.Active && len(state.TapB.Samples) > 0 {
		sampSlice = state.TapB.Samples
		traceIsA = false
	}
	if len(sampSlice) > 0 {
		r := btn.Rect()
		waveStripH := Profile().DensityValues().ChainMiniWaveH
		waveStripR := image.Rect(r.Min.X+2, r.Max.Y-waveStripH-1, r.Max.X-2, r.Max.Y-1)
		drawChainStageMiniWaveform(dst, waveStripR, sampSlice, traceIsA)
	}

	// FX badge count: only on the InsertFX stage.
	if stageIdx < len(stages) && stages[stageIdx] == scope.StageInsertFX {
		drawChainFXBadgeCount(dst, btn.Rect(), chainInsertFXCountForActiveRow(state))
	}
}
