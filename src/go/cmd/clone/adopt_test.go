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
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// adoptFixture: a clone directory holding a candidate (from a real -no-refine
// run), and a scratch module root with an index.
func adoptFixture(t *testing.T) (dir, repo string, doc instrument.Doc) {
	t.Helper()
	refs := t.TempDir()
	t.Setenv(wavref.EnvVar, refs)
	var out, errb bytes.Buffer
	if code := run([]string{"-inst", "testbar", "-wav", barWAV(t), "-f0", "261.63", "-no-refine"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	dir = filepath.Join(refs, "clones", "testbar")
	b, _ := os.ReadFile(filepath.Join(dir, "candidate.json"))
	doc, err := instrument.ParseFile(b)
	if err != nil {
		t.Fatal(err)
	}
	repo = t.TempDir()
	inst := filepath.Join(repo, "internal/assets/instruments")
	if err := os.MkdirAll(inst, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(inst, "index.json"), []byte("{\n  \"schema_version\": \"0.0.1\",\n  \"instruments\": [\n    \"kick\"\n  ]\n}\n"), 0o644)
	return dir, repo, doc
}

func writeSessions(t *testing.T, rows ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ear_sessions.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(rows, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func row(when, sha, accept string) string {
	return `{"session":"` + when + `","family":"testbar","candidates":[{"id":"K","doc_sha":"` + sha + `","accept":"` + accept + `"}]}`
}

func adopt(t *testing.T, dir, repo, sessions string) (int, string) {
	var out, errb bytes.Buffer
	code := run([]string{"adopt", "-inst", "testbar", "-dir", dir, "-repo", repo, "-sessions", sessions}, &out, &errb)
	return code, out.String() + errb.String()
}

func TestAdoptRefusesWithoutAYes(t *testing.T) {
	dir, repo, doc := adoptFixture(t)
	sha := clone.DocSHA(doc)
	for name, sessions := range map[string]string{
		"no session file": filepath.Join(t.TempDir(), "none.jsonl"),
		"other candidate": writeSessions(t, row("2026-10-13T10:00:00Z", "sha256:0000000000000000", "y")),
		"answered n":      writeSessions(t, row("2026-10-13T10:00:00Z", sha, "n")),
		"newest says p":   writeSessions(t, row("2026-10-13T10:00:00Z", sha, "y"), row("2026-10-13T11:00:00Z", sha, "p")),
	} {
		if code, msg := adopt(t, dir, repo, sessions); code != 2 {
			t.Errorf("%s: exit %d (%s), want 2", name, code, msg)
		}
		if _, err := os.Stat(filepath.Join(repo, "internal/assets/instruments", string(doc.ID)+".json")); err == nil {
			t.Fatalf("%s: wrote the document anyway", name)
		}
	}
}

func TestAdoptWritesTheDocumentOnAYes(t *testing.T) {
	dir, repo, doc := adoptFixture(t)
	// A later measure-loudness pass rewrites render.amplitude: the verdict
	// still applies (doc_sha excludes it).
	doc.Render.Amplitude = 0.42
	b, _ := instrument.MarshalFile(doc)
	os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644)
	sessions := writeSessions(t, row("2026-10-13T09:00:00Z", clone.DocSHA(doc), "n"), row("2026-10-13T10:00:00Z", clone.DocSHA(doc), "y"))
	code, msg := adopt(t, dir, repo, sessions)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, msg)
	}
	got, err := os.ReadFile(filepath.Join(repo, "internal/assets/instruments", string(doc.ID)+".json"))
	if err != nil || !bytes.Equal(got, b) {
		t.Fatalf("shipped file differs from the candidate (err %v)", err)
	}
	var idx struct {
		Instruments []string `json:"instruments"`
	}
	ib, _ := os.ReadFile(filepath.Join(repo, "internal/assets/instruments/index.json"))
	if err := json.Unmarshal(ib, &idx); err != nil || len(idx.Instruments) != 2 || instrument.ID(idx.Instruments[1]) != doc.ID {
		t.Fatalf("index %s", ib)
	}
	if !strings.Contains(msg, "make asset-pack") {
		t.Fatalf("no follow-up steps: %s", msg)
	}
}

// TestAdoptTunedKindReadsTheTunedDirectory: `-kind tuned` ships a `refs tune`
// candidate from $BEATMO_WAV_REFS/tuned/<inst>/ behind the same y gate, and
// never falls back to the clone directory.
func TestAdoptTunedKindReadsTheTunedDirectory(t *testing.T) {
	dir, repo, doc := adoptFixture(t)
	tuned := filepath.Join(wavref.Root(), "tuned", "testbar")
	if err := os.MkdirAll(tuned, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "candidate.json"))
	os.WriteFile(filepath.Join(tuned, "candidate.json"), b, 0o644)
	sessions := writeSessions(t, row("2026-10-19T10:00:00Z", clone.DocSHA(doc), "y"))
	tunedAdopt := func() (int, string) {
		var out, errb bytes.Buffer
		code := run([]string{"adopt", "-inst", "testbar", "-kind", "tuned", "-repo", repo, "-sessions", sessions}, &out, &errb)
		return code, out.String() + errb.String()
	}
	if code, msg := tunedAdopt(); code != 0 {
		t.Fatalf("exit %d: %s", code, msg)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "internal/assets/instruments", string(doc.ID)+".json")); err != nil || !bytes.Equal(got, b) {
		t.Fatalf("shipped file differs from the tuned candidate (err %v)", err)
	}
	os.Remove(filepath.Join(tuned, "candidate.json"))
	if code, msg := tunedAdopt(); code != 1 || !strings.Contains(msg, "make tune INST=testbar") {
		t.Fatalf("no tuned candidate: exit %d (%s), want 1 naming make tune", code, msg)
	}
	var out, errb bytes.Buffer
	if code := run([]string{"adopt", "-inst", "testbar", "-kind", "bogus"}, &out, &errb); code != 1 {
		t.Fatalf("unknown kind: exit %d", code)
	}
}

