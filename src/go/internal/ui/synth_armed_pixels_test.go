//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Both barrel lanes paint data, not just chrome, in the crop_synth_control_armed scene, counted with the existing
// text hook and filled-rect interceptor, so a blank lane fails here before a screenshot.
func TestSynthArmedPixelsBothLanesRenderData(t *testing.T) {
	g := driveScene(t, "crop_synth_control_armed")
	dv := g.drum
	if dv == nil {
		t.Fatal("game.drum is nil")
	}

	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	kIdx, armed := dv.synthControlArmed(inst)
	if !armed {
		t.Fatalf("crop_synth_control_armed did not leave a control armed for instrument %q", inst)
	}
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("dv.instEditorArmedEditor is nil after crop_synth_control_armed")
	}

	// drawSynthDetailPane skips an empty rect, so an off-window armed control would be invisible.
	if kIdx < 0 || kIdx >= len(dv.instEditorControls) || dv.instEditorControls[kIdx].Rect().Empty() {
		t.Fatalf("armed control kIdx=%d has an empty/out-of-range rect — armed card would render off-window", kIdx)
	}

	origHook := drawTextStyledHook
	var texts []string
	drawTextStyledHook = func(s string) { texts = append(texts, s) }
	t.Cleanup(func() { drawTextStyledHook = origHook })

	// The lane rects are set inside Draw, so read them after it.
	screen := ebiten.NewImage(1280, 720)
	rects := collectFilledRects(t, func() {
		g.Draw(screen)
	})

	valLane := e.ValueLaneRect()
	resLane := e.ResLaneRect()
	if valLane.Empty() {
		t.Fatal("ValueLaneRect is empty after Draw — value lane would render nothing")
	}
	if resLane.Empty() {
		t.Fatal("ResLaneRect is empty after Draw — scene must arm a CONTINUOUS control (step badge) so both lanes exist")
	}

	if len(texts) < 2 {
		t.Fatalf("expected barrel-lane labels drawn across both lanes, got %d: %v", len(texts), texts)
	}

	// The lane hairline always paints in exactly this colour, computed by the renderer's own formula; no chrome token
	// shares it.
	accent := color.RGBAModel.Convert(WithAlphaFromColor(colConceptStroke, AlphaStrong)).(color.RGBA)

	valueData := rectsWithColorInside(rects, valLane, accent)
	resData := rectsWithColorInside(rects, resLane, accent)

	if valueData == 0 {
		t.Errorf("value lane %v: 0 accent-colored data rects — the value lane rendered no content (blank-lane regression)", valLane)
	}
	if resData == 0 {
		t.Errorf("resolution lane %v: 0 accent-colored data rects — the resolution lane rendered no content (blank-lane regression)", resLane)
	}
	if testing.Verbose() {
		t.Logf("armed card: valueLane=%v dataRects=%d resLane=%v dataRects=%d totalTexts=%d",
			valLane, valueData, resLane, resData, len(texts))
	}
}
