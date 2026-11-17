package ui

import "image/color"

// design_types.go — Runtime types for the generated design system. These
// are hand-written (not generated) because they are the API the runtime
// uses to consume specs. The generator (cmd/gen_design_tokens) emits
// values OF these types in design_components.gen.go.
//
// Keep this file small. Anything that varies per-component goes in
// ComponentSpec; anything that varies per draw call goes in
// ComponentState. Helpers that compute runtime colors from a spec live in
// render.go (Phase 4).

// ComponentCategory groups specs for documentation, sanity checks, and
// future per-category styling rules. The set is closed; adding a new
// category requires updating both the YAML schema validator and this enum.
type ComponentCategory string

const (
	CategoryButton     ComponentCategory = "button"
	CategoryInput      ComponentCategory = "input"
	CategoryPanel      ComponentCategory = "panel"
	CategoryMenu       ComponentCategory = "menu"
	CategoryContainer  ComponentCategory = "container"
	CategoryViz        ComponentCategory = "viz"
	CategoryWidget     ComponentCategory = "widget"
	CategoryDecoration ComponentCategory = "decoration"
)

// BorderRef pairs a base color with a named alpha bucket. Resolve() yields
// the runtime NRGBA composite. Alpha == 0 means "no border" (the resolver
// returns a fully-transparent color, and call sites can short-circuit).
type BorderRef struct {
	BaseColor color.RGBA
	Alpha     uint8
}

// Resolve returns the runtime border color (base + bucket alpha).
func (b BorderRef) Resolve() color.NRGBA {
	return color.NRGBA{R: b.BaseColor.R, G: b.BaseColor.G, B: b.BaseColor.B, A: b.Alpha}
}

// InteractionDelta is a per-state set of brightness offsets applied at
// draw time. Values are signed and bounded at ±127; 0 means "no change".
//
// FillDelta affects the body color; BorderDelta affects the border color
// (or the focus accent for inputs); HighlightDelta is reserved for the
// 1-px top-edge highlight on press states. All three default to 0.
//
// Per-component variation is preserved verbatim from the pre-refactor
// values: ButtonStyle hover {12, 20}, ColorSwatch hover {20, 0},
// TextInput focus {30, 80}, DrumCell active top-strip {30, 0}, etc.
type InteractionDelta struct {
	FillDelta      int8
	BorderDelta    int8
	HighlightDelta int8
}

// IsZero reports whether the delta has no effect.
func (d InteractionDelta) IsZero() bool {
	return d.FillDelta == 0 && d.BorderDelta == 0 && d.HighlightDelta == 0
}

// Interaction bundles the three runtime states a chrome control reacts
// to. Press is applied by drawButton; Hover and Focus are applied at the
// call site via Render() (Phase 4).
type Interaction struct {
	Hover InteractionDelta
	Press InteractionDelta
	Focus InteractionDelta
}

// ComponentSpec is the full visual recipe for a single chrome surface,
// generated from a DESIGN.md `components:` entry. Specs are immutable
// values; never mutate one in place. Construct a copy if you need a
// modified version (rare — prefer adding a variant to the YAML).
type ComponentSpec struct {
	ID       ComponentID
	Category ComponentCategory

	Fill         color.RGBA
	Border       BorderRef
	Radius       int
	Height       int
	IconColor    color.RGBA
	HasIconColor bool // false → leave the call-site IconColor unchanged

	TopEdgeHighlight bool

	Interaction Interaction

	// Variants carry full token rebindings keyed by string ("mobile",
	// "active", "disabled", etc.). Phase 2 PR1 leaves this nil; it is
	// populated by later PRs as variants are migrated. Resolution helpers
	// will live in render.go.
	Variants map[string]ComponentSpec

	// DynamicAnchor / AnimationAnchor are non-empty for components whose
	// runtime appearance is bound to a closure (ColorSwatchStyle.Color)
	// or an animation (record-pulse alpha). The runtime resolves them via
	// SpecRegistry (Phase 2 PR2). An anchor named in the YAML but not
	// registered at runtime is a generator error.
	DynamicAnchor   string
	AnimationAnchor string
}

// ComponentID is a generated enum: one constant per DESIGN.md component
// entry. Lookups via Spec(ID) are typed; misspellings are compile errors.
// The enum constants are emitted into design_components.gen.go.
type ComponentID int

// ComponentState is the per-frame variable input to Render(). It is
// hand-constructed at each call site from current input/UI state.
type ComponentState struct {
	Hovered  bool
	Pressed  bool
	Focused  bool
	Active   bool
	Disabled bool
}
