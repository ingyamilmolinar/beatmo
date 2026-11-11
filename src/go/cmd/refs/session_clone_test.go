//go:build !js

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// cloneDirFixture writes a clone candidate (a real inverted document) and its
// meta under root/<name>/, the layout cmd/clone produces.
func cloneDirFixture(t *testing.T, root string, name instrument.ID, ref string) instrument.Doc {
	t.Helper()
	a := clone.Analysis{Rate: 48000, FloorDB: -80, Pitch: clone.PitchInfo{Pitched: true, F0Hz: 261.63},
		Modes: []clone.Mode{{Freq: 261.63, Alpha: 2, Amp: 0.3}, {Freq: 722, Alpha: 5, Amp: 0.1, Phase: 1}}}
	d, _, err := clone.Invert(a, clone.InvertOptions{ID: name + "-clone"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, string(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := instrument.MarshalFile(d)
	os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644)
	m, _ := json.Marshal(map[string]any{"ref": ref, "fit_pitch": 3.0})
	os.WriteFile(filepath.Join(dir, "meta.json"), m, 0o644)
	return d
}

func TestWithCloneAddsCandidateK(t *testing.T) {
	root := t.TempDir()
	d := cloneDirFixture(t, root, roster.TubularBells, "vcsl-tubular")
	fc, err := withClone(familyCandidates{}, false, roster.TubularBells, root)
	if err != nil {
		t.Fatal(err)
	}
	if fc.Reference != "vcsl-tubular" || fc.Base != roster.TubularBells || len(fc.Candidates) != 1 {
		t.Fatalf("family %+v", fc)
	}
	k := fc.Candidates[0]
	if k.ID != "k" || k.Doc == nil || k.DocSHA != clone.DocSHA(d) || k.FitPitch != 3 {
		t.Fatalf("candidate K %+v", k)
	}
	// A family in the candidates file keeps its own candidates and gains K.
	in := familyCandidates{Reference: "r", Base: roster.TubularBells, Candidates: []candidateSpec{{ID: "a", Label: "x", Hypothesis: "y", Overrides: map[string]float64{"g": 1}}}}
	fc, err = withClone(in, true, roster.TubularBells, root)
	if err != nil || len(fc.Candidates) != 2 || fc.Reference != "r" || len(in.Candidates) != 1 {
		t.Fatalf("merged %+v err %v (input mutated: %d)", fc, err, len(in.Candidates))
	}
	if got := cloneFamilies(root); len(got) != 1 || got[0] != roster.TubularBells {
		t.Fatalf("clone families %v", got)
	}
	if _, err := withClone(familyCandidates{}, false, "no-clone-here", root); err == nil {
		t.Fatal("a family without a clone must be an error")
	}
}

func TestSessionRecordsTheCloneVerdict(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	root := t.TempDir()
	d := cloneDirFixture(t, root, roster.Kalimba, "vcsl-kalimba")
	fc, err := withClone(familyCandidates{}, false, roster.Kalimba, root)
	if err != nil {
		t.Fatal(err)
	}
	ref := wavref.Ref{ID: "vcsl-kalimba", Measured: &wavref.Measured{SemisFromA3: 3, DurationSec: 1}}
	render := func(familyCandidates, wavref.Ref) (sessionClips, sessionRenderMeta, error) {
		return sessionClips{refPath: "/R.wav", curPath: "/C.wav", candPaths: map[string]string{"K": "/K.wav"}, order: []string{"R", "C", "K"}},
			sessionRenderMeta{Pitch: 3, Dur: 1}, nil
	}
	out := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	in := strings.Join([]string{"", "4", "+", "k", "maybe", "y", "", "1", ""}, "\n") + "\n"
	var log bytes.Buffer
	k, err := runSessionFamily(&log, strings.NewReader(in), &fakePlayer{}, roster.Kalimba, fc, ref, render, out, false, false)
	if err != nil || k != verdictRecorded {
		t.Fatalf("%v %v\n%s", k, err, log.String())
	}
	s := readOneSession(t, out)
	c := s.Candidates[0]
	if c.ID != "K" || c.DocSHA != clone.DocSHA(d) || c.RecipeSHA != c.DocSHA || c.Accept != "y" || c.Rating != 4 || c.VsCurrent != "better" || s.Ship != "K" {
		t.Fatalf("clone candidate %+v ship %q", c, s.Ship)
	}
	if !strings.Contains(log.String(), "invalid: want y, n or p") {
		t.Fatal("a non-answer must be re-asked")
	}
}
