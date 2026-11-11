package synthdial

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

// lt builds a live term with an explicit span over range r.
func lt(param string, sp Space, from, to, w float64, r Range) LiveTerm {
	t := Term{Param: param, Space: sp, From: f64(from), To: f64(to), Weight: w}
	return LiveTerm{Term: t, Range: r, Span: SpanFor(t, r)}
}

func TestCoordValueRoundTrip(t *testing.T) {
	cases := []LiveTerm{
		lt("a", SpaceLinear, 0, 1, 1, Range{0, 1}),
		lt("b", SpaceLinear, 1, 0, 1, Range{0, 1}), // falls as the dial rises
		lt("c", SpaceLog, 80, 18000, 1, Range{20, 20000}),
		lt("d", SpaceLog, 2, 0.001, 1, Range{0, 2}), // a falling log span
		lt("e", SpaceMagnitude, 0, 12, 1, Range{-24, 24}),
	}
	for _, c := range cases {
		for _, u := range []float64{0, 0.13, 0.5, 0.77, 1} {
			v := c.Value(u, 1)
			if got := c.Coord(v); math.Abs(got-u) > 1e-12 {
				t.Errorf("%s: Coord(Value(%v)) = %v", c.Param, u, got)
			}
		}
		if c.Value(0, 1) != c.Span.From || c.Value(1, 1) != c.Span.To {
			t.Errorf("%s: span ends are not exact: %v %v", c.Param, c.Value(0, 1), c.Value(1, 1))
		}
	}
}

func TestSpanDefaultsToTheParamRange(t *testing.T) {
	if s := SpanFor(Term{Param: "x"}, Range{-2, 5}); s != (Span{-2, 5}) {
		t.Errorf("linear default span = %v", s)
	}
	if s := SpanFor(Term{Param: "x", Space: SpaceMagnitude}, Range{-24, 12}); s != (Span{0, 24}) {
		t.Errorf("magnitude default span = %v, want 0 → the larger edge", s)
	}
	if s := SpanFor(Term{Param: "x", To: f64(3)}, Range{0, 10}); s != (Span{0, 3}) {
		t.Errorf("one declared end = %v", s)
	}
	// Ranges differ per instrument family (a bass's decay rate tops out at 12,
	// a conga's does not): a declared end beyond THIS instrument's range clips
	// to it, so the dial reads against the knob the user sees.
	if s := SpanFor(Term{Param: "x", From: f64(24), To: f64(0.5), Space: SpaceLog}, Range{0.5, 12}); s != (Span{12, 0.5}) {
		t.Errorf("clipped span = %v, want {12 0.5}", s)
	}
	if s := SpanFor(Term{Param: "x", From: f64(0), To: f64(30), Space: SpaceMagnitude}, Range{-12, 24}); s != (Span{0, 24}) {
		t.Errorf("clipped magnitude span = %v, want {0 24}", s)
	}
}

func TestReadingIsTheWeightedClampedMean(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{
		lt("a", SpaceLinear, 0, 1, 1, Range{0, 1}),
		lt("b", SpaceLinear, 0, 10, 3, Range{0, 20}),
	}}
	got := r.Reading(map[string]float64{"a": 0.2, "b": 15}) // b is past its span: counts as 1
	if want := (0.2*1 + 1*3) / 4; math.Abs(got-want) > 1e-12 {
		t.Fatalf("Reading = %v, want %v", got, want)
	}
	// An omitted weight counts as 1; a term with no value does not count.
	r2 := Resolved{Terms: []LiveTerm{lt("a", SpaceLinear, 0, 1, 0, Range{0, 1}), lt("z", SpaceLinear, 0, 1, 5, Range{0, 1})}}
	if got := r2.Reading(map[string]float64{"a": 0.4}); math.Abs(got-0.4) > 1e-12 {
		t.Fatalf("Reading = %v, want 0.4", got)
	}
}

