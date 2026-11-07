package ui

// DESIGN.md names a dynamic or animation anchor; the app Binds a function to it. An unbound lookup panics on
// purpose, so a YAML anchor without a runtime closure fails loud; TestAnchorsAllRegistered checks every spec.

import (
	"image/color"
	"sync"
)

// For module-global behaviour; per-instance closures (one row's swatch) live on ColorSwatchStyle.
type DynamicColorFn func() color.Color

// Progress 0..1; the YAML target (e.g. icon-color.alpha) is applied by the call site.
type AnimationFn func() float64

// Reads every frame, writes at startup or on a configuration change.
var specRegistry = struct {
	mu   sync.RWMutex
	dyn  map[string]DynamicColorFn
	anim map[string]AnimationFn
}{
	dyn:  map[string]DynamicColorFn{},
	anim: map[string]AnimationFn{},
}

// A rebind replaces the entry: the supported way to swap implementations.
func BindDynamic(anchor string, fn DynamicColorFn) {
	specRegistry.mu.Lock()
	defer specRegistry.mu.Unlock()
	specRegistry.dyn[anchor] = fn
}

func BindAnimation(anchor string, fn AnimationFn) {
	specRegistry.mu.Lock()
	defer specRegistry.mu.Unlock()
	specRegistry.anim[anchor] = fn
}

// No panic here, so the common no-anchor path allocates nothing; MustLookupDynamic panics.
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

// Test-only.
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
