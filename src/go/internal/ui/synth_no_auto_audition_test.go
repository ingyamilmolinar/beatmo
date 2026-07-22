package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests pin the runtime-edit contract for the Synth tab:
//
//	Editing a knob or a stage pill ONLY updates the instrument config
//	(audio.SetInstrumentParam → voice-cache invalidation → the NEXT
//	trigger renders with the new values). It must NOT auto-play the
//	instrument. The one-shot audition is reserved for the explicit
//	Preview button (previewActiveSynth).
//
// Why: the auto-audition fired audio.Play(id) on every knob release and
// stage toggle — an immediate, full-gain (vol=1.0) one-shot with no
// scheduler lead that bypasses the row/node volume sequencer hits get.
// Layered over the scheduled voices of the same instrument during
// playback it overshoots the master chain and is heard as a crackling /
// sharp transient alongside the unwanted "preview".

// newSnareSynthTabGame builds the standard snare/drum-snare synth-tab
// harness used by the drag e2e tests.
func newSnareSynthTabGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	return g
}

// firstVisibleSynthControl returns the index, knob, and hit area of the first
// laid-out (non-empty rect) synth knob.
func firstVisibleSynthControl(t *testing.T, g *Game) (int, *Knob, *HitArea) {
	t.Helper()
	knobs := g.drum.SynthTabControls()
	idx := -1
	for i, k := range knobs {
		if !k.Rect().Empty() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no visible synth knob")
	}
	var hit *HitArea
	hits := g.drum.eqPanelZone.HitAreas()
	for i, h := range hits {
		if h.Tag == fmt.Sprintf("synth-control-%d", idx) {
			hit = &hits[i]
			break
		}
	}
	if hit == nil {
		var tags []string
		for _, h := range hits {
			tags = append(tags, h.Tag)
		}
		t.Fatalf("no hit area for synth-control-%d; tags=%s", idx, strings.Join(tags, ","))
	}
	return idx, knobs[idx], hit
}

// TestSynthStageTogglePill_DoesNotAutoAudition flips a modular per-stage
// bypass pill. The toggle must mutate the param (next trigger renders with
// the stage bypassed/enabled) but must not auto-play the instrument.
func TestSynthStageTogglePill_DoesNotAutoAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)

	var auditions int
	prev := SwapSynthAuditionFnForTest(func(string) { auditions++ })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	const instID, param = "ut-chip-modular", "filter_enabled"
	before := synthStageEnabled(instID, param)
	g.drum.toggleSynthStage(instID, param)

	if got := synthStageEnabled(instID, param); got == before {
		t.Fatalf("stage toggle did not flip %s (still %v)", param, got)
	}
	if auditions != 0 {
		t.Errorf("stage toggle auto-auditioned %d time(s); want 0 (config-only, next trigger picks it up)", auditions)
	}
}

// TestPreviewButton_StillAuditionsExactlyOnce is the guard rail for the
// fix: removing the automatic audition must NOT remove the explicit
// Preview button's one-shot.
func TestPreviewButton_StillAuditionsExactlyOnce(t *testing.T) {
	g := newSnareSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	var auditions []string
	prev := SwapSynthAuditionFnForTest(func(id string) { auditions = append(auditions, id) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	g.drum.previewActiveSynth()

	if len(auditions) != 1 || auditions[0] != "snare" {
		t.Errorf("Preview button auditions = %v, want exactly one for %q", auditions, "snare")
	}
}

// TestSetInstrumentParam_NeverTriggersPlayback pins the audio-layer half of
// the contract directly: a param write dispatches config (platform callback,
// voice-cache invalidation) but never schedules a voice. Every Play* entry
// point (desktop, WASM, stub) stamps RecordVoiceTrigger, so an untouched
// LastTriggerAt proves no playback happened. This is what "change the config
// and let the next trigger pick it up" means at the engine boundary.
func TestSetInstrumentParam_NeverTriggersPlayback(t *testing.T) {
	assertDefaultParityState(t)
	const instID = "no-auto-audition-probe"
	audio.BindInstrumentToRecipe(instID, "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	if !audio.LastTriggerAt(instID).IsZero() {
		t.Fatalf("probe instrument %q already has a trigger timestamp", instID)
	}
	audio.SetInstrumentParam(instID, "decay", 2.5)
	if got := audio.LastTriggerAt(instID); !got.IsZero() {
		t.Errorf("SetInstrumentParam triggered playback: LastTriggerAt = %v, want zero (no voice scheduled)", got)
	}
}

// TestSynthControlWheelNudge_DoesNotAutoAudition drives the Task-5 desktop wheel
// nudge (synthControlHitAdapter.OnWheel): a notch over the card must propagate the
// param (next-trigger model) and NOT fire an audition.
func TestSynthControlWheelNudge_DoesNotAutoAudition(t *testing.T) {
	g := newSnareSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	var auditions []string
	prev := SwapSynthAuditionFnForTest(func(id string) { auditions = append(auditions, id) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })
	idx, knob, hit := firstVisibleSynthControl(t, g)
	paramName := g.drum.SynthTabBindings()[idx].def.Name
	r := knob.Rect()
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	knob.Value = 0.25 // low value so +1 notch always increases (discrete or endless)
	if res := hit.Handler.OnWheel(cx, cy, 1); res != InputConsumed {
		t.Fatalf("wheel over knob card should be consumed (nudge), got %v", res)
	}
	if _, ok := audio.GetInstrumentParams("snare")[paramName]; !ok {
		t.Fatalf("param %q was not propagated to the audio config on wheel nudge", paramName)
	}
	if len(auditions) != 0 {
		t.Errorf("knob wheel nudge auto-auditioned %v; want none", auditions)
	}
}
