//go:build !js

package main

// match_map.go -- `refs match -map`: the coverage map between a set of
// recordings and the roster, both ways round.
//
// Per RECORDING: its three closest voices, what the catalogue (or the
// filename) says it is, and where that instrument actually ranked -- with the
// direction the voice differs in (darker, noisier, decays faster) when it is
// not the closest. Per INSTRUMENT: the recordings of it in the set and how it
// ranks on them, which recordings it is the closest voice to anyway, and what
// it is missing. The map is written to MAP.md beside the match clips; the
// terminal gets the gaps.
//
// Nothing here changes a ranking: it reads the manifest `refs match` just
// wrote.

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// Instrument coverage states, best first.
const (
	mapMatched     = "matched"      // a recording of it ranks it in the top 3
	mapFidelityGap = "fidelity gap" // recordings of it exist, but it ranks below 3 on all of them
	mapStandIn     = "stand-in"     // no recording of it; it is in the top 3 for some other recording
	mapUncovered   = "uncovered"    // no recording of it and no top-3 place anywhere
)

// matchTop is how deep a ranking counts as "found" in the map: the clips
// `make ab-match` plays.
const matchTop = 3

var (
	freesoundURLID  = regexp.MustCompile(`/previews/\d+/(\d+)_`)
	freesoundFileID = regexp.MustCompile(`^(\d+)__`)
)

// matchCatalogueRef is the catalogued reference a recording IS, if any: the
// catalogue's own converted file (same filename), a local source (same path),
// or a freesound download (the sound id its filename starts with).
func matchCatalogueRef(path string, c wavref.Cat) (wavref.Ref, bool) {
	base := filepath.Base(path)
	abs, _ := filepath.Abs(path)
	fsID := ""
	if m := freesoundFileID.FindStringSubmatch(base); m != nil {
		fsID = m[1]
	}
	for _, r := range c.Refs {
		if r.Status != "" || r.Tier != "instrument" {
			continue
		}
		if r.File == base {
			return r, true
		}
		if r.Source == nil {
			continue
		}
		if r.Source.Provider == "local" && r.Source.URL == abs {
			return r, true
		}
		if m := freesoundURLID.FindStringSubmatch(r.Source.URL); fsID != "" && m != nil && m[1] == fsID {
			return r, true
		}
	}
	return wavref.Ref{}, false
}

// mapRecording is one manifest entry with what it is supposed to be.
type mapRecording struct {
	e matchEntry
	// label is what the recording is meant to be: the catalogue ref's
	// instruments, else the filename's namesake; source says which.
	label  []instrument.ID
	source string
	// refID is the catalogue reference it is a copy of ("" = none); name is
	// how the map shows it -- a raw download of a catalogued recording is
	// tagged, since it often shares the catalogue copy's filename.
	refID, name string
	// want is the label plus every member of the label's families.
	want map[instrument.ID]bool
	// famRank is the best rank of any wanted voice (0 = unlabelled or absent).
	famRank int
	famBest instrument.ID
}

// mapDirection describes how cand differs from the recording ref, in the
// words a person tuning the voice needs: brighter/darker, noisier/cleaner,
// dies faster/holds longer. Empty when every trait is within its threshold.
func mapDirection(ref fingerprint.EssenceTraits, cand *fingerprint.EssenceTraits) string {
	if cand == nil || ref.BrightnessHz <= 0 || cand.BrightnessHz <= 0 {
		return ""
	}
	var out []string
	if oct := math.Log2(cand.BrightnessHz / ref.BrightnessHz); math.Abs(oct) >= 0.25 {
		w := "brighter"
		if oct < 0 {
			w = "darker"
		}
		out = append(out, fmt.Sprintf("%s (%.0f vs %.0f Hz)", w, cand.BrightnessHz, ref.BrightnessHz))
	}
	if d := cand.Noise - ref.Noise; math.Abs(d) >= 0.08 {
		w := "noisier"
		if d < 0 {
			w = "cleaner"
		}
		out = append(out, fmt.Sprintf("%s (noise %.2f vs %.2f)", w, cand.Noise, ref.Noise))
	}
	if d := cand.DecayDB - ref.DecayDB; math.Abs(d) >= 4 {
		w := "holds longer"
		if d < 0 {
			w = "dies faster"
		}
		out = append(out, fmt.Sprintf("%s (%.0f vs %.0f dB)", w, cand.DecayDB, ref.DecayDB))
	}
	return strings.Join(out, ", ")
}

