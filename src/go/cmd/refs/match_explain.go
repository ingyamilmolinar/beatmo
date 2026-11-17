//go:build !js

package main

// match_explain.go -- `refs match -explain`: WHY did the matcher rank a
// recording the way it did? For one recording it prints every essence term
// of the top candidates and of any instrument asked about, so a surprising
// ranking can be traced to the term that decided it instead of guessed at.

import (
	"fmt"
	"io"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// runMatchExplain ranks the roster against each recording the glob names and
// prints the per-term distances of the top n and of insts.
func runMatchExplain(out, diag io.Writer, glob string, maxSec float64, insts []instrument.ID, n, workers int) error {
	paths, err := expandMatchGlob(glob)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("refs match -explain: no files match %q", glob)
	}
	tmp, err := os.MkdirTemp("", "refs-explain-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	ids := rosterIDs()
	for _, p := range paths {
		ref, err := loadMatchReference(p, tmp, maxSec)
		if err != nil {
			return err
		}
		f0 := matchRefPitch(ref)
		refE := fingerprint.AnalyzeEssence(ref, fingerprint.EssenceOpts{})
		ranked, err := rankRoster(ref, f0, ids, workers, func(id instrument.ID) bool { return slices.Contains(insts, id) })
		if err != nil {
			return err
		}
		hubs := map[instrument.ID]float64{}
		if h, err := committedHubness(); err == nil {
			hubs, _ = h.hubValues(ids)
		}
		pitch := "unpitched"
		if f0 > 0 {
			pitch = fmt.Sprintf("%s %.1f Hz", noteNameForHz(f0), f0)
		}
		fmt.Fprintf(out, "\n%s  %s  window %.2fs  attack %.0f ms  class %s\n", matchShortName(ref.Label), pitch,
			refE.WindowSec, 1000*refE.AttackSec, matchClassOf(ref))
		tw := tabwriter.NewWriter(out, 0, 0, 1, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "#\tinstrument\tscore\traw\thub\tcolor\tevol\ttexture\tdecay\tattack\tpitch\theld s\t")
		show := map[string]bool{}
		for _, id := range insts {
			show[string(id)] = true
		}
		for i, r := range ranked {
			if i >= n && !show[string(r.cand.Instrument)] {
				continue
			}
			e := r.ess
			fmt.Fprintf(tw, "%d\t%s\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%+.1f\t%.2f\t\n", i+1, r.cand.Instrument,
				r.cand.Score, r.rawEssence, hubs[r.cand.Instrument], e.Color, e.Evolution, e.Texture, e.Decay,
				fingerprint.EssenceWeights.Attack*e.Attack, r.cand.RenderPitch, r.cand.RenderSec)
		}
		tw.Flush()
	}
	return nil
}
