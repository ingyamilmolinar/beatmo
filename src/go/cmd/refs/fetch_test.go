//go:build !js

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestFFmpegArgs_DeterministicAndCorrect(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, StartSec: 0.5, MaxSec: 2.0}
	got := strings.Join(ffmpegArgs("in.aif", "out.wav", cv), " ")
	for _, want := range []string{
		"-map_metadata -1", // no source tags
		"-bitexact",        // no encoder/timestamp tags
		"-ac 1",
		"-ar 44100",
		"-sample_fmt s16",
		"-ss 0.5",
		"-t 2",
		"-i in.aif",
		"out.wav",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ffmpeg args missing %q\n  got: %s", want, got)
		}
	}
}

// -ss must precede -i so ffmpeg seeks the input rather than decoding and
// discarding, which is both faster and frame-accurate for our purposes.
func TestFFmpegArgs_SeekBeforeInput(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, StartSec: 1.0, MaxSec: 2.0}
	args := ffmpegArgs("in.aif", "out.wav", cv)
	ss, in := -1, -1
	for i, a := range args {
		switch a {
		case "-ss":
			ss = i
		case "-i":
			in = i
		}
	}
	if ss < 0 || in < 0 || ss > in {
		t.Errorf("-ss must come before -i; got %v", args)
	}
}

func TestFFmpegArgs_OmitsSeekWhenZero(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0}
	for _, a := range ffmpegArgs("in.aif", "out.wav", cv) {
		if a == "-ss" {
			t.Error("-ss emitted for StartSec 0")
		}
	}
}

func TestSemisFromA3(t *testing.T) {
	cases := []struct {
		f0   float64
		want int
	}{
		{220.0, 0},   // A3
		{440.0, 12},  // A4
		{110.0, -12}, // A2
		{261.63, 3},  // C4
		{587.33, 17}, // D5
		{0, 0},       // undetected pitch is reported as the origin
	}
	for _, c := range cases {
		if got := semisFromA3(c.f0); got != c.want {
			t.Errorf("semisFromA3(%.2f) = %d, want %d", c.f0, got, c.want)
		}
	}
}

// The whole point of measuring is to stop trusting filenames. Assert the
// measurement against a file whose true pitch we know independently: a
// GENERATED A4 harmonic tone saved under a name that claims a different note
// (tests never read reference recordings -- see internal/wavref).
func TestMeasureFile_DetectsPitchFromAudioNotName(t *testing.T) {
	const sr = 44100
	pcm := make([]float32, 2*sr)
	for i := range pcm {
		ph := 2 * math.Pi * 440 * float64(i) / sr
		pcm[i] = float32(0.3*math.Sin(ph) + 0.15*math.Sin(2*ph) + 0.08*math.Sin(3*ph))
	}
	data, err := wavgen.EncodeWAV(pcm, sr)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	p := filepath.Join(t.TempDir(), "cello_C2.wav") // the name lies; the audio is A4
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := measureFile(p)
	if err != nil {
		t.Fatalf("measureFile: %v", err)
	}
	if math.Abs(m.F0Hz-440.0) > 12.0 {
		t.Errorf("F0Hz = %.2f, want ~440 (A4)", m.F0Hz)
	}
	if m.SemisFromA3 != 12 {
		t.Errorf("SemisFromA3 = %d, want 12", m.SemisFromA3)
	}
	if m.Peak <= 0 || m.Peak > 1.01 {
		t.Errorf("Peak = %f, want (0,1]", m.Peak)
	}
	if m.RMS <= 0 || m.RMS > m.Peak {
		t.Errorf("RMS = %f, want (0, Peak]", m.RMS)
	}
	if m.DurationSec <= 0 {
		t.Errorf("DurationSec = %f, want > 0", m.DurationSec)
	}
}

// Every ref -- instrument or song tier -- lands in the one reference-audio
// root. Nothing is ever written into the repository's tree.
func TestDestDir_EveryTierGoesToTheRoot(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	for _, r := range []wavref.Ref{
		{ID: "x", Tier: wavref.TierInstrument, File: "x.wav"},
		{ID: "y", Tier: wavref.TierSong, File: "y.wav"},
	} {
		d, err := destDir(r)
		if err != nil {
			t.Fatalf("destDir(%s): %v", r.ID, err)
		}
		if d != tmp {
			t.Errorf("%s ref dest = %q, want %q ($%s)", r.Tier, d, tmp, wavref.EnvVar)
		}
	}
}