func randomResolved(rng *rand.Rand) (Resolved, map[string]float64) {
	n := 1 + rng.Intn(5)
	r := Resolved{}
	vals := map[string]float64{}
	for i := 0; i < n; i++ {
		name := string(rune('a' + i))
		var t LiveTerm
		switch rng.Intn(3) {
		case 0:
			lo, hi := rng.Float64(), 1+rng.Float64()
			if rng.Intn(2) == 0 {
				lo, hi = hi, lo
			}
			t = lt(name, SpaceLinear, lo, hi, 0.2+rng.Float64(), Range{-1, 3})
			vals[name] = -1 + 4*rng.Float64() // may sit outside the span
		case 1:
			t = lt(name, SpaceLog, 0.01, 2, 0.2+rng.Float64(), Range{0, 4})
			vals[name] = 4 * rng.Float64()
		default:
			t = lt(name, SpaceMagnitude, 0, 12, 0.2+rng.Float64(), Range{-24, 24})
			vals[name] = -24 + 48*rng.Float64()
		}
		r.Terms = append(r.Terms, t)
	}
	return r, vals
}

// Turn lands EXACTLY on its target, is monotone in it, and never leaves a
// param's range — over random spans, spaces, weights and out-of-span values.
func TestTurnLandsExactlyAndIsMonotone(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		r, start := randomResolved(rng)
		prev := -1.0
		for k := 0; k <= 100; k++ {
			target := float64(k) / 100
			out := r.Turn(start, target)
			got := r.Reading(out)
			if math.Abs(got-target) > 1e-9 {
				t.Fatalf("case %d target %v: Reading(Turn) = %v (start reading %v)", i, target, got, r.Reading(start))
			}
			if got < prev-1e-12 {
				t.Fatalf("case %d: not monotone at %v", i, target)
			}
			prev = got
			for _, tm := range r.Terms {
				if v := out[tm.Param]; v < tm.Range.Min || v > tm.Range.Max {
					t.Fatalf("case %d: %s = %v outside %v", i, tm.Param, v, tm.Range)
				}
			}
		}
	}
}

// At 0 and 100 every term sits exactly on its span end (unless it already
// sat past it); turning to the
// current reading returns the start values bit for bit.
func TestTurnEndpointsAreExactAndIdentityAtTheReading(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 500; i++ {
		r, start := randomResolved(rng)
		top, bottom := r.Turn(start, 1), r.Turn(start, 0)
		for _, tm := range r.Terms {
			want := func(end float64) float64 {
				if tm.space() == SpaceMagnitude && start[tm.Param] < 0 {
					end = -end
				}
				return math.Min(math.Max(end, tm.Range.Min), tm.Range.Max)
			}
			u := tm.Coord(start[tm.Param])
			// A term already past the end being turned to stays put.
			if u >= 1 {
				if top[tm.Param] != start[tm.Param] {
					t.Fatalf("case %d: %s past its top moved at 100", i, tm.Param)
				}
			} else if top[tm.Param] != want(tm.Span.To) {
				t.Fatalf("case %d: %s at 100 = %v, want span end %v", i, tm.Param, top[tm.Param], tm.Span.To)
			}
			if u <= 0 {
				if bottom[tm.Param] != start[tm.Param] {
					t.Fatalf("case %d: %s below its bottom moved at 0", i, tm.Param)
				}
			} else if bottom[tm.Param] != want(tm.Span.From) {
				t.Fatalf("case %d: %s at 0 = %v, want span start %v", i, tm.Param, bottom[tm.Param], tm.Span.From)
			}
		}
		same := r.Turn(start, r.Reading(start))
		for _, tm := range r.Terms {
			if same[tm.Param] != start[tm.Param] {
				t.Fatalf("case %d: turning to the current reading moved %s", i, tm.Param)
			}
		}
	}
}

