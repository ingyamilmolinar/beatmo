package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestWASMRenderTableCoversBuiltinSynthInstruments is a Go↔JS drift guard.
//
// Every built-in instrument bound to a SynthRecipe (builtinInstrumentRecipeBindings)
// is rendered on WASM through the JS audio bridge, which keys on the static
// `RENDER` / `RENDER_INFO` tables in src/js/audio.js. If an instrument id is
// absent from those tables, playSoundParams(id) finds no render function, no
// registered sample, and throws "Unknown sound: <id>" — the instrument is
// SILENT in the browser even though it works on desktop (the native build
// renders it directly via render_modular, never consulting the JS table).
//
// This exact drift shipped `organ` and `sax` as Go builtins (config.go,
// instrument_ids_gen.go, the bindings below) without adding them to audio.js,
// so every template that used organ/sax produced no sound on beatmo.io while
// playing fine on desktop. Templates are "just audio config" and must drive the
// identical engine on both platforms; this test pins that contract so a new
// builtin can never reach the browser unrenderable again.
func TestWASMRenderTableCoversBuiltinSynthInstruments(t *testing.T) {
	path := filepath.Join("..", "..", "..", "js", "audio.js")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("audio.js not readable (%v) — skipping JS source guard", err)
	}
	src := string(b)

	render := jsMapKeys(t, src, "RENDER")
	renderInfo := jsMapKeys(t, src, "RENDER_INFO")

	// The config-first modular instrument family (kick voices, etc.) no longer
	// lives inline in audio.js's RENDER/RENDER_INFO literals: it is generated
	// into modular_instruments.gen.js and merged at runtime via
	// Object.assign(RENDER, MODULAR_RENDER) / Object.assign(RENDER_INFO,
	// MODULAR_RENDER_INFO). Union those generated keys so this guard sees the
	// same effective tables the browser does.
	genPath := filepath.Join("..", "..", "..", "js", "modular_instruments.gen.js")
	if gb, gerr := os.ReadFile(genPath); gerr == nil {
		gsrc := string(gb)
		for k := range jsFreezeMapKeys(t, gsrc, "MODULAR_RENDER") {
			render[k] = true
		}
		for k := range jsFreezeMapKeys(t, gsrc, "MODULAR_RENDER_INFO") {
			renderInfo[k] = true
		}
	} else {
		t.Logf("modular_instruments.gen.js not readable (%v) — relying on inline RENDER tables only", gerr)
	}

	for instID := range builtinInstrumentRecipeBindings {
		if !render[instID] {
			t.Errorf("instrument %q is a builtin recipe-bound synth but has no RENDER[%q] entry in audio.js — it will throw \"Unknown sound\" and be silent on WASM", instID, instID)
		}
		if !renderInfo[instID] {
			t.Errorf("instrument %q has no RENDER_INFO[%q] entry in audio.js — its WASM render duration/amp/paramBlock is undefined", instID, instID)
		}
	}
}

// jsMapKeys extracts the top-level keys of a `const <name> = { ... };` object
// literal in audio.js. Keys may be bare (snare:) or quoted ('bass-guitar':).
// Nested object keys (seconds:, amp:) sit after a `{` on the same line, never at
// line start, so the line-anchored pattern skips them.
func jsMapKeys(t *testing.T, src, name string) map[string]bool {
	t.Helper()
	head := "const " + name + " = {"
	start := indexAfter(src, head)
	if start < 0 {
		t.Fatalf("audio.js: could not find `%s`", head)
	}
	rest := src[start:]
	end := indexAfter(rest, "\n};")
	if end < 0 {
		t.Fatalf("audio.js: could not find end of `%s` object", name)
	}
	block := rest[:end]
	keyRe := regexp.MustCompile(`(?m)^\s*['"]?([A-Za-z0-9_-]+)['"]?\s*:`)
	out := make(map[string]bool)
	for _, m := range keyRe.FindAllStringSubmatch(block, -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("audio.js: parsed zero keys from `%s` — parser drift", name)
	}
	return out
}

// jsFreezeMapKeys extracts the top-level keys of a generated
// `export const <name> = Object.freeze({ ... });` object literal (the shape
// emitted by cmd/gen-modular-instruments). Same key grammar as jsMapKeys.
func jsFreezeMapKeys(t *testing.T, src, name string) map[string]bool {
	t.Helper()
	head := "const " + name + " = Object.freeze({"
	start := indexAfter(src, head)
	if start < 0 {
		t.Fatalf("modular_instruments.gen.js: could not find `%s`", head)
	}
	rest := src[start:]
	end := indexAfter(rest, "\n});")
	if end < 0 {
		t.Fatalf("modular_instruments.gen.js: could not find end of `%s` object", name)
	}
	block := rest[:end]
	keyRe := regexp.MustCompile(`(?m)^\s*['"]?([A-Za-z0-9_-]+)['"]?\s*:`)
	out := make(map[string]bool)
	for _, m := range keyRe.FindAllStringSubmatch(block, -1) {
		out[m[1]] = true
	}
	if len(out) == 0 {
		t.Fatalf("modular_instruments.gen.js: parsed zero keys from `%s` — parser drift", name)
	}
	return out
}

// indexAfter returns the index just past the first occurrence of sub in s, or
// -1 if absent.
func indexAfter(s, sub string) int {
	i := indexOf(s, sub)
	if i < 0 {
		return -1
	}
	return i + len(sub)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
