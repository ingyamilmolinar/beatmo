package ui

import (
	"strings"
	"testing"
)

func TestParityFatalDumpContainsDiagnostics(t *testing.T) {
	g := buildTestGame(t)
	g.SetPlaying(true)
	e := mismatchEntry{Row: 0, Abs: 5, Source: "test", Kind: "view_vs_truth", Scheduled: true, Slate: false, InWindow: true}
	dump := g.buildParityFatalDump(e)
	for _, want := range []string{
		"[PARITY]", "row=0 abs=5", "kind=view_vs_truth",
		"transport:", "seqNextIdxs=", "nextBeatIdxs=",
		"predictor visible=", "timeline offset=", "graph start=",
		"recent mismatches:",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump missing %q\n---\n%s", want, dump)
		}
	}
}
