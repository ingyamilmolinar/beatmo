package main

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
)

// timing_test.go pins the one part of the pipeline with real logic rather than
// mapping: how a note's PERFORMED time — swing, tuplets, grace notes, rolls,
// breaths, fermatas, agogic spans — becomes a node on its WRITTEN step plus a
// delay/rush. The end-to-end guards (TestGrooveIsDerivedFromNotation,
// TestSwingDelaysExactlyTheSwungPositions) only prove groove appears where
// notation exists; these pin the value.

const testDiv = 12 // divisions per quarter: holds sixteenths AND triplets

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSwingShiftWarpsEachPair(t *testing.T) {
	eighths := musicxml.Direction{Kind: musicxml.DirSwing, SwingRatio: 2, SwingUnit: 0.5}
	cases := []struct{ pos, want float64 }{
		{0, 0},
		{0.5, 1.0 / 6},   // the offbeat eighth lands on the triplet's third
		{0.25, 1.0 / 12}, // a sixteenth inside a swung pair moves proportionally
		{0.75, 1.0 / 12},
		{1, 0},
		{1.5, 1.0 / 6},
	}
	for _, c := range cases {
		if got := swingShiftQuarters(c.pos, eighths); !near(got, c.want) {
			t.Errorf("2:1 eighth swing at %v quarters: shift %v, want %v", c.pos, got, c.want)
		}
	}
	mpc := musicxml.Direction{Kind: musicxml.DirSwing, SwingRatio: 1.5, SwingUnit: 0.25}
	if got := swingShiftQuarters(0.25, mpc); !near(got, 0.05) {
		t.Errorf("3:2 sixteenth swing on the offbeat sixteenth: %v quarters, want 0.05 (a fifth of a sixteenth)", got)
	}
	straight := musicxml.Direction{Kind: musicxml.DirSwing, SwingRatio: 1, SwingUnit: 0.5}
	if got := swingShiftQuarters(0.5, straight); !near(got, 0) {
		t.Errorf("1:1 swing must be straight, got %v", got)
	}
}

func TestOffsetBecomesDelayOrRushWithinOneStep(t *testing.T) {
	var st scoreStats
	if h := withOffset(Hit{Step: 3}, 1.0/3, &st); h.Groove != grooveDelay || h.GroovePct != 0.333 {
		t.Errorf("+1/3 step: %+v, want delay 0.333", h)
	}
	if h := withOffset(Hit{Step: 3}, -0.25, &st); h.Groove != grooveRush || h.GroovePct != 0.25 {
		t.Errorf("-1/4 step: %+v, want rush 0.25", h)
	}
	if h := withOffset(Hit{Step: 3}, 0.0001, &st); h.Groove != "" || h.GroovePct != 0 {
		t.Errorf("a rounding residue must not become a groove: %+v", h)
	}
	if st.GrooveClamped != 0 {
		t.Fatalf("clamped %d in-window offsets", st.GrooveClamped)
	}
	if h := withOffset(Hit{Step: 3}, 1.4, &st); h.Groove != grooveDelay || h.GroovePct != 1 || st.GrooveClamped != 1 {
		t.Errorf("an offset past the window must clamp to one step AND be counted: %+v, clamped=%d", h, st.GrooveClamped)
	}
}

func TestBreathMarkDelaysTheNextOnset(t *testing.T) {
	notes := []musicxml.Note{
		{OnsetDiv: 0, Articulations: musicxml.Articulations{BreathMark: true}},
		{OnsetDiv: 12},
		{OnsetDiv: 24, Articulations: musicxml.Articulations{Caesura: true}},
		{OnsetDiv: 36},
	}
	d := breathDelays(notes)
	if d[0] != 0 || d[12] != breathBeats || d[24] != 0 || d[36] != caesuraBeats {
		t.Errorf("breath delays %v: the mark delays the FOLLOWING onset", d)
	}
}

