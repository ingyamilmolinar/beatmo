//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// a3Hz is the pitch origin every Beatmo semitone offset is measured from.
const a3Hz = 220.0

// ffmpegArgs builds a deterministic conversion command. -map_metadata -1
// and -bitexact strip source tags and encoder/timestamp tags, so repeated
// conversions of the same input produce the same file.
func ffmpegArgs(in, out string, cv wavref.Convert) []string {
	args := []string{"-nostdin", "-y", "-loglevel", "error"}
	if cv.StartSec > 0 {
		// -ss before -i: seek the input rather than decode-and-discard.
		args = append(args, "-ss", strconv.FormatFloat(cv.StartSec, 'g', -1, 64))
	}
	args = append(args, "-i", in,
		"-map_metadata", "-1",
		"-bitexact",
		"-ac", strconv.Itoa(cv.Channels),
		"-ar", strconv.Itoa(cv.Rate),
		"-sample_fmt", sampleFmt(cv.Bits),
	)
	if cv.MaxSec > 0 {
		args = append(args, "-t", strconv.FormatFloat(cv.MaxSec, 'g', -1, 64))
	}
	return append(args, out)
}

func sampleFmt(bits int) string {
	if bits == 32 {
		return "s32"
	}
	return "s16"
}

// semisFromA3 converts a detected fundamental to Beatmo's semitone offset
// from A3=220Hz. An undetected pitch (0) reports the origin.
func semisFromA3(f0 float64) int {
	if f0 <= 0 {
		return 0
	}
	return int(math.Round(12 * math.Log2(f0/a3Hz)))
}

// measureFile derives every `measured` field FROM THE AUDIO. Upstream
// filename and metadata pitch labels have been wrong repeatedly, so they
// are never consulted.
func measureFile(path string) (wavref.Measured, error) {
	samples, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
	if err != nil {
		return wavref.Measured{}, fmt.Errorf("decode %s: %w", path, err)
	}
	w := wave.Wave{Samples: make([]float64, len(samples)), SampleRate: rate, Label: filepath.Base(path)}
	for i, s := range samples {
		w.Samples[i] = float64(s)
	}
	return measureWave(w), nil
}

// measureWave is measureFile's shared core, operating on samples already in
// memory rather than a path on disk. `refs compare` (compare.go) needs this
// directly: a synth render never touches disk before it is measured, so
// there is no path to hand measureFile.
func measureWave(w wave.Wave) wavref.Measured {
	var peak, sumsq float64
	for _, v := range w.Samples {
		if a := math.Abs(v); a > peak {
			peak = a
		}
		sumsq += v * v
	}
	rms := 0.0
	if len(w.Samples) > 0 {
		rms = math.Sqrt(sumsq / float64(len(w.Samples)))
	}
	f0 := fingerprint.DetectF0(w)
	return wavref.Measured{
		F0Hz:        f0,
		SemisFromA3: semisFromA3(f0),
		Peak:        peak,
		RMS:         rms,
		DurationSec: w.Duration(),
	}
}

