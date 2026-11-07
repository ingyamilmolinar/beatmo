package main

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/musicxml"
	"github.com/ingyamilmolinar/beatmo/internal/scoreref"
)

// The ONLY path by which a template acquires notes, which is what lets the fidelity tests treat the scores as oracles.
// Each layer (the melody; an optional groove looped to its length): parse → trim → lanes / kit rows → every note on its
// WRITTEN step, its performed time from the notation, the difference as delay/rush. Ties arrive merged.

// Scores carry no mixing levels.
const defaultScoreInstVolume = 0.8

// In BEATS, converted to steps per template, so their audible size does not depend on the grid.
const (
	// graceLeadBeats is how far ahead of its principal a grace note sounds.
	graceLeadBeats = 0.55 / 16
	// fermataDragBeats holds a fermata behind the beat.
	fermataDragBeats = 0.60 / 16
	// The ramp across a rit./accel. span: small at the marking, full at its end, as the instruction is played.
	agogicMinBeats = 0.15 / 16
	agogicMaxBeats = 0.50 / 16
	// Bounds a rit./accel. never closed by "a tempo": a phrase-ending gesture, not a standing instruction.
	agogicSpanMeasures = 2
	// tenutoDragBeats is the slight lean a tenuto puts on a note.
	tenutoDragBeats = 0.12 / 16
	// arpRollBeats separates successive notes of a rolled chord.
	arpRollBeats = 1.0 / 16
	// Before the NEXT onset; sized to stay inside the one-step window even at 32 steps per bar with a rit. or a roll
	// stacked on the same note.
	breathBeats  = 1.0 / 16
	caesuraBeats = 1.0 / 8
)

// The engine's groove window: one grid subdivision = one step. A larger offset is clamped and counted.
const maxOffsetSteps = 1.0

// Constants so a typo is a compile error, not a generation-time panic.
const (
	grooveDelay = "delay"
	grooveRush  = "rush"
)

// Every count is asserted against a declared budget, so a spec change that starts losing notes fails.
type scoreStats struct {
	// ChordTonesDropped counts notes below a part's MaxVoices cap (and grace
	// notes whose principal was such a note).
	ChordTonesDropped int
	// StepCollisions counts notes whose written step was already taken in the
	// same row — the grid is one node per step per row.
	StepCollisions int
	// RegisterDropped counts notes outside a part's keyboard split (BelowMIDI).
	RegisterDropped int
	// NotesEmitted counts notes that became hits.
	NotesEmitted int
	// GrooveClamped counts hits whose performed offset exceeded one step and
	// was clamped to it.
	GrooveClamped int
}

// layerSpec is one score's contribution to a template.
type layerSpec struct {
	ScoreID  string
	From, To int
	Movement int
	Parts    []PartMap
}

func (s TemplateSpec) melodyLayer() layerSpec {
	return layerSpec{ScoreID: s.ScoreID, From: s.From, To: s.To, Movement: s.Movement, Parts: s.Parts}
}

func (g GrooveLayer) layer() layerSpec {
	return layerSpec{ScoreID: g.ScoreID, From: g.From, To: g.To, Parts: g.Parts}
}

// layerMovement is the movement a layer selects; 0 (unset) means the first.
func layerMovement(l layerSpec) int {
	if l.Movement > 0 {
		return l.Movement
	}
	return 1
}

// specMovement is layerMovement for a template's melody layer.
func specMovement(spec TemplateSpec) int { return layerMovement(spec.melodyLayer()) }

// The Movement test is not optional: a multi-movement file RESTARTS its measure numbering (K387 prints measure "1"
// four times), so the printed number alone selects bars from every movement.
func inRange(n musicxml.Note, spec TemplateSpec) bool { return inLayer(n, spec.melodyLayer()) }

func inLayer(n musicxml.Note, l layerSpec) bool {
	return n.Movement == layerMovement(l) && n.Measure >= l.From && n.Measure <= l.To
}

// loadedLayer is a parsed layer with its excerpt located in score time.
type loadedLayer struct {
	spec      layerSpec
	sc        musicxml.Score
	div       int // divisions per quarter, uniform across the mapped parts
	originDiv int // the downbeat of the first selected measure
	lengthDiv int // the notated length of the selected measures
}

