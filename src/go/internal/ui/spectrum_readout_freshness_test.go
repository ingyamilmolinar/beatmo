//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The Spectrum tab smooths three things so the readout is stable while a beat
// plays: the headline sentence holds for spectrumBannerDwell, each bar falls
// back to a peak-hold between transients, and the dB axis anchors to the
// loudest peak seen. All three are right for a signal that is merely moving.
//
// None of them is right across a DISCONTINUITY. Muting a row, soloing one, or
// switching channel does not change the signal — it changes WHAT IS BEING
// MEASURED. Holding the previous answer then means the tab states something
// about a mix that no longer exists, for up to 1.2 s in the sentence and the
// emphasised band, and for the life of the session in the watermark.

// TestBannerHoldResetRetractsTheStaleLineAtOnce — a reset means the claim on
// screen is about a mix that no longer exists. It must come DOWN on the next
// frame; nothing is a better answer than something false.
func TestBannerHoldResetRetractsTheStaleLineAtOnce(t *testing.T) {
	var h bannerHold
	now := time.Now()
	first := spectrumBanner{Text: "kick and snare are fighting", Band: 2}
	if got := h.update(first, now); got.Text != first.Text {
		t.Fatalf("first line = %q, want %q", got.Text, first.Text)
	}

	next := spectrumBanner{Text: "nothing fighting", Band: -1}

	// Without a reset the dwell correctly holds the old line.
	if got := h.update(next, now.Add(100*time.Millisecond)); got.Text != first.Text {
		t.Fatalf("during dwell got %q, want the held %q", got.Text, first.Text)
	}

	h.reset()
	got := h.update(next, now.Add(110*time.Millisecond))
	if got.Text == first.Text {
		t.Errorf("the stale line %q survived the reset", first.Text)
	}
	if got.Band != -1 {
		t.Errorf("the emphasised band %d survived the reset", got.Band)
	}
}

// TestBannerDoesNotLatchTheFirstFrameAfterADiscontinuity — the analyser needs
// a frame or two to reflect a mute (its window still holds the old audio).
// Measured on a real circuit: the frame right after the mute reported
// "almost nothing above 4k Hz" and every frame after it reported "nothing
// fighting". Latching that first frame would replace a stale sentence with a
// wrong one and then protect IT for the full dwell — worse than the bug.
func TestBannerDoesNotLatchTheFirstFrameAfterADiscontinuity(t *testing.T) {
	var h bannerHold
	now := time.Now()
	h.update(spectrumBanner{Text: "kick and snare are fighting", Band: 2}, now)
	h.reset()

	spurious := spectrumBanner{Text: "almost nothing above 4k Hz", Band: 7}
	settled := spectrumBanner{Text: "nothing fighting", Band: -1}

	// One transitional frame, then the real answer from then on.
	now = now.Add(16 * time.Millisecond)
	if got := h.update(spurious, now); got.Text == spurious.Text {
		t.Errorf("latched the transitional reading %q one frame after the discontinuity", spurious.Text)
	}
	for i := 0; i < 60; i++ {
		now = now.Add(16 * time.Millisecond)
		if got := h.update(settled, now); got.Text == spurious.Text {
			t.Fatalf("frame %d: showing the transitional reading %q", i, spurious.Text)
		}
	}
	// ...and the settled answer must actually arrive, well inside the dwell.
	if got := h.update(settled, now); got.Text != settled.Text {
		t.Errorf("settled line never appeared: showing %q, want %q", got.Text, settled.Text)
	}
}

// spectrumReadoutIsStale reports whether the zone is still carrying a reading
// taken before the last discontinuity: a held sentence, a held bar level, or
// a watermark from the previous measurement.
func spectrumReadoutIsStale(z *EQPanelZone) bool {
	if z.spectrumBannerHold.hasShown {
		return true
	}
	for i := range z.spectrumPeaks.PeaksDB {
		if z.spectrumPeaks.PeaksDB[i] != spectrumNoPeakDB || z.spectrumPeaks.MaxPeaksDB[i] != spectrumNoPeakDB {
			return true
		}
	}
	return z.specMaxDB != spectrumMinDB
}

// seedStaleSpectrumReadout puts the zone in the state it would be in after a
// few seconds of playback: a sentence on screen, bars held up by the
// peak-hold, and an axis anchored to a loud transient.
func seedStaleSpectrumReadout(z *EQPanelZone) {
	z.spectrumBannerHold.update(spectrumBanner{Text: "kick and snare are fighting", Band: 2}, time.Now())
	z.spectrumPeaks.PeaksDB[2] = -18
	z.spectrumPeaks.MaxPeaksDB[2] = -12
	z.specMaxDB = -6
	z.eqMaxDB = -6
}

// TestMutingARowRefreshesTheSpectrumReadout — the row is gone from the mix
// the instant it is muted. The sentence naming it, the lane emphasising its
// band and its held bar must all go with it.
func TestMutingARowRefreshesTheSpectrumReadout(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone
	if z == nil {
		t.Fatal("no EQ panel zone")
	}
	seedStaleSpectrumReadout(z)

	g.drum.toggleMute(0)

	if spectrumReadoutIsStale(z) {
		t.Error("muting a row left the previous sentence / held bars / axis anchor in place")
	}
}

// TestSoloingARowRefreshesTheSpectrumReadout — solo mutes every other row, so
// it is the largest discontinuity of the three.
func TestSoloingARowRefreshesTheSpectrumReadout(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	z := g.drum.eqPanelZone
	seedStaleSpectrumReadout(z)

	g.drum.toggleSolo(0)

	if spectrumReadoutIsStale(z) {
		t.Error("soloing a row left the previous readout in place")
	}
}

// TestChannelChangeRefreshesTheSpectrumReadout — the axis anchor already
// re-baselined here; the sentence and the held bars did not, so the tab
// described the previous channel while showing the new one.
func TestChannelChangeRefreshesTheSpectrumReadout(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	seedStaleSpectrumReadout(z)

	z.SetActiveChannel(channel.Of(roster.Kick))

	if spectrumReadoutIsStale(z) {
		t.Error("switching channel left the previous readout in place")
	}
}

// TestRealMuteButtonPressRefreshesTheSpectrumReadout drives the actual mute
// keycap through Game.Update. The tests above call toggleMute directly, which
// proves the notifier works but not that any button reaches it — a wiring
// this shallow is exactly the kind that passes unit tests while the feature
// is dead on screen.
func TestRealMuteButtonPressRefreshesTheSpectrumReadout(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	_ = g.Update()

	z := g.drum.eqPanelZone
	btns := g.drum.rowRackZone.RowMuteBtns()
	if len(btns) == 0 || btns[0] == nil {
		t.Fatal("no row mute button")
	}
	r := btns[0].Rect()
	if r.Empty() {
		t.Fatal("mute button has no rect after layout")
	}

	seedStaleSpectrumReadout(z)
	wasMuted := g.drum.Rows[0].Muted

	clickAt(g, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	if g.drum.Rows[0].Muted == wasMuted {
		t.Fatal("the click did not reach the mute button; the test proves nothing")
	}
	if spectrumReadoutIsStale(z) {
		t.Error("a real mute press left the previous Spectrum readout in place")
	}
}
