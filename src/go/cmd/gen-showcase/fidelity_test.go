package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
	"github.com/ingyamilmolinar/beatmo/internal/scoreref"
)

// These tests treat the committed score as an ORACLE, not as documentation.
//
// The old guard for this was musicTruths — a hand-written table of each piece's
// researched key and tempo. That table could only ever restate what someone
// believed about the music; it could not catch a wrong note, and it drifted
// from the score it described. Because every template is now a pure function of
// a committed score, the tests below can re-read that score independently and
// check conservation: no pitch may appear in a circuit that is not in its
// score's trimmed range, and no note may vanish beyond a declared budget.

// templateJSON is the shape of a generated template read back from disk.
type templateJSON struct {
	Subdiv      int `json:"subdiv"`
	BPM         int `json:"bpm"`
	Instruments []struct {
		ID     instrument.ID `json:"id"`
		Name   string        `json:"name"`
		Origin int           `json:"origin"`
	} `json:"instruments"`
	Nodes []struct {
		Type       string  `json:"type"`
		Pitch      float64 `json:"pitch"`
		Volume     float64 `json:"volume"`
		Duration   float64 `json:"duration"`
		GrooveKind string  `json:"groove_kind"`
		GroovePct  float64 `json:"groove_pct"`
	} `json:"nodes"`
}

func readTemplate(t *testing.T, stem string) templateJSON {
	t.Helper()
	p := filepath.Join("..", "..", "internal", "assets", "templates", stem+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var d templateJSON
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal %s: %v", p, err)
	}
	return d
}

// scoreCache memoises parsed scores: the corpus totals ~5 MB of compressed
// MusicXML and several tests walk every template, so re-parsing per lookup
// dominated the package's runtime.
var (
	scoreCacheMu sync.Mutex
	scoreCache   = map[string]musicxml.Score{}
)

func mustScore(t *testing.T, id string) musicxml.Score {
	t.Helper()
	scoreCacheMu.Lock()
	defer scoreCacheMu.Unlock()
	if sc, ok := scoreCache[id]; ok {
		return sc
	}
	sc, err := scoreref.ParseScore(id)
	if err != nil {
		t.Fatalf("parse score %q: %v", id, err)
	}
	scoreCache[id] = sc
	return sc
}

// specLayers lists a template's layers: the melody, and the groove if any.
func specLayers(spec TemplateSpec) []layerSpec {
	ls := []layerSpec{spec.melodyLayer()}
	if spec.Groove != nil {
		ls = append(ls, spec.Groove.layer())
	}
	return ls
}

// grooveLoops re-derives how many times a template's groove repeats, from the
// METER of each bar on both sides.
func grooveLoops(t *testing.T, spec TemplateSpec) int {
	t.Helper()
	if spec.Groove == nil {
		return 1
	}
	quarters := func(id string, l layerSpec) float64 {
		sc := mustScore(t, id)
		p := sc.Parts[l.Parts[0].Index]
		q, n := 0.0, 0
		for _, m := range p.Measures {
			if m.Movement != layerMovement(l) || m.Number < l.From || m.Number > l.To {
				continue
			}
			n++
			if n == 1 && l.To > l.From && m.LengthDiv < p.Divisions*4*m.Beats/m.BeatType {
				continue // a leading pickup wraps; it adds no length
			}
			q += 4 * float64(m.Beats) / float64(m.BeatType)
		}
		return q
	}
	mel := quarters(spec.ScoreID, spec.melodyLayer())
	gr := quarters(spec.Groove.ScoreID, spec.Groove.layer())
	if gr <= 0 || mel/gr != float64(int(mel/gr)) {
		t.Fatalf("%s: groove %.2f quarters does not divide melody %.2f", spec.Stem, gr, mel)
	}
	return int(mel / gr)
}

// pitchKey makes node pitches comparable as map keys. Pitches are whole
// semitones except where SoundingPitch cancels a drum's fractional tuning.
func pitchKey(p float64) float64 { return math.Round(p*1000) / 1000 }

