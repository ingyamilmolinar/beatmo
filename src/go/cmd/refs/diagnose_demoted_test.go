//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/synthdiag"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// demotedFakeDiagnosis is a synthetic two-axis diagnosis: one ordinary
// MISMATCH axis and one DEMOTED MISMATCH axis. Every assertion below is about
// the difference in how those two rows are treated.
func demotedFakeDiagnosis() instrumentDiagnosis {
	return instrumentDiagnosis{
		Instrument: roster.TubularBells,
		RefID:      "bells-ref",
		Axes: []axisResult{
			{ID: "a.good", Group: "envelope", Name: "trusted axis",
				Verdict: verdictMismatch, Direction: "high"},
			{ID: "a.demoted", Group: "envelope", Name: "demoted axis",
				Verdict: verdictMismatch, Direction: "low", Demoted: true},
		},
	}
}

// A demoted axis is still MEASURED and still PRINTED -- suppressing it would
// throw away a reading that is often the most informative thing on the page
// once you know why it was demoted. What must change is that the reader can
// see the demotion without carrying the list in their head.
func TestPrintDiagnosis_TagsDemotedAxes(t *testing.T) {
	var buf bytes.Buffer
	printDiagnosis(&buf, demotedFakeDiagnosis())
	out := buf.String()

	if !strings.Contains(out, "demoted axis") {
		t.Fatalf("a demoted axis must still be printed:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "demoted axis"):
			if !strings.Contains(line, "[demoted]") {
				t.Errorf("demoted row lacks the [demoted] tag:\n%s", line)
			}
		case strings.Contains(line, "trusted axis"):
			if strings.Contains(line, "[demoted]") {
				t.Errorf("trusted row wrongly tagged demoted:\n%s", line)
			}
		}
	}
}

// formatVerdict is the one place the tag is produced, so its degenerate case
// (a not-comparable axis, whose verdict string is empty) is pinned here.
func TestFormatVerdict_Demoted(t *testing.T) {
	cases := []struct {
		a    axisResult
		want string
	}{
		{axisResult{Verdict: verdictMismatch}, string(verdictMismatch)},
		{axisResult{Verdict: verdictMismatch, Demoted: true}, string(verdictMismatch) + " [demoted]"},
		{axisResult{Verdict: "", Demoted: true}, "[demoted]"},
		{axisResult{Verdict: ""}, ""},
	}
	for _, c := range cases {
		if got := formatVerdict(c.a); got != c.want {
			t.Errorf("formatVerdict(%+v) = %q, want %q", c.a, got, c.want)
		}
	}
}

// The stage-sensitivity list is a ROUTING table. Routing someone to a knob on
// the strength of an axis calibration showed to mislead is the exact failure
// the demotion flag exists to prevent, so the demoted MISMATCH must not appear.
func TestPrintAblation_SkipsDemotedAxes(t *testing.T) {
	rep := synthdiag.Report{
		Instrument: roster.TubularBells,
		Stages:     []string{"resonator"},
		Axes:       []string{"a.good", "a.demoted"},
		Names:      []string{"trusted axis", "demoted axis"},
		Sens:       [][]float64{{1.5, 9.9}},
	}
	var buf bytes.Buffer
	printAblation(&buf, demotedFakeDiagnosis(), rep)
	out := buf.String()

	if !strings.Contains(out, "trusted axis") {
		t.Fatalf("printAblation dropped the trusted MISMATCH axis:\n%s", out)
	}
	if strings.Contains(out, "demoted axis") {
		t.Errorf("printAblation routed a DEMOTED axis:\n%s", out)
	}
}

// The -all matrix exists to expose cross-instrument patterns. A demoted axis
// would paint a full column of MISMATCHes -- which reads as the strongest
// pattern on the page -- so it gets no column at all.
func TestPrintSummaryMatrix_SkipsDemotedAxes(t *testing.T) {
	var buf bytes.Buffer
	printSummaryMatrix(&buf, []instrumentDiagnosis{demotedFakeDiagnosis()})
	out := buf.String()

	if !strings.Contains(out, "trusted axis") {
		t.Fatalf("summary matrix dropped the trusted MISMATCH axis:\n%s", out)
	}
	if strings.Contains(out, "demoted axis") {
		t.Errorf("summary matrix gave a DEMOTED axis a column:\n%s", out)
	}
}

// An instrument whose ONLY MISMATCH is on a demoted axis has nothing to
// report in the matrix: it must not be listed as a mismatching instrument on
// the strength of a row that is excluded from ranking.
func TestPrintSummaryMatrix_DemotedOnlyInstrumentNotListed(t *testing.T) {
	d := instrumentDiagnosis{
		Instrument: roster.Harp,
		Axes: []axisResult{
			{ID: "a.demoted", Name: "demoted axis", Verdict: verdictMismatch, Demoted: true},
		},
	}
	var buf bytes.Buffer
	printSummaryMatrix(&buf, []instrumentDiagnosis{d})
	if out := buf.String(); strings.Contains(out, "harp") {
		t.Errorf("instrument with only a demoted MISMATCH was listed:\n%s", out)
	}
}

// The -1 sentinel must NEVER reach the report as a value. band.t60 marks an
// unfittable band with -1 so the vector stays aligned with the band labels;
// printing Values[0] put "-1.000 s" in the REFERENCE column of an axis whose
// verdict came from the mean over the bands that WERE fittable.
func TestPrintDiagnosis_VectorRowsPrintAHeadlineNotComponentZero(t *testing.T) {
	d := instrumentDiagnosis{
		Instrument: roster.TubularBells,
		Axes: []axisResult{
			{ID: "band.t60", Group: "resolved decay", Name: "band T60", Kind: axisSeconds,
				Ref: -1, Synth: -1, RefText: "(vector: 6/10 bands measurable)",
				SynthText: "(vector: 7/10 bands measurable)",
				Delta:     "mean 2.43x (worst 4k 6.52x)", Verdict: verdictMismatch},
			{ID: "scalar.axis", Group: "resolved decay", Name: "plain axis", Kind: axisSeconds,
				Ref: 1.25, Synth: 2.5, Verdict: verdictOK},
		},
	}
	var buf bytes.Buffer
	printDiagnosis(&buf, d)
	out := buf.String()

	if strings.Contains(out, "-1.000 s") {
		t.Errorf("the unfittable-band sentinel printed as a duration:\n%s", out)
	}
	if !strings.Contains(out, "6/10 bands measurable") || !strings.Contains(out, "7/10 bands measurable") {
		t.Errorf("vector row lost its headline:\n%s", out)
	}
	// The comparator's own display is what carries the compared quantity, and
	// it must not have a second copy of the magnitude appended.
	if !strings.Contains(out, "mean 2.43x (worst 4k 6.52x)") {
		t.Errorf("vector row lost its comparator display:\n%s", out)
	}
	// A scalar row is untouched.
	if !strings.Contains(out, "1.250 s") || !strings.Contains(out, "2.500 s") {
		t.Errorf("scalar row stopped printing its numbers:\n%s", out)
	}
}

// A side that could not be measured still prints n/a: the headline describes
// a measurement that exists, and must never mask an unavailability.
func TestFormatValueText_NAWinsOverHeadline(t *testing.T) {
	a := axisResult{Kind: axisSeconds, RefNA: "no modes", RefText: "(vector: 3 components)"}
	if got := formatValueText(a, 0, a.RefNA, a.RefText); got != "n/a" {
		t.Errorf("formatValueText with an NA = %q, want n/a", got)
	}
}
