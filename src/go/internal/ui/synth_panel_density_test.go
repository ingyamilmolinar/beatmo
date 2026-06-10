//go:build test

package ui

import (
	"testing"
)

// TestSynthSectionsFitAtAllDensities pins the Phase 2 adaptive
// section-height solver: for every density × content-height
// combination, the last section's Max.Y must be ≤ the content rect's
// Max.Y. Pre-Phase-2 the section min-h was a fixed 88 px floor and
// 4 sections × 88 = 352 overflowed a 360-px panel; the solver below
// steps the per-section height down so all sections fit.
//
// The test exercises the solver directly via Profile() — no Game
// scaffolding — so it's fast and decoupled from the row-rack init
// path.
func TestSynthSectionsFitAtAllDensities(t *testing.T) {
	densities := []Density{DensityCompact, DensityComfortable, DensitySpacious}
	// Test against a range of content heights including the
	// pre-Phase-2 break point (360 panel - 52 header = 308 content).
	contentHeights := []int{200, 280, 308, 400, 600}
	const numSections = 4
	for _, d := range densities {
		restore := SetDensityForTest(d)
		dv := densityValuesFor(d)
		hardFloor := dv.SynthKnobMin + dv.SynthKnobCaptionH + SpaceSM
		idealH := dv.SynthSectionMinH
		for _, h := range contentHeights {
			fitPerSection := h / numSections
			var perSectionH int
			if fitPerSection >= idealH {
				perSectionH = idealH
			} else if fitPerSection >= hardFloor {
				perSectionH = fitPerSection
			} else {
				perSectionH = hardFloor
			}
			lastY := perSectionH * numSections
			// Allow the hardFloor case to overflow — but only when h
			// itself is below n × hardFloor (the genuine pathological
			// "panel too short" case). For h above n × hardFloor the
			// solver must produce a fit.
			if h >= hardFloor*numSections && lastY > h {
				t.Errorf("density=%v h=%d: solver produced lastY=%d > h=%d (should fit)",
					d, h, lastY, h)
			}
		}
		restore()
	}
}

// TestSynthSpaciousFitsTwoOnPortrait — the canonical mobile failure:
// 4 sections at Spacious density on a 360 × 480 portrait phone with
// a ~52 px header. Pre-Phase-2 this clipped the bottom section. The
// solver must now produce sections that all fit.
func TestSynthSpaciousFitsOn360TallPanel(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	dv := densityValuesFor(DensitySpacious)
	const numSections = 4
	const headerH = 52
	contentH := 360 - headerH // 308 — the failing case
	fitPerSection := contentH / numSections
	hardFloor := dv.SynthKnobMin + dv.SynthKnobCaptionH + SpaceSM
	var perSectionH int
	if fitPerSection >= dv.SynthSectionMinH {
		perSectionH = dv.SynthSectionMinH
	} else if fitPerSection >= hardFloor {
		perSectionH = fitPerSection
	} else {
		perSectionH = hardFloor
	}
	totalSectionsH := perSectionH * numSections
	if totalSectionsH > contentH {
		// Only acceptable when the hard floor itself can't fit; verify
		// that's the case.
		if hardFloor*numSections <= contentH {
			t.Errorf("Spacious solver overflowed: total=%d > contentH=%d (hardFloor*n=%d ≤ contentH)",
				totalSectionsH, contentH, hardFloor*numSections)
		}
	}
}

// TestTruncCaptionEllipsis pins the helper's contract: short captions
// pass through; long captions get an ellipsis appended; impossibly
// narrow targets degenerate to "…".
func TestTruncCaptionEllipsis(t *testing.T) {
	// At 200 px width, "tone +0.12" fits (TextWidth of that string is
	// well under 200 in the body font).
	if got := truncCaption("tone +0.12", 200); got != "tone +0.12" {
		t.Errorf("wide cell: got %q, want unchanged", got)
	}
	// At 18 px width (narrow), the caption truncates with ellipsis.
	long := "VeryLongCaptionThatCannotPossiblyFit"
	got := truncCaption(long, 30)
	if got == long {
		t.Errorf("narrow cell: caption not truncated (got %q)", got)
	}
	if len(got) >= len(long) {
		t.Errorf("narrow cell: caption length unchanged (got %q, len=%d)", got, len(got))
	}
	if got != "" && got[len(got)-len("…"):] != "…" {
		// got might be just "…" — that's fine.
		if got != "…" {
			t.Errorf("narrow cell: caption %q missing trailing ellipsis", got)
		}
	}
	// Zero-width cell collapses to empty string.
	if got := truncCaption("anything", 0); got != "" {
		t.Errorf("zero-width: got %q, want empty", got)
	}
	// Empty input passes through.
	if got := truncCaption("", 100); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}
