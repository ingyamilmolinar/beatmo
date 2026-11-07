package ui

import (
	"image"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/hajimehoshi/ebiten/v2"
)

// A basic user cannot read a spectrum but can read a sentence, so the tab leads with ONE line and the bars
// are its evidence; the band it names is emphasised. One line only: a list is the analyser report this replaces.

// Below this the plot keeps the whole rect: a squeezed plot reads as broken, a missing sentence does not.
const spectrumBannerMinPanelH = 72

// Every spectrum layer re-derives its geometry from the reduced plot rect, so they cannot drift apart.
func spectrumBannerSplit(rect image.Rectangle) (banner, plot image.Rectangle) {
	if rect.Dy() < spectrumBannerMinPanelH {
		return image.Rectangle{}, rect
	}
	h := StyledTextHeight(RoleCaption) + SpaceXS*2
	banner = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+h)
	plot = image.Rect(rect.Min.X, banner.Max.Y, rect.Max.X, rect.Max.Y)
	return banner, plot
}

// spectrumBanner is the resolved sentence for the current frame.
type spectrumBanner struct {
	Text string
	// -1 when the line names no band (clipping, or the all-clear).
	Band int
	// The all-clear is drawn quietly so a healthy mix is not scolded.
	Warn bool
	// The one finding that is a real error, toned as one.
	Clipping bool
}

// Findings arrive ranked, so this is the first one or the all-clear.
func resolveSpectrumBanner(fs []finding) spectrumBanner {
	if len(fs) == 0 {
		return spectrumBanner{Text: i18n.T(i18n.KeySpectrumFindOK), Band: -1}
	}
	f := fs[0]
	return spectrumBanner{Text: findingSentence(f), Band: f.Band, Warn: true, Clipping: f.Kind == findingClipping}
}

// Separated by TONE, never a glyph or a red badge (error red is text-only): clipping gets colError, a
// finding primary text, the all-clear the secondary tone.
func drawSpectrumBanner(dst *ebiten.Image, rect image.Rectangle, b spectrumBanner) {
	if rect.Empty() || b.Text == "" {
		return
	}
	col := colTextSecondary
	switch {
	case b.Clipping:
		col = colError
	case b.Warn:
		col = colTextPrimary
	}
	y := rect.Min.Y + (rect.Dy()-StyledTextHeight(RoleCaption))/2
	DrawTextStyled(dst, b.Text, rect.Min.X+SpaceXS, y, RoleCaption, col)
}

// Beneath the bars, so it reads as a highlighted lane, not a cover.
func drawSpectrumBandEmphasis(dst *ebiten.Image, barRect image.Rectangle, band int) {
	if band < 0 || band >= len(isoBands) || barRect.Dx() <= 0 || barRect.Dy() <= 0 {
		return
	}
	numBands := len(isoBands)
	barSpace := barRect.Dx() - (numBands - 1)
	if barSpace < numBands {
		return
	}
	barWidth := barSpace / numBands
	x0 := barRect.Min.X + band*(barWidth+1)
	x1 := x0 + barWidth
	if x1 > barRect.Max.X {
		x1 = barRect.Max.X
	}
	if x0 >= barRect.Max.X || x1 <= x0 {
		return
	}
	drawRect(dst, image.Rect(x0, barRect.Min.Y, x1, barRect.Max.Y), colSpectrumBandEmphasis, true)
}

// Findings move with the beat and the top one alternates several times a second; long enough to read,
// short enough that a real change (a mute) still feels immediate.
const spectrumBannerDwell = 1200 * time.Millisecond

// Not the dwell: it only has to outlast the analyser settling. The frame after a mute still reports the
// old mix, so accepting it would show a WRONG line and protect it for a full dwell.
const spectrumBannerRearm = 200 * time.Millisecond

// A flapping candidate never wins: each change of candidate restarts its clock.
type bannerHold struct {
	shown     spectrumBanner
	hasShown  bool
	cand      spectrumBanner
	hasCand   bool
	candSince time.Time
	// Unlike the cold start, which shows its first line at once: then there is no claim to retract.
	rearming bool
}

// For a discontinuity (mute, solo, channel change): the held sentence describes a mix that no longer exists.
// It also clears the emphasised lane, which rides on the held value.
func (h *bannerHold) reset() {
	*h = bannerHold{rearming: true}
}

func (h *bannerHold) update(next spectrumBanner, now time.Time) spectrumBanner {
	if h.rearming {
		if !h.hasCand || next.Text != h.cand.Text {
			h.cand, h.hasCand, h.candSince = next, true, now
			return spectrumBanner{Band: -1}
		}
		if now.Sub(h.candSince) < spectrumBannerRearm {
			return spectrumBanner{Band: -1}
		}
		h.shown, h.hasShown = next, true
		h.hasCand, h.rearming = false, false
		return h.shown
	}
	if !h.hasShown {
		h.shown, h.hasShown = next, true
		h.hasCand = false
		return h.shown
	}
	// Same sentence as on screen: nothing pending any more.
	if next.Text == h.shown.Text {
		h.shown = next // keep Band/tone fresh
		h.hasCand = false
		return h.shown
	}
	if !h.hasCand || next.Text != h.cand.Text {
		h.cand, h.hasCand, h.candSince = next, true, now
		return h.shown
	}
	if now.Sub(h.candSince) >= spectrumBannerDwell {
		h.shown = next
		h.hasCand = false
	}
	return h.shown
}
