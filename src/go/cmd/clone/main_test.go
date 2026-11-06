package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// barWAV writes a generated two-mode struck bar (C4 + an inharmonic partial).
func barWAV(t *testing.T) string {
	t.Helper()
	rate := 48000
	n := 2 * rate
	pcm := make([]float32, 2400+n)
	for i := 0; i < n; i++ {
		tt := float64(i) / float64(rate)
		pcm[2400+i] = float32(0.4*math.Exp(-3*tt)*math.Cos(2*math.Pi*261.63*tt) +
			0.15*math.Exp(-9*tt)*math.Cos(2*math.Pi*1043.1*tt+0.4))
	}
	b, err := wavgen.EncodeWAV(pcm, rate)
	if err != nil {
		t.Fatal(err)
	}
	wav := filepath.Join(t.TempDir(), "bar.wav")
	if err := os.WriteFile(wav, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return wav
}

func TestCloneWritesTheCandidateAndItsReport(t *testing.T) {
	refs := t.TempDir()
	t.Setenv(wavref.EnvVar, refs)
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "testbar", "-wav", barWAV(t), "-f0", "261.63", "-gens", "2", "-workers", "2"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	dir := filepath.Join(refs, "clones", "testbar")
	raw, err := os.ReadFile(filepath.Join(dir, "analysis.json"))
	if err != nil {
		t.Fatal(err)
	}
	var a clone.Analysis
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	if a.Format != clone.AnalysisFormat || len(a.Modes) == 0 {
		t.Fatalf("analysis.json: %+v", a)
	}
	for _, f := range []string{"report.md", "report.json", "candidate.json", "meta.json", "reference.wav", "candidate.wav"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	db, _ := os.ReadFile(filepath.Join(dir, "candidate.json"))
	doc, err := instrument.ParseFile(db)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ID != "testbar-clone" || !doc.Render.Melodic {
		t.Fatalf("candidate id %q melodic %v", doc.ID, doc.Render.Melodic)
	}
	var m Meta
	mb, _ := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err := json.Unmarshal(mb, &m); err != nil || m.DocSHA != clone.DocSHA(doc) || m.Format != MetaFormat {
		t.Fatalf("meta %+v err %v", m, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", strings.TrimPrefix(m.DocSHA, "sha256:")+".json")); err != nil {
		t.Fatalf("history copy: %v", err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	for _, want := range []string{"## Candidate", "## Loss", "analysis only", "refined"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("report.md lacks %q", want)
		}
	}
}

func TestCloneFlags(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "x", "-wav", "w", "-as", "a", "-into", "b"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "exclusive") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	errb.Reset()
	if code := run([]string{"-inst", "testbar", "-wav", barWAV(t), "-no-refine", "-into", "no-such-inst"}, &out, &errb); code != 1 {
		t.Fatalf("-into an unknown id: exit %d", code)
	}
}

func TestCloneRefusalStillWritesAndExits2(t *testing.T) {
	refs := t.TempDir()
	t.Setenv(wavref.EnvVar, refs)
	b, _ := wavgen.EncodeWAV(make([]float32, 48000), 48000)
	wav := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(wav, b, 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "silent", "-wav", wav}, &out, &errb); code != 2 {
		t.Fatalf("exit %d, want 2 (refusal): %s", code, errb.String())
	}
	if _, err := os.Stat(filepath.Join(refs, "clones", "silent", "report.md")); err != nil {
		t.Fatalf("a refusal must still write the report: %v", err)
	}
}
