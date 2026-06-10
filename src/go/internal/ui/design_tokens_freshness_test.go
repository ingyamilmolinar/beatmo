package ui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDesignTokensFreshness re-runs the design-token generator into a
// tempdir and confirms each output byte-matches the committed file.
// Failure means DESIGN.md changed but `make gen-design-tokens` was not
// run; the diff in the failure tells you what to regenerate.
//
// Why this is not a substitute for design_md_drift_test.go: that test
// independently parses DESIGN.md and asserts each runtime constant
// matches, catching generator-vs-DESIGN.md divergence (the regenerated
// .gen.go and the committed .gen.go can both be wrong if the generator
// has a bug). Both tests are needed; they catch different bugs.
func TestDesignTokensFreshness(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("skipping on WASM (no go toolchain in browser tests)")
	}
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	srcDir := repoSubdir(t, filepath.Join("src", "go"))
	uiDir := filepath.Join(srcDir, "internal", "ui")

	tmpDir := t.TempDir()
	tmpTokens := filepath.Join(tmpDir, "design_tokens.gen.go")
	tmpComponents := filepath.Join(tmpDir, "design_components.gen.go")
	tmpProfile := filepath.Join(tmpDir, "design_profile.gen.go")
	tmpDensity := filepath.Join(tmpDir, "design_density.gen.go")

	goBin := goExecutable()
	cmd := exec.Command(goBin, "run", "./cmd/gen_design_tokens",
		"-design", "../../DESIGN.md",
		"-out", tmpTokens,
		"-out-components", tmpComponents,
		"-out-profile", tmpProfile,
		"-out-density", tmpDensity)
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	for _, pair := range []struct {
		name      string
		committed string
		regen     string
	}{
		{"design_tokens.gen.go", filepath.Join(uiDir, "design_tokens.gen.go"), tmpTokens},
		{"design_components.gen.go", filepath.Join(uiDir, "design_components.gen.go"), tmpComponents},
		{"design_profile.gen.go", filepath.Join(uiDir, "design_profile.gen.go"), tmpProfile},
		{"design_density.gen.go", filepath.Join(uiDir, "design_density.gen.go"), tmpDensity},
	} {
		committed, err := os.ReadFile(pair.committed)
		if err != nil {
			t.Errorf("read committed %s: %v", pair.name, err)
			continue
		}
		regenerated, err := os.ReadFile(pair.regen)
		if err != nil {
			t.Errorf("read regenerated %s: %v", pair.name, err)
			continue
		}
		if !bytes.Equal(committed, regenerated) {
			t.Errorf("%s is stale — DESIGN.md changed.\n"+
				"Run `make gen-design-tokens` to refresh.\n"+
				"committed=%d bytes, regenerated=%d bytes",
				pair.name, len(committed), len(regenerated))
		}
	}
}

// repoSubdir walks up from the test's cwd until it finds the requested
// directory under what looks like the repo root.
func repoSubdir(t *testing.T, sub string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, sub)
		// Require a go.mod sentinel so a stray nested directory (e.g. a
		// leftover src/go/src/go from a misdirected generator run) cannot
		// shadow the real repo src/go in the upward walk.
		if _, err := os.Stat(filepath.Join(try, "go.mod")); err == nil {
			return try
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("cannot find %s walking up from %s", sub, cwd)
	return ""
}

// goExecutable prefers the bundled toolchain at .tools/go/bin/go (the
// project standard, see CLAUDE.md), falling back to PATH.
func goExecutable() string {
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 8; i++ {
			try := filepath.Join(dir, ".tools", "go", "bin", "go")
			if _, err := os.Stat(try); err == nil {
				return try
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "go"
}