// runFetch is the ONE command that rewrites catalogue.json, so it must
// refuse to operate on a structurally invalid catalogue rather than
// laundering the corruption back into the source of truth (unlike list,
// verify, doctor -- which already gate on loadCatalogue()'s Validate()).
// The embedded catalogue.json is always valid, so this exercises the guard
// via the overridable catalogueLoader seam rather than corrupting real data.
func TestRunFetch_RefusesToOperateOnInvalidCatalogue(t *testing.T) {
	orig := catalogueLoader
	defer func() { catalogueLoader = orig }()
	wantErr := errors.New("boom: structurally invalid catalogue")
	catalogueLoader = func() (wavref.Cat, error) { return wavref.Cat{}, wantErr }

	var buf bytes.Buffer
	err := runFetch(&buf, fetchOpts{})
	if err == nil {
		t.Fatal("runFetch should refuse when the catalogue fails to load/validate, got nil error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("runFetch error = %v, want it to wrap %v", err, wantErr)
	}
	if buf.Len() != 0 {
		t.Errorf("runFetch wrote output before validating the catalogue: %q", buf.String())
	}
}

// writeCatalogue's write must be crash-safe: write to a same-directory temp
// file, then rename over the target, and never leave the temp file behind.
func TestWriteCatalogueAtomic_WritesAndLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalogue.json")
	c := wavref.Cat{Refs: []wavref.Ref{{
		ID: "x", Tier: wavref.TierInstrument, Instruments: []instrument.ID{"x"}, File: "x.wav",
		Source: &wavref.Source{Provider: "local", URL: "http://example.invalid/x"},
	}}}

	if err := writeCatalogueAtomic(path, c); err != nil {
		t.Fatalf("writeCatalogueAtomic: %v", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back written catalogue: %v", err)
	}
	var got wavref.Cat
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal written catalogue: %v", err)
	}
	if len(got.Refs) != 1 || got.Refs[0].ID != "x" {
		t.Errorf("round-tripped catalogue = %+v, want one ref with id x", got)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file left behind at %s.tmp (stat err=%v)", path, err)
	}
}