// Each term keeps its offset: turning up moves every term toward its top by
// the same fraction of its remaining travel.
func TestTurnMovesEveryTermProportionally(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{
		lt("cut", SpaceLinear, 0, 1, 1, Range{0, 1}),
		lt("crisp", SpaceLinear, 0, 1, 1, Range{0, 1}),
	}}
	out := r.Turn(map[string]float64{"cut": 0.2, "crisp": 0.4}, 0.65)
	if math.Abs(out["cut"]-0.6) > 1e-12 || math.Abs(out["crisp"]-0.7) > 1e-12 {
		t.Fatalf("turn 0.3 → 0.65 gave %v, want cut 0.6 and crisp 0.7 (half of each remaining travel)", out)
	}
}

// A raw edit can leave a term outside its span. Turning toward that side
// leaves it alone; turning away brings it back in continuously.
func TestTurnOutOfSpanTermsNeverMoveTheWrongWay(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{
		lt("cut", SpaceLinear, 100, 1000, 1, Range{20, 20000}),
		lt("crisp", SpaceLinear, 0, 1, 1, Range{0, 1}),
	}}
	start := map[string]float64{"cut": 40, "crisp": 0.5} // cut is below its span
	cur := r.Reading(start)
	if down := r.Turn(start, cur-0.2); down["cut"] != 40 {
		t.Fatalf("turning down moved an already-below-span term: %v", down["cut"])
	}
	prev := 40.0
	for k := 1; k <= 10; k++ {
		up := r.Turn(start, cur+(1-cur)*float64(k)/10)
		if up["cut"] < prev {
			t.Fatalf("turning up moved cut down: %v after %v", up["cut"], prev)
		}
		prev = up["cut"]
	}
	if prev != 1000 {
		t.Fatalf("at 100 cut = %v, want its span end 1000", prev)
	}
}

func TestMagnitudeKeepsSign(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{lt("drop", SpaceMagnitude, 0, 12, 1, Range{-24, 24})}}
	start := map[string]float64{"drop": -6}
	if got := r.Reading(start); got != 0.5 {
		t.Fatalf("reading of −6 over |0..12| = %v, want 0.5", got)
	}
	if v := r.Turn(start, 1)["drop"]; v != -12 {
		t.Fatalf("magnitude up from −6 = %v, want −12", v)
	}
	if v := r.Turn(start, 0.75)["drop"]; v != -9 {
		t.Fatalf("magnitude to 0.75 from −6 = %v, want −9", v)
	}
	if v := r.Turn(start, 0)["drop"]; v != 0 {
		t.Fatalf("magnitude down = %v, want 0", v)
	}
}

func TestLogFloorReadsAsTheSpanStart(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{lt("atk", SpaceLog, 0.001, 2, 1, Range{0, 2})}}
	if got := r.Reading(map[string]float64{"atk": 0}); got != 0 {
		t.Fatalf("an attack of 0 reads %v, want 0", got)
	}
	if v := r.Turn(map[string]float64{"atk": 0}, 0)["atk"]; v != 0 {
		t.Fatalf("turning a floored term to 0 rewrote it to %v", v)
	}
}

