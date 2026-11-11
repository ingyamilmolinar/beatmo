package ui

import (
	"image"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Spectrum tab's headline sentence.
//
// The redesign's premise is that a basic user cannot read a spectrum, but can
// read a sentence. So the tab leads with ONE line of plain English and keeps
// the bars beneath it as the evidence for that line. The band the sentence
// refers to is emphasised in the plot, so the words and the picture always
// point at the same place.
//
// Only one line is ever shown — the highest-ranked finding. A list of problems
// is an analyser report, which is the thing this redesign is moving away from.

// spectrumBannerMinPanelH is the shortest panel that can afford to give a row
// to the sentence. Below this the plot keeps the whole rect: a squeezed
// 10-band plot reads as broken, whereas a missing sentence just reads as a
// plain spectrum.
const spectrumBannerMinPanelH = 72

// spectrumBannerSplit carves the sentence strip off the top of the panel and
// returns (banner, plot). On a panel too short to afford it the banner comes
// back empty and the plot receives the whole rect.
//
// Splitting here — rather than threading a banner height through
// spectrumPanelRows — keeps every existing spectrum renderer (bars, curve,
// pre-EQ overlay, cursor) untouched: they all receive the reduced rect and
// re-derive their own geometry from it exactly as before, so the layers cannot
// drift out of alignment.
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
	// Band is the ISO band the sentence refers to, or -1 when it refers to no
	// particular place (clipping, or the calm "balanced" line).
	Band int
	// Warn separates a problem from the reassurance line, which is drawn in a
	// quiet tone so a healthy mix does not look like it is being scolded.
	Warn bool
	// Clipping marks the one finding that is a real error rather than a
	// balance observation, and is toned accordingly.
	Clipping bool
}

// resolveSpectrumBanner picks the single line to show. Findings arrive already
// ranked, so this is "the first one, or the calm line when there are none".
func resolveSpectrumBanner(fs []finding) spectrumBanner {
	if len(fs) == 0 {
		return spectrumBanner{Text: i18n.T(i18n.KeySpectrumFindOK), Band: -1}
	}
	f := fs[0]
	return spectrumBanner{Text: findingSentence(f), Band: f.Band, Warn: true, Clipping: f.Kind == findingClipping}
}

// drawSpectrumBanner paints the sentence.
//
// A problem and the all-clear are separated by TONE, not by a glyph or a
// coloured badge: DESIGN.md reserves the three reds for specific roles (error
// is text-only) and forbids raw chrome glyphs. So clipping — a genuine error
// condition, and the one the Levels tab already paints in the error tone —
// gets colError; other findings get the primary text tone; the all-clear line
// recedes to the secondary tone so a healthy mix is not scolded.
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

// drawSpectrumBandEmphasis tints the full height of one ISO band's column so
// the reader's eye lands where the sentence says the problem is. Drawn beneath
// the bars, so it reads as a highlighted lane rather than covering the data.
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

// spectrumBannerDwell is how long a DIFFERENT finding must stay on top before
// it replaces the sentence on screen.
//
// Without it the banner is unreadable: findings are recomputed every frame from
// an analyzer window that moves with the beat, so on a real circuit the top
// finding alternates between "fighting" and "nothing above 4k" several times a
// second. Long enough to read a short sentence, short enough that a real change
// (the user mutes a row) still feels immediate.
const spectrumBannerDwell = 1200 * time.Millisecond

// spectrumBannerRearm is how long a line must hold after a DISCONTINUITY
// (mute / solo / channel change) before it is allowed on screen. The banner
// shows nothing until then.
//
// It is not the dwell: there is no stale line left to protect, so this only
// has to outlast the analyser's own settling. Measured on a real circuit, the
// single frame after a mute still reports the pre-mute window ("almost
// nothing above 4k Hz") and every frame after it reports the truth — so
// accepting the first frame would swap a stale sentence for a WRONG one and
// then protect it for the full dwell. Twelve frames at 60 fps is far more
// than the one or two needed, and short enough to read as immediate.
const spectrumBannerRearm = 200 * time.Millisecond

// bannerHold keeps the displayed sentence steady. It shows the first line
// immediately, then swaps only once a DIFFERENT line has been the top finding
// continuously for spectrumBannerDwell — a candidate that flaps never wins,
// because each change of candidate restarts its clock.
type bannerHold struct {
	shown     spectrumBanner
	hasShown  bool
	cand      spectrumBanner
	hasCand   bool
	candSince time.Time
	// rearming is set by reset: the banner stays blank until a line has been
	// the top finding for spectrumBannerRearm. Distinct from the cold start,
	// which shows its first line at once — at startup there is no previous
	// claim to retract and nothing has settled to wait for.
	rearming bool
}

// reset drops the held sentence so the next update shows its line at once.
//
// The dwell exists to stop findings flapping on a signal that is merely
// moving. Across a DISCONTINUITY — a mute, a solo, a channel change — there
// is nothing to flap against: what the tab is measuring has changed, and
// holding the previous sentence states something about a mix that no longer
// exists. The emphasised band rides on the same held value, so this clears
// the highlighted lane with it.
func (h *bannerHold) reset() {
	*h = bannerHold{rearming: true}
}

func (h *bannerHold) update(next spectrumBanner, now time.Time) spectrumBanner {
	if h.rearming {
		// Blank until one line has settled. Each different reading restarts
		// the clock, so a frame of transitional noise cannot win.
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