// localFixtureWAV writes a small, deterministic sine-wave WAV to dir and
// returns its path. Using internal/wavgen keeps this fixture free of any
// real/copyrighted audio, matching how the rest of the repo avoids
// committing real WAVs to git.
func localFixtureWAV(t *testing.T, dir, name string, freqHz float64) string {
	t.Helper()
	pcm := wavgen.Sine(freqHz, 0.5, 1.0, 44100)
	b, err := wavgen.EncodeWAV(pcm, 44100)
	if err != nil {
		t.Fatalf("EncodeWAV: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return p
}

// provider "local" is documented on wavref.Source ("uiowa-mis | vcsl |
// freesound | archive-org | imslp | local") but fetchOne had no branch for
// it -- it always did an HTTP GET of Source.URL. This is the recovery route
// promised for the lossy freesound-preview entries: drop a lossless
// original on disk, point Source.URL at it, flip Provider to "local".
// Source.URL non-empty case: the path is the RAW source and goes through
// the same ffmpeg convert step as every other provider.
func TestFetchOneLocal_URLPathGoesThroughConvertPipeline(t *testing.T) {
	srcDir := t.TempDir()
	destDirPath := t.TempDir()
	rawPath := localFixtureWAV(t, srcDir, "raw-source.wav", 330.0) // E4-ish

	out := filepath.Join(destDirPath, "local-fixture.wav")
	r := &wavref.Ref{
		ID: "local-fixture-url", Tier: wavref.TierInstrument,
		Instruments: []instrument.ID{"x"}, File: "local-fixture.wav",
		Source:  &wavref.Source{Provider: localProvider, URL: rawPath},
		Convert: &wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0},
	}

	did, err := fetchOneLocal(io.Discard, r, out)
	if err != nil {
		t.Fatalf("fetchOneLocal: %v", err)
	}
	if !did {
		t.Error("fetchOneLocal reported did=false for a fresh fetch")
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("converted output not written: %v", err)
	}
	if r.PCMSHA256 == "" {
		t.Error("PCMSHA256 not populated")
	}
	if r.Measured == nil {
		t.Fatal("Measured not populated")
	}
	if math.Abs(r.Measured.F0Hz-330.0) > 15.0 {
		t.Errorf("Measured.F0Hz = %.1f, want ~330 (the fixture's real frequency, not a filename guess)", r.Measured.F0Hz)
	}

	// The hash must be reproducible from the ACTUAL converted bytes, not
	// hand-written: recompute independently and compare.
	wantHash, err := hashFile(out)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if r.PCMSHA256 != wantHash {
		t.Errorf("PCMSHA256 = %s, want %s (recomputed from the actual output file)", r.PCMSHA256, wantHash)
	}
}

// Source.URL empty case: the destination is already the final audio (no
// raw source to convert), so it is measured/hashed in place with no ffmpeg
// invocation.
func TestFetchOneLocal_EmptyURLUsesDestinationInPlace(t *testing.T) {
	destDirPath := t.TempDir()
	out := localFixtureWAV(t, destDirPath, "already-final.wav", 220.0) // A3

	r := &wavref.Ref{
		ID: "local-fixture-inplace", Tier: wavref.TierInstrument,
		Instruments: []instrument.ID{"x"}, File: "already-final.wav",
		Source: &wavref.Source{Provider: localProvider, URL: ""},
	}

	did, err := fetchOneLocal(io.Discard, r, out)
	if err != nil {
		t.Fatalf("fetchOneLocal: %v", err)
	}
	if !did {
		t.Error("fetchOneLocal reported did=false for a fresh fetch")
	}
	if r.Measured == nil {
		t.Fatal("Measured not populated")
	}
	if math.Abs(r.Measured.F0Hz-220.0) > 15.0 {
		t.Errorf("Measured.F0Hz = %.1f, want ~220 (A3)", r.Measured.F0Hz)
	}
	wantHash, err := hashFile(out)
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if r.PCMSHA256 != wantHash {
		t.Errorf("PCMSHA256 = %s, want %s", r.PCMSHA256, wantHash)
	}
}

// A missing local source (either the URL path, or the destination when URL
// is empty) must error clearly, not silently skip -- it is a configuration
// mistake the caller needs to see.
func TestFetchOneLocal_MissingSourceErrorsClearly(t *testing.T) {
	destDirPath := t.TempDir()
	out := filepath.Join(destDirPath, "never-written.wav")

	t.Run("missing URL path", func(t *testing.T) {
		r := &wavref.Ref{
			ID: "local-fixture-missing-url", Tier: wavref.TierInstrument,
			Instruments: []instrument.ID{"x"}, File: "never-written.wav",
			Source: &wavref.Source{Provider: localProvider, URL: filepath.Join(destDirPath, "does-not-exist.wav")},
		}
		if _, err := fetchOneLocal(io.Discard, r, out); err == nil {
			t.Error("fetchOneLocal accepted a nonexistent local source path")
		}
	})

	t.Run("missing destination with empty URL", func(t *testing.T) {
		r := &wavref.Ref{
			ID: "local-fixture-missing-dest", Tier: wavref.TierInstrument,
			Instruments: []instrument.ID{"x"}, File: "never-written.wav",
			Source: &wavref.Source{Provider: localProvider, URL: ""},
		}
		if _, err := fetchOneLocal(io.Discard, r, out); err == nil {
			t.Error("fetchOneLocal accepted an empty URL with no file already at the destination")
		}
	})
}

// runFetch must persist a changed catalogue through the SAME injectable
// seam runChoose already uses (catalogueWriter), not by calling
// writeCatalogue directly. No test previously drove runFetch into its
// changed>0 write branch, so this hazard was only incidental: the moment a
// test exercises a real write path (trivial now that the local provider
// makes fixtures cheap), an unguarded runFetch would silently clobber the
// real, tracked internal/wavref/catalogue.json as a side effect of `go
// test`. This drives runFetch down exactly that write path (a local-provider
// ref with -force) with catalogueWriter redirected to a throwaway file, and
// asserts the REAL catalogue.json on disk is byte-for-byte unchanged.
func TestRunFetch_WritesThroughCatalogueWriterSeam(t *testing.T) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		t.Skip("module root not found")
	}
	realPath := filepath.Join(root, "internal", "wavref", "catalogue.json")
	before, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("read real catalogue.json before test: %v", err)
	}
	// Safety net independent of the assertion below: if the seam under test
	// is broken and this test DOES clobber the real tracked file, restore it
	// unconditionally so a red run never leaves the working tree corrupted.
	t.Cleanup(func() {
		if err := os.WriteFile(realPath, before, 0o644); err != nil {
			t.Errorf("failed to restore real catalogue.json after test: %v", err)
		}
	})

	origWriter := catalogueWriter
	t.Cleanup(func() { catalogueWriter = origWriter })
	tmpWrite := filepath.Join(t.TempDir(), "catalogue.json")
	catalogueWriter = func(c wavref.Cat) error { return writeCatalogueAtomic(tmpWrite, c) }

	origLoader := catalogueLoader
	t.Cleanup(func() { catalogueLoader = origLoader })

	srcDir := t.TempDir()
	rawPath := localFixtureWAV(t, srcDir, "raw-source.wav", 440.0)
	envDir := t.TempDir()
	t.Setenv(wavref.EnvVar, envDir)

	fixtureRef := wavref.Ref{
		ID: "fetch-seam-fixture", Tier: wavref.TierSong, Template: "x",
		File:    "fetch-seam-fixture.wav",
		Source:  &wavref.Source{Provider: localProvider, URL: rawPath},
		Convert: &wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0},
	}
	catalogueLoader = func() (wavref.Cat, error) { return wavref.Cat{Refs: []wavref.Ref{fixtureRef}}, nil }

	var buf bytes.Buffer
	if err := runFetch(&buf, fetchOpts{Force: true}); err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if !strings.Contains(buf.String(), "updated catalogue.json") {
		t.Fatalf("runFetch did not report a write; output:\n%s", buf.String())
	}

	// The write must have landed on the redirected throwaway path...
	if _, err := os.Stat(tmpWrite); err != nil {
		t.Errorf("catalogueWriter seam was not used -- nothing written to %s: %v", tmpWrite, err)
	}
	// ...and NEVER on the real, tracked catalogue.json.
	after, err := os.ReadFile(realPath)
	if err != nil {
		t.Fatalf("read real catalogue.json after test: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("runFetch modified the real, tracked internal/wavref/catalogue.json -- it must write only through catalogueWriter")
	}
}

