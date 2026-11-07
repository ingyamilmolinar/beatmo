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

// The "-local" tail shows in refs list / refs families that the reference came off someone's disk.
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
	// Free text. No licence or credit: reference audio is never committed or shipped, so there is nothing to license.
	Note string
	// F0Hz > 0 pins Measured.F0Hz instead of the detector (YIN's sub-harmonic on struck sources).
	F0Hz float64
	// StartSec / MaxSec are the deterministic trim window applied by
	// ffmpeg, the same one every catalogued reference carries.
	StartSec float64
	MaxSec   float64
	// Scan adopts every resolvable file in the drop-in dir instead of one
	// named -wav.
	Scan bool
}

// A new slice: the caller's can alias the memoized catalogue singleton.
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

// adoptInto applies ONE adoption to c without loading or writing, so a scan loads once and writes once (each
// load-and-write re-read the memoized catalogue and clobbered the last). The audio is converted like every catalogued
// reference and measured BY THE TOOL; the instrument is returned because opts arrives by value.
func adoptInto(w io.Writer, c wavref.Cat, opts adoptOpts) (wavref.Cat, string, error) {
	// Before touching the catalogue, so a missing file or unresolvable name never leaves a half-applied adoption.
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
	// The roster is the authority, not the catalogue: adoption is the only way an id the catalogue never mentions gets a
	// reference.
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

	// force: an explicit adoption always re-converts; skipping on a matching hash would ignore a re-recorded take.
	if _, err := fetchOne(w, &ref, true); err != nil {
		return c, "", fmt.Errorf("refs adopt: %w", err)
	}

	// An explicit -f0 overrides the detector: YIN reports a sub-harmonic on struck sources, and refs compare renders the
	// synth AT Measured.F0Hz, so an octave-wrong reading auditions the instrument an octave low.
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

// runAdoptScan adopts every drop-in whose name resolves to a registered instrument and REPORTS the rest: one unnamed
// file must not cost twenty adoptions. A broken adoption of a resolvable file still aborts.
func runAdoptScan(w io.Writer, opts adoptOpts) error {
	files, err := wavref.ListSources()
	if err != nil {
		return err
	}
	if len(files) == 0 {
		// So the printed path is somewhere you can drop a file right now.
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

// Leftover positional arguments are an error: an unquoted multi-word -note stops Go's flag parsing at the first bare
// word, truncating the note and dropping every later flag, and the command would write a wrong entry and report success.
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
			"from the first bare word on is dropped, including later flags.",
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