func loadLayer(stem string, l layerSpec) (loadedLayer, error) {
	sc, err := scoreref.ParseScore(l.ScoreID)
	if err != nil {
		return loadedLayer{}, fmt.Errorf("template %q: parse score %q: %w", stem, l.ScoreID, err)
	}
	if sc.Divisions <= 0 {
		return loadedLayer{}, fmt.Errorf("template %q: score %q has divisions=%d", stem, l.ScoreID, sc.Divisions)
	}
	div, err := uniformDivisions(sc, stem, l)
	if err != nil {
		return loadedLayer{}, err
	}
	origin, length, err := excerptSpan(sc, stem, l)
	if err != nil {
		return loadedLayer{}, err
	}
	return loadedLayer{spec: l, sc: sc, div: div, originDiv: origin, lengthDiv: length}, nil
}

// uniformDivisions checks that every part a layer selects uses the same
// divisions-per-quarter, which is what makes cross-part onset comparison valid.
func uniformDivisions(sc musicxml.Score, stem string, l layerSpec) (int, error) {
	want, have := 0, false
	for _, pm := range l.Parts {
		if pm.Index < 0 || pm.Index >= len(sc.Parts) {
			return 0, fmt.Errorf("template %q: part index %d out of range (score %q has %d parts)",
				stem, pm.Index, l.ScoreID, len(sc.Parts))
		}
		div := sc.Parts[pm.Index].Divisions
		if div <= 0 {
			div = sc.Divisions
		}
		if !have {
			want, have = div, true
			continue
		}
		if div != want {
			return 0, fmt.Errorf("template %q: part %d (%s) uses %d divisions per quarter but "+
				"an earlier selected part uses %d — onsets are compared across parts, so "+
				"mixed divisions would mis-place every note",
				stem, pm.Index, sc.Parts[pm.Index].Name, div, want)
		}
	}
	if !have {
		return 0, fmt.Errorf("template %q: layer %q maps no parts", stem, l.ScoreID)
	}
	return want, nil
}

// The loop starts on a DOWNBEAT, not the first sounding note, or every bar line slides off the score's and a groove
// cannot land on them. A leading PICKUP bar wraps to the end of the loop, leading back into bar one, as a player repeats it.
func excerptSpan(sc musicxml.Score, stem string, l layerSpec) (origin, length int, err error) {
	part := sc.Parts[l.Parts[0].Index]
	mv := layerMovement(l)
	var bars []musicxml.Measure
	for _, m := range part.Measures {
		if m.Movement == mv && m.Number >= l.From && m.Number <= l.To {
			bars = append(bars, m)
		}
	}
	if len(bars) == 0 {
		return 0, 0, fmt.Errorf("template %q: score %q has no measures %d..%d in movement %d",
			stem, l.ScoreID, l.From, l.To, mv)
	}
	if len(bars) > 1 && isPickup(bars[0], part.Divisions) {
		bars = bars[1:]
	}
	origin = bars[0].OnsetDiv
	for _, m := range bars {
		length += m.LengthDiv
	}
	if length <= 0 {
		return 0, 0, fmt.Errorf("template %q: score %q measures %d..%d have no length", stem, l.ScoreID, l.From, l.To)
	}
	return origin, length, nil
}

// isPickup reports whether a bar is shorter than its meter says.
func isPickup(m musicxml.Measure, div int) bool {
	if m.Beats <= 0 || m.BeatType <= 0 || div <= 0 {
		return false
	}
	return m.LengthDiv < div*4*m.Beats/m.BeatType
}

// rowKey identifies an output row: pitched lanes are keyed by part and lane,
// kit rows by part and instrument (plus a spill index when two hits of one
// instrument land on one step).
type rowKey struct {
	layer, part int
	lane        int
	kitInst     instrument.ID
	spill       int
}

type rowAcc struct {
	key  rowKey
	inst InstSpec
	hits map[int]Hit
}

// assembler accumulates rows across layers in a stable order.
type assembler struct {
	rows  []*rowAcc
	byKey map[rowKey]*rowAcc
}

func (a *assembler) row(k rowKey, mk func() InstSpec) *rowAcc {
	if r, ok := a.byKey[k]; ok {
		return r
	}
	r := &rowAcc{key: k, inst: mk(), hits: map[int]Hit{}}
	a.byKey[k] = r
	a.rows = append(a.rows, r)
	return r
}

