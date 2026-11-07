//go:build !js

package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// roster.All, not what catalogue.json mentions: unmentioned registered ids each carry their own document, and auditing
// against the catalogue would keep them invisible.
func rosterIDs() []instrument.ID { return slices.Clone(roster.All) }

// normalizeStem folds a filename stem into instrument-id shape: lowercase,
// with underscores and spaces as hyphens. "Kick_Deep" and "kick deep" both
// become "kick-deep".
func normalizeStem(stem string) string {
	s := strings.ToLower(stem)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return strings.Trim(s, "-")
}

// The id must match the whole normalised stem or a prefix ending at a hyphen, the LONGEST winning ("kick-lofi_take3" is
// kick-lofi, "mycrash" is rejected). Adopting for the wrong instrument is the worst quiet failure, so an ambiguous name
// is an error with the near misses.

func instrumentFromFilename(name string) (instrument.ID, error) {
	stem := normalizeStem(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
	if stem == "" {
		return "", fmt.Errorf("refs: %q has no usable name", name)
	}

	var best instrument.ID
	for _, id := range rosterIDs() {
		if stem != string(id) && !strings.HasPrefix(stem, string(id)+"-") {
			continue
		}
		if len(id) > len(best) {
			best = id
		}
	}
	if best != "" {
		return best, nil
	}

	msg := fmt.Sprintf("refs: cannot tell which instrument %q is for "+
		"(the filename must start with a registered instrument id, e.g. kick-lofi.wav or kick-lofi_take3.wav)",
		filepath.Base(name))
	if near := nearestRosterIDs(stem, 5); len(near) > 0 {
		msg += fmt.Sprintf("; closest registered ids: %s", strings.Join(instrument.Strings(near), ", "))
	}
	return "", fmt.Errorf("%s; or pass -instrument explicitly", msg)
}

// Two characters is a coincidence ("backing-track" → "banjo"), noise in a scan report; below this the naming rule alone
// is more useful.
const nearMissMinPrefix = 3

// nearestRosterIDs suggests registered ids for a stem that resolved to
// nothing, by progressively shortening the stem's own prefix. "snare2"
// yields the snare family rather than a bare refusal.
func nearestRosterIDs(stem string, n int) []instrument.ID {
	ids := rosterIDs()
	slices.Sort(ids)
	for cut := len(stem); cut >= nearMissMinPrefix; cut-- {
		var out []instrument.ID
		for _, id := range ids {
			if strings.HasPrefix(string(id), stem[:cut]) {
				out = append(out, id)
				if len(out) == n {
					break
				}
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}
