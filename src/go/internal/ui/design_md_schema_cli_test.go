// The generator binary against malformed inputs exits non-zero, names the schema and the field's JSON pointer, so the
// CLI wiring is covered, not just the validator.

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// One targeted corruption per case, every output redirected into the temp dir.
func TestGeneratorRejectsSchemaViolations(t *testing.T) {
	if runtime.GOOS == "js" {
		t.Skip("CLI integration test requires host process exec")
	}

	repoRoot := findRepoRoot(t)
	designOrig, err := os.ReadFile(filepath.Join(repoRoot, "DESIGN.md"))
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(string) string
		wantSub []string
	}{
		{
			name: "version is not a string",
			mutate: func(in string) string {
				return strings.Replace(in, "version: \"1\"", "version: 1", 1)
			},
			wantSub: []string{"schema validation failed", "version", "Expected: string"},
		},
		{
			name: "version not in supported enum",
			mutate: func(in string) string {
				return strings.Replace(in, "version: \"1\"", "version: \"99\"", 1)
			},
			wantSub: []string{"schema validation failed", "version", "must be one of the following"},
		},
		{
			name: "color hex pattern violation",
			mutate: func(in string) string {
				return strings.Replace(in,
					"primary: \"#FFB30A\"",
					"primary: \"#abc\"",
					1)
			},
			wantSub: []string{"schema validation failed", "Does not match pattern"},
		},
		{
			name: "alpha out of range",
			mutate: func(in string) string {
				return strings.Replace(in,
					"strong:          180",
					"strong:          999",
					1)
			},
			wantSub: []string{"schema validation failed", "Must be less than or equal to 255"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := tc.mutate(string(designOrig))
			if mutated == string(designOrig) {
				t.Fatalf("mutator left DESIGN.md unchanged — adjust the search/replace anchor")
			}

			tmpDir := t.TempDir()
			tmpDesign := filepath.Join(tmpDir, "DESIGN.md")
			if err := os.WriteFile(tmpDesign, []byte(mutated), 0o644); err != nil {
				t.Fatalf("write tmp DESIGN.md: %v", err)
			}

			tmpTokens := filepath.Join(tmpDir, "tokens.gen.go")
			tmpComps := filepath.Join(tmpDir, "components.gen.go")
			tmpProfile := filepath.Join(tmpDir, "profile.gen.go")
			// Every output redirected, even the uninspected ones: on its reject path the generator once left the default-path
			// files deleted.
			tmpDensity := filepath.Join(tmpDir, "density.gen.go")
			tmpSequence := filepath.Join(tmpDir, "sequence.gen.go")
			schemaPath := filepath.Join(repoRoot, "scripts", "gen_design_tokens", "schema.json")

			goBin := filepath.Join(repoRoot, ".tools", "go", "bin", "go")
			if _, err := os.Stat(goBin); err != nil {
				goBin = "go"
			}
			cmd := exec.Command(goBin, "run", "./cmd/gen_design_tokens",
				"-design", tmpDesign,
				"-schema", schemaPath,
				"-out", tmpTokens,
				"-out-components", tmpComps,
				"-out-profile", tmpProfile,
				"-out-density", tmpDensity,
				"-out-sequence", tmpSequence,
			)
			cmd.Dir = filepath.Join(repoRoot, "src", "go")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("generator exited 0 on schema violation; expected non-zero. Output:\n%s", out)
			}
			combined := string(out)
			for _, want := range tc.wantSub {
				if !strings.Contains(combined, want) {
					t.Errorf("expected output to contain %q; full output:\n%s", want, combined)
				}
			}
		})
	}
}

// Bounded, so it never walks past the root.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "DESIGN.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("repo root (containing DESIGN.md) not found from %s", cwd)
	return ""
}
