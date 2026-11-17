//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// adoptMaxSecDefault is the clip length an adopted recording is trimmed to
// when -max-sec is not given. It matches fetchOne's own default so an
// adopted reference is the same shape as a fetched one.
const adoptMaxSecDefault = 2.0

// adoptIDSuffix builds the default ref id for an adopted recording. The
// "-local" tail is what makes it obvious in `refs list` / `refs families`
// that this reference came off someone's disk rather than a catalogued
// upstream corpus.
func adoptIDSuffix(inst instrument.ID) string { return string(inst) + "-local" }

// adoptOpts is one `refs adopt` invocation.
type adoptOpts struct {
	// Instrument is the roster id whose reference is being replaced
	// ("snare-lofi", "kick", ...). Empty derives it from the filename.
	Instrument instrument.ID
	// Wav is a bare filename inside $BEATMO_WAV_SOURCES, or any path.
	Wav string
	// ID overrides the generated ref id, so one instrument can keep
	// several adopted takes side by side.
	ID string
	// Note is free text recorded in the catalogue. There is no licence or
	// credit: reference audio is local dev tooling, never committed and
	// never shipped, so there is nothing to license to anyone.
	Note string
	// F0Hz, when > 0, pins Measured.F0Hz instead of trusting the detector.
	// See adoptInto for why this exists (YIN octave artifact on struck
	// sources).
	F0Hz float64
	// StartSec / MaxSec are the deterministic trim window applied by
	// ffmpeg, the same one every catalogued reference carries.
	StartSec float64
	MaxSec   float64
	// Scan adopts every resolvable file in the drop-in dir instead of one
	// named -wav.
	Scan bool
}

// upsertRef replaces the ref carrying newRef's id, or appends it. Returns
// the new slice without mutating the caller's (which can alias the memoized
// catalogue singleton -- see repointInstrument).
func upsertRef(refs []wavref.Ref, newRef wavref.Ref) []wavref.Ref {
	out := make([]wavref.Ref, 0, len(refs)+1)
	replaced := false
	for _, r := range refs {
		if r.ID == newRef.ID {
			out, replaced = append(out, newRef), true
			continue
		}
		out = append(out, r)
	}
	if !replaced {
		out = append(out, newRef)
	}
	return out
}

