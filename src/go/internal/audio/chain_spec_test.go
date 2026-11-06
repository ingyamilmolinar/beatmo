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

// TestChainSpecGenFileIsCurrent ensures the checked-in src/js/chain_spec.gen.js
// matches what cmd/gen-chain-spec emits right now. If the canonical chain
// configuration in chain_spec.go changes (compressor params, soft-clip
// threshold, headroom, sample rate), the gen file must be regenerated in the
// same commit — otherwise the browser's WebAudio chain silently diverges from
// the desktop Go/C chain and the platforms sound different. Mirrors
// TestSynthParamABIGenFileIsCurrent.
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

// TestChainSpecLiveValues asserts the desktop audio path is wired to the
// spec, so the spec can't drift away from what the code does.
func TestChainSpecLiveValues(t *testing.T) {
	spec := CanonicalChainSpec()
	if env.AudioSampleRate.DefaultString() != strconv.Itoa(spec.SampleRate) {
		t.Errorf("desktop default sample rate %s != spec %d", env.AudioSampleRate.DefaultString(), spec.SampleRate)
	}
}
