//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// hashFile decodes a WAV and returns its PCM hash. Decoding lives here, not
// in internal/wavref, because wavref importing internal/audio would close an
// import cycle with that package's own tests.
func hashFile(path string) (string, error) {
	samples, rate, err := audio.DecodeWAVAtRate(path, wavref.Rate)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", path, err)
	}
	return wavref.PCMHash(samples, rate), nil
}

// runVerify hashes local audio against the catalogue. id == "" verifies
// every ref. A missing file is reported, not an error: absence is a fetch
// concern. A hash mismatch IS an error — the audio changed under us.
func runVerify(w io.Writer, id string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	var refs []wavref.Ref
	if id != "" {
		r, ok := c.Ref(id)
		if !ok {
			return fmt.Errorf("refs verify: unknown ref %q", id)
		}
		refs = []wavref.Ref{r}
	} else {
		refs = append(refs, c.Refs...)
		sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	}

	var verified, noHash, missing, decodeErrs, bad int
	for _, r := range refs {
		if r.Status != "" {
			continue
		}
		p, found := wavref.Resolve(r.ID)
		if !found {
			fmt.Fprintf(w, "missing  %s (%s)\n", r.ID, r.File)
			missing++
			continue
		}
		got, err := hashFile(p)
		if err != nil {
			fmt.Fprintf(w, "ERROR    %s\n  %v\n", r.ID, err)
			decodeErrs++
			continue
		}
		if err := wavref.VerifyHash(r.ID, got); err != nil {
			fmt.Fprintf(w, "MISMATCH %s\n  %v\n  file %s\n", r.ID, err, p)
			bad++
			continue
		}
		// wavref.VerifyHash returns nil both when the hash matched AND when
		// the ref carries no recorded pcm_sha256 (it short-circuits — see
		// internal/wavref/resolve.go verifyRefHash). Those are NOT the same
		// outcome: one actually checked the audio, the other checked
		// nothing. Distinguish them so "N ok" can't silently mean "N never
		// hashed".
		if r.PCMSHA256 == "" {
			fmt.Fprintf(w, "ok       %s (no hash yet)\n", r.ID)
			noHash++
			continue
		}
		fmt.Fprintf(w, "ok       %s\n", r.ID)
		verified++
	}
	fmt.Fprintf(w, "\n%d verified, %d no-hash, %d missing, %d mismatched, %d decode error(s)\n",
		verified, noHash, missing, bad, decodeErrs)
	if bad > 0 || decodeErrs > 0 {
		return fmt.Errorf("refs verify: %d mismatched, %d decode error(s)", bad, decodeErrs)
	}
	return nil
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	id := fs.String("id", "", "verify a single ref id (default: all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runVerify(os.Stdout, *id)
}
