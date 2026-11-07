//go:build !js

package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestPrintAuditionHeader_MultiOption(t *testing.T) {
	var buf bytes.Buffer
	printAuditionHeader(&buf, multiFam())
	out := buf.String()
	for _, want := range []string{string(roster.Cello), "cello-A3", "2", "+0"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrintAuditionHeader_SingleOption(t *testing.T) {
	var buf bytes.Buffer
	printAuditionHeader(&buf, singleFam())
	out := buf.String()
	for _, want := range []string{string(roster.Bassoon), "bassoon-A2", "1"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q, got:\n%s", want, out)
		}
	}
}

func TestFormatAuditionSh_EmitsFamilyRefAndPerOptionKeys(t *testing.T) {
	fam := multiFam()
	results := []compareResult{
		{RealPath: "/tmp/x/REAL__cello-A3.wav", SynthPath: "/tmp/x/SYNTH__cello.wav", Dur: 2.34},
		{RealPath: "/tmp/x/REAL__cello-A3.wav", SynthPath: "/tmp/x/SYNTH__cello-warm.wav", Dur: 2.34},
	}
	out := formatAuditionSh(fam, results)
	for _, want := range []string{
		"BEATMO_AB_FAMILY='cello'",
		"BEATMO_AB_REF='cello-A3'",
		"BEATMO_AB_REAL='/tmp/x/REAL__cello-A3.wav'",
		"BEATMO_AB_DUR='2.34'",
		"BEATMO_AB_N='2'",
		"BEATMO_AB_OPT1='cello'",
		"BEATMO_AB_OPT1_PATH='/tmp/x/SYNTH__cello.wav'",
		"BEATMO_AB_OPT2='cello-warm'",
		"BEATMO_AB_OPT2_PATH='/tmp/x/SYNTH__cello-warm.wav'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("formatAuditionSh missing %q, got:\n%s", want, out)
		}
	}
}

func TestFormatAuditionSh_DoneSentinelWhenNoFamilyPending(t *testing.T) {
	out := formatAuditionDone()
	if !strings.Contains(out, "BEATMO_AB_DONE='1'") {
		t.Errorf("formatAuditionDone = %q, want BEATMO_AB_DONE='1'", out)
	}
}

// The -tags test synth stub: only that the pipeline completes and writes a file per member, never content.

func TestRunAuditionPlan_RendersEveryMemberAgainstSharedReference(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "cello-A3")

	var out, diag bytes.Buffer
	if err := runAuditionPlan(&out, &diag, roster.Cello, true, filepath.Join(tmp, "verdicts.jsonl"), "", ""); err != nil {
		t.Fatalf("runAuditionPlan: %v", err)
	}
	shOut := out.String()
	if !strings.Contains(shOut, "BEATMO_AB_FAMILY='cello'") {
		t.Errorf("missing family assignment:\n%s", shOut)
	}
	if !strings.Contains(shOut, "BEATMO_AB_REF='cello-A3'") {
		t.Errorf("missing ref assignment (want the shared family reference):\n%s", shOut)
	}
	// ONE reference clip for the family: per-member copies would each carry their own level-match gain while only the first
	// is played.
	sharedReal := filepath.Join(tmp, "compare", "cello", "REAL__cello-A3.wav")
	synthCello := filepath.Join(tmp, "compare", "cello", "SYNTH__cello.wav")
	synthWarm := filepath.Join(tmp, "compare", "cello-warm", "SYNTH__cello-warm.wav")
	for _, p := range []string{sharedReal, synthCello, synthWarm} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected rendered file %s: %v", p, err)
		}
	}
	strayReal := filepath.Join(tmp, "compare", "cello-warm", "REAL__cello-A3.wav")
	if _, err := os.Stat(strayReal); err == nil {
		t.Errorf("a per-member reference copy was written at %s -- every option must be "+
			"level-matched against the ONE clip formatAuditionSh actually plays", strayReal)
	}
	// The plan must point at that single shared clip.
	if !strings.Contains(shOut, "BEATMO_AB_REAL='"+sharedReal+"'") {
		t.Errorf("BEATMO_AB_REAL must name the shared reference %s:\n%s", sharedReal, shOut)
	}
}

// After level-matching the reference and every option sit at one RMS, so the listener grades timbre, not loudness
// (per-member gains played options several dB apart against the one reference heard).
func TestRunAuditionPlan_EveryOptionSharesOneLevel(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "cello-A3")

	var out, diag bytes.Buffer
	if err := runAuditionPlan(&out, &diag, roster.Cello, true, filepath.Join(tmp, "verdicts.jsonl"), "", ""); err != nil {
		t.Fatalf("runAuditionPlan: %v", err)
	}

	rmsOf := func(path string) float64 {
		t.Helper()
		s, _, err := audio.DecodeWAVAtRate(path, wavref.Rate)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		var sumsq float64
		for _, v := range s {
			sumsq += float64(v) * float64(v)
		}
		if len(s) == 0 {
			return 0
		}
		return math.Sqrt(sumsq / float64(len(s)))
	}

	refRMS := rmsOf(filepath.Join(tmp, "compare", "cello", "REAL__cello-A3.wav"))
	if refRMS <= 0 {
		t.Skip("stub renderer produced no reference signal")
	}
	for _, opt := range []string{
		filepath.Join(tmp, "compare", "cello", "SYNTH__cello.wav"),
		filepath.Join(tmp, "compare", "cello-warm", "SYNTH__cello-warm.wav"),
	} {
		got := rmsOf(opt)
		if got <= 0 {
			continue // stub renderer silence; nothing to compare
		}
		if db := 20 * math.Log10(got/refRMS); math.Abs(db) > 0.5 {
			t.Errorf("%s sits %+.2f dB from the reference -- every option must be level-matched "+
				"against the ONE reference clip that gets played", filepath.Base(opt), db)
		}
	}
}

