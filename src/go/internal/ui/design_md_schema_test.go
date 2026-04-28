// Code review follow-up (B2): independent JSON Schema validation of
// DESIGN.md. The generator runs the same check at codegen time; this
// test runs it from the test binary so a CI run that doesn't shell out
// to `make gen-design-tokens` still catches schema-level authoring
// errors. It also verifies the validator's rejection paths — missing
// required fields, type mismatches, unknown keys, format violations —
// using mutated copies of DESIGN.md.

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// TestDesignMDValidatesAgainstSchema asserts the unmodified DESIGN.md
// front matter passes the schema. This is the same check the generator
// runs at `make gen-design-tokens` time; running it here catches drift
// in environments where the codegen step is skipped (read-only CI runs,
// editor go-test integrations, fresh checkout before `make`).
func TestDesignMDValidatesAgainstSchema(t *testing.T) {
	docJSON, schemaURI := loadDesignAndSchema(t)
	docLoader := gojsonschema.NewBytesLoader(docJSON)
	schemaLoader := gojsonschema.NewReferenceLoader(schemaURI)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !result.Valid() {
		var sb strings.Builder
		for _, e := range result.Errors() {
			fmt.Fprintf(&sb, "  %s: %s\n", e.Field(), e.Description())
		}
		t.Fatalf("DESIGN.md fails schema validation:\n%s", sb.String())
	}
}

// schemaRejectionCase parametrises a negative test: a mutator that
// deliberately corrupts the parsed YAML map, plus the substring we
// expect to see in at least one validation error description.
//
// The "want" substring tracks the gojsonschema error format. Test
// failure messages include the full error list so the substring can be
// updated in step with library upgrades.
type schemaRejectionCase struct {
	name   string
	mutate func(map[string]any)
	want   string
}

// TestSchemaRejectsAuthoringErrors covers the four classes of
// authoring mistake that strict yaml.v3 unmarshal cannot catch on its
// own: missing required keys, type mismatches, format/pattern
// violations, and value-range violations. Each case mutates a fresh
// in-memory copy of DESIGN.md and asserts the schema rejects it.
//
// If a future schema change relaxes one of these rules, the test
// should be updated in the same PR — the rule deletion is then a
// deliberate, reviewed change rather than an invisible regression.
func TestSchemaRejectsAuthoringErrors(t *testing.T) {
	docMap, schemaURI := loadDesignMapAndSchema(t)

	cases := []schemaRejectionCase{
		{
			name:   "missing required: colors",
			mutate: func(m map[string]any) { delete(m, "colors") },
			want:   "(root): colors is required",
		},
		{
			name:   "missing required: components",
			mutate: func(m map[string]any) { delete(m, "components") },
			want:   "(root): components is required",
		},
		{
			name:   "missing required: profileOverrides",
			mutate: func(m map[string]any) { delete(m, "profileOverrides") },
			want:   "(root): profileOverrides is required",
		},
		{
			name: "unknown top-level key",
			mutate: func(m map[string]any) {
				m["bogus"] = "value"
			},
			want: "Additional property bogus is not allowed",
		},
		{
			name: "wrong type for version (must be string)",
			mutate: func(m map[string]any) {
				m["version"] = 1
			},
			want: "Invalid type. Expected: string",
		},
		{
			name: "version not in enum",
			mutate: func(m map[string]any) {
				m["version"] = "999"
			},
			want: "version must be one of the following:",
		},
		{
			name: "color hex pattern violation (3-digit shorthand)",
			mutate: func(m map[string]any) {
				colors := m["colors"].(map[string]any)
				colors["test-bad"] = "#abc"
			},
			want: "Does not match pattern",
		},
		{
			name: "color hex pattern violation (no leading hash)",
			mutate: func(m map[string]any) {
				colors := m["colors"].(map[string]any)
				colors["test-bad"] = "112233"
			},
			want: "Does not match pattern",
		},
		{
			name: "spacing must end in px",
			mutate: func(m map[string]any) {
				spacing := m["spacing"].(map[string]any)
				spacing["test-bad"] = "10"
			},
			want: "Does not match pattern",
		},
		{
			name: "alpha out of range (>255)",
			mutate: func(m map[string]any) {
				alpha := m["alpha"].(map[string]any)
				alpha["test-bad"] = 999
			},
			want: "Must be less than or equal to 255",
		},
		{
			name: "alpha out of range (<0)",
			mutate: func(m map[string]any) {
				alpha := m["alpha"].(map[string]any)
				alpha["test-bad"] = -5
			},
			want: "Must be greater than or equal to 0",
		},
		{
			name: "key name violates kebab-case pattern",
			mutate: func(m map[string]any) {
				colors := m["colors"].(map[string]any)
				colors["BadCamelCase"] = "#ff0000"
			},
			want: "Additional property BadCamelCase is not allowed",
		},
		{
			name: "component category not in enum",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["category"] = "bogus-category"
			},
			want: "category must be one of the following",
		},
		{
			name: "component unknown field",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["mysteryField"] = "value"
			},
			want: "Additional property mysteryField is not allowed",
		},
		{
			name: "backgroundColor not a token reference",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["backgroundColor"] = "#ff0000"
			},
			want: "Does not match pattern",
		},
		{
			name: "border missing required color",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["border"] = map[string]any{
					"alpha": "border-thin",
				}
			},
			want: "color is required",
		},
		{
			name: "interaction delta out of range",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["interaction"] = map[string]any{
					"hover": map[string]any{"fillDelta": 999},
				}
			},
			want: "Must be less than or equal to 127",
		},
		{
			name: "dynamic missing required anchor",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["dynamic"] = map[string]any{
					"kind": "callback",
				}
			},
			want: "anchor is required",
		},
		{
			name: "animation kind not in enum",
			mutate: func(m map[string]any) {
				components := m["components"].(map[string]any)
				secondary := components["button-secondary"].(map[string]any)
				secondary["animation"] = map[string]any{
					"kind":   "telegraph",
					"anchor": "RecordPulseAnimator",
				}
			},
			want: "kind must be one of the following",
		},
		{
			name: "profileOverride field not an integer",
			mutate: func(m map[string]any) {
				profiles := m["profileOverrides"].(map[string]any)
				desktop := profiles["desktop"].(map[string]any)
				desktop["rowHeight"] = "twenty-eight"
			},
			want: "Invalid type. Expected: integer",
		},
		{
			name: "profileOverride field negative",
			mutate: func(m map[string]any) {
				profiles := m["profileOverrides"].(map[string]any)
				desktop := profiles["desktop"].(map[string]any)
				desktop["rowHeight"] = -1
			},
			want: "Must be greater than or equal to 0",
		},
		{
			name: "profileOverride profile is not an object",
			mutate: func(m map[string]any) {
				profiles := m["profileOverrides"].(map[string]any)
				profiles["tablet"] = "not-a-mapping"
			},
			want: "Invalid type. Expected: object",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutated := deepCopyMap(docMap)
			tc.mutate(mutated)

			jsonBytes, err := json.Marshal(mutated)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			docLoader := gojsonschema.NewBytesLoader(jsonBytes)
			schemaLoader := gojsonschema.NewReferenceLoader(schemaURI)
			result, err := gojsonschema.Validate(schemaLoader, docLoader)
			if err != nil {
				t.Fatalf("validate: %v", err)
			}
			if result.Valid() {
				t.Fatalf("schema accepted a document the test expected to reject. mutator should have produced an error containing %q", tc.want)
			}
			found := false
			var all strings.Builder
			for _, e := range result.Errors() {
				msg := e.Field() + ": " + e.Description()
				fmt.Fprintf(&all, "  %s\n", msg)
				if strings.Contains(msg, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("expected validation error containing %q; got:\n%s", tc.want, all.String())
			}
		})
	}
}

