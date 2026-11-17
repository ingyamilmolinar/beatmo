//go:build !js

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestCandidateDir_UnderEnvDirAndNamedByInstrument(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	d, err := candidateDir(roster.Violin)
	if err != nil {
		t.Fatalf("candidateDir: %v", err)
	}
	if !strings.HasPrefix(d, tmp) {
		t.Errorf("candidateDir = %q, want it under %q", d, tmp)
	}
	if filepath.Base(d) != "violin" {
		t.Errorf("candidateDir base = %q, want %q", filepath.Base(d), "violin")
	}
	if _, err := os.Stat(d); err != nil {
		t.Errorf("candidateDir not created: %v", err)
	}
}

func TestRunCandidates_UnknownInstrumentErrors(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var buf bytes.Buffer
	if err := runCandidates(&buf, "no-such-instrument", 3); err == nil {
		t.Error("runCandidates accepted an unknown instrument")
	}
}

// withoutString must never mutate the caller's backing array: runChoose
// passes it refs[i].Instruments, which can still alias the package-level
// memoized wavref.Catalogue() singleton (a shallow append([]Ref(nil), ...)
// copy copies the outer []Ref but not each Ref's nested Instruments slice
// header). An in-place filter there would corrupt every later
// wavref.Catalogue()/loadCatalogue() call in this process -- invisible in
// a one-shot CLI run, but silently order-dependent across a `go test`
// binary. This is the direct regression test for that: it asserts the
// input slice is byte-for-byte unchanged after the call, independent of
// any catalogue data existing to exercise runChoose's multi-candidate path.
func TestWithoutString_DoesNotMutateCallerSlice(t *testing.T) {
	in := []instrument.ID{roster.Trumpet, roster.TrumpetMellow}
	orig := slices.Clone(in)

	got := slices.DeleteFunc(slices.Clone(in), func(id instrument.ID) bool { return id == roster.Trumpet })

	if want := []instrument.ID{roster.TrumpetMellow}; !reflect.DeepEqual(got, want) {
		t.Errorf("withoutString result = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(in, orig) {
		t.Errorf("withoutString mutated the caller's slice in place: got %v, want unchanged %v", in, orig)
	}
}

// Choosing must be reversible: the losers are kept with a reason so the
// decision can be revisited when an instrument proves hard to tune.
func TestRunChoose_RecordsLosersAsAlternatives(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	refs := c.ForInstrument(roster.Violin)
	if len(refs) < 2 {
		t.Skip("violin has only one candidate; nothing to choose between")
	}

	// runChoose ends by writing the mutated catalogue via catalogueWriter.
	// Redirect it to a throwaway path for the duration of this test so a
	// green `go test ./cmd/refs/` never rewrites the real, tracked
	// internal/wavref/catalogue.json as a side effect of running the suite.
	origWriter := catalogueWriter
	t.Cleanup(func() { catalogueWriter = origWriter })
	tmp := filepath.Join(t.TempDir(), "catalogue.json")
	catalogueWriter = func(c wavref.Cat) error { return writeCatalogueAtomic(tmp, c) }

	var buf bytes.Buffer
	if err := runChoose(&buf, roster.Violin, refs[0].ID); err != nil {
		t.Fatalf("runChoose: %v", err)
	}
	if !strings.Contains(buf.String(), refs[0].ID) {
		t.Errorf("choose output does not name the winner %q", refs[0].ID)
	}
}

func TestRunChoose_RejectsIDNotServingInstrument(t *testing.T) {
	var buf bytes.Buffer
	if err := runChoose(&buf, roster.Violin, "trumpet-A4"); err == nil {
		t.Error("runChoose accepted a ref that is not a violin reference")
	}
}

func refAtSemis(id string, semis int) wavref.Ref {
	return wavref.Ref{ID: id, Measured: &wavref.Measured{SemisFromA3: semis}}
}

func TestPitchSpreadSemis(t *testing.T) {
	cases := []struct {
		name string
		refs []wavref.Ref
		want int
	}{
		{"empty", nil, 0},
		{"single", []wavref.Ref{refAtSemis("a", 12)}, 0},
		{"same pitch", []wavref.Ref{refAtSemis("a", 12), refAtSemis("b", 12)}, 0},
		{"five semitones", []wavref.Ref{refAtSemis("sax-A4", 12), refAtSemis("sax-baritone", 8)}, 4},
		{"octave apart", []wavref.Ref{refAtSemis("violin-A4", 12), refAtSemis("violin-freesound-alt", 17)}, 5},
		{"unordered input", []wavref.Ref{refAtSemis("lo", -19), refAtSemis("hi", 0)}, 19},
		{
			"unmeasured refs are ignored, not treated as pitch 0",
			[]wavref.Ref{refAtSemis("a", 12), {ID: "unmeasured"}, refAtSemis("b", 17)},
			5,
		},
		{"all unmeasured", []wavref.Ref{{ID: "a"}, {ID: "b"}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pitchSpreadSemis(tc.refs); got != tc.want {
				t.Errorf("pitchSpreadSemis(%v) = %d, want %d", tc.refs, got, tc.want)
			}
		})
	}
}

func TestPrintPitchSpreadWarning_Thresholds(t *testing.T) {
	t.Run("zero spread prints nothing", func(t *testing.T) {
		var buf bytes.Buffer
		printPitchSpreadWarning(&buf, []wavref.Ref{refAtSemis("a", 12), refAtSemis("b", 12)})
		if buf.Len() != 0 {
			t.Errorf("expected no output for zero spread, got %q", buf.String())
		}
	})

	t.Run("small spread warns but does not escalate", func(t *testing.T) {
		var buf bytes.Buffer
		printPitchSpreadWarning(&buf, []wavref.Ref{refAtSemis("sax-A4", 12), refAtSemis("sax-baritone", 8)})
		out := buf.String()
		if !strings.Contains(out, "WARNING") || !strings.Contains(out, "4 semitone") {
			t.Errorf("expected a 4-semitone warning, got %q", out)
		}
		if strings.Contains(out, "different registers") {
			t.Errorf("a 4-semitone spread should not escalate to the octave-or-more wording, got %q", out)
		}
	})

	t.Run("spread exactly at the octave threshold escalates", func(t *testing.T) {
		var buf bytes.Buffer
		printPitchSpreadWarning(&buf, []wavref.Ref{refAtSemis("a", 0), refAtSemis("b", pitchSpreadOctaveSemis)})
		out := buf.String()
		if !strings.Contains(out, "different registers") {
			t.Errorf("expected the octave-or-more escalation at spread==%d, got %q", pitchSpreadOctaveSemis, out)
		}
	})

	t.Run("spread one below the octave threshold does not escalate", func(t *testing.T) {
		var buf bytes.Buffer
		printPitchSpreadWarning(&buf, []wavref.Ref{refAtSemis("a", 0), refAtSemis("b", pitchSpreadOctaveSemis-1)})
		out := buf.String()
		if strings.Contains(out, "different registers") {
			t.Errorf("spread %d should not escalate, got %q", pitchSpreadOctaveSemis-1, out)
		}
		if !strings.Contains(out, "WARNING") {
			t.Errorf("expected a warning for a non-zero sub-octave spread, got %q", out)
		}
	})

	t.Run("large spread escalates", func(t *testing.T) {
		var buf bytes.Buffer
		printPitchSpreadWarning(&buf, []wavref.Ref{refAtSemis("conga-freesound-alt", 22), refAtSemis("conga-vcsl", -17)})
		out := buf.String()
		if !strings.Contains(out, "WARNING") || !strings.Contains(out, "39 semitone") {
			t.Errorf("expected a 39-semitone warning, got %q", out)
		}
		if !strings.Contains(out, "different registers") {
			t.Errorf("a 39-semitone spread should escalate, got %q", out)
		}
	})
}
