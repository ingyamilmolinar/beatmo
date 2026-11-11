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

// adoptHarness wires runAdopt up to a throwaway world: a drop-in source
// dir, a converted-refs dir, and a catalogue that is loaded from and
// written to memory. Nothing here touches the committed catalogue.json or
// the developer's own ~/.beatmo directories.
type adoptHarness struct {
	sources string
	refs    string
	written *wavref.Cat
	writes  int
	out     bytes.Buffer
}

func newAdoptHarness(t *testing.T, seed []wavref.Ref) *adoptHarness {
	t.Helper()
	root := t.TempDir()
	h := &adoptHarness{refs: root, sources: filepath.Join(root, "sources")}
	t.Setenv(wavref.EnvVar, root)
	if err := os.MkdirAll(h.sources, 0o755); err != nil {
		t.Fatal(err)
	}

	cat := wavref.Cat{Refs: seed}
	origLoader, origWriter := catalogueLoader, catalogueWriter
	t.Cleanup(func() { catalogueLoader, catalogueWriter = origLoader, origWriter })
	catalogueLoader = func() (wavref.Cat, error) { return cat, nil }
	catalogueWriter = func(c wavref.Cat) error {
		copied := c
		h.written = &copied
		h.writes++
		return nil
	}
	return h
}

// drop writes a raw source WAV into the drop-in directory.
func (h *adoptHarness) drop(t *testing.T, name string, freqHz float64) string {
	t.Helper()
	return localFixtureWAV(t, h.sources, name, freqHz)
}

func (h *adoptHarness) writtenRef(t *testing.T, id string) wavref.Ref {
	t.Helper()
	if h.written == nil {
		t.Fatal("no catalogue was written")
	}
	r, ok := h.written.Ref(id)
	if !ok {
		t.Fatalf("ref %q not in the written catalogue: %v", id, refIDs(h.written.Refs))
	}
	return r
}

func baseOpts(inst instrument.ID, wav string) adoptOpts {
	return adoptOpts{Instrument: inst, Wav: wav}
}

// The headline case: a bare filename in the drop-in dir becomes the
// instrument's reference, with the audio converted into the refs dir.
func TestRunAdopt_ConvertsTheDropInFileIntoTheRefsDir(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	r := h.writtenRef(t, "snare-lofi-local")
	if r.File == "" {
		t.Fatal("adopted ref records no file")
	}
	if _, err := os.Stat(filepath.Join(h.refs, r.File)); err != nil {
		t.Errorf("converted audio not written into the refs dir: %v", err)
	}
}

// Provenance is the whole point: every field the catalogue tracks must be
// filled in from the adoption, not left blank for someone to hand-edit.
func TestRunAdopt_RecordsFullProvenance(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	raw := h.drop(t, "snare_hot.wav", 220.0)

	opts := baseOpts(roster.SnareLofi, "snare_hot.wav")
	opts.Note = "recorded in the garage"
	if err := runAdopt(&h.out, opts); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	r := h.writtenRef(t, "snare-lofi-local")
	if r.Source == nil {
		t.Fatal("adopted ref has no source block")
	}
	if r.Source.Provider != localProvider {
		t.Errorf("provider = %q, want %q", r.Source.Provider, localProvider)
	}
	if r.Source.URL != raw {
		t.Errorf("source URL = %q, want the drop-in path %q", r.Source.URL, raw)
	}
	if r.Note != "recorded in the garage" {
		t.Errorf("note = %q, want the supplied note", r.Note)
	}
	if r.Tier != wavref.TierInstrument {
		t.Errorf("tier = %q, want %q", r.Tier, wavref.TierInstrument)
	}
}

// f0/peak/rms/duration and the PCM hash must be MEASURED from the converted
// audio by the tool. Hand-written metadata is how upstream pitch labels
// ended up wrong repeatedly.
func TestRunAdopt_MeasuresAndHashesTheConvertedAudio(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 330.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	r := h.writtenRef(t, "snare-lofi-local")
	if r.Measured == nil {
		t.Fatal("Measured not populated")
	}
	if math.Abs(r.Measured.F0Hz-330.0) > 15.0 {
		t.Errorf("Measured.F0Hz = %.1f, want ~330 (the fixture's real frequency)", r.Measured.F0Hz)
	}
	want, err := hashFile(filepath.Join(h.refs, r.File))
	if err != nil {
		t.Fatalf("hashFile: %v", err)
	}
	if r.PCMSHA256 != want {
		t.Errorf("PCMSHA256 = %q, want %q recomputed from the written audio", r.PCMSHA256, want)
	}
}

