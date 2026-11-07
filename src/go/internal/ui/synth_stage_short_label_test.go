//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Too narrow, a stage chip shows its designed short label (ENS/FRM/RES), never a mid-word ellipsis, in every locale.
func TestStageChipShortLabelNoEllipsis(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })

	longStages := []synthSectionID{
		synthSectionEnsemble,
		synthSectionFormant,
		synthSectionResonator,
	}
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		for _, id := range longStages {
			full := sectionLabelLocalized(id)
			shortKey := sectionShortLabelKey(id)
			if shortKey == "" {
				t.Fatalf("locale %v: stage %v has no short-label key", loc, id)
			}
			short := i18n.T(shortKey)

			// Pick a width that the full label overflows but the short label fits.
			availPx := StyledTextWidth(full, RoleCaption) - 1
			if StyledTextWidth(short, RoleCaption) > availPx {
				t.Fatalf("locale %v: short label %q wider than the overflow width %d", loc, short, availPx)
			}

			got := sectionLabelChipFitted(id, availPx)
			if got != short {
				t.Errorf("locale %v stage %v: got %q, want short label %q", loc, id, got, short)
			}
			if strings.Contains(got, "…") || strings.Contains(got, "...") {
				t.Errorf("locale %v stage %v: chip label %q contains an ellipsis", loc, id, got)
			}
		}
	}
}
