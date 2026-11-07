package audio

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"
)

// The derived schema must keep the shipped names, order and identities exactly; a deliberate reshape re-freezes the
// layout file in the same change.
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

// The engine reads every field as-is, so a Doc may not describe a substituted value ("(0: 300)", a NaN sentinel, a
// fallback): those notes are generated into the C header.
func TestFieldDocsDescribeNoSubstitutedDefaults(t *testing.T) {
	bad := regexp.MustCompile(`(?i)\(0: ?[0-9]|legacy|fallback|\bNaN\b|kp_get|mp_get`)
	for _, f := range modularFields {
		if bad.MatchString(f.Doc) {
			t.Errorf("%s.%s Doc %q describes a substituted default", f.Block, f.Name, f.Doc)
		}
	}
}
