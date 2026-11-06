//go:build !test && !js

package audio

import "os"

// renderModularRoundParamsToFloat32 is a TEST hook for the config-only engine
// refactor (Phase A, docs/superpowers/plans/2026-10-02-config-only-synth-engine.md).
// When set, renderModularBlock rounds every param to float32 before handing the
// (now double) block to C — reproducing exactly what the float block delivered
// before the switch. With it on, every render must be BIT-IDENTICAL to the
// pre-switch baseline: that proves the type switch itself (layout, field order,
// every reader) is clean, separately from the intended effect of delivering the
// seed values exactly.
//
// BEATMO_MODULAR_PARAMS_F32=1 turns it on for a whole process — a causation
// probe for any test: if a failure disappears under it, the failure is caused
// only by seed values now reaching C exactly.
var renderModularRoundParamsToFloat32 = os.Getenv("BEATMO_MODULAR_PARAMS_F32") != ""

// roundModularParamsToFloat32 returns a copy of b with every element rounded
// to float32.
func roundModularParamsToFloat32(b modularBlock) modularBlock {
	out := make(modularBlock, len(b))
	for i, v := range b {
		out[i] = float64(float32(v))
	}
	return out
}
