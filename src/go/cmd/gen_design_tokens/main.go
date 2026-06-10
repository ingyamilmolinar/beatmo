// Command gen_design_tokens generates Go source for the UI design system
// from DESIGN.md's YAML front matter. DESIGN.md is the source of truth;
// this generator is the bridge to runtime Go code.
//
// Phase 0 scope: primitives only — colors, spacing, rounded, alpha. The
// generator emits internal/ui/design_tokens.gen.go containing one
// genXxx variable/constant per token. Phase 2 will add components, Phase 5
// will add profile overrides.
//
// Validation strategy: a JSON Schema (scripts/gen_design_tokens/schema.json)
// runs first on the parsed YAML — it catches missing required fields,
// value-range violations, format/pattern errors, and unknown keys at the
// schema level (with file path, JSON-pointer location, and rule name in
// the error message). Then yaml.v3 strict decode (KnownFields(true))
// re-validates that the typed Go shape can absorb the data without
// surprises. The two layers complement each other: schema catches
// authoring mistakes; KnownFields catches typed-decoder gaps.
//
// Usage:
//
//	go run ./cmd/gen_design_tokens -design DESIGN.md -out internal/ui/design_tokens.gen.go
//
// Wired by internal/ui/generate_design.go via //go:generate. Run via
// `make gen-design-tokens` from the repo root.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// supportedSchemaVersion is the integer-string the YAML front matter must
// carry. Bumping it (e.g. "1" → "2") is how a breaking schema change is
// announced; old generators reject the new file with a clear error and
// new generators reject old files. The original "alpha" string was a
// pre-versioning placeholder and predates this contract.
const supportedSchemaVersion = "1"

var frontMatterRE = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)

// designSpec mirrors the YAML front matter of DESIGN.md. Phase 0 cares only
// about colors, spacing, rounded, alpha. The other top-level keys are
// declared so KnownFields(true) doesn't reject them; their contents are
// ignored until later phases.
type designSpec struct {
	Version                   string    `yaml:"version"`
	Name                      string    `yaml:"name"`
	Description               string    `yaml:"description"`
	Colors                    yaml.Node `yaml:"colors"`
	Typography                yaml.Node `yaml:"typography"`
	Spacing                   yaml.Node `yaml:"spacing"`
	Rounded                   yaml.Node `yaml:"rounded"`
	Icon                      yaml.Node `yaml:"icon"`
	Alpha                     yaml.Node `yaml:"alpha"`
	Components                yaml.Node `yaml:"components"`
	ProfileOverrides          yaml.Node `yaml:"profileOverrides"`
	Densities                 yaml.Node `yaml:"densities"`
	InstrumentSwatches        yaml.Node `yaml:"instrumentSwatches"`
	InstrumentDefaults        yaml.Node `yaml:"instrumentDefaults"`
	InstrumentFallbackPalette yaml.Node `yaml:"instrumentFallbackPalette"`
	Animations                yaml.Node `yaml:"animations"`
	Geometry                  yaml.Node `yaml:"geometry"`
}

// keyVal preserves source order from the YAML mapping (kept for stable
// output diffs that mirror DESIGN.md ordering).
type keyVal struct {
	Key, Val string
}