// testLayer wraps a single synthetic part as a loaded 4/4 layer of `bars` bars.
func testLayer(p musicxml.Part, bars int) loadedLayer {
	p.Divisions = testDiv
	for i := 0; i < bars; i++ {
		p.Measures = append(p.Measures, musicxml.Measure{Movement: 1, Number: i + 1, OnsetDiv: i * 4 * testDiv,
			LengthDiv: 4 * testDiv, Beats: 4, BeatType: 4})
	}
	for i := range p.Notes {
		n := &p.Notes[i]
		n.Movement = 1
		if m, ok := p.MeasureAt(n.OnsetDiv); ok {
			n.Measure = m.Number
		}
		if n.DurationDiv == 0 {
			n.DurationDiv = 3
		}
	}
	sc := musicxml.Score{Divisions: testDiv, Parts: []musicxml.Part{p}}
	pm := PartMap{Index: 0, Name: p.Name, Inst: roster.PianoGrand, Display: "Piano", MaxVoices: 3}
	return loadedLayer{
		spec: layerSpec{From: 1, To: bars, Parts: []PartMap{pm}},
		sc:   sc, div: testDiv, originDiv: 0, lengthDiv: bars * 4 * testDiv,
	}
}

func runLayer(t *testing.T, l loadedLayer, tm timing) ([]*rowAcc, scoreStats) {
	t.Helper()
	asm := &assembler{byKey: map[rowKey]*rowAcc{}}
	var st scoreStats
	if err := emitLayer(TemplateSpec{Stem: "test"}, 0, l, 1, tm, asm, &st); err != nil {
		t.Fatal(err)
	}
	return asm.rows, st
}

func hitsByStep(r *rowAcc) map[int]Hit { return r.hits }

// A grace note sounds just before its principal, on the step before it — and
// the principal (the written note) is kept even when the grace is higher.
// Grouped with its principal as a chord tone, a higher grace used to take the
// melody lane and push the principal out.
func TestGraceNotePrecedesItsPrincipal(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 0, Semitone: 0, Voice: 1, Staff: 1},
		{OnsetDiv: 12, Semitone: 5, Voice: 1, Staff: 1, Grace: true},
		{OnsetDiv: 12, Semitone: 3, Voice: 1, Staff: 1},
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0].MaxVoices = 1
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	if len(rows) != 1 {
		t.Fatalf("rows %d, want one melody lane", len(rows))
	}
	h := hitsByStep(rows[0])
	if h[4].Pitch != 3 {
		t.Errorf("principal at step 4 has pitch %v, want the written note 3", h[4].Pitch)
	}
	g, ok := h[3]
	if !ok || g.Pitch != 5 {
		t.Fatalf("grace not on the step before its principal: %+v", h)
	}
	want := 1 - graceLeadBeats*4
	if g.Groove != grooveDelay || math.Abs(g.GroovePct-want) > 0.001 {
		t.Errorf("grace performs %v steps after its written step, got %s %v", want, g.Groove, g.GroovePct)
	}
	if st.ChordTonesDropped != 0 {
		t.Errorf("dropped %d notes; a grace is not a chord tone", st.ChordTonesDropped)
	}
}

// A rolled chord sounds bottom-up: each higher lane a sixteenth of a beat later.
func TestArpeggioRollsTheChord(t *testing.T) {
	roll := musicxml.Articulations{Arpeggiate: 1}
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 0, Semitone: 0, Voice: 1, Staff: 1, Articulations: roll},
		{OnsetDiv: 0, Semitone: 4, Voice: 1, Staff: 1, Chord: true, Articulations: roll},
		{OnsetDiv: 0, Semitone: 7, Voice: 1, Staff: 1, Chord: true, Articulations: roll},
	}}
	rows, _ := runLayer(t, testLayer(p, 1), timing{stepsPerQuarter: 4})
	if len(rows) != 3 {
		t.Fatalf("rows %d, want three lanes", len(rows))
	}
	// Lane 0 is the TOP note, which a bottom-up roll plays last.
	for lane, wantRank := range []float64{2, 1, 0} {
		h := rows[lane].hits[0]
		want := roundPct(wantRank * arpRollBeats * 4)
		if wantRank == 0 {
			if h.Groove != "" {
				t.Errorf("lowest note of the roll is grooved: %+v", h)
			}
			continue
		}
		if h.Groove != grooveDelay || !near(h.GroovePct, want) {
			t.Errorf("lane %d: %s %v, want delay %v", lane, h.Groove, h.GroovePct, want)
		}
	}
}