// timing is the per-template conversion from score time to steps.
type timing struct {
	stepsPerQuarter int
	// swing applies to every layer that has none of its own: a groove's swing
	// is the band's feel, so the melody is swung with it.
	swing     musicxml.Direction
	haveSwing bool
}

// buildFromScore converts one TemplateSpec into a Showcase.
func buildFromScore(spec TemplateSpec) (Showcase, scoreStats, error) {
	var stats scoreStats
	if spec.Subdiv <= 0 || spec.Subdiv%4 != 0 {
		return Showcase{}, stats, fmt.Errorf("template %q: subdiv %d must be a positive multiple of 4", spec.Stem, spec.Subdiv)
	}
	spq := spec.Subdiv / 4
	if spq != 4 && spq != 8 {
		// The engine accepts 4, 8, 16 or 32 steps per beat; no score here needs finer than 8.
		return Showcase{}, stats, fmt.Errorf("template %q: subdiv %d gives %d steps per beat; want 4 or 8", spec.Stem, spec.Subdiv, spq)
	}

	mel, err := loadLayer(spec.Stem, spec.melodyLayer())
	if err != nil {
		return Showcase{}, stats, err
	}
	totalQuarters := float64(mel.lengthDiv) / float64(mel.div)
	totalSteps := totalQuarters * float64(spq)
	if totalSteps != math.Trunc(totalSteps) || int(totalSteps)%4 != 0 {
		return Showcase{}, stats, fmt.Errorf("template %q: excerpt is %.3f quarters = %.2f steps; the loop must be a whole number of steps divisible by 4 (the circuit is a square)",
			spec.Stem, totalQuarters, totalSteps)
	}

	tm := timing{stepsPerQuarter: spq}
	var groove loadedLayer
	loops := 0
	if spec.Groove != nil {
		groove, err = loadLayer(spec.Stem, spec.Groove.layer())
		if err != nil {
			return Showcase{}, stats, err
		}
		gq := float64(groove.lengthDiv) / float64(groove.div)
		ratio := totalQuarters / gq
		if ratio != math.Trunc(ratio) || ratio < 1 {
			return Showcase{}, stats, fmt.Errorf("template %q: groove %q is %.3f quarters long, which does not divide the %.3f-quarter excerpt",
				spec.Stem, spec.Groove.ScoreID, gq, totalQuarters)
		}
		loops = int(ratio)
		for _, pm := range spec.Groove.Parts {
			if sw, ok := groove.sc.Parts[pm.Index].SwingAt(groove.originDiv); ok {
				tm.swing, tm.haveSwing = sw, true
				break
			}
		}
	}

	asm := &assembler{byKey: map[rowKey]*rowAcc{}}
	if err := emitLayer(spec, 0, mel, 1, tm, asm, &stats); err != nil {
		return Showcase{}, stats, err
	}
	if spec.Groove != nil {
		if err := emitLayer(spec, 1, groove, loops, tm, asm, &stats); err != nil {
			return Showcase{}, stats, err
		}
	}

	var rows []RowSpec
	var insts []InstSpec
	for _, r := range asm.rows {
		if len(r.hits) == 0 {
			continue
		}
		hits := make([]Hit, 0, len(r.hits))
		for _, h := range r.hits {
			hits = append(hits, h)
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].Step < hits[j].Step })
		rows = append(rows, RowSpec{Inst: r.inst.ID, Hits: hits, Layer: r.key.layer, Kit: r.key.kitInst != ""})
		insts = append(insts, r.inst)
	}
	if len(rows) == 0 {
		return Showcase{}, stats, fmt.Errorf("template %q: produced no rows", spec.Stem)
	}

	// The circuit runs at ONE tempo: the one covering most of the selected measures, not the opening or closing mark. A
	// groove template sets BPM: the genre decides the tempo.
	bpm := spec.BPM
	if bpm == 0 {
		bpm = int(math.Round(mel.sc.DominantTempo(spec.From, spec.To)))
	}
	if bpm <= 0 {
		bpm = int(math.Round(mel.sc.TempoBPM))
	}

	return Showcase{
		Stem:       spec.Stem,
		BPM:        bpm,
		Subdiv:     spec.Subdiv,
		TotalSteps: int(totalSteps),
		Insts:      insts,
		Rows:       rows,
	}, stats, nil
}