// runAdopt points one instrument at a recording of your own.
//
// The file is NOT trusted as-is and NOT copied into the tree: it goes
// through the identical ffmpeg convert step every catalogued reference
// goes through (mono / 44.1 kHz / 16-bit / trimmed), lands in
// $BEATMO_WAV_REFS, and is then measured and hashed BY THE TOOL. f0, peak,
// rms, duration and pcm_sha256 are never hand-written -- upstream pitch
// labels have been wrong often enough that measuring is the only thing
// worth believing.
//
// What is committed is the catalogue entry: where the audio came from, with
// what trim, and what it measured. The recording itself stays outside the
// repo -- no audio file is ever versioned (TestNoAudioFilesInRepository).
// adoptInto applies ONE adoption to c and returns the updated catalogue
// along with the instrument it was applied to. It neither loads nor writes:
// the caller owns that, which is what lets a bulk scan load once, apply
// many, and write once.
//
// The instrument is RETURNED rather than left on opts because opts arrives
// by value: resolving a filename-derived instrument into the local copy
// left every caller still holding the empty one it passed in.
//
// Splitting this out fixed a real bug. When runAdoptScan called a
// load-and-write runAdopt per file, every adoption re-read the SAME starting
// catalogue -- loadCatalogue serves a memoized singleton -- so each write
// clobbered the last and a scan of twenty takes persisted exactly one.
func adoptInto(w io.Writer, c wavref.Cat, opts adoptOpts) (wavref.Cat, string, error) {
	// Resolve the audio and the instrument BEFORE touching the catalogue,
	// so a missing file or an unresolvable name can never leave a
	// half-applied adoption.
	src, err := wavref.SourcePath(opts.Wav)
	if err != nil {
		return c, "", fmt.Errorf("refs adopt: %w", err)
	}
	inst := opts.Instrument
	if inst == "" {
		// The drop-in directory is self-describing: snare-lofi.wav is a
		// snare-lofi reference.
		if inst, err = instrumentFromFilename(opts.Wav); err != nil {
			return c, "", err
		}
	}
	// The GAME's registry is the authority, not the catalogue. 18 registered
	// ids (snare-crack, kick-lofi, fm-bell-soft, ...) are never mentioned by
	// catalogue.json, and adoption is the only way any of them ever gets a
	// reference -- validating against the catalogue would lock them out of
	// the one command that can fix them.
	if !slices.Contains(rosterIDs(), inst) {
		msg := fmt.Sprintf("refs adopt: %q is not a registered instrument", inst)
		if near := nearestRosterIDs(string(inst), 5); len(near) > 0 {
			msg += fmt.Sprintf("; did you mean %s?", strings.Join(instrument.Strings(near), ", "))
		}
		return c, "", fmt.Errorf("%s (see `refs families` for every id)", msg)
	}
	opts.Instrument = inst

	id := opts.ID
	if id == "" {
		id = adoptIDSuffix(opts.Instrument)
	}
	// Re-adopting the same id is the normal "I recorded a better take"
	// path. Taking over an id that belongs to a DIFFERENT instrument is
	// not: it would strip that instrument's ground truth as a side effect.
	if existing, ok := c.Ref(id); ok {
		for _, i := range existing.Instruments {
			if i != opts.Instrument {
				return c, "", fmt.Errorf("refs adopt: ref id %q is already the reference for %s; "+
					"pass -id to name a new one", id, strings.Join(instrument.Strings(existing.Instruments), ", "))
			}
		}
	}

	maxSec := opts.MaxSec
	if maxSec <= 0 {
		maxSec = adoptMaxSecDefault
	}
	ref := wavref.Ref{
		ID:          id,
		Tier:        wavref.TierInstrument,
		Instruments: []instrument.ID{opts.Instrument},
		File:        id + ".wav",
		Note:        opts.Note,
		Source: &wavref.Source{
			Provider: localProvider,
			URL:      src,
		},
		Convert: &wavref.Convert{
			Channels: 1, Rate: wavref.Rate, Bits: 16,
			StartSec: opts.StartSec, MaxSec: maxSec,
		},
	}

	// force=true: an explicit adoption always re-converts and re-measures.
	// Skipping on a matching hash would silently ignore a re-recorded take.
	if _, err := fetchOne(w, &ref, true); err != nil {
		return c, "", fmt.Errorf("refs adopt: %w", err)
	}

	// An explicit -f0 overrides the detector. YIN reports a sub-harmonic on
	// struck sources often enough that this matters: the acoustic snare whose
	// modes sit at 225/355/520 Hz measures 113.8 Hz, exactly half. Everything
	// downstream trusts Measured.F0Hz -- `refs compare` renders the synth AT
	// that pitch -- so an octave-wrong measurement silently auditions the
	// instrument an octave low and makes every comparison meaningless.
	if opts.F0Hz > 0 && ref.Measured != nil {
		ref.Measured.F0Hz = opts.F0Hz
		ref.Measured.SemisFromA3 = semisFromA3(opts.F0Hz)
		fmt.Fprintf(w, "pinned   %s  f0=%.1fHz (%+d semis from A3) by -f0\n",
			ref.ID, ref.Measured.F0Hz, ref.Measured.SemisFromA3)
	}

	reason := fmt.Sprintf("superseded for %s by the adopted local recording %s", opts.Instrument, id)
	nAlts := countAlternatives(c.Refs, opts.Instrument, id)
	c.Refs = repointInstrument(upsertRef(c.Refs, ref), opts.Instrument, id, reason)

	fmt.Fprintf(w, "adopted %s as the reference for %s (%d previous reference(s) kept as alternatives)\n",
		id, opts.Instrument, nAlts)
	return c, string(opts.Instrument), nil
}

// runAdopt is the single-file command: load, apply, validate, write.
func runAdopt(w io.Writer, opts adoptOpts) error {
	c, err := catalogueLoader()
	if err != nil {
		return err
	}
	c, inst, err := adoptInto(w, c, opts)
	if err != nil {
		return err
	}
	if err := writeAdopted(c); err != nil {
		return err
	}
	fmt.Fprintf(w, "hear it:  make ab INST=%s\n", inst)
	return nil
}

// writeAdopted refuses to persist a catalogue the next `refs list` would
// reject, then writes it through the shared seam.
func writeAdopted(c wavref.Cat) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("refs adopt: refusing to write an invalid catalogue: %w", err)
	}
	return catalogueWriter(c)
}

