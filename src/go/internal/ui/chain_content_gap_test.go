//go:build test

package ui

import "testing"

func TestChainContentGapToken(t *testing.T) {
	if got := Profile().DensityValues().ChainContentGap; got <= 0 {
		t.Fatalf("ChainContentGap density token unset (got %d)", got)
	}
}
