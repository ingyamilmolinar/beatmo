//go:build !js

// Package main tests for synth-match CLI helpers.
package main

import (
	"testing"
)

func TestSeedVarName_HandlesEmptyParts(t *testing.T) {
	cases := map[string]string{
		"violin":          "violinSeed",
		"guitar-electric": "guitarElectricSeed",
		"fm-bell":         "fmBellSeed",
		"--sax":           "saxSeed",
		"foo--bar":        "fooBarSeed",
	}
	for in, want := range cases {
		if got := seedVarName(in); got != want {
			t.Errorf("seedVarName(%q)=%q, want %q", in, got, want)
		}
	}
}
