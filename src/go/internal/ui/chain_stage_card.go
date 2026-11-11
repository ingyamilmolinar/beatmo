package ui

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// chain_stage_card.go — per-stage card decorations for the Chain
// panel's left column. Extracted from chain_panel_zone.go (Phase 3 of
// the audio-panel redesign) so the renderer that adds card chrome
// (mini-waveform when the stage is a tap, insert-FX badge count on
// the FX stage) lives next to its scope-specific dependencies.
//
// The functions are pure renderers — they read inputs (samples,
// effect-chain count) and draw into the supplied destination image.
// No state, no mutation. Callers (drawHeader in chain_panel_zone.go)
// are responsible for sequencing.

// drawChainStageMiniWaveform paints a small downsampled waveform
// trace inside `rect` (the bottom strip of a stage button). When
// `samples` is empty the function is a no-op so silent stages stay
// clean. The trace uses TokenVizScopeTrace* tokens at low alpha so
// it doesn't compete with the stage label.
func drawChainStageMiniWaveform(dst *ebiten.Image, rect image.Rectangle, samples []float64, isA bool) {
	if rect.Dx() < 6 || rect.Dy() < 3 || len(samples) == 0 {
		return
	}
	col := genColorVizScopeTraceA
	if !isA {
		col = genColorVizScopeTraceB
	}
	traceCol := WithAlpha(col, AlphaStrong)

	// Decimate samples to one per pixel column. For each column take the
	// max absolute amplitude of all samples falling in it so transient
	// peaks remain visible in the thumbnail.
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
		// Connect previous y to this y with a tall 1px slice so the
		// decimated trace reads continuous, not as scattered points.
		y0, y1 := prevY, y
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		drawRect(dst, image.Rect(x, y0, x+1, y1+1), traceCol, true)
		prevY = y
	}
}

// drawChainFXBadgeCount paints a small "+N" badge in the top-left
// corner of the InsertFX stage card showing how many insert effects
// are currently active on the supplied row instrument. Returns
// silently when count is 0 so silent FX cards stay clean.
//
// The full FX list is surfaced via the hover-dwell tooltip
// (chain_panel_zone.go drives that path), so this badge is the at-
// a-glance count + the tooltip is the audit detail.
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

// chainInsertFXCountForActiveRow returns the count of enabled insert
// effects for the instrument tied to the currently-active tap (A or
// B). When neither tap is on an instrument-bearing stage this returns
// 0. Pure function — no mutation, no I/O beyond audio.GetInsertEffects.
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

// drawChainStageCardDecorations paints the mini-waveform (when this
// stage is a tap) and the insert-FX badge count (when this stage is
// the FX card). Called from chain_panel_zone.go's drawHeader for each
// stage button. Pure renderer — uses the zone only to read tap state
// and the scope.State via callback.
func (z *ChainPanelZone) drawChainStageCardDecorations(dst *ebiten.Image, btn *Button, stageIdx int, isATap, isBTap bool) {
	if btn == nil || btn.Rect().Empty() {
		return
	}
	stages := scope.AllStages()

	var state *scope.State
	if z.callbacks.ScopeState != nil {
		state = z.callbacks.ScopeState()
	}

	// Mini-waveform: only when this stage carries one of the two taps
	// and its trace is currently visible (per-trace swatch toggle).
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
		// Density-aware strip height: the taller Spacious (mobile) rows give
		// the mini-waveform room to read instead of the old fixed 6px.
		waveStripH := Profile().DensityValues().ChainMiniWaveH
		waveStripR := image.Rect(r.Min.X+2, r.Max.Y-waveStripH-1, r.Max.X-2, r.Max.Y-1)
		drawChainStageMiniWaveform(dst, waveStripR, sampSlice, traceIsA)
	}

	// FX badge count: only on the InsertFX stage.
	if stageIdx < len(stages) && stages[stageIdx] == scope.StageInsertFX {
		drawChainFXBadgeCount(dst, btn.Rect(), chainInsertFXCountForActiveRow(state))
	}
}
