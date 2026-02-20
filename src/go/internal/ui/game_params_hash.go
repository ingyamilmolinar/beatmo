package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// paramsHash computes a lightweight hash of node parameters to detect edits.
func (g *Game) paramsHash() uint64 {
	// NOTE: This runs every Update to detect edits. It must be stable across map
	// iteration order (Go maps do not guarantee iteration order stability).
	const (
		fnvOffset uint64 = 1469598103934665603
		fnvPrime  uint64 = 1099511628211
		mix       uint64 = 0x9e3779b97f4a7c15
	)
	var xor uint64
	var sum uint64
	for id, n := range g.graph.Nodes {
		// Skip invisible nodes.
		if n.Type == model.NodeTypeInvisible {
			continue
		}
		nh := fnvOffset
		nh ^= uint64(uint32(id))
		nh *= fnvPrime
		nh ^= uint64(uint32(n.Type)) + mix
		nh *= fnvPrime
		// Mix main fields.
		nh ^= mathFloat64Bits(n.Params.Volume) + mix
		nh *= fnvPrime
		nh ^= mathFloat64Bits(n.Params.Pitch) + mix
		nh *= fnvPrime
		nh ^= mathFloat64Bits(n.Params.Duration) + mix
		nh *= fnvPrime
		for _, s := range []string{n.Params.LogicKind, n.Params.GrooveKind} {
			for i := 0; i < len(s); i++ {
				nh ^= uint64(s[i]) + mix
				nh *= fnvPrime
			}
		}
		nh ^= uint64(uint32(n.Params.LogicN)) + mix
		nh *= fnvPrime
		nh ^= mathFloat64Bits(n.Params.LogicP) + mix
		nh *= fnvPrime
		// Groove fields.
		nh ^= mathFloat64Bits(n.Params.GroovePct) + mix
		nh *= fnvPrime

		// Combine commutatively so map iteration order can't change the output.
		xor ^= nh
		sum += nh*mix + fnvPrime
	}
	return xor ^ (sum * fnvPrime)
}

// mathFloat64Bits inlined to avoid importing math/bits here.
func mathFloat64Bits(f float64) uint64 { return math.Float64bits(f) }
