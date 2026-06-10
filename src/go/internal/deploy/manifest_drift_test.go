package deploy

// The browser-test harness and `make serve-lan` both serve files directly out
// of src/js/, so every committed file is reachable at runtime regardless of
// what scripts/deploy-wasm-gcs.sh stages into build/wasm_site/. The result is
// a silent class of bug: a new `import './X.gen.js'` in audio.js or a new
// `new URL('./Y_worklet.js', ...)` ships green on serve-lan + every browser
// test, then 404s on beatmo.io.
//
// This test parses src/js/index.html and src/js/audio.js for every relative
// URL the browser will fetch at runtime, then parses scripts/deploy-wasm-gcs.sh
// for its declared file buckets, and asserts the two sides agree.
//
// To add a new runtime dependency: add it to one of the *_FILES arrays in
// deploy-wasm-gcs.sh and this test will pass automatically.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the repository root, derived from this
// file's location so the test is stable under `go test` from any cwd.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../src/go/internal/deploy/manifest_drift_test.go -> repo root is 4 levels up
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// extractJSRefs returns the set of relative-URL filenames a single JS source
// file references at runtime: static imports, dynamic imports, new URL(...)
// targets, and fetch("...") arguments. Strips the leading "./".
func extractJSRefs(src string) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimPrefix(s, "./")
		if s == "" || strings.Contains(s, "://") || strings.HasPrefix(s, "/") {
			return
		}
		out[s] = struct{}{}
	}

	// `import ... from './X'` or `from "./X"`
	for _, m := range regexp.MustCompile(`(?m)from\s+['"](\./[^'"]+)['"]`).FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	// `import('./X')` dynamic
	for _, m := range regexp.MustCompile(`import\(\s*['"](\./[^'"]+)['"]\s*\)`).FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	// `new URL('./X', import.meta.url)` -- worklets + workers
	for _, m := range regexp.MustCompile(`new\s+URL\(\s*['"](\./[^'"]+)['"]\s*,\s*import\.meta\.url\s*\)`).FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	// `fetch("X.wasm")` / `fetch('X.wasm')` -- entry HTML uses no ./ prefix
	for _, m := range regexp.MustCompile(`fetch\(\s*['"]([^'"]+\.wasm)['"]\s*\)`).FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	return out
}

// extractHTMLRefs returns the filenames referenced by <script src="..."> and
// link/href tags, plus any fetch("...wasm") in inline scripts (re-uses the JS
// extractor for the inline-script case).
func extractHTMLRefs(html string) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimPrefix(s, "./")
		if s == "" || strings.Contains(s, "://") || strings.HasPrefix(s, "/") {
			return
		}
		out[s] = struct{}{}
	}
	for _, m := range regexp.MustCompile(`(?i)<script\b[^>]*\bsrc\s*=\s*['"]([^'"]+)['"]`).FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	for _, m := range regexp.MustCompile(`(?i)<link\b[^>]*\bhref\s*=\s*['"]([^'"]+\.(?:js|css))['"]`).FindAllStringSubmatch(html, -1) {
		add(m[1])
	}
	for k := range extractJSRefs(html) {
		out[k] = struct{}{}
	}
	return out
}

// extractDeployManifest reads deploy-wasm-gcs.sh and pulls the three bash
// arrays (ENTRY_FILES, ASSET_FILES, WASM_FILES) into one set of filenames.
func extractDeployManifest(t *testing.T, script string) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	// Match `NAME_FILES=( ... )` non-greedy, then strip quoted entries.
	arrayRE := regexp.MustCompile(`(?s)(?:ENTRY_FILES|ASSET_FILES|WASM_FILES)=\(\s*(.*?)\)`)
	itemRE := regexp.MustCompile(`"([^"]+)"`)
	matches := arrayRE.FindAllStringSubmatch(script, -1)
	if len(matches) == 0 {
		t.Fatal("could not find ENTRY_FILES/ASSET_FILES/WASM_FILES arrays in deploy-wasm-gcs.sh")
	}
	for _, m := range matches {
		for _, item := range itemRE.FindAllStringSubmatch(m[1], -1) {
			out[item[1]] = struct{}{}
		}
	}
	return out
}

