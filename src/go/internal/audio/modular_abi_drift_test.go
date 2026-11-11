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

// ABI drift guards for the three positional param blocks. The WASM bridge
// fills heap[*_PARAM_INDEX[name]] by position and passes the block to C as a
// struct pointer, so the Go schema slices MUST mirror the C struct field
// declaration order exactly. modular_param_schema_test.go pins the Go side
// to a hand-maintained want-list, but until now NOTHING verified that list
// against the C headers — inserting a field mid-struct in modular.h while
// appending it to the Go schema would scramble every following param
// (the "synth generator silenced by a foreign param write" class of bug).
//
// These tests parse the real C structs:
//   - modular_params (src/c/modular.h)      ↔ ModularParamSchema()
//   - fm_params      (src/c/fmsynth.h)      ↔ FMParamSchema()

// cStructFields extracts float field names in declaration order from the
// typedef struct { ... } <name>; block. Handles comma declarator lists
// (`float a, b, c;`) and the FM op arrays (`float fm_op_x[FM_MAX_OPS];` →
// fm_op1_x..fm_op4_x, matching the schema's flattened names).
// cStructBlock returns the comment-stripped body of the `typedef struct { ... }
// <structName>;` block in header.
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
					// fm_op_ratio[FM_MAX_OPS] flattens to fm_op1_ratio.. in the
					// schema: the index lands after the "fm_op" prefix.
					base := am[1] // e.g. fm_op_ratio
					suffix := strings.TrimPrefix(base, "fm_op_")
					if suffix == base {
						t.Fatalf("%s.%s: array field outside the fm_op_* convention", structName, base)
					}
					for i := 1; i <= 4; i++ {
						fields = append(fields, fmt.Sprintf("fm_op%d_%s", i, suffix))
					}
				default:
					// Gen-bank by-field array: gen_<f>[N] flattens to
					// gen1_<f>..genN_<f> (field-major, matching the schema).
					n, err := strconv.Atoi(am[2])
					if err != nil {
						t.Fatalf("%s.%s: unsupported array bound %q", structName, am[1], am[2])
					}
					base := am[1] // e.g. gen_source
					suffix := strings.TrimPrefix(base, "gen_")
					if suffix == base {
						// A stage TABLE column array: <f>[N] flattens to
						// <f>[0]..<f>[N-1] (instrument.TableCell).
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

// TestFMParamsSchemaMatchesCStruct deleted with the FM-family migration (Phase-7,
// the LAST family): the fm_params C struct was removed (fmsynth.h) — FM knobs now
// travel in the wide modular_params block (gen_fm_* columns), drift-tested by
// TestModularParamsSchemaMatchesCStruct above. No FM family block remains.

// TestModularParamsAreDouble pins Phase A of the config-only engine refactor
// (docs/superpowers/plans/2026-10-02-config-only-synth-engine.md): the modular
// block is double so a literal lifted out of C into config is the SAME 64-bit
// value (bit-exact), not its float32 rounding.
func TestModularParamsAreDouble(t *testing.T) {
	root := repoRootForC(t)
	block := cStructBlock(t, filepath.Join(root, "src/c/modular_params.gen.h"), "modular_params")
	if regexp.MustCompile(`(?m)^\s*float\s`).MatchString(block) {
		t.Fatal("modular_params still declares float fields — every field must be double")
	}
}

// TestModularParamsAreNeverNarrowed: Phase A's promise is that a config value
// reaches the DSP at full double precision. Declaring the struct double is not
// enough — a `float x = p->field`, an `lrintf(p->field)`, or a float-typed
// helper parameter rounds it back to float32 at the READ. Compile the voice
// sources with -Wfloat-conversion and require zero implicit double→float
// conversions (explicit `(float)` casts on output samples do not warn).
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

// TestOnlyTheModularParamBlockCrossesTheABI: the generated JS ABI carries the
// modular block only — no 7-field synth_params block.
func TestOnlyTheModularParamBlockCrossesTheABI(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRootForC(t), "src/js/synth_param_abi.gen.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SYNTH_PARAM_") {
		t.Fatal("synth_param_abi.gen.js still exports the 7-field SYNTH_PARAM_* block")
	}
}

// TestGeneratedCHeaderIsCurrent: src/c/modular_params.gen.h is generated from
// modular_fields.go by `go run ./cmd/gen-synth-abi -c`; a hand edit or a stale
// regeneration fails here (the JS half is TestSynthParamABIGenFileIsCurrent).
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
