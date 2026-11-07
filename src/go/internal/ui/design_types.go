package ui

import "image/color"

// Hand-written: the API the runtime consumes specs through; the generator emits values of these types.
// Per-component variation goes in ComponentSpec, per-draw in ComponentState.

// Closed: a new category needs the YAML validator and this enum updated together.
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

// Alpha 0 means no border (fully transparent), so call sites can short-circuit.
type BorderRef struct {
	BaseColor color.RGBA
	Alpha     uint8
}

// Resolve returns the runtime border color (base + bucket alpha).
func (b BorderRef) Resolve() color.NRGBA {
	return color.NRGBA{R: b.BaseColor.R, G: b.BaseColor.G, B: b.BaseColor.B, A: b.Alpha}
}

// Signed, bounded at ±127; 0 = no change. HighlightDelta is reserved for the press-state top edge.
type InteractionDelta struct {
	FillDelta      int8
	BorderDelta    int8
	HighlightDelta int8
}

// IsZero reports whether the delta has no effect.
func (d InteractionDelta) IsZero() bool {
	return d.FillDelta == 0 && d.BorderDelta == 0 && d.HighlightDelta == 0
}

// Press is applied by drawButton; hover and focus at the call site via Render().
type Interaction struct {
	Hover InteractionDelta
	Press InteractionDelta
	Focus InteractionDelta
}

// Immutable: copy to modify, or better, add a variant to the YAML.
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

	// Keyed by variant name ("mobile", "active", "disabled").
	Variants map[string]ComponentSpec

	// For components bound to a closure or an animation; an anchor named in YAML but not registered at runtime
	// is a generator error.
	DynamicAnchor   string
	AnimationAnchor string
}

// Typed lookups: a misspelled component is a compile error.
type ComponentID int

type ComponentState struct {
	Hovered  bool
	Pressed  bool
	Focused  bool
	Active   bool
	Disabled bool
}
