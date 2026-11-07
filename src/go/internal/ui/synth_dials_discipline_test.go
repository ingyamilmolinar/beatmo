package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// The rules that need the instruments, over the shipped file and the whole roster; Load enforces the structural
// ones.

type rosterSchema struct {
	id      string
	defs    map[string]audio.ParamDef
	schema  []audio.ParamDef
	claimed map[string]map[string]bool // card id → params it claims on this instrument
	table   map[string]bool            // card ids of the table this instrument uses
}

func shippedDials(t *testing.T) synthdial.File {
	t.Helper()
	f, err := synthdial.Load(assets.SynthDialsJSON)
	if err != nil {
		t.Fatalf("the shipped synth_dials.json does not load: %v", err)
	}
	return f
}

func rosterSchemas(t *testing.T) []rosterSchema {
	t.Helper()
	var out []rosterSchema
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		rs := rosterSchema{id: string(id), defs: map[string]audio.ParamDef{}, schema: schema, claimed: map[string]map[string]bool{}, table: map[string]bool{}}
		for _, d := range schema {
			rs.defs[d.Name] = d
		}
		for _, def := range synthCategoryTableFor(schema) {
			rs.table[def.ID] = true
		}
		for _, g := range buildMacroGroupsAll(schema, audio.MergedInstrumentParamsRO(id)) {
			set := map[string]bool{}
			for _, p := range g.Params {
				set[p] = true
			}
			rs.claimed[g.ID] = set
		}
		out = append(out, rs)
	}
	if len(out) < 100 {
		t.Fatalf("roster walk found only %d instruments with a schema", len(out))
	}
	return out
}

func cardTables() (melodic, percussion map[string]bool) {
	melodic, percussion = map[string]bool{}, map[string]bool{}
	for _, d := range macroGroupDefsMelodic {
		melodic[d.ID] = true
	}
	for _, d := range macroGroupDefsPercussion {
		percussion[d.ID] = true
	}
	return melodic, percussion
}

// Every named card exists, and within one table no param is driven by two dials, so dials compose independently.
func TestSynthDialsCardsExistAndShareNoParamWithinATable(t *testing.T) {
	f := shippedDials(t)
	melodic, percussion := cardTables()
	for _, tbl := range []struct {
		name  string
		cards map[string]bool
	}{{"melodic", melodic}, {"percussion", percussion}} {
		owner := map[string]string{}
		for _, d := range f.Dials {
			inTable := false
			for _, c := range d.Cards {
				if !melodic[c] && !percussion[c] {
					t.Errorf("dial %q names unknown card %q", d.ID, c)
				}
				inTable = inTable || tbl.cards[c]
			}
			if !inTable {
				continue
			}
			for _, tm := range d.Terms {
				if prev, ok := owner[tm.Param]; ok {
					t.Errorf("%s table: %q is driven by both %q and %q", tbl.name, tm.Param, prev, d.ID)
				}
				owner[tm.Param] = d.ID
			}
		}
	}
}

// A term no card ever claims is dead metadata.
func TestEveryDialTermIsDeclaredAndClaimedByItsCard(t *testing.T) {
	f := shippedDials(t)
	roster := rosterSchemas(t)
	for _, d := range f.Dials {
		for _, tm := range d.Terms {
			declared, claimed := false, false
			for _, rs := range roster {
				if _, ok := rs.defs[tm.Param]; !ok {
					continue
				}
				declared = true
				for _, c := range d.Cards {
					if rs.claimed[c][tm.Param] {
						claimed = true
					}
				}
			}
			if !declared {
				t.Errorf("dial %q: no shipped instrument declares %q", d.ID, tm.Param)
			} else if !claimed {
				t.Errorf("dial %q: %q is never claimed by its cards %v on any instrument", d.ID, tm.Param, d.Cards)
			}
		}
	}
}

