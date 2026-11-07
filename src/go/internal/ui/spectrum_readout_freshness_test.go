//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The sentence hold, the bar peak-hold and the axis anchor suit a moving signal, not a DISCONTINUITY: a mute,
// solo or channel change changes what is measured, so held answers describe a mix that no longer exists.

// Down on the next frame: nothing beats something false.
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

// The analyser window still holds the old audio for a frame or two, so latching the first frame would protect a
// wrong sentence for the full dwell.
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

// A held sentence, bar level or watermark from before the last discontinuity.
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

// A few seconds into playback: a sentence up, bars held, the axis on a loud transient.
func seedStaleSpectrumReadout(z *EQPanelZone) {
	z.spectrumBannerHold.update(spectrumBanner{Text: "kick and snare are fighting", Band: 2}, time.Now())
	z.spectrumPeaks.PeaksDB[2] = -18
	z.spectrumPeaks.MaxPeaksDB[2] = -12
	z.specMaxDB = -6
	z.eqMaxDB = -6
}

// The sentence naming the row, its lane and its held bar go at once.
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

// Solo mutes every other row, the largest discontinuity.
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

// The sentence and held bars refresh with the axis.
func TestChannelChangeRefreshesTheSpectrumReadout(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	seedStaleSpectrumReadout(z)

	z.SetActiveChannel(channel.Of(roster.Kick))

	if spectrumReadoutIsStale(z) {
		t.Error("switching channel left the previous readout in place")
	}
}

// The real keycap through Game.Update: calling toggleMute proves the notifier, not that a button reaches it.
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
