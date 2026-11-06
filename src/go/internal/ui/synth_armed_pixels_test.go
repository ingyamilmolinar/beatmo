//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthArmedPixelsBothLanesRenderData locks in the just-verified-pristine
// armed two-barrel editor: BOTH the resolution lane (left) and the value
// lane (right) of drawArmedControlCard (synth_panel_zone.go) must paint real
// DATA — not just chrome — when the crop_synth_control_armed scene arms a
// continuous control. A future regression that blanks a lane (or leaves the
// armed card off-window) must fail this fast Go test, not just be caught by
// a human/agent staring at a screenshot.
//
// Interceptors reused as-is, no new hooks invented:
//   - drawTextStyledHook (synth_barrel_render_test.go) — observes every
//     string DrawTextStyled receives, proving the barrel-lane label stack
//     actually drew something.
//   - collectFilledRects / drawnRect / rectsWithColorInside
//     (render_verify_test.go, audio_panel_render_pixels_test.go) — the same
//     "count data-colored filled rects inside a subject rect" discriminator
//     TestAudioPanelTabsRenderNonBlankPixels uses for Wave/Spectrum/Meters/
//     Scope, applied here to the two barrel lanes instead of a tab subject.
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

	// Guards the "armed control off-window → invisible" bug the controller
	// just fixed in the scene: the control's own on-screen rect (what
	// drawSynthDetailPane checks before calling drawArmedControlCard at all,
	// see synth_panel_zone.go's `if r.Empty() { continue }`) must be
	// non-empty BEFORE a single Draw runs.
	if kIdx < 0 || kIdx >= len(dv.instEditorControls) || dv.instEditorControls[kIdx].Rect().Empty() {
		t.Fatalf("armed control kIdx=%d has an empty/out-of-range rect — armed card would render off-window", kIdx)
	}

	origHook := drawTextStyledHook
	var texts []string
	drawTextStyledHook = func(s string) { texts = append(texts, s) }
	t.Cleanup(func() { drawTextStyledHook = origHook })

	// e.ValueLaneRect()/ResLaneRect() are populated as a side effect of
	// drawArmedControlCard's e.SetCardRect(capR) call, which only happens
	// inside Draw — so the lane rects can only be read AFTER this Draw,
	// mirroring how the real screenshot harness (and drawSynthDetailPane
	// itself) observes them.
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

	// The accent hairline drawBarrelLane (synth_barrel_render.go) always
	// paints — "drawn first ... only the label's own tint is conditional" —
	// is the per-lane data discriminator. Its exact color is derived via the
	// SAME formula the renderer uses (WithAlphaFromColor(colConceptStroke,
	// AlphaStrong), matching drawArmedControlCard's barrelLaneStyle.Accent),
	// not a hand-picked magic value, so it is guaranteed to match whatever
	// the render path actually emits. The card shell/bevel/divider colors
	// are all distinct tokens (TokenSurface3, adjustColor, TokenBorderMedium
	// at AlphaSubtle), so this color can only come from a lane's hairline or
	// its center-row emphasis tint — never from chrome.
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
