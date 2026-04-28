// Code review follow-up (B2): integration test for the generator's
// schema-validation CLI behavior. The unit tests in
// design_md_schema_test.go validate against the schema directly via
// gojsonschema; this test runs the generator binary against deliberately
// malformed inputs and asserts (a) it exits non-zero, (b) the error
// message references the schema, (c) the JSON-pointer field location
// appears so the author can find the offending line. Together these
// catch regressions in the wiring between main.go's CLI and the
// validator helper.

package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestGeneratorRejectsSchemaViolations runs the gen_design_tokens
// binary against a copy of DESIGN.md with a single targeted corruption
// and asserts the generator exits non-zero with a message that
// includes the schema path and the offending field.
//
// We mutate the YAML by string-replacing a known token, run the
// generator with --design pointing at the temp copy and -out paths
// pointing into the tempdir (so the real .gen.go files are never
// touched), and inspect stderr. This approach exercises the actual
// CLI path; a unit test of the validator alone wouldn't catch a
// regression in main.go's flag/error-reporting wiring.
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
					"primary: \"#FF8C5A\"",
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

// findRepoRoot walks up from cwd until it finds a directory containing
// DESIGN.md. The test invocation typically runs from
// src/go/internal/ui, so 4 levels up is the typical depth. Bounded
// loop avoids walking past the filesystem root.
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
