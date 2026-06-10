//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// newEQPanelZoneForTest constructs an EQPanelZone with empty callbacks for
// unit tests. The returned zone has flat (0 dB) band gains.
func newEQPanelZoneForTest(t *testing.T) *EQPanelZone {
	t.Helper()
	return NewEQPanelZone(EQCallbacks{})
}

func TestEQPanel_SampleCurveReturnsRequestedLength(t *testing.T) {
	z := newEQPanelZoneForTest(t)
	samples := z.SampleCurve(32)
	if len(samples) != 32 {
		t.Fatalf("SampleCurve(32) returned %d samples", len(samples))
	}
}

func TestEQPanel_SampleCurveTracksGains(t *testing.T) {
	z := newEQPanelZoneForTest(t)

	// Flat: all samples within 1 dB of zero.
	flat := z.SampleCurve(32)
	for i, s := range flat {
		if math.Abs(s) > 1.0 {
			t.Fatalf("flat sample[%d]=%.2f exceeds 1 dB tolerance", i, s)
		}
	}

	// Boost band 0 by 12 dB; expect at least one sample > 6 dB.
	z.SetBandGainDB(0, 12.0)
	boosted := z.SampleCurve(32)
	maxSample := math.Inf(-1)
	for _, s := range boosted {
		if s > maxSample {
			maxSample = s
		}
	}
	if maxSample < 6.0 {
		t.Fatalf("expected boosted curve max >6 dB, got %.2f", maxSample)
	}
}

// TestEQPeek_AlwaysEmptyOnMobile asserts the post-2026-05-10 unified-layout
// invariant: the EQ peek strip is NEVER allocated. The historical 24-px
// sparkline-above-the-bar surface rendered as a black band on default boot
// and the bottom segmented control's "EQ" tab already provides the same
// expand-EQ affordance, so the peek was retired. Mobile-collapsed,
// mobile-expanded, and desktop layouts must all leave eqPeekRect empty.
func TestEQPeek_AlwaysEmptyOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect must be empty on default mobile boot (peek strip retired); got %v",
			g.drum.eqPeekRect)
	}

	// Expand the EQ panel — peek must still be empty.
	g.drum.mobileEQCollapsed = false
	g.drum.SetMobileEQMode(true)
	g.Layout(360, 700)
	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect must remain empty when EQ panel is expanded; got %v",
			g.drum.eqPeekRect)
	}
}

func TestEQPeek_AlwaysEmptyOnDesktop(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect must be empty on desktop; got %v", g.drum.eqPeekRect)
	}
}