// emitLayer converts every mapped part of one layer, repeated loops times.
func emitLayer(spec TemplateSpec, li int, l loadedLayer, loops int, tm timing, asm *assembler, stats *scoreStats) error {
	for pi, pm := range l.spec.Parts {
		part := l.sc.Parts[pm.Index]
		var notes []musicxml.Note
		percussion := false
		for _, n := range part.Notes {
			if n.Rest || !inLayer(n, l.spec) {
				continue
			}
			notes = append(notes, n)
			if n.Unpitched {
				percussion = true
			}
		}
		if percussion {
			if err := emitKit(spec, li, pi, pm, part, l, notes, loops, tm, asm, stats); err != nil {
				return err
			}
			continue
		}
		emitPitched(spec, li, pi, pm, part, l, notes, loops, tm, asm, stats)
	}
	return nil
}

// place records one note as a hit, returning false (and counting a collision)
// when the row already has a node on that step.
func place(r *rowAcc, h Hit, stats *scoreStats) bool {
	if _, taken := r.hits[h.Step]; taken {
		stats.StepCollisions++
		return false
	}
	r.hits[h.Step] = h
	stats.NotesEmitted++
	return true
}

// hitAt builds a Hit for a note written at writtenSteps and performed at
// performedSteps, both measured from the excerpt's first downbeat.
func hitAt(writtenSteps, performedSteps float64, stats *scoreStats) Hit {
	step := int(math.Round(writtenSteps))
	return withOffset(Hit{Step: step}, performedSteps-float64(step), stats)
}

// withOffset stores an offset (in steps, positive = late) on a hit as the
// node's delay/rush, clamping to the one-step window.
func withOffset(h Hit, off float64, stats *scoreStats) Hit {
	if off > maxOffsetSteps || off < -maxOffsetSteps {
		stats.GrooveClamped++
		off = math.Max(-maxOffsetSteps, math.Min(maxOffsetSteps, off))
	}
	pct := roundPct(math.Abs(off))
	if pct == 0 {
		return h
	}
	h.GroovePct = pct
	if off > 0 {
		h.Groove = grooveDelay
	} else {
		h.Groove = grooveRush
	}
	return h
}

// roundPct quantises a groove percentage so regenerating the templates does not
// churn the JSON on floating-point noise.
func roundPct(v float64) float64 { return math.Round(v*1000) / 1000 }

// stepsOf converts a score onset into steps from the loop's first downbeat. A
// pickup note (before that downbeat) wraps to the end of the loop.
func stepsOf(l loadedLayer, onsetDiv int, tm timing) float64 {
	s := float64(onsetDiv-l.originDiv) / float64(l.div) * float64(tm.stepsPerQuarter)
	if s < 0 {
		s += loopStepsOf(l, tm)
	}
	return s
}

// swingFor returns the swing in force for a note: the part's own marking, or
// the template-wide groove swing when the part marks none.
func swingFor(part musicxml.Part, onsetDiv int, tm timing) (musicxml.Direction, bool) {
	for _, d := range part.Directions {
		if (d.Kind == musicxml.DirSwing || d.Kind == musicxml.DirStraight) && d.OnsetDiv <= onsetDiv {
			// The part speaks for itself about swing; honour it either way.
			return part.SwingAt(onsetDiv)
		}
	}
	return tm.swing, tm.haveSwing
}

// A piecewise-linear warp of each pair of swung units: the first half stretches to ratio/(ratio+1) of the pair and the
// second compresses, so the offbeat lands swung and anything between moves proportionally.
func swingShiftQuarters(posInBar float64, sw musicxml.Direction) float64 {
	unit := sw.SwingUnit
	if unit <= 0 || sw.SwingRatio <= 0 {
		return 0
	}
	pair := 2 * unit
	x := math.Mod(posInBar, pair)
	if x < 0 {
		x += pair
	}
	split := pair * sw.SwingRatio / (sw.SwingRatio + 1)
	var warped float64
	if x <= unit {
		warped = x * split / unit
	} else {
		warped = split + (x-unit)*(pair-split)/unit
	}
	return warped - x
}

