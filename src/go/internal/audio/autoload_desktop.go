//go:build !js && !test

package audio

// AutoLoadEmbeddedWAVs registers all embedded WAVs as instruments.
// On desktop, we write each asset to a temporary file and decode via miniaudio.
func AutoLoadEmbeddedWAVs() {
	// Catalog init already records embedded WAV metadata. Desktop path now
	// defers decoding until the user selects a sample.
	InitDefaultCatalog()
}
