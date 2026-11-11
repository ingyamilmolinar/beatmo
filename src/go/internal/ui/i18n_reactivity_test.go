package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// End-to-end: building a Game registers an i18n.OnChange listener that
// invalidates the DrumView's row caches on a locale switch.
func TestLocaleSwitchInvalidatesDrumViewCaches(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	if g.drum == nil {
		t.Skip("test game has no DrumView")
	}
	if g.i18nCancel != nil {
		defer g.i18nCancel()
	}
	// Seed a non-invalidated state, then switch locale.
	g.drum.rowCacheW = 123
	g.drum.rowsLayerDirty = false
	i18n.SetLocale(i18n.LocaleES)
	if g.drum.rowCacheW != 0 || !g.drum.rowsLayerDirty {
		t.Fatalf("locale switch did not invalidate row caches: rowCacheW=%d rowsLayerDirty=%v",
			g.drum.rowCacheW, g.drum.rowsLayerDirty)
	}
}
