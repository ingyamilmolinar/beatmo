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

func TestEQPeek_VisibleWhenCollapsed(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("expected mobileEQCollapsed=true on default mobile layout")
	}
	if g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect empty when EQ is collapsed; expected 24px peek strip")
	}
	if g.drum.eqPeekRect.Dy() != 24 {
		t.Fatalf("eqPeekRect height=%d; expected 24", g.drum.eqPeekRect.Dy())
	}
	if g.drum.eqPeekRect.Max.Y != g.drum.bottomActionBarRect.Min.Y {
		t.Fatalf("eqPeekRect should sit directly above bottom action bar: peek.Max.Y=%d barTop=%d",
			g.drum.eqPeekRect.Max.Y, g.drum.bottomActionBarRect.Min.Y)
	}
}

func TestEQPeek_HiddenWhenExpanded(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.drum.mobileEQCollapsed = false // force-expand for test
	g.drum.mobileEQMode = true
	g.Layout(360, 700)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect should be empty when EQ panel is expanded")
	}
}

func TestEQPeek_HiddenOnDesktop(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect should be empty on desktop")
	}
}