func TestRunAuditionPlan_UnknownFamilyErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	var out, diag bytes.Buffer
	if err := runAuditionPlan(&out, &diag, "not-a-real-family", true, filepath.Join(tmp, "verdicts.jsonl"), "", ""); err == nil {
		t.Error("runAuditionPlan accepted an unknown family")
	}
}

// The BEATMO_AB_DONE path only wires formatAuditionDone to pickFamily's found=false, both tested on their own.

// --- runAuditionPrompt: prompt-only, no rendering, appends verdicts.

func TestRunAuditionPrompt_RecordsVerdictAndAppends(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")

	in := strings.NewReader("1 too nasal\n")
	var out bytes.Buffer
	kind, err := runAuditionPrompt(&out, in, roster.Cello, true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}

	verdicts, err := readVerdicts(outPath)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if len(verdicts) != 1 {
		t.Fatalf("verdicts = %d, want 1", len(verdicts))
	}
	v := verdicts[0]
	if v.Family != roster.Cello || v.Reference != "cello-A3" || v.Verdict != "1" || v.Chosen != roster.Cello || v.Note != "too nasal" {
		t.Errorf("verdict = %+v, unexpected", v)
	}
	if v.SemisFromA3 != 0 {
		t.Errorf("SemisFromA3 = %d, want 0 (cello-A3's measured pitch)", v.SemisFromA3)
	}
	if len(v.Options) != 2 || v.Options[0] != roster.Cello || v.Options[1] != roster.CelloWarm {
		t.Errorf("Options = %v, want [cello cello-warm]", v.Options)
	}
}

func TestRunAuditionPrompt_SkipDoesNotAppend(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")

	kind, err := runAuditionPrompt(&bytes.Buffer{}, strings.NewReader("s\n"), roster.Bassoon, true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictSkipped {
		t.Fatalf("kind = %v, want verdictSkipped", kind)
	}
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		t.Fatalf("readVerdicts: %v", err)
	}
	if len(verdicts) != 0 {
		t.Errorf("skip must not append a verdict, got %d", len(verdicts))
	}
}