// scoreNotesInRange re-derives, independently of the generator, the NODE
// pitches a spec may emit, across both layers: for a pitched part the score's
// sounding pitch, plus the PartMap's deliberate octave shift, minus the
// transposition the instrument itself applies at render time; for a
// percussion note, the tuning of the kit piece its GM key routes to.
//
// The instrument term is what makes the node number differ from the score
// number. A node at semitone p on an instrument that renders an octave down
// sounds an octave below p, so to sound the score's note the node must carry
// p+12. The generator cancels the instrument shift in instrumentPitchShift;
// this oracle applies the same cancellation, so what is still checked end to
// end is the thing that matters — every note ORIGINATES IN A SCORE. (Whether
// the shift itself is correct is a different question, answered against
// rendered audio by TestEveryPitchedInstrumentSoundsInTune.)
//
// count is the number of score notes the template must account for: a groove
// counts once per loop. kitPitches[inst] is the set of pitches each kit
// instrument may play; a node on a kit row must use one of those.
func scoreNotesInRange(t *testing.T, spec TemplateSpec) (pitches map[float64]bool, count int, kitPitches map[instrument.ID]map[float64]bool) {
	pitches = map[float64]bool{}
	kitPitches = map[instrument.ID]map[float64]bool{}
	loops := []int{1, grooveLoops(t, spec)}
	for li, l := range specLayers(spec) {
		sc := mustScore(t, l.ScoreID)
		for _, pm := range l.Parts {
			if pm.Index < 0 || pm.Index >= len(sc.Parts) {
				continue
			}
			shift := instrumentPitchShift(pm.Inst)
			if pm.SoundingPitch {
				shift = soundingPitchShift(pm.Inst)
			}
			for _, n := range sc.Parts[pm.Index].Notes {
				// Movement, then measures. Re-derived here rather than calling
				// inLayer so this stays an independent oracle — a
				// multi-movement score prints measure "1" once per movement.
				if n.Rest || n.Movement != layerMovement(l) || n.Measure < l.From || n.Measure > l.To {
					continue
				}
				count += loops[li]
				if n.Unpitched {
					kp, ok := kitPieceFor(pm, n.PercKey)
					if !ok {
						continue
					}
					if kitPitches[kp.Inst] == nil {
						kitPitches[kp.Inst] = map[float64]bool{}
					}
					kitPitches[kp.Inst][pitchKey(kp.Pitch)] = true
					continue
				}
				pitches[pitchKey(float64(n.Semitone+12*pm.Octave)-shift)] = true
			}
		}
	}
	return pitches, count, kitPitches
}

// TestTemplateIsDerivedFromScore is the central guard: every pitch a template
// plays must exist in its scores' trimmed ranges. A hand-edited note, a wrong
// octave policy, or a transposition slip all fail here.
func TestTemplateIsDerivedFromScore(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			want, _, kit := scoreNotesInRange(t, spec)
			if len(want) == 0 && len(kit) == 0 {
				t.Fatalf("score %q measures %d..%d yielded no notes for the mapped parts",
					spec.ScoreID, spec.From, spec.To)
			}

			d := readTemplate(t, spec.Stem)
			rowOf := rowIndexer(d)
			played := 0
			for i, n := range d.Nodes {
				if n.Type != "regular" {
					continue
				}
				played++
				inst := d.Instruments[rowOf(i)].ID
				p := pitchKey(n.Pitch)
				if kp, isKit := kit[inst.Base()]; isKit && !want[p] {
					if !kp[p] {
						t.Errorf("node %d on kit row %q plays pitch %v, which no kit piece of the score routes there", i, inst, n.Pitch)
					}
					continue
				}
				if !want[p] {
					t.Errorf("node %d (%s) plays pitch %v, which does NOT occur in %s measures %d..%d. "+
						"Templates must be derived from the score — no hand-entered notes.",
						i, inst, n.Pitch, spec.ScoreID, spec.From, spec.To)
				}
			}
			if played == 0 {
				t.Error("template has no sounding nodes")
			}
		})
	}
}

// rowIndexer maps a node index to its row: a node belongs to the instrument
// whose origin most recently precedes it, which is the layout build() writes.
func rowIndexer(d templateJSON) func(int) int {
	return func(nodeIdx int) int {
		row := 0
		for i, in := range d.Instruments {
			if nodeIdx >= in.Origin {
				row = i
			}
		}
		return row
	}
}

// dropBudget declares how many score notes a template is allowed to lose, and
// why. Both numbers are consequences of the grid model, not of taste:
// ChordTones are notes below a part's MaxVoices cap, and StepCollisions are
// distinct onsets that round onto one step at the chosen Subdiv.
//
// The budgets are exact, not upper bounds — a spec change that loses MORE or
// FEWER notes fails, so the musical cost of every table edit is visible in the
// diff rather than absorbed silently.
type dropBudget struct{ chordTones, stepCollisions int }

