//go:build !test && !js

package synthdiag

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The violin "too much bass" bug was found by hand by toggling stages and
// measuring energy below 1 kHz; the body resonator's low modes were the
// culprit. The ablation matrix must rediscover that the resonator is a
// top-leverage stage for brightness ratio.
//
// The gate is "in the top 3", not "first". The measured order is filter first,
// resonator second: violinSeed pairs body_mix=1.0 with a low-pass at 3100 Hz
// sitting just above the body's ~2.5 kHz bridge hill, so opening the filter
// releases MORE high-frequency energy than removing the body does, on an axis
// that is a ratio of high-band to low-band energy. Both are real findings about
// the same tuning decision; ranking them against each other would be reading
// precision into a sensitivity number that stage interaction does not support.
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

// The floor subtraction has to hold on the REAL registry, not just on the
// hand-built descriptors the fast-path test uses: env.release is KindSeconds,
// whose raw self-comparison Norm is 1/mismatch, so before the subtraction
// every no-effect stage read a spurious ~0.50 there. After it, a stage that
// does not touch the release must read exactly 0 -- and nothing may go
// negative.
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
