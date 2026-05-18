//go:build !js

package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestEveryCRenderHasPVariant asserts that every render_X() prototype in
// src/c/drums.h and src/c/fmsynth.h has a matching render_X_p() prototype.
// This is the v1 freezing-surface guard from the Phase 2 plan: it forbids
// a future contributor from adding a new C renderer without a
// parameterized variant, which would silently fail to surface in the
// SynthRecipe registry. WAV-loader helpers and result_description are
// exempt because they aren't part of the synthesis dispatch.
func TestEveryCRenderHasPVariant(t *testing.T) {
	root := repoRootForC(t)
	for _, header := range []string{"src/c/drums.h", "src/c/fmsynth.h"} {
		path := filepath.Join(root, header)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(bytes)

		// Find every `void render_X(float *out, int sampleRate, int samples);`
		// — the unparameterized variant. The _p() variant has a 4th arg.
		renderRE := regexp.MustCompile(`(?m)^\s*void\s+(render_[a-z_]+)\s*\(float \*out, int sampleRate, int samples\)\s*;`)
		matches := renderRE.FindAllStringSubmatch(text, -1)
		if len(matches) == 0 {
			t.Errorf("no render_X prototypes found in %s — regex drift?", header)
			continue
		}
		for _, m := range matches {
			name := m[1]
			pVariant := name + "_p"
			if !strings.Contains(text, pVariant+"(") {
				t.Errorf("%s declares %s() but no matching %s() — every C renderer must be parameterized so the SynthRecipe registry can reach it", header, name, pVariant)
			}
		}
	}
}

// TestEveryRegisteredRecipeBelongsToCHeader (native build only) cross-checks
// the other direction: every recipe in the registry must reach a
// render_X_p() that is declared in one of the C headers. This catches
// orphan recipes whose render function was deleted at the C side.
//
// (We don't run this under -tags test because the stub build doesn't have
// CGo and the renderer table is empty there.)

// repoRootForC returns the repository root by walking up from the current
// test file's directory until it finds a directory containing src/c.
func repoRootForC(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned !ok")
	}
	dir := filepath.Dir(file)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "src", "c")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not locate repo root (no src/c directory found above test file)")
	return ""
}
