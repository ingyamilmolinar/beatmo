package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestModularInstrumentsGenFileIsCurrent ensures the checked-in
// src/js/modular_instruments.gen.js matches what cmd/gen-modular-instruments
// emits right now. The file is generated from the config-first Go table
// modularInstrumentDefs (modular_instruments.go); if a row is added, cloned, or
// removed, the gen file must be regenerated in the same commit — otherwise the
// browser's RENDER/RENDER_INFO tables (which Object.assign the generated maps)
// silently diverge from the Go source of truth, leaving a new modular
// instrument silent on WASM ("Unknown sound"). Mirrors
// TestSynthParamABIGenFileIsCurrent and TestChainSpecGenFileIsCurrent.
func TestModularInstrumentsGenFileIsCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess codegen relies on POSIX go binary path")
	}
	_, thisFile, _, _ := runtime.Caller(0)
	srcGo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // src/go
	repoRoot := filepath.Dir(filepath.Dir(srcGo))               // repo root
	genJSPath := filepath.Join(repoRoot, "src", "js", "modular_instruments.gen.js")

	want, err := os.ReadFile(genJSPath)
	if err != nil {
		t.Fatalf("read %s: %v", genJSPath, err)
	}

	goBin := goBinaryPath() // defined in synth_param_schema_test.go (same package)
	cmd := exec.Command(goBin, "run", "./cmd/gen-modular-instruments")
	cmd.Dir = srcGo
	got, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go run ./cmd/gen-modular-instruments failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go run ./cmd/gen-modular-instruments failed: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf(`modular_instruments.gen.js is stale.
Regenerate with:
  cd src/go && go run ./cmd/gen-modular-instruments > ../js/modular_instruments.gen.js

Diff (want = on-disk, got = generator output):
--- want (%d bytes) ---
%s
--- got (%d bytes) ---
%s`, len(want), string(want), len(got), string(got))
	}
}