const goodFile = `{"schema_version": "0.0.1", "dials": [
 {"id":"bright","cards":["brightness","tone"],"label":"synth.dial.bright","gloss":"synth.dial.bright.gloss","metric":"bright.centroid","view":"colour","lo":"synth.dial.bright.lo","hi":"synth.dial.bright.hi","terms":[
   {"param":"filter_cutoff","from":80,"to":18000,"space":"log"},
   {"param":"post_darken","from":1,"to":0,"weight":0.7}]},
 {"id":"edge","cards":["brightness"],"label":"a","gloss":"b","metric":"","rest":"start","view":"colour","lo":"c","hi":"d","terms":[{"param":"filter_resonance"}]},
 {"id":"sweep","cards":["brightness"],"label":"a","gloss":"b","metric":"","view":"colour-time","lo":"c","hi":"d","terms":[{"param":"filtenv_amt"}]},
 {"id":"fourth","cards":["brightness"],"label":"a","gloss":"b","metric":"","view":"lfo-target","lo":"c","hi":"d","terms":[{"param":"post_brightness"}]},
 {"id":"fifth","cards":["brightness"],"label":"a","gloss":"b","metric":"","view":"pitch","lo":"c","hi":"d","terms":[{"param":"p5"}]},
 {"id":"sixth","cards":["brightness"],"label":"a","gloss":"b","metric":"","view":"wave","lo":"c","hi":"d","terms":[{"param":"p6"}]},
 {"id":"seventh","cards":["brightness"],"label":"a","gloss":"b","metric":"","view":"noise","lo":"c","hi":"d","terms":[{"param":"p7"}]}
]}`

func TestLoadAcceptsAWellFormedFile(t *testing.T) {
	f, err := Load([]byte(goodFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Dials) != 7 || f.Dials[0].Terms[0].space() != SpaceLog || f.Dials[0].Terms[1].space() != SpaceLinear {
		t.Fatalf("decoded wrong: %+v", f)
	}
	if f.Dials[0].Terms[1].W() != 0.7 || f.Dials[0].Terms[0].W() != 1 || f.Dials[1].Rest != "start" {
		t.Fatalf("weights/rest decoded wrong: %+v", f.Dials[:2])
	}
}

func TestLoadRejectsEachStructuralFault(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"version":         {`"0.0.1"`, `"9.9.9"`, "unsupported"},
		"duplicate id":    {`"id":"edge"`, `"id":"bright"`, "duplicate"},
		"no card":         {`"cards":["brightness","tone"]`, `"cards":[]`, "no card"},
		"negative weight": {`"weight":0.7`, `"weight":-0.7`, "weight"},
		"unknown space":   {`"space":"log"`, `"space":"exp"`, "space"},
		"unknown field":   {`"metric":"bright.centroid"`, `"metric":"bright.centroid","colour":1`, "unknown field"},
		"old jnd field":   {`{"param":"filter_resonance"}`, `{"param":"filter_resonance","jnd":0.5}`, "unknown field"},
		"no label":        {`"label":"synth.dial.bright"`, `"label":""`, "label"},
		"repeat param":    {`{"param":"post_darken"`, `{"param":"filter_cutoff"`, "twice"},
		"empty span":      {`"from":1,"to":0`, `"from":1,"to":1`, "span"},
		"log at zero":     {`"from":80,"to":18000`, `"from":0,"to":18000`, "log"},
		"log no span":     {`{"param":"filter_cutoff","from":80,"to":18000,"space":"log"}`, `{"param":"filter_cutoff","space":"log"}`, "log"},
		"negative mag":    {`{"param":"filtenv_amt"}`, `{"param":"filtenv_amt","space":"magnitude","from":-1,"to":2}`, "magnitude"},
		"unknown rest":    {`"rest":"start"`, `"rest":"middle"`, "rest"},
		"orphan req_min":  {`{"param":"filtenv_amt"}`, `{"param":"filtenv_amt","requires_min":0.1}`, "requires"},
		"unknown view":    {`"view":"colour-time"`, `"view":"sparkles"`, "view"},
		"no end words":    {`"lo":"synth.dial.bright.lo"`, `"lo":""`, "end word"},
	}
	for name, c := range cases {
		bad := strings.Replace(goodFile, c.from, c.to, 1)
		if bad == goodFile {
			t.Fatalf("%s: substitution did not apply", name)
		}
		if _, err := Load([]byte(bad)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got err %v, want one mentioning %q", name, err, c.want)
		}
	}
}