// Adopting re-points the instrument: the previous reference loses it and is
// recorded as a reversible alternative.
func TestRunAdopt_RepointsTheInstrumentAndRecordsTheOldReference(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	for _, r := range h.written.Refs {
		if r.ID != "snare-lofi-local" && servesInstrument(r, roster.SnareLofi) {
			t.Errorf("ref %q still serves snare-lofi after adoption", r.ID)
		}
	}
	alts := h.writtenRef(t, "snare-lofi-local").Alternatives
	if len(alts) != 1 || !strings.Contains(alts[0].URL, "snare-vcsl") {
		t.Errorf("previous reference not recorded as an alternative: %+v", alts)
	}
}

// A family sibling sharing the old reference must keep it: adopting for one
// variant is not a decision about the others.
func TestRunAdopt_LeavesSiblingsOnTheirExistingReference(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare, roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	old := h.writtenRef(t, "snare-vcsl")
	if !servesInstrument(old, roster.Snare) {
		t.Errorf("sibling snare lost its reference: %v", old.Instruments)
	}
	if servesInstrument(old, roster.SnareLofi) {
		t.Errorf("adopted instrument still on the old reference: %v", old.Instruments)
	}
}

// An instrument whose catalogue entry says "no clean recording found" is
// the single best reason to adopt your own file, so it must be allowed --
// there is no existing real ref to re-point away from.
func TestRunAdopt_WorksForANeedsSourceInstrument(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{{
		ID: "clavinet-status", Tier: wavref.TierInstrument, Instruments: []instrument.ID{roster.Clavinet},
		Status: wavref.StatusNeedsSource, Note: "no licence-clean recording found",
	}})
	h.drop(t, "clavinet.wav", 440.0)

	if err := runAdopt(&h.out, baseOpts(roster.Clavinet, "clavinet.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if r := h.writtenRef(t, "clavinet-local"); !servesInstrument(r, roster.Clavinet) {
		t.Errorf("adopted ref does not serve clavinet: %v", r.Instruments)
	}
	// The status ref is the catalogue's record of WHY there was no audio;
	// it is not a candidate and must survive untouched.
	if s := h.writtenRef(t, "clavinet-status"); s.Status != wavref.StatusNeedsSource {
		t.Errorf("status ref was altered: %+v", s)
	}
}

// Re-adopting replaces the audio rather than erroring or accumulating a
// second ref -- you re-record a take and run the command again.
func TestRunAdopt_SecondAdoptionReplacesTheFirst(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "take1.wav", 220.0)
	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "take1.wav")); err != nil {
		t.Fatalf("first runAdopt: %v", err)
	}
	first := h.writtenRef(t, "snare-lofi-local").PCMSHA256

	// The second adoption loads the catalogue the first one wrote.
	prev := *h.written
	catalogueLoader = func() (wavref.Cat, error) { return prev, nil }
	h.drop(t, "take2.wav", 440.0)
	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "take2.wav")); err != nil {
		t.Fatalf("second runAdopt: %v", err)
	}

	n := 0
	for _, r := range h.written.Refs {
		if r.ID == "snare-lofi-local" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("re-adopting produced %d refs with the same id, want 1", n)
	}
	if got := h.writtenRef(t, "snare-lofi-local").PCMSHA256; got == first {
		t.Error("re-adopting did not re-measure: hash unchanged despite different audio")
	}
}

// A typo'd instrument id must fail loudly. Silently creating a reference
// for "snare2" would leave the real snare-lofi un-adopted and the mistake
// invisible until an audition session went looking for it.
func TestRunAdopt_RejectsAnInstrumentTheCatalogueDoesNotKnow(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	err := runAdopt(&h.out, baseOpts("snare2", "snare_hot.wav"))
	if err == nil {
		t.Fatal("runAdopt accepted an unknown instrument id")
	}
	if !strings.Contains(err.Error(), "snare2") {
		t.Errorf("error does not name the offending id: %v", err)
	}
	if h.written != nil {
		t.Error("a rejected adoption still wrote the catalogue")
	}
}

func TestRunAdopt_RejectsAMissingWavFile(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "not_there.wav")); err == nil {
		t.Fatal("runAdopt accepted a file that does not exist")
	}
	if h.written != nil {
		t.Error("a rejected adoption still wrote the catalogue")
	}
}