// destDir picks where a ref's audio lands: the one reference-audio root.
// Reference audio is never written into the repository.
func destDir(wavref.Ref) (string, error) {
	d := wavref.Root()
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func download(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// Some hosts (archive.org, github raw) reject an empty UA.
	req.Header.Set("User-Agent", "beatmo-refs/1.0")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

type fetchOpts struct {
	ID          string
	Instrument  instrument.ID
	Tier        string
	OnlyMissing bool
	Force       bool
	// SkipAbsentLocal passes over a ref adopted from a file that is not on
	// this machine instead of failing: CI fetches the public corpus only.
	SkipAbsentLocal bool
}

func (o fetchOpts) selects(r wavref.Ref) bool {
	if r.Status != "" {
		return false // nothing to fetch for synthetic / needs-source
	}
	if o.ID != "" && r.ID != o.ID {
		return false
	}
	if o.Tier != "" && r.Tier != o.Tier {
		return false
	}
	if o.Instrument != "" {
		found := false
		for _, i := range r.Instruments {
			if i == o.Instrument {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if o.OnlyMissing {
		if _, ok := wavref.Resolve(r.ID); ok {
			return false
		}
	}
	return true
}

// localProvider is the Source.Provider value that means "the audio is
// already a local file, not something to HTTP-GET". Documented on
// wavref.Source but never wired up until now -- see fetchOneLocal.
const localProvider = "local"

// fetchOne downloads, converts, measures and hashes a single ref, updating
// it in place. It returns false when the ref was skipped as already good.
func fetchOne(w io.Writer, r *wavref.Ref, force bool) (bool, error) {
	if !force && r.PCMSHA256 != "" {
		if p, ok := wavref.Resolve(r.ID); ok {
			if got, err := hashFile(p); err == nil && got == r.PCMSHA256 {
				fmt.Fprintf(w, "skip     %s (already matches recorded hash)\n", r.ID)
				return false, nil
			}
		}
	}

	dir, err := destDir(*r)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}
	out := filepath.Join(dir, r.File)

	if r.Source.Provider == localProvider {
		return fetchOneLocal(w, r, out)
	}

	tmp, err := os.MkdirTemp("", "refs-fetch-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)

	raw := filepath.Join(tmp, "raw"+filepath.Ext(r.Source.URL))
	fmt.Fprintf(w, "get      %s\n", r.ID)
	if err := download(r.Source.URL, raw); err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}

	cv := wavref.Convert{Channels: 1, Rate: wavref.Rate, Bits: 16, MaxSec: 2.0}
	if r.Convert != nil {
		cv = *r.Convert
	}
	cmd := exec.Command("ffmpeg", ffmpegArgs(raw, out, cv)...)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("%s: ffmpeg: %w\n%s", r.ID, err, outBytes)
	}

	return fetchOneFinish(w, r, out)
}

// fetchOneLocal handles Source.Provider == "local": the audio already lives
// on disk, so there is nothing to HTTP-GET. This is the recovery route for
// a lossy freesound-preview entry: drop the lossless original on disk,
// point Source.URL at it, flip Provider to "local", and re-fetch -- no
// further code change needed.
//
//   - Source.URL non-empty: treated as a filesystem path to the RAW
//     source. It goes through the SAME ffmpeg convert step as every other
//     provider, so a local ref gets identical treatment (deterministic
//     trim/resample/bitdepth) and a real, tool-computed pcm_sha256 --
//     never hand-written.
//   - Source.URL empty: the destination file is assumed to be ALREADY the
//     final, correctly-converted audio (no raw source to convert from), so
//     it is measured and hashed in place without invoking ffmpeg.
//
// Either way it is an error, not a silent skip, when the expected file
// does not exist -- a missing local source is a configuration mistake the
// caller needs to see.
func fetchOneLocal(w io.Writer, r *wavref.Ref, out string) (bool, error) {
	fmt.Fprintf(w, "get      %s (local)\n", r.ID)

	switch {
	case r.Source.URL != "":
		if _, err := os.Stat(r.Source.URL); err != nil {
			return false, fmt.Errorf("%s: local source: %w", r.ID, err)
		}
		cv := wavref.Convert{Channels: 1, Rate: wavref.Rate, Bits: 16, MaxSec: 2.0}
		if r.Convert != nil {
			cv = *r.Convert
		}
		cmd := exec.Command("ffmpeg", ffmpegArgs(r.Source.URL, out, cv)...)
		if outBytes, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Errorf("%s: ffmpeg: %w\n%s", r.ID, err, outBytes)
		}
	default:
		if _, err := os.Stat(out); err != nil {
			return false, fmt.Errorf("%s: local provider with no source.url requires the audio to already be in place at %s: %w", r.ID, out, err)
		}
	}

	return fetchOneFinish(w, r, out)
}

// fetchOneFinish is the tail shared by every provider once `out` holds the
// final, already-converted audio: measure, hash, record, log.
func fetchOneFinish(w io.Writer, r *wavref.Ref, out string) (bool, error) {
	m, err := measureFile(out)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}
	h, err := hashFile(out)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}
	r.Measured = &m
	r.PCMSHA256 = h
	fmt.Fprintf(w, "ok       %s  f0=%.1fHz (%+d semis from A3)  %.2fs  peak=%.3f\n",
		r.ID, m.F0Hz, m.SemisFromA3, m.DurationSec, m.Peak)
	return true, nil
}