// A tuplet note keeps its nearest written step and carries the difference.
func TestTupletOnsetsBecomeRushAndDelay(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 0, Semitone: 0, Voice: 1, Staff: 1},
		{OnsetDiv: 4, Semitone: 2, Voice: 1, Staff: 1}, // eighth triplet: 1/3 quarter
		{OnsetDiv: 8, Semitone: 4, Voice: 1, Staff: 1},
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0].MaxVoices = 1
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	h := rows[0].hits
	if x := h[1]; x.Pitch != 2 || x.Groove != grooveDelay || x.GroovePct != 0.333 {
		t.Errorf("second triplet note: %+v, want step 1 delayed 0.333", x)
	}
	if x := h[3]; x.Pitch != 4 || x.Groove != grooveRush || x.GroovePct != 0.333 {
		t.Errorf("third triplet note: %+v, want step 3 rushed 0.333", x)
	}
	if st.StepCollisions != 0 {
		t.Errorf("%d collisions: triplets fit a sixteenth grid as rush/delay", st.StepCollisions)
	}
}

// Swing reaches a part through the template-wide groove swing, and only the
// swung positions move.
func TestTemplateSwingAppliesToEveryLayer(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 0, Voice: 1, Staff: 1}, {OnsetDiv: 6, Voice: 1, Staff: 1},
		{OnsetDiv: 12, Voice: 1, Staff: 1}, {OnsetDiv: 18, Voice: 1, Staff: 1},
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0].MaxVoices = 1
	tm := timing{stepsPerQuarter: 4, swing: musicxml.Direction{Kind: musicxml.DirSwing, SwingRatio: 2, SwingUnit: 0.5}, haveSwing: true}
	rows, _ := runLayer(t, l, tm)
	h := rows[0].hits
	for _, step := range []int{0, 4} {
		if h[step].Groove != "" {
			t.Errorf("downbeat step %d grooved: %+v", step, h[step])
		}
	}
	for _, step := range []int{2, 6} {
		if h[step].Groove != grooveDelay || h[step].GroovePct != 0.667 {
			t.Errorf("offbeat step %d: %+v, want delay 0.667 (2:1 swing = 2/3 of a step)", step, h[step])
		}
	}
}

// Kit pieces that route to one instrument share its row; a same-step clash
// spills into a second row of that instrument instead of being dropped.
func TestKitPiecesShareARowAndSpillOnClash(t *testing.T) {
	p := musicxml.Part{Name: "Drums", Notes: []musicxml.Note{
		{OnsetDiv: 0, Unpitched: true, PercKey: 41, Voice: 1, Staff: 1},
		{OnsetDiv: 12, Unpitched: true, PercKey: 43, Voice: 1, Staff: 1},
		{OnsetDiv: 12, Unpitched: true, PercKey: 45, Voice: 2, Staff: 1},
		{OnsetDiv: 0, Unpitched: true, PercKey: 36, Voice: 2, Staff: 1},
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0] = PartMap{Index: 0, Name: "Drums", Display: "Drums"}
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	var names []string
	for _, r := range rows {
		names = append(names, r.inst.Name+"="+string(r.inst.ID))
	}
	if len(rows) != 3 || rows[0].inst.ID != roster.Kick || rows[1].inst.ID != roster.Tom || rows[2].inst.ID != roster.Tom {
		t.Fatalf("rows %v: want kick, toms, toms ·2", names)
	}
	if rows[1].hits[0].Pitch != gmKit[41].Pitch || rows[1].hits[4].Pitch != gmKit[43].Pitch {
		t.Errorf("tom heights not carried as pitch: %+v", rows[1].hits)
	}
	if rows[2].hits[4].Pitch != gmKit[45].Pitch || rows[2].inst.Name != "Toms ·2" {
		t.Errorf("clashing tom did not spill: %+v %q", rows[2].hits, rows[2].inst.Name)
	}
	if st.StepCollisions != 0 || st.NotesEmitted != 4 {
		t.Errorf("stats %+v", st)
	}
}

// A pickup before the loop's first downbeat wraps to the end of the loop.
func TestPickupWrapsToTheEndOfTheLoop(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 0, Semitone: 9, Voice: 1, Staff: 1, DurationDiv: 6},
		{OnsetDiv: 6, Semitone: 0, Voice: 1, Staff: 1},
	}}
	l := testLayer(p, 2)
	l.originDiv = 6 // the downbeat after a one-eighth pickup
	l.lengthDiv = 4 * testDiv
	l.spec.Parts[0].MaxVoices = 1
	rows, _ := runLayer(t, l, timing{stepsPerQuarter: 4})
	h := rows[0].hits
	if h[0].Pitch != 0 || h[14].Pitch != 9 {
		t.Errorf("hits %+v: want the downbeat note on step 0 and the pickup on step 14 (last eighth)", h)
	}
}

