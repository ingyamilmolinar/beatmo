package audio

import "testing"

func withDefaultAudio(t *testing.T) {
	t.Helper()
	Reset()
	ResetInstruments()
	ResetCatalogForTest(nil)
	t.Cleanup(func() {
		Reset()
		ResetInstruments()
		ResetCatalogForTest(nil)
	})
}
