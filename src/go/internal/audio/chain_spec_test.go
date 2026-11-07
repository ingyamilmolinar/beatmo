package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// A chain value changed without regenerating the gen file makes the browser silently diverge from desktop.
func TestChainSpecGenFileIsCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("subprocess codegen relies on POSIX go binary path")
	}
	_, thisFile, _, _ := runtime.Caller(0)
	srcGo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile))) // src/go
	repoRoot := filepath.Dir(filepath.Dir(srcGo))               // repo root
	genJSPath := filepath.Join(repoRoot, "src", "js", "chain_spec.gen.js")

	want, err := os.ReadFile(genJSPath)
	if err != nil {
		t.Fatalf("read %s: %v", genJSPath, err)
	}

	goBin := goBinaryPath() // defined in synth_param_schema_test.go (same package)
	cmd := exec.Command(goBin, "run", "./cmd/gen-chain-spec")
	cmd.Dir = srcGo
	got, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go run ./cmd/gen-chain-spec failed: %v\nstderr:\n%s", err, ee.Stderr)
		}
		t.Fatalf("go run ./cmd/gen-chain-spec failed: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf(`chain_spec.gen.js is stale.
Regenerate with:
  cd src/go && go run ./cmd/gen-chain-spec > ../js/chain_spec.gen.js

Diff (want = on-disk, got = generator output):
--- want (%d bytes) ---
%s
--- got (%d bytes) ---
%s`, len(want), string(want), len(got), string(got))
	}
}

// So the spec can't drift from what the code does.
func TestChainSpecLiveValues(t *testing.T) {
	spec := CanonicalChainSpec()
	if env.AudioSampleRate.DefaultString() != strconv.Itoa(spec.SampleRate) {
		t.Errorf("desktop default sample rate %s != spec %d", env.AudioSampleRate.DefaultString(), spec.SampleRate)
	}
}