// swingSteps returns the swing displacement of a note, in steps.
func swingSteps(part musicxml.Part, n musicxml.Note, l loadedLayer, tm timing) float64 {
	sw, ok := swingFor(part, n.OnsetDiv, tm)
	if !ok {
		return 0
	}
	m, ok := part.MeasureAt(n.OnsetDiv)
	if !ok {
		return 0
	}
	posInBar := float64(n.OnsetDiv-m.OnsetDiv) / float64(l.div)
	return swingShiftQuarters(posInBar, sw) * float64(tm.stepsPerQuarter)
}

// Precedence by specificity: a mark on THIS note (fermata) beats a span (rit./accel.), which beats a tenuto's lean.
func articulationBeats(part musicxml.Part, n musicxml.Note, l loadedLayer) float64 {
	if n.Articulations.Fermata {
		return fermataDragBeats
	}
	if b, ok := agogicBeats(part, n, l); ok {
		return b
	}
	if n.Articulations.Tenuto {
		return tenutoDragBeats
	}
	return 0
}

// A span runs to the next "a tempo" (unclosed: agogicSpanMeasures bars) and ramps from min to max: at full strength
// on its first note it would be a tempo change, not a ritardando.
func agogicBeats(part musicxml.Part, n musicxml.Note, l loadedLayer) (float64, bool) {
	var active musicxml.Direction
	var haveActive bool
	spanEnd := 0
	haveEnd := false

	for _, d := range part.Directions {
		if d.Measure < l.spec.From || d.Measure > l.spec.To {
			continue
		}
		switch d.Kind {
		case musicxml.DirRitardando, musicxml.DirAccelerando:
			if d.OnsetDiv <= n.OnsetDiv {
				active, haveActive = d, true
				spanEnd, haveEnd = 0, false
			}
		case musicxml.DirATempo:
			if d.OnsetDiv <= n.OnsetDiv {
				haveActive = false // the span closed before this note
			} else if haveActive && !haveEnd {
				spanEnd, haveEnd = d.OnsetDiv, true
			}
		}
	}
	if !haveActive {
		return 0, false
	}
	if !haveEnd {
		spanEnd, haveEnd = active.OnsetDiv+agogicSpanMeasures*measureLenAt(part, active.OnsetDiv, l.div), true
	}
	if n.OnsetDiv > spanEnd {
		return 0, false
	}
	b := agogicMaxBeats
	if spanEnd > active.OnsetDiv {
		frac := float64(n.OnsetDiv-active.OnsetDiv) / float64(spanEnd-active.OnsetDiv)
		b = agogicMinBeats + frac*(agogicMaxBeats-agogicMinBeats)
	}
	b = math.Max(agogicMinBeats, math.Min(agogicMaxBeats, b))
	if active.Kind == musicxml.DirAccelerando {
		b = -b
	}
	return b, true
}

// From the measure timeline, so a meter change sizes the span correctly; no timeline = a 4/4 bar.
func measureLenAt(part musicxml.Part, onsetDiv, div int) int {
	if m, ok := part.MeasureAt(onsetDiv); ok && m.Beats > 0 && m.BeatType > 0 {
		if v := div * 4 * m.Beats / m.BeatType; v > 0 {
			return v
		}
	}
	return div * 4
}

// breathDelays maps each onset to the extra time (beats) a breath mark or a
// caesura on the PREVIOUS onset takes from it.
func breathDelays(notes []musicxml.Note) map[int]float64 {
	marks := map[int]float64{}
	var onsets []int
	seen := map[int]bool{}
	for _, n := range notes {
		if n.Grace {
			continue
		}
		if !seen[n.OnsetDiv] {
			seen[n.OnsetDiv] = true
			onsets = append(onsets, n.OnsetDiv)
		}
		b := 0.0
		if n.Articulations.BreathMark {
			b = breathBeats
		}
		if n.Articulations.Caesura {
			b = caesuraBeats
		}
		if b > marks[n.OnsetDiv] {
			marks[n.OnsetDiv] = b
		}
	}
	sort.Ints(onsets)
	out := map[int]float64{}
	for i := 1; i < len(onsets); i++ {
		if b := marks[onsets[i-1]]; b > 0 {
			out[onsets[i]] = b
		}
	}
	return out
}

