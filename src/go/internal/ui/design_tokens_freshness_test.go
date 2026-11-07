package ui

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The generator's output into a temp dir byte-matches the committed files, or `make gen-design-tokens` was not
// run. The drift test catches a consistently wrong generator, which this cannot.
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
	// Not compared here, but redirected: its default is the real tracked templates file.
	tmpSequence := filepath.Join(tmpDir, "instrument_sequence.gen.go")

	goBin := goExecutable()
	cmd := exec.Command(goBin, "run", "./cmd/gen_design_tokens",
		"-design", "../../DESIGN.md",
		"-out", tmpTokens,
		"-out-components", tmpComponents,
		"-out-profile", tmpProfile,
		"-out-density", tmpDensity,
		"-out-sequence", tmpSequence)
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

func repoSubdir(t *testing.T, sub string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, sub)
		// A go.mod sentinel, so a stray nested src/go cannot shadow the real one.
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

// The bundled toolchain first, then PATH.
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
