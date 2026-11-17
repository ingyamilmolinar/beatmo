//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestArmGrowSubjectIsScoped pins that the grow animation's single subject is
// keyed by SCOPE as well as instrument id. Without the scope, a synth arm on
// instrument X would drive the sampler's reflow for the same X (and vice
// versa) because both tabs edit the same instrument with different index
// spaces.
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

// TestArmGrowScopeSwitchRestartsProgress pins that changing scope restarts the
// eased progress from rest, exactly as changing instrument or index does — a
// retained progress value would make the new subject snap in half-grown.
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
