//go:build test || js

package audio

// instanceAlreadyRegistered reports whether id is in the playable instrument
// registry. Under the test/wasm builds the registry is a flat id list (Register
// ignores the Instrument value), so membership is a list scan via Instruments().
func instanceAlreadyRegistered(id string) bool {
	for _, existing := range Instruments() {
		if existing == id {
			return true
		}
	}
	return false
}

// registerInstanceAlias adds id to the flat id registry. The test/wasm Register
// ignores its Instrument argument (no per-id Go renderer exists in these
// builds), so nil is correct; render equivalence in these builds is supplied
// entirely by the recipe binding + seeding the caller performs (wasm) or by the
// availability bookkeeping the test exercises.
func registerInstanceAlias(id, base string) {
	Register(id, nil)
}
