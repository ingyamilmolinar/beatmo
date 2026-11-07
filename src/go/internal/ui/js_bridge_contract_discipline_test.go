//go:build test

package ui

// The Go↔JS seam fails OPEN: an argument Go omits takes a JS default nothing reconciles (the Spectrum tab
// went blank on 64 bins), and a `typeof X === "function"` guard naming a non-export silently runs a fallback
// that measures the wrong thing.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A JS default is invisible from Go, so for these parameters it would be a second source of truth.
var noDefaultBridgeFns = []string{
	"enableChannelAnalyzer",
	"channelAnalyzerSnapshot",
	"enablePreEQAnalyzer",
	"enableSynthAnalyzer",
	"setInstrumentBlock",
	"renderDocPCM",
}

var (
	// Arity parse: only the arrow form, which is what the bridge exports use.
	jsWindowFnRe = regexp.MustCompile(`(?m)^\s*(?:window|globalThis)\.([A-Za-z_$][\w$]*)\s*=\s*(?:async\s+)?\(([^)]*)\)\s*=>`)
	// ANY assignment to a global, whatever the right-hand side.
	jsWindowAnyRe = regexp.MustCompile(`(?m)^\s*(?:window|globalThis)\.([A-Za-z_$][\w$]*)\s*=[^=]`)
	goInvokeRe    = regexp.MustCompile(`js\.Global\(\)\.Get\("([^"]+)"\)`)
	typeofFnRe    = regexp.MustCompile(`typeof\s+(?:window|globalThis)\.?([A-Za-z_$][\w$]*)\s*===\s*["']function["']`)
	// `g := js.Global()` … `g.Set("name", …)` — the aliased registration form.
	goAliasedSetRe = regexp.MustCompile(`\bg\.Set\("([^"]+)"`)
)

// Three levels up lands on `src/` in the normal layout and on the repo root in the one CI uses.
func repoPath(rel ...string) string {
	withSrc := filepath.Join(append([]string{"..", "..", "..", "src"}, rel...)...)
	if _, err := os.Stat(filepath.Dir(withSrc)); err == nil {
		return withSrc
	}
	return filepath.Join(append([]string{"..", "..", ".."}, rel...)...)
}

