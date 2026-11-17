//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A chip too narrow for its full stage label falls back to the stage's
// DESIGNED short label ("ENV", "F.ENV") — never a mid-word ellipsis
// ("ENVELOP…", "FILTER E…" in the mobile_bottom_nav_synth screenshot,
// 2026-07-04 critique A7). Ellipsis remains the last resort for widths
// even the short label can't fit.
func TestChipLabelFallsBackToShortNotMidWordEllipsis(t *testing.T) {
	full := sectionLabelLocalized(synthSectionEnvelope)
	avail := TextWidth(full) - 1 // just too narrow for the full label

	got := sectionLabelChipFitted(synthSectionEnvelope, avail)
	if want := i18n.T(i18n.KeySynthStageEnvelopeShort); got != want {
		t.Fatalf("cramped ENVELOPE chip = %q, want short label %q", got, want)
	}

	got = sectionLabelChipFitted(synthSectionFilterEnv, TextWidth(sectionLabelLocalized(synthSectionFilterEnv))-1)
	if want := i18n.T(i18n.KeySynthStageFilterEnvShort); got != want {
		t.Fatalf("cramped FILTER ENV chip = %q, want short label %q", got, want)
	}
}

// A chip wide enough keeps the full label untouched.
func TestChipLabelFullWhenItFits(t *testing.T) {
	full := sectionLabelLocalized(synthSectionEnvelope)
	if got := sectionLabelChipFitted(synthSectionEnvelope, TextWidth(full)); got != full {
		t.Fatalf("roomy chip = %q, want full label %q", got, full)
	}
}
