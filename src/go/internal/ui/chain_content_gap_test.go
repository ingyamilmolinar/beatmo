//go:build test

package ui

import "testing"

// TestChainContentGapToken pins the chain stage-column → trace content gap to a
// density token (was a hardcoded +8) so the Chain tab layout is re-styleable.
func TestChainContentGapToken(t *testing.T) {
	if got := Profile().DensityValues().ChainContentGap; got <= 0 {
		t.Fatalf("ChainContentGap density token unset (got %d)", got)
	}
}
