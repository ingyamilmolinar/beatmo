//go:build !test && !js

package ui

import (
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// dialChangeAudible reports whether played differs audibly from centre (the
// shared oracle, soundview.CompareNotes), and the two measurements behind the
// verdict.
func dialChangeAudible(played, centre []float64, sr int) (ok bool, db, cents float64) {
	c := soundview.CompareNotes(played, centre, sr)
	return c.Audible(), c.GlobalDB, c.Cents
}

// dialMeasurableMinTravel is the smallest move (in span coordinate) a dial end
// must make on at least one of its params: 5 % of the term's span.
const dialMeasurableMinTravel = 0.05

// dialMeasurableMinGap is how far (in readout units) a dial end must be from
// the shipped reading before it is held to the contract: an end the sound
// already sits at (Drive 0 on a clean voice) has nowhere to go.
const dialMeasurableMinGap = 10

// TestEveryDialIsMeasurable is the contract behind the dials (spec
// 2026-10-13 §6.3, 2026-10-25 §3.2): no dead or ineffective control is SHOWN.
// The DSP decides which dials an instrument shows (synth_dial_relevance.go:
// a dial whose 0 → 100 sweep is inaudible is hidden), so there is no
// exception list: for every shipped instrument and every dial its root shows
// after that verdict, the dial is turned to 0 and to 100 through the live
// edit path (setSynthDialReading → one SetInstrumentParamsLive, what a drag
// does) and the instrument is triggered through the playback path
// (RenderAsPlayed: the cached, length-gated, peak-normalized voice a soloed
// row actually plays).
//
//   - PARAMS: each end ≥ dialMeasurableMinGap from the shipped reading moves at
//     least one term by ≥ dialMeasurableMinTravel of its span.
//   - AUDIO: the 0 ↔ 100 sweep sounds different (soundview.CompareNotes) —
//     on the PLAYED voice, independently of the graph render the verdicts
//     were measured on.
//
// Dials that appear only once a stage is switched on are held to PARAMS; the
// sound with that stage on is measured by the same relevance function when
// the tab first shows it.
func TestEveryDialIsMeasurable(t *testing.T) {
	g := buildTestGame(t)
	dv := g.drum
	// Pin everything the played voice depends on, so the verdicts are the
	// same alone and after any other test in the package: the shipped
	// documents (a private, empty user library). The tempo is not one of
	// them: a voice is render.seconds long at every tempo.
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	t.Cleanup(audio.ResetInstrumentDocs)
	// Measure the sound, not the prewarm race: a dispatch miss renders the new
	// voice instead of serving the best cached one (delivery has its own test,
	// TestSynthDialTurnReachesEveryPlayedNote).
	audio.SetDispatchMustNotRender(false)
	t.Cleanup(func() { audio.SetDispatchMustNotRender(true) })

	checked, swept := 0, 0
	measure := func(id, variant string, base audio.RecipeParams, cardID string, d synthdial.Resolved, setup func(), sweep bool) {
		reset := func() {
			audio.ResetInstrumentParams(instrument.ID(id))
			if setup != nil {
				setup()
			}
		}
		reset()
		r0 := synthDialPercent(d.Reading(base))
		key := cardID + "/" + d.Dial.ID
		var at [2][]float64
		sr := 0
		for i, end := range []int{0, 100} {
			dv.setSynthDialReading(instrument.ID(id), d, end)
			after := audio.MergedInstrumentParamsRO(instrument.ID(id))
			if sweep {
				at[i], sr = audio.RenderAsPlayed(instrument.ID(id), 0, 1, 4)
			}
			travel := 0.0
			for _, tm := range d.Terms {
				travel = math.Max(travel, math.Abs(tm.Coord(after[tm.Param])-tm.Coord(base[tm.Param])))
			}
			reset()
			if abs(end-r0) < dialMeasurableMinGap {
				continue
			}
			checked++
			if travel < dialMeasurableMinTravel {
				t.Errorf("%s%s %s: turning to %d moved no param by %.0f%% of its span (max %.3f)", id, variant, key, end, 100*dialMeasurableMinTravel, travel)
			}
		}
		if !sweep {
			return
		}
		swept++
		if c := soundview.CompareNotes(at[1], at[0], sr); !c.Audible() {
			t.Errorf("%s%s %s is SHOWN but its 0 ↔ 100 sweep is inaudible on the played voice (%.2f dB, local %.2f dB, band %.2f dB, %.1f¢): the relevance verdict is stale (make gen-dial-relevance) or the graph render and the played voice disagree", id, variant, key, c.GlobalDB, c.LocalDB, c.BandDB, c.Cents)
		}
	}
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		audio.ResetInstrumentParams(id)
		base := cloneRecipeParams(audio.MergedInstrumentParamsRO(id))
		secs := dv.synthVoiceSecs(id)
		doc, _ := audio.InstrumentDoc(id)
		hidden, ok := shippedDialRelevance(audio.SoundSignature(doc))
		if !ok {
			t.Errorf("%s: no relevance verdict for its shipped sound — run make gen-dial-relevance", id)
		}
		onRoot := map[string]bool{}
		for _, cg := range withoutDials(buildMacroGroupsForVoice(schema, base, secs), hidden) { // as the Synth root shows them
			for _, d := range cg.Dials {
				onRoot[d.Dial.ID] = true
				measure(string(id), "", base, cg.ID, d, nil, true)
			}
		}
		for _, k := range hidden {
			onRoot[k[strings.Index(k, "/")+1:]] = true
		}
		// Dials that appear only once their stage is switched on (the
		// developer's "+ Add" / stage pill): PARAMS on this instrument with
		// exactly the stages their terms live in switched on.
		gates := synthStageGates(schema)
		defs := map[string]audio.ParamDef{}
		for _, pd := range schema {
			defs[pd.Name] = pd
		}
		allOn := cloneRecipeParams(base)
		for _, en := range gates {
			allOn[en] = 1
		}
		for _, cg := range buildMacroGroupsAll(schema, allOn) {
			for _, d := range cg.Dials {
				if onRoot[d.Dial.ID] {
					continue
				}
				enables := map[string]float64{}
				for _, tm := range d.Terms {
					if en := gates[sectionForParamDef(defs[tm.Param])]; en != "" && base[en] < 0.5 {
						enables[en] = 1
					}
				}
				if len(enables) == 0 {
					continue // not stage-gated: hidden by its own gates on this sound
				}
				on := cloneRecipeParams(base)
				for k, v := range enables {
					on[k] = v
				}
				var resolved *synthdial.Resolved
				for _, cg2 := range buildMacroGroupsForVoice(schema, on, secs) {
					for _, d2 := range cg2.Dials {
						if d2.Dial.ID == d.Dial.ID {
							resolved = &d2
						}
					}
				}
				if resolved == nil {
					continue
				}
				measure(string(id), "+stage", on, cg.ID, *resolved, func() { audio.SetInstrumentParamsLive(id, enables) }, false)
				onRoot[d.Dial.ID] = true
			}
		}
		audio.ResetInstrumentParams(id)
	}
	if checked < 600 || swept < 600 {
		t.Fatalf("only %d dial ends / %d sweeps checked — the roster walk is broken", checked, swept)
	}
	t.Logf("checked %d dial ends and %d shown sweeps", checked, swept)
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cloneRecipeParams(m audio.RecipeParams) audio.RecipeParams {
	out := make(audio.RecipeParams, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
