//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

type covRow struct {
	Instrument instrument.ID
	State      string // covered | synthetic | needs-source | unclassified
	RefID      string
	Note       string
}

// coverage classifies every base instrument in the builtin roster against
// the catalogue.
func coverage() ([]covRow, error) {
	c, err := loadCatalogue()
	if err != nil {
		return nil, err
	}

	// The roster IS the set of instruments to report. There is no folding:
	// every registered id is a distinct sound and gets its own row.
	bases := map[instrument.ID]bool{}
	for _, id := range roster.All {
		bases[id] = true
	}

	rows := make([]covRow, 0, len(bases))
	for inst := range bases {
		row := covRow{Instrument: inst, State: "unclassified"}
		for _, r := range c.ForInstrument(inst) {
			switch r.Status {
			case "":
				row.State, row.RefID, row.Note = "covered", r.ID, ""
			case wavref.StatusSynthetic:
				row.State, row.RefID, row.Note = "synthetic", r.ID, r.Note
			case wavref.StatusNeedsSource:
				row.State, row.RefID, row.Note = "needs-source", r.ID, r.Note
			}
			if row.State == "covered" {
				break // a real reference outranks any status entry
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Instrument < rows[j].Instrument })
	return rows, nil
}

// runDoctor writes the coverage report. Output is deterministic so it can
// be pinned as a golden.
func runDoctor(w io.Writer) error {
	rows, err := coverage()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "INSTRUMENT\tSTATE\tREF\tNOTE")
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.State]++
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Instrument, r.State, dashIfEmpty(r.RefID), dashIfEmpty(r.Note))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	stems, err := templateStems()
	if err != nil {
		return err
	}
	var withRef, withoutRef []string
	for _, s := range stems {
		if _, ok := c.ForTemplate(s); ok {
			withRef = append(withRef, s)
		} else {
			withoutRef = append(withoutRef, s)
		}
	}

	fmt.Fprintf(w, "\ninstruments: %d covered, %d synthetic, %d needs-source, %d unclassified (of %d)\n",
		counts["covered"], counts["synthetic"], counts["needs-source"], counts["unclassified"], len(rows))
	fmt.Fprintf(w, "templates:   %d with a song reference, %d without (of %d)\n",
		len(withRef), len(withoutRef), len(stems))
	if len(withoutRef) > 0 {
		fmt.Fprintf(w, "  no song reference: %s\n", strings.Join(withoutRef, " "))
	}
	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// templateStems lists the committed template stems from the assets dir.
func templateStems() ([]string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return nil, fmt.Errorf("refs doctor: cannot locate the module root (no go.mod above cwd)")
	}
	dir := filepath.Join(root, "internal", "assets", "templates")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("refs doctor: read templates: %w", err)
	}
	var out []string
	for _, e := range ents {
		if n := e.Name(); strings.HasSuffix(n, ".json") {
			out = append(out, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// A CORPUS-QUALITY report: F0Hz is always populated (the octave guard returns a number for a cymbal), so a low confidence
// on a pitched instrument means the RECORDING needs replacing. Behind -pitch because it decodes every clip, and the
// coverage golden must not churn on a detector change.
func runPitchDoctor(w io.Writer) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REFERENCE\tF0 (Hz)\tCONFIDENCE\tPITCH AXES\tINSTRUMENTS")
	var reliable, unreliable, skipped int
	for _, r := range refs {
		if r.Status != "" || r.Tier != wavref.TierInstrument {
			continue
		}
		p, ok := wavref.Resolve(r.ID)
		if !ok {
			skipped++
			continue
		}
		samples, rate, err := audio.DecodeWAVAtRate(p, wavref.Rate)
		if err != nil {
			fmt.Fprintf(tw, "%s\tERROR\t%v\t-\t%s\n", r.ID, err, strings.Join(instrument.Strings(r.Instruments), " "))
			skipped++
			continue
		}
		f := make([]float64, len(samples))
		for i, v := range samples {
			f[i] = float64(v)
		}
		e := fingerprint.DetectF0Detailed(wave.Wave{Samples: f, SampleRate: rate})
		state := "SUPPRESSED"
		if e.Reliable() {
			state = "measured"
			reliable++
		} else {
			unreliable++
		}
		fmt.Fprintf(tw, "%s\t%.2f\t%.3f\t%s\t%s\n",
			r.ID, e.Hz, e.Confidence, state, strings.Join(instrument.Strings(r.Instruments), " "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%d reference(s) with a reliable fundamental, %d without, %d not resolvable locally\n",
		reliable, unreliable, skipped)
	fmt.Fprintf(w, "threshold: confidence >= %.2f (fingerprint.F0ReliableConfidence)\n",
		fingerprint.F0ReliableConfidence)
	fmt.Fprintln(w, `"SUPPRESSED" means noise fraction, inharmonicity and partial series deviation report`)
	fmt.Fprintln(w, `"not measurable" against this reference rather than a number built on a fundamental`)
	fmt.Fprintln(w, `that isn't there. Expected for percussion; on a pitched instrument it means the CLIP`)
	fmt.Fprintln(w, `needs attention, not the synth.`)
	return nil
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	pitch := fs.Bool("pitch", false, "report per-reference F0 detection confidence instead of instrument coverage")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pitch {
		return runPitchDoctor(os.Stdout)
	}
	return runDoctor(os.Stdout)
}
