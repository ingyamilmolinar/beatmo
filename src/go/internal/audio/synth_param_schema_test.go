package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The checked-in JS file must equal what the generator emits now: the JS write loop drifting from the C struct layout
// fails silently otherwise.
func TestSynthParamABIGenFileIsCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess codegen relies on POSIX go binary path")
	}
	// src/go is the module root and src/js the JS root.
	_, thisFile, _, _ := runtime.Caller(0)
	srcGo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // src/go
	repoRoot := filepath.Dir(filepath.Dir(srcGo))               // repo root
	genJSPath := filepath.Join(repoRoot, "src", "js", "synth_param_abi.gen.js")

	want, err := os.ReadFile(genJSPath)
	if err != nil {
		t.Fatalf("read %s: %v", genJSPath, err)
	}

	// The same go binary, so the generator compiles against the in-tree package.
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
  cd src/go && go run ./cmd/gen-synth-abi -o ../js/synth_param_abi.gen.js

Diff (want = on-disk, got = generator output):
--- want (%d bytes) ---
%s
--- got (%d bytes) ---
%s`, len(want), string(want), len(got), string(got))
	}
}

// Falls back to PATH when GOROOT is unavailable.
func goBinaryPath() string {
	if goroot := runtime.GOROOT(); goroot != "" {
		return filepath.Join(goroot, "bin", "go")
	}
	return "go"
}

func TestTableColumnsFlattenToCells(t *testing.T) {
	idx := ModularParamSchemaIndexMap()
	a, okA := idx["modal_freq[0]"]
	b, okB := idx["modal_freq[127]"]
	if !okA || !okB || b-a != 127 {
		t.Fatalf("modal_freq cells: [0]=%d,%v [127]=%d,%v", a, okA, b, okB)
	}
	if !IsTableField("modal_freq[5]") || !IsTableField("modal_count") || IsTableField("modal_tune") {
		t.Fatal("IsTableField misclassifies")
	}
	if _, ok := idx["modal_freq"]; ok {
		t.Fatal("a table column must not appear as a scalar")
	}
}