func main() {
	designPath := flag.String("design", "DESIGN.md", "path to DESIGN.md (relative to working dir)")
	schemaPath := flag.String("schema", "", "path to schema.json (defaults to scripts/gen_design_tokens/schema.json near DESIGN.md)")
	outPath := flag.String("out", "src/go/internal/ui/design_tokens.gen.go", "output for primitive tokens")
	outComponentsPath := flag.String("out-components", "src/go/internal/ui/design_components.gen.go", "output for component specs")
	outProfilePath := flag.String("out-profile", "src/go/internal/ui/design_profile.gen.go", "output for profile overrides")
	outDensityPath := flag.String("out-density", "src/go/internal/ui/design_density.gen.go", "output for density values")
	flag.Parse()

	resolved, err := resolveDesignPath(*designPath)
	if err != nil {
		fail("locate DESIGN.md: %v", err)
	}
	body, err := os.ReadFile(resolved)
	if err != nil {
		fail("read %s: %v", resolved, err)
	}

	// Schema validation runs FIRST: malformed YAML never reaches the
	// typed unmarshal path. Errors here carry JSON-pointer locations
	// like "(root).components.button-secondary.iconColor" so the
	// authoring mistake is found at the offending field, not buried
	// inside a downstream Go decode failure.
	resolvedSchema, err := resolveSchemaPath(*schemaPath, resolved)
	if err != nil {
		fail("locate schema.json: %v", err)
	}
	if err := validateAgainstSchema(body, resolvedSchema); err != nil {
		fail("schema validation failed:\n%v", err)
	}

	spec, err := parseFrontMatter(body)
	if err != nil {
		fail("parse %s: %v", resolved, err)
	}
	if spec.Version != supportedSchemaVersion {
		fail("DESIGN.md version=%q but generator supports %q (bump the generator before changing the version)",
			spec.Version, supportedSchemaVersion)
	}

	colors, err := parseScalarNode(&spec.Colors, "colors")
	if err != nil {
		fail("%v", err)
	}
	spacing, err := parseScalarNode(&spec.Spacing, "spacing")
	if err != nil {
		fail("%v", err)
	}
	rounded, err := parseScalarNode(&spec.Rounded, "rounded")
	if err != nil {
		fail("%v", err)
	}
	icon, err := parseScalarNode(&spec.Icon, "icon")
	if err != nil {
		fail("%v", err)
	}
	alpha, err := parseScalarNode(&spec.Alpha, "alpha")
	if err != nil {
		fail("%v", err)
	}

	// Validate alpha bucket count is small and known. Phase 2 will require
	// every alpha reference in components to be one of these buckets.
	for _, kv := range alpha {
		if _, err := strconv.Atoi(kv.Val); err != nil {
			fail("alpha.%s = %q: not an integer", kv.Key, kv.Val)
		}
	}

	swatches, err := parseInstrumentSwatches(&spec.InstrumentSwatches)
	if err != nil {
		fail("instrumentSwatches: %v", err)
	}

	defaults, err := parseInstrumentDefaults(&spec.InstrumentDefaults)
	if err != nil {
		fail("instrumentDefaults: %v", err)
	}

	fallbacks, err := parseInstrumentFallbackPalette(&spec.InstrumentFallbackPalette)
	if err != nil {
		fail("instrumentFallbackPalette: %v", err)
	}

	animations, err := parseAnimations(&spec.Animations)
	if err != nil {
		fail("animations: %v", err)
	}

	geometry, err := parseGeometry(&spec.Geometry)
	if err != nil {
		fail("geometry: %v", err)
	}

	out, err := emitTokens(colors, spacing, rounded, icon, alpha, swatches, defaults, fallbacks, animations, geometry)
	if err != nil {
		fail("emit tokens: %v", err)
	}
	if err := writeIfChanged(*outPath, out); err != nil {
		fail("write %s: %v", *outPath, err)
	}

	// Phase 2: parse and emit components. Build a token resolver from
	// already-parsed primitives so component fields like "{colors.X}" can
	// be substituted with concrete RGBs / pixel values / alpha buckets.
	resolver := newTokenResolver(colors, spacing, rounded, alpha)
	components, err := parseComponents(&spec.Components, resolver)
	if err != nil {
		fail("components: %v", err)
	}
	compsOut, err := emitComponents(components)
	if err != nil {
		fail("emit components: %v", err)
	}
	if err := writeIfChanged(*outComponentsPath, compsOut); err != nil {
		fail("write %s: %v", *outComponentsPath, err)
	}

	// Phase 5a: profile overrides → design_profile.gen.go.
	profiles, err := parseProfileOverrides(&spec.ProfileOverrides)
	if err != nil {
		fail("profileOverrides: %v", err)
	}
	profileOut, err := emitProfiles(profiles)
	if err != nil {
		fail("emit profiles: %v", err)
	}
	if err := writeIfChanged(*outProfilePath, profileOut); err != nil {
		fail("write %s: %v", *outProfilePath, err)
	}

	// Audio-panel density tier → design_density.gen.go. Orthogonal to
	// profile overrides — see densities.go for the design rationale.
	densities, err := parseDensities(&spec.Densities)
	if err != nil {
		fail("densities: %v", err)
	}
	if len(densities) > 0 {
		densityOut, err := emitDensities(densities)
		if err != nil {
			fail("emit densities: %v", err)
		}
		if err := writeIfChanged(*outDensityPath, densityOut); err != nil {
			fail("write %s: %v", *outDensityPath, err)
		}
	}
}

// resolveDesignPath finds DESIGN.md by walking up from the current working
// directory. Lets the generator be invoked from anywhere in the repo.
func resolveDesignPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return p, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, p)
		if _, err := os.Stat(try); err == nil {
			return try, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not found walking up from %s", cwd)
}

// resolveSchemaPath locates schema.json and returns an absolute path.
// If the caller passes a non-empty hint we use that directly (resolving
// to absolute if needed). Otherwise we walk up from the resolved
// DESIGN.md path looking for scripts/gen_design_tokens/schema.json —
// this lets the generator be invoked from any working directory
// (Makefile target, //go:generate, ad-hoc) without -schema flag
// plumbing. Returning absolute is important: the gojsonschema
// reference loader treats `file://` URIs literally and gets confused
// by relative-looking paths.
func resolveSchemaPath(hint, designPath string) (string, error) {
	resolveAbs := func(p string) (string, error) {
		if filepath.IsAbs(p) {
			return p, nil
		}
		return filepath.Abs(p)
	}
	if hint != "" {
		abs, err := resolveAbs(hint)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("schema hint %q not found (resolved=%s): %w", hint, abs, err)
		}
		return abs, nil
	}
	startAbs, err := resolveAbs(designPath)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(startAbs)
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, "scripts", "gen_design_tokens", "schema.json")
		if _, err := os.Stat(try); err == nil {
			return try, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("schema.json not found near %s (looked for scripts/gen_design_tokens/schema.json)", startAbs)
}

