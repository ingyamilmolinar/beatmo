//go:build !js

package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
//   - synth_params   (src/c/synth_params.h) ↔ SynthParamSchema()
//   - modular_params (src/c/modular.h)      ↔ ModularParamSchema()
//   - fm_params      (src/c/fmsynth.h)      ↔ FMParamSchema()

// cStructFields extracts float field names in declaration order from the
// typedef struct { ... } <name>; block. Handles comma declarator lists
// (`float a, b, c;`), the embedded `synth_params base;` member (expanded to
// the base schema), and the FM op arrays (`float fm_op_x[FM_MAX_OPS];` →
// fm_op1_x..fm_op4_x, matching the schema's flattened names).
func cStructFields(t *testing.T, header, structName string) []string {
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

	var fields []string
	declRE := regexp.MustCompile(`(?m)^\s*(float|int|synth_params)\s+([^;]+);`)
	arrayRE := regexp.MustCompile(`^(\w+)\[(\w+)\]$`)
	for _, m := range declRE.FindAllStringSubmatch(block, -1) {
		typ, decls := m[1], m[2]
		if typ == "synth_params" {
			// Embedded base block occupies the first 7 positional slots.
			fields = append(fields, SynthParamSchema()...)
			continue
		}
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
				case "12":
					// Phase-1 gen bank by-field array: gen_<f>[12] flattens to
					// gen1_<f>..gen12_<f> (field-major, matching the schema).
					base := am[1] // e.g. gen_source
					suffix := strings.TrimPrefix(base, "gen_")
					if suffix == base {
						t.Fatalf("%s.%s: [12] array field outside the gen_* convention", structName, base)
					}
					for i := 1; i <= 12; i++ {
						fields = append(fields, fmt.Sprintf("gen%d_%s", i, suffix))
					}
				default:
					t.Fatalf("%s.%s: unsupported array bound %q", structName, am[1], am[2])
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

func TestSynthParamsSchemaMatchesCStruct(t *testing.T) {
	root := repoRootForC(t)
	cFields := cStructFields(t, filepath.Join(root, "src/c/synth_params.h"), "synth_params")
	assertSchemaMatchesCStruct(t, SynthParamSchema(), cFields, "synth_params")
}

func TestModularParamsSchemaMatchesCStruct(t *testing.T) {
	root := repoRootForC(t)
	cFields := cStructFields(t, filepath.Join(root, "src/c/modular.h"), "modular_params")
	assertSchemaMatchesCStruct(t, ModularParamSchema(), cFields, "modular_params")
}

// TestFMParamsSchemaMatchesCStruct deleted with the FM-family migration (Phase-7,
// the LAST family): the fm_params C struct was removed (fmsynth.h) — FM knobs now
// travel in the wide modular_params block (gen_fm_* columns), drift-tested by
// TestModularParamsSchemaMatchesCStruct above. No FM family block remains.