// loadDesignAndSchema reads DESIGN.md, extracts the YAML front matter,
// converts it to JSON via the same path the generator uses, and returns
// the JSON bytes plus the file:// URI of the schema. Used by the
// pass-case test.
func loadDesignAndSchema(t *testing.T) ([]byte, string) {
	t.Helper()
	docMap, uri := loadDesignMapAndSchema(t)
	jsonBytes, err := json.Marshal(docMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return jsonBytes, uri
}

// loadDesignMapAndSchema returns the parsed YAML front matter as a
// JSON-friendly map plus the schema's file:// URI. Used by the
// rejection-case test which mutates the map before re-marshalling.
func loadDesignMapAndSchema(t *testing.T) (map[string]any, string) {
	t.Helper()
	designPath := findDesignMD(t)
	body, err := os.ReadFile(designPath)
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	front := frontMatter(string(body))
	if front == "" {
		t.Fatalf("DESIGN.md has no YAML front matter")
	}

	var raw map[string]any
	if err := yaml.Unmarshal([]byte(front), &raw); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	doc := normalizeForJSONTest(raw).(map[string]any)

	schemaPath := findSchemaJSON(t, designPath)
	return doc, "file://" + schemaPath
}

// normalizeForJSONTest mirrors the generator's normalizeForJSON
// helper. Duplicated here rather than imported because the generator
// lives in package main; pulling it into a shared library would
// expand the scope of this PR. Both functions must stay equivalent —
// if either changes, update the other.
func normalizeForJSONTest(in any) any {
	switch v := in.(type) {
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			out[fmt.Sprint(k)] = normalizeForJSONTest(vv)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			out[k] = normalizeForJSONTest(vv)
		}
		return out
	case []any:
		for i := range v {
			v[i] = normalizeForJSONTest(v[i])
		}
		return v
	default:
		return v
	}
}

// findSchemaJSON walks up from designPath looking for the schema file
// the generator uses. Lives here in test code so that breaking the
// schema location (moving the file or renaming it) is a test failure
// rather than an opaque generator panic.
func findSchemaJSON(t *testing.T, designPath string) string {
	t.Helper()
	dir := filepath.Dir(designPath)
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, "scripts", "gen_design_tokens", "schema.json")
		if _, err := os.Stat(try); err == nil {
			return try
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("schema.json not found near %s", designPath)
	return ""
}

// deepCopyMap clones a map[string]any tree by JSON round-trip. This
// is acceptable for the schema-test fixture which is small and only
// runs in tests; production code paths use the generator's typed
// decode instead.
func deepCopyMap(m map[string]any) map[string]any {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		panic(err)
	}
	var out map[string]any
	if err := json.NewDecoder(&buf).Decode(&out); err != nil {
		panic(err)
	}
	return out
}