// One node per step per row, so chords split into lanes: notes grouped by ONSET across voices (a piano staff carries
// four or more MusicXML voices), lane 0 the highest (LOWEST for a bass part), the rest past MaxVoices dropped and
// counted. Grace notes go before their principal in its lane, or a higher grace would outrank the written note.
func emitPitched(spec TemplateSpec, li, pi int, pm PartMap, part musicxml.Part, l loadedLayer, notes []musicxml.Note, loops int, tm timing, asm *assembler, stats *scoreStats) {
	maxVoices := pm.MaxVoices
	if maxVoices <= 0 {
		maxVoices = 1
	}
	pitchShift := instrumentPitchShift(pm.Inst)
	if pm.SoundingPitch {
		pitchShift = soundingPitchShift(pm.Inst)
	}
	breaths := breathDelays(notes)

	var regular, graces []musicxml.Note
	for _, n := range notes {
		if pm.BelowMIDI > 0 && n.Semitone+a3MIDI >= pm.BelowMIDI {
			stats.RegisterDropped += loops
			continue
		}
		if n.Grace {
			graces = append(graces, n)
		} else {
			regular = append(regular, n)
		}
	}

	groups := map[int][]musicxml.Note{}
	var onsets []int
	for _, n := range regular {
		if _, ok := groups[n.OnsetDiv]; !ok {
			onsets = append(onsets, n.OnsetDiv)
		}
		groups[n.OnsetDiv] = append(groups[n.OnsetDiv], n)
	}
	sort.Ints(onsets)

	type laned struct {
		n      musicxml.Note
		lane   int
		rollBe float64 // arpeggio offset in beats
	}
	var keep []laned
	laneOf := map[[3]int]int{} // (onset, voice, staff) → lane of a principal
	for _, onset := range onsets {
		g := groups[onset]
		// Rolled chord: rank every tone bottom-up (or top-down for a downward
		// roll) BEFORE the lane cap, because the roll is played on all of them.
		roll := 0
		for _, n := range g {
			if n.Articulations.Arpeggiate != 0 {
				roll = n.Articulations.Arpeggiate
			}
		}
		asc := append([]musicxml.Note(nil), g...)
		sort.SliceStable(asc, func(i, j int) bool { return asc[i].Semitone < asc[j].Semitone })
		rollIdx := func(n musicxml.Note) int {
			for i := range asc {
				if asc[i].Semitone == n.Semitone && asc[i].Voice == n.Voice && asc[i].Staff == n.Staff {
					if roll < 0 {
						return len(asc) - 1 - i
					}
					return i
				}
			}
			return 0
		}

		sort.SliceStable(g, func(i, j int) bool {
			if g[i].Semitone != g[j].Semitone {
				if pm.Lowest {
					return g[i].Semitone < g[j].Semitone
				}
				return g[i].Semitone > g[j].Semitone
			}
			// Ties: a held note outranks a passing one at the same pitch.
			return g[i].DurationDiv > g[j].DurationDiv
		})
		for p, n := range g {
			if p >= maxVoices {
				stats.ChordTonesDropped += loops
				continue
			}
			r := 0.0
			if roll != 0 {
				// Compressed so the whole roll fits the one-step window.
				spacing := arpRollBeats
				if k := len(asc); k > 1 {
					spacing = math.Min(spacing, maxOffsetSteps/float64(tm.stepsPerQuarter)/float64(k-1))
				}
				r = float64(rollIdx(n)) * spacing
			}
			keep = append(keep, laned{n: n, lane: p, rollBe: r})
			laneOf[[3]int{onset, n.Voice, n.Staff}] = p
		}
	}

	rowFor := func(lane int) *rowAcc {
		return asm.row(rowKey{layer: li, part: pi, lane: lane}, func() InstSpec {
			// A part split into lanes yields several rows on one instrument;
			// the row LABEL is what the user reads, so number them.
			name := pm.Display
			if lane > 0 {
				name = fmt.Sprintf("%s ·%d", pm.Display, lane+1)
			}
			return instSpec(pm, pm.Inst, name, pm.Volume)
		})
	}

	loopSteps := float64(l.lengthDiv) / float64(l.div) * float64(tm.stepsPerQuarter)
	for rep := 0; rep < loops; rep++ {
		base := float64(rep) * loopSteps
		for _, k := range keep {
			n := k.n
			written := base + stepsOf(l, n.OnsetDiv, tm)
			offBeats := articulationBeats(part, n, l) + k.rollBe + breaths[n.OnsetDiv]
			performed := written + swingSteps(part, n, l, tm) + offBeats*float64(tm.stepsPerQuarter)
			h := hitAt(written, performed, stats)
			h.Pitch = float64(n.Semitone+12*pm.Octave) - pitchShift
			h.Vol = n.Velocity // 0 => instrument default
			h.Dur = noteDuration(pm.Inst, float64(n.DurationDiv)/float64(l.div))
			place(rowFor(k.lane), h, stats)
		}
		// Grace notes, after their principals so the principal owns its step.
		emitGraces(pm, l, graces, laneOf, base, rowFor, pitchShift, tm, stats)
	}
}