// The catalogue written back must satisfy the same structural invariants
// every read path enforces -- adoption must never be able to produce a file
// that the next `refs list` refuses to load.
func TestRunAdopt_WritesAStructurallyValidCatalogue(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare, roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if err := h.written.Validate(); err != nil {
		t.Errorf("written catalogue is structurally invalid: %v", err)
	}
}

// An explicit path outside the drop-in dir is honoured, so you can adopt
// straight out of wherever a recording landed.
func TestRunAdopt_AcceptsAnExplicitPathOutsideTheDropInDir(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	elsewhere := localFixtureWAV(t, t.TempDir(), "anywhere.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, elsewhere)); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if got := h.writtenRef(t, "snare-lofi-local").Source.URL; got != elsewhere {
		t.Errorf("source URL = %q, want %q", got, elsewhere)
	}
}

// -id lets one instrument keep several adopted takes side by side.
func TestRunAdopt_HonoursAnExplicitRefID(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	opts := baseOpts(roster.SnareLofi, "snare_hot.wav")
	opts.ID = "snare-lofi-garage-take3"
	if err := runAdopt(&h.out, opts); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if r := h.writtenRef(t, "snare-lofi-garage-take3"); !servesInstrument(r, roster.SnareLofi) {
		t.Errorf("explicit-id ref does not serve the instrument: %v", r.Instruments)
	}
}

// A ref id already used by a DIFFERENT instrument must not be silently
// hijacked -- that would strip the other instrument's ground truth.
func TestRunAdopt_RejectsAnIDBelongingToAnotherInstrument(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{
		refServing("snare-vcsl", roster.SnareLofi),
		refServing("cello-C2", roster.Cello),
	})
	h.drop(t, "snare_hot.wav", 220.0)

	opts := baseOpts(roster.SnareLofi, "snare_hot.wav")
	opts.ID = "cello-C2"
	if err := runAdopt(&h.out, opts); err == nil {
		t.Fatal("runAdopt hijacked a ref id belonging to another instrument")
	}
	if h.written != nil {
		t.Error("a rejected adoption still wrote the catalogue")
	}
}

// The trim window is what turns a 10-second take into a 2-second reference
// clip, so it has to reach the ffmpeg convert step.
func TestRunAdopt_AppliesTheTrimWindow(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	opts := baseOpts(roster.SnareLofi, "snare_hot.wav")
	opts.StartSec, opts.MaxSec = 0.1, 0.25
	if err := runAdopt(&h.out, opts); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	r := h.writtenRef(t, "snare-lofi-local")
	if r.Convert == nil {
		t.Fatal("no convert block recorded")
	}
	if r.Convert.StartSec != 0.1 || r.Convert.MaxSec != 0.25 {
		t.Errorf("convert window = %+v, want start 0.1 max 0.25", *r.Convert)
	}
	if math.Abs(r.Measured.DurationSec-0.25) > 0.05 {
		t.Errorf("converted duration = %.3fs, want ~0.25s -- the trim did not reach ffmpeg",
			r.Measured.DurationSec)
	}
}

// The listener has to be told what to do next; an adoption is only half a
// change until it has been heard.
func TestRunAdopt_TellsYouHowToHearTheResult(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.SnareLofi)})
	h.drop(t, "snare_hot.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare_hot.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if got := h.out.String(); !strings.Contains(got, "make ab INST=snare-lofi") {
		t.Errorf("output does not point at the A/B listen step:\n%s", got)
	}
}

// A note or credit containing spaces that reaches the CLI unquoted makes
// Go's flag package stop parsing at the first bare word: the note is
// silently truncated to one word AND every flag after it is silently
// dropped (a -max-sec past an unquoted -note never applies). That is the
// worst kind of failure -- it writes a wrong catalogue entry and reports
// success. Leftover positional arguments must be rejected instead.
func TestParseAdoptArgs_RejectsLeftoverPositionalArguments(t *testing.T) {
	_, err := parseAdoptArgs([]string{
		"-instrument", string(roster.SnareLofi), "-wav", "x.wav",
		"-note", "scratch", "test", "take", "-max-sec", "1.5",
	})
	if err == nil {
		t.Fatal("parseAdoptArgs accepted unparsed trailing arguments")
	}
	if !strings.Contains(err.Error(), "test") {
		t.Errorf("error does not show what was dropped: %v", err)
	}
}

