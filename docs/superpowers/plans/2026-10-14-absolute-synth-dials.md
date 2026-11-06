# Absolute Synth Dials Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **This repo:** NEVER stage or commit (user rule) — every "commit" point below is a checkpoint: run the listed tests and move on. Never `git stash`. Run Go through `../../.tools/go/bin/go` from `src/go`.

**Goal:** Make every Synth-tab macro dial's position a pure function of the instrument document (absolute spans, proportional-shift turning), expand the dial catalogue, and prove every offered dial moves params AND the played audio.

**Architecture:** `internal/synthdial` gains a span coordinate, `Reading` and `Turn` (pure); the UI drops all dial session state and keeps only a gesture-scoped capture; `synth_dials.json` is re-expressed in absolute spans and extended; a real-render measurability test is the contract.

**Tech Stack:** Go 1.26 (bundled), Ebiten (ebitenstub under `-tags test`), C DSP via cgo for `!test` real renders.

**Spec:** `docs/superpowers/specs/2026-10-13-absolute-synth-dials-design.md`

## Global Constraints

- Dial reading = `Σ wᵢ·clamp(uᵢ,0,1)/Σ wᵢ` over LIVE terms; UI shows `round(100·reading)` as an integer 0…100.
- `Turn(start, target)`: target == reading → start verbatim; up moves `uᵢ<1` to `uᵢ+(1−uᵢ)k`; down moves `uᵢ>0` to `uᵢ(1−k)`; `k` closed-form when all `uᵢ∈[0,1]`, else bisection; `u==1 → to`, `u==0 → from` exactly.
- Nothing about a dial is stored on `DrumView` or in any file; the only state is a gesture capture.
- One detent = ONE `audio.SetInstrumentParamsLive`; release/Accept = ONE `commitInstrumentParams`; Esc = start values verbatim.
- `MaxDialsPerCard = 6`, laid out in rows of ≤ 3.
- Gates may not read a param driven by their own dial.
- Every shipped live term value lies inside its span; spans inside the param range.
- Calibration: dials without `rest:"start"` — median shipped reading ∈ [20, 80], ≤ 25 % of instruments within 5 of an end.
- Measurability: every end ≥ 10 units from the shipped reading moves ≥ 1 term by ≥ 5 % of its span and is audible in the played voice; the 0↔100 sweep is audible; `synth_dial_known_weak.json` must END EMPTY (leftovers need the user's sign-off).
- UI work is not done without inspected screenshots (desktop + `MOBILE=1`, `XDG_CONFIG_HOME` isolated).
- No dial gesture auditions.

## Review Focus

1. **A raw knob edit outside a term's span** (user drags `filter_cutoff` to 20 Hz below an 80 Hz span floor) — the reading clamps to 0 and the next Bright turn up moves that term continuously back in, never the wrong way. Pinned in Task 1 (`TestTurnOutOfSpanTermsNeverMoveTheWrongWay`).
2. **A turn that flips another dial's liveness** (Length up past the voice length hides Hold) — the card's dial row must not reshuffle mid-drag. Pinned in Task 3 (`TestDialRowFrozenDuringGesture`).
3. **Undo/redo and re-import while a dial reads non-50** — the reading must be identical after each (no hidden state). Pinned in Task 3 (`TestSynthDialReadingIsAFunctionOfTheDocument`).
4. **A dial whose terms are all at one edge** (Drive at 0 on a clean voice) — dragging down is a no-op that commits nothing (no undo step, no log line). Pinned in Task 3 (`TestDialAtItsEdgeCommitsNothing`).
5. **Magnitude terms with a negative value** (a downward pitch drop) — turning keeps the sign and grows/shrinks |v|. Pinned in Task 1 (`TestMagnitudeKeepsSign`).

---

### Task 1: `synthdial` — span coordinate, Reading, Turn

**Files:**
- Modify: `src/go/internal/synthdial/dial.go` (Term fields, Load validation)
- Create: `src/go/internal/synthdial/coord.go` (span + coordinate)
- Create: `src/go/internal/synthdial/turn.go` (`Reading`, `Turn`)
- Modify: `src/go/internal/synthdial/resolve.go` (drop `Reach`/`ResolveReachable`/`Apply`/`travels`; LiveTerm carries span + range)
- Delete: `src/go/internal/synthdial/morph.go`
- Test: rewrite `src/go/internal/synthdial/synthdial_test.go`

**Interfaces:**
- Produces:
  - `type Term struct { Param string; From, To *float64; Space Space; Weight float64; Requires string; RequiresMin, RequiresBelow float64; When string }` (`json:"from,omitempty"` …), `func (t Term) RequirementMet(v float64) bool`, `func (t Term) W() float64` (weight, 1 when 0).
  - `type Dial struct { ID string; Cards []string; Label, Gloss, Metric string; MetricFalls bool; Rest string; Terms []Term }`
  - `type Range struct{ Min, Max float64 }`, `type Span struct{ From, To float64 }`
  - `type LiveTerm struct { Term; Range Range; Span Span }`, `func (t LiveTerm) Coord(v float64) float64`, `func (t LiveTerm) Value(u, ref float64) float64` (ref = current value, for magnitude sign)
  - `func SpanFor(t Term, r Range) Span`
  - `type Resolved struct { Dial Dial; Terms []LiveTerm }`, `func (r Resolved) Reading(values map[string]float64) float64`, `func (r Resolved) Turn(start map[string]float64, target float64) map[string]float64`
  - `func Resolve(f File, card string, live Live) []Resolved` (cap `MaxDialsPerCard = 6`)
  - `func Load(b []byte) (File, error)`

- [ ] **Step 1: Write the failing tests** — `synthdial_test.go`:

```go
package synthdial

import (
	"math"
	"math/rand"
	"testing"
)

func f64(v float64) *float64 { return &v }

func lt(param string, sp Space, from, to float64, w float64, r Range) LiveTerm {
	t := Term{Param: param, Space: sp, From: f64(from), To: f64(to), Weight: w}
	return LiveTerm{Term: t, Range: r, Span: SpanFor(t, r)}
}

func TestCoordValueRoundTrip(t *testing.T) {
	cases := []LiveTerm{
		lt("a", SpaceLinear, 0, 1, 1, Range{0, 1}),
		lt("b", SpaceLinear, 1, 0, 1, Range{0, 1}), // falls
		lt("c", SpaceLog, 80, 18000, 1, Range{20, 20000}),
		lt("d", SpaceMagnitude, 0, 12, 1, Range{-24, 24}),
	}
	for _, c := range cases {
		for _, u := range []float64{0, 0.13, 0.5, 0.77, 1} {
			v := c.Value(u, 1)
			if got := c.Coord(v); math.Abs(got-u) > 1e-12 {
				t.Errorf("%s: Coord(Value(%v)) = %v", c.Param, u, got)
			}
		}
		if c.Value(0, 1) != c.Span.From || c.Value(1, 1) != c.Span.To {
			t.Errorf("%s: endpoints are not exact", c.Param)
		}
	}
}

func TestReadingIsWeightedClampedMean(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{
		lt("a", SpaceLinear, 0, 1, 1, Range{0, 1}),
		lt("b", SpaceLinear, 0, 10, 3, Range{0, 20}),
	}}
	got := r.Reading(map[string]float64{"a": 0.2, "b": 15}) // b clamps to 1
	if want := (0.2*1 + 1*3) / 4; math.Abs(got-want) > 1e-12 {
		t.Fatalf("Reading = %v, want %v", got, want)
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
				t.Fatalf("case %d target %v: Reading(Turn) = %v", i, target, got)
			}
			if got < prev-1e-12 {
				t.Fatalf("case %d: not monotone at %v", i, target)
			}
			prev = got
			for _, lt := range r.Terms {
				if v := out[lt.Param]; v < lt.Range.Min-1e-12 || v > lt.Range.Max+1e-12 {
					t.Fatalf("case %d: %s = %v outside %v", i, lt.Param, v, lt.Range)
				}
			}
		}
	}
}

func TestTurnEndpointsAreExactAndIdentityAtTheReading(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 500; i++ {
		r, start := randomResolved(rng)
		top, bottom := r.Turn(start, 1), r.Turn(start, 0)
		for _, tm := range r.Terms {
			if top[tm.Param] != clampTo(tm.Span.To, tm) && tm.Space != SpaceMagnitude {
				t.Fatalf("case %d: %s at 100 = %v, want span end %v", i, tm.Param, top[tm.Param], tm.Span.To)
			}
			if bottom[tm.Param] != clampTo(tm.Span.From, tm) && tm.Space != SpaceMagnitude {
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

func clampTo(v float64, t LiveTerm) float64 { return math.Min(math.Max(v, t.Range.Min), t.Range.Max) }

func TestTurnOutOfSpanTermsNeverMoveTheWrongWay(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{
		lt("cut", SpaceLog, 80, 18000, 1, Range{20, 20000}),
		lt("crisp", SpaceLinear, 0, 1, 1, Range{0, 1}),
	}}
	start := map[string]float64{"cut": 20, "crisp": 0.5} // cut below its span
	up := r.Turn(start, r.Reading(start)+0.2)
	if up["cut"] < 20 {
		t.Fatalf("turning up moved cut down to %v", up["cut"])
	}
	down := r.Turn(start, r.Reading(start)-0.2)
	if down["cut"] != 20 {
		t.Fatalf("turning down moved an already-below-span term: %v", down["cut"])
	}
}

func TestMagnitudeKeepsSign(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{lt("drop", SpaceMagnitude, 0, 12, 1, Range{-24, 24})}}
	start := map[string]float64{"drop": -6}
	if v := r.Turn(start, 1)["drop"]; v != -12 {
		t.Fatalf("magnitude up from −6 = %v, want −12", v)
	}
	if v := r.Turn(start, 0)["drop"]; v != 0 {
		t.Fatalf("magnitude down = %v, want 0", v)
	}
}

func TestLogFloorReadsAsZero(t *testing.T) {
	r := Resolved{Terms: []LiveTerm{lt("atk", SpaceLog, 0.001, 2, 1, Range{0, 2})}}
	if got := r.Reading(map[string]float64{"atk": 0}); got != 0 {
		t.Fatalf("an attack of 0 reads %v, want 0", got)
	}
}

func TestLoadRejectsEachStructuralFault(t *testing.T) {
	base := `{"schema_version":"0.0.1","dials":[{"id":"x","cards":["c"],"label":"l","gloss":"g","metric":"","terms":[%s]}]}`
	bad := map[string]string{
		"negative weight":   `{"param":"p","weight":-1}`,
		"unknown space":     `{"param":"p","space":"cubic"}`,
		"empty span":        `{"param":"p","from":1,"to":1}`,
		"log non-positive":  `{"param":"p","space":"log","from":0,"to":1}`,
		"log without span":  `{"param":"p","space":"log"}`,
		"magnitude negative": `{"param":"p","space":"magnitude","from":-1,"to":2}`,
		"orphan requires_min": `{"param":"p","requires_min":0.1}`,
		"unknown field jnd": `{"param":"p","jnd":0.1}`,
	}
	for name, term := range bad {
		if _, err := Load([]byte(fmt.Sprintf(base, term))); err == nil {
			t.Errorf("%s: Load accepted it", name)
		}
	}
	if _, err := Load([]byte(fmt.Sprintf(base, `{"param":"p","from":0,"to":1}`))); err != nil {
		t.Errorf("a well-formed file was rejected: %v", err)
	}
}

func TestResolveDropsDeadTermsAndDialsAndCapsAtSix(t *testing.T) { /* 7 dials on one card, one dead → 6 resolved in file order; a dial with only dead terms is dropped */ }
```

(The last test body: build a `File` with 8 dials on card `c`; dial 2's only term is dead (live returns false); assert `len(Resolve)==6`, ids in file order skipping dial 2.)

- [ ] **Step 2: Run, verify failure** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/synthdial/` → FAIL (undefined `SpanFor`, `Reading`, `Turn`).

- [ ] **Step 3: Implement**

`coord.go`:

```go
package synthdial

import "math"

// Span is a term's absolute travel: its param's value at dial 0 (From) and at
// dial 100 (To). To < From means the param falls as the dial rises.
type Span struct{ From, To float64 }

// SpanFor resolves a term's span against its param's range: an omitted end
// defaults to the range (magnitude: 0 → the larger edge).
func SpanFor(t Term, r Range) Span {
	s := Span{From: r.Min, To: r.Max}
	if t.space() == SpaceMagnitude {
		s = Span{From: 0, To: math.Max(r.Max, -r.Min)}
	}
	if t.From != nil {
		s.From = *t.From
	}
	if t.To != nil {
		s.To = *t.To
	}
	return s
}

func (t LiveTerm) s(v float64) float64 {
	switch t.space() {
	case SpaceLog:
		return math.Log(math.Max(v, math.Min(t.Span.From, t.Span.To)))
	case SpaceMagnitude:
		return math.Abs(v)
	}
	return v
}

// Coord is v's position on the term's span (unclamped: <0 below From, >1 past To).
func (t LiveTerm) Coord(v float64) float64 {
	a, b := t.s(t.Span.From), t.s(t.Span.To)
	return (t.s(v) - a) / (b - a)
}

// Value is the param value at coordinate u (exact at 0 and 1), clamped to the
// param range. ref is the current value: a magnitude term keeps its sign.
func (t LiveTerm) Value(u, ref float64) float64 {
	var v float64
	switch {
	case u == 0:
		v = t.Span.From
	case u == 1:
		v = t.Span.To
	default:
		a, b := t.s(t.Span.From), t.s(t.Span.To)
		x := a + u*(b-a)
		if t.space() == SpaceLog {
			x = math.Exp(x)
		}
		v = x
	}
	if t.space() == SpaceMagnitude && ref < 0 {
		v = -v
	}
	return math.Min(math.Max(v, t.Range.Min), t.Range.Max)
}
```

`turn.go`:

```go
package synthdial

// Reading is the dial's position (0..1) for these param values: the
// weighted mean of each live term's clamped span coordinate. A pure function
// of the values — the dial stores nothing.
func (r Resolved) Reading(values map[string]float64) float64 {
	sum, w := 0.0, 0.0
	for _, t := range r.Terms {
		v, ok := values[t.Param]
		if !ok {
			continue
		}
		sum += t.W() * clamp(t.Coord(v), 0, 1)
		w += t.W()
	}
	if w == 0 {
		return 0
	}
	return sum / w
}

// Turn returns the values that put the dial at target (0..1), moving every
// term proportionally from start toward the end being turned to (each term
// keeps its offset). A term already at or past that end stays.
func (r Resolved) Turn(start map[string]float64, target float64) map[string]float64 {
	target = clamp(target, 0, 1)
	out := make(map[string]float64, len(r.Terms))
	for _, t := range r.Terms {
		if v, ok := start[t.Param]; ok {
			out[t.Param] = v
		}
	}
	cur := r.Reading(start)
	if target == cur {
		return out
	}
	up := target > cur
	us := make([]float64, len(r.Terms))
	inside := true
	for i, t := range r.Terms {
		if v, ok := start[t.Param]; ok {
			us[i] = t.Coord(v)
			if us[i] < 0 || us[i] > 1 {
				inside = false
			}
		}
	}
	moved := func(u, k float64) (float64, bool) {
		if up && u < 1 {
			return u + (1-u)*k, true
		}
		if !up && u > 0 {
			return u * (1 - k), true
		}
		return u, false
	}
	readingAt := func(k float64) float64 {
		sum, w := 0.0, 0.0
		for i, t := range r.Terms {
			if _, ok := start[t.Param]; !ok {
				continue
			}
			u, _ := moved(us[i], k)
			sum += t.W() * clamp(u, 0, 1)
			w += t.W()
		}
		return sum / w
	}
	var k float64
	switch {
	case target == 1 || target == 0:
		k = 1
	case inside && up:
		k = (target - cur) / (1 - cur)
	case inside:
		k = (cur - target) / cur
	default:
		lo, hi := 0.0, 1.0
		for i := 0; i < 64; i++ {
			mid := (lo + hi) / 2
			if (readingAt(mid) < target) == up {
				lo = mid
			} else {
				hi = mid
			}
		}
		k = (lo + hi) / 2
	}
	for i, t := range r.Terms {
		v, ok := start[t.Param]
		if !ok {
			continue
		}
		if u, m := moved(us[i], k); m {
			if k == 1 {
				u = map[bool]float64{true: 1, false: 0}[up]
			}
			out[t.Param] = t.Value(u, v)
		}
	}
	return out
}
```

`dial.go`: replace the `Term` struct per the Interfaces block (`From/To *float64`, `Weight float64 json:"weight,omitempty"`, no `JND`), add `Dial.Rest string json:"rest,omitempty"`, `func (t Term) W() float64 { if t.Weight == 0 { return 1 }; return t.Weight }`, `MaxDialsPerCard = 6`. `Validate`: weight `< 0` → error; `From`/`To` both set and equal → error; log → both set and > 0 required; magnitude → set ends ≥ 0; `Rest` ∈ {"", "start"}. Keep `clamp` (move it from morph.go into coord.go). `resolve.go`: `LiveTerm{Term, Range, Span}`, `Live func(Term) (Range, bool)`, Resolve fills `Span: SpanFor(t, r)`; delete `ResolveReachable`, `Apply`, `Reach`, `travels`. Delete `morph.go`.

- [ ] **Step 4: Run** `go test -tags test -modfile=go.test.mod ./internal/synthdial/` → PASS. (`./internal/ui` will not compile yet — Task 2/3 fix it.)

---

### Task 2: Re-express `synth_dials.json` in spans + schema-aware discipline

**Files:**
- Modify: `src/go/internal/assets/synth_dials.json`
- Modify: `src/go/internal/ui/synth_dials_discipline_test.go`

**Interfaces:**
- Consumes: Task 1 `Term`, `SpanFor`, `LiveTerm.Coord`, `Resolved.Reading`.
- Produces: `dialSpanReport(t) map[string][]float64` (dial id → shipped readings), used by the calibration test.

- [ ] **Step 1: Convert every existing term.** Rules: positive `weight` → `from`=range low edge side, `to`=high (omit when it equals the param range); negative `weight` → swap (`from` = high, `to` = low); drop `jnd`; `weight` field now = influence (start at 1, keep `0.5` on minor terms like `gen1_harm_mix`); log terms get explicit spans (`filter_cutoff` 80→18000, `amp_attack` 0.001→2, `lfo_rate` per its range, `formant_shift` its range, `pitchenv_decay` its range falling). Remove `post_decay_rate` from Length and `settles` from Length's `amp_decay` (self-referential). Add `rest:"start"` to Drive, Weight, Room, Depth, Delay, Vibrato, Looseness, Drift, Ring, Body.

- [ ] **Step 2: Write the new discipline tests** in `synth_dials_discipline_test.go`:
  - `TestDialGatesNeverReadTheirOwnDial` — for each dial, collect its term params; any term whose `Requires` is in that set, or whose `When` reads one (table `synthDialConditionReads = map[string][]string{"ensemble": {...}, "release": {"amp_sustain","amp_decay"}, "settles": {"amp_decay"}, "pitchenv": {"osc_enabled","osc_type"}, "morphs": {"formant_morph_to","formant_vowel"}}` — declared next to `synthDialConditions` and checked for completeness), fails.
  - `TestDialSpansLieInsideTheParamRange` — per roster instrument declaring the param.
  - `TestShippedValuesLieInsideTheirSpans` — for every shipped instrument, every LIVE term (via `buildMacroGroups`), `Coord(value) ∈ [0,1]` (± 1e-9).
  - `TestDialCalibrationSpread` — `t.Logf` min/median/max per dial; for dials with `Rest==""`: median ∈ [0.20, 0.80] and ≤ 25 % within 0.05 of an end; for `rest:"start"` dials: `Span.From` equals the param's neutral (0 for every listed param — assert `SpanFor(...).From == 0` or the declared from equals 0).
  - Update `TestNoInstrumentLosesADialToTheCap` (cap 6, delete `synthDialCapAccepted`) and `TestNoDialDrivesAPickerOrTuning` (log check now on span > 0).

- [ ] **Step 3: Run** `go test -tags test -modfile=go.test.mod -run 'SynthDial|Dial' ./internal/ui/` after Task 3 compiles; iterate spans until `TestShippedValuesLieInsideTheirSpans` and `TestDialCalibrationSpread` pass (widen spans to hold outliers; move spans so medians centre).

---

### Task 3: UI — pure reading, gesture capture, no session state

**Files:**
- Modify: `src/go/internal/ui/synth_dials.go` (delete state; add reading + gesture)
- Modify: `src/go/internal/ui/synth_dial_input.go`
- Modify: `src/go/internal/ui/synth_macro_render.go` (`macroCardDials`, draw loop, no self-check)
- Modify: `src/go/internal/ui/synth_dial_render.go` (slot `reading int`, 0…100 arc + readout)
- Modify: `src/go/internal/ui/synth_panel_zone.go:1282` (freeze macros during a gesture)
- Modify: `src/go/internal/ui/drumview.go:504` (field), `src/go/internal/ui/import.go:119` (delete)
- Modify: `src/go/internal/ui/scene_catalog.go` (`synthDialSceneTurns` = absolute 0…100)
- Modify tests: `synth_dials_input_test.go`, `synth_dial_live_apply_test.go`, `synth_dial_realtime_test.go`, `synth_dial_export_import_test.go`, `export_import_master_audio_identity_test.go` (`turnHeadlineDial`)

**Interfaces:**
- Consumes: Task 1 `Resolved.Reading/Turn`.
- Produces:
  - `func (dv *DrumView) synthDialReading(inst string, d synthdial.Resolved) int` (0…100)
  - `type synthDialGesture struct { inst, card, dial string; d synthdial.Resolved; start map[string]float64; startReading int }`
  - `func (dv *DrumView) beginSynthDialGesture(inst, card, dial string) (*synthDialGesture, bool)`
  - `func (dv *DrumView) turnSynthDial(g *synthDialGesture, target int)` (live write; no commit)
  - `func (dv *DrumView) endSynthDialGesture(g *synthDialGesture, commit bool)` (commit when values differ from start; else restore start when !commit)
  - `func (dv *DrumView) setSynthDialReading(inst string, d synthdial.Resolved, target int)` — one-shot (begin + turn + end-without-commit) for scenes/tests.
  - DrumView field `synthDialActive *synthDialGesture` (non-nil = freeze the macro cache for `inst`).

- [ ] **Step 1: Write failing tests** (`synth_dials_input_test.go`, real input via `frameAt` / `dragFrom`):
  - `TestSynthDialDragAppliesTurnExactlyAndCommitsOnce`: read `start := audio.MergedInstrumentParamsRO(inst)`, `r0 := dv.synthDialReading`; drag Bright up 60 px (= +30); assert the audio layer's values for every live term == `d.Turn(start, float64(r0+30)/100)` exactly; one undo step; one `EventInstrumentParamsCommitted`; no drill.
  - `TestSynthDialDragBackRestoresExactly`: drag +40 then back to the press point in one gesture → params bit-equal to start, no undo step recorded.
  - `TestSynthDialTapOpensTheWheelAndCardTapDrills` (keep; wheel range 0..100, value = reading).
  - `TestSynthDialWheelEscRestoresVerbatim`.
  - `TestRawKnobEditMovesTheReading`: `audio.SetInstrumentParam(inst,"filter_cutoff",x)`, re-layout → displayed slot reading == `round(100*d.Reading(current))`.
  - `TestSynthDialReadingIsAFunctionOfTheDocument`: turn Bright→80, Length→30 on violin and Weight→60 on row 2; record readings on root for both; (a) export → `newViolinMacroGame` fresh Game → import → readings equal; (b) export → `audio.ResetInstrumentDocs` + reset globals → import → equal; (c) `undo` then `redo` → equal; (d) switch tab away and back → equal.
  - `TestDialRowFrozenDuringGesture`: on an instrument where Length up flips Hold (find via `synthDialConditions["settles"]` and a decay crossing the voice length; violin with `amp_decay` near voice length), press on Length, drag to 100, assert the card's `nDials` and dial ids unchanged across every drag frame; after release the row may change.
  - `TestDialAtItsEdgeCommitsNothing`: Drive on violin reads 0; drag down 60 px → no undo step, no commit event.

- [ ] **Step 2: Implement** `synth_dials.go`: delete `synthDialKey`, `synthDialState`, `synthDialPos`, `synthDialSelfCheck`, `synthDialValuesMoved`, `setSynthDial`, `synthDialReadBack`, `synthDialSnapshot`, `snapshotSynthDial`, `restoreSynthDial`, `cloneFloatMap` (if unused), `synthDialOffset*`, `synthDialCentreSnap`, `snapSynthDialPos`, `dialOfferedEnds`, `synthDialReach`. `resolveCardDials` calls `synthdial.Resolve`. Add:

```go
func (dv *DrumView) synthDialReading(inst string, d synthdial.Resolved) int {
	return int(math.Round(100 * d.Reading(audio.MergedInstrumentParamsRO(inst))))
}

func (dv *DrumView) beginSynthDialGesture(inst, card, dial string) (*synthDialGesture, bool) {
	d, ok := dv.synthDialByID(card, dial)
	if !ok || inst == "" {
		return nil, false
	}
	cur := audio.MergedInstrumentParamsRO(inst)
	start := make(map[string]float64, len(d.Terms))
	for _, t := range d.Terms {
		if v, ok := cur[t.Param]; ok {
			start[t.Param] = v
		}
	}
	g := &synthDialGesture{inst: inst, card: card, dial: dial, d: d, start: start,
		startReading: int(math.Round(100 * d.Reading(start)))}
	dv.synthDialActive = g
	return g, true
}

func (dv *DrumView) turnSynthDial(g *synthDialGesture, target int) {
	target = max(0, min(100, target))
	vals := g.start
	if target != g.startReading {
		vals = g.d.Turn(g.start, float64(target)/100)
	}
	audio.SetInstrumentParamsLive(g.inst, vals)
	g.last = target
}

func (dv *DrumView) endSynthDialGesture(g *synthDialGesture, commit bool) {
	if dv.synthDialActive == g {
		dv.synthDialActive = nil
	}
	changed := false
	cur := audio.MergedInstrumentParamsRO(g.inst)
	for p, v := range g.start {
		if cur[p] != v {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	if commit {
		dv.commitInstrumentParams(g.inst)
		return
	}
	audio.SetInstrumentParamsLive(g.inst, g.start)
}
```

(`synthDialGesture` also carries `last int`, the last written target.) `setSynthDialReading(inst, d, target)`: build the gesture from `d` directly (not by id), `turnSynthDial`, clear `synthDialActive` without commit/restore — used by scenes and real-render tests.

`synth_dial_input.go`: the adapter holds `g *synthDialGesture`; `OnPress` → `begin`; `OnDrag` → after `tapMaxMovePx`, `target := g.startReading + (h.startY-y)*100/synthDialDragSpanPx`; magnet: `if abs(target-g.startReading) <= 1 { target = g.startReading }`; skip when `target == g.last`; `turnSynthDial`; `invalidateSynthPanel`. `OnRelease`: moved → `end(g, true)`; not moved → `end(g,false)` + open wheel. Wheel: `NumberModel{Min:0, Max:100, Value: reading}`, format = `strconv.Itoa`, ladder format `"±"+step`; `OnChange` → `turnSynthDial(g, int(m.Value))`; `OnCancel` → `end(g,false)`; `OnCommit` → `end(g,true)`.

`synth_panel_zone.go`: in the macro-cache block, `if a := dv.synthDialActive; a != nil && a.inst == resolved && macros != nil { /* keep macros, keep key */ } else if macros == nil || key != … { rebuild }`.

`synth_dial_render.go`: `macroDialSlot{id, label string; reading int; dim bool}`; arc track full 270° (`top−sweep … top+sweep`), value arc from `top−sweep` to `top−sweep + 2·sweep·reading/100`; no tick; readout `strconv.Itoa(reading)`. `synth_macro_render.go`: `macroCardDials` fills `reading: dv.synthDialReading(resolved, d)`; draw loop re-reads readings (no self-check).

Scenes: `synthDialSceneTurns = map[string]int{"brightness/bright": 85, "brightness/edge": 60, "length/length": 30, "length/tail": 20}` applied by `dv.setSynthDialReading`. `turnHeadlineDial` uses the hit adapter (begin/turn/end) with an absolute target.

- [ ] **Step 3: Rewrite the remaining test files** that referenced the deleted API: `synth_dial_live_apply_test.go` (one `SetInstrumentParamsLive` per detent via `turnSynthDial`), `synth_dial_realtime_test.go`, `synth_dial_export_import_test.go` (no "state cleared" assertion; add "readings equal after import"), the `!test` render/audible tests compile against `setSynthDialReading` (logic rewritten in Task 6).

- [ ] **Step 4: Run** `go test -tags test -modfile=go.test.mod ./internal/synthdial/ ./internal/ui/ -run 'Dial|Macro|Synth'` → PASS; then `go vet -tags '!test' ./internal/ui/` equivalent: `../../.tools/go/bin/go vet ./internal/ui/` (compiles the `!test` files).

---

### Task 4: Six dials per card — multi-row layout

**Files:**
- Modify: `src/go/internal/ui/synth_dial_render.go` (`macroCardDialRects`, `macroDialRowsH`)
- Modify: `src/go/internal/ui/synth_macro_render.go` (`macroRootCardH(rows)`, `layoutMacroRoot` cell height)
- Test: `src/go/internal/ui/synth_dials_render_layout_test.go` (new, `-tags test`)

**Interfaces:**
- Produces: `func macroDialRows(n int) int` (= ceil(n/3)), `func macroRootCardH(rows int) int`.

- [ ] **Step 1: Failing test** `TestSixDialsLayOutInTwoRowsWithoutOverlap`: for n=1..6 and card widths {220, 300, 390-mobile}, `macroCardDialRects(card, n)` returns n non-empty, pairwise non-overlapping rects inside the card; n ≤ 3 → one row; 4..6 → two rows; a too-narrow card (width 90) still returns n rects (one per row) instead of none.
- [ ] **Step 2: Implement**: `perRow := min(3, n)`; if the cell width < `SynthDialD + 24` (arc + text), reduce `perRow` until it fits (≥1); rows = ceil(n/perRow); each row `macroDialRowH()` tall with `SpaceXS` between. `macroRootCardH(rows) = 2*inset + topH + rows*(rowH+SpaceXS)`; `layoutMacroRoot` uses the max rows over all cards (uniform card height).
- [ ] **Step 3: Run** layout test + `TestSynthDrawCallBudget*` / alloc budgets; update budgets only with a one-line rationale if they moved.

---

### Task 5: Catalogue expansion

**Files:**
- Modify: `src/go/internal/assets/synth_dials.json`
- Modify: `src/go/internal/ui/synth_dials.go` (`morphs` condition + `synthDialConditionReads`)
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go`

- [ ] **Step 1: Add terms** (spec §4): Snap + `gen1_tom_stick`, `gen1_atk_amt`; Length + `gen1_env_fast_rate`↓log, `gen1_env_tail_rate`↓log, `gen1_env_tail_mix`↑, `gen1_kick_env0`↓, `gen1_kick_hold`↑, `gen1_cym_rattle_decay`↑, `gen1_cym_rattle_hold`↑, `gen1_cym_decay_tilt`↓; Bright + `gen1_cym_mode_hi`↑log; Sizzle + `gen1_cym_rattle`↑; Bite + `osc_bow_slope`↑; Flam Spread/Strength cards += `punch`.
- [ ] **Step 2: Add dials**: `glassy` (sound: `osc_bow_pos` falling), `brilliance` (sound: `osc_bow_loss`, direction by metric `bright.centroid`), `bloom` (brightness: `filtenv_attack` log 0.001→1, requires `filtenv_amt`, rest start), `click` (attack + punch? NO — punch already has `gen1_atk_amt` in Snap; Click is melodic `attack` only: `gen1_atk_amt`↑, `gen1_atk_rate`↓log; rest start), `shine` (voice: `formant_sing`↑, rest start), `talk` (voice: `formant_morph_rate`↑log 0.05→8, `when: morphs`, rest start), `air` (boom: `gen1_kick_hiss`↑, rest start), `shimmer` (tone: `gen1_cym_beat`↑, `gen1_cym_tone_m`↑, rest start).
- [ ] **Step 3: `morphs` condition**: `func(e synthDialEnv) bool { return int(e.p["formant_morph_to"]+0.5) != int(e.p["formant_vowel"]+0.5) }`.
- [ ] **Step 4: i18n**: `KeySynthDial<Name>` + `Gloss` for the 8 new dials in `keys.go` (and its exhaustive list), en + es catalogues. Glosses (en): Glassy "Warm, or glassy near the bridge?"; Brilliance "Mellow string, or a ringing, brilliant one?"; Bloom "Bright at once, or blooming slowly?"; Click "Soft start, or a clicky attack?"; Shine "Plain voice, or a ringing singer's shine?"; Talk "Steady vowel, or talking between vowels?"; Air "Clean thump, or a breath of air?"; Shimmer "Plain wash, or a shimmering metallic ring?". Update glosses of dials whose terms grew (Length, Snap, Bite).
- [ ] **Step 5: Run** the discipline suite (Task 2 tests + coverage + i18n + cap) → PASS; recalibrate spans as needed.

---

### Task 6: Measurability contract (real renders, `make test-real` only)

**Files:**
- Modify: `src/go/internal/ui/synth_dials_audible_test.go` → `TestEveryDialIsMeasurable`
- Modify: `src/go/internal/ui/synth_dials_render_test.go` (`TestEveryDialMovesItsMetric` absolute)
- Modify: `src/go/internal/ui/testdata/synth_dial_known_weak.json`
- Modify: `src/go/internal/assets/synth_dials.json` (fixes)

- [ ] **Step 1: Rewrite the audibility test** — per shipped instrument (plus a stage-on variant per instrument for dials only reachable with a stage enabled: enable every `*_enabled` of the dial's terms' stages via `audio.SetInstrumentParamsLive`, re-resolve): `r0 := synthDialReading`; `shipped := RenderAsPlayed`; for each end e ∈ {0,100} with `|e−r0| ≥ 10`: `setSynthDialReading(id, d, e)`, assert some term moved ≥ 5 % of its span (`|Coord(new)−Coord(old)| ≥ 0.05`), render, `dialChangeAudible(played, shipped)`; also render at 0 and 100 and require `dialChangeAudible(at0, at100)`. Failures keyed `card/dial up|down|sweep` → ratchet file (`SYNTH_DIAL_WEAK_UPDATE=1` regenerates).
- [ ] **Step 2: Rewrite the metric test**: replace `d.Apply(anchor,pos)` with `d.Turn(anchor, end)`; judge each end ≥ 10 units from the shipped reading against the plain render (keep `dialMetricExceptions` semantics; keys `up`/`down`).
- [ ] **Step 3: Run** `cd src/go && xvfb-run -a ../../.tools/go/bin/go test -run 'TestEveryDialIsMeasurable|TestEveryDialMovesItsMetric' -count=1 ./internal/ui/ -timeout 3600s` (via `run_in_background`); triage each failure class: span too narrow → widen; term inaudible on a family → gate (`requires`/`when`) or drop; dial dead on an instrument → gate so it doesn't resolve. Loop until the weak list is empty or every leftover has a reason to show the user.

---

### Task 7: Visual verification + docs

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` (new scenes `crop_synth_dials_six` — `hihat` or the drum with the most Length dials, root scrolled to Length; `crop_synth_dials_voice` — a formant voice, root scrolled to Voice)
- Modify: `CLAUDE.md` (Synth macro dials paragraph), memory file `project_synth_macro_dials.md`

- [ ] **Step 1**: build `CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/…/beatmo_screenshot ./cmd`; capture `crop_synth_dials_root`, `_perc_root`, `_turned`, `synth_dials_popup`, `_six`, `_voice` desktop and mobile (`-mobile -scene-pass mobile -win-w 390 -win-h 844`) with `XDG_CONFIG_HOME=<scratch>`; open every PNG; fix and recapture until legible (labels, readouts, two rows, no overlap).
- [ ] **Step 2**: `TestRuntimeThemeSwitchMatchesBootEveryScene` / `TestNoColorRendersOutsideTheTheme` / `scene_crop_test` pass on the new scenes.
- [ ] **Step 3**: Rewrite CLAUDE.md's "Synth macro dials" paragraph (absolute spans, pure reading, gesture-only state, cap 6, measurability contract, `make test-real` guards); update memory.
- [ ] **Step 4**: Final: `go test -tags test -modfile=go.test.mod ./internal/synthdial/ ./internal/ui/ ./internal/i18n/` full; `make test-real`-scope dial tests; report results faithfully.