// emitGraces places each grace note on the step(s) before its principal, in the
// principal's lane.
func emitGraces(pm PartMap, l loadedLayer, graces []musicxml.Note, laneOf map[[3]int]int,
	base float64, rowFor func(int) *rowAcc, pitchShift float64, tm timing, stats *scoreStats) {
	// Graces sharing a principal are ordered as written; the LAST is nearest.
	type key [3]int
	byPrincipal := map[key][]musicxml.Note{}
	var order []key
	for _, g := range graces {
		k := key{g.OnsetDiv, g.Voice, g.Staff}
		if _, ok := byPrincipal[k]; !ok {
			order = append(order, k)
		}
		byPrincipal[k] = append(byPrincipal[k], g)
	}
	for _, k := range order {
		gs := byPrincipal[k]
		lane, ok := laneOf[k]
		if !ok {
			// The principal was dropped (below the voice cap) or is absent:
			// its ornament goes with it.
			stats.ChordTonesDropped += len(gs)
			continue
		}
		pWritten := base + stepsOf(l, k[0], tm)
		pStep := math.Round(pWritten)
		// The nearest grace sounds graceLeadBeats ahead, from the step before; each earlier grace of a run takes the step
		// before that, at the same point in its step (packing at the lead's spacing would exceed the groove window).
		lastOffset := (pWritten - graceLeadBeats*float64(tm.stepsPerQuarter)) - (pStep - 1)
		// An off-grid principal can put the grace's ideal time inside the principal's own step; it then sounds AT that step.
		lastOffset = math.Min(lastOffset, maxOffsetSteps)
		for i, g := range gs {
			back := float64(len(gs) - i)
			written := pStep - back
			performed := written + lastOffset
			if written < 0 {
				// A grace before the excerpt's first downbeat wraps to the end
				// of the loop, where the circuit plays it before the downbeat.
				written += loopStepsOf(l, tm)
				performed += loopStepsOf(l, tm)
			}
			h := hitAt(written, performed, stats)
			h.Pitch = float64(g.Semitone+12*pm.Octave) - pitchShift
			h.Vol = g.Velocity
			place(rowFor(lane), h, stats)
		}
	}
}

// Only a melodic voice honours a duration by sounding shorter; every other voice is sized by resampling, which
// transposes it by -12·log2(duration), so it gets none (0 = the instrument default).
func noteDuration(inst instrument.ID, quarters float64) float64 {
	if !audio.IsMelodicInstrument(inst) {
		return 0
	}
	return quarters
}

func loopStepsOf(l loadedLayer, tm timing) float64 {
	return float64(l.lengthDiv) / float64(l.div) * float64(tm.stepsPerQuarter)
}