func sortedKeys(m map[string]struct{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestDeployManifestCoversRuntimeRefs(t *testing.T) {
	root := repoRoot(t)
	html := mustRead(t, filepath.Join(root, "src", "js", "index.html"))
	audio := mustRead(t, filepath.Join(root, "src", "js", "audio.js"))
	script := mustRead(t, filepath.Join(root, "scripts", "deploy-wasm-gcs.sh"))

	runtimeRefs := map[string]struct{}{}
	for k := range extractHTMLRefs(html) {
		runtimeRefs[k] = struct{}{}
	}
	for k := range extractJSRefs(audio) {
		runtimeRefs[k] = struct{}{}
	}

	manifest := extractDeployManifest(t, script)

	// Sanity floor: if the parser regressed and matched nothing, fail loudly
	// instead of silently passing.
	if len(runtimeRefs) == 0 {
		t.Fatal("parsed zero runtime refs from index.html + audio.js; parser regression")
	}
	if len(manifest) == 0 {
		t.Fatal("parsed zero entries from deploy script; parser regression")
	}

	var missing []string
	for ref := range runtimeRefs {
		if _, ok := manifest[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("deploy script is missing files referenced at runtime by index.html + audio.js:\n  %s\n\nruntime refs: %v\nmanifest:    %v",
			strings.Join(missing, "\n  "),
			sortedKeys(runtimeRefs),
			sortedKeys(manifest),
		)
	}

	// Each manifest entry must also resolve to an actual file under src/js/.
	// This catches the inverse drift: a stale manifest entry whose source was
	// renamed or removed.
	var orphan []string
	for f := range manifest {
		p := filepath.Join(root, "src", "js", f)
		if _, err := os.Stat(p); err != nil {
			orphan = append(orphan, f)
		}
	}
	sort.Strings(orphan)
	if len(orphan) > 0 {
		t.Fatalf("deploy manifest lists files that do not exist under src/js/:\n  %s", strings.Join(orphan, "\n  "))
	}

	// index.html itself must be in the manifest (it's the entry point but
	// nothing references it by name from inside JS, so it must be declared
	// explicitly in the script).
	if _, ok := manifest["index.html"]; !ok {
		t.Error("deploy manifest must contain index.html (entry point)")
	}
}

func TestExtractJSRefsParserCoverage(t *testing.T) {
	// Smoke-test the JS extractor against a synthetic source covering every
	// reference shape the live audio.js uses. If any shape stops matching, a
	// new runtime dep can slip past the live test.
	src := `import { X } from './static.gen.js';
import("./dyn.js");
const u1 = new URL('./worklet_a.js', import.meta.url);
const u2 = new URL("./worker_b.js", import.meta.url);
const r  = await fetch("main.wasm");
const skip1 = new URL('https://elsewhere.example/x.js', import.meta.url);
const skip2 = import('/absolute.js');
`
	got := extractJSRefs(src)
	want := []string{"static.gen.js", "dyn.js", "worklet_a.js", "worker_b.js", "main.wasm"}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("extractJSRefs did not pick up %q from synthetic source; got %v", w, sortedKeys(got))
		}
	}
	if _, ok := got["x.js"]; ok {
		t.Error("extractJSRefs should skip external https:// URLs")
	}
	if _, ok := got["absolute.js"]; ok {
		t.Error("extractJSRefs should skip absolute /paths")
	}
}

func TestDeployManifestDetectsMissingRef(t *testing.T) {
	// Synthesize the failure mode: a JS source references a file that the
	// manifest doesn't declare. The same set-diff used by
	// TestDeployManifestCoversRuntimeRefs must catch it.
	jsSrc := `import { X } from './nope.gen.js'; const u = new URL('./nope_worklet.js', import.meta.url);`
	manifestScript := `ENTRY_FILES=( "index.html" )
ASSET_FILES=( "audio.js" )
WASM_FILES=( "main.wasm" )`

	runtimeRefs := extractJSRefs(jsSrc)
	manifest := extractDeployManifest(t, manifestScript)

	var missing []string
	for ref := range runtimeRefs {
		if _, ok := manifest[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	sort.Strings(missing)
	want := []string{"nope.gen.js", "nope_worklet.js"}
	if len(missing) != len(want) {
		t.Fatalf("missing=%v want=%v", missing, want)
	}
	for i, w := range want {
		if missing[i] != w {
			t.Fatalf("missing[%d]=%q want %q", i, missing[i], w)
		}
	}
}

func TestDeployManifestSetsExplicitMIMEAndCache(t *testing.T) {
	root := repoRoot(t)
	script := mustRead(t, filepath.Join(root, "scripts", "deploy-wasm-gcs.sh"))

	// Guarantees that the script does not regress to silent MIME inference.
	mustContain := []struct {
		needle, why string
	}{
		{`application/wasm`, "WASM streaming compile mandates Content-Type: application/wasm"},
		{`application/javascript`, "JS modules need explicit Content-Type to survive strict-MIME browsers"},
		{`text/html`, "index.html must be served with a real text/html type"},
		{`Cache-Control`, "Cache-Control must be set explicitly so CDN behavior is deterministic"},
		{`no-cache`, "entry files (index.html + *.gen.js) must use no-cache to pick up new bundles"},
		{`max-age=`, "asset files must declare a finite max-age"},
	}
	for _, m := range mustContain {
		if !strings.Contains(script, m.needle) {
			t.Errorf("deploy-wasm-gcs.sh missing %q -- %s", m.needle, m.why)
		}
	}
}