// validateAgainstSchema parses the YAML front matter of body, converts
// it to JSON, and validates against the JSON Schema at schemaPath.
// Errors are reported with JSON-pointer field locations and the failing
// schema rule, not just a generic "decode failed."
//
// We use yaml.v3 → map[string]interface{} → encoding/json round-trip
// rather than gojsonschema's YAML loader (which doesn't exist) or a
// dedicated YAML-to-JSON Schema validator (which would add another
// dep). Round-trip cost is single-digit milliseconds for DESIGN.md;
// negligible vs the rest of the generator's work.
func validateAgainstSchema(body []byte, schemaPath string) error {
	frontMatch := frontMatterRE.FindSubmatch(body)
	if frontMatch == nil {
		return fmt.Errorf("no YAML front matter (--- delimited block at top of DESIGN.md)")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(frontMatch[1], &doc); err != nil {
		return fmt.Errorf("YAML parse: %w", err)
	}
	doc = normalizeForJSON(doc).(map[string]any)
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("YAML→JSON: %w", err)
	}

	abs, err := filepath.Abs(schemaPath)
	if err != nil {
		return fmt.Errorf("absolute schema path: %w", err)
	}
	schemaURI := "file://" + abs
	schemaLoader := gojsonschema.NewReferenceLoader(schemaURI)
	docLoader := gojsonschema.NewBytesLoader(jsonBytes)
	result, err := gojsonschema.Validate(schemaLoader, docLoader)
	if err != nil {
		return fmt.Errorf("schema load: %w", err)
	}
	if result.Valid() {
		return nil
	}
	var sb strings.Builder
	for _, e := range result.Errors() {
		fmt.Fprintf(&sb, "  %s: %s\n", e.Field(), e.Description())
	}
	return fmt.Errorf("DESIGN.md violates schema (%s):\n%s", schemaPath, sb.String())
}

// normalizeForJSON converts yaml.v3's map[interface{}]interface{}
// values into JSON-compatible map[string]interface{}. yaml.v3 actually
// emits map[string]interface{} at the top level when keys are strings,
// but nested mappings can still produce non-string keys depending on
// document shape. This walks the tree once and rewrites them.
func normalizeForJSON(in any) any {
	switch v := in.(type) {
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			out[fmt.Sprint(k)] = normalizeForJSON(vv)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, vv := range v {
			out[k] = normalizeForJSON(vv)
		}
		return out
	case []any:
		for i := range v {
			v[i] = normalizeForJSON(v[i])
		}
		return v
	default:
		return v
	}
}

func parseFrontMatter(body []byte) (*designSpec, error) {
	m := frontMatterRE.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("no YAML front matter (--- delimited block at top)")
	}
	dec := yaml.NewDecoder(bytes.NewReader(m[1]))
	dec.KnownFields(true)
	var spec designSpec
	if err := dec.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode YAML: %w", err)
	}
	return &spec, nil
}

// parseScalarNode walks a YAML mapping node, accepting only string-scalar
// children. Preserves source order so generated diffs mirror DESIGN.md.
func parseScalarNode(n *yaml.Node, label string) ([]keyVal, error) {
	if n == nil || n.Kind == 0 {
		return nil, fmt.Errorf("DESIGN.md %s: missing", label)
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("DESIGN.md %s: not a mapping (got kind %d)", label, n.Kind)
	}
	out := make([]keyVal, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := n.Content[i]
		v := n.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("DESIGN.md %s: non-scalar key at line %d", label, k.Line)
		}
		// For Phase 0 we only consume scalar leaves. Mapping leaves
		// (typography roles, component recipes) are ignored here and
		// surfaced in later phases.
		if v.Kind != yaml.ScalarNode {
			continue
		}
		out = append(out, keyVal{Key: k.Value, Val: v.Value})
	}
	// Stable ordering: preserve YAML source order (already true above).
	// Provide a deterministic fallback in case yaml.v3 ever reorders.
	if !sort.SliceIsSorted(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	}) {
		// keep source order
	}
	return out, nil
}

// colorOut and intOut are the template-binding shapes for emitted symbols.
type colorOut struct {
	Ident   string
	R, G, B uint8
	SrcKey  string
	SrcHex  string
}