// TestGrooveWindowIsOneStep pins the layout the engine's groove window
// depends on: one grid unit per step, so the exported subdivision is steps
// per quarter and a groove percentage is a fraction of one step.
func TestGrooveWindowIsOneStep(t *testing.T) {
	sh := Showcase{Stem: "t", BPM: 100, Subdiv: 16, TotalSteps: 16,
		Insts: []InstSpec{{ID: roster.Kick, Name: "Kick", Volume: 1}},
		Rows:  []RowSpec{{Inst: roster.Kick, Hits: []Hit{{Step: 1}, {Step: 2}}}}}
	b, err := build(sh)
	if err != nil {
		t.Fatal(err)
	}
	var d doc
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatal(err)
	}
	if d.Subdiv != 4 {
		t.Fatalf("exported subdiv %d, want 4 units per beat (one per sixteenth step)", d.Subdiv)
	}
	var at1, at2 *node
	for i := range d.Nodes {
		n := &d.Nodes[i]
		if n.Type != "regular" {
			continue
		}
		if at1 == nil {
			at1 = n
		} else {
			at2 = n
		}
	}
	if at1 == nil || at2 == nil || math.Abs(float64(at2.I-at1.I))+math.Abs(float64(at2.J-at1.J)) != 1 {
		t.Errorf("adjacent steps must be one grid unit apart: %+v %+v", at1, at2)
	}
}

func TestAgogicSpanRampsAndCloses(t *testing.T) {
	part := musicxml.Part{Directions: []musicxml.Direction{
		{Kind: musicxml.DirRitardando, Measure: 1, OnsetDiv: 0},
		{Kind: musicxml.DirATempo, Measure: 1, OnsetDiv: 48},
	}}
	l := testLayer(part, 4)
	first, _ := agogicBeats(l.sc.Parts[0], musicxml.Note{OnsetDiv: 0, Measure: 1}, l)
	late, _ := agogicBeats(l.sc.Parts[0], musicxml.Note{OnsetDiv: 40, Measure: 1}, l)
	if !(first >= agogicMinBeats && late > first && late <= agogicMaxBeats) {
		t.Errorf("rit. must ramp from %v toward %v: got %v then %v", agogicMinBeats, agogicMaxBeats, first, late)
	}
	if _, ok := agogicBeats(l.sc.Parts[0], musicxml.Note{OnsetDiv: 60, Measure: 2}, l); ok {
		t.Error("a tempo did not close the span")
	}
	acc := testLayer(musicxml.Part{Directions: []musicxml.Direction{{Kind: musicxml.DirAccelerando, Measure: 1, OnsetDiv: 0}}}, 4)
	if b, ok := agogicBeats(acc.sc.Parts[0], musicxml.Note{OnsetDiv: 12, Measure: 1}, acc); !ok || b >= 0 {
		t.Errorf("accel. must rush (negative), got %v", b)
	}
	// Unclosed: expires after agogicSpanMeasures bars of the meter in force.
	if _, ok := agogicBeats(acc.sc.Parts[0], musicxml.Note{OnsetDiv: agogicSpanMeasures*48 + 1, Measure: 3}, acc); ok {
		t.Error("an unclosed accel. outlived its span")
	}
}

func TestFermataBeatsSpanBeatsTenuto(t *testing.T) {
	l := testLayer(musicxml.Part{Directions: []musicxml.Direction{{Kind: musicxml.DirRitardando, Measure: 1, OnsetDiv: 0}}}, 2)
	p := l.sc.Parts[0]
	ferm := musicxml.Note{OnsetDiv: 12, Measure: 1, Articulations: musicxml.Articulations{Fermata: true, Tenuto: true}}
	if got := articulationBeats(p, ferm, l); got != fermataDragBeats {
		t.Errorf("fermata under a rit.: %v, want the fermata's %v", got, fermataDragBeats)
	}
	ten := musicxml.Note{OnsetDiv: 12, Measure: 1, Articulations: musicxml.Articulations{Tenuto: true}}
	if got := articulationBeats(p, ten, l); got == tenutoDragBeats || got <= 0 {
		t.Errorf("tenuto under a rit.: %v, want the span's drift to win", got)
	}
	plain := testLayer(musicxml.Part{}, 1)
	if got := articulationBeats(plain.sc.Parts[0], ten, plain); got != tenutoDragBeats {
		t.Errorf("bare tenuto: %v, want %v", got, tenutoDragBeats)
	}
}

