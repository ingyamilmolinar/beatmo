package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// components_test.go covers parseComponents → resolveComponent → emitComponents,
// the actual heart of Phase-2 codegen. Round 1 tested only the top-level
// helpers (pascal, parseHexRGB, etc.); these tests exercise the resolution
// algorithm itself, including every error branch in resolveComponent.

// makeResolver builds a tokenResolver populated with a known set of names
// that the component fixtures below reference.
func makeResolver() *tokenResolver {
	return newTokenResolver(
		[]keyVal{
			{Key: "primary", Val: "#FF0000"},
			{Key: "accent", Val: "#00FF00"},
			{Key: "icon", Val: "#0000FF"},
			{Key: "border", Val: "#808080"},
		},
		[]keyVal{{Key: "md", Val: "8px"}, {Key: "lg", Val: "16px"}},
		[]keyVal{{Key: "sm", Val: "2px"}, {Key: "lg", Val: "8px"}},
		[]keyVal{{Key: "subtle", Val: "100"}, {Key: "strong", Val: "200"}},
	)
}

// parseComponentNode converts a YAML fragment into a `components:` mapping
// node ready to feed parseComponents.
func parseComponentNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var n yaml.Node
	if err := yaml.Unmarshal([]byte(src), &n); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	return n.Content[0]
}

