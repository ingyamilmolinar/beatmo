//go:build !js

package main

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/synthdiag"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The ablation addendum only earns its place if it routes the axes that need
// routing: exactly the MISMATCH rows, ranked, with "no stage moves it" said
// out loud rather than left as a missing line a reader would read as "not
// analysed".
func TestPrintAblation_ListsOnlyMismatchAxesRanked(t *testing.T) {
	d := instrumentDiagnosis{
		Instrument: roster.Violin,
		Axes: []axisResult{
			{ID: "a.ok", Name: "fine axis", Verdict: verdictOK},
			{ID: "a.drift", Name: "drifty axis", Verdict: verdictDrift},
			{ID: "a.bad", Name: "bad axis", Verdict: verdictMismatch},
			{ID: "a.orphan", Name: "orphan axis", Verdict: verdictMismatch},
			{ID: "", Name: "unkeyed axis", Verdict: verdictMismatch},
		},
	}
	rep := synthdiag.Report{
		Instrument: roster.Violin,
		Stages:     []string{"osc", "resonator", "lfo"},
		Axes:       []string{"a.bad"},
		Names:      []string{"bad axis"},
		Sens: [][]float64{
			{0.25},
			{3.5},
			{math.NaN()},
		},
	}
	var buf bytes.Buffer
	printAblation(&buf, d, rep)
	out := buf.String()

	for _, absent := range []string{"fine axis", "drifty axis", "unkeyed axis"} {
		if strings.Contains(out, absent) {
			t.Errorf("printAblation listed %q; only MISMATCH axes with an ID belong here:\n%s", absent, out)
		}
	}
	if !strings.Contains(out, "bad axis") {
		t.Fatalf("printAblation dropped the MISMATCH axis:\n%s", out)
	}
	// Ranked largest-first, and the NaN stage is dropped rather than ranked 0.
	if i, j := strings.Index(out, "resonator 3.50"), strings.Index(out, "osc 0.25"); i < 0 || j < 0 || i > j {
		t.Errorf("stages must be ranked by sensitivity, largest first:\n%s", out)
	}
	if strings.Contains(out, "lfo") {
		t.Errorf("a not-comparable (NaN) stage must be dropped, not ranked:\n%s", out)
	}
	if !strings.Contains(out, "orphan axis") || !strings.Contains(out, "engine-level") {
		t.Errorf("an axis no stage moves must be reported as engine-level, not omitted:\n%s", out)
	}
	if !strings.Contains(out, "does not sum") {
		t.Errorf("the header must say the matrix is not a decomposition:\n%s", out)
	}
	// The numbers are floor-subtracted (a KindSeconds axis reads 1/mismatch,
	// not 0, when compared with itself), so the header has to say what the
	// scale's origin IS or a reader will take a raw Norm reading.
	if !strings.Contains(out, "no-effect floor") {
		t.Errorf("the header must name the no-effect floor:\n%s", out)
	}
}

func TestPrintAblationMatrix_RendersNaNAsDash(t *testing.T) {
	rep := synthdiag.Report{
		Instrument: roster.Snare,
		Stages:     []string{"post", "fm"},
		Axes:       []string{"a.x", "a.y"},
		Names:      []string{"axis x", "axis y"},
		Sens: [][]float64{
			{1.5, math.NaN()},
			{0.0, 2.25},
		},
	}
	var buf bytes.Buffer
	printAblationMatrix(&buf, rep)
	out := buf.String()
	for _, want := range []string{string(roster.Snare), "post", "fm", "axis x", "axis y", "1.50", "2.25", "-", "no-effect floor"} {
		if !strings.Contains(out, want) {
			t.Errorf("matrix missing %q:\n%s", want, out)
		}
	}
}

func TestPrintAblationMatrix_NoActiveStages(t *testing.T) {
	var buf bytes.Buffer
	printAblationMatrix(&buf, synthdiag.Report{Instrument: roster.Kick})
	if !strings.Contains(buf.String(), "no active stages") {
		t.Errorf("an empty report must say so explicitly:\n%s", buf.String())
	}
}