func TestRunAuditionPrompt_QuitReturnsQuitKind(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	kind, err := runAuditionPrompt(&bytes.Buffer{}, strings.NewReader("q\n"), roster.Bassoon, true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictQuit {
		t.Errorf("kind = %v, want verdictQuit", kind)
	}
}

func TestRunAuditionPrompt_ResumeSkipsAlreadyAnsweredAutoPick(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	if err := appendVerdict(outPath, earVerdict{Family: roster.Bassoon, Verdict: "y"}); err != nil {
		t.Fatal(err)
	}

	// Resume skips bassoon (alphabetically first) and lands on cello, multi-option: hence a numeric choice.
	kind, err := runAuditionPrompt(&bytes.Buffer{}, strings.NewReader("1\n"), "", true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("verdicts = %d, want 2 (bassoon pre-seeded + one new)", len(verdicts))
	}
	if verdicts[1].Family == roster.Bassoon {
		t.Errorf("resume should have skipped bassoon, but it was re-recorded: %+v", verdicts[1])
	}
}

// Exit signalling via sentinel errors, so the CLI dispatch is testable without killing the test process.

func TestCmdAudition_QuitReturnsSentinelError(t *testing.T) {
	tmp := t.TempDir()
	restoreStdin := swapStdin(t, "q\n")
	defer restoreStdin()

	err := cmdAudition([]string{"-instrument", string(roster.Bassoon), "-out", filepath.Join(tmp, "v.jsonl")})
	if !errors.Is(err, errAuditionQuit) {
		t.Errorf("cmdAudition = %v, want errAuditionQuit", err)
	}
}

func TestCmdAudition_ReplayReturnsSentinelError(t *testing.T) {
	tmp := t.TempDir()
	restoreStdin := swapStdin(t, "r\n")
	defer restoreStdin()

	err := cmdAudition([]string{"-instrument", string(roster.Bassoon), "-out", filepath.Join(tmp, "v.jsonl")})
	if !errors.Is(err, errAuditionReplay) {
		t.Errorf("cmdAudition = %v, want errAuditionReplay", err)
	}
}

func TestCmdAudition_RecordedReturnsNilError(t *testing.T) {
	tmp := t.TempDir()
	restoreStdin := swapStdin(t, "y\n")
	defer restoreStdin()

	err := cmdAudition([]string{"-instrument", string(roster.Bassoon), "-out", filepath.Join(tmp, "v.jsonl")})
	if err != nil {
		t.Errorf("cmdAudition = %v, want nil", err)
	}
}

// swapStdin feeds content to os.Stdin and sends os.Stdout to /dev/null for the same span, so the interactive session
// stays out of the make test log.
func swapStdin(t *testing.T, content string) func() {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	origIn, origOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = r, devNull
	go func() {
		w.WriteString(content)
		w.Close()
	}()
	return func() {
		os.Stdin, os.Stdout = origIn, origOut
		devNull.Close()
	}
}

// The plain-English confirmation keeps a single-option "1" from recording a stronger claim than intended.

func TestFormatVerdictConfirmation_Multi_NumericChoice(t *testing.T) {
	fam := multiFam()
	got := formatVerdictConfirmation(fam, "2", roster.CelloWarm, "attack too soft")
	want := `recorded: cello-warm is closest (note: "attack too soft")`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatVerdictConfirmation_Multi_NoNote(t *testing.T) {
	fam := multiFam()
	got := formatVerdictConfirmation(fam, "1", roster.Cello, "")
	want := "recorded: cello is closest"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatVerdictConfirmation_Multi_Neither(t *testing.T) {
	fam := multiFam()
	got := formatVerdictConfirmation(fam, "n", "", "")
	want := "recorded: neither option is close"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatVerdictConfirmation_Single_Yes(t *testing.T) {
	fam := singleFam()
	got := formatVerdictConfirmation(fam, "y", roster.Bassoon, "spot on")
	want := `recorded: bassoon sounds like the real thing (note: "spot on")`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatVerdictConfirmation_Single_Partly(t *testing.T) {
	fam := singleFam()
	got := formatVerdictConfirmation(fam, "p", roster.Bassoon, "")
	want := "recorded: bassoon partly sounds like the real thing"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatVerdictConfirmation_Single_No(t *testing.T) {
	fam := singleFam()
	got := formatVerdictConfirmation(fam, "n", "", "")
	want := "recorded: bassoon does NOT sound like the real thing"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRunAuditionPrompt_EchoesWhatWasRecorded_Multi(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")

	var out bytes.Buffer
	kind, err := runAuditionPrompt(&out, strings.NewReader("2 too bright\n"), roster.Cello, true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if !strings.Contains(out.String(), `recorded: cello-warm is closest (note: "too bright")`) {
		t.Errorf("expected a plain-English confirmation naming cello-warm, got:\n%s", out.String())
	}
}

func TestRunAuditionPrompt_EchoesWhatWasRecorded_SingleViaOneSynonym(t *testing.T) {
	// The "1" -> "y" synonym must still be echoed as the STRONG single-
	// option claim it actually records, not silently.
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")

	var out bytes.Buffer
	kind, err := runAuditionPrompt(&out, strings.NewReader("1 test-note\n"), roster.Bassoon, true, outPath, "", "")
	if err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if kind != verdictRecorded {
		t.Fatalf("kind = %v, want verdictRecorded", kind)
	}
	if !strings.Contains(out.String(), `recorded: bassoon sounds like the real thing (note: "test-note")`) {
		t.Errorf("expected a plain-English confirmation, got:\n%s", out.String())
	}
}

// The single-option prompt advertises "1": an undocumented input mapping to a strong verdict was the problem, not the
// synonym.

func TestPromptLine_SingleOption_Advertises1Synonym(t *testing.T) {
	line := promptLine(singleFam())
	if !strings.Contains(line, "y/1=yes") {
		t.Errorf("single-option prompt does not advertise the 1 synonym:\n%s", line)
	}
}

// The header prints once per family, at the prompt.

func TestRunAuditionPlan_DoesNotPrintFamilyHeader(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	stageGeneratedRefs(t, tmp, "bassoon-A2")
	var out, diag bytes.Buffer
	if err := runAuditionPlan(&out, &diag, roster.Bassoon, true, filepath.Join(tmp, "verdicts.jsonl"), "", ""); err != nil {
		t.Fatalf("runAuditionPlan: %v", err)
	}
	if strings.Contains(diag.String(), "===") {
		t.Errorf("runAuditionPlan's diag output should not print the family header (that's the prompt step's job), got:\n%s", diag.String())
	}
}

func TestRunAuditionPrompt_PrintsFamilyHeaderExactlyOnce(t *testing.T) {
	tmp := t.TempDir()
	outPath := filepath.Join(tmp, "verdicts.jsonl")
	var out bytes.Buffer
	if _, err := runAuditionPrompt(&out, strings.NewReader("y\n"), roster.Bassoon, true, outPath, "", ""); err != nil {
		t.Fatalf("runAuditionPrompt: %v", err)
	}
	if n := strings.Count(out.String(), "=== bassoon"); n != 1 {
		t.Errorf("family header printed %d times, want 1:\n%s", n, out.String())
	}
}