func TestResolveComponentHappyPath(t *testing.T) {
	// One component exercising every supported field.
	src := `
button-primary:
  category: button
  backgroundColor: "{colors.primary}"
  iconColor: "{colors.icon}"
  rounded: "{rounded.sm}"
  height: "{spacing.md}"
  topEdgeHighlight: true
  border:
    color: "{colors.border}"
    alpha: subtle
  interaction:
    hover:
      fillDelta: 8
      borderDelta: 4
    press:
      fillDelta: -8
    focus:
      highlightDelta: 12
  dynamic:
    kind: color
    anchor: SwatchColorFn
  animation:
    kind: pulse
    anchor: PulseAnimator
`
	r := makeResolver()
	comps, err := parseComponents(parseComponentNode(t, src), r)
	if err != nil {
		t.Fatalf("parseComponents: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("len=%d", len(comps))
	}
	c := comps[0]

	if c.ID != "ButtonPrimary" || c.YAMLName != "button-primary" || c.Category != "button" {
		t.Errorf("identity: %+v", c)
	}
	if !c.HasFill || c.FillR != 0xFF || c.FillG != 0 || c.FillB != 0 {
		t.Errorf("fill: %+v", c)
	}
	if !c.HasIconColor || c.IconB != 0xFF {
		t.Errorf("icon: %+v", c)
	}
	if !c.HasRadius || c.Radius != 2 {
		t.Errorf("radius: %d", c.Radius)
	}
	if !c.HasHeight || c.Height != 8 {
		t.Errorf("height: %d", c.Height)
	}
	if !c.HasTopEdgeHighlight || !c.TopEdgeHighlight {
		t.Errorf("topEdgeHighlight: %+v", c)
	}
	if !c.HasBorder || c.BorderR != 0x80 || c.BorderAlpha != "genAlphaSubtle" {
		t.Errorf("border: %+v", c)
	}
	if !c.HasInteraction {
		t.Fatalf("interaction not set")
	}
	if !c.Interaction.Hover.NonZero || c.Interaction.Hover.FillDelta != 8 ||
		c.Interaction.Hover.BorderDelta != 4 {
		t.Errorf("hover: %+v", c.Interaction.Hover)
	}
	if !c.Interaction.Press.NonZero || c.Interaction.Press.FillDelta != -8 {
		t.Errorf("press: %+v", c.Interaction.Press)
	}
	if !c.Interaction.Focus.NonZero || c.Interaction.Focus.HighlightDelta != 12 {
		t.Errorf("focus: %+v", c.Interaction.Focus)
	}
	if c.DynamicAnchor != "SwatchColorFn" {
		t.Errorf("DynamicAnchor=%q", c.DynamicAnchor)
	}
	if c.AnimationAnchor != "PulseAnimator" {
		t.Errorf("AnimationAnchor=%q", c.AnimationAnchor)
	}
}

func TestResolveComponentBorderWithoutAlphaIsOpaque(t *testing.T) {
	// Border without an explicit alpha bucket falls back to "255" (opaque).
	src := `
chip:
  category: chip
  border:
    color: "{colors.border}"
`
	r := makeResolver()
	comps, err := parseComponents(parseComponentNode(t, src), r)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := comps[0].BorderAlpha; got != "255" {
		t.Errorf("BorderAlpha=%q want \"255\"", got)
	}
}

func TestResolveComponentErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"unresolved-bg-color",
			"x:\n  backgroundColor: \"{colors.missing}\"\n",
			"backgroundColor",
		},
		{
			"unresolved-icon-color",
			"x:\n  iconColor: \"{colors.missing}\"\n",
			"iconColor",
		},
		{
			"unresolved-border-color",
			"x:\n  border:\n    color: \"{colors.missing}\"\n",
			"border.color",
		},
		{
			"unknown-alpha-bucket",
			"x:\n  border:\n    color: \"{colors.border}\"\n    alpha: not-a-bucket\n",
			"border.alpha",
		},
		{
			"unresolved-rounded",
			"x:\n  rounded: \"{rounded.missing}\"\n",
			"rounded",
		},
		{
			"unresolved-height",
			"x:\n  height: \"{spacing.missing}\"\n",
			"height",
		},
		{
			"dynamic-missing-anchor",
			"x:\n  dynamic:\n    kind: color\n",
			"dynamic",
		},
		{
			"animation-missing-anchor",
			"x:\n  animation:\n    kind: pulse\n",
			"animation",
		},
	}
	r := makeResolver()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseComponents(parseComponentNode(t, c.src), r)
			if err == nil {
				t.Fatalf("want error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestParseComponentsRejectsUnknownField(t *testing.T) {
	// decodeComponentStrict uses KnownFields(true). A stray field name
	// must produce an error pointing to the offending component.
	src := "ghost:\n  bogusField: 1\n"
	_, err := parseComponents(parseComponentNode(t, src), makeResolver())
	if err == nil {
		t.Fatalf("want error on unknown field")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error %q does not name the offending component", err.Error())
	}
}

func TestParseComponentsNilNodeReturnsNil(t *testing.T) {
	got, err := parseComponents(nil, makeResolver())
	if err != nil || got != nil {
		t.Errorf("nil: got=%v err=%v", got, err)
	}
}

func TestParseComponentsRejectsSequence(t *testing.T) {
	var n yaml.Node
	_ = yaml.Unmarshal([]byte("- a\n- b\n"), &n)
	if _, err := parseComponents(n.Content[0], makeResolver()); err == nil {
		t.Errorf("sequence: want error")
	}
}

func TestDecodeComponentStrictBadShape(t *testing.T) {
	// A scalar node where a mapping is expected can't decode into componentRaw.
	var n yaml.Node
	_ = yaml.Unmarshal([]byte("just-a-string\n"), &n)
	if _, err := decodeComponentStrict(n.Content[0]); err == nil {
		t.Errorf("scalar: want error")
	}
}

func TestResolveDeltaNonZeroDetection(t *testing.T) {
	cases := []struct {
		in      interactionDeltaRaw
		nonzero bool
	}{
		{interactionDeltaRaw{}, false},
		{interactionDeltaRaw{FillDelta: 1}, true},
		{interactionDeltaRaw{BorderDelta: 1}, true},
		{interactionDeltaRaw{HighlightDelta: 1}, true},
		{interactionDeltaRaw{FillDelta: -1}, true}, // negative still counts
		{interactionDeltaRaw{FillDelta: 1, BorderDelta: -1, HighlightDelta: 1}, true},
	}
	for _, c := range cases {
		got := resolveDelta(&c.in)
		if got.NonZero != c.nonzero {
			t.Errorf("resolveDelta(%+v) NonZero=%v want %v", c.in, got.NonZero, c.nonzero)
		}
		// The integer deltas must pass through unchanged.
		if got.FillDelta != c.in.FillDelta || got.BorderDelta != c.in.BorderDelta ||
			got.HighlightDelta != c.in.HighlightDelta {
			t.Errorf("resolveDelta(%+v) deltas: %+v", c.in, got)
		}
	}
}

func TestEmitComponentsTemplateOutput(t *testing.T) {
	// Three components covering: full-featured, minimal, interaction-only.
	src := `
button-primary:
  category: button
  backgroundColor: "{colors.primary}"
  rounded: "{rounded.sm}"
  height: "{spacing.md}"
chip:
  category: chip
  border:
    color: "{colors.border}"
    alpha: subtle
toggle:
  interaction:
    hover:
      fillDelta: 5
    press:
      fillDelta: -5
`
	comps, err := parseComponents(parseComponentNode(t, src), makeResolver())
	if err != nil {
		t.Fatalf("parseComponents: %v", err)
	}
	out, err := emitComponents(comps)
	if err != nil {
		t.Fatalf("emitComponents: %v", err)
	}
	source := string(out)
	for _, want := range []string{
		"package ui",
		"ComponentButtonPrimary",
		"ComponentChip",
		"ComponentToggle",
		"componentCount",
		"BorderRef{",
		"genAlphaSubtle",
		"InteractionDelta",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("emitComponents output missing %q", want)
		}
	}
}

func TestEmitComponentsCategoryFallback(t *testing.T) {
	// A component without a category should still emit valid Go source —
	// the template's `categoryConst` helper has a fallback for the empty
	// string case.
	src := "no-cat:\n  backgroundColor: \"{colors.primary}\"\n"
	comps, _ := parseComponents(parseComponentNode(t, src), makeResolver())
	if _, err := emitComponents(comps); err != nil {
		t.Fatalf("emitComponents err=%v", err)
	}
}
