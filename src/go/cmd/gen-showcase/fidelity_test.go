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

// The committed score is the ORACLE: re-read independently, no pitch may appear in a circuit that is not in its trimmed
// range and no note may vanish beyond a declared budget.

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

// Several tests walk every template over ~5 MB of compressed MusicXML.
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

// scoreNotesInRange re-derives the node pitches a spec may emit: the score's sounding pitch + the PartMap's octave
// shift − the instrument's own render transposition (as instrumentPitchShift cancels it), or a kit piece's tuning.
// count is the score notes to account for (a groove once per loop); kitPitches[inst] are a kit row's allowed pitches.
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
				// Re-derived rather than via inLayer, to stay an independent oracle.
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

// dropBudget: notes a template may lose, both from the grid model (ChordTones past MaxVoices, StepCollisions onto one
// step). Exact, not upper bounds, so every spec edit's musical cost shows in the diff.
type dropBudget struct{ chordTones, stepCollisions int }

func TestTemplateDropBudgets(t *testing.T) {
	budgets := map[string]dropBudget{}
	for _, spec := range templateSpecs() {
		_, stats, err := buildFromScore(spec)
		if err != nil {
			t.Fatalf("%s: %v", spec.Stem, err)
		}
		budgets[spec.Stem] = dropBudget{stats.ChordTonesDropped, stats.StepCollisions}

		// Every selected score note is emitted, dropped as a chord tone, or dropped as a step collision.
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

// A step collision, unlike a chord tone, signals a bad spec: the Subdiv is too coarse for the passage (sextuplets into
// four slots), and the fix is a finer Subdiv, not a bigger budget.
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

// Index selects the part; Name asserts it still points there. Scores repeat part names, so selection cannot be by name,
// and an unchecked index would follow a re-ordered upstream score to a different instrument.
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

// coveringTempo walks the score's markings itself: calling musicxml's DominantTempo would compare the template against
// the field the generator read, an oracle that agrees by construction.
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

// The tempo marked FOR THE MEASURES THE TEMPLATE WAS CUT FROM, asked of the score's tempo timeline: a single
// collapsed tempo field took whichever marking appeared last in the document.
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

// Both directions: marked dynamics must produce volumes, and none must leave every volume at 0 (the instrument
// default). Inventing a level where Bach marks none, or from MIDI's constant velocity, would be unsourced.
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

// layerTimingNotation: the notation in a layer's range that may move a note off its step, from the raw score.
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

// A groove may appear only where the notation asks for one (in the layer's range, or the groove layer's swing for
// the whole band), and inside the engine's one-step window.
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

// 12 rows: a famous break notates up to seven voices that each need a row, after chord parts are capped at two voices
// and same-family drums share one.
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

// gen-showcase decides what a template contains, internal/assets where it appears in the menu; nothing links them at
// compile time, so a forgotten stem would generate a JSON file that never ships.
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
