//go:build !js && !test

package audio

// ResetCatalogForTest allows tests (or callers) to inject a synthetic catalog.
// It is a no-op in production code paths.
func ResetCatalogForTest(entries []SoundMeta) {
	catalogMu.Lock()
	catalog = entries
	catalogByID = map[string]SoundMeta{}
	for _, m := range entries {
		catalogByID[m.ID] = m
	}
	catalogMu.Unlock()
	bumpCatalogVersion()
}
