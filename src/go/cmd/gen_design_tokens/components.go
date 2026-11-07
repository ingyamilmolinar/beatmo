package main

// Parses DESIGN.md's components: block (Stitch's shape plus category, border, iconColor, topEdgeHighlight,
// interaction), resolves {colors.X} / {spacing.X} / {rounded.X} / {alpha.X} and emits a ComponentSpec per entry. typography,
// textColor, variants, dynamic and animation are accepted so the YAML lints, but emit nothing.

import (
	"bytes"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Absence is "not specified" (a Go zero value). The JSON Schema runs first and locates errors by JSON pointer;
// KnownFields(true) here is the backup.
type componentRaw struct {
	Category         string          `yaml:"category"`
	BackgroundColor  string          `yaml:"backgroundColor"`
	TextColor        string          `yaml:"textColor"`
	IconColor        string          `yaml:"iconColor"`
	Rounded          string          `yaml:"rounded"`
	Height           string          `yaml:"height"`
	Typography       string          `yaml:"typography"`
	TopEdgeHighlight *bool           `yaml:"topEdgeHighlight"`
	Border           *borderRaw      `yaml:"border"`
	Interaction      *interactionRaw `yaml:"interaction"`
	Dynamic          *anchorRaw      `yaml:"dynamic"`          // populates ComponentSpec.DynamicAnchor; resolved at runtime via spec_registry.go
	Animation        *anchorRaw      `yaml:"animation"`        // populates ComponentSpec.AnimationAnchor; resolved at runtime via spec_registry.go
	Variants         map[string]any  `yaml:"variants"`         // reserved; not yet emitted
	ProfileOverrides map[string]any  `yaml:"profileOverrides"` // reserved; not yet emitted
}

type borderRaw struct {
	Color string `yaml:"color"`
	Alpha string `yaml:"alpha"`
}

type interactionRaw struct {
	Hover *interactionDeltaRaw `yaml:"hover"`
	Press *interactionDeltaRaw `yaml:"press"`
	Focus *interactionDeltaRaw `yaml:"focus"`
}

type interactionDeltaRaw struct {
	FillDelta      int `yaml:"fillDelta"`
	BorderDelta    int `yaml:"borderDelta"`
	HighlightDelta int `yaml:"highlightDelta"`
}

type anchorRaw struct {
	Kind   string `yaml:"kind"`
	Anchor string `yaml:"anchor"`
	Target string `yaml:"target"`
}

// component is componentRaw with every reference resolved to a concrete value.
type component struct {
	ID                        string // sanitised identifier ("button-secondary" → "ButtonSecondary")
	YAMLName                  string // original YAML key, for diagnostics + comments
	Category                  string
	HasFill                   bool
	FillR, FillG, FillB       uint8
	FillIdent                 string // genColor* identifier of the fill token (runtime-theme rebuild)
	HasBorder                 bool
	BorderR, BorderG, BorderB uint8
	BorderIdent               string // genColor* identifier of the border base token
	BorderAlpha               string // named bucket constant ("genAlphaSubtle"); empty if no border
	Radius                    int
	HasRadius                 bool
	Height                    int
	HasHeight                 bool
	HasIconColor              bool
	IconR, IconG, IconB       uint8
	IconIdent                 string // genColor* identifier of the icon token
	TopEdgeHighlight          bool
	HasTopEdgeHighlight       bool
	Interaction               interactionResolved
	HasInteraction            bool

	// DynamicAnchor names a per-instance runtime closure, AnimationAnchor a global animator; both bind through
	// SpecRegistry.Bind, and an anchor never registered panics at its first lookup.
	DynamicAnchor   string
	AnimationAnchor string
}

type interactionResolved struct {
	Hover, Press, Focus interactionDeltaResolved
}

type interactionDeltaResolved struct {
	FillDelta, BorderDelta, HighlightDelta int
	NonZero                                bool
}

type tokenResolver struct {
	colors  map[string][3]uint8 // key → (R, G, B); alpha is always 255 in DESIGN.md
	spacing map[string]int
	rounded map[string]int
	alphas  map[string]bool // set of valid bucket names, e.g. "subtle"
}

func newTokenResolver(colors, spacing, rounded, alpha []keyVal) *tokenResolver {
	r := &tokenResolver{
		colors:  map[string][3]uint8{},
		spacing: map[string]int{},
		rounded: map[string]int{},
		alphas:  map[string]bool{},
	}
	for _, kv := range colors {
		rr, gg, bb, err := parseHexRGB(kv.Val)
		if err != nil {
			continue
		}
		r.colors[kv.Key] = [3]uint8{rr, gg, bb}
	}
	for _, kv := range spacing {
		if v, err := parsePx(kv.Val); err == nil {
			r.spacing[kv.Key] = v
		}
	}
	for _, kv := range rounded {
		if v, err := parsePx(kv.Val); err == nil {
			r.rounded[kv.Key] = v
		}
	}
	for _, kv := range alpha {
		r.alphas[kv.Key] = true
	}
	return r
}

func (t *tokenResolver) color(ref string) (rgb [3]uint8, ok bool) {
	key, kind, valid := parseRef(ref)
	if !valid || kind != "colors" {
		return rgb, false
	}
	v, ok := t.colors[key]
	return v, ok
}

func (t *tokenResolver) spacingPx(ref string) (int, bool) {
	key, kind, valid := parseRef(ref)
	if !valid || kind != "spacing" {
		return 0, false
	}
	v, ok := t.spacing[key]
	return v, ok
}

func (t *tokenResolver) roundedPx(ref string) (int, bool) {
	key, kind, valid := parseRef(ref)
	if !valid || kind != "rounded" {
		return 0, false
	}
	v, ok := t.rounded[key]
	return v, ok
}

func (t *tokenResolver) alphaBucket(name string) bool {
	return t.alphas[name]
}

// parseRef splits "{colors.primary}" → ("primary", "colors", true).
// Returns (_, _, false) for any other shape.
func parseRef(s string) (key, kind string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return "", "", false
	}
	inner := s[1 : len(s)-1]
	dot := strings.IndexByte(inner, '.')
	if dot <= 0 || dot == len(inner)-1 {
		return "", "", false
	}
	return inner[dot+1:], inner[:dot], true
}

