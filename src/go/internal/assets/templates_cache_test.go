package assets

import "testing"

// TestTemplates_Cached guards the "opening the template menu hangs on WASM
// mobile" bug. The overflow/template menu calls assets.Templates() every frame
// from BOTH its draw and input paths while open. Uncached, each call re-reads
// and re-json.Unmarshals all ~31 embedded template files (several 150-285 KB) —
// ~21 ms per call on native, hundreds of ms on single-threaded WASM mobile —
// so simply having the menu open re-parses > 1 MB of JSON multiple times per
// frame and freezes the phone.
//
// Templates are embedded and immutable, so the parse result must be built once
// and reused. This asserts successive calls return the SAME backing storage
// (proving no re-read/re-parse), which fails on the uncached implementation.
func TestTemplates_Cached(t *testing.T) {
	a := Templates()
	b := Templates()
	if len(a) == 0 || len(b) == 0 {
		t.Fatal("Templates() returned no templates")
	}
	// Same backing array for the returned slice AND for each template's Bytes
	// means Templates() reused a cached result instead of re-reading the embed
	// and re-unmarshaling on every call.
	if &a[0].Bytes[0] != &b[0].Bytes[0] {
		t.Fatal("Templates() re-reads and re-parses every embedded template on " +
			"each call — the template menu (which calls it every frame) freezes " +
			"WASM mobile. Build the result once and cache it.")
	}
}
