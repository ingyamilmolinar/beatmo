//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Keyed by scope too: synth and sampler edit the same instrument with different index spaces.
func TestArmGrowSubjectIsScoped(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum

	dv.setArmGrowSubject(armedScopeSynth, roster.Kick, 4, 1)

	if idx, ok := dv.armGrowControl(armedScopeSynth, roster.Kick); !ok || idx != 4 {
		t.Fatalf("synth scope should see its own subject: idx=%d ok=%v", idx, ok)
	}
	if _, ok := dv.armGrowControl(armedScopeSampler, roster.Kick); ok {
		t.Fatal("sampler scope must NOT see the synth's grow subject for the same instrument")
	}

	dv.setArmGrowSubject(armedScopeSampler, roster.Kick, 1, 1)
	if idx, ok := dv.armGrowControl(armedScopeSampler, roster.Kick); !ok || idx != 1 {
		t.Fatalf("sampler scope should see its own subject: idx=%d ok=%v", idx, ok)
	}
	if _, ok := dv.armGrowControl(armedScopeSynth, roster.Kick); ok {
		t.Fatal("synth scope must NOT see the sampler's grow subject after the switch")
	}
}

// A new scope restarts from rest, or the new subject snaps in half-grown.
func TestArmGrowScopeSwitchRestartsProgress(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum

	dv.setArmGrowSubject(armedScopeSynth, roster.Kick, 2, 1)
	dv.armGrowT = 0.8
	dv.setArmGrowSubject(armedScopeSampler, roster.Kick, 2, 1)

	if dv.armGrowT != 0 {
		t.Fatalf("switching scope must restart progress from rest, got armGrowT=%v", dv.armGrowT)
	}
}