// measureLenAt reads the meter IN FORCE, so a span after a meter change is
// sized by the new bar, not by the score's opening time signature.
func TestMeasureLenFollowsTheMeter(t *testing.T) {
	p := musicxml.Part{Measures: []musicxml.Measure{
		{Number: 1, OnsetDiv: 0, LengthDiv: 48, Beats: 4, BeatType: 4},
		{Number: 2, OnsetDiv: 48, LengthDiv: 42, Beats: 7, BeatType: 8},
	}}
	if got := measureLenAt(p, 10, testDiv); got != 48 {
		t.Errorf("4/4 bar: %d", got)
	}
	if got := measureLenAt(p, 50, testDiv); got != 42 {
		t.Errorf("7/8 bar: %d, want 42", got)
	}
	if got := measureLenAt(musicxml.Part{}, 0, testDiv); got != 48 {
		t.Errorf("no timeline: %d, want a 4/4 bar", got)
	}
}

// A run of grace notes takes one step each before its principal, every one
// at the same point of its step — never more than the one-step window late.
func TestGraceRunStaysInsideTheWindow(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 24, Semitone: 1, Voice: 1, Staff: 1, Grace: true},
		{OnsetDiv: 24, Semitone: 2, Voice: 1, Staff: 1, Grace: true},
		{OnsetDiv: 24, Semitone: 3, Voice: 1, Staff: 1, Grace: true},
		{OnsetDiv: 24, Semitone: 4, Voice: 1, Staff: 1},
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0].MaxVoices = 1
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	h := rows[0].hits
	for step, pitch := range map[int]float64{5: 1, 6: 2, 7: 3, 8: 4} {
		if h[step].Pitch != pitch {
			t.Errorf("step %d plays %v, want %v (graces in written order before the principal)", step, h[step].Pitch, pitch)
		}
	}
	if h[5].GroovePct != h[7].GroovePct || h[5].Groove != grooveDelay {
		t.Errorf("grace offsets %+v %+v: every grace sits at the same point of its step", h[5], h[7])
	}
	if st.GrooveClamped != 0 {
		t.Errorf("a grace run was clamped %d times", st.GrooveClamped)
	}
}

// A roll across a big chord is compressed to fit the one-step window rather
// than clamping its top notes.
func TestBigChordRollFitsTheWindow(t *testing.T) {
	roll := musicxml.Articulations{Arpeggiate: 1}
	var notes []musicxml.Note
	for i, semi := range []int{0, 4, 7, 12, 16, 19} {
		notes = append(notes, musicxml.Note{OnsetDiv: 0, Semitone: semi, Voice: 1, Staff: 1, Chord: i > 0, Articulations: roll})
	}
	l := testLayer(musicxml.Part{Name: "x", Notes: notes}, 1)
	l.spec.Parts[0].MaxVoices = 6
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	if st.GrooveClamped != 0 {
		t.Errorf("a six-note roll clamped %d notes", st.GrooveClamped)
	}
	if top := rows[0].hits[0]; top.Groove != grooveDelay || top.GroovePct != 1 {
		t.Errorf("top of the roll: %+v, want the full one-step delay", top)
	}
}

// A grace before an off-grid principal sounds no later than the principal's
// grid step — still ahead of the principal, never past the window.
func TestGraceBeforeOffGridPrincipalStaysInWindow(t *testing.T) {
	p := musicxml.Part{Name: "x", Notes: []musicxml.Note{
		{OnsetDiv: 25, Semitone: 5, Voice: 1, Staff: 1, Grace: true},
		{OnsetDiv: 25, Semitone: 3, Voice: 1, Staff: 1}, // step 8.33: rounds DOWN onto step 8
	}}
	l := testLayer(p, 1)
	l.spec.Parts[0].MaxVoices = 1
	rows, st := runLayer(t, l, timing{stepsPerQuarter: 4})
	if st.GrooveClamped != 0 {
		t.Fatalf("clamped %d", st.GrooveClamped)
	}
	// Ideal grace time 8.33-0.14 = 8.2 is inside the principal's own step;
	// from step 7 it is capped at a full step late — sounding at step 8,
	// still ahead of the principal at 8.33.
	g, ok := rows[0].hits[7]
	if !ok || g.Pitch != 5 || g.Groove != grooveDelay || g.GroovePct != 1 {
		t.Errorf("grace %+v: want step 7 delayed exactly one step", rows[0].hits)
	}
	if pr := rows[0].hits[8]; pr.Pitch != 3 || pr.Groove != grooveDelay || pr.GroovePct != 0.333 {
		t.Errorf("principal %+v: want step 8 delayed 0.333", pr)
	}
}
