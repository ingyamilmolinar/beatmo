package deploy

import "testing"

// wasm_section_test.go — unit-tests the section walker TestWasmAssetSizeBudget
// uses to tell a `make wasm` binary (-s -w, no `name` section) from the
// UNSTRIPPED one a *.browser.test.js suite leaves behind in src/js/main.wasm.
// The distinction is worth a test of its own because getting it wrong is
// silent in both directions: a false positive skips the size gate forever, a
// false negative reports the 0.82 MiB `name` section as a code-size regression.

// wasmModule assembles a minimal module: magic + version, then each section as
// (id, ULEB payload length, payload).
func wasmModule(sections ...[]byte) []byte {
	b := []byte("\x00asm\x01\x00\x00\x00")
	for _, s := range sections {
		b = append(b, s...)
	}
	return b
}

// customSection builds a custom (id 0) section named name with no body.
func customSection(name string) []byte {
	payload := append(uleb(uint64(len(name))), name...)
	return append(append([]byte{0}, uleb(uint64(len(payload)))...), payload...)
}

func uleb(v uint64) []byte {
	var out []byte
	for {
		c := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			c |= 0x80
		}
		out = append(out, c)
		if v == 0 {
			return out
		}
	}
}

func TestWasmHasCustomSection(t *testing.T) {
	// A long name forces a multi-byte ULEB length, so a single-byte-only
	// decoder cannot pass this table.
	long := ""
	for len(long) < 200 {
		long += "x"
	}

	cases := []struct {
		name string
		mod  []byte
		want string
		ok   bool
	}{
		{"stripped: no custom sections at all", wasmModule(), "name", false},
		{"unstripped: name section present", wasmModule(customSection("name")), "name", true},
		{"name section after other customs", wasmModule(customSection("producers"), customSection("name")), "name", true},
		{"only other customs (a -w build keeps go:buildid + producers)",
			wasmModule(customSection("go:buildid"), customSection("producers")), "name", false},
		{"a name-PREFIXED section is not the name section", wasmModule(customSection("names")), "name", false},
		{"multi-byte ULEB section length", wasmModule(customSection(long), customSection("name")), "name", true},
		{"non-custom section is skipped by payload length",
			wasmModule([]byte{10, 3, 1, 2, 3}, customSection("name")), "name", true},
		{"not a wasm module", []byte("not wasm at all"), "name", false},
		{"truncated payload does not panic", []byte("\x00asm\x01\x00\x00\x00\x00\x7f"), "name", false},
		{"empty input does not panic", nil, "name", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wasmHasCustomSection(tc.mod, tc.want); got != tc.ok {
				t.Errorf("wasmHasCustomSection(..., %q) = %v, want %v", tc.want, got, tc.ok)
			}
		})
	}
}