// No term drives an enum, a stepped value or a tuning param on any instrument; log terms need a positive range.
func TestNoDialDrivesAPickerOrTuning(t *testing.T) {
	f := shippedDials(t)
	roster := rosterSchemas(t)
	widest := map[string]synthdial.Range{}
	for _, d := range f.Dials {
		for _, tm := range d.Terms {
			if synthDialTuningParams[tm.Param] {
				t.Errorf("dial %q drives tuning param %q", d.ID, tm.Param)
			}
			for _, rs := range roster {
				def, ok := rs.defs[tm.Param]
				if !ok {
					continue
				}
				if len(def.Enum) > 0 || def.Step > 0 {
					t.Errorf("dial %q drives %q, a picker/stepped param on %s", d.ID, tm.Param, rs.id)
				}
				if tm.Space == synthdial.SpaceLog && def.Max <= 0 {
					t.Errorf("dial %q: log term %q has Max %v on %s", d.ID, tm.Param, def.Max, rs.id)
				}
				k := d.ID + "/" + tm.Param
				if w, ok := widest[k]; ok {
					widest[k] = synthdial.Range{Min: math.Min(w.Min, def.Min), Max: math.Max(w.Max, def.Max)}
				} else {
					widest[k] = synthdial.Range{Min: def.Min, Max: def.Max}
				}
			}
		}
	}
	// Within the widest declared range: an end beyond every range is unreachable travel.
	for _, d := range f.Dials {
		for _, tm := range d.Terms {
			w, ok := widest[d.ID+"/"+tm.Param]
			if !ok {
				continue
			}
			lo, hi := w.Min, w.Max
			if tm.Space == synthdial.SpaceMagnitude {
				lo, hi = 0, math.Max(w.Max, -w.Min)
			}
			for _, end := range []*float64{tm.From, tm.To} {
				if end != nil && (*end < lo-1e-9 || *end > hi+1e-9) {
					t.Errorf("dial %q: %q span end %v lies outside every instrument's range [%v, %v]", d.ID, tm.Param, *end, lo, hi)
				}
			}
		}
	}
}

func TestSynthDialStringsAndMetricsExist(t *testing.T) {
	f := shippedDials(t)
	for _, d := range f.Dials {
		for _, k := range []string{d.Label, d.Gloss} {
			for _, loc := range i18n.AllLocales() {
				if !i18n.Has(loc, i18n.Key(k)) {
					t.Errorf("dial %q: key %q missing from the %s catalogue", d.ID, k, loc)
				}
			}
		}
		if d.Metric != "" {
			if _, ok := fingerprint.DefaultRegistry().ByID(d.Metric); !ok {
				t.Errorf("dial %q: metric %q is not a fingerprint descriptor", d.ID, d.Metric)
			}
		}
	}
}

// No instrument silently loses a dial to the per-card cap.
func TestNoInstrumentLosesADialToTheCap(t *testing.T) {
	f := shippedDials(t)
	for _, rs := range rosterSchemas(t) {
		for card, claimed := range rs.claimed {
			if !rs.table[card] {
				continue
			}
			n := 0
			var names []string
			for _, d := range f.Dials {
				if !d.OnCard(card) {
					continue
				}
				for _, tm := range d.Terms {
					if def, ok := rs.defs[tm.Param]; ok && claimed[tm.Param] && synthDialDrivable(def) {
						n++
						names = append(names, d.ID)
						break
					}
				}
			}
			if n <= synthdial.MaxDialsPerCard {
				continue
			}
			t.Errorf("%s card %q has %d live dials %v; the cap of %d hides %v", rs.id, card, n, names, synthdial.MaxDialsPerCard, names[synthdial.MaxDialsPerCard:])
		}
	}
}