// Kit pieces on one instrument share its row (tom heights are pitches on one tom row); a second hit on one step spills
// into a second row of that instrument, and a further collision is counted.
func emitKit(spec TemplateSpec, li, pi int, pm PartMap, part musicxml.Part, l loadedLayer, notes []musicxml.Note, loops int, tm timing, asm *assembler, stats *scoreStats) error {
	// Assign rows in a stable order: by instrument, in the order of the kit's
	// GM keys, so a kick row always precedes the snare.
	type piece struct {
		key int
		kp  KitPiece
	}
	var pieces []piece
	seenKey := map[int]bool{}
	for _, n := range notes {
		if seenKey[n.PercKey] {
			continue
		}
		seenKey[n.PercKey] = true
		kp, ok := kitPieceFor(pm, n.PercKey)
		if !ok {
			return fmt.Errorf("template %q: score %q part %q plays GM percussion key %d (%s), which has no Beatmo instrument — add it to the part's Kit or to gmKit",
				spec.Stem, l.spec.ScoreID, part.Name, n.PercKey, gmName(n.PercKey))
		}
		pieces = append(pieces, piece{n.PercKey, kp})
	}
	sort.Slice(pieces, func(i, j int) bool { return pieces[i].key < pieces[j].key })
	instOrder := map[instrument.ID]int{}
	for _, p := range pieces {
		if _, ok := instOrder[p.kp.Inst]; !ok {
			instOrder[p.kp.Inst] = len(instOrder)
		}
	}
	// Create the rows up front so their order follows the kit, not the order
	// in which the first hit of each happens to occur.
	rowFor := func(kp KitPiece, spill int) *rowAcc {
		return asm.row(rowKey{layer: li, part: pi, lane: instOrder[kp.Inst], kitInst: kp.Inst, spill: spill}, func() InstSpec {
			name := kp.Display
			if spill > 0 {
				name = fmt.Sprintf("%s ·%d", kp.Display, spill+1)
			}
			vol := pm.Volume
			if kp.Volume > 0 {
				vol = kp.Volume
			}
			return instSpec(pm, kp.Inst, name, vol)
		})
	}
	byInst := map[instrument.ID]KitPiece{}
	for _, p := range pieces {
		if _, ok := byInst[p.kp.Inst]; !ok {
			byInst[p.kp.Inst] = p.kp
		}
	}
	insts := make([]string, 0, len(byInst))
	for inst := range byInst {
		insts = append(insts, string(inst))
	}
	sort.Slice(insts, func(i, j int) bool { return instOrder[instrument.ID(insts[i])] < instOrder[instrument.ID(insts[j])] })
	for _, inst := range insts {
		rowFor(byInst[instrument.ID(inst)], 0)
	}

	sorted := append([]musicxml.Note(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].OnsetDiv < sorted[j].OnsetDiv })
	for rep := 0; rep < loops; rep++ {
		base := float64(rep) * loopStepsOf(l, tm)
		for _, n := range sorted {
			kp, _ := kitPieceFor(pm, n.PercKey)
			written := base + stepsOf(l, n.OnsetDiv, tm)
			performed := written + swingSteps(part, n, l, tm) +
				articulationBeats(part, n, l)*float64(tm.stepsPerQuarter)
			if n.Grace {
				// A drum grace note is a flam: struck just ahead, one step back.
				written = math.Round(written) - 1
				performed -= graceLeadBeats * float64(tm.stepsPerQuarter)
				if written < 0 {
					written += loopStepsOf(l, tm)
					performed += loopStepsOf(l, tm)
				}
			}
			h := hitAt(written, performed, stats)
			h.Pitch = kp.Pitch
			h.Vol = n.Velocity
			// A drum hit's notated duration is not a sustain; the instrument's
			// own decay is the sound.
			r := rowFor(kp, 0)
			if _, taken := r.hits[h.Step]; taken {
				r = rowFor(kp, 1)
			}
			place(r, h, stats)
		}
	}
	return nil
}

func instSpec(pm PartMap, inst instrument.ID, name string, vol float64) InstSpec {
	if vol <= 0 {
		// Must be > 0: export writes volume without omitempty and import copies it with no 0→default fallback.
		vol = defaultScoreInstVolume
	}
	return InstSpec{ID: inst, Name: name, Volume: vol}
}

// a3MIDI is the MIDI key the score model calls Semitone 0.
const a3MIDI = 57

// instrumentPitchShift is the whole-semitone transposition an instrument applies to every node (InstrumentPitchInfo),
// cancelled so the circuit sounds the WRITTEN pitch; a fractional part is a deliberate detune. Gated on render.melodic,
// NOT InstrumentPitchInfo's pitched (true of drums too): that would transpose every percussion part.
func instrumentPitchShift(instID instrument.ID) float64 {
	if !audio.IsMelodicInstrument(instID) {
		return 0
	}
	offset, pitched := audio.InstrumentPitchInfo(instID)
	if !pitched {
		return 0
	}
	return math.Round(offset)
}

// soundingPitchShift cancels the FULL measured offset, so a timpani part on a drum voice sounds the written pitch:
// the tom's tuning is not a whole number of semitones.
func soundingPitchShift(instID instrument.ID) float64 {
	offset, pitched := audio.InstrumentPitchInfo(instID)
	if !pitched {
		return 0
	}
	return math.Round(offset*1000) / 1000
}