type intOut struct {
	Ident  string
	Value  int
	SrcKey string
}

// floatOut emits a float32 constant. Used for icon.stroke which is a
// fractional logical-unit weight (e.g. 1.75) — pixel sizes round to int,
// but stroke must preserve the fractional value or the per-render
// proportional calc loses precision at small icon sizes.
type floatOut struct {
	Ident  string
	Value  float64
	SrcKey string
}

// swatchOut backs each entry in `instrumentSwatches:`. The generator
// emits one per-swatch color.RGBA constant plus a slice literal that
// preserves source order.
type swatchOut struct {
	Ident  string
	Name   string
	R, G, B uint8
	SrcHex string
}

// parseInstrumentSwatches parses the optional `instrumentSwatches:` block.
// Returns nil if the block is absent (the YAML decoder leaves the Node in
// its zero state). Each entry is a {name, hex} pair; both fields are
// required. Schema-level validation already enforces the kebab-case
// name pattern and #RRGGBB[AA] hex pattern, but we re-validate the hex
// here to populate the parsed RGB values for the generator.
func parseInstrumentSwatches(node *yaml.Node) ([]swatchOut, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("instrumentSwatches: expected sequence, got kind=%d", node.Kind)
	}
	out := make([]swatchOut, 0, len(node.Content))
	for i, entry := range node.Content {
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("instrumentSwatches[%d]: expected mapping", i)
		}
		var name, hex string
		for j := 0; j < len(entry.Content); j += 2 {
			k, v := entry.Content[j].Value, entry.Content[j+1].Value
			switch k {
			case "name":
				name = v
			case "hex":
				hex = v
			default:
				return nil, fmt.Errorf("instrumentSwatches[%d]: unknown key %q", i, k)
			}
		}
		if name == "" || hex == "" {
			return nil, fmt.Errorf("instrumentSwatches[%d]: name and hex are required", i)
		}
		r, g, b, err := parseHexRGB(hex)
		if err != nil {
			return nil, fmt.Errorf("instrumentSwatches[%d] (%s) hex %q: %w", i, name, hex, err)
		}
		out = append(out, swatchOut{
			Ident:  identForSwatch(name),
			Name:   name,
			R:      r,
			G:      g,
			B:      b,
			SrcHex: hex,
		})
	}
	return out, nil
}

func identForSwatch(name string) string {
	return "genInstrumentSwatch" + pascal(name)
}

// geometryOut backs each entry in `geometry:`. Generic float64 — the
// generator emits one float64 constant per entry. If a future entry
// needs a different type (int pixel size, uint8, etc.), introduce
// per-key suffix conventions or a kind discriminator.
type geometryOut struct {
	Ident  string
	SrcKey string
	Value  float64
}

// parseGeometry parses the optional `geometry:` block. Each entry is a
// scalar number; schema enforces non-negative and the kebab-case key
// pattern. Source order is preserved so generated output diffs mirror
// DESIGN.md.
func parseGeometry(node *yaml.Node) ([]geometryOut, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("geometry: expected mapping, got kind=%d", node.Kind)
	}
	out := make([]geometryOut, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("geometry.%s: expected scalar number", k.Value)
		}
		f, err := strconv.ParseFloat(v.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("geometry.%s = %q: %w", k.Value, v.Value, err)
		}
		out = append(out, geometryOut{
			Ident:  "genGeom" + pascal(k.Value),
			SrcKey: k.Value,
			Value:  f,
		})
	}
	return out, nil
}

// filterAnims partitions animationOuts by kind so the template can emit
// each into its own typed-struct block.
func filterAnims(in []animationOut, kind string) []animationOut {
	out := make([]animationOut, 0, len(in))
	for _, a := range in {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out
}

// instrumentDefaultOut backs each entry in `instrumentDefaults:`. Same
// shape as swatchOut but the Name carries the *instrument id* (matched
// against import.go's instrument-name lookup) — distinct from the
// curated swatch names which are just human aliases.
type instrumentDefaultOut struct {
	Ident   string
	ID      string
	R, G, B uint8
	SrcHex  string
}

// parseInstrumentDefaults parses the optional `instrumentDefaults:` block.
// Returns nil when absent. Each entry is `{id, color}` — the runtime
// resolves a row's instrument id against this map, falling back to the
// fallback palette if missing.
func parseInstrumentDefaults(node *yaml.Node) ([]instrumentDefaultOut, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("instrumentDefaults: expected sequence, got kind=%d", node.Kind)
	}
	out := make([]instrumentDefaultOut, 0, len(node.Content))
	for i, entry := range node.Content {
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("instrumentDefaults[%d]: expected mapping", i)
		}
		var id, hex string
		for j := 0; j < len(entry.Content); j += 2 {
			k, v := entry.Content[j].Value, entry.Content[j+1].Value
			switch k {
			case "id":
				id = v
			case "color":
				hex = v
			default:
				return nil, fmt.Errorf("instrumentDefaults[%d]: unknown key %q", i, k)
			}
		}
		if id == "" || hex == "" {
			return nil, fmt.Errorf("instrumentDefaults[%d]: id and color are required", i)
		}
		r, g, b, err := parseHexRGB(hex)
		if err != nil {
			return nil, fmt.Errorf("instrumentDefaults[%d] (%s) color %q: %w", i, id, hex, err)
		}
		out = append(out, instrumentDefaultOut{
			Ident:  "genInstrumentDefault" + pascal(id),
			ID:     id,
			R:      r,
			G:      g,
			B:      b,
			SrcHex: hex,
		})
	}
	return out, nil
}