// Source order is kept so the emitted file diffs cleanly when DESIGN.md gains a component.
func parseComponents(node *yaml.Node, resolver *tokenResolver) ([]component, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("components: expected a mapping (got kind %d)", node.Kind)
	}
	out := make([]component, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		name := k.Value
		// yaml.v3 has no per-node KnownFields, so the value is re-encoded and decoded strictly through a Decoder.
		raw, err := decodeComponentStrict(v)
		if err != nil {
			return nil, fmt.Errorf("components.%s (line %d): %w", name, k.Line, err)
		}
		comp, err := resolveComponent(name, raw, resolver, k.Line)
		if err != nil {
			return nil, err
		}
		out = append(out, comp)
	}
	return out, nil
}

func decodeComponentStrict(node *yaml.Node) (*componentRaw, error) {
	// Round-trip through yaml.Marshal/Decode so KnownFields(true) applies.
	buf, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(buf))
	dec.KnownFields(true)
	var raw componentRaw
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func resolveComponent(name string, raw *componentRaw, r *tokenResolver, line int) (component, error) {
	c := component{ID: pascal(name), YAMLName: name, Category: raw.Category}

	if raw.BackgroundColor != "" {
		rgb, ok := r.color(raw.BackgroundColor)
		if !ok {
			return c, fmt.Errorf("components.%s.backgroundColor (line %d): unresolved %q", name, line, raw.BackgroundColor)
		}
		c.HasFill = true
		c.FillR, c.FillG, c.FillB = rgb[0], rgb[1], rgb[2]
		key, _, _ := parseRef(raw.BackgroundColor)
		c.FillIdent = identForColor(key)
	}

	if raw.IconColor != "" {
		rgb, ok := r.color(raw.IconColor)
		if !ok {
			return c, fmt.Errorf("components.%s.iconColor (line %d): unresolved %q", name, line, raw.IconColor)
		}
		c.HasIconColor = true
		c.IconR, c.IconG, c.IconB = rgb[0], rgb[1], rgb[2]
		key, _, _ := parseRef(raw.IconColor)
		c.IconIdent = identForColor(key)
	}

	if raw.Border != nil {
		rgb, ok := r.color(raw.Border.Color)
		if !ok {
			return c, fmt.Errorf("components.%s.border.color (line %d): unresolved %q", name, line, raw.Border.Color)
		}
		c.HasBorder = true
		c.BorderR, c.BorderG, c.BorderB = rgb[0], rgb[1], rgb[2]
		key, _, _ := parseRef(raw.Border.Color)
		c.BorderIdent = identForColor(key)
		// Omitted = opaque. A named bucket marks a translucent border (scrim, hairline) apart from an opaque coloured one
		// (Mute/Solo active).
		if raw.Border.Alpha == "" {
			c.BorderAlpha = "255"
		} else {
			if !r.alphaBucket(raw.Border.Alpha) {
				return c, fmt.Errorf("components.%s.border.alpha (line %d): %q is not a named bucket (see DESIGN.md `alpha:`)", name, line, raw.Border.Alpha)
			}
			c.BorderAlpha = "genAlpha" + pascal(raw.Border.Alpha)
		}
	}

	if raw.Rounded != "" {
		px, ok := r.roundedPx(raw.Rounded)
		if !ok {
			return c, fmt.Errorf("components.%s.rounded (line %d): unresolved %q", name, line, raw.Rounded)
		}
		c.HasRadius = true
		c.Radius = px
	}

	if raw.Height != "" {
		px, ok := r.spacingPx(raw.Height)
		if !ok {
			return c, fmt.Errorf("components.%s.height (line %d): unresolved %q", name, line, raw.Height)
		}
		c.HasHeight = true
		c.Height = px
	}

	if raw.TopEdgeHighlight != nil {
		c.HasTopEdgeHighlight = true
		c.TopEdgeHighlight = *raw.TopEdgeHighlight
	}

	if raw.Interaction != nil {
		c.HasInteraction = true
		if raw.Interaction.Hover != nil {
			c.Interaction.Hover = resolveDelta(raw.Interaction.Hover)
		}
		if raw.Interaction.Press != nil {
			c.Interaction.Press = resolveDelta(raw.Interaction.Press)
		}
		if raw.Interaction.Focus != nil {
			c.Interaction.Focus = resolveDelta(raw.Interaction.Focus)
		}
	}

	if raw.Dynamic != nil {
		if raw.Dynamic.Anchor == "" {
			return c, fmt.Errorf("components.%s.dynamic (line %d): missing anchor:", name, line)
		}
		c.DynamicAnchor = raw.Dynamic.Anchor
	}
	if raw.Animation != nil {
		if raw.Animation.Anchor == "" {
			return c, fmt.Errorf("components.%s.animation (line %d): missing anchor:", name, line)
		}
		c.AnimationAnchor = raw.Animation.Anchor
	}

	return c, nil
}