func TestTemplateDropBudgets(t *testing.T) {
	budgets := map[string]dropBudget{}
	for _, spec := range templateSpecs() {
		_, stats, err := buildFromScore(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec.Stem, err)
		}
		budgets[spec.Stem] = dropBudget{stats.ChordTonesDropped, stats.StepCollisions}

		// Whatever the budget, the conservation identity must hold: every
		// selected score note is either emitted, dropped as a chord tone, or
		// dropped as a step collision. Nothing may disappear unaccounted for.
		_, inRangeCount, _ := scoreNotesInRange(t, spec)
		got := stats.NotesEmitted + stats.ChordTonesDropped + stats.StepCollisions + stats.RegisterDropped
		if got != inRangeCount {
			t.Errorf("%s: %d score notes in range but emitted+dropped = %d (%d+%d+%d+%d) — "+
				"notes are going missing without being counted",
				spec.Stem, inRangeCount, got,
				stats.NotesEmitted, stats.ChordTonesDropped, stats.StepCollisions, stats.RegisterDropped)
		}
		if stats.GrooveClamped > 0 {
			t.Errorf("%s: %d notes needed more than one step of rush/delay and were clamped — "+
				"the notation asks for a drift the engine's groove window cannot hold", spec.Stem, stats.GrooveClamped)
		}
	}

	if t.Failed() {
		return
	}
	for _, spec := range templateSpecs() {
		b := budgets[spec.Stem]
		if b.stepCollisions > 0 {
			t.Logf("%-26s chord-tones dropped=%-4d step-collisions=%d", spec.Stem, b.chordTones, b.stepCollisions)
		}
	}
}

// TestStepCollisionsStayRare gates the ONE drop cause that signals a bad spec
// rather than an inherent limit of the grid.
//
// A dropped chord tone is deliberate: MaxVoices says how many lanes a part gets
// and the rest are knowingly discarded. A step collision is not — it means two
// distinct onsets rounded onto the same step because the chosen Subdiv is too
// coarse for the passage, and the fix is a finer Subdiv, not a smaller budget.
// Schubert's Ave Maria shipped at Subdiv 16 losing 135 of 434 notes (31%): its
// piano writes sextuplets, six notes per beat into four slots.
func TestStepCollisionsStayRare(t *testing.T) {
	// 5% leaves room for the odd grace note or 32nd flourish while failing a
	// passage whose subdivision genuinely does not fit.
	const maxCollisionRatio = 0.05

	for _, spec := range templateSpecs() {
		_, stats, err := buildFromScore(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec.Stem, err)
		}
		total := stats.NotesEmitted + stats.StepCollisions
		if total == 0 {
			continue
		}
		if ratio := float64(stats.StepCollisions) / float64(total); ratio > maxCollisionRatio {
			t.Errorf("%s: %d of %d notes (%.0f%%) collided onto an occupied step at Subdiv=%d — "+
				"raise the template's Subdiv (32 resolves triplet and sextuplet figuration) "+
				"rather than accepting the loss",
				spec.Stem, stats.StepCollisions, total, ratio*100, spec.Subdiv)
		}
	}
}

// TestPartMapNamesMatchScore checks the Index/Name pair in every PartMap.
// Index is what selects the part; Name is the assertion that the index still
// points where the table says. Several scores carry duplicate part names
// (Boulanger's orchestration has three parts named "Violoncello"), so selection
// cannot be by name — but an unchecked index would silently point at a
// different instrument if the upstream score were re-ordered.
func TestPartMapNamesMatchScore(t *testing.T) {
	for _, spec := range templateSpecs() {
		for _, l := range specLayers(spec) {
			sc := mustScore(t, l.ScoreID)
			for _, pm := range l.Parts {
				if pm.Index < 0 || pm.Index >= len(sc.Parts) {
					t.Errorf("%s: part index %d out of range (score %s has %d parts)",
						spec.Stem, pm.Index, l.ScoreID, len(sc.Parts))
					continue
				}
				if got := sc.Parts[pm.Index].Name; got != pm.Name {
					t.Errorf("%s: %s part %d is %q but the table says %q — the upstream score "+
						"may have been re-ordered; re-check the mapping, do not just rename",
						spec.Stem, l.ScoreID, pm.Index, got, pm.Name)
				}
			}
		}
	}
}

