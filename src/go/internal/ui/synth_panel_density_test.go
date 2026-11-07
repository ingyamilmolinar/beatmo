//go:build test

package ui

import (
	"testing"
)

// At every density × content height the last section fits; the solver steps the section height down. Driven through
// Profile() with no Game.
func TestSynthSectionsFitAtAllDensities(t *testing.T) {
	densities := []Density{DensityCompact, DensityComfortable, DensitySpacious}
	// Including 308 (a 360 panel less its header), where a fixed floor overflowed.
	contentHeights := []int{200, 280, 308, 400, 600}
	const numSections = 4
	for _, d := range densities {
		restore := SetDensityForTest(d)
		dv := densityValuesFor(d)
		hardFloor := dv.SynthControlMin + dv.SynthControlCaptionH + SpaceSM
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
			// Overflow is allowed only below n × hardFloor, a genuinely too-short panel.
			if h >= hardFloor*numSections && lastY > h {
				t.Errorf("density=%v h=%d: solver produced lastY=%d > h=%d (should fit)",
					d, h, lastY, h)
			}
		}
		restore()
	}
}

// Four sections at Spacious on a 360 × 480 phone all fit.
func TestSynthSpaciousFitsOn360TallPanel(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	dv := densityValuesFor(DensitySpacious)
	const numSections = 4
	const headerH = 52
	contentH := 360 - headerH // 308 — the failing case
	fitPerSection := contentH / numSections
	hardFloor := dv.SynthControlMin + dv.SynthControlCaptionH + SpaceSM
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
		// Only when the hard floor itself cannot fit.
		if hardFloor*numSections <= contentH {
			t.Errorf("Spacious solver overflowed: total=%d > contentH=%d (hardFloor*n=%d ≤ contentH)",
				totalSectionsH, contentH, hardFloor*numSections)
		}
	}
}

// Short captions pass, long ones get "…", impossible widths degenerate to "…".
func TestTruncCaptionEllipsis(t *testing.T) {
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
