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

// Through the shared oracle, soundview.CompareNotes.
func dialChangeAudible(played, centre []float64, sr int) (ok bool, db, cents float64) {
	c := soundview.CompareNotes(played, centre, sr)
	return c.Audible(), c.GlobalDB, c.Cents
}

// In span coordinate.
const dialMeasurableMinTravel = 0.05

// In readout units: an end the sound already sits at has nowhere to go.
const dialMeasurableMinGap = 10

// No dead control is shown, with no exception list: every dial shown, turned to 0 and 100 through the live edit
// path, moves a term ≥ 5% of its span from any end ≥ 10 away, and its sweep is audible on the PLAYED voice.
// Stage-gated dials are held to the param half.
func TestEveryDialIsMeasurable(t *testing.T) {
	g := buildTestGame(t)
	dv := g.drum
	// The shipped documents and an empty user library, so the verdicts do not depend on test order.
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	t.Cleanup(audio.ResetInstrumentDocs)
	// A miss renders instead of serving the best cached voice, so this measures the sound, not the prewarm race.
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
		// With exactly the stages their terms live in switched on.
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
