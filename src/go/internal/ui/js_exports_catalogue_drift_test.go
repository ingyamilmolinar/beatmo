//go:build test

package ui

// The smoke test can only check catalogue → globalThis; this closes the other direction from the Go source,
// so no export ships untracked and no stale entry outlives its export. Double-underscore names are private
// diagnostics, not bridge surface.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Each must exist in audio.js.
var jsNativeCatalogueEntries = map[string]bool{
	"captureInstrumentPCM": true,
	// The Synth-tab graph render (audio.RenderDocForGraph's browser half).
	"renderDocPCM":      true,
	"decodeWavToPCM":    true,
	"idbDeleteSample":   true,
	"idbGetAllSamples":  true,
	"idbPutSample":      true,
	"registerSamplePCM": true,
	// Read by the js-only Stage-D drain with no Go registration.
	"getTimingTraces":   true,
	"resetTimingTraces": true,
	// Used by the browser benches only.
	"startAudioProbe":   true,
	"stopAudioProbe":    true,
	"audioProbeRaw":     true,
	"audioProbeReport":  true,
	"insertEffectHosts": true,
	// Called from package audio, outside this scan.
	"audioGraphStats":    true,
	"retuneInsertEffect": true,
	"resetDSPState":      true,
	// Called from package audio, outside this scan.
	"cancelLaneVoices":      true,
	"stopLaneAt":            true,
	"cancelAllLaneFuture":   true,
	"laneVoiceStateForTest": true,
	// Read through js.Global().Get from package audio, outside this scan.
	"warmInstrumentBatch":   true,
	"voiceCachedFor":        true,
	"isMelodicInstrumentJS": true,
	// Read through js.Global().Get from package audio, outside this scan.
	"flushPendingChannelOps": true,
	// Called from package audio, outside this scan.
	"setInstrumentBlock":   true,
	"clearInstrumentBlock": true,
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
		// Both layouts exist in CI.
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
