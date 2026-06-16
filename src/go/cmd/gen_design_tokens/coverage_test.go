package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func regexpMatch(pattern, s string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// coverage_test.go covers pure parsing/rendering helpers in the
// gen_design_tokens generator. main() itself is not tested (it's the
// argv/os.Exit shell); every other helper is exercised with happy +
// failure paths so the cover tool sees both branches.

// ─── pascal / identifier conversion ───────────────────────────────────────

func TestPascal(t *testing.T) {
	cases := []struct{ in, want string }{
		{"primary", "Primary"},
		{"primary-bright", "PrimaryBright"},
		{"a-b-c", "ABC"},
		{"", ""},
		{"-leading", "Leading"},                    // empty first segment skipped
		{"trailing-", "Trailing"},                  // empty last segment skipped
		{"middle--double", "MiddleDouble"},         // adjacent dashes
		{"already-pascalCase", "AlreadyPascalCase"}, // lowercase second char preserved
	}
	for _, c := range cases {
		if got := pascal(c.in); got != c.want {
			t.Errorf("pascal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIdentForFamily(t *testing.T) {
	if got := identForColor("primary-bright"); got != "genColorPrimaryBright" {
		t.Errorf("identForColor: %q", got)
	}
	if got := identForSpacing("md"); got != "genSpacingMd" {
		t.Errorf("identForSpacing: %q", got)
	}
	if got := identForRounded("lg"); got != "genRoundedLg" {
		t.Errorf("identForRounded: %q", got)
	}
	if got := identForIcon("grid"); got != "genIconGrid" {
		t.Errorf("identForIcon: %q", got)
	}
	if got := identForAlpha("subtle"); got != "genAlphaSubtle" {
		t.Errorf("identForAlpha: %q", got)
	}
	if got := identForSwatch("808-sub"); got != "genInstrumentSwatch808Sub" {
		t.Errorf("identForSwatch: %q", got)
	}
}

// ─── parseHexRGB ──────────────────────────────────────────────────────────

func TestParseHexRGBValid(t *testing.T) {
	r, g, b, err := parseHexRGB("#FF8040")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if r != 0xFF || g != 0x80 || b != 0x40 {
		t.Errorf("got (%d,%d,%d) want (255,128,64)", r, g, b)
	}

	// Whitespace tolerance.
	if _, _, _, err := parseHexRGB("  #abcdef  "); err != nil {
		t.Errorf("whitespace: %v", err)
	}
}

func TestParseHexRGBInvalid(t *testing.T) {
	for _, s := range []string{
		"FF80",       // too short
		"#FF80",      // too short with prefix
		"#FF8040AA",  // too long
		"#GGHHII",    // non-hex
		"FF8040AA",   // too long without prefix
	} {
		if _, _, _, err := parseHexRGB(s); err == nil {
			t.Errorf("parseHexRGB(%q): want error", s)
		}
	}
}

// ─── parsePx ──────────────────────────────────────────────────────────────

func TestParsePx(t *testing.T) {
	cases := []struct {
		in   string
		want int
		err  bool
	}{
		{"12", 12, false},
		{"12px", 12, false},
		{" 0px ", 0, false},
		{"-4px", -4, false},
		{"abc", 0, true},
		{"12.5px", 0, true}, // floats rejected
	}
	for _, c := range cases {
		got, err := parsePx(c.in)
		if (err != nil) != c.err {
			t.Errorf("parsePx(%q) err=%v wantErr=%v", c.in, err, c.err)
		}
		if !c.err && got != c.want {
			t.Errorf("parsePx(%q) = %d want %d", c.in, got, c.want)
		}
	}
}

// ─── pxOuts / iconOuts ────────────────────────────────────────────────────

func TestPxOutsHappyPath(t *testing.T) {
	in := []keyVal{
		{Key: "sm", Val: "4px"},
		{Key: "md", Val: "8"},
	}
	out, err := pxOuts(in, identForSpacing)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 2 || out[0].Ident != "genSpacingSm" || out[0].Value != 4 ||
		out[1].Ident != "genSpacingMd" || out[1].Value != 8 {
		t.Errorf("unexpected output: %#v", out)
	}
}

func TestPxOutsBadValueErrors(t *testing.T) {
	if _, err := pxOuts([]keyVal{{Key: "x", Val: "boom"}}, identForSpacing); err == nil {
		t.Errorf("want error on non-numeric")
	}
}

func TestIconOutsRoutesByKey(t *testing.T) {
	in := []keyVal{
		{Key: "grid", Val: "16"},
		{Key: "padding", Val: "2"},
		{Key: "radius", Val: "4"},
		{Key: "stroke", Val: "1.5"},
	}
	ints, floats, err := iconOuts(in)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(ints) != 3 || len(floats) != 1 {
		t.Fatalf("ints=%d floats=%d want 3,1", len(ints), len(floats))
	}
	if floats[0].Value != 1.5 {
		t.Errorf("stroke=%v", floats[0].Value)
	}
}

func TestIconOutsRejectsUnknownKey(t *testing.T) {
	if _, _, err := iconOuts([]keyVal{{Key: "extra", Val: "1"}}); err == nil {
		t.Errorf("want error on unknown key")
	}
	if _, _, err := iconOuts([]keyVal{{Key: "stroke", Val: "nope"}}); err == nil {
		t.Errorf("want error on bad stroke value")
	}
	if _, _, err := iconOuts([]keyVal{{Key: "grid", Val: "x"}}); err == nil {
		t.Errorf("want error on bad grid value")
	}
}

// ─── parseRef (components.go) ─────────────────────────────────────────────

func TestParseRef(t *testing.T) {
	cases := []struct {
		in       string
		wantKey  string
		wantKind string
		ok       bool
	}{
		{"{colors.primary}", "primary", "colors", true},
		{"{spacing.md}", "md", "spacing", true},
		{"{rounded.lg}", "lg", "rounded", true},
		{" {colors.x} ", "x", "colors", true}, // whitespace
		{"colors.primary", "", "", false},      // missing braces
		{"{colors}", "", "", false},            // no dot
		{"{.primary}", "", "", false},          // empty kind
		{"{colors.}", "", "", false},           // empty key
	}
	for _, c := range cases {
		key, kind, ok := parseRef(c.in)
		if ok != c.ok || key != c.wantKey || kind != c.wantKind {
			t.Errorf("parseRef(%q) = (%q,%q,%v) want (%q,%q,%v)",
				c.in, key, kind, ok, c.wantKey, c.wantKind, c.ok)
		}
	}
}

// ─── tokenResolver ────────────────────────────────────────────────────────

func TestNewTokenResolverIgnoresInvalidValues(t *testing.T) {
	r := newTokenResolver(
		[]keyVal{{Key: "ok", Val: "#FF0000"}, {Key: "bad", Val: "not-hex"}},
		[]keyVal{{Key: "md", Val: "8px"}, {Key: "bad", Val: "boom"}},
		[]keyVal{{Key: "sm", Val: "4px"}, {Key: "bad", Val: "boom"}},
		[]keyVal{{Key: "subtle", Val: "0.4"}},
	)
	if rgb, ok := r.color("{colors.ok}"); !ok || rgb != [3]uint8{0xFF, 0, 0} {
		t.Errorf("color resolve: rgb=%v ok=%v", rgb, ok)
	}
	if _, ok := r.color("{colors.bad}"); ok {
		t.Errorf("invalid color should not resolve")
	}
	if _, ok := r.color("colors.ok"); ok {
		t.Errorf("non-ref string should not resolve")
	}
	if _, ok := r.color("{spacing.md}"); ok {
		t.Errorf("kind mismatch should reject")
	}
	if px, ok := r.spacingPx("{spacing.md}"); !ok || px != 8 {
		t.Errorf("spacing: %d ok=%v", px, ok)
	}
	if _, ok := r.spacingPx("{colors.ok}"); ok {
		t.Errorf("kind mismatch on spacingPx")
	}
	if px, ok := r.roundedPx("{rounded.sm}"); !ok || px != 4 {
		t.Errorf("rounded: %d ok=%v", px, ok)
	}
	if _, ok := r.roundedPx("{colors.ok}"); ok {
		t.Errorf("kind mismatch on roundedPx")
	}
	if !r.alphaBucket("subtle") {
		t.Errorf("alpha bucket missing")
	}
	if r.alphaBucket("nonexistent") {
		t.Errorf("unknown alpha bucket should be false")
	}
}

// ─── normalizeForJSON ─────────────────────────────────────────────────────

func TestNormalizeForJSONHandlesInterfaceMaps(t *testing.T) {
	in := map[any]any{
		1:        "num-key",
		"nested": map[any]any{"a": 1},
		"list":   []any{map[any]any{"k": "v"}},
		"plain":  map[string]any{"x": 2},
	}
	out := normalizeForJSON(in).(map[string]any)
	if out["1"] != "num-key" {
		t.Errorf("non-string key: %v", out["1"])
	}
	if nested := out["nested"].(map[string]any); nested["a"] != 1 {
		t.Errorf("nested: %v", nested)
	}
	list := out["list"].([]any)
	if got := list[0].(map[string]any)["k"]; got != "v" {
		t.Errorf("list-of-maps: %v", got)
	}
	if plain := out["plain"].(map[string]any); plain["x"] != 2 {
		t.Errorf("plain: %v", plain)
	}

	// Scalar passthrough.
	if got := normalizeForJSON("hello"); got != "hello" {
		t.Errorf("scalar passthrough: %v", got)
	}
}

// ─── parseFrontMatter / parseScalarNode ──────────────────────────────────

func TestParseFrontMatterValid(t *testing.T) {
	doc := []byte("---\nversion: \"1\"\nname: test\n---\nbody\n")
	spec, err := parseFrontMatter(doc)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if spec.Version != "1" || spec.Name != "test" {
		t.Errorf("spec=%+v", spec)
	}
}

func TestParseFrontMatterMissing(t *testing.T) {
	if _, err := parseFrontMatter([]byte("# no front matter")); err == nil {
		t.Errorf("want error when front matter missing")
	}
}

func TestParseFrontMatterRejectsUnknownFields(t *testing.T) {
	doc := []byte("---\nversion: \"1\"\nbogus: oops\n---\n")
	if _, err := parseFrontMatter(doc); err == nil {
		t.Errorf("KnownFields(true) should reject unknown top-level key")
	}
}

func TestParseScalarNodeMapping(t *testing.T) {
	src := []byte("a: 1\nb: 2\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Top-level node is a Document; descend to mapping.
	mapping := n.Content[0]
	out, err := parseScalarNode(mapping, "test")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 2 || out[0].Key != "a" || out[1].Val != "2" {
		t.Errorf("out=%+v", out)
	}

	// Empty/nil node is an error.
	if _, err := parseScalarNode(nil, "test"); err == nil {
		t.Errorf("nil node: want error")
	}

	// Sequence is rejected.
	var seq yaml.Node
	_ = yaml.Unmarshal([]byte("- a\n- b\n"), &seq)
	if _, err := parseScalarNode(seq.Content[0], "test"); err == nil {
		t.Errorf("sequence: want error")
	}
}

// ─── parseGeometry ────────────────────────────────────────────────────────

func TestParseGeometry(t *testing.T) {
	src := []byte("a: 1.5\nb: 2\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := parseGeometry(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 2 || out[0].Value != 1.5 || out[1].Ident != "genGeomB" {
		t.Errorf("out=%+v", out)
	}

	// nil returns nil, no error.
	if got, err := parseGeometry(nil); err != nil || got != nil {
		t.Errorf("nil node: got=%v err=%v", got, err)
	}

	// Bad value type → error.
	var bad yaml.Node
	_ = yaml.Unmarshal([]byte("a: not-a-number\n"), &bad)
	if _, err := parseGeometry(bad.Content[0]); err == nil {
		t.Errorf("want error on non-numeric value")
	}
}

// ─── filterAnims ──────────────────────────────────────────────────────────

func TestFilterAnimsByKind(t *testing.T) {
	in := []animationOut{
		{Ident: "a", Kind: "expDecay"},
		{Ident: "b", Kind: "sinPulse"},
		{Ident: "c", Kind: "expDecay"},
	}
	out := filterAnims(in, "expDecay")
	if len(out) != 2 || out[0].Ident != "a" || out[1].Ident != "c" {
		t.Errorf("filter expDecay: %+v", out)
	}
	if got := filterAnims(in, "missing"); len(got) != 0 {
		t.Errorf("filter missing: %d", len(got))
	}
}

// ─── parseProfileOverrides / emitProfiles ─────────────────────────────────

func TestParseProfileOverrides(t *testing.T) {
	src := []byte("desktop:\n  fontSize: 14\nmobile:\n  fontSize: 16\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, err := parseProfileOverrides(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got["desktop"]["fontSize"] != 14 || got["mobile"]["fontSize"] != 16 {
		t.Errorf("got=%v", got)
	}

	// nil → empty result, no error.
	if got, err := parseProfileOverrides(nil); err != nil || len(got) != 0 {
		t.Errorf("nil: %v %v", got, err)
	}

	// Non-mapping → error.
	var bad yaml.Node
	_ = yaml.Unmarshal([]byte("- a\n- b\n"), &bad)
	if _, err := parseProfileOverrides(bad.Content[0]); err == nil {
		t.Errorf("sequence: want error")
	}

	// Non-integer field value → error.
	var bad2 yaml.Node
	_ = yaml.Unmarshal([]byte("desktop:\n  bad: hello\n"), &bad2)
	if _, err := parseProfileOverrides(bad2.Content[0]); err == nil {
		t.Errorf("non-int: want error")
	}
}

func TestEmitProfilesProducesValidGo(t *testing.T) {
	in := profileSet{
		"desktop": {"fontSize": 14, "rowHeight": 32},
		"mobile":  {"fontSize": 16, "rowHeight": 48},
	}
	out, err := emitProfiles(in)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	src := string(out)
	for _, want := range []string{
		"package ui",
		"type profileValues struct",
		"genDesktopProfile",
		"genMobileProfile",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("emitProfiles output missing %q", want)
		}
	}
	// gofmt aligns field colons, so the exact whitespace between "FontSize:"
	// and "14" depends on the longest field name. Match each field with a
	// regex that tolerates one or more spaces after the colon.
	for _, want := range []string{
		`FontSize:\s+14`,
		`FontSize:\s+16`,
		`RowHeight:\s+32`,
		`RowHeight:\s+48`,
	} {
		matched, err := regexpMatch(want, src)
		if err != nil {
			t.Fatalf("regex %q: %v", want, err)
		}
		if !matched {
			t.Errorf("emitProfiles output missing pattern %q", want)
		}
	}
}

func TestEmitProfilesRequiresEveryFieldOnEveryProfile(t *testing.T) {
	in := profileSet{
		"desktop": {"a": 1, "b": 2},
		"mobile":  {"a": 3}, // missing "b"
	}
	if _, err := emitProfiles(in); err == nil {
		t.Errorf("want error when profile is missing a field")
	}
}

// ─── writeIfChanged ───────────────────────────────────────────────────────

func TestWriteIfChangedSkipsWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	content := []byte("hello")
	if err := writeIfChanged(path, content); err != nil {
		t.Fatalf("first write: %v", err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	mtime1 := info1.ModTime()

	// Second call with same content must not rewrite (mtime unchanged).
	if err := writeIfChanged(path, content); err != nil {
		t.Fatalf("second write: %v", err)
	}
	info2, _ := os.Stat(path)
	if !info2.ModTime().Equal(mtime1) {
		t.Errorf("identical content rewrote file (mtime changed)")
	}

	// Different content must rewrite.
	if err := writeIfChanged(path, []byte("world")); err != nil {
		t.Fatalf("third write: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, []byte("world")) {
		t.Errorf("file content not updated: got %q", got)
	}
}

func TestWriteIfChangedCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deeper", "nest", "out.txt")
	if err := writeIfChanged(path, []byte("x")); err != nil {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

// ─── resolveDesignPath ────────────────────────────────────────────────────

func TestResolveDesignPathAbsolutePassesThrough(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "DESIGN.md")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := resolveDesignPath(path)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != path {
		t.Errorf("got=%q want=%q", got, path)
	}
}

func TestResolveDesignPathNotFound(t *testing.T) {
	// Pick a file name nothing in the upward walk should accidentally
	// match. Walking up from CWD (the test working dir) should fail.
	if _, err := resolveDesignPath("__nonexistent_design_marker__.md"); err == nil {
		t.Errorf("want error when file not found")
	}
}

// ─── parseInstrumentSwatches ──────────────────────────────────────────────

func TestParseInstrumentSwatches(t *testing.T) {
	// Happy path.
	src := []byte("- name: kick-soft\n  hex: \"#FF0000\"\n- name: snare-tight\n  hex: \"#00FF00\"\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := parseInstrumentSwatches(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 2 || out[0].Name != "kick-soft" || out[0].R != 0xFF ||
		out[1].Ident != "genInstrumentSwatchSnareTight" {
		t.Errorf("out=%+v", out)
	}

	// nil → nil.
	if got, err := parseInstrumentSwatches(nil); err != nil || got != nil {
		t.Errorf("nil: got=%v err=%v", got, err)
	}

	// Mapping (not sequence) → error.
	var mapping yaml.Node
	_ = yaml.Unmarshal([]byte("a: 1\n"), &mapping)
	if _, err := parseInstrumentSwatches(mapping.Content[0]); err == nil {
		t.Errorf("mapping: want error")
	}

	// Missing required field.
	var missing yaml.Node
	_ = yaml.Unmarshal([]byte("- name: kick\n"), &missing)
	if _, err := parseInstrumentSwatches(missing.Content[0]); err == nil {
		t.Errorf("missing hex: want error")
	}

	// Unknown key.
	var bad yaml.Node
	_ = yaml.Unmarshal([]byte("- name: x\n  hex: \"#000000\"\n  extra: y\n"), &bad)
	if _, err := parseInstrumentSwatches(bad.Content[0]); err == nil {
		t.Errorf("unknown key: want error")
	}

	// Bad hex value.
	var badHex yaml.Node
	_ = yaml.Unmarshal([]byte("- name: x\n  hex: not-hex\n"), &badHex)
	if _, err := parseInstrumentSwatches(badHex.Content[0]); err == nil {
		t.Errorf("bad hex: want error")
	}
}

// ─── parseInstrumentDefaults ──────────────────────────────────────────────

func TestParseInstrumentDefaults(t *testing.T) {
	src := []byte("- id: kick\n  color: \"#112233\"\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := parseInstrumentDefaults(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 1 || out[0].ID != "kick" || out[0].R != 0x11 ||
		out[0].Ident != "genInstrumentDefaultKick" {
		t.Errorf("out=%+v", out)
	}

	if got, err := parseInstrumentDefaults(nil); err != nil || got != nil {
		t.Errorf("nil: got=%v err=%v", got, err)
	}

	// Mapping → error.
	var mapping yaml.Node
	_ = yaml.Unmarshal([]byte("a: 1\n"), &mapping)
	if _, err := parseInstrumentDefaults(mapping.Content[0]); err == nil {
		t.Errorf("mapping: want error")
	}

	// Unknown key.
	var bad yaml.Node
	_ = yaml.Unmarshal([]byte("- id: x\n  color: \"#000000\"\n  extra: y\n"), &bad)
	if _, err := parseInstrumentDefaults(bad.Content[0]); err == nil {
		t.Errorf("unknown key: want error")
	}

	// Missing required.
	var missing yaml.Node
	_ = yaml.Unmarshal([]byte("- id: only\n"), &missing)
	if _, err := parseInstrumentDefaults(missing.Content[0]); err == nil {
		t.Errorf("missing color: want error")
	}
}

// ─── parseInstrumentFallbackPalette ───────────────────────────────────────

func TestParseInstrumentFallbackPalette(t *testing.T) {
	src := []byte("- color: \"#AABBCC\"\n- color: \"#DDEEFF\"\n")
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := parseInstrumentFallbackPalette(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 2 || out[0].R != 0xAA || out[1].B != 0xFF {
		t.Errorf("out=%+v", out)
	}

	if got, err := parseInstrumentFallbackPalette(nil); err != nil || got != nil {
		t.Errorf("nil: got=%v err=%v", got, err)
	}

	// Wrong shape.
	var mapping yaml.Node
	_ = yaml.Unmarshal([]byte("a: 1\n"), &mapping)
	if _, err := parseInstrumentFallbackPalette(mapping.Content[0]); err == nil {
		t.Errorf("mapping: want error")
	}

	// Unknown key.
	var bad yaml.Node
	_ = yaml.Unmarshal([]byte("- color: \"#000000\"\n  extra: y\n"), &bad)
	if _, err := parseInstrumentFallbackPalette(bad.Content[0]); err == nil {
		t.Errorf("unknown key: want error")
	}
}

// ─── parseAnimations ──────────────────────────────────────────────────────

func TestParseAnimationsAllKinds(t *testing.T) {
	src := []byte(
		"glow:\n  kind: exp-decay\n  rate: 0.5\n  threshold: 0.01\n" +
			"pulse:\n  kind: sin-pulse\n  frame-step: 0.1\n  amplitude: 0.5\n  base: 1.0\n  alpha-scale: 200\n" +
			"fade-out:\n  kind: fade\n  factor: 0.95\n",
	)
	var n yaml.Node
	if err := yaml.Unmarshal(src, &n); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := parseAnimations(n.Content[0])
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	for _, a := range out {
		switch a.Name {
		case "glow":
			if a.Rate != 0.5 || a.Threshold != 0.01 {
				t.Errorf("glow: %#v", a)
			}
		case "pulse":
			if a.FrameStep != 0.1 || a.AlphaScale != 200 {
				t.Errorf("pulse: %#v", a)
			}
		case "fade-out":
			if a.Factor != 0.95 {
				t.Errorf("fade-out: %#v", a)
			}
		}
	}
}

func TestParseAnimationsErrorPaths(t *testing.T) {
	mkNode := func(src string) *yaml.Node {
		var n yaml.Node
		if err := yaml.Unmarshal([]byte(src), &n); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return n.Content[0]
	}

	// nil → nil, no error.
	if got, err := parseAnimations(nil); err != nil || got != nil {
		t.Errorf("nil: got=%v err=%v", got, err)
	}

	// Sequence → error (need mapping).
	if _, err := parseAnimations(mkNode("- a\n- b\n")); err == nil {
		t.Errorf("sequence: want error")
	}

	for _, c := range []struct {
		name, src, why string
	}{
		{"missing-kind", "x:\n  rate: 1\n", "missing kind discriminator"},
		{"unknown-kind", "x:\n  kind: bogus\n", "kind not in enum"},
		{"unknown-field", "x:\n  kind: fade\n  bogus: 1\n", "unknown per-entry key"},
		{"expdecay-missing-required", "x:\n  kind: exp-decay\n  rate: 1\n", "exp-decay missing threshold"},
		{"sinpulse-missing-required", "x:\n  kind: sin-pulse\n  base: 1\n", "sin-pulse missing fields"},
		{"fade-missing-required", "x:\n  kind: fade\n", "fade missing factor"},
		{"bad-rate", "x:\n  kind: exp-decay\n  rate: nope\n  threshold: 1\n", "non-numeric rate"},
		{"value-not-mapping", "x: scalar\n", "anim entry must be mapping"},
	} {
		if _, err := parseAnimations(mkNode(c.src)); err == nil {
			t.Errorf("%s: want error (%s)", c.name, c.why)
		}
	}
}

// ─── emitTokens (full happy path) ─────────────────────────────────────────

func TestEmitTokensProducesValidGoSource(t *testing.T) {
	colors := []keyVal{{Key: "primary", Val: "#FF0000"}, {Key: "accent", Val: "#00FF00"}}
	spacing := []keyVal{{Key: "sm", Val: "4px"}, {Key: "md", Val: "8px"}}
	rounded := []keyVal{{Key: "sm", Val: "2px"}}
	icon := []keyVal{
		{Key: "grid", Val: "16"},
		{Key: "padding", Val: "2"},
		{Key: "radius", Val: "4"},
		{Key: "stroke", Val: "1.5"},
	}
	alpha := []keyVal{{Key: "subtle", Val: "100"}, {Key: "overlay", Val: "200"}}
	swatches := []swatchOut{{Ident: "genInstrumentSwatchKick", Name: "kick", R: 1, G: 2, B: 3, SrcHex: "#010203"}}

	out, err := emitTokens(colors, spacing, rounded, icon, alpha, swatches, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	src := string(out)
	for _, want := range []string{
		"package ui",
		"genColorPrimary",
		"genColorAccent",
		"genSpacingSm",
		"genSpacingMd",
		"genRoundedSm",
		"genIconGrid",
		"genIconStroke",
		"genAlphaSubtle",
		"genAlphaOverlay",
		"genInstrumentSwatchKick",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("emitTokens output missing %q", want)
		}
	}
}

func TestEmitTokensRejectsBadHex(t *testing.T) {
	colors := []keyVal{{Key: "broken", Val: "not-hex"}}
	if _, err := emitTokens(colors, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Errorf("want error on bad color hex")
	}
}

func TestEmitTokensRejectsAlphaOutOfRange(t *testing.T) {
	icon := []keyVal{
		{Key: "grid", Val: "16"},
		{Key: "padding", Val: "2"},
		{Key: "radius", Val: "4"},
		{Key: "stroke", Val: "1"},
	}
	for _, bad := range []string{"-1", "256", "boom"} {
		alpha := []keyVal{{Key: "x", Val: bad}}
		if _, err := emitTokens(nil, nil, nil, icon, alpha, nil, nil, nil, nil, nil, nil); err == nil {
			t.Errorf("want error for alpha=%q", bad)
		}
	}
}

// ─── resolveSchemaPath ────────────────────────────────────────────────────

func TestResolveSchemaPathHintAbsolute(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	got, err := resolveSchemaPath(schemaPath, "")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != schemaPath {
		t.Errorf("got=%q want=%q", got, schemaPath)
	}
}

func TestResolveSchemaPathHintMissing(t *testing.T) {
	if _, err := resolveSchemaPath("/no/such/schema.json", ""); err == nil {
		t.Errorf("want error for missing hint")
	}
}

func TestResolveSchemaPathFallbackNotFound(t *testing.T) {
	// Place DESIGN.md in an isolated directory tree with no scripts/
	// directory anywhere upstream — schema lookup must fail cleanly.
	dir := t.TempDir()
	designPath := filepath.Join(dir, "DESIGN.md")
	if err := os.WriteFile(designPath, []byte(""), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := resolveSchemaPath("", designPath); err == nil {
		t.Errorf("want error when schema lookup walks off the tree")
	}
}

// ─── validateAgainstSchema ────────────────────────────────────────────────

func TestValidateAgainstSchemaHappy(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{
		"type": "object",
		"required": ["version"],
		"properties": {"version": {"type": "string"}}
	}`), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	body := []byte("---\nversion: \"1\"\n---\nbody\n")
	if err := validateAgainstSchema(body, schemaPath); err != nil {
		t.Errorf("happy path err: %v", err)
	}
}

func TestValidateAgainstSchemaMissingFrontMatter(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	_ = os.WriteFile(schemaPath, []byte(`{"type":"object"}`), 0o644)
	if err := validateAgainstSchema([]byte("# no front matter"), schemaPath); err == nil {
		t.Errorf("want error when front matter missing")
	}
}

func TestValidateAgainstSchemaInvalidDoc(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "schema.json")
	_ = os.WriteFile(schemaPath, []byte(`{
		"type": "object",
		"required": ["version"]
	}`), 0o644)
	body := []byte("---\nname: missing-version\n---\n")
	if err := validateAgainstSchema(body, schemaPath); err == nil {
		t.Errorf("want error when doc fails schema")
	}
}
