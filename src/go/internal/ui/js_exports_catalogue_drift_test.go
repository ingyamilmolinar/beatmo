//go:build test

package ui

// Drift guard for the WASM bridge export catalogue.
//
// src/js/wasm_bridge_smoke.browser.test.js is the canonical catalogue of the
// JS bridge surface (see CLAUDE.md § JS Test Charter): every Go-registered
// export must have an entry there. The smoke test itself can only check
// catalogue→globalThis (its "ghost" pass) — it cannot enumerate what Go
// registers. This test closes the other direction at Go-source level: it
// scans every js.Global().Set("name", ...) registration in this package and
// fails when a name has no catalogue entry, so a new export cannot ship
// untracked.
//
// Reverse direction: every catalogue entry must either be a Go registration
// or appear in jsNativeCatalogueEntries (functions defined in src/js/audio.js
// rather than Go) — so deleting a Go export with a stale catalogue entry also
// fails here.
//
// Intentionally-private debug exports use a double-underscore prefix
// (__synthEvtTrace, __wasmReady, ...) and are exempt: they are internal
// diagnostics, not bridge surface.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// jsNativeCatalogueEntries are catalogue names implemented directly in
// src/js/audio.js (the sampler tab's JS-side surface), not registered from
// Go. Adding a name here requires the function to exist in audio.js.
var jsNativeCatalogueEntries = map[string]bool{
	"captureInstrumentPCM": true,
	"decodeWavToPCM":       true,
	"idbDeleteSample":      true,
	"idbGetAllSamples":     true,
	"idbPutSample":         true,
	"registerSamplePCM":    true,
}

var jsGlobalSetRe = regexp.MustCompile(`js\.Global\(\)\.Set\("([^"]+)"`)

func TestJSExportCatalogueDrift(t *testing.T) {
	goFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	goExports := map[string]bool{}
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue // registrations live in production files only
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range jsGlobalSetRe.FindAllStringSubmatch(string(data), -1) {
			goExports[m[1]] = true
		}
	}
	if len(goExports) < 100 {
		t.Fatalf("found only %d js.Global().Set registrations — scan is broken", len(goExports))
	}

	smokePath := filepath.Join("..", "..", "..", "src", "js", "wasm_bridge_smoke.browser.test.js")
	smokeRaw, err := os.ReadFile(smokePath)
	if err != nil {
		// Fall back to the repo-root-relative layout used when the package
		// path resolves differently (defensive; both layouts exist in CI).
		smokePath = filepath.Join("..", "..", "..", "js", "wasm_bridge_smoke.browser.test.js")
		smokeRaw, err = os.ReadFile(smokePath)
	}
	if err != nil {
		t.Fatalf("read smoke catalogue: %v", err)
	}
	smoke := string(smokeRaw)

	catalogueRe := regexp.MustCompile(`name: "([^"]+)"`)
	catalogue := map[string]bool{}
	for _, m := range catalogueRe.FindAllStringSubmatch(smoke, -1) {
		catalogue[m[1]] = true
	}
	if len(catalogue) < 100 {
		t.Fatalf("parsed only %d catalogue entries from %s — parse is broken", len(catalogue), smokePath)
	}

	var missing []string
	for name := range goExports {
		if strings.HasPrefix(name, "__") {
			continue // private debug export, exempt by convention
		}
		if !catalogue[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("Go export %q has no entry in wasm_bridge_smoke.browser.test.js — "+
			"add it to the CATALOGUE (existence-only `skipCall: true` is fine for "+
			"mutating/diagnostic exports)", name)
	}

	var stale []string
	for name := range catalogue {
		if !goExports[name] && !jsNativeCatalogueEntries[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("catalogue entry %q has no Go registration and is not in "+
			"jsNativeCatalogueEntries — remove the stale entry (or add it to the "+
			"JS-native allowlist if it moved to audio.js)", name)
	}

	for name := range jsNativeCatalogueEntries {
		if !catalogue[name] {
			t.Errorf("jsNativeCatalogueEntries lists %q but the catalogue has no such entry", name)
		}
	}
}
