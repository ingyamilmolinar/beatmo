//go:build test

package ui

import (
	"image"
	"testing"

	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

func TestScopePanelZoneLayout(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	if z.ID() != "scope-panel" {
		t.Errorf("expected ID 'scope-panel', got %q", z.ID())
	}
	r := image.Rect(0, 0, 800, 160)
	z.Layout(r)
	for i, btn := range z.stageButtons {
		if btn == nil {
			t.Errorf("stage button %d is nil", i)
			continue
		}
		if btn.Rect().Empty() {
			t.Errorf("stage button %d has empty rect", i)
		}
	}
	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Error("expected non-empty hit areas")
	}
}

func TestScopeTapSelection(t *testing.T) {
	assertDefaultParityState(t)
	z := NewScopePanelZone(ScopeCallbacks{})
	// Initially no taps.
	if z.tapA >= 0 || z.tapB >= 0 {
		t.Error("expected no taps initially")
	}
	// Click Synth -> A.
	z.handleStageClick(scope.StageSynth)
	if z.tapA != scope.StageSynth {
		t.Errorf("expected tapA=StageSynth, got %d", z.tapA)
	}
	// Click EQ -> B.
	z.handleStageClick(scope.StageEQ)
	if z.tapB != scope.StageEQ {
		t.Errorf("expected tapB=StageEQ, got %d", z.tapB)
	}
	// Click Synth again -> clear A.
	z.handleStageClick(scope.StageSynth)
	if z.tapA >= 0 {
		t.Error("expected tapA cleared")
	}
}
