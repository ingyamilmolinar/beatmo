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

// When the listener writes a word, does the axis claiming to measure it fire in the direction the word implies? Notes
// carry far more than the verdict letter, so this is better powered than good-vs-bad, and the only check that catches an
// axis measuring the RIGHT property in the WRONG direction.

// wantDir is what the SYNTH reads relative to the reference: a word for what the synth NEEDS maps to the opposite
// ("needs warmer" = the synth is too BRIGHT).
type termRule struct {
	// Words are matched as substrings of the lowercased note, so "breathy"
	// also catches "breathiness".
	Words   []string
	Axis    string
	WantDir string
	Why     string
}

// Small and literal: every entry cites the word as written; nothing is inferred from words nobody used.
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

// The success criterion is stated over RULES, so the arithmetic lives in code, not counted by eye. The buckets partition
// the rules that saw a family: agreeing (always fired the right way), backwards (any wrong-way or partial firing),
// silent (never fired).
type termAggregate struct {
	WithFamily int
	Agreeing   int
	Backwards  int
	Silent     int
}

// A rule with no family is counted nowhere: the corpus never used that word.
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
// So a termRule can never quietly name an axis the registry no longer has.
func registryHasAxis(name string) bool {
	for _, d := range fingerprint.DefaultRegistry().All() {
		if d.Name == name {
			return true
		}
	}
	return false
}

// The rules are hypotheses, and the corpus can falsify them, so discovery ranks EVERY axis per term by how
// consistently it fires on the families using the word versus those that do not.

// termCandidate is one axis's discovery score for one term.
type termCandidate struct {
	Axis        string
	InFired     int // families using the word, where this axis fired
	InTotal     int
	OutFired    int // families NOT using the word, where it fired
	OutTotal    int
	Specificity float64 // inRate - outRate; >0 means it discriminates this word
}

// The negative class is families with a note in OTHER words, never no note: silence says nothing, and scoring against
// it inflates broadly-firing axes. Only firing in the implied direction counts.
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