func TestResolveDropsDeadTermsAndDialsAndCapsAtSix(t *testing.T) {
	f, err := Load([]byte(goodFile))
	if err != nil {
		t.Fatal(err)
	}
	live := func(tm Term) (Range, bool) {
		switch tm.Param {
		case "filter_cutoff":
			return Range{20, 20000}, true
		case "post_darken", "post_brightness", "filtenv_amt", "filter_resonance", "p5", "p6", "p7":
			return Range{0, 4}, true
		}
		return Range{}, false
	}
	got := Resolve(f, "brightness", live)
	if len(got) != MaxDialsPerCard || MaxDialsPerCard != 6 {
		t.Fatalf("got %d dials, want the cap 6", len(got))
	}
	if got[5].Dial.ID != "sixth" {
		t.Errorf("file order: sixth dial = %q, want sixth (seventh is capped)", got[5].Dial.ID)
	}
	if sp := got[0].Terms[0].Span; sp != (Span{80, 18000}) {
		t.Errorf("resolved span = %v", sp)
	}
	if sp := got[1].Terms[0].Span; sp != (Span{0, 4}) {
		t.Errorf("default span = %v, want the range", sp)
	}
	// Bright loses its darken term on an instrument without it; Edge without
	// resonance drops entirely and lets the seventh in.
	noRes := func(tm Term) (Range, bool) {
		if tm.Param == "filter_resonance" || tm.Param == "post_darken" {
			return Range{}, false
		}
		return live(tm)
	}
	got = Resolve(f, "brightness", noRes)
	if fmt.Sprint(ids(got)) != "[bright sweep fourth fifth sixth seventh]" || len(got[0].Terms) != 1 {
		t.Errorf("dead term/dial must drop: got %v", ids(got))
	}
	if got := Resolve(f, "tone", live); len(got) != 1 || got[0].Dial.ID != "bright" {
		t.Errorf("a dial serves every card it lists: tone got %v", ids(got))
	}
}

func ids(rs []Resolved) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Dial.ID)
	}
	return out
}

func TestRequirementThresholds(t *testing.T) {
	gate := Term{Param: "lfo_rate", Requires: "lfo_depth", RequiresMin: 0.15}
	if gate.RequirementMet(0.025) || !gate.RequirementMet(0.2) {
		t.Error("requires_min: a 0.025 depth must gate the speed off and 0.2 on")
	}
	below := Term{Param: "x", Requires: "post_decay", RequiresBelow: 1}
	if below.RequirementMet(1) || !below.RequirementMet(0.6) {
		t.Error("requires_below: 1 must gate off and 0.6 on")
	}
	if !(Term{Param: "x"}).RequirementMet(0) {
		t.Error("a term with no Requires is always met")
	}
	plain := Term{Param: "body_mix", Requires: "body_model"}
	if plain.RequirementMet(0) || !plain.RequirementMet(1) {
		t.Error("a bare Requires still means non-zero")
	}
}

// Only a value at or below zero floors to a log span's start (0 = instant /
// off / no decay). A POSITIVE value below the span is out of span like any
// other — Coord < 0 for a rising span, > 1 for a falling one — so Turn moves
// it on its continuous path and the discipline guard can see it.
func TestLogFloorsOnlyNonPositiveValues(t *testing.T) {
	up := lt("cut", SpaceLog, 80, 18000, 1, Range{20, 20000})
	if u := up.Coord(20); u >= 0 {
		t.Fatalf("20 Hz on an 80→18k span: coord %v, want < 0", u)
	}
	if u := up.Coord(0); u != 0 {
		t.Fatalf("0 on a log span: coord %v, want the floor (0)", u)
	}
	down := lt("rate", SpaceLog, 40, 0.5, 1, Range{0, 100})
	if u := down.Coord(0.25); u <= 1 {
		t.Fatalf("rate 0.25 on a 40→0.5 span: coord %v, want > 1", u)
	}
	if u := down.Coord(0); u != 1 {
		t.Fatalf("rate 0 (no decay) on a 40→0.5 span: coord %v, want the long end (1)", u)
	}
}