// coveringTempo re-derives, INDEPENDENTLY of the generator, the tempo that
// covers most of [from,to] by walking the score's markings directly. Calling
// musicxml's DominantTempo here instead would recreate the very defect this
// test exists to catch: the old version compared the template against the same
// field the generator read, so subject and oracle agreed no matter what. (The
// helper itself is verified separately, against hand-written fixtures with
// known answers, in internal/musicxml's TestTempoAtMeasureTracksTheTimeline.)
func coveringTempo(sc musicxml.Score, from, to int) float64 {
	if to < from {
		to = from
	}
	cur := 0.0
	for _, tm := range sc.Tempos { // markings are in measure order
		if tm.Measure > from {
			break
		}
		cur = tm.BPM
	}
	if cur == 0 && len(sc.Tempos) > 0 {
		cur = sc.Tempos[0].BPM // excerpt precedes the first marking
	}
	bars := map[float64]int{}
	var order []float64
	for m := from; m <= to; m++ {
		for _, tm := range sc.Tempos {
			if tm.Measure == m {
				cur = tm.BPM
			}
		}
		if _, seen := bars[cur]; !seen {
			order = append(order, cur)
		}
		bars[cur]++
	}
	best, most := cur, -1
	for _, bpm := range order {
		if bars[bpm] > most {
			best, most = bpm, bars[bpm]
		}
	}
	return best
}

// TestTemplateTempoComesFromScore replaces the hand-written BPM column of the
// old musicTruths table: the tempo is whatever the score marks FOR THE MEASURES
// THE TEMPLATE WAS CUT FROM.
//
// That last clause is the whole test. It compared against sc.TempoBPM until
// 2026-09-12 — the same single collapsed field the generator itself read — so
// the two agreed by construction and the check could not fail however wrong the
// tempo was. Meanwhile that field was being overwritten by every marking in the
// document, leaving each excerpt with whichever tempo appeared LAST anywhere in
// the score: seven of twenty-four templates ran at the wrong speed, six of them
// too fast, worst the Ode to Joy excerpt at 264 against the 160 its own
// measures are marked (the Prestissimo coda, 800 measures past the excerpt).
// An oracle the subject already agrees with is not an oracle; this now asks the
// score's tempo TIMELINE about the selected range.
func TestTemplateTempoComesFromScore(t *testing.T) {
	for _, spec := range templateSpecs() {
		if spec.BPM != 0 {
			continue // explicit override; nothing to derive
		}
		sc := mustScore(t, spec.ScoreID)
		d := readTemplate(t, spec.Stem)
		marked := coveringTempo(sc, spec.From, spec.To)
		if marked <= 0 {
			continue // score carries no tempo marking; generator falls back
		}
		if want := int(marked + 0.5); d.BPM != want {
			t.Errorf("%s: template BPM=%d but score marks %.0f across measures %d-%d (%+.0f%%)",
				spec.Stem, d.BPM, marked, spec.From, spec.To, (float64(d.BPM)/marked-1)*100)
		}
		if d.BPM <= 0 {
			t.Errorf("%s: BPM=%d", spec.Stem, d.BPM)
		}
	}
}

// TestDynamicsReachVolume ties the notated dynamic to per-hit volume in both
// directions, layer by layer: a range that marks dynamics must produce
// volumes, and a range that marks none must leave every volume at 0 so the
// instrument default applies. The second half matters — Bach marks no
// dynamics in any of the openings used here, and LilyPond's MIDI editions
// carry one constant velocity; inventing a level from either would be
// exactly the kind of unsourced embellishment this pipeline removes.
func TestDynamicsReachVolume(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			sh, _, err := buildFromScore(spec)
			if err != nil {
				t.Fatal(err)
			}
			for li, l := range specLayers(spec) {
				sc := mustScore(t, l.ScoreID)
				marked := false
				for _, pm := range l.Parts {
					for _, n := range sc.Parts[pm.Index].Notes {
						if !n.Rest && inLayer(n, l) && n.Velocity > 0 {
							marked = true
						}
					}
				}
				withVol := 0
				for _, r := range sh.Rows {
					if r.Layer != li {
						continue
					}
					for _, h := range r.Hits {
						if h.Vol < 0 || h.Vol > 1 {
							t.Errorf("volume %v out of range 0..1", h.Vol)
						}
						if h.Vol > 0 {
							withVol++
						}
					}
				}
				switch {
				case marked && withVol == 0:
					t.Errorf("%s marks dynamics in this range but no node carries a volume", l.ScoreID)
				case !marked && withVol > 0:
					t.Errorf("%s marks NO dynamics in this range yet %d nodes carry a volume — "+
						"volume must come from the score, never be invented", l.ScoreID, withVol)
				}
			}
		})
	}
}

