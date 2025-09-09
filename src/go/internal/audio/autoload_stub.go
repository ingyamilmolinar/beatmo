//go:build test

package audio

// AutoLoadEmbeddedWAVs is a no-op in tests.
func AutoLoadEmbeddedWAVs() {}

// SampleLoadProgress in tests returns zeros.
func SampleLoadProgress() (int, int) { return 0, 0 }