// mapWorstTerm names the essence term that costs cand the most against the
// first-ranked voice -- the term that kept it from matching.
func mapWorstTerm(cand, best *fingerprint.EssenceScore) string {
	if cand == nil || best == nil {
		return ""
	}
	w := fingerprint.EssenceWeights
	terms := []struct {
		name string
		d    float64
	}{
		{"colour", w.Color * (cand.Color - best.Color)},
		{"evolution", w.Evolution * (cand.Evolution - best.Evolution)},
		{"texture", w.Texture * (cand.Texture - best.Texture)},
		{"decay", w.Decay * (cand.Decay - best.Decay)},
		{"attack", w.Attack * (cand.Attack - best.Attack)},
	}
	sort.SliceStable(terms, func(a, b int) bool { return terms[a].d > terms[b].d })
	if terms[0].d <= 0 {
		return ""
	}
	return fmt.Sprintf("%s +%.2f", terms[0].name, terms[0].d)
}

func candidateOf(e matchEntry, inst instrument.ID) (matchCandidate, int) {
	for i, c := range e.Ranking {
		if c.Instrument == inst {
			return c, i + 1
		}
	}
	return matchCandidate{}, 0
}

// buildMatchMap labels every entry and returns the recordings and, per
// roster instrument, its coverage state.
func buildMatchMap(man matchManifest, c wavref.Cat) ([]mapRecording, error) {
	famOf, err := matchFamilies(c)
	if err != nil {
		return nil, err
	}
	var recs []mapRecording
	for _, e := range man.Entries {
		r := mapRecording{e: e, want: map[instrument.ID]bool{}, name: matchShortName(e.Name)}
		if ref, ok := matchCatalogueRef(e.Source, c); ok {
			r.label, r.source, r.refID = ref.Instruments, "catalogue "+ref.ID, ref.ID
			if p, ok := wavref.Resolve(ref.ID); !ok || !sameFile(p, e.Source) {
				r.name += " (download)"
			}
		} else if e.Namesake != "" {
			r.label, r.source = []instrument.ID{e.Namesake}, "filename"
		}
		r.want = matchWanted(r.label, famOf)
		for i, cand := range e.Ranking {
			if r.want[cand.Instrument] {
				r.famRank, r.famBest = i+1, cand.Instrument
				break
			}
		}
		recs = append(recs, r)
	}
	return recs, nil
}

type mapInstrument struct {
	id           instrument.ID
	kind, status string
	// own: recordings labelled as this instrument, with its rank on each.
	own []mapOwn
	// top1 / top3: recordings it is the closest / a top-3 voice for.
	top1, top3 []string
	// closest: the recording it ranks best on (when it has none of its own).
	closest     string
	closestRank int
	state       string
	note        string
}

type mapOwn struct {
	rec  string
	rank int
	dir  string
	term string
}

// catalogueStatusOf is how the catalogue covers inst: "ref <id>[, <id>]",
// "synthetic", "needs-source" or "-".
func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}

func catalogueStatusOf(inst instrument.ID, c wavref.Cat) string {
	var refs []string
	status := ""
	for _, r := range c.Refs {
		if r.Tier != "instrument" || !slices.Contains(r.Instruments, inst) {
			continue
		}
		if r.Status != "" {
			status = r.Status
			continue
		}
		refs = append(refs, r.ID)
	}
	if len(refs) > 0 {
		return "ref " + strings.Join(refs, ", ")
	}
	if status != "" {
		return status
	}
	return "-"
}

func buildInstrumentMap(recs []mapRecording, c wavref.Cat, ids []instrument.ID) []mapInstrument {
	var out []mapInstrument
	for _, id := range ids {
		mi := mapInstrument{id: id, kind: broadClass(id), status: catalogueStatusOf(id, c)}
		for _, r := range recs {
			cand, rank := candidateOf(r.e, id)
			if rank == 0 {
				continue
			}
			name := r.name
			if slices.Contains(r.label, id) {
				var best *fingerprint.EssenceScore
				if len(r.e.Ranking) > 0 {
					best = r.e.Ranking[0].Terms
				}
				o := mapOwn{rec: name, rank: rank, dir: mapDirection(r.e.Traits, cand.Traits)}
				if rank > 1 {
					o.term = mapWorstTerm(cand.Terms, best)
				}
				mi.own = append(mi.own, o)
			}
			if rank == 1 {
				mi.top1 = append(mi.top1, name)
			}
			if rank <= matchTop {
				mi.top3 = append(mi.top3, name)
			}
			if mi.closestRank == 0 || rank < mi.closestRank {
				mi.closest, mi.closestRank = name, rank
			}
		}
		bestOwn := 0
		for _, o := range mi.own {
			if bestOwn == 0 || o.rank < bestOwn {
				bestOwn = o.rank
			}
		}
		switch {
		case bestOwn >= 1 && bestOwn <= matchTop:
			mi.state = mapMatched
		case len(mi.own) > 0:
			mi.state = mapFidelityGap
		case len(mi.top3) > 0:
			mi.state = mapStandIn
		default:
			mi.state = mapUncovered
		}
		out = append(out, mi)
	}
	return out
}

