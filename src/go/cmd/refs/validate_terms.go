//go:build !js

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// validate_terms.go — `refs validate -terms`: when the listener uses a word,
// does the axis that claims to measure that word agree?
//
// WHY THIS IS A BETTER TEST THAN GOOD-VS-BAD
// ------------------------------------------
// The AUC in validate.go splits 44 families into 7 good and 6 bad, which leaves
// every confidence interval about 0.3 wide -- enough to identify the extremes
// and nothing else. But the notes carry far more information than the verdict
// letter does: a listener who writes "too airy" has told us something specific
// and falsifiable about ONE property, and seventeen notes carry words like that.
//
// So this asks a narrower, better-powered question per axis: of the families
// whose note contains a word, how many does the corresponding axis flag in the
// direction the word implies? An axis that claims to measure airiness and does
// not fire on the notes that say "airy" has not earned the claim, whatever its
// overall AUC.
//
// This is also the only check that can catch an axis measuring the RIGHT
// property in the WRONG direction, which the corpus has already produced once
// (bright.centroid, AUC 0.36).

// termRule maps a word a listener actually wrote to the axis that should have
// seen it, and the direction it should have moved.
//
// wantDir is what the SYNTH should read relative to the reference: "high" means
// the axis should flag the synth as too high on that property. A word
// describing what the synth NEEDS ("warmer", "fuller") therefore maps to the
// property being too high in the opposite sense -- "needs warmer" means the
// synth is currently too BRIGHT.
type termRule struct {
	// Words are matched as substrings of the lowercased note, so "breathy"
	// also catches "breathiness".
	Words   []string
	Axis    string
	WantDir string
	Why     string
}

// termRules is deliberately small and literal. Every entry cites the word as
// the listener wrote it; nothing is inferred from words nobody used.
var termRules = []termRule{
	{
		Words: []string{"airy", "breathy", "wind instrument", "like a flute"},
		Axis:  "noise persistence", WantDir: "high",
		Why: "continuous noise through the note is what makes a bowed string read as blown",
	},
	{
		Words: []string{"warmer", "fuller", "needs warmness", "too bright", "pitched up or bright"},
		Axis:  "brightness ratio", WantDir: "high",
		Why: "asking for warmth means the synth currently sits too far above its own pitch",
	},
	{
		Words: []string{"too dark", "dark and mellow", "duller"},
		Axis:  "brightness ratio", WantDir: "low",
		Why: "the opposite complaint on the same axis",
	},
	{
		Words: []string{"wooden", "marimba"},
		Axis:  "mode harmonicity", WantDir: "high",
		Why: "a struck bar's modes are NOT integer multiples; a synth that reads as a plucked string is too harmonic. " +
			"Direction is a hypothesis pending re-audition: both kalimba and rimshot seeds changed after these " +
			"verdicts were recorded (the kalimba gen_gain fix targeted exactly \"sounds like a plucked string\"), " +
			"and the real kalimba reads 0.91 harmonic under the strong-mode window, so this rule is currently " +
			"unverifiable against the recorded notes.",
	},
	{
		Words: []string{"metallic", "bell", "resonant", "wavy", "waves in time"},
		Axis:  "beating depth", WantDir: "low",
		Why: "a bell's near-coincident partials beat; a clean additive tone does not",
	},
	{
		Words: []string{"longer decay", "let it ring"},
		Axis:  "band T60", WantDir: "low",
		Why: "a ring the listener wants extended is currently too short, band by band. \"Prolongs\" was dropped " +
			"from this rule's words: on tubular-bells it described the wavy metallic character (already caught " +
			"by the beating-depth rule via \"wavy\"/\"waves in time\"), not clip length, and the axis correctly " +
			"reports the synth ringing LONGER there, not shorter.",
	},
	{
		Words: []string{"too sustained", "ghost", "lingers"},
		Axis:  "decay T20", WantDir: "high",
		Why: "the synth outlives the reference",
	},
	{
		Words: []string{"hissy", "fuzzy"},
		Axis:  "spectral flatness", WantDir: "low",
		Why: "hiss is coloured, band-limited noise; wire is broadband",
	},
	{
		Words: []string{"rattly", "crack", "snap"},
		Axis:  "onset bite", WantDir: "low",
		Why: "the rattle and the crack are broadband onset energy the synth lacks",
	},
	{
		Words: []string{"fluty"},
		Axis:  "onset bite", WantDir: "low",
		Why: "a bowed string without its bow bite reads as blown",
	},
	{
		Words: []string{"nasal"},
		Axis:  "first formant", WantDir: "low",
		Why: "nasality is a raised low formant the synth lacks",
	},
}