// runAdoptScan adopts every file waiting in the drop-in directory whose
// name resolves to a registered instrument, and REPORTS the ones that do
// not instead of failing the run. Dropping a folder of takes and running
// one command is the bulk workflow; a single unnamed file in that folder
// must not cost you the other twenty adoptions.
//
// A genuine failure on a resolvable file (unreadable audio, ffmpeg error)
// still aborts: that is a broken adoption, not an unclaimed file.
func runAdoptScan(w io.Writer, opts adoptOpts) error {
	files, err := wavref.ListSources()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		// Create it, so the path printed below is somewhere you can drop
		// a file right now rather than one you have to mkdir first.
		if err := os.MkdirAll(wavref.SourcesDir(), 0o755); err != nil {
			return err
		}
		fmt.Fprintf(w, "no audio waiting in %s\n", wavref.SourcesDir())
		fmt.Fprintf(w, "drop files there named after the instrument they are for (snare-lofi.wav, kick-deep_take3.wav)\n")
		return nil
	}

	c, err := catalogueLoader()
	if err != nil {
		return err
	}

	var adopted, skipped []string
	var insts []string
	for _, f := range files {
		one := opts
		one.Wav = f
		// Per-file id, so a scan never collapses two takes onto one ref.
		one.ID = ""
		one.Instrument = ""
		// Resolving here is what CLASSIFIES the file: an unresolvable
		// name is an unclaimed drop-in to report, not a failed run.
		if _, err := instrumentFromFilename(f); err != nil {
			skipped = append(skipped, fmt.Sprintf("  %-40s %v", filepath.Base(f), err))
			continue
		}
		// Thread the catalogue through: each adoption builds on the last,
		// so one write at the end carries every one of them.
		inst, err := "", error(nil)
		if c, inst, err = adoptInto(w, c, one); err != nil {
			return err
		}
		adopted = append(adopted, filepath.Base(f))
		insts = append(insts, inst)
	}

	if len(adopted) > 0 {
		if err := writeAdopted(c); err != nil {
			return err
		}
	}

	fmt.Fprintf(w, "\n%d adopted, %d skipped (of %d in %s)\n",
		len(adopted), len(skipped), len(files), wavref.SourcesDir())
	if len(skipped) > 0 {
		fmt.Fprintln(w, "skipped -- rename these after the instrument they are for, or adopt them with -instrument:")
		for _, l := range skipped {
			fmt.Fprintln(w, l)
		}
	}
	if len(insts) > 0 {
		fmt.Fprintf(w, "hear them:  %s\n", strings.Join(abCommands(insts), "  "))
	}
	return nil
}

// abCommands renders the A/B listen command for each adopted instrument.
func abCommands(insts []string) []string {
	out := make([]string, 0, len(insts))
	for _, i := range insts {
		out = append(out, "make ab INST="+i)
	}
	return out
}

// parseAdoptArgs turns a command line into adoptOpts.
//
// It is split out of cmdAdopt so the ARGUMENT handling is testable, because
// one failure mode here is silent and destructive: a note that
// reaches the CLI unquoted ("-note scratch test take") makes Go's flag
// package stop parsing at the first bare word, so the note is truncated to
// one word AND every flag after it is dropped -- a -max-sec past an
// unquoted -note simply never applies. The command then writes a wrong
// catalogue entry and reports success. Leftover positional arguments are
// therefore an error, naming what was dropped.
func parseAdoptArgs(args []string) (adoptOpts, error) {
	fs := flag.NewFlagSet("adopt", flag.ExitOnError)
	var opts adoptOpts
	fs.StringVar((*string)(&opts.Instrument), "instrument", "", "instrument id to re-point (see `refs families`)")
	fs.StringVar(&opts.Wav, "wav", "", "WAV filename inside the drop-in dir ($BEATMO_WAV_REFS/sources), or any path")
	fs.StringVar(&opts.ID, "id", "", "ref id to create (default <instrument>-local)")
	fs.StringVar(&opts.Note, "note", "", "free-text note recorded in the catalogue")
	fs.Float64Var(&opts.F0Hz, "f0", 0, "pin the measured fundamental in Hz, overriding the detector (for the documented YIN octave artifact: a struck/percussive source often measures an octave low)")
	fs.Float64Var(&opts.StartSec, "start-sec", 0, "seconds to skip at the head of the source")
	fs.Float64Var(&opts.MaxSec, "max-sec", adoptMaxSecDefault, "seconds of audio to keep")
	scan := fs.Bool("scan", false, "adopt every resolvable file waiting in the drop-in dir")
	if err := fs.Parse(args); err != nil {
		return adoptOpts{}, err
	}
	opts.Scan = *scan
	if rest := fs.Args(); len(rest) > 0 {
		return adoptOpts{}, fmt.Errorf("refs adopt: unparsed argument(s): %s\n"+
			"  A multi-word -note must be quoted; without quotes everything "+
			"from the first bare word on is dropped, including later flags",
			strings.Join(rest, " "))
	}
	if opts.Wav == "" && !opts.Scan {
		return adoptOpts{}, fmt.Errorf("refs adopt: -wav is required " +
			"(a filename inside the drop-in dir, or any path); or -scan to adopt everything waiting there")
	}
	return opts, nil
}

func cmdAdopt(args []string) error {
	opts, err := parseAdoptArgs(args)
	if err != nil {
		return err
	}
	if opts.Scan {
		return runAdoptScan(os.Stdout, opts)
	}
	return runAdopt(os.Stdout, opts)
}
