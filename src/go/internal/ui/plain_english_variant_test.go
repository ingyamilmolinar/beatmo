package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Every gen-slot family labels its variant knob "Variant" and glosses are keyed by label, so the gloss must not
// name one family.
func TestVariantGlossDoesNotNameOneFamily(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		g := strings.ToLower(PlainEnglish("Variant"))
		if g == "" {
			t.Fatalf("%v: Variant has no gloss", loc)
		}
		for _, fam := range []string{"kick", "snare", "tom", "cymbal", "hat", "bombo", "caja", "platillo"} {
			if strings.Contains(g, fam) {
				t.Errorf("%v: Variant gloss %q names %q, but every family's Variant shares it", loc, g, fam)
			}
		}
	}
}