// instrumentFallbackOut backs each entry in `instrumentFallbackPalette:`.
type instrumentFallbackOut struct {
	Ident   string
	Index   int
	R, G, B uint8
	SrcHex  string
}

// animationOut backs each entry in `animations:`. Three kinds of
// animation share one Go struct; emitter routes per-kind fields into
// per-kind generated symbols.
type animationOut struct {
	Ident      string // identifier: "genAnimNodeTriggerGlow" etc.
	Name       string // YAML key (kebab-case)
	Kind       string // "exp-decay" | "sin-pulse" | "fade"
	Rate       float64
	Threshold  float64
	FrameStep  float64
	Amplitude  float64
	Base       float64
	AlphaScale int
	Factor     float64
}

// parseAnimations parses the optional `animations:` block. Each entry
// has a `kind` discriminator and per-kind required fields:
//   - exp-decay  → rate, threshold
//   - sin-pulse  → frame-step, amplitude, base, [alpha-scale]
//   - fade       → factor
// Schema-level validation already covers field ranges; this parser
// enforces per-kind required fields and rejects unknown kinds.
func parseAnimations(node *yaml.Node) ([]animationOut, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("animations: expected mapping, got kind=%d", node.Kind)
	}
	out := make([]animationOut, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if v.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("animations.%s: expected mapping", k.Value)
		}
		entry := animationOut{
			Ident: "genAnim" + pascal(k.Value),
			Name:  k.Value,
		}
		seen := map[string]bool{}
		for j := 0; j+1 < len(v.Content); j += 2 {
			fk := v.Content[j].Value
			fv := v.Content[j+1].Value
			seen[fk] = true
			switch fk {
			case "kind":
				entry.Kind = fv
			case "rate":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.rate = %q: %w", k.Value, fv, err)
				}
				entry.Rate = f
			case "threshold":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.threshold = %q: %w", k.Value, fv, err)
				}
				entry.Threshold = f
			case "frame-step":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.frame-step = %q: %w", k.Value, fv, err)
				}
				entry.FrameStep = f
			case "amplitude":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.amplitude = %q: %w", k.Value, fv, err)
				}
				entry.Amplitude = f
			case "base":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.base = %q: %w", k.Value, fv, err)
				}
				entry.Base = f
			case "alpha-scale":
				n, err := strconv.Atoi(fv)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.alpha-scale = %q: %w", k.Value, fv, err)
				}
				entry.AlphaScale = n
			case "factor":
				f, err := strconv.ParseFloat(fv, 64)
				if err != nil {
					return nil, fmt.Errorf("animations.%s.factor = %q: %w", k.Value, fv, err)
				}
				entry.Factor = f
			default:
				return nil, fmt.Errorf("animations.%s: unknown field %q", k.Value, fk)
			}
		}
		// Per-kind required fields.
		switch entry.Kind {
		case "exp-decay":
			if !seen["rate"] || !seen["threshold"] {
				return nil, fmt.Errorf("animations.%s (exp-decay): rate and threshold are required", k.Value)
			}
		case "sin-pulse":
			if !seen["frame-step"] || !seen["amplitude"] || !seen["base"] {
				return nil, fmt.Errorf("animations.%s (sin-pulse): frame-step, amplitude, base are required", k.Value)
			}
		case "fade":
			if !seen["factor"] {
				return nil, fmt.Errorf("animations.%s (fade): factor is required", k.Value)
			}
		case "":
			return nil, fmt.Errorf("animations.%s: kind is required", k.Value)
		default:
			return nil, fmt.Errorf("animations.%s: unknown kind %q (allowed: exp-decay, sin-pulse, fade)", k.Value, entry.Kind)
		}
		out = append(out, entry)
	}
	return out, nil
}

