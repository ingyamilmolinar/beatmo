package ui

// spec_registry.go — Runtime side of the dynamic / animation anchor
// system. Components in DESIGN.md can declare:
//
//	dynamic:
//	  kind: callback
//	  anchor: ColorSwatchColorFn
//
//	animation:
//	  kind: pulse
//	  anchor: RecordPulseAnimator
//	  target: icon-color.alpha
//
// At codegen time the generator copies the anchor name into
// ComponentSpec.DynamicAnchor / ComponentSpec.AnimationAnchor.
//
// At runtime the application registers a function for each anchor name
// via Bind. Looking up an unregistered anchor panics — this is a
// load-bearing safety net: a YAML edit that references a nonexistent
// runtime closure must not silently render the wrong color, it must
// fail loud at first use so the gap is visible immediately.
//
// Why a string-keyed registry rather than the original plan's typed
// AnchorID enum: with the string key the YAML is the single source of
// truth (add an anchor, register it; no third place). The audit trail
// is a single `git grep '\(Bind\|GetDynamic\|GetAnimation\)("'` away.
// The trade-off is no compile-time check that every YAML anchor has a
// runtime registration — TestAnchorsAllRegistered closes that gap by
// scanning every ComponentSpec at startup and failing the test suite
// if any anchor is named in YAML but missing from the registry.

import (
	"image/color"
	"sync"
)

// DynamicColorFn returns a runtime color for a single component
// instance. Receiver-style closures are supported by binding a
// per-instance closure under the same anchor name; the registry
// stores the most recent registration and is intended for module-
// global behaviors (instrument-palette cycling, brand-color sweeps).
// Per-instance bindings (one row's color swatch closure) live on the
// ColorSwatchStyle struct; the anchor name on the spec only proves
// that the spec expects a closure binding.
type DynamicColorFn func() color.Color

// AnimationFn returns a normalized progress value (0..1) for animator
// anchors. The "target" field on the YAML — e.g.
// "icon-color.alpha" — is the application of the progress to the
// rendered output, owned by the call site.
type AnimationFn func() float64

// specRegistry owns the runtime closures for anchor-bound components.
// Module-global; concurrent reads are common (every render frame),
// writes happen at startup or on configuration change.
var specRegistry = struct {
	mu   sync.RWMutex
	dyn  map[string]DynamicColorFn
	anim map[string]AnimationFn
}{
	dyn:  map[string]DynamicColorFn{},
	anim: map[string]AnimationFn{},
}

// BindDynamic registers a runtime color closure for the given anchor
// name. Subsequent registrations replace the prior entry — the call
// site that owns the anchor decides who's authoritative; rebinding is
// the supported way to swap implementations (for tests, profile
// changes, etc.).
func BindDynamic(anchor string, fn DynamicColorFn) {
	specRegistry.mu.Lock()
	defer specRegistry.mu.Unlock()
	specRegistry.dyn[anchor] = fn
}

// BindAnimation registers a runtime animator for the given anchor
// name. Same rebinding semantics as BindDynamic.
func BindAnimation(anchor string, fn AnimationFn) {
	specRegistry.mu.Lock()
	defer specRegistry.mu.Unlock()
	specRegistry.anim[anchor] = fn
}

// LookupDynamic returns the bound closure for a dynamic anchor, or
// nil + false if no closure has been registered. Returning a sentinel
// rather than panicking on the lookup keeps the path allocation-free
// for the common case of "spec without a dynamic anchor"; the panic
// happens via MustLookupDynamic, used only by call sites that have
// already established the anchor is non-empty on the spec.
func LookupDynamic(anchor string) (DynamicColorFn, bool) {
	specRegistry.mu.RLock()
	defer specRegistry.mu.RUnlock()
	fn, ok := specRegistry.dyn[anchor]
	return fn, ok
}

// LookupAnimation is the animation counterpart of LookupDynamic.
func LookupAnimation(anchor string) (AnimationFn, bool) {
	specRegistry.mu.RLock()
	defer specRegistry.mu.RUnlock()
	fn, ok := specRegistry.anim[anchor]
	return fn, ok
}

// allBoundAnchors returns the set of currently-registered anchor
// names (across both dynamic and animation tables). Test-only — used
// by TestAnchorsAllRegistered to prove every YAML-declared anchor has
// a runtime binding.
func allBoundAnchors() map[string]bool {
	specRegistry.mu.RLock()
	defer specRegistry.mu.RUnlock()
	out := make(map[string]bool, len(specRegistry.dyn)+len(specRegistry.anim))
	for k := range specRegistry.dyn {
		out[k] = true
	}
	for k := range specRegistry.anim {
		out[k] = true
	}
	return out
}
