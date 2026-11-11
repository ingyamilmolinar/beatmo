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

// rosterIDs is the game's instrument registration: the canonical list every
// reference tool measures itself against.
//
// This is deliberately roster.All rather than "whatever
// catalogue.json happens to mention". The catalogue does not mention every
// registered id (snare-crack, kick-lofi, fm-bell-soft, ...), each of which
// carries its own document and so its own sound; auditing against the
// catalogue would keep them invisible forever.
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

// instrumentFromFilename maps a drop-in filename to a registered instrument
// id, so the drop-in directory is self-describing and `refs adopt` needs no
// -instrument for the common case.
//
// The id must match the whole normalised stem, or a prefix of it ending at a
// hyphen -- so "kick-lofi_garage_take3.wav" is a kick-lofi reference, while
// "mycrash.wav" is rejected rather than silently adopted as a crash. The
// LONGEST match wins, which is what keeps "kick-lofi.wav" from collapsing
// onto "kick". Adopting a file for the wrong instrument is the worst thing
// this command could do quietly, so an ambiguous name is an error carrying
// the near misses rather than a guess.

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

// nearMissMinPrefix is the shortest stem prefix allowed to produce a
// suggestion. Two characters is a coincidence, not a near miss --
// "backing-track" would otherwise be offered "banjo" and "bass-808", which
// is noise in a scan report that lists every unclaimed file. Below this,
// stating the naming rule alone is more useful than five wrong ids.
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
