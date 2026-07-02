package synthmatch

import (
	"go/format"
	"regexp"
	"strings"
	"testing"
)

// hasKV reports whether s contains a `"key": value` entry, tolerant of the
// padding gofmt inserts to align composite-literal values by the longest key.
func hasKV(s, key, val string) bool {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s+` + regexp.QuoteMeta(val) + `\b`)
	return re.MatchString(s)
}

func TestPatchSeedBlock_UpdatesAndInserts(t *testing.T) {
	src := []byte("package audio\n\nvar saxSeed = RecipeParams{\n\t\"filter_cutoff\": 1000,\n\t\"gain\": 0.9, // keep comment\n}\n")
	out, err := PatchSeedBlock(src, "saxSeed", map[string]float64{"filter_cutoff": 1450, "amp_attack": 0.02})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !hasKV(s, "filter_cutoff", "1450") {
		t.Errorf("cutoff not updated:\n%s", s)
	}
	if !hasKV(s, "amp_attack", "0.02") {
		t.Errorf("new key not inserted:\n%s", s)
	}
	if !strings.Contains(s, "// keep comment") {
		t.Errorf("comment not preserved:\n%s", s)
	}
	// Output must be gofmt-stable (PatchSeedBlock returns formatted bytes).
	formatted, err := format.Source(out)
	if err != nil {
		t.Errorf("output not valid Go: %v", err)
	}
	if string(formatted) != s {
		t.Errorf("output is not gofmt-stable:\n%s", s)
	}
}

func TestPatchSeedBlock_UnknownVarErrors(t *testing.T) {
	_, err := PatchSeedBlock([]byte("package audio\n"), "nopeSeed", map[string]float64{"x": 1})
	if err == nil {
		t.Error("expected error for missing var block")
	}
}

func TestPatchSeedBlock_PreservesSlashComment(t *testing.T) {
	src := []byte("package audio\n\nvar xSeed = RecipeParams{\n\t\"filter_cutoff\": 600, // bright / open\n}\n")
	out, err := PatchSeedBlock(src, "xSeed", map[string]float64{"filter_cutoff": 1234})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "1234") || !strings.Contains(s, "bright / open") {
		t.Fatalf("bad patch: %s", s)
	}
}

func TestPatchSeedBlock_PreservesOtherKeysAndOrder(t *testing.T) {
	src := []byte("package audio\n\nvar s = RecipeParams{\n\t\"a\": 1,\n\t\"b\": 2,\n\t\"c\": 3,\n}\n")
	out, err := PatchSeedBlock(src, "s", map[string]float64{"b": 20})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	ia, ib, ic := strings.Index(s, `"a"`), strings.Index(s, `"b"`), strings.Index(s, `"c"`)
	if !(ia < ib && ib < ic) {
		t.Errorf("key order not preserved:\n%s", s)
	}
	if !hasKV(s, "a", "1") || !hasKV(s, "c", "3") {
		t.Errorf("untouched keys altered:\n%s", s)
	}
	if !hasKV(s, "b", "20") {
		t.Errorf("b not updated:\n%s", s)
	}
}