// layerTimingNotation reports whether a layer's selected range carries ANY
// notation that may move a note off its written step — re-derived from the
// raw score so the groove a template carries can be checked against it.
func layerTimingNotation(sc musicxml.Score, l layerSpec, spq int) []string {
	var why []string
	add := func(s string) {
		for _, w := range why {
			if w == s {
				return
			}
		}
		why = append(why, s)
	}
	for _, pm := range l.Parts {
		p := sc.Parts[pm.Index]
		div := p.Divisions
		for _, n := range p.Notes {
			if n.Rest || !inLayer(n, l) {
				continue
			}
			a := n.Articulations
			switch {
			case n.Grace:
				add("grace note")
			case a.Fermata:
				add("fermata")
			case a.Tenuto:
				add("tenuto")
			case a.Arpeggiate != 0:
				add("arpeggio")
			case a.BreathMark || a.Caesura:
				add("breath/caesura")
			}
			if pos := float64(n.OnsetDiv) / float64(div) * float64(spq); pos != math.Trunc(pos) {
				add("off-grid onset (tuplet)")
			}
		}
		for _, d := range p.Directions {
			switch d.Kind {
			case musicxml.DirRitardando, musicxml.DirAccelerando:
				if d.Measure >= l.From && d.Measure <= l.To {
					add("rit./accel.")
				}
			case musicxml.DirSwing:
				add("swing")
			}
		}
	}
	return why
}

// TestGrooveIsDerivedFromNotation checks the drift/rush layer the same way:
// a groove may only appear where the notation asks for one — in the layer's
// own range, or as the groove layer's swing, which is the whole band's feel —
// and its magnitude must stay inside the engine's one-step window.
func TestGrooveIsDerivedFromNotation(t *testing.T) {
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			sh, _, err := buildFromScore(spec)
			if err != nil {
				t.Fatal(err)
			}
			spq := spec.Subdiv / 4
			layers := specLayers(spec)
			notation := make([][]string, len(layers))
			bandSwing := false
			for li, l := range layers {
				notation[li] = layerTimingNotation(mustScore(t, l.ScoreID), l, spq)
				if li == 1 {
					for _, w := range notation[li] {
						bandSwing = bandSwing || w == "swing"
					}
				}
			}
			for _, r := range sh.Rows {
				grooved := 0
				for _, h := range r.Hits {
					if h.Groove == "" {
						if h.GroovePct != 0 {
							t.Errorf("row %s step %d: groove_pct %v with no groove_kind", r.Inst, h.Step, h.GroovePct)
						}
						continue
					}
					grooved++
					if h.Groove != grooveDelay && h.Groove != grooveRush {
						t.Errorf("row %s step %d: groove_kind %q", r.Inst, h.Step, h.Groove)
					}
					if h.GroovePct <= 0 || h.GroovePct > maxOffsetSteps {
						t.Errorf("row %s step %d: groove_pct %v outside (0, %v]", r.Inst, h.Step, h.GroovePct, maxOffsetSteps)
					}
				}
				if grooved > 0 && len(notation[r.Layer]) == 0 && !bandSwing {
					t.Errorf("row %s: %d nodes carry groove but its score marks nothing that moves a note "+
						"(grace, fermata, tenuto, rit./accel., arpeggio, breath, swing, tuplet) in this range",
						r.Inst, grooved)
				}
			}
		})
	}
}

