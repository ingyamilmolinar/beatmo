//go:build !test && !js

package audio

import "os"

// A causation probe (BEATMO_MODULAR_PARAMS_F32=1): rounding every param to float32 before C reproduces what a float
// block delivered, so a failure that vanishes under it is caused only by values now reaching C exactly.
var renderModularRoundParamsToFloat32 = os.Getenv("BEATMO_MODULAR_PARAMS_F32") != ""

func roundModularParamsToFloat32(b modularBlock) modularBlock {
	out := make(modularBlock, len(b))
	for i, v := range b {
		out[i] = float64(float32(v))
	}
	return out
}