// parseInstrumentFallbackPalette parses the optional
// `instrumentFallbackPalette:` block. Each entry is `{color}` (no
// human label — the index IS the identity).
func parseInstrumentFallbackPalette(node *yaml.Node) ([]instrumentFallbackOut, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("instrumentFallbackPalette: expected sequence, got kind=%d", node.Kind)
	}
	out := make([]instrumentFallbackOut, 0, len(node.Content))
	for i, entry := range node.Content {
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("instrumentFallbackPalette[%d]: expected mapping", i)
		}
		var hex string
		for j := 0; j < len(entry.Content); j += 2 {
			k, v := entry.Content[j].Value, entry.Content[j+1].Value
			if k != "color" {
				return nil, fmt.Errorf("instrumentFallbackPalette[%d]: unknown key %q", i, k)
			}
			hex = v
		}
		if hex == "" {
			return nil, fmt.Errorf("instrumentFallbackPalette[%d]: color is required", i)
		}
		r, g, b, err := parseHexRGB(hex)
		if err != nil {
			return nil, fmt.Errorf("instrumentFallbackPalette[%d] color %q: %w", i, hex, err)
		}
		out = append(out, instrumentFallbackOut{
			Ident:  fmt.Sprintf("genInstrumentFallback%d", i),
			Index:  i,
			R:      r,
			G:      g,
			B:      b,
			SrcHex: hex,
		})
	}
	return out, nil
}

// emitTokens renders the primitives .gen.go output (colors, spacing,
// rounded, icon, alpha, instrumentSwatches). Component emission lives in
// emitComponents (components.go).
func emitTokens(colors, spacing, rounded, icon, alpha []keyVal, swatches []swatchOut, defaults []instrumentDefaultOut, fallbacks []instrumentFallbackOut, animations []animationOut, geometry []geometryOut) ([]byte, error) {
	colorOuts := make([]colorOut, 0, len(colors))
	for _, kv := range colors {
		r, g, b, err := parseHexRGB(kv.Val)
		if err != nil {
			return nil, fmt.Errorf("colors.%s = %q: %w", kv.Key, kv.Val, err)
		}
		colorOuts = append(colorOuts, colorOut{
			Ident:  identForColor(kv.Key),
			R:      r,
			G:      g,
			B:      b,
			SrcKey: kv.Key,
			SrcHex: kv.Val,
		})
	}

	spacingOuts, err := pxOuts(spacing, identForSpacing)
	if err != nil {
		return nil, fmt.Errorf("spacing: %w", err)
	}
	roundedOuts, err := pxOuts(rounded, identForRounded)
	if err != nil {
		return nil, fmt.Errorf("rounded: %w", err)
	}

	iconInts, iconFloats, err := iconOuts(icon)
	if err != nil {
		return nil, fmt.Errorf("icon: %w", err)
	}

	alphaOuts := make([]intOut, 0, len(alpha))
	for _, kv := range alpha {
		v, err := strconv.Atoi(kv.Val)
		if err != nil {
			return nil, fmt.Errorf("alpha.%s = %q: %w", kv.Key, kv.Val, err)
		}
		if v < 0 || v > 255 {
			return nil, fmt.Errorf("alpha.%s = %d: outside [0,255]", kv.Key, v)
		}
		alphaOuts = append(alphaOuts, intOut{
			Ident:  identForAlpha(kv.Key),
			Value:  v,
			SrcKey: kv.Key,
		})
	}

	tpl := template.Must(template.New("gen").Funcs(template.FuncMap{
		"intToUint8": func(i int) string { return strconv.Itoa(i) },
	}).Parse(genTemplate))

	var buf bytes.Buffer
	expDecay := filterAnims(animations, "exp-decay")
	sinPulse := filterAnims(animations, "sin-pulse")
	fade := filterAnims(animations, "fade")
	data := struct {
		Colors             []colorOut
		Spacing            []intOut
		Rounded            []intOut
		IconInts           []intOut
		IconFloats         []floatOut
		Alpha              []intOut
		Swatches           []swatchOut
		InstrumentDefaults []instrumentDefaultOut
		InstrumentFallback []instrumentFallbackOut
		AnimExpDecay       []animationOut
		AnimSinPulse       []animationOut
		AnimFade           []animationOut
		HasAnimations      bool
		Geometry           []geometryOut
	}{colorOuts, spacingOuts, roundedOuts, iconInts, iconFloats, alphaOuts, swatches, defaults, fallbacks, expDecay, sinPulse, fade, len(animations) > 0, geometry}
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt: %w\n--- raw output ---\n%s", err, buf.String())
	}
	return formatted, nil
}

func pxOuts(items []keyVal, ident func(string) string) ([]intOut, error) {
	out := make([]intOut, 0, len(items))
	for _, kv := range items {
		v, err := parsePx(kv.Val)
		if err != nil {
			return nil, fmt.Errorf("%s = %q: %w", kv.Key, kv.Val, err)
		}
		out = append(out, intOut{Ident: ident(kv.Key), Value: v, SrcKey: kv.Key})
	}
	return out, nil
}

