//go:build !js

package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestShQuote_SurvivesEval(t *testing.T) {
	for _, in := range []string{
		"/plain/path.wav",
		"/with space/file.wav",
		"/with'quote/file.wav",
		`/with"double/file.wav`,
		"/with$VAR/file.wav",
		"/with;rm -rf x/file.wav",
		"/with`backtick`/file.wav",
		"/with\\backslash/file.wav",
	} {
		// eval the assignment exactly as the Makefile recipes do, then print
		// it back: the round trip must be lossless and must not execute
		// anything.
		script := "eval " + shQuote("V="+shQuote(in)) + "; printf %s \"$V\""
		got, err := exec.Command("bash", "-c", script).Output()
		if err != nil {
			t.Errorf("%q: eval failed: %v", in, err)
			continue
		}
		if string(got) != in {
			t.Errorf("round trip changed %q -> %q", in, string(got))
		}
	}
}

func TestFormatAuditionSh_QuotesUnsafePaths(t *testing.T) {
	fam := multiFam()
	res := []compareResult{
		{RealPath: "/tmp/it's here/REAL.wav", SynthPath: "/tmp/a/S1.wav", Dur: 2},
		{RealPath: "/tmp/it's here/REAL.wav", SynthPath: "/tmp/b/S2.wav", Dur: 2},
	}
	out := formatAuditionSh(fam, res)
	if strings.Contains(out, "BEATMO_AB_REAL='/tmp/it's here/REAL.wav'") {
		t.Errorf("single quote emitted unescaped -- eval would break out of the string:\n%s", out)
	}
	script := out + "\nprintf %s \"$BEATMO_AB_REAL\""
	got, err := exec.Command("bash", "-c", script).Output()
	if err != nil {
		t.Fatalf("the emitted plan is not eval-able: %v\n%s", err, out)
	}
	if string(got) != "/tmp/it's here/REAL.wav" {
		t.Errorf("BEATMO_AB_REAL round-tripped as %q", string(got))
	}
}

func TestFormatCompareSh_QuotesUnsafePaths(t *testing.T) {
	out := formatCompareSh("/tmp/it's/R.wav", "/tmp/it's/S.wav", "ref-id", 2.5)
	script := out + "\nprintf '%s|%s' \"$BEATMO_AB_REAL\" \"$BEATMO_AB_SYNTH\""
	got, err := exec.Command("bash", "-c", script).Output()
	if err != nil {
		t.Fatalf("the emitted plan is not eval-able: %v\n%s", err, out)
	}
	if want := "/tmp/it's/R.wav|/tmp/it's/S.wav"; string(got) != want {
		t.Errorf("round-tripped as %q, want %q", string(got), want)
	}
}
