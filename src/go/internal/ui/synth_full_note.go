package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// synth_full_note.go — the full-note "Your sound" card: adaptive display
// window + DAW-clip-style min/max column drawing over the real rendered hit
// (audio.RenderInstrumentPreview), which responds to EVERY stage param —
// envelope, LFO, burst, pitch-env — unlike the cycle-normalized "Up close"
// steady wave.

const (
	synthFullNoteWindowMinMs = 120
	synthFullNoteWindowMaxMs = 1000
)

// synthFullNoteWindowMs picks the display window for the full-note card from
// the instrument's effective envelope (≈1.2×(attack+decay+release)), clamped
// so a short kick fills the band and a pad isn't truncated. Non-modular
// recipes fall back to their decay knob, mirroring RenderInstrumentPreview's
// own amp fallback.
func synthFullNoteWindowMs(instID string) int {
	a, d, _, r, ok := synthADSRParamsFor(instID)
	if !ok {
		merged := conceptMergedParams(instID)
		d = merged["decay"]
		if d <= 0 {
			d = 0.3
		}
		a, r = 0.005, 0.08
	}
	ms := int(1.2 * (a + d + r) * 1000)
	if ms < synthFullNoteWindowMinMs {
		ms = synthFullNoteWindowMinMs
	}
	if ms > synthFullNoteWindowMaxMs {
		ms = synthFullNoteWindowMaxMs
	}
	return ms
}

// pcmMinMaxColumns bins pcm into cols columns and returns each column's
// min/max — the classic DAW-clip envelope drawing input.
func pcmMinMaxColumns(pcm []float64, cols int) (mins, maxs []float64) {
	if cols < 1 || len(pcm) == 0 {
		return nil, nil
	}
	mins = make([]float64, cols)
	maxs = make([]float64, cols)
	for c := 0; c < cols; c++ {
		lo := c * len(pcm) / cols
		hi := (c + 1) * len(pcm) / cols
		if hi <= lo {
			hi = lo + 1
		}
		if hi > len(pcm) {
			hi = len(pcm)
		}
		mn, mx := pcm[lo], pcm[lo]
		for _, v := range pcm[lo:hi] {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		mins[c], maxs[c] = mn, mx
	}
	return mins, maxs
}

// drawSynthFullNote paints the full-note "Your sound" card: the whole rendered
// hit as a DAW-clip min/max envelope, so envelope/LFO/burst/pitch knobs all
// visibly reshape it. Ghost (pre-drag render) drawn faint underneath.
func drawSynthFullNote(dst *ebiten.Image, rect image.Rectangle, mirror *synthMirror) {
	if rect.Dx() < 32 || rect.Dy() < 24 {
		return
	}
	drawRoundedRect(dst, rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), RadiusSM, false)

	pad := SpaceSM
	captionH := StyledTextHeight(RoleMicro)
	DrawTextStyled(dst, i18n.T(i18n.KeyCapYourSound), rect.Min.X+pad, rect.Min.Y+pad/2, RoleMicro, TokenTextSecondary())

	traceRect := image.Rect(rect.Min.X+pad, rect.Min.Y+pad+captionH, rect.Max.X-pad, rect.Max.Y-pad)
	if traceRect.Dx() < 8 || traceRect.Dy() < 6 {
		return
	}
	drawRect(dst, traceRect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)
	if mirror == nil {
		return
	}
	// No-copy read (read-only contract): copying the ~48k-float full note per
	// frame blew the Synth-tab frame byte budget.
	pcm, ghost := mirror.snapshotFullRef()
	midY := traceRect.Min.Y + traceRect.Dy()/2
	half := float64(traceRect.Dy()-2) / 2
	drawCols := func(buf []float64, gain float64, col, fill interface {
		RGBA() (r, g, b, a uint32)
	}) {
		mins, maxs := pcmMinMaxColumns(buf, traceRect.Dx())
		for c := range mins {
			yHi := midY - int(clampSignedUnit(maxs[c]*gain)*half)
			yLo := midY - int(clampSignedUnit(mins[c]*gain)*half)
			if yLo <= yHi {
				yLo = yHi + 1
			}
			x := traceRect.Min.X + c
			if fill != nil {
				drawRect(dst, image.Rect(x, yHi, x+1, yLo), fill, true)
			}
			if col != nil {
				drawRect(dst, image.Rect(x, yHi, x+1, yHi+1), col, true)
				drawRect(dst, image.Rect(x, yLo-1, x+1, yLo), col, true)
			}
		}
	}
	if len(ghost) > 1 {
		drawCols(ghost, mirrorTraceGain(ghost), WithAlpha(genColorBorder, AlphaSubtle), nil)
	}
	if len(pcm) > 1 {
		drawCols(pcm, mirrorTraceGain(pcm), colWaveTrace, colSynthOscFill)
	}
	drawRect(dst, image.Rect(traceRect.Min.X, midY, traceRect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

// drawSynthFullNote (method) routes the full-note "Your sound" card through
// the synth sprite cache (Task 2, synth_sprite_cache.go). instID is used ONLY
// to compute the cache key — the render itself still reads pcm/ghost straight
// off mirror, unchanged (drawSynthFullNote, the free function above, is the
// verbatim render callback).
//
// Key includes whether a ghost trace is currently present: the full-note
// render is fed by updateSynthFullNoteLive, which is a debounced OFF-THREAD
// (stale-while-revalidate) render — the params hash can change on the same
// frame the async job is only just submitted, before its result lands. The
// ghost flips synchronously at request time, so folding it into the key
// forces a re-render at that same moment rather than blitting a texture that
// still shows the pre-edit pcm. The remaining staleness window (between
// request and the async render actually landing) is closed by
// synthTabUpdate's Invalidate() call when synthMirror.consumeReady() fires —
// see synth_panel_zone.go.
func (p *synthPanel) drawSynthFullNote(dst *ebiten.Image, rect image.Rectangle, instID string, mirror *synthMirror) {
	hasGhost := false
	if mirror != nil {
		_, ghost := mirror.snapshotFullRef()
		hasGhost = len(ghost) > 1
	}
	key := struct {
		hash  string
		ghost bool
	}{p.synthParamsHash(instID), hasGhost}
	p.sprites.fullNote.Draw(dst, rect, key, func(img *ebiten.Image, r image.Rectangle) {
		drawSynthFullNote(img, r, mirror)
	})
}

// clampSignedUnit clamps v to [-1, 1].
func clampSignedUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
