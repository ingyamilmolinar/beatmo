package audio

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

// TestModularFieldsReproduceTheShippedLayout is the safety net for moving the
// ABI into one table: the derived schema must keep the shipped names, order and
// identities exactly (frozen in testdata/modular_schema_layout.json before the
// table existed). Later Phase C tasks reshape the block deliberately and
// re-freeze this file in the same change.
func TestModularFieldsReproduceTheShippedLayout(t *testing.T) {
	raw, err := os.ReadFile("testdata/modular_schema_layout.json")
	if err != nil {
		t.Fatal(err)
	}
	var want []struct {
		Name     string
		Identity float64
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	got := ModularParamSchema()
	ident := modularFieldInits()
	if len(got) != len(want) {
		t.Fatalf("schema has %d fields, shipped layout %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i] != w.Name || ident[w.Name] != w.Identity {
			t.Fatalf("field %d: got %s=%v, want %s=%v", i, got[i], ident[got[i]], w.Name, w.Identity)
		}
	}
}

// TestFieldDocsDescribeNoSubstitutedDefaults: since Phase C the engine reads
// every field as-is, so a Doc may not say a value is swapped for another one
// ("(0: 300)", a NaN sentinel, a fallback, a legacy path) — those notes were
// generated straight into modular_params.gen.h and contradicted the code.
func TestFieldDocsDescribeNoSubstitutedDefaults(t *testing.T) {
	bad := regexp.MustCompile(`(?i)\(0: ?[0-9]|legacy|fallback|\bNaN\b|kp_get|mp_get`)
	for _, f := range modularFields {
		if bad.MatchString(f.Doc) {
			t.Errorf("%s.%s Doc %q describes a substituted default", f.Block, f.Name, f.Doc)
		}
	}
}
