//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// loadCatalogue is the CLI's single entry to the catalogue. It runs the
// STRUCTURAL invariants too — wavref.Catalogue() only checks schema_version,
// so without this a malformed catalogue.json would be tolerated silently.
func loadCatalogue() (wavref.Cat, error) {
	c, err := wavref.Catalogue()
	if err != nil {
		return wavref.Cat{}, err
	}
	if err := c.Validate(); err != nil {
		return wavref.Cat{}, err
	}
	return c, nil
}

// presence describes whether a ref's audio is resolvable right now.
func presence(r wavref.Ref) string {
	if r.Status != "" {
		return r.Status
	}
	if _, ok := wavref.Resolve(r.ID); ok {
		return "present"
	}
	return "missing"
}

func refTargets(r wavref.Ref) string {
	if r.Tier == wavref.TierSong {
		return r.Template
	}
	return strings.Join(instrument.Strings(r.Instruments), ",")
}

// runList writes a table of catalogue entries. tier == "" lists all.
func runList(w io.Writer, tier string) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIER\tTARGETS\tSTATE")
	for _, r := range refs {
		if tier != "" && r.Tier != tier {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.ID, r.Tier, refTargets(r), presence(r))
	}
	return tw.Flush()
}

func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tier := fs.String("tier", "", "filter by tier: instrument|song")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runList(os.Stdout, *tier)
}
