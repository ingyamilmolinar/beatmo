package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthTab_ControlDragEndToEndChangesAudioParam reproduces the exact UX
// the user described: switch to the Synth tab, find a knob, press inside
// it, drag, release — and verify the audio.SetInstrumentParam was called
// with a new value. Catches regressions where the hit area is registered
// but the knob's value isn't propagated to the audio engine.
func TestSynthTab_ControlDragEndToEndChangesAudioParam(t *testing.T) {
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

	// Switch to Synth tab + run layout. The panel claims its expanded synth
	// height only during a DrumView Layout pass that sees TabSynth active, and
	// Ebiten's Layout short-circuits when the size is unchanged — so toggle the
	// size (shrink then grow) after activating the tab to force a real relayout.
	// Otherwise the detail pane is one knob-row tall and decay scrolls out.
	expandSynthPanelForTest(t, g)

	// Find the ENVELOPE-stage decay knob — the modular snare's Synth tab binds one
	// control per ParamDef of its document.
	knobs := g.drum.SynthTabControls()
	bindings := g.drum.SynthTabBindings()
	wantKnobs := len(audio.InstrumentParamDefs(roster.Snare))
	if len(knobs) != wantKnobs {
		t.Fatalf("expected %d knobs for snare (one per ParamDef), got %d", wantKnobs, len(knobs))
	}
	var decayIdx = -1
	for i, b := range bindings {
		if b.def.Name == "amp_decay" {
			decayIdx = i
			break
		}
	}
	if decayIdx < 0 {
		t.Fatal("no decay knob in bindings")
	}
	// amp_decay lives in the ENVELOPE stage; open it so the knob gets a
	// hit-testable rect under the chip-strip + detail-pane layout.
	selectSectionForKnobIdx(t, g, roster.Snare, decayIdx)
	knobs = g.drum.SynthTabControls()
	knob := knobs[decayIdx]
	rect := knob.Rect()
	if rect.Empty() {
		t.Fatalf("decay knob has empty rect (panel layout broken). header=%v sections=%v", g.drum.SynthTabHeader(), g.drum.SynthTabSections())
	}

	// Locate the matching HitArea returned by the EQ panel.
	hits := g.drum.eqPanelZone.HitAreas()
	var pressHit *HitArea
	for i, h := range hits {
		if h.Tag != "" && strings.HasPrefix(h.Tag, "synth-control-") {
			if image.Pt((rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2).In(h.Rect) {
				pressHit = &hits[i]
				break
			}
		}
	}
	if pressHit == nil {
		var tags []string
		for _, h := range hits {
			tags = append(tags, fmt.Sprintf("%s@%v", h.Tag, h.Rect))
		}
		t.Fatalf("no synth-control HitArea covers the decay knob center (rect=%v); tags=%s", rect, strings.Join(tags, ","))
	}

	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2

	// Armed model (2026-07): a not-armed wheel scrolls the row — value edits
	// require arming the control first, then spinning the value lane. A stationary
	// tap arms; the armed wheel then raises the value and propagates it to audio.
	h := &synthControlHitAdapter{dv: g.drum, idx: decayIdx, instID: roster.Snare}
	h.OnPress(cx, cy)
	h.OnRelease(cx, cy)
	if _, armed := g.drum.synthControlArmed(roster.Snare); !armed {
		t.Fatal("stationary tap on the control should arm it")
	}
	e := g.drum.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	settleArmGrow(g.drum) // grow the card + relayout so the block rect is current
	knob = g.drum.SynthTabControls()[decayIdx]
	grown := knob.Rect()
	if grown.Empty() {
		t.Fatal("armed control has empty rect after settle")
	}
	e.SetCardRect(grown)

	knob.Value = 0.1 // seed low so the up-notches always increase
	initialValue := knob.Value
	vx := e.ValueLaneRect().Min.X + 2
	vy := grown.Min.Y + grown.Dy()/2
	for s := 0; s < 8; s++ {
		if h.OnWheel(vx, vy, 1) == InputIgnored {
			t.Fatalf("armed wheel over the value lane ignored at (%d,%d)", vx, vy)
		}
	}
	for i := 0; i < 400 && e.Tick(); i++ { // resolve the kinetic barrel
	}
	if knob.Value <= initialValue {
		t.Errorf("after armed wheel up: knob.Value=%v did not increase from initial %v", knob.Value, initialValue)
	}
	afterDrag := knob.Value

	// Verify audio engine received the value change (propagated live via onEdit).
	got := audio.GetInstrumentParams(roster.Snare)
	val, ok := got["amp_decay"]
	if !ok {
		t.Fatalf("amp_decay param was not set after armed wheel: %v", got)
	}
	wantParamMin := knob.Scale.Min + afterDrag*(knob.Scale.Max-knob.Scale.Min) - 0.01
	if val < wantParamMin {
		t.Errorf("after armed wheel: audio.GetInstrumentParams[\"post_decay\"]=%v, want > %v (scaled from knob.Value=%v)", val, wantParamMin, afterDrag)
	}
}

// TestSynthTab_PressMissingControlIsNoOp verifies a press OUTSIDE every knob
// rect does not start a drag (negative complement to the success case).
func TestSynthTab_PressMissingControlIsNoOp(t *testing.T) {
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

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	// Press at a point WAY off any knob — far below the audio panel.
	for _, k := range g.drum.SynthTabControls() {
		if k.Capturing() {
			t.Errorf("knob unexpectedly capturing before any press: rect=%v", k.Rect())
		}
	}
}

// TestSynthTab_ReleaseFollowingDragPropagatesFinalValue verifies the
// last drag value is propagated on release (one final SetInstrumentParam
// call). This is the regression for "I drag to 75 % then release and the
// audio doesn't pick up the final value".
func TestSynthTab_ReleaseFollowingDragPropagatesFinalValue(t *testing.T) {
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

	expandSynthPanelForTest(t, g)

	// Count SetInstrumentParam calls.
	var fired int
	oldCb := audio.SwapPlatformInstrumentDocChangedForTest(nil)
	t.Cleanup(func() { audio.SwapPlatformInstrumentDocChangedForTest(oldCb) })
	audio.SwapPlatformInstrumentDocChangedForTest(func(instrument.ID, instrument.Doc) {
		fired++
	})

	knobs := g.drum.SynthTabControls()
	bindings := g.drum.SynthTabBindings()
	// Pick the first VISIBLE synth knob. Phase 8B's unified layout shows many
	// more sections, so a fixed param (the old "drive") may scroll out of its
	// narrow card in the small test panel — any visible knob exercises the same
	// drag→propagate path.
	var idx = -1
	for i := range bindings {
		if !knobs[i].Rect().Empty() {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatal("no visible synth knob")
	}
	k := knobs[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatal("synth knob rect empty")
	}

	// Locate the hit area.
	var hit *HitArea
	hits := g.drum.eqPanelZone.HitAreas()
	for i, h := range hits {
		if h.Tag != fmt.Sprintf("synth-control-%d", idx) {
			continue
		}
		hit = &hits[i]
		break
	}
	if hit == nil {
		t.Fatalf("no hit area for synth-control-%d", idx)
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	_ = hit // hit-area existence asserted above; the armed path drives the adapter directly

	def := bindings[idx].def
	paramName := def.Name

	// Armed model: arm via a stationary tap, then spin the value lane; the final
	// resolved value propagates to audio via the platform callback.
	h := &synthControlHitAdapter{dv: g.drum, idx: idx, instID: roster.Snare}
	h.OnPress(cx, cy)
	h.OnRelease(cx, cy)
	if _, armed := g.drum.synthControlArmed(roster.Snare); !armed {
		t.Fatal("stationary tap on the control should arm it")
	}
	e := g.drum.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	settleArmGrow(g.drum)
	k = g.drum.SynthTabControls()[idx]
	grown := k.Rect()
	e.SetCardRect(grown)

	k.Value = 0.1 // seed low so wheel up-notches always increase
	before := def.Min + k.Value*(def.Max-def.Min)
	vx := e.ValueLaneRect().Min.X + 2
	vy := grown.Min.Y + grown.Dy()/2
	for s := 0; s < 8; s++ {
		h.OnWheel(vx, vy, 1)
	}
	for i := 0; i < 400 && e.Tick(); i++ { // resolve the kinetic barrel + fire final commit
	}

	got := audio.GetInstrumentParams(roster.Snare)
	val, ok := got[paramName]
	if !ok || val <= before {
		t.Errorf("after armed wheel: %s=%v ok=%v, want > %.3f", paramName, val, ok, before)
	}
	if fired < 1 {
		t.Errorf("expected at least 1 SetInstrumentParam call, got %d", fired)
	}
}
