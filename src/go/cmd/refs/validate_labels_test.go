//go:build !js

package main

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The label says so on the row itself, not only in a legend nobody reads.
func TestAxisFlagAppendsDemoted(t *testing.T) {
	s := axisScore{AUC: 0.95, SE: 0.01, Comparable: true, FlagDemoted: true}
	got := axisFlag(s)
	if got != "DEMOTED" {
		t.Fatalf("axisFlag(demoted) = %q, want exactly \"DEMOTED\" -- every word in the AUC "+
			"vocabulary makes a trust claim, and demotion says none of them applies", got)
	}
	if strings.Contains(axisFlag(axisScore{AUC: 0.95, SE: 0.01, Comparable: true}), "DEMOTED") {
		t.Fatal("a non-demoted axis must not be labelled DEMOTED")
	}
}

// An axis flagging more than half the families the LISTENER accepted calls a defect where a human heard none.
func TestAxisDemotesItselfOnTheGoodGroup(t *testing.T) {
	cases := []struct {
		name      string
		fires     float64
		nMeasured int
		want      bool
	}{
		{"fires on every good family", 1.0, 8, true},
		{"fires on 5 of 8", 0.625, 8, true},
		{"fires on exactly half", 0.5, 8, false},
		{"fires on 2 of 8", 0.25, 8, false},
		{"never measured on the good group", 1.0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := axisScore{AUC: 0.9, SE: 0.01, Comparable: true,
				GoodFiresRate: c.fires, NGoodMeasured: c.nMeasured}
			if got := s.Demoted(); got != c.want {
				t.Fatalf("Demoted()=%v want %v (good-fires %.2f over n=%d)",
					got, c.want, c.fires, c.nMeasured)
			}
			if c.want && axisFlag(s) != "DEMOTED" {
				t.Fatalf("axisFlag=%q, want DEMOTED", axisFlag(s))
			}
			if c.want && s.DemotionReason() != "fires on the good group" {
				t.Fatalf("DemotionReason=%q", s.DemotionReason())
			}
		})
	}
}

// TestScoreAxesComputesGoodFiresRate drives the real aggregation: Norm >= 1
// means at/past mismatch, and only GOOD-graded families count.
func TestScoreAxesComputesGoodFiresRate(t *testing.T) {
	mk := func(class verdictClass, norm float64) validationSample {
		return validationSample{Class: class, Diagnosis: instrumentDiagnosis{
			Axes: []axisResult{{ID: "x.y", Name: "x", Group: "g", Norm: norm, Verdict: verdictMismatch}},
		}}
	}
	rows := scoreAxes([]validationSample{
		mk(classGood, 2.0), mk(classGood, 1.0), mk(classGood, 0.2), mk(classGood, 0.1),
		mk(classBad, 5.0), mk(classMid, 5.0),
	})
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	r := rows[0]
	if r.NGoodMeasured != 4 {
		t.Fatalf("NGoodMeasured=%d, want 4 (mid and bad families must not count)", r.NGoodMeasured)
	}
	if math.Abs(r.GoodFiresRate-0.5) > 1e-9 {
		t.Fatalf("GoodFiresRate=%g, want 0.5 (Norm>=1 on 2 of 4 good families)", r.GoodFiresRate)
	}
	if r.Demoted() {
		t.Error("exactly half is not MORE than half; the rule must not demote here")
	}
}

func TestAxisFlagAppendsSaturated(t *testing.T) {
	s := axisScore{AUC: 0.5, SE: 0.1, Comparable: true, MismatchRate: 0.99}
	if !strings.Contains(axisFlag(s), "SATURATED") {
		t.Fatalf("axisFlag(saturated) = %q", axisFlag(s))
	}
}

// A blank cell would read as missing data, not "chosen by a person".
func TestCalLabelReportsHandWhenUncalibrated(t *testing.T) {
	if got := calLabel("no.such.descriptor"); got != "hand" {
		t.Fatalf("calLabel(unknown) = %q, want hand", got)
	}
}

// TestCalLabelReportsFileSource joins the registry to the table: whatever
// source the embedded threshold file records is what the column prints.
func TestCalLabelReportsFileSource(t *testing.T) {
	f := fingerprint.LoadThresholdFile()
	checked := 0
	for id, e := range f {
		if e.Mismatch <= 0 {
			continue
		}
		if got := calLabel(id); got != e.Source {
			t.Errorf("calLabel(%q) = %q, want %q", id, got, e.Source)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no calibrated descriptors in the embedded threshold file yet")
	}
	t.Logf("checked %d calibrated descriptors", checked)
}

// TestScoreAxesCarriesDescriptorIdentity: the CAL and DEMOTED columns are
// keyed by descriptor ID, so scoreAxes must carry it through rather than
// re-deriving it from the human-facing axis name.
func TestScoreAxesCarriesDescriptorIdentity(t *testing.T) {
	d, ok := fingerprint.DefaultRegistry().ByID(fingerprint.IDEnvRingT60)
	if !ok {
		t.Skip("ring T60 descriptor missing")
	}
	samples := []validationSample{{
		Class: classGood,
		Diagnosis: instrumentDiagnosis{Axes: []axisResult{
			{ID: d.ID, Name: d.Name, Group: d.Group, Norm: 0.1},
		}},
	}, {
		Class: classBad,
		Diagnosis: instrumentDiagnosis{Axes: []axisResult{
			{ID: d.ID, Name: d.Name, Group: d.Group, Norm: 0.9},
		}},
	}}
	rows := scoreAxes(samples)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].ID != d.ID {
		t.Fatalf("row ID = %q, want %q", rows[0].ID, d.ID)
	}
	if rows[0].FlagDemoted != d.Demoted {
		t.Fatalf("row FlagDemoted = %v, want %v", rows[0].FlagDemoted, d.Demoted)
	}
}