// termHit is one (rule, family) evaluation.
type termHit struct {
	Family  instrument.ID
	Axis    string
	Fired   bool
	GotDir  string
	Verdict string
	Note    string
}

// termScore aggregates one rule.
type termScore struct {
	Rule     termRule
	Hits     []termHit
	Agree    int
	Fired    int
	Measured int
}

// matchTerm reports whether note mentions any of the rule's words.
func matchTerm(note string, r termRule) bool {
	n := strings.ToLower(note)
	for _, w := range r.Words {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// scoreTerms evaluates every rule against every note that mentions it.
func scoreTerms(samples []validationSample, verdicts []earVerdict) []termScore {
	noteFor := make(map[string]earVerdict, len(verdicts))
	for _, v := range verdicts {
		noteFor[string(v.Family)] = v
	}

	out := make([]termScore, 0, len(termRules))
	for _, rule := range termRules {
		sc := termScore{Rule: rule}
		for _, s := range samples {
			v, ok := noteFor[string(s.Family)]
			if !ok || v.Note == "" || !matchTerm(v.Note, rule) {
				continue
			}
			a, ok := s.Diagnosis.Axis(rule.Axis)
			if !ok || a.RefNA != "" || a.SynthNA != "" {
				sc.Hits = append(sc.Hits, termHit{Family: s.Family, Axis: rule.Axis,
					GotDir: "n/a", Verdict: v.Verdict, Note: v.Note})
				continue
			}
			sc.Measured++
			fired := a.Verdict != verdictOK
			if fired {
				sc.Fired++
			}
			if fired && a.Direction == rule.WantDir {
				sc.Agree++
			}
			sc.Hits = append(sc.Hits, termHit{Family: s.Family, Axis: rule.Axis,
				Fired: fired, GotDir: a.Direction, Verdict: v.Verdict, Note: v.Note})
		}
		out = append(out, sc)
	}
	sort.SliceStable(out, func(i, j int) bool { return len(out[i].Hits) > len(out[j].Hits) })
	return out
}

// termAggregate is the one-line summary of the whole TERM table: the spec's
// §9.2 success criterion is stated over RULES, not over families, so the
// arithmetic that judges it has to live in code rather than be re-counted by
// eye off the table (which is how the "8/9 agree" claim was reached -- it
// counted MISS rows as agreement).
//
// The three buckets PARTITION the rules that have at least one family:
//   - agreeing:  fired on every family it saw, always in the implied direction
//   - backwards: fired somewhere, but at least one of those firings ran the
//     other way (or the rule fired on some families and not others)
//   - silent:    never fired at all, so the axis says nothing about the word
type termAggregate struct {
	WithFamily int
	Agreeing   int
	Backwards  int
	Silent     int
}

// aggregateTermScores buckets the scored rules. A rule with no family at all
// is not counted anywhere: the corpus simply never used that word.
func aggregateTermScores(scores []termScore) termAggregate {
	var agg termAggregate
	for _, sc := range scores {
		if len(sc.Hits) == 0 {
			continue
		}
		agg.WithFamily++
		switch {
		case sc.Fired == 0:
			agg.Silent++
		case sc.Agree == sc.Fired:
			agg.Agreeing++
		default:
			agg.Backwards++
		}
	}
	return agg
}

// String renders the aggregate exactly as the report prints it.
func (a termAggregate) String() string {
	return fmt.Sprintf("rules with a family: %d; agreeing (AGREE==FIRED>0): %d; backwards: %d; silent (FIRED==0): %d",
		a.WithFamily, a.Agreeing, a.Backwards, a.Silent)
}

func runValidateTerms(w, diag io.Writer, outPath string) error {
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		return err
	}
	if len(verdicts) == 0 {
		return fmt.Errorf("refs validate -terms: no verdicts in %s -- run `make ab-all` first", outPath)
	}
	samples := collectValidationSamples(diag, verdicts)
	scores := scoreTerms(samples, verdicts)

	fmt.Fprintln(w, "Do the perceptual axes agree with the words the listener used?")
	fmt.Fprintln(w, "Each row is one vocabulary term, the axis that claims to measure it, and how")
	fmt.Fprintln(w, "often that axis flagged the named families in the implied direction.")
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TERM\tAXIS\tWANT\tAGREE\tFIRED\tMEASURED\tFAMILIES")
	for _, sc := range scores {
		if len(sc.Hits) == 0 {
			continue
		}
		var fams []string
		for _, h := range sc.Hits {
			fams = append(fams, string(h.Family))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			strings.Join(sc.Rule.Words[:1], ""), sc.Rule.Axis, sc.Rule.WantDir,
			sc.Agree, sc.Fired, sc.Measured, strings.Join(fams, " "))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\n%s\n", aggregateTermScores(scores))

	fmt.Fprintln(w, "\nPer-family detail:")
	for _, sc := range scores {
		if len(sc.Hits) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n  %s -> %s (want %s)\n    %s\n",
			strings.Join(sc.Rule.Words, " / "), sc.Rule.Axis, sc.Rule.WantDir, sc.Rule.Why)
		for _, h := range sc.Hits {
			mark := "MISS"
			switch {
			case h.GotDir == "n/a":
				mark = "n/a "
			case h.Fired && h.GotDir == sc.Rule.WantDir:
				mark = "OK  "
			case h.Fired:
				mark = "BACKWARDS"
			}
			note := h.Note
			if len(note) > 64 {
				note = note[:64] + "..."
			}
			fmt.Fprintf(w, "    %-9s %-16s got=%-5s  %q\n", mark, h.Family, h.GotDir, note)
		}
	}
	printTermDiscovery(w, samples, verdicts)

	fmt.Fprintln(w, "\nOK        = the axis flagged this family in the direction the word implies")
	fmt.Fprintln(w, "MISS      = the word was used but the axis did not flag the family at all")
	fmt.Fprintln(w, "BACKWARDS = the axis flagged it in the OPPOSITE direction -- acting on it would")
	fmt.Fprintln(w, "            move the instrument away from what the listener asked for")
	return nil
}

// registryHasAxis reports whether a named axis exists, so a termRule cannot
// quietly reference an axis that was renamed or removed.
func registryHasAxis(name string) bool {
	for _, d := range fingerprint.DefaultRegistry().All() {
		if d.Name == name {
			return true
		}
	}
	return false
}

// --- Discovery -------------------------------------------------------------
//
// The rules above are HYPOTHESES: a human decided which axis ought to
// correspond to which word. That is exactly the kind of guess the corpus can
// check, and the first pass already falsified one of them -- "wooden" was
// mapped to inharmonicity and agreed on none of the families where the
// listener wrote it, because the kalimba's inharmonicity had already been
// tuned to match while its partial SEQUENCE had not.
//
// So this searches instead of assuming: for each term, it ranks EVERY axis by
// how consistently it fires on the families whose notes use that word, versus
// how often it fires on the families that do not. An axis that fires on the
// named families and stays quiet elsewhere is measuring something specific to
// that word; one that fires on everything is not, however plausible its name.

// termCandidate is one axis's discovery score for one term.
type termCandidate struct {
	Axis        string
	InFired     int // families using the word, where this axis fired
	InTotal     int
	OutFired    int // families NOT using the word, where it fired
	OutTotal    int
	Specificity float64 // inRate - outRate; >0 means it discriminates this word
}

// discoverForTerm ranks every axis by how specifically it tracks one term.
//
// The negative class is families that HAVE a note but chose different words --
// never families with no note at all. A listener who wrote nothing about an
// instrument has not told us it is un-airy; they have told us nothing. Scoring
// against silence inflates every axis that fires broadly, which on the first
// pass put ring T60 (which fires on 55% of everything) among the top matches
// for "airy" purely by firing a lot.
//
// Direction counts too: an axis that fires on the right families in the WRONG
// direction is not measuring the word, it is measuring something anticorrelated
// with it, and counting it as agreement is how bright.centroid looked
// reasonable for as long as it did.
func discoverForTerm(samples []validationSample, noteFor map[string]earVerdict, rule termRule) []termCandidate {
	type acc struct{ inF, inT, outF, outT int }
	byAxis := map[string]*acc{}

	for _, s := range samples {
		v := noteFor[string(s.Family)]
		if v.Note == "" {
			continue // described nothing; carries no evidence either way
		}
		uses := matchTerm(v.Note, rule)
		for _, a := range s.Diagnosis.Axes {
			if a.RefNA != "" || a.SynthNA != "" {
				continue
			}
			x, ok := byAxis[a.Name]
			if !ok {
				x = &acc{}
				byAxis[a.Name] = x
			}
			// "Fired" here means fired IN THE DIRECTION the word implies, so a
			// backwards axis cannot score as a match.
			fired := a.Verdict != verdictOK && a.Direction == rule.WantDir
			if uses {
				x.inT++
				if fired {
					x.inF++
				}
			} else {
				x.outT++
				if fired {
					x.outF++
				}
			}
		}
	}

	var out []termCandidate
	for name, x := range byAxis {
		if x.inT == 0 || x.outT == 0 {
			continue
		}
		inRate := float64(x.inF) / float64(x.inT)
		outRate := float64(x.outF) / float64(x.outT)
		out = append(out, termCandidate{
			Axis: name, InFired: x.inF, InTotal: x.inT,
			OutFired: x.outF, OutTotal: x.outT,
			Specificity: inRate - outRate,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Specificity != out[j].Specificity {
			return out[i].Specificity > out[j].Specificity
		}
		return out[i].Axis < out[j].Axis
	})
	return out
}

// minTermFamilies is how many families must use a word before the discovery
// ranking is worth printing. Below this the top axis is chance.
const minTermFamilies = 3

func printTermDiscovery(w io.Writer, samples []validationSample, verdicts []earVerdict) {
	noteFor := make(map[string]earVerdict, len(verdicts))
	for _, v := range verdicts {
		noteFor[string(v.Family)] = v
	}

	fmt.Fprintln(w, "\n\nDISCOVERY: which axis actually tracks each word?")
	fmt.Fprintln(w, "SPECIFICITY = (fires in the implied direction on families USING the word)")
	fmt.Fprintln(w, "            - (fires the same way on families that wrote a note using OTHER words).")
	fmt.Fprintln(w, "The negative class is described-but-differently, never un-described: silence about")
	fmt.Fprintln(w, "an instrument is not evidence it lacks the property. An axis that fires broadly")
	fmt.Fprintln(w, "therefore scores near zero instead of being rewarded for it.")

	for _, rule := range termRules {
		n := 0
		for _, s := range samples {
			if v, ok := noteFor[string(s.Family)]; ok && v.Note != "" && matchTerm(v.Note, rule) {
				n++
			}
		}
		if n < minTermFamilies {
			fmt.Fprintf(w, "\n  %q: only %d famil(y/ies) use this word -- too few to rank axes against.\n",
				rule.Words[0], n)
			continue
		}
		cands := discoverForTerm(samples, noteFor, rule)
		fmt.Fprintf(w, "\n  %q (%d families)   [hypothesis: %s]\n", rule.Words[0], n, rule.Axis)
		for i, c := range cands {
			if i >= 4 {
				break
			}
			flag := ""
			if c.Axis == rule.Axis {
				flag = "  <- the hypothesis"
			}
			fmt.Fprintf(w, "    %+.2f  %-28s fires %d/%d on-word vs %d/%d off-word%s\n",
				c.Specificity, c.Axis, c.InFired, c.InTotal, c.OutFired, c.OutTotal, flag)
		}
	}
}