// Most instruments get a root dial, and every dial is reachable on a root or once its stage is added.
func TestSynthDialCoverageAcrossTheRoster(t *testing.T) {
	f := shippedDials(t)
	live := map[string]int{}
	reachable := map[string]bool{}
	withDials := 0
	roster := rosterSchemas(t)
	for _, rs := range roster {
		has := false
		for _, g := range buildMacroGroups(rs.schema, audio.MergedInstrumentParamsRO(instrument.ID(rs.id))) {
			for _, d := range g.Dials {
				live[d.Dial.ID]++
				reachable[d.Dial.ID] = true
				has = true
			}
		}
		if has {
			withDials++
		}
		// Every stage switched on.
		allOn := audio.RecipeParams{}
		for k, v := range audio.MergedInstrumentParamsRO(instrument.ID(rs.id)) {
			allOn[k] = v
		}
		for _, d := range rs.schema {
			if isSynthEnableParam(d.Name) {
				allOn[d.Name] = 1
			}
		}
		for _, g := range buildMacroGroupsAll(rs.schema, allOn) {
			for _, d := range g.Dials {
				reachable[d.Dial.ID] = true
			}
		}
	}
	var dead []string
	for _, d := range f.Dials {
		if !reachable[d.ID] {
			dead = append(dead, d.ID)
		}
	}
	sort.Strings(dead)
	if len(dead) > 0 {
		t.Errorf("dials reachable on no shipped instrument: %s", strings.Join(dead, ", "))
	}
	if withDials*10 < len(roster)*9 {
		t.Errorf("only %d of %d instruments show a dial on the root", withDials, len(roster))
	}
	t.Logf("%d/%d instruments show dials; live counts %v", withDials, len(roster), live)
}

func paramsEqual(a, b audio.RecipeParams) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// A typo in either silently hides the term everywhere.
func TestDialTermGatesNameRealConditionsAndParams(t *testing.T) {
	f := shippedDials(t)
	declared := map[string]bool{}
	for _, rs := range rosterSchemas(t) {
		for p := range rs.defs {
			declared[p] = true
		}
	}
	for _, d := range f.Dials {
		for _, tm := range d.Terms {
			if tm.When != "" && synthDialConditions[tm.When] == nil {
				t.Errorf("dial %q term %q: unknown condition %q", d.ID, tm.Param, tm.When)
			}
			if tm.Requires != "" && !declared[tm.Requires] {
				t.Errorf("dial %q term %q: requires undeclared param %q", d.ID, tm.Param, tm.Requires)
			}
		}
	}
}

// Or turning hides the term mid-gesture and the reading jumps; cross-dial gates are fine (the row is frozen).
func TestDialGatesNeverReadTheirOwnDial(t *testing.T) {
	for name := range synthDialConditions {
		if _, ok := synthDialConditionReads[name]; !ok {
			t.Errorf("condition %q has no synthDialConditionReads entry", name)
		}
	}
	for _, d := range shippedDials(t).Dials {
		own := map[string]bool{}
		for _, tm := range d.Terms {
			own[tm.Param] = true
		}
		for _, tm := range d.Terms {
			if own[tm.Requires] {
				t.Errorf("dial %q: term %q requires %q, which the same dial drives", d.ID, tm.Param, tm.Requires)
			}
			for _, p := range synthDialConditionReads[tm.When] {
				if own[p] {
					t.Errorf("dial %q: term %q is gated by %q, which reads %q — driven by the same dial", d.ID, tm.Param, tm.When, p)
				}
			}
		}
	}
}

// Per dial: live terms whose shipped value lies outside the span, and the shipped readings.
func shippedDialReadings(t *testing.T) (outside map[string][]string, readings map[string][]float64) {
	outside, readings, _ = shippedDialCalibration(t)
	return outside, readings
}

// Plus, per dial, how many read within 5 of an end a wider span could still leave; an end the params cannot pass
// (an organ's sustain of 1) is the param's truth.
func shippedDialCalibration(t *testing.T) (outside map[string][]string, readings map[string][]float64, widenable map[string]int) {
	outside, readings, widenable, _ = shippedDialCalibrationSides(t)
	return outside, readings, widenable
}