// A properly quoted multi-word note is the normal case and must survive
// intact, along with every flag after it.
func TestParseAdoptArgs_KeepsAQuotedMultiWordNoteAndLaterFlags(t *testing.T) {
	opts, err := parseAdoptArgs([]string{
		"-instrument", string(roster.SnareLofi), "-wav", "x.wav",
		"-note", "scratch test take", "-max-sec", "1.5",
	})
	if err != nil {
		t.Fatalf("parseAdoptArgs: %v", err)
	}
	if opts.Note != "scratch test take" {
		t.Errorf("Note = %q, want the whole quoted note", opts.Note)
	}
	if opts.MaxSec != 1.5 {
		t.Errorf("MaxSec = %v, want 1.5 -- a flag after the note was dropped", opts.MaxSec)
	}
}

func TestParseAdoptArgs_RequiresWav(t *testing.T) {
	if _, err := parseAdoptArgs([]string{"-instrument", string(roster.SnareLofi)}); err == nil {
		t.Fatal("parseAdoptArgs accepted an invocation with no -wav")
	}
}

// The 18 registered ids the catalogue never mentions (snare-crack, kick-lofi,
// fm-bell-soft, ...) each carry their own factory recipe, so each is a distinct
// sound that deserves its own ground truth. Adoption is the ONLY way they
// ever get one, so it must not require the catalogue to have heard of them.
func TestRunAdopt_AcceptsARegisteredInstrumentTheCatalogueNeverMentions(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "snare-lofi.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "snare-lofi.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if r := h.writtenRef(t, "snare-lofi-local"); !servesInstrument(r, roster.SnareLofi) {
		t.Errorf("adopted ref does not serve snare-lofi: %v", r.Instruments)
	}
	// The base keeps its own reference: snare-lofi is a different sound, not
	// a re-pointing of snare.
	if base := h.writtenRef(t, "snare-vcsl"); !servesInstrument(base, roster.Snare) {
		t.Errorf("base snare lost its reference: %v", base.Instruments)
	}
}

// With the drop-in dir self-describing, -instrument is optional: the
// filename names the instrument.
func TestRunAdopt_DerivesTheInstrumentFromTheFilename(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "snare-lofi_garage_take3.wav", 220.0)

	opts := adoptOpts{Wav: "snare-lofi_garage_take3.wav"}
	if err := runAdopt(&h.out, opts); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if r := h.writtenRef(t, "snare-lofi-local"); !servesInstrument(r, roster.SnareLofi) {
		t.Errorf("filename-derived ref does not serve snare-lofi: %v", r.Instruments)
	}
}

// An explicit -instrument still wins, for a file whose name cannot say it.
func TestRunAdopt_ExplicitInstrumentOverridesTheFilename(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "take3.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts(roster.SnareLofi, "take3.wav")); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}
	if r := h.writtenRef(t, "snare-lofi-local"); !servesInstrument(r, roster.SnareLofi) {
		t.Errorf("explicit -instrument ignored: %v", r.Instruments)
	}
}

// An id that is not registered in the game must be refused even when the
// catalogue happens to mention it -- the roster is the authority.
func TestRunAdopt_RejectsAnIDTheGameDoesNotRegister(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("ghost-ref", "not-a-real-instrument")})
	h.drop(t, "x.wav", 220.0)

	if err := runAdopt(&h.out, baseOpts("not-a-real-instrument", "x.wav")); err == nil {
		t.Fatal("runAdopt accepted an instrument the game does not register")
	}
	if h.written != nil {
		t.Error("a rejected adoption still wrote the catalogue")
	}
}

// Bulk mode: drop a folder of takes, adopt everything whose name resolves,
// and report the rest instead of failing the whole run.
func TestRunAdoptScan_AdoptsEveryResolvableFileAndReportsTheRest(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "snare-lofi.wav", 220.0)
	h.drop(t, "kick-lofi.wav", 110.0)
	h.drop(t, "backing-track.wav", 330.0) // names no instrument

	if err := runAdoptScan(&h.out, adoptOpts{}); err != nil {
		t.Fatalf("runAdoptScan: %v", err)
	}

	for _, id := range []string{"snare-lofi-local", "kick-lofi-local"} {
		if r := h.writtenRef(t, id); r.PCMSHA256 == "" {
			t.Errorf("%s was not measured", id)
		}
	}
	out := h.out.String()
	if !strings.Contains(out, "backing-track.wav") {
		t.Errorf("scan did not report the unresolvable file:\n%s", out)
	}
	if !strings.Contains(out, "2 adopted") {
		t.Errorf("scan did not summarise what it did:\n%s", out)
	}
}