func readRepoFile(t *testing.T, rel ...string) string {
	t.Helper()
	p := repoPath(rel...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func jsDeclaredArity(t *testing.T) map[string]int {
	t.Helper()
	src := readRepoFile(t, "js", "audio.js")
	out := map[string]int{}
	for _, m := range jsWindowFnRe.FindAllStringSubmatch(src, -1) {
		params := strings.TrimSpace(m[2])
		n := 0
		if params != "" {
			n = len(strings.Split(params, ","))
		}
		out[m[1]] = n
	}
	if len(out) < 20 {
		t.Fatalf("parsed only %d window.X = (…) => exports from audio.js — the scan is broken", len(out))
	}
	return out
}

func TestBridgeCallsPassEveryDeclaredArgument(t *testing.T) {
	arity := jsDeclaredArity(t)
	files, err := filepath.Glob(repoPath("go", "internal", "audio", "*_wasm.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no internal/audio/*_wasm.go files found (%v) — the scan is broken", err)
	}
	var src string
	for _, f := range files {
		src += readRepoFile(t, "go", "internal", "audio", filepath.Base(f)) + "\n"
	}

	want := map[string]bool{}
	for _, fn := range noDefaultBridgeFns {
		want[fn] = true
		if _, ok := arity[fn]; !ok {
			t.Errorf("%q is listed in noDefaultBridgeFns but audio.js declares no such window export — "+
				"the name is stale or the export moved", fn)
		}
	}

	seen := map[string]bool{}
	for _, loc := range goInvokeRe.FindAllStringSubmatchIndex(src, -1) {
		name := src[loc[2]:loc[3]]
		if !want[name] {
			continue
		}
		// Searched forward by OFFSET, not a line window: a comment between Get and Invoke must not hide the call.
		rest := src[loc[1]:]
		k := strings.Index(rest, ".Invoke(")
		if k < 0 {
			t.Errorf("%s: Get(%q) with no .Invoke after it — the scan cannot see this call site", name, name)
			continue
		}
		inner, args := invokeArgs(rest[k+len(".Invoke("):])
		seen[name] = true
		if args != arity[name] {
			t.Errorf("Go calls %s(%s) with %d argument(s) but audio.js declares %d. "+
				"The missing one falls back to a JS default that Go cannot see — exactly how "+
				"channelAnalyzerSnapshot silently ran at 64 bins and emptied the Spectrum tab's "+
				"bass bands in the browser. Pass every parameter explicitly.",
				name, inner, args, arity[name])
		}
	}
	for _, fn := range noDefaultBridgeFns {
		if !seen[fn] && arity[fn] > 0 {
			t.Errorf("no Go call site found for %q in internal/audio/*_wasm.go — if it moved, update this guard", fn)
		}
	}
}

// The guard idiom is fine; naming something never registered turns a typo, or a Go test helper taken for an
// export, into a skipped check or a fallback that reports the wrong measurement.
func TestTypeofGuardsNameRealExports(t *testing.T) {
	goFiles, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	known := map[string]bool{}
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range jsGlobalSetRe.FindAllStringSubmatch(string(b), -1) {
			known[m[1]] = true
		}
		// The aliased `g := js.Global(); g.Set(…)` form too: a name the scan cannot see looks like one that doesn't exist.
		for _, m := range goAliasedSetRe.FindAllStringSubmatch(string(b), -1) {
			known[m[1]] = true
		}
	}
	if len(known) < 100 {
		t.Fatalf("found only %d js.Global().Set registrations — scan is broken", len(known))
	}
	for name := range jsNativeCatalogueEntries {
		known[name] = true
	}
	// Every global any shipped JS file assigns, in any assignment form.
	jsFiles, err := filepath.Glob(repoPath("js", "*.js"))
	if err != nil {
		t.Fatalf("glob js: %v", err)
	}
	jsFiles = append(jsFiles, repoPath("js", "index.html"))
	nJS := 0
	for _, f := range jsFiles {
		base := filepath.Base(f)
		if strings.HasSuffix(base, ".test.js") {
			continue // a test's own helpers are not bridge surface
		}
		b, err := os.ReadFile(f)
		if err != nil {
			continue // index.html is optional in the alternate layout
		}
		for _, m := range jsWindowAnyRe.FindAllStringSubmatch(string(b), -1) {
			known[m[1]] = true
			nJS++
		}
	}
	if nJS < 50 {
		t.Fatalf("found only %d window.X assignments across src/js — the scan is broken", nJS)
	}

	testFiles, err := filepath.Glob(repoPath("js", "*.browser.test.js"))
	if err != nil {
		t.Fatalf("glob js tests: %v", err)
	}
	if len(testFiles) < 20 {
		t.Fatalf("found only %d browser tests — glob is broken", len(testFiles))
	}

	var bad []string
	for _, f := range testFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range typeofFnRe.FindAllStringSubmatch(string(b), -1) {
			name := m[1]
			switch {
			case known[name]:
			case strings.HasPrefix(name, "__"): // private debug hook, exempt by convention
			case len(name) <= 2: // a local (cb, f, …), not a bridge name
			case standardJSGlobals[name]:
			default:
				bad = append(bad, filepath.Base(f)+": "+name)
			}
		}
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Errorf("%s — `typeof %s === \"function\"` guards a name that is NOT a registered JS export, "+
			"not implemented in audio.js, and not a standard global. The guard will always be false, so "+
			"whatever it protects never runs (and any fallback beside it silently takes over). If it is a "+
			"Go helper, it is not reachable from the browser; if it is a new export, register it and add it "+
			"to the wasm_bridge_smoke catalogue.", strings.SplitN(b, ": ", 2)[0], strings.SplitN(b, ": ", 2)[1])
	}
}

var standardJSGlobals = map[string]bool{
	"AudioContext": true, "webkitAudioContext": true, "OfflineAudioContext": true,
	"requestAnimationFrame": true, "queueMicrotask": true, "structuredClone": true,
	"ResizeObserver": true, "IntersectionObserver": true, "gc": true,
	"showOpenFilePicker": true, "showSaveFilePicker": true, "reportError": true,
}

// Commas and the closing parenthesis inside a nested call (string(id)) do not count.
func invokeArgs(s string) (string, int) {
	depth, args, end := 0, 0, len(s)
	nonEmpty := false
	for i, r := range s {
		switch r {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				end = i
			}
			depth--
		case ',':
			if depth == 0 {
				args++
			}
		default:
			if r != ' ' && r != '\t' && r != '\n' {
				nonEmpty = true
			}
		}
		if end != len(s) {
			break
		}
	}
	inner := strings.TrimSpace(s[:end])
	if !nonEmpty || inner == "" {
		return inner, 0
	}
	return inner, args + 1
}

// Whichever assignment runs last wins, silently: an audio.js global named like a Go export once made Go's push
// call itself until the page died.
func TestAudioJSGlobalsNeverShadowGoExports(t *testing.T) {
	goExports := map[string]string{}
	for _, dir := range []string{".", filepath.Join("..", "audio")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, re := range []*regexp.Regexp{jsGlobalSetRe, goAliasedSetRe} {
				for _, m := range re.FindAllStringSubmatch(string(b), -1) {
					goExports[m[1]] = filepath.Base(f)
				}
			}
		}
	}
	if len(goExports) < 100 {
		t.Fatalf("found only %d Go-registered JS exports — the scan is broken", len(goExports))
	}
	src := readRepoFile(t, "js", "audio.js")
	seen := map[string]bool{}
	for _, m := range jsWindowAnyRe.FindAllStringSubmatch(src, -1) {
		name := m[1]
		if f, ok := goExports[name]; ok && !seen[name] {
			seen[name] = true
			t.Errorf("audio.js assigns window.%s, which Go also registers (%s): one silently replaces the other", name, f)
		}
	}
}