// End-to-end through fetchOne (not just fetchOneLocal directly): provider
// "local" must be routed correctly and skip the HTTP download path
// entirely -- Source.URL here is a filesystem path, not a URL, so any code
// path that tried to http.Get it would fail loudly.
func TestFetchOne_RoutesLocalProviderAwayFromHTTPDownload(t *testing.T) {
	srcDir := t.TempDir()
	rawPath := localFixtureWAV(t, srcDir, "raw-source.wav", 440.0)

	envDir := t.TempDir()
	t.Setenv(wavref.EnvVar, envDir)

	r := &wavref.Ref{
		ID: "local-fixture-e2e", Tier: wavref.TierSong, Template: "x",
		File:    "local-fixture-e2e.wav",
		Source:  &wavref.Source{Provider: localProvider, URL: rawPath},
		Convert: &wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0},
	}

	did, err := fetchOne(io.Discard, r, false)
	if err != nil {
		t.Fatalf("fetchOne: %v", err)
	}
	if !did {
		t.Error("fetchOne reported did=false for a fresh fetch")
	}
	if r.PCMSHA256 == "" {
		t.Error("PCMSHA256 not populated")
	}
	if _, err := os.Stat(filepath.Join(envDir, "local-fixture-e2e.wav")); err != nil {
		t.Errorf("output not written to the expected dest dir: %v", err)
	}
}