func resolveDelta(d *interactionDeltaRaw) interactionDeltaResolved {
	res := interactionDeltaResolved{
		FillDelta:      d.FillDelta,
		BorderDelta:    d.BorderDelta,
		HighlightDelta: d.HighlightDelta,
	}
	res.NonZero = res.FillDelta != 0 || res.BorderDelta != 0 || res.HighlightDelta != 0
	return res
}

// emitComponents renders design_components.gen.go.
func emitComponents(comps []component) ([]byte, error) {
	tpl := template.Must(template.New("components").Funcs(template.FuncMap{
		"id": func(c component) string { return c.ID },
		"categoryConst": func(s string) string {
			if s == "" {
				return "ComponentCategory(\"\")"
			}
			return "Category" + pascal(s)
		},
	}).Parse(componentsTemplate))

	// Source order = stable enum order.
	ids := make([]string, len(comps))
	for i, c := range comps {
		ids[i] = c.ID
	}
	if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i] < ids[j] }) {
		// keep source order
	}

	data := struct {
		Components []component
	}{comps}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt: %w\n--- raw output ---\n%s", err, buf.String())
	}
	return formatted, nil
}

const componentsTemplate = `// Code generated by cmd/gen_design_tokens. DO NOT EDIT.
//
// Source of truth: DESIGN.md ` + "`components:`" + ` block.
// Regenerate via: make gen-design-tokens

package ui

// ComponentID enum — one per DESIGN.md component entry, in source order.
const (
{{- range $i, $c := .Components}}
{{- if eq $i 0}}
	Component{{id $c}} ComponentID = iota
{{- else}}
	Component{{id $c}}
{{- end}}
{{- end}}
	componentCount
)

// componentSpecs is the generated registry, indexed by ComponentID. It is
// BUILT from the live genColor* token variables so a runtime theme
// (theme_runtime.go ApplyTheme) can rebuild it after overwriting them.
var componentSpecs = buildComponentSpecs()

// rebuildComponentSpecs re-derives the registry from the current token
// variables. Called by ApplyTheme after the genColor* writes.
func rebuildComponentSpecs() { componentSpecs = buildComponentSpecs() }

func buildComponentSpecs() [componentCount]ComponentSpec {
	return [componentCount]ComponentSpec{
{{- range .Components}}
	Component{{id .}}: {
		ID:       Component{{id .}},
		{{- if .Category}}
		Category: {{categoryConst .Category}},
		{{- end}}
		{{- if .HasFill}}
		Fill:     {{.FillIdent}},
		{{- end}}
		{{- if .HasBorder}}
		Border: BorderRef{
			BaseColor: {{.BorderIdent}},
			Alpha:     {{.BorderAlpha}},
		},
		{{- end}}
		{{- if .HasRadius}}
		Radius: {{.Radius}},
		{{- end}}
		{{- if .HasHeight}}
		Height: {{.Height}},
		{{- end}}
		{{- if .HasIconColor}}
		IconColor:    {{.IconIdent}},
		HasIconColor: true,
		{{- end}}
		{{- if .HasTopEdgeHighlight}}
		TopEdgeHighlight: {{.TopEdgeHighlight}},
		{{- end}}
		{{- if .HasInteraction}}
		Interaction: Interaction{
			{{- if .Interaction.Hover.NonZero}}
			Hover: InteractionDelta{FillDelta: {{.Interaction.Hover.FillDelta}}, BorderDelta: {{.Interaction.Hover.BorderDelta}}, HighlightDelta: {{.Interaction.Hover.HighlightDelta}}},
			{{- end}}
			{{- if .Interaction.Press.NonZero}}
			Press: InteractionDelta{FillDelta: {{.Interaction.Press.FillDelta}}, BorderDelta: {{.Interaction.Press.BorderDelta}}, HighlightDelta: {{.Interaction.Press.HighlightDelta}}},
			{{- end}}
			{{- if .Interaction.Focus.NonZero}}
			Focus: InteractionDelta{FillDelta: {{.Interaction.Focus.FillDelta}}, BorderDelta: {{.Interaction.Focus.BorderDelta}}, HighlightDelta: {{.Interaction.Focus.HighlightDelta}}},
			{{- end}}
		},
		{{- end}}
		{{- if .DynamicAnchor}}
		DynamicAnchor: {{printf "%q" .DynamicAnchor}},
		{{- end}}
		{{- if .AnimationAnchor}}
		AnimationAnchor: {{printf "%q" .AnimationAnchor}},
		{{- end}}
	},
{{- end}}
	}
}

// Spec returns the immutable ComponentSpec for the given ID. The returned
// value must not be mutated; copy if a one-off variant is needed.
func Spec(id ComponentID) ComponentSpec {
	if id < 0 || id >= componentCount {
		return ComponentSpec{}
	}
	return componentSpecs[id]
}
`