// catalogueLoader is how runFetch obtains the catalogue. It defaults to
// loadCatalogue (the same schema+structural-Validate gate list/verify/doctor
// use) rather than wavref.Catalogue() directly: fetch is the ONE command
// that rewrites catalogue.json, so it is precisely the one that must refuse
// to operate on (and launder) a structurally invalid file. A package-level
// var rather than a hardcoded call so the refusal path is exercisable in
// tests without corrupting the embedded catalogue.json.
//
// catalogueWriter is how BOTH catalogue-mutating commands -- runFetch here
// and runChoose in candidates.go -- persist a mutated catalogue. It defaults
// to writeCatalogue (which rewrites the real, tracked catalogue.json in the
// source tree) but is a package-level var, kept next to catalogueLoader as
// the pair of seams that guard the one file every other command
// (list/verify/doctor/candidates) only ever reads, precisely so tests can
// redirect it to a throwaway path instead of mutating the committed file as
// a side effect of running `go test ./cmd/refs/`. fetch used to call
// writeCatalogue directly, which was safe only by omission -- no test drove
// it into its changed>0 write branch. The moment one did (see
// TestRunFetch_WritesThroughCatalogueWriterSeam), an unguarded direct call
// would have silently rewritten catalogue.json on every test run.
var (
	catalogueLoader = loadCatalogue
	catalogueWriter = writeCatalogue
)

// runFetch fetches every selected ref and writes the updated catalogue back.
func runFetch(w io.Writer, opts fetchOpts) error {
	c, err := catalogueLoader()
	if err != nil {
		return err
	}
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })

	changed := 0
	var failures []error
	for i := range refs {
		if !opts.selects(refs[i]) {
			continue
		}
		if opts.SkipAbsentLocal && localSourceAbsent(refs[i]) {
			fmt.Fprintf(w, "local-only %s (adopted from a file that is not on this machine)\n", refs[i].ID)
			continue
		}
		did, err := fetchOne(w, &refs[i], opts.Force)
		if err != nil {
			fmt.Fprintf(w, "FAIL     %v\n", err)
			failures = append(failures, err)
			continue
		}
		if did {
			changed++
		}
	}

	if changed > 0 {
		c.Refs = refs
		if err := catalogueWriter(c); err != nil {
			return err
		}
		fmt.Fprintf(w, "\nupdated catalogue.json (%d ref(s))\n", changed)
	}
	if len(failures) > 0 {
		return fmt.Errorf("refs fetch: %d ref(s) failed", len(failures))
	}
	return nil
}

// localSourceAbsent reports a locally adopted ref whose audio can be made on
// no machine but the one holding its source file.
func localSourceAbsent(r wavref.Ref) bool {
	if !locallyAdopted(r) {
		return false
	}
	if r.Source.URL != "" {
		if _, err := os.Stat(r.Source.URL); err == nil {
			return false
		}
	}
	dir, err := destDir(r)
	if err != nil {
		return true
	}
	_, err = os.Stat(filepath.Join(dir, r.File))
	return err != nil
}

// writeCatalogue rewrites catalogue.json in the source tree. The file is
// embedded, so a rebuild picks up the change.
func writeCatalogue(c wavref.Cat) error {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return fmt.Errorf("cannot locate the module root to write catalogue.json")
	}
	path := filepath.Join(root, "internal", "wavref", "catalogue.json")
	return writeCatalogueAtomic(path, c)
}

// writeCatalogueAtomic writes c as indented JSON to path via a same-directory
// temp file + rename, mirroring the atomic-rename convention already used by
// internal/userprefs' stores: a crash, kill, or full disk mid-write can
// never truncate the single source of truth, because the rename either
// hasn't happened yet (old file intact) or already has (new file intact).
// Split out from writeCatalogue so it's testable against a throwaway path
// instead of the real catalogue.json.
func writeCatalogueAtomic(path string, c wavref.Cat) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	// Same directory as the target so the rename stays on one filesystem.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp) // don't litter a half-finished write on failure
		return err
	}
	return nil
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	var o fetchOpts
	fs.StringVar(&o.ID, "id", "", "fetch a single ref id")
	fs.StringVar((*string)(&o.Instrument), "instrument", "", "fetch refs for one instrument")
	fs.StringVar(&o.Tier, "tier", "", "fetch one tier: instrument|song")
	fs.BoolVar(&o.OnlyMissing, "only-missing", false, "skip refs whose audio is already present")
	fs.BoolVar(&o.Force, "force", false, "re-fetch even when the local hash already matches")
	fs.BoolVar(&o.SkipAbsentLocal, "skip-absent-local", false, "pass over refs adopted from a file not on this machine instead of failing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runFetch(os.Stdout, o)
}
