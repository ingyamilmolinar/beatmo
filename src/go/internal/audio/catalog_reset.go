//go:build !js && !test

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// ResetCatalogForTest allows tests (or callers) to inject a synthetic catalog.
// It is a no-op in production code paths.
func ResetCatalogForTest(entries []SoundMeta) {
	catalogMu.Lock()
	catalog = entries
	catalogByID = map[instrument.ID]SoundMeta{}
	for _, m := range entries {
		catalogByID[m.ID] = m
	}
	catalogMu.Unlock()
	bumpCatalogVersion()
}
