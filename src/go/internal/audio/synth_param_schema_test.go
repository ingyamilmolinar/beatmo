package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// Phase 2 — synth_param_schema_test ensures the checked-in JS gen file
// matches what cmd/gen-synth-abi would emit RIGHT NOW. If the Go schema
// changes (reorder/add/remove a knob), the gen file must be regenerated
// in the same commit. This test prevents the failure mode where the C
// struct memory layout drifts from the JS write loop silently.
//
// We don't run the codegen at every audio test (it spawns a subprocess);
// only this dedicated test triggers it. The test is skipped under the
// `test` build path's race detector contexts where stdout capture in a
// subprocess can be flaky on shared CI runners — see goleak comment in
// main_test.go.
func TestSynthParamABIGenFileIsCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess codegen relies on POSIX go binary path")
	}
	// Find repo root by walking up from this test file's directory until
	// we hit src/. CLAUDE.md guarantees src/go/ is the Go module root and
	// src/js/ is the JS root, so synth_param_abi.gen.js lives at
	// src/js/synth_param_abi.gen.js relative to the repo root.
	_, thisFile, _, _ := runtime.Caller(0)
	srcGo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // src/go
	repoRoot := filepath.Dir(filepath.Dir(srcGo))               // repo root
	genJSPath := filepath.Join(repoRoot, "src", "js", "synth_param_abi.gen.js")

	want, err := os.ReadFile(genJSPath)
	if err != nil {
		t.Fatalf("read %s: %v", genJSPath, err)
	}

	// Run cmd/gen-synth-abi and capture its stdout. We use the same go
	// binary that's running this test so the schema source compiles
	// against the in-tree internal/audio package.
	goBin := goBinaryPath()
	cmd := exec.Command(goBin, "run", "./cmd/gen-synth-abi")
	cmd.Dir = srcGo
	got, err := cmd.Output()
	if err != nil {
		// Stderr is more useful than the wrapped error for the user.
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go run ./cmd/gen-synth-abi failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go run ./cmd/gen-synth-abi failed: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf(`synth_param_abi.gen.js is stale.
Regenerate with:
  cd src/go && go run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js

Diff (want = on-disk, got = generator output):
--- want (%d bytes) ---
%s
--- got (%d bytes) ---
%s`, len(want), string(want), len(got), string(got))
	}
}

// goBinaryPath returns the path to the go binary that executed this test.
// Falls back to PATH lookup when GOROOT is unavailable (e.g. shared CI).
func goBinaryPath() string {
	if goroot := runtime.GOROOT(); goroot != "" {
		return filepath.Join(goroot, "bin", "go")
	}
	return "go"
}

// TestSynthParamSchemaShape pins the schema list against the Go SynthParams
// struct so adding/removing a field on one side fails fast on the other.
// Pre-Phase-2 a desync would only show up at audio render time as
// "instrument got the wrong knob".
func TestSynthParamSchemaShape(t *testing.T) {
	got := SynthParamSchema()
	want := []string{"pitch", "decay", "tone", "drive", "body", "brightness", "fundamental"}
	if len(got) != len(want) {
		t.Fatalf("schema len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, n := range want {
		if got[i] != n {
			t.Errorf("schema[%d] = %q, want %q", i, got[i], n)
		}
	}
	idx := SynthParamSchemaIndexMap()
	for i, n := range want {
		if idx[n] != i {
			t.Errorf("index map %q = %d, want %d", n, idx[n], i)
		}
	}
	ident := SynthParamSchemaIdentity()
	for _, n := range want {
		if _, ok := ident[n]; !ok {
			t.Errorf("identity map missing %q", n)
		}
	}
}
