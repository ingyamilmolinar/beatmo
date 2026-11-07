// The generator's schema check, run from the test binary so a run that skips codegen still catches authoring
// errors, plus its rejection paths on mutated copies.

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

// Catches drift where codegen is skipped (read-only CI, editor go test, a fresh checkout).
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

// The substring tracks gojsonschema's message format; failures print the full list for upgrades.
type schemaRejectionCase struct {
	name   string
	mutate func(map[string]any)
	want   string
}

// What strict unmarshal cannot catch: missing keys, type mismatches, format/pattern and range violations. Relaxing a
// rule means updating this in the same change.
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

// Converted the way the generator converts it.
func loadDesignAndSchema(t *testing.T) ([]byte, string) {
	t.Helper()
	docMap, uri := loadDesignMapAndSchema(t)
	jsonBytes, err := json.Marshal(docMap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return jsonBytes, uri
}

// A map the rejection cases mutate before re-marshalling.
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

// The generator is package main, so this mirrors its normalizeForJSON; keep the two equivalent.
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

// A moved schema fails here rather than as an opaque generator panic.
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

// A JSON round trip, fine for a small test fixture.
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
