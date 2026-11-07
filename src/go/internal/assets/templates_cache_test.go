package assets

import "testing"

// The menu calls Templates() every frame from its draw and input paths, so it must return the SAME cached storage:
// re-parsing the embedded templates per call froze WASM mobile.
func TestTemplates_Cached(t *testing.T) {
	a := Templates()
	b := Templates()
	if len(a) == 0 || len(b) == 0 {
		t.Fatal("Templates() returned no templates")
	}
	// The same backing arrays prove nothing was re-read or re-parsed.
	if &a[0].Bytes[0] != &b[0].Bytes[0] {
		t.Fatal("Templates() re-reads and re-parses every embedded template on " +
			"each call — the template menu (which calls it every frame) freezes " +
			"WASM mobile. Build the result once and cache it.")
	}
}
