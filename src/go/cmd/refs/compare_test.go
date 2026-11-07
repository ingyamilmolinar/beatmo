//go:build !js

package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func almostEqual(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestComputeGains_EqualisesTwoLevels(t *testing.T) {
	// The corpus's own spread cited in the task: oboe RMS 0.047, violin 0.094.
	realRMS, synthRMS := 0.047, 0.094
	rg, sg := computeGains(realRMS, synthRMS)
	if !almostEqual(realRMS*rg, synthRMS*sg, 1e-9) {
		t.Errorf("gains do not equalise: real %.6f*%.4f=%.6f vs synth %.6f*%.4f=%.6f",
			realRMS, rg, realRMS*rg, synthRMS, sg, synthRMS*sg)
	}
}

func TestComputeGains_SilentInputReturnsUnityGain(t *testing.T) {
	rg, sg := computeGains(0, 0.1)
	if rg != 1 || sg != 1 {
		t.Errorf("computeGains(0, 0.1) = (%v, %v), want (1, 1) -- can't compute a ratio against silence", rg, sg)
	}
}

func TestClampGainsForPeak_PreventsClipping(t *testing.T) {
	// 0.5 x 3 would clip: the WORST peak comes down to the ceiling, the gains' ratio intact.
	realGain, synthGain := 3.0, 1.5
	realPeak, synthPeak := 0.5, 0.6
	origRatio := realGain / synthGain

	rg, sg := clampGainsForPeak(realGain, synthGain, realPeak, synthPeak, 0.99)

	worst := math.Max(realPeak*rg, synthPeak*sg)
	if worst > 0.99+1e-9 {
		t.Errorf("clamped worst peak = %.4f, want <= 0.99", worst)
	}
	if !almostEqual(worst, 0.99, 1e-6) {
		t.Errorf("clamp should bring the worst peak exactly to the ceiling, got %.6f", worst)
	}
	if !almostEqual(rg/sg, origRatio, 1e-9) {
		t.Errorf("clamp changed the relative gain ratio: got %.6f, want %.6f", rg/sg, origRatio)
	}
}

func TestClampGainsForPeak_NoOpWhenUnderCeiling(t *testing.T) {
	rg, sg := clampGainsForPeak(1.2, 1.1, 0.3, 0.3, 0.99)
	if rg != 1.2 || sg != 1.1 {
		t.Errorf("clamp changed gains that were already under the ceiling: got (%.4f, %.4f)", rg, sg)
	}
}

func TestLevelMatchGains_RMSEqualisesRMS(t *testing.T) {
	rg, sg, err := levelMatchGains(matchRMS, 0.047, 0.108, 0.094, 0.2)
	if err != nil {
		t.Fatalf("levelMatchGains: %v", err)
	}
	if !almostEqual(0.047*rg, 0.094*sg, 1e-6) {
		t.Errorf("rms mode did not equalise RMS: %.6f vs %.6f", 0.047*rg, 0.094*sg)
	}
}

func TestLevelMatchGains_PeakEqualisesPeak(t *testing.T) {
	rg, sg, err := levelMatchGains(matchPeak, 0.047, 0.108, 0.094, 0.2)
	if err != nil {
		t.Fatalf("levelMatchGains: %v", err)
	}
	if !almostEqual(0.108*rg, 0.2*sg, 1e-6) {
		t.Errorf("peak mode did not equalise peaks: %.6f vs %.6f", 0.108*rg, 0.2*sg)
	}
}

func TestLevelMatchGains_NoneLeavesSamplesUntouched(t *testing.T) {
	rg, sg, err := levelMatchGains(matchNone, 0.047, 0.9, 0.094, 0.95)
	if err != nil {
		t.Fatalf("levelMatchGains: %v", err)
	}
	if rg != 1 || sg != 1 {
		t.Errorf("-match none must leave samples untouched, got gains (%.4f, %.4f)", rg, sg)
	}
}

func TestLevelMatchGains_UnknownModeErrors(t *testing.T) {
	if _, _, err := levelMatchGains("loudness", 0.1, 0.5, 0.1, 0.5); err == nil {
		t.Error("levelMatchGains accepted an unknown -match mode")
	}
}

func TestLevelMatchGains_ClipGuardCapsRMSMode(t *testing.T) {
	// Both clips are already loud; equalising their RMS at face value would
	// push peaks over 1.0. The guard must bring both down proportionally.
	rg, sg, err := levelMatchGains(matchRMS, 0.3, 0.95, 0.6, 0.9)
	if err != nil {
		t.Fatalf("levelMatchGains: %v", err)
	}
	if p := 0.95 * rg; p > 0.99+1e-9 {
		t.Errorf("real peak after gain = %.4f, want <= 0.99", p)
	}
	if p := 0.9 * sg; p > 0.99+1e-9 {
		t.Errorf("synth peak after gain = %.4f, want <= 0.99", p)
	}
}

func TestScaleSamples(t *testing.T) {
	got := scaleSamples([]float64{0.1, -0.2, 0.5}, 2.0)
	want := []float64{0.2, -0.4, 1.0}
	for i := range want {
		if !almostEqual(got[i], want[i], 1e-9) {
			t.Errorf("scaleSamples[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestIsNearSilent(t *testing.T) {
	if !isNearSilent(make([]float64, 1000)) {
		t.Error("an all-zero buffer must be reported near-silent")
	}
	if !isNearSilent(nil) {
		t.Error("an empty buffer must be reported near-silent")
	}
	loud := make([]float64, 1000)
	for i := range loud {
		loud[i] = 0.3
	}
	if isNearSilent(loud) {
		t.Error("a buffer with real signal must not be reported near-silent")
	}
}

func TestPickReference_ExplicitRefMustServeInstrument(t *testing.T) {
	candidates := []wavref.Ref{{ID: "violin-D5"}, {ID: "violin-freesound-alt"}}
	if _, err := pickReference(candidates, "trumpet-A4"); err == nil {
		t.Error("pickReference accepted a ref id that is not among the instrument's candidates")
	}
}

func TestPickReference_DefaultsToFirstWhenRefOmitted(t *testing.T) {
	candidates := []wavref.Ref{{ID: "violin-D5"}, {ID: "violin-freesound-alt"}}
	r, err := pickReference(candidates, "")
	if err != nil {
		t.Fatalf("pickReference: %v", err)
	}
	if r.ID != "violin-D5" {
		t.Errorf("pickReference chose %q, want the first candidate %q", r.ID, "violin-D5")
	}
}

func TestPickReference_HonoursExplicitRef(t *testing.T) {
	candidates := []wavref.Ref{{ID: "violin-D5"}, {ID: "violin-freesound-alt"}}
	r, err := pickReference(candidates, "violin-freesound-alt")
	if err != nil {
		t.Fatalf("pickReference: %v", err)
	}
	if r.ID != "violin-freesound-alt" {
		t.Errorf("pickReference = %q, want %q", r.ID, "violin-freesound-alt")
	}
}

func TestCompareDir_UnderEnvDirAndNamedByInstrument(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	d, err := compareDir(roster.Oboe)
	if err != nil {
		t.Fatalf("compareDir: %v", err)
	}
	if !strings.HasPrefix(d, tmp) {
		t.Errorf("compareDir = %q, want it under %q", d, tmp)
	}
	if filepath.Base(d) != "oboe" {
		t.Errorf("compareDir base = %q, want %q", filepath.Base(d), "oboe")
	}
	if filepath.Base(filepath.Dir(d)) != "compare" {
		t.Errorf("compareDir parent = %q, want %q", filepath.Base(filepath.Dir(d)), "compare")
	}
}

func TestRunCompare_UnknownInstrumentErrors(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var buf bytes.Buffer
	if err := runCompare(&buf, "no-such-instrument", "", 0, matchRMS); err == nil {
		t.Error("runCompare accepted an unknown instrument")
	}
}

func TestRunCompare_RefNotServingInstrumentErrors(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var buf bytes.Buffer
	// trumpet-A4 exists in the catalogue but is not a violin reference.
	if err := runCompare(&buf, roster.Violin, "trumpet-A4", 0, matchRMS); err == nil {
		t.Error("runCompare accepted a -ref that does not serve the instrument")
	}
}

func TestRunCompare_UnknownMatchModeErrors(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var buf bytes.Buffer
	if err := runCompare(&buf, roster.Oboe, "", 0, "loudness"); err == nil {
		t.Error("runCompare accepted an unknown -match mode")
	}
}

// The full pipeline with no network or recording: bassoon's one reference as a generated fixture, and the -tags test synth
// stub, whose content is not asserted.
func TestRunCompare_FullRun_WritesLevelMatchedFilesUnderEnvDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "bassoon-A2")

	var buf bytes.Buffer
	if err := runCompare(&buf, roster.Bassoon, "", 0, matchRMS); err != nil {
		t.Fatalf("runCompare: %v", err)
	}

	realPath := filepath.Join(tmp, "compare", "bassoon", "REAL__bassoon-A2.wav")
	synthPath := filepath.Join(tmp, "compare", "bassoon", "SYNTH__bassoon.wav")
	for _, p := range []string{realPath, synthPath} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected output file %s: %v", p, err)
		}
	}

	out := buf.String()
	for _, want := range []string{"REAL", "SYNTH", realPath, synthPath} {
		if !strings.Contains(out, want) {
			t.Errorf("runCompare output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestRunCompare_RefFlagSelectsNamedCandidate(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if refs := c.ForInstrument(roster.Violin); len(refs) < 2 {
		t.Skip("violin has only one candidate; nothing to select between")
	}

	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefsFor(t, tmp, roster.Violin)

	var buf bytes.Buffer
	if err := runCompare(&buf, roster.Violin, "violin-freesound-alt", 0, matchRMS); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	want := filepath.Join(tmp, "compare", "violin", "REAL__violin-freesound-alt.wav")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected the explicitly -ref'd candidate's file: %v", err)
	}
}

func TestRunCompare_MultipleCandidatesNotesTheChoice(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if refs := c.ForInstrument(roster.Violin); len(refs) < 2 {
		t.Skip("violin has only one candidate")
	}

	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefsFor(t, tmp, roster.Violin)
	var buf bytes.Buffer
	if err := runCompare(&buf, roster.Violin, "", 0, matchRMS); err != nil {
		t.Fatalf("runCompare: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "-ref") {
		t.Errorf("expected a note about -ref when multiple candidates exist, got:\n%s", out)
	}
}

// --- -sh (machine-readable output) ------------------------------------

func TestFormatCompareSh_EmitsFourQuotedKeys(t *testing.T) {
	out := formatCompareSh("/tmp/a b/REAL__oboe-A4.wav", "/tmp/a b/SYNTH__oboe.wav", "oboe-A4", 1.566326530612245)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := []string{
		`BEATMO_AB_REAL='/tmp/a b/REAL__oboe-A4.wav'`,
		`BEATMO_AB_SYNTH='/tmp/a b/SYNTH__oboe.wav'`,
		`BEATMO_AB_DUR='1.57'`,
		`BEATMO_AB_REF='oboe-A4'`,
	}
	if len(lines) != len(want) {
		t.Fatalf("formatCompareSh produced %d lines, want %d:\n%s", len(lines), len(want), out)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestRunCompareSh_PrintsOnlyTheFourKeysOnStdout(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "bassoon-A2")

	var out, diag bytes.Buffer
	if err := runCompareSh(&out, &diag, roster.Bassoon, "", 0, matchRMS); err != nil {
		t.Fatalf("runCompareSh: %v", err)
	}

	got := out.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("runCompareSh stdout has %d lines, want exactly 4 (no table, no prose):\n%s", len(lines), got)
	}
	wantPrefixes := []string{"BEATMO_AB_REAL='", "BEATMO_AB_SYNTH='", "BEATMO_AB_DUR='", "BEATMO_AB_REF='"}
	for i, p := range wantPrefixes {
		if !strings.HasPrefix(lines[i], p) {
			t.Errorf("stdout line %d = %q, want prefix %q", i, lines[i], p)
		}
	}
	if strings.Contains(got, "REAL\t") || strings.Contains(got, "SYNTH\t") {
		t.Errorf("runCompareSh stdout must not contain the human-readable table:\n%s", got)
	}

	realPath := filepath.Join(tmp, "compare", "bassoon", "REAL__bassoon-A2.wav")
	if _, err := os.Stat(realPath); err != nil {
		t.Errorf("runCompareSh must still write the REAL wav: %v", err)
	}
	synthPath := filepath.Join(tmp, "compare", "bassoon", "SYNTH__bassoon.wav")
	if _, err := os.Stat(synthPath); err != nil {
		t.Errorf("runCompareSh must still write the SYNTH wav: %v", err)
	}

	// The report must land on diag, not be dropped: -sh renders it there instead of running the pipeline twice.
	diagOut := diag.String()
	for _, want := range []string{"F0 (Hz)", "REAL", "SYNTH", "What to listen for"} {
		if !strings.Contains(diagOut, want) {
			t.Errorf("runCompareSh must write the human-readable report to diag (missing %q), got:\n%s", want, diagOut)
		}
	}
}

// --- -list (enumerate comparable instruments) --------------------------

func TestRunCompareList_IncludesKnownCoveredInstrument(t *testing.T) {
	var buf bytes.Buffer
	if err := runCompareList(&buf); err != nil {
		t.Fatalf("runCompareList: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "oboe") {
		t.Errorf("runCompareList output missing known-covered instrument %q:\n%s", "oboe", out)
	}
}

func TestRunCompareList_ExcludesStatusOnlyInstrument(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	// Only a status entry and no real audio: not comparable.
	for _, r := range c.ForInstrument(roster.Accordion) {
		if r.Status == "" {
			t.Skip("accordion now has a real reference; pick another status-only instrument for this test")
		}
	}

	var buf bytes.Buffer
	if err := runCompareList(&buf); err != nil {
		t.Fatalf("runCompareList: %v", err)
	}
	out := buf.String()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "accordion" {
			t.Errorf("runCompareList must not list status-only instrument %q, got line:\n%s", "accordion", line)
		}
	}
}

// A generated sine under each reference's catalogue filename, so a test drives the real pipeline with no recording.
func stageGeneratedRefs(t *testing.T, root string, ids ...string) {
	t.Helper()
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	for _, id := range ids {
		r, ok := c.Ref(id)
		if !ok || r.File == "" {
			t.Fatalf("stageGeneratedRefs: %q is not a catalogued reference with audio", id)
		}
		localFixtureWAV(t, root, r.File, 220)
	}
}

// stageGeneratedRefsFor stages every real reference that serves inst.
func stageGeneratedRefsFor(t *testing.T, root string, inst instrument.ID) {
	t.Helper()
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	for _, r := range c.ForInstrument(inst) {
		if r.Status == "" {
			stageGeneratedRefs(t, root, r.ID)
		}
	}
}