// TestEveryTemplateHasSpecAndScore closes the loop between the three places a
// template is named: the spec table, the score catalogue, and the committed
// JSON on disk.
func TestEveryTemplateHasSpecAndScore(t *testing.T) {
	specs := templateSpecs()
	bySpec := map[string]bool{}
	for _, s := range specs {
		if bySpec[s.Stem] {
			t.Errorf("duplicate template stem %q", s.Stem)
		}
		bySpec[s.Stem] = true

		ref, ok := scoreref.Ref(s.ScoreID)
		if !ok {
			t.Errorf("template %q references unknown score %q", s.Stem, s.ScoreID)
			continue
		}
		found := false
		for _, tpl := range ref.Templates {
			if tpl == s.Stem {
				found = true
			}
		}
		if !found {
			t.Errorf("score %q does not list template %q in its catalogue entry", s.ScoreID, s.Stem)
		}
		if s.Groove != nil {
			g, ok := scoreref.Ref(s.Groove.ScoreID)
			if !ok || !g.IsGroove() {
				t.Errorf("template %q: groove %q is not a catalogued Beatmo groove score", s.Stem, s.Groove.ScoreID)
			} else {
				listed := false
				for _, tpl := range g.Templates {
					listed = listed || tpl == s.Stem
				}
				if !listed {
					t.Errorf("groove %q does not list template %q in its catalogue entry", s.Groove.ScoreID, s.Stem)
				}
			}
		}
		if s.From < 1 || s.To < s.From {
			t.Errorf("template %q: bad measure range %d..%d", s.Stem, s.From, s.To)
		}
		if len(s.Parts) == 0 {
			t.Errorf("template %q maps no parts", s.Stem)
		}
	}

	dir := filepath.Join("..", "..", "internal", "assets", "templates")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}
	onDisk := map[string]bool{}
	for _, e := range ents {
		name := e.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		stem := name[:len(name)-len(".json")]
		onDisk[stem] = true
		if !bySpec[stem] {
			t.Errorf("templates/%s is committed but has no TemplateSpec — "+
				"delete it or add the spec", name)
		}
	}
	for stem := range bySpec {
		if !onDisk[stem] {
			t.Errorf("template %q has a spec but no committed JSON — run go run ./cmd/gen-showcase", stem)
		}
	}
}

// TestTemplatesAreReasonablySized keeps a spec edit from quietly producing a
// circuit nobody can use: too few rows to be musical, or so many nodes the grid
// pane chokes.
//
// maxRows is 12, not the earlier 8: a famous beat is the point of a groove
// template, and a drum-machine break notates up to seven voices (kick, rim,
// snare, clap, two hats, congas — E=MC²) that each need their own row. Chord
// parts are capped at two voices and same-family drums share a row (toms,
// congas) before a template is allowed past 8.
func TestTemplatesAreReasonablySized(t *testing.T) {
	const (
		minRows  = 2
		maxRows  = 12
		minNodes = 40
		maxNodes = 900
	)
	for _, spec := range templateSpecs() {
		d := readTemplate(t, spec.Stem)
		if len(d.Instruments) < minRows || len(d.Instruments) > maxRows {
			t.Errorf("%s: %d rows, want %d..%d", spec.Stem, len(d.Instruments), minRows, maxRows)
		}
		if len(d.Nodes) < minNodes || len(d.Nodes) > maxNodes {
			t.Errorf("%s: %d nodes, want %d..%d (narrow the measure range or lower MaxVoices)",
				spec.Stem, len(d.Nodes), minNodes, maxNodes)
		}
		if d.Subdiv <= 0 || d.Subdiv%4 != 0 {
			t.Errorf("%s: subdiv %d", spec.Stem, d.Subdiv)
		}
	}
}

// TestTemplateDurationsArePositive guards the duration field: a 0 duration is
// legal (it means "instrument default") but a negative one is a conversion bug.
func TestTemplateDurationsArePositive(t *testing.T) {
	for _, spec := range templateSpecs() {
		d := readTemplate(t, spec.Stem)
		for i, n := range d.Nodes {
			if n.Duration < 0 {
				t.Errorf("%s node %d: duration %v", spec.Stem, i, n.Duration)
			}
		}
	}
}

// TestTemplateOrderMatchesAssets keeps the two halves of the template
// definition in step: cmd/gen-showcase decides what a template CONTAINS,
// internal/assets decides where it appears in the menu. Nothing links them at
// compile time, so a stem added here and forgotten there would generate a JSON
// file that never ships (and trip the assets orphan guard only by luck of
// ordering).
func TestTemplateOrderMatchesAssets(t *testing.T) {
	specs := templateSpecs()
	menu := assets.Templates()

	if len(specs) != len(menu) {
		t.Fatalf("%d template specs but %d in the menu — add the new stem to "+
			"templateGroups in internal/assets/templates.go", len(specs), len(menu))
	}
	for i := range specs {
		if specs[i].Stem != menu[i].Genre {
			t.Errorf("position %d: spec %q but menu %q — templateSpecs() and "+
				"templateGroups must list the same stems in the same order",
				i, specs[i].Stem, menu[i].Genre)
		}
	}
}