// iconOuts splits the icon: block into integer keys (grid/padding/radius)
// and the fractional `stroke` key. The schema enforces presence of all
// four, so the generator simply routes each known key to the right
// output bucket. Unknown keys become an error so a typo (e.g.
// `radius_default: 2`) is caught at generation time.
func iconOuts(items []keyVal) ([]intOut, []floatOut, error) {
	ints := make([]intOut, 0, 3)
	floats := make([]floatOut, 0, 1)
	for _, kv := range items {
		switch kv.Key {
		case "grid", "padding", "radius":
			v, err := strconv.Atoi(strings.TrimSpace(kv.Val))
			if err != nil {
				return nil, nil, fmt.Errorf("icon.%s = %q: %w", kv.Key, kv.Val, err)
			}
			ints = append(ints, intOut{Ident: identForIcon(kv.Key), Value: v, SrcKey: kv.Key})
		case "stroke":
			f, err := strconv.ParseFloat(strings.TrimSpace(kv.Val), 64)
			if err != nil {
				return nil, nil, fmt.Errorf("icon.%s = %q: %w", kv.Key, kv.Val, err)
			}
			floats = append(floats, floatOut{Ident: identForIcon(kv.Key), Value: f, SrcKey: kv.Key})
		default:
			return nil, nil, fmt.Errorf("icon.%s: unknown key (allowed: grid, padding, radius, stroke)", kv.Key)
		}
	}
	return ints, floats, nil
}

func parseHexRGB(raw string) (r, g, b uint8, err error) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("expected #RRGGBB, got %q", raw)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, err
	}
	return uint8(v >> 16 & 0xFF), uint8(v >> 8 & 0xFF), uint8(v & 0xFF), nil
}

func parsePx(raw string) (int, error) {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "px"))
	return strconv.Atoi(s)
}

// Identifier conversion: kebab-case → PascalCase with a "gen" prefix.
// e.g. "primary-bright" → "genColorPrimaryBright". The prefix flags the
// symbol as generated so future readers immediately know not to hand-edit it.

func identForColor(key string) string   { return "genColor" + pascal(key) }
func identForSpacing(key string) string { return "genSpacing" + pascal(key) }
func identForRounded(key string) string { return "genRounded" + pascal(key) }
func identForIcon(key string) string    { return "genIcon" + pascal(key) }
func identForAlpha(key string) string   { return "genAlpha" + pascal(key) }

func pascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

// writeIfChanged writes content to path only if it differs from the
// existing file. Keeps mtime stable when the generator is a no-op (which
// matters for build cache invalidation).
func writeIfChanged(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen_design_tokens: "+format+"\n", args...)
	os.Exit(1)
}

