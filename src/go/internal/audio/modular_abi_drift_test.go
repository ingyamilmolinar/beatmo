//go:build !js

package audio

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The WASM bridge fills the block by position and C reads it as a struct, so the Go schema must mirror the C field
// order exactly; a field inserted mid-struct would scramble every later param. These parse the real C struct.

// The comment-stripped body of `typedef struct { ... } <structName>;` in header.
func cStructBlock(t *testing.T, header, structName string) string {
	t.Helper()
	raw, err := os.ReadFile(header)
	if err != nil {
		t.Fatalf("read %s: %v", header, err)
	}
	text := string(raw)
	// Isolate the typedef block ending in `} <structName>;`.
	endRE := regexp.MustCompile(`\}\s*` + regexp.QuoteMeta(structName) + `\s*;`)
	endLoc := endRE.FindStringIndex(text)
	if endLoc == nil {
		t.Fatalf("%s: typedef for %s not found", header, structName)
	}
	start := strings.LastIndex(text[:endLoc[0]], "typedef struct")
	if start < 0 {
		t.Fatalf("%s: typedef struct opener for %s not found", header, structName)
	}
	block := text[start:endLoc[0]]

	// Strip comments.
	block = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(block, "")
	block = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(block, "")

	return block
}

func cStructFields(t *testing.T, header, structName string) []string {
	t.Helper()
	block := cStructBlock(t, header, structName)

	var fields []string
	declRE := regexp.MustCompile(`(?m)^\s*(float|double|int)\s+([^;]+);`)
	arrayRE := regexp.MustCompile(`^(\w+)\[(\w+)\]$`)
	for _, m := range declRE.FindAllStringSubmatch(block, -1) {
		decls := m[2]
		for _, d := range strings.Split(decls, ",") {
			d = strings.TrimSpace(d)
			if am := arrayRE.FindStringSubmatch(d); am != nil {
				switch am[2] {
				case "FM_MAX_OPS":
					// fm_op_ratio[FM_MAX_OPS] flattens to fm_op1_ratio…
					base := am[1] // e.g. fm_op_ratio
					suffix := strings.TrimPrefix(base, "fm_op_")
					if suffix == base {
						t.Fatalf("%s.%s: array field outside the fm_op_* convention", structName, base)
					}
					for i := 1; i <= 4; i++ {
						fields = append(fields, fmt.Sprintf("fm_op%d_%s", i, suffix))
					}
				default:
					// gen_<f>[N] flattens field-major to gen1_<f>..genN_<f>.
					n, err := strconv.Atoi(am[2])
					if err != nil {
						t.Fatalf("%s.%s: unsupported array bound %q", structName, am[1], am[2])
					}
					base := am[1] // e.g. gen_source
					suffix := strings.TrimPrefix(base, "gen_")
					if suffix == base {
						// A table column <f>[N] flattens to <f>[0]..<f>[N-1].
						for i := 0; i < n; i++ {
							fields = append(fields, fmt.Sprintf("%s[%d]", base, i))
						}
						continue
					}
					for i := 1; i <= n; i++ {
						fields = append(fields, fmt.Sprintf("gen%d_%s", i, suffix))
					}
				}
				continue
			}
			fields = append(fields, d)
		}
	}
	if len(fields) == 0 {
		t.Fatalf("%s: no fields parsed for %s", header, structName)
	}
	return fields
}

func assertSchemaMatchesCStruct(t *testing.T, schema, cFields []string, what string) {
	t.Helper()
	if len(schema) != len(cFields) {
		t.Fatalf("%s: Go schema has %d names, C struct has %d fields\nGo: %v\nC:  %v",
			what, len(schema), len(cFields), schema, cFields)
	}
	for i := range schema {
		if schema[i] != cFields[i] {
			t.Errorf("%s: position %d: Go schema %q != C field %q — positional ABI drift",
				what, i, schema[i], cFields[i])
		}
	}
}

func TestModularParamsSchemaMatchesCStruct(t *testing.T) {
	root := repoRootForC(t)
	cFields := cStructFields(t, filepath.Join(root, "src/c/modular_params.gen.h"), "modular_params")
	assertSchemaMatchesCStruct(t, ModularParamSchema(), cFields, "modular_params")
}

// Double, so a literal lifted out of C into config is the SAME 64-bit value, not its float32 rounding.
func TestModularParamsAreDouble(t *testing.T) {
	root := repoRootForC(t)
	block := cStructBlock(t, filepath.Join(root, "src/c/modular_params.gen.h"), "modular_params")
	if regexp.MustCompile(`(?m)^\s*float\s`).MatchString(block) {
		t.Fatal("modular_params still declares float fields — every field must be double")
	}
}

// A `float x = p->field` or float-typed helper rounds the value back at the READ, so the voice sources compile with
// -Wfloat-conversion and must emit no implicit double→float conversion.
func TestModularParamsAreNeverNarrowed(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler on PATH")
	}
	root := repoRootForC(t)
	var report []string
	for _, src := range []string{"modular.c", "modular_stages.c", "modal_bank.c", "fmsynth.c", "adsr.c", "drums.c"} {
		out, _ := exec.Command(cc, "-fsyntax-only", "-Wfloat-conversion", "-I", filepath.Join(root, "src/c"),
			filepath.Join(root, "src/c", src)).CombinedOutput()
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "[-Wfloat-conversion]") {
				report = append(report, line)
			}
		}
	}
	if len(report) > 0 {
		t.Fatalf("%d implicit double→float conversions in the modular voice (every one rounds a config value to float32):\n%s",
			len(report), strings.Join(report, "\n"))
	}
}

func TestOnlyTheModularParamBlockCrossesTheABI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootForC(t), "src/js/synth_param_abi.gen.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SYNTH_PARAM_") {
		t.Fatal("synth_param_abi.gen.js still exports the 7-field SYNTH_PARAM_* block")
	}
}

// A hand edit or stale regeneration fails here.
func TestGeneratedCHeaderIsCurrent(t *testing.T) {
	root := repoRootForC(t)
	cmd := exec.Command(goBinaryPath(), "run", "./cmd/gen-synth-abi", "-c")
	cmd.Dir = filepath.Join(root, "src", "go")
	got, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go run ./cmd/gen-synth-abi -c: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("go run ./cmd/gen-synth-abi -c: %v", err)
	}
	have, err := os.ReadFile(filepath.Join(root, "src", "c", "modular_params.gen.h"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, have) {
		t.Fatal("src/c/modular_params.gen.h is stale: cd src/go && go run ./cmd/gen-synth-abi -c -o ../c/modular_params.gen.h")
	}
}
