//go:build !test && !js

package synthdiag

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The violin's excess bass came from the body resonator's low modes. "Top 3", not "first": the low-pass just above the
// body's bridge hill ranks higher on a high/low ratio, and stage interaction does not support ranking the two.
func TestAblate_ViolinResonatorDrivesBrightness(t *testing.T) {
	rep, err := Ablate(roster.Violin, 12, 44100, 2.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := rep.TopStages(fingerprint.IDBrightnessRatio, len(rep.Stages))
	t.Logf("violin %s ranking (floor-subtracted): %+v", fingerprint.IDBrightnessRatio, all)

	top := rep.TopStages(fingerprint.IDBrightnessRatio, 3)
	if len(top) == 0 {
		t.Fatal("no stage sensitivity for brightness ratio")
	}
	found := false
	for _, s := range top {
		if s.Stage == "resonator" {
			found = true
		}
	}
	if !found {
		t.Fatalf("resonator must be in the top 3 for brightness ratio, got %+v (full ranking %+v)", top, all)
	}
}

// On the REAL registry: a seconds axis compared with itself reads 1/mismatch, so a stage that does not touch the
// release must read exactly 0 after the floor, never negative.
func TestAblate_ViolinReleaseSensitivityFlooredAtZero(t *testing.T) {
	rep, err := Ablate(roster.Violin, 12, 44100, 2.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	all := rep.TopStages(fingerprint.IDEnvRelease, len(rep.Stages))
	if len(all) == 0 {
		t.Fatalf("no stage sensitivity for %s (stages: %v)", fingerprint.IDEnvRelease, rep.Stages)
	}
	t.Logf("violin %s ranking (floor-subtracted): %+v", fingerprint.IDEnvRelease, all)

	sawZero := false
	for _, s := range all {
		if s.Norm < 0 {
			t.Errorf("stage %s: sensitivity must never be negative, got %v", s.Stage, s.Norm)
		}
		if s.Norm == 0 {
			sawZero = true
		}
	}
	if !sawZero {
		t.Errorf("at least one stage must read EXACTLY 0 on %s once the no-effect floor is subtracted; got %+v",
			fingerprint.IDEnvRelease, all)
	}
}