// A scan over an empty drop-in dir is a no-op that says so, not an error
// and not a catalogue write.
func TestRunAdoptScan_EmptyDropInDirWritesNothing(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})

	if err := runAdoptScan(&h.out, adoptOpts{}); err != nil {
		t.Fatalf("runAdoptScan: %v", err)
	}
	if h.written != nil {
		t.Error("an empty scan wrote the catalogue")
	}
	if !strings.Contains(h.out.String(), wavref.SourcesDir()) {
		t.Errorf("an empty scan does not say where it looked:\n%s", h.out.String())
	}
}

// REGRESSION: a scan must load the catalogue once, apply every adoption to
// that one value, and write once.
//
// The first implementation called a load-and-write runAdopt per file. Since
// loadCatalogue serves a MEMOIZED singleton, every file re-read the same
// starting catalogue and each write clobbered the previous one -- a scan of
// twenty takes persisted exactly one, reporting all twenty as adopted. The
// write count is the sharp edge of that bug, so it is asserted directly.
func TestRunAdoptScan_WritesOnceWithEveryAdoptionInIt(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "snare-lofi.wav", 220.0)
	h.drop(t, "kick-lofi.wav", 110.0)
	h.drop(t, "tom-lofi.wav", 165.0)

	if err := runAdoptScan(&h.out, adoptOpts{}); err != nil {
		t.Fatalf("runAdoptScan: %v", err)
	}

	if h.writes != 1 {
		t.Errorf("catalogue written %d times, want exactly 1", h.writes)
	}
	for _, id := range []string{"snare-lofi-local", "kick-lofi-local", "tom-lofi-local"} {
		r := h.writtenRef(t, id)
		if r.PCMSHA256 == "" {
			t.Errorf("%s survived the scan without measurements", id)
		}
	}
	// The pre-existing reference is untouched by any of it.
	if base := h.writtenRef(t, "snare-vcsl"); !servesInstrument(base, roster.Snare) {
		t.Errorf("scan disturbed an unrelated reference: %v", base.Instruments)
	}
}

// A scan that adopts nothing must not write at all -- rewriting the
// catalogue to an identical value churns the file for no reason.
func TestRunAdoptScan_WritesNothingWhenNoFileResolves(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "backing-track.wav", 220.0)

	if err := runAdoptScan(&h.out, adoptOpts{}); err != nil {
		t.Fatalf("runAdoptScan: %v", err)
	}
	if h.writes != 0 {
		t.Errorf("catalogue written %d times for a scan that adopted nothing, want 0", h.writes)
	}
}

// "Where do I put files?" should be answered by the directory existing. A
// scan over a missing drop-in dir creates it, so the printed path is
// somewhere you can immediately drop a file rather than a path you must
// first mkdir.
func TestRunAdoptScan_CreatesTheDropInDirWhenMissing(t *testing.T) {
	root := t.TempDir()
	t.Setenv(wavref.EnvVar, root)
	origLoader, origWriter := catalogueLoader, catalogueWriter
	t.Cleanup(func() { catalogueLoader, catalogueWriter = origLoader, origWriter })
	catalogueLoader = func() (wavref.Cat, error) { return wavref.Cat{}, nil }
	catalogueWriter = func(wavref.Cat) error { return nil }

	var out bytes.Buffer
	if err := runAdoptScan(&out, adoptOpts{}); err != nil {
		t.Fatalf("runAdoptScan: %v", err)
	}

	if fi, err := os.Stat(wavref.SourcesDir()); err != nil || !fi.IsDir() {
		t.Errorf("drop-in dir not created at %s: %v", wavref.SourcesDir(), err)
	}
}

// The "hear it" hint has to name the instrument that was actually adopted,
// which is not the same as the one the caller passed: with the drop-in dir
// self-describing, -instrument is usually empty and the id comes from the
// filename. Resolving it into a COPY of the options left the hint printing
// a bare `make ab INST=`, so the one line telling you how to audition the
// take you just adopted named nothing at all.
func TestRunAdopt_HearItHintNamesTheFilenameDerivedInstrument(t *testing.T) {
	h := newAdoptHarness(t, []wavref.Ref{refServing("snare-vcsl", roster.Snare)})
	h.drop(t, "snare-lofi_garage_take3.wav", 220.0)

	opts := adoptOpts{Wav: "snare-lofi_garage_take3.wav"}
	if err := runAdopt(&h.out, opts); err != nil {
		t.Fatalf("runAdopt: %v", err)
	}

	if got := h.out.String(); !strings.Contains(got, "make ab INST=snare-lofi") {
		t.Errorf("hint does not name the derived instrument:\n%s", got)
	}
}