const genTemplate = `// Code generated by cmd/gen_design_tokens. DO NOT EDIT.
//
// Source of truth: DESIGN.md (YAML front matter).
// Regenerate via: make gen-design-tokens

package ui

import "image/color"

// ── Colors ─────────────────────────────────────────────────────────────────
//
// One variable per entry under DESIGN.md ` + "`colors:`" + `. Alpha is always
// 255 here (Stitch only allows opaque #RRGGBB); semi-transparent uses
// compose these via WithAlpha() + a named bucket below.
var (
{{- range .Colors}}
	{{.Ident}} = color.RGBA{ {{.R}}, {{.G}}, {{.B}}, 255 } // colors.{{.SrcKey}} = {{.SrcHex}}
{{- end}}
)

// ── Spacing ───────────────────────────────────────────────────────────────
const (
{{- range .Spacing}}
	{{.Ident}} = {{.Value}} // spacing.{{.SrcKey}}
{{- end}}
)

// ── Rounded (corner radii) ────────────────────────────────────────────────
const (
{{- range .Rounded}}
	{{.Ident}} = {{.Value}} // rounded.{{.SrcKey}}
{{- end}}
)

// ── Icon system (logical 24-unit grid) ────────────────────────────────────
//
// Every IconID renders onto a square logical canvas of ` + "`genIconGrid`" + ` units.
// Stroke is fractional — converted to pixel width per icon size at render
// time as ` + "`genIconStrokeWeight * (renderSize / genIconGrid)`" + `.
const (
{{- range .IconInts}}
	{{.Ident}} = {{.Value}} // icon.{{.SrcKey}}
{{- end}}
)

// genIconStrokeWeight is float32 because the stroke is a fractional
// logical-unit weight (Lucide-style 1.75). Pixel sizes round to int but
// preserving the fraction here matters for proportional stroke at small
// icon sizes.
{{- range .IconFloats}}
const {{.Ident}} float32 = {{printf "%g" .Value}} // icon.{{.SrcKey}}
{{- end}}

// ── Alpha buckets ─────────────────────────────────────────────────────────
//
// Pair with WithAlpha(token, bucket) — never inline a raw 0-255 alpha for
// chrome.
const (
{{- range .Alpha}}
	{{.Ident}} uint8 = {{.Value}} // alpha.{{.SrcKey}}
{{- end}}
)
{{if .Swatches}}
// ── Instrument swatches (curated suggestion palette) ──────────────────────
//
// Per-row color picker reads these as suggested swatches above the free
// hue wheel. Beatmo-only extension; the chrome runtime ignores this set.
// Source order from DESIGN.md is preserved.
var (
{{- range .Swatches}}
	{{.Ident}} = color.RGBA{ {{.R}}, {{.G}}, {{.B}}, 255 } // instrumentSwatches.{{.Name}} = {{.SrcHex}}
{{- end}}
)

// GenInstrumentSwatch is the public shape of an entry in
// genInstrumentSwatches; the type is exported so the consumer
// (instrument_palette.go) can range over the slice without import gymnastics.
type GenInstrumentSwatch struct {
	Name string
	RGBA color.RGBA
	Hex  string
}

var genInstrumentSwatches = []GenInstrumentSwatch{
{{- range .Swatches}}
	{Name: "{{.Name}}", RGBA: {{.Ident}}, Hex: "{{.SrcHex}}"},
{{- end}}
}
{{end}}
{{if .InstrumentDefaults}}
// ── Instrument default colors (per built-in id) ───────────────────────────
//
// Consumed by the chrome runtime (theme.go's instColors map) — each
// row whose instrument id appears here inherits this hex as its
// drum-cell tint, edge tint, and rack swatch unless the user picks
// a custom color. Source order from DESIGN.md is preserved.
var (
{{- range .InstrumentDefaults}}
	{{.Ident}} = color.RGBA{ {{.R}}, {{.G}}, {{.B}}, 255 } // instrumentDefaults.{{.ID}} = {{.SrcHex}}
{{- end}}
)

var genInstrumentDefaults = map[string]color.RGBA{
{{- range .InstrumentDefaults}}
	"{{.ID}}": {{.Ident}},
{{- end}}
}
{{end}}
{{if .InstrumentFallback}}
// ── Instrument fallback palette (cycled for unknown ids) ──────────────────
//
// When an instrument id is not present in genInstrumentDefaults, the
// runtime cycles this slice. Source order matters; do not sort.
var (
{{- range .InstrumentFallback}}
	{{.Ident}} = color.RGBA{ {{.R}}, {{.G}}, {{.B}}, 255 } // instrumentFallbackPalette[{{.Index}}] = {{.SrcHex}}
{{- end}}
)

var genInstrumentFallbackPalette = []color.RGBA{
{{- range .InstrumentFallback}}
	{{.Ident}},
{{- end}}
}
{{end}}
{{if .HasAnimations}}
// ── Animations ────────────────────────────────────────────────────────────
//
// Three closed kinds emitted as three typed structs. Runtime helpers in
// animation_helpers.go consume these (DecayStep, SinPulse, FadeFactor).
// Editing DESIGN.md's animations: block and re-running the generator is
// the only way to change cadence — never inline magic numbers in draw
// call sites.

// ExpDecayAnim drives a per-frame exponential decay loop: v *= rate
// each tick; exit when v < threshold.
type ExpDecayAnim struct {
	Rate      float64
	Threshold float64
}

// SinPulseAnim drives a per-frame oscillator: out = base + amplitude *
// |sin(frame * frame-step)|. AlphaScale (uint8) is the optional
// 0..255 ceiling when the output drives an alpha channel.
type SinPulseAnim struct {
	FrameStep  float64
	Amplitude  float64
	Base       float64
	AlphaScale uint8
}

// FadeFactor is a 0..1 multiplier applied via fadeColor() at the call
// site. One typed alias per role keeps the call site readable.
type FadeFactor float64

var (
{{- range .AnimExpDecay}}
	{{.Ident}} = ExpDecayAnim{Rate: {{.Rate}}, Threshold: {{.Threshold}}} // animations.{{.Name}}
{{- end}}
{{- range .AnimSinPulse}}
	{{.Ident}} = SinPulseAnim{FrameStep: {{.FrameStep}}, Amplitude: {{.Amplitude}}, Base: {{.Base}}, AlphaScale: {{.AlphaScale}}} // animations.{{.Name}}
{{- end}}
{{- range .AnimFade}}
	{{.Ident}} FadeFactor = {{.Factor}} // animations.{{.Name}}
{{- end}}
)
{{end}}
{{if .Geometry}}
// ── Geometry ──────────────────────────────────────────────────────────────
//
// Theme-level visual sizes / multipliers / fractions. Profile-dependent
// sizes (nodeMinPx / nodeMaxPx / edgeThickMul) live in profileOverrides.
const (
{{- range .Geometry}}
	{{.Ident}} = {{.Value}} // geometry.{{.SrcKey}}
{{- end}}
)
{{end}}`
