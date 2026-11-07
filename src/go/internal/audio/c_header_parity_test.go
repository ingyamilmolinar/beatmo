//go:build !js

package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// Every instrument renders through render_modular_p with a full param block; a second entry point is how C-side
// defaults would creep back.
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