func mdCell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

// writeMatchMap writes MAP.md and prints the summary and the gaps.
func writeMatchMap(term io.Writer, dir string, man matchManifest) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	recs, err := buildMatchMap(man, c)
	if err != nil {
		return err
	}
	insts := buildInstrumentMap(recs, c, rosterIDs())

	var b strings.Builder
	fmt.Fprintf(&b, "# WAV ↔ instrument map\n\n%s · %d recordings · %d instruments · roster %s\n\n",
		time.Now().Format("2006-01-02 15:04"), len(recs), len(insts), firstRosterHash(man))
	fmt.Fprintf(&b, "Generated by `refs match -map` (`make wav-match-map`). Rankings are the ESSENCE distance: colour, its evolution, noise texture, the decay shape and the attack, with pitch, note length, level and room removed by construction (docs/superpowers/specs/2026-10-17-wav-match-essence-design.md). Labels come from the reference catalogue (or, failing that, an instrument named in the filename); the matcher never sees them.\n\n")

	// Summary: a catalogued recording counts once, however many copies of
	// it (its catalogue file, a raw download) were matched.
	var unique, labelled, fam1, fam3, kind1 int
	counted := map[string]bool{}
	for _, r := range recs {
		if r.refID != "" {
			if counted[r.refID] {
				continue
			}
			counted[r.refID] = true
		}
		unique++
		if len(r.label) == 0 {
			continue
		}
		labelled++
		if r.famRank == 1 {
			fam1++
		}
		if r.famRank >= 1 && r.famRank <= matchTop {
			fam3++
		}
		if k := broadClass(r.label[0]); k != "" && len(r.e.Ranking) > 0 && broadClass(r.e.Ranking[0].Instrument) == k {
			kind1++
		}
	}
	states := map[string][]string{}
	for _, mi := range insts {
		states[mi.state] = append(states[mi.state], string(mi.id))
	}
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "- **Recordings**: %d files = %d distinct recordings, of which %d are labelled. Their own instrument family ranks #1 on %d, in the top 3 on %d; the first-ranked voice is the right KIND of instrument on %d. (A raw download of a catalogued recording is listed beside the catalogue copy -- the two are trimmed and encoded differently, so their agreement is itself a robustness check -- but counted once.)\n", len(recs), unique, labelled, fam1, fam3, kind1)
	fmt.Fprintf(&b, "- **Instruments**: %d matched (a recording of it ranks it top 3) · %d fidelity gaps (recordings of it exist, it ranks lower on all) · %d stand-ins (no recording of its own, but the closest voice to one) · %d uncovered.\n\n",
		len(states[mapMatched]), len(states[mapFidelityGap]), len(states[mapStandIn]), len(states[mapUncovered]))

	// Recordings.
	fmt.Fprintf(&b, "## Recordings\n\n| recording | labelled as | pitch | #1 | #2 | #3 | own family | how the own voice differs |\n|---|---|---|---|---|---|---|---|\n")
	for _, r := range recs {
		pitch := "unpitched"
		if r.e.Pitched {
			pitch = fmt.Sprintf("%s (%.0f Hz)", r.e.Note, r.e.F0Hz)
		}
		label := "—"
		if len(r.label) > 0 {
			label = strings.Join(instrument.Strings(r.label), ", ") + " (" + r.source + ")"
		}
		var top [matchTop]string
		for i := 0; i < matchTop && i < len(r.e.Ranking); i++ {
			top[i] = fmt.Sprintf("%s %.2f", r.e.Ranking[i].Instrument, r.e.Ranking[i].Score)
			if r.want[r.e.Ranking[i].Instrument] {
				top[i] = "**" + top[i] + "**"
			}
		}
		own, diff := "—", ""
		if len(r.label) > 0 {
			switch {
			case r.famRank == 0:
				own = "not ranked"
			case r.famRank == 1:
				own = "✓ #1 " + string(r.famBest)
			default:
				own = fmt.Sprintf("#%d %s", r.famRank, r.famBest)
				cand, _ := candidateOf(r.e, r.famBest)
				var best *fingerprint.EssenceScore
				if len(r.e.Ranking) > 0 {
					best = r.e.Ranking[0].Terms
				}
				parts := []string{}
				if t := mapWorstTerm(cand.Terms, best); t != "" {
					parts = append(parts, "loses on "+t)
				}
				if d := mapDirection(r.e.Traits, cand.Traits); d != "" {
					parts = append(parts, "voice is "+d)
				}
				diff = strings.Join(parts, "; ")
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", mdCell(r.name), mdCell(label), pitch,
			top[0], top[1], top[2], own, mdCell(diff))
	}

	// Instruments, grouped by state then kind.
	order := []string{mapFidelityGap, mapUncovered, mapStandIn, mapMatched}
	heads := map[string]string{
		mapMatched:     "Matched — a recording of it ranks it in the top 3",
		mapFidelityGap: "Fidelity gaps — recordings of it exist, but the synth ranks below 3 on all of them",
		mapStandIn:     "Stand-ins — no recording of its own here, but the closest voice to some other recording",
		mapUncovered:   "Uncovered — no recording of it here and in no recording's top 3",
	}
	for _, st := range order {
		var rows []mapInstrument
		for _, mi := range insts {
			if mi.state == st {
				rows = append(rows, mi)
			}
		}
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(a, b int) bool {
			if rows[a].kind != rows[b].kind {
				return rows[a].kind < rows[b].kind
			}
			return rows[a].id < rows[b].id
		})
		fmt.Fprintf(&b, "\n## %s (%d)\n\n| instrument | kind | catalogue | its recordings: rank (what loses it; how the voice differs) | closest voice for | best rank anywhere |\n|---|---|---|---|---|---|\n", heads[st], len(rows))
		for _, mi := range rows {
			var own []string
			for _, o := range mi.own {
				s := fmt.Sprintf("%s #%d", o.rec, o.rank)
				var why []string
				if o.term != "" {
					why = append(why, o.term)
				}
				if o.dir != "" {
					why = append(why, o.dir)
				}
				if len(why) > 0 {
					s += " (" + strings.Join(why, "; ") + ")"
				}
				own = append(own, s)
			}
			ownS := "—"
			if len(own) > 0 {
				ownS = strings.Join(own, "<br>")
			}
			top1 := "—"
			if len(mi.top1) > 0 {
				top1 = strings.Join(mi.top1, "<br>")
			} else if len(mi.top3) > 0 {
				top1 = "(top 3) " + strings.Join(mi.top3, "<br>")
			}
			best := "—"
			if mi.closestRank > 0 {
				best = fmt.Sprintf("#%d on %s", mi.closestRank, mi.closest)
			}
			kind := mi.kind
			if kind == "" {
				kind = "?"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", mi.id, kind, mdCell(mi.status), mdCell(ownS), mdCell(top1), mdCell(best))
		}
	}

	path := filepath.Join(dir, "MAP.md")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}

	// Terminal: the summary and the gaps.
	fmt.Fprintf(term, "\nrecordings: %d files, %d distinct (%d labelled): own family #1 %d, top-3 %d, right kind #1 %d\n", len(recs), unique, labelled, fam1, fam3, kind1)
	for _, st := range order {
		ids := states[st]
		fmt.Fprintf(term, "%-13s %3d  %s\n", st, len(ids), strings.Join(ids, " "))
	}
	fmt.Fprintf(term, "\nmap: %s\n", path)
	return nil
}

