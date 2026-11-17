//go:build !js

package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// TestTheEngineHasOneRenderEntryPoint: every instrument renders through
// render_modular_p with a full param block. The NULL-params render_modular
// (the engine's own defaults) was deleted in Phase C of the config-only
// refactor; a second entry point is how C-side defaults would creep back in.
func TestTheEngineHasOneRenderEntryPoint(t *testing.T) {
	root := repoRootForC(t)
	var found []string
	for _, header := range []string{"src/c/drums.h", "src/c/fmsynth.h", "src/c/modular.h"} {
		b, err := os.ReadFile(filepath.Join(root, header))
		if err != nil {
			t.Fatalf("read %s: %v", header, err)
		}
		for _, m := range regexp.MustCompile(`(?m)^\s*void\s+(render_[a-z_]+)\s*\(`).FindAllSubmatch(b, -1) {
			found = append(found, string(m[1]))
		}
	}
	if len(found) != 1 || found[0] != "render_modular_p" {
		t.Fatalf("render entry points = %v, want exactly [render_modular_p]", found)
	}
}

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
