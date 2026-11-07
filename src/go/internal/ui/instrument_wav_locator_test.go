//go:build test

package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wavgen"
)

// The real os.ReadFile and net/http locators against GENERATED WAV bytes: no recording is ever read.
func TestWavFileAndURLLocators(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	const inst = roster.Snare
	pcm := wavgen.Sine(440, 0.6, 0.05, 48000)
	audio.PutUserSample(inst, pcm, 48000)
	t.Cleanup(func() { audio.DropBuiltinSampleOverride(inst) })

	bundle, err := g.drum.BuildInstrumentBundle(0)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	ref := bundle.Instruments[0].WavRef
	if ref == nil {
		t.Fatal("no wav ref")
	}
	canonical, err := wavgen.EncodeWAV(pcm, 48000)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if ref.Sha256 != audio.Sha256Hex(canonical) {
		t.Fatalf("pin %q != canonical hash %q", ref.Sha256, audio.Sha256Hex(canonical))
	}

	// bundleWithSource clones the exported doc but pins a single WAV source.
	bundleWithSource := func(src audio.WavSource) *audio.InstrumentBundle {
		d := bundle.Instruments[0]
		d.WavRef = &audio.WavRef{Name: ref.Name, Sha256: ref.Sha256, Sources: []audio.WavSource{src}}
		return &audio.InstrumentBundle{
			Envelope: bundle.Envelope, Format: bundle.Format, Effects: bundle.Effects,
			Instruments: []audio.BundleInstrument{d},
		}
	}
	keepBoth := func(ImportCollision) CollisionChoice { return CollisionKeepBoth }

	// --- FILE locator: write the canonical WAV to a temp file (not ./assets). ---
	wavPath := filepath.Join(t.TempDir(), "shared.wav")
	if err := os.WriteFile(wavPath, canonical, 0o644); err != nil {
		t.Fatalf("write temp wav: %v", err)
	}
	fileRes, err := g.drum.ApplyInstrumentBundle(bundleWithSource(audio.WavSource{Kind: audio.WavSourceFile, Ref: wavPath}), keepBoth, -1, g.drum.wavImportLocators())
	if err != nil {
		t.Fatalf("file import: %v", err)
	}
	t.Cleanup(func() { audio.UnregisterSamplePCM(fileRes[0].ID) })
	if fileRes[0].WavState != audio.WavResolveMatched {
		t.Errorf("file locator: state = %v, want Matched", fileRes[0].WavState)
	}
	if !audio.IsUserSample(fileRes[0].ID) {
		t.Errorf("file locator: sample not bound")
	}

	// --- URL locator: serve the canonical WAV over HTTP. ---
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(canonical)
	}))
	defer srv.Close()
	urlRes, err := g.drum.ApplyInstrumentBundle(bundleWithSource(audio.WavSource{Kind: audio.WavSourceURL, Ref: srv.URL}), keepBoth, -1, g.drum.wavImportLocators())
	if err != nil {
		t.Fatalf("url import: %v", err)
	}
	t.Cleanup(func() { audio.UnregisterSamplePCM(urlRes[0].ID) })
	if urlRes[0].WavState != audio.WavResolveMatched {
		t.Errorf("url locator: state = %v, want Matched", urlRes[0].WavState)
	}
	if !audio.IsUserSample(urlRes[0].ID) {
		t.Errorf("url locator: sample not bound")
	}

	// --- A URL serving different bytes is a version mismatch (not a bind). ---
	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		other, _ := wavgen.EncodeWAV(wavgen.Sine(880, 0.6, 0.05, 48000), 48000)
		_, _ = w.Write(other)
	}))
	defer badSrv.Close()
	badRes, _ := g.drum.ApplyInstrumentBundle(bundleWithSource(audio.WavSource{Kind: audio.WavSourceURL, Ref: badSrv.URL}), keepBoth, -1, g.drum.wavImportLocators())
	t.Cleanup(func() {
		if len(badRes) > 0 {
			audio.UnregisterSamplePCM(badRes[0].ID)
		}
	})
	if len(badRes) == 0 || badRes[0].WavState != audio.WavResolveMismatch {
		t.Errorf("wrong-bytes URL should be a mismatch, got %+v", badRes)
	}
}
