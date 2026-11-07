//go:build !test && !js

package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// roster.All is ORDER, not just a set (ResetInstruments seeds the menu order from it), and the pre-commit hook only
// checks those who installed it.
func TestInstrumentIDsGenUpToDate(t *testing.T) {
	goBin := goToolForGenerator(t)

	// The generator writes into its working directory: run in place it would rewrite the files under test and always pass.
	root := t.TempDir()
	pkg := filepath.Join(root, "audio")
	assets := filepath.Join(root, "assets")
	for _, d := range []string{pkg, assets, filepath.Join(assets, "instruments")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	for src, dst := range map[string]string{
		"gen_instrument_ids.go":            filepath.Join(pkg, "gen_instrument_ids.go"),
		"../assets/instruments/index.json": filepath.Join(assets, "instruments", "index.json"),
		"../assets/startup_demo.json":      filepath.Join(assets, "startup_demo.json"),
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("reading generator input %s: %v", src, err)
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			t.Fatalf("writing %s: %v", dst, err)
		}
	}

	cmd := exec.Command(goBin, "run", "gen_instrument_ids.go")
	cmd.Dir = pkg
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("re-running the generator failed: %v\n%s", err, b)
	}

	// The generated roster package.
	for committedPath, freshPath := range map[string]string{
		"../instrument/roster/roster_gen.go": filepath.Join(root, "instrument", "roster", "roster_gen.go"),
	} {
		committed, err := os.ReadFile(committedPath)
		if err != nil {
			t.Fatalf("reading committed %s: %v", committedPath, err)
		}
		fresh, err := os.ReadFile(freshPath)
		if err != nil {
			t.Fatalf("generator produced no %s: %v", committedPath, err)
		}
		if string(fresh) != string(committed) {
			t.Errorf("%s is STALE — run `go generate ./internal/audio/`.\n"+
				"The roster constants must mirror the instrument "+
				"documents' index order (internal/assets/instruments/index.json); instOrder "+
				"is seeded from it, so a stale slice misorders the instrument menu.", committedPath)
		}
	}
}

// Prefers the repo's bundled toolchain.
func goToolForGenerator(t *testing.T) string {
	t.Helper()
	if p, err := filepath.Abs("../../../.tools/go/bin/go"); err == nil {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	p, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go toolchain available to re-run the generator: %v", err)
	}
	return p
}