// shipTestbar puts a shipped testbar document beside the index, as adopt finds an instrument it replaces.
func shipTestbar(t *testing.T, repo string, d instrument.Doc) {
	t.Helper()
	b, err := instrument.MarshalFile(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal/assets/instruments", string(d.ID)+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readShipped(t *testing.T, repo string, id instrument.ID) instrument.Doc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repo, "internal/assets/instruments", string(id)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := instrument.ParseFile(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAdoptKeepsTheShippedLevelAndNoOrigin(t *testing.T) {
	dir, repo, doc := adoptFixture(t)
	shipped := doc.Clone()
	shipped.Render.Amplitude = 0.77
	shipTestbar(t, repo, shipped)
	doc.Origin, doc.Render.Amplitude = doc.ID, 0.42
	b, _ := instrument.MarshalFile(doc)
	os.WriteFile(filepath.Join(dir, "candidate.json"), b, 0o644)
	if code, msg := adopt(t, dir, repo, writeSessions(t, row("2026-11-07T10:00:00Z", clone.DocSHA(doc), "y"))); code != 0 {
		t.Fatalf("exit %d: %s", code, msg)
	}
	got := readShipped(t, repo, doc.ID)
	if got.Render.Amplitude != 0.77 || got.Origin != "" {
		t.Fatalf("shipped amplitude %v origin %q, want the shipped 0.77 and no origin", got.Render.Amplitude, got.Origin)
	}
	if clone.DocSHA(got) != clone.DocSHA(doc) {
		t.Fatal("adopt changed the sound the listener approved")
	}
}

func TestAdoptRefusesACandidateOfAnOlderNoteLength(t *testing.T) {
	dir, repo, doc := adoptFixture(t)
	shipped := doc.Clone()
	shipped.Render.Seconds = doc.Render.Seconds / 2
	shipTestbar(t, repo, shipped)
	sessions := writeSessions(t, row("2026-11-07T10:00:00Z", clone.DocSHA(doc), "y"))
	code, msg := adopt(t, dir, repo, sessions)
	if code != 2 || !strings.Contains(msg, "render.seconds") {
		t.Fatalf("exit %d (%s), want 2 naming render.seconds", code, msg)
	}
	if got := readShipped(t, repo, doc.ID); got.Render.Seconds != shipped.Render.Seconds {
		t.Fatal("wrote the candidate anyway")
	}
	meta := map[string]any{"set": map[string]float64{"render.seconds": doc.Render.Seconds}}
	mb, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(dir, "meta.json"), mb, 0o644)
	if code, msg := adopt(t, dir, repo, sessions); code != 0 {
		t.Fatalf("a pick that set the length: exit %d (%s)", code, msg)
	}
	if got := readShipped(t, repo, doc.ID); got.Render.Seconds != doc.Render.Seconds {
		t.Fatalf("render.seconds %v, want the length the pick set (%v)", got.Render.Seconds, doc.Render.Seconds)
	}
}