func firstRosterHash(man matchManifest) string {
	for _, e := range man.Entries {
		return e.RosterHash
	}
	return "-"
}

// matchLabel is what the recording at path is meant to be: its catalogue
// reference's instruments, else the instrument its filename names.
func matchLabel(path string, namesake instrument.ID, c wavref.Cat) []instrument.ID {
	if ref, ok := matchCatalogueRef(path, c); ok {
		return ref.Instruments
	}
	if namesake != "" {
		return []instrument.ID{namesake}
	}
	return nil
}

// matchFamilies maps every audition-family member to its family's members.
func matchFamilies(c wavref.Cat) (map[instrument.ID][]instrument.ID, error) {
	fams, err := buildAuditionFamilies(c)
	if err != nil {
		return nil, err
	}
	famOf := map[instrument.ID][]instrument.ID{}
	for _, f := range fams {
		for _, m := range f.Members {
			famOf[m] = f.Members
		}
	}
	return famOf, nil
}

// matchWanted is label plus every member of the label's families.
func matchWanted(label []instrument.ID, famOf map[instrument.ID][]instrument.ID) map[instrument.ID]bool {
	want := map[instrument.ID]bool{}
	for _, inst := range label {
		want[inst] = true
		for _, m := range famOf[inst] {
			want[m] = true
		}
	}
	return want
}
