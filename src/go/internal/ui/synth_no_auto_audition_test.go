package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A knob or stage pill edit only changes the config the NEXT trigger renders; only Preview auditions. An automatic
// full-gain one-shot layered over the scheduled voices overshot the master and crackled.

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
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	return g
}

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

// The param changes; the instrument does not play.
func TestSynthStageTogglePill_DoesNotAutoAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)

	var auditions int
	prev := SwapSynthAuditionFnForTest(func(instrument.ID) { auditions++ })
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

// Removing the automatic audition must not remove Preview's one-shot.
func TestPreviewButton_StillAuditionsExactlyOnce(t *testing.T) {
	g := newSnareSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	var auditions []string
	prev := SwapSynthAuditionFnForTest(func(id instrument.ID) { auditions = append(auditions, string(id)) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	g.drum.previewActiveSynth()

	if len(auditions) != 1 || auditions[0] != "snare" {
		t.Errorf("Preview button auditions = %v, want exactly one for %q", auditions, "snare")
	}
}

// A param write dispatches config but never schedules a voice: every Play* entry point stamps RecordVoiceTrigger,
// so an untouched LastTriggerAt proves it.
func TestSetInstrumentParam_NeverTriggersPlayback(t *testing.T) {
	assertDefaultParityState(t)
	const instID = "no-auto-audition-probe"
	_ = audio.InstallInstrumentDocForTest(instID, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(instID) })

	if !audio.LastTriggerAt(instID).IsZero() {
		t.Fatalf("probe instrument %q already has a trigger timestamp", instID)
	}
	audio.SetInstrumentParam(instID, "amp_decay", 2.5)
	if got := audio.LastTriggerAt(instID); !got.IsZero() {
		t.Errorf("SetInstrumentParam triggered playback: LastTriggerAt = %v, want zero (no voice scheduled)", got)
	}
}

// An armed value-lane notch propagates the param and auditions nothing; an unarmed wheel only scrolls.
func TestSynthControlWheelNudge_DoesNotAutoAudition(t *testing.T) {
	g := newSnareSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	var auditions []string
	prev := SwapSynthAuditionFnForTest(func(id instrument.ID) { auditions = append(auditions, string(id)) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })
	idx, knob, _ := firstVisibleSynthControl(t, g)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()
	paramName := dv.SynthTabBindings()[idx].def.Name
	r := knob.Rect()
	knob.Value = 0.25 // low value so the notch always increases (discrete or endless)

	dv.armSynthControl(instID, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)
	h := &synthControlHitAdapter{dv: dv, idx: idx, instID: instID}
	vx, vy := e.ValueLaneRect().Min.X+2, r.Min.Y+r.Dy()/2

	if res := h.OnWheel(vx, vy, 1); res != InputConsumed {
		t.Fatalf("armed wheel over the value lane should be consumed (nudge), got %v", res)
	}
	// ImpulseNotch becomes a value change only through Tick.
	for i := 0; i < 240 && e.Tick(); i++ {
	}
	if _, ok := audio.GetInstrumentParams(instID)[paramName]; !ok {
		t.Fatalf("param %q was not propagated to the audio config on armed wheel nudge", paramName)
	}
	if knob.Value <= 0.25 {
		t.Fatalf("armed wheel nudge should have increased the knob value above 0.25, got %.6f", knob.Value)
	}
	if len(auditions) != 0 {
		t.Errorf("armed knob wheel nudge auto-auditioned %v; want none", auditions)
	}
}