// Per side: [0] some term's From is inside its range, [1] some term's To is.
func shippedDialCalibrationSides(t *testing.T) (outside map[string][]string, readings map[string][]float64, widenable map[string]int, sides map[string][2]bool) {
	t.Helper()
	outside, readings, widenable, sides = map[string][]string{}, map[string][]float64{}, map[string]int{}, map[string][2]bool{}
	for _, rs := range rosterSchemas(t) {
		m := audio.MergedInstrumentParamsRO(instrument.ID(rs.id))
		// As the root resolves them: the voice length feeds the caps and the "settles" gate.
		for _, g := range buildMacroGroupsForVoice(rs.schema, m, voiceSecsOf(rs.id)) {
			for _, d := range g.Dials {
				r := d.Reading(m)
				readings[d.Dial.ID] = append(readings[d.Dial.ID], r)
				sd := sides[d.Dial.ID]
				for _, tm := range d.Terms {
					inside := func(v float64) bool { return v > tm.Range.Min && v < tm.Range.Max }
					sd[0] = sd[0] || inside(tm.Span.From)
					sd[1] = sd[1] || inside(tm.Span.To)
				}
				sides[d.Dial.ID] = sd
				if r <= 0.05 || r >= 0.95 {
					for _, tm := range d.Terms {
						end := tm.Span.To
						if r <= 0.05 {
							end = tm.Span.From
						}
						if end > tm.Range.Min && end < tm.Range.Max {
							widenable[d.Dial.ID]++
							break
						}
					}
				}
				for _, tm := range d.Terms {
					u := tm.Coord(m[tm.Param])
					// At or past the off end it sounds off and reads 0.
					if d.Dial.Rest == "start" && u < 0 {
						continue
					}
					if u < -1e-9 || u > 1+1e-9 {
						outside[d.Dial.ID] = append(outside[d.Dial.ID], fmt.Sprintf("%s %s=%v (span %v)", rs.id, tm.Param, m[tm.Param], tm.Span))
					}
				}
			}
		}
	}
	return outside, readings, widenable, sides
}

// So the reading is honest; on a resting dial a value past the OFF end reads 0, which is honest too.
func TestShippedValuesLieInsideTheirSpans(t *testing.T) {
	outside, _ := shippedDialReadings(t)
	for _, id := range sortedStringKeys(outside) {
		t.Errorf("dial %q: shipped values outside the span: %v", id, outside[id])
	}
}

// Median reading in [20, 80], at most a quarter within 5 of a leavable end, unless the dial rests at a natural off
// (each 0 then the param's off edge).
func TestDialCalibrationSpread(t *testing.T) {
	_, readings, widenable, sides := shippedDialCalibrationSides(t)
	f := shippedDials(t)
	rest := map[string]bool{}
	for _, d := range f.Dials {
		rest[d.ID] = d.Rest == "start"
	}
	for _, id := range sortedStringKeys(readings) {
		r := append([]float64(nil), readings[id]...)
		sort.Float64s(r)
		med := r[len(r)/2]
		edge := 0
		for _, v := range r {
			if v <= 0.05 || v >= 0.95 {
				edge++
			}
		}
		t.Logf("%-12s n=%3d min %3.0f med %3.0f max %3.0f  near-end %3d%% (widenable %d)  rest=%v", id, len(r), 100*r[0], 100*med, 100*r[len(r)-1], 100*edge/len(r), widenable[id], rest[id])
		if rest[id] {
			continue
		}
		// Only where a span end could still move.
		if (med < 0.20 && sides[id][0]) || (med > 0.80 && sides[id][1]) {
			t.Errorf("dial %q: median shipped reading %.0f is outside [20, 80] — recentre its spans", id, 100*med)
		}
		if w := widenable[id]; w*4 > len(r) {
			t.Errorf("dial %q: %d of %d shipped sounds read within 5 of an end a wider span could leave — widen its spans", id, w, len(r))
		}
	}
	// A resting dial's 0 is each term's off edge on every instrument.
	for _, rs := range rosterSchemas(t) {
		for _, d := range f.Dials {
			if d.Rest != "start" {
				continue
			}
			for _, tm := range d.Terms {
				def, ok := rs.defs[tm.Param]
				if !ok {
					continue
				}
				r := synthdial.Range{Min: def.Min, Max: def.Max}
				lt := synthdial.LiveTerm{Term: tm, Range: r, Span: synthdial.SpanFor(tm, r)}
				off := def.Min
				if lt.Span.From > lt.Span.To {
					off = def.Max
				}
				if tm.Space == synthdial.SpaceMagnitude {
					off = 0
				}
				if u := lt.Coord(off); u > 1e-9 {
					t.Errorf("resting dial %q: %q's off edge %v reads %v, not 0 — its span must start at or past off", d.ID, tm.Param, off, u)
				}
			}
		}
	}
}

func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// render.seconds, the same at every tempo.
func voiceSecsOf(id string) float64 {
	if d, ok := audio.InstrumentDoc(instrument.ID(id)); ok {
		return d.Render.Seconds
	}
	return 0
}
