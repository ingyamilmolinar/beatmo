//go:build !test && !js

package audio

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestInstrumentIDsGenUpToDate re-runs the generator against a COPY of its
// inputs and fails when the committed instrument_ids_gen.go no longer matches.
//
// This exists because the file went stale and nothing said so. roster.All
// is not just a set — ResetInstruments seeds instOrder straight from it
// (engine_instruments.go), so it is the ORDER the instrument menu lists within a
// category, and it used to be the order NewDrumView picked the default first row
// from. The generator's contract is that it mirrors ResetInstruments' registration
// order: engine_instruments.go's literals, then modular_instruments.go's table. When
// the kick/snare/clap conversion moved those ids from the literals into the table
// without a regen, the committed slice kept them in their old positions and quietly
// stopped describing the roster it claims to describe.
//
// A pre-commit hook rejects stale *.gen.go, but only for someone who ran
// `make install-hooks`, and only at commit time. This is the same check on the
// normal test path.
//
// Real build only: the generator parses the `!test && !js` engine_instruments.go,
// so under -tags test it would be comparing against a different source of truth.
func TestInstrumentIDsGenUpToDate(t *testing.T) {
	goBin := goToolForGenerator(t)

	// The generator resolves every path relative to its working directory and
	// WRITES instrument_ids_gen.go there (and ../instrument/roster/roster_gen.go),
	// so it runs over a copy — running it in place would rewrite the very files
	// under test and always pass.
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

// goToolForGenerator locates a go toolchain, preferring the repo's bundled one
// (the rest of the project pins it to avoid version drift).
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
