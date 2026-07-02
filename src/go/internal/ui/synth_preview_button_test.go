//go:build test

package ui

import (
	"image"
	"testing"
)

// Synth-tab Preview button: plays a one-shot of the active instrument with
// the CURRENT (unsaved) knob state, routed through the same synthAuditionFn
// hook as the knob-release audition — no parallel preview voice, identical
// trigger plumbing. Lives in the header next to Save / Save As / Reset and
// collapses FIRST into the overflow chevron (audition is the least
// load-bearing header action: knob release already auditions).

func TestSynthHeaderPreviewLaidOutOnWidePanel(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := synthHeaderForTest(t, image.Rect(0, 0, 1200, 200), "kick-1", "drum-kick", false)
	if h.previewRect.Empty() {
		t.Fatal("previewRect empty on a wide desktop header")
	}
	if h.previewRect.Dx() < TextWidth("Preview") {
		t.Errorf("preview width %d < label width %d (would truncate)", h.previewRect.Dx(), TextWidth("Preview"))
	}
	if got, want := h.previewRect.Dy(), Profile().DensityValues().SynthHeaderButtonH; got != want {
		t.Errorf("preview height %d, want SynthHeaderButtonH %d", got, want)
	}
}

func TestSynthHeaderPreviewOnMobileMeetsTouchMin(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	h := synthHeaderForTest(t, image.Rect(0, 0, 900, 200), "kick-1", "drum-kick", true)
	if h.previewRect.Empty() {
		t.Fatal("preview button empty on a wide mobile header (must exist on mobile)")
	}
	if h.previewRect.Dy() < Profile().MinTarget {
		t.Errorf("preview height %d < touch-min %d", h.previewRect.Dy(), Profile().MinTarget)
	}
}

func TestSynthPreviewButtonFiresAuditionOnce(t *testing.T) {
	assertDefaultParityState(t)
	var calls []string
	prev := SwapSynthAuditionFnForTest(func(id string) { calls = append(calls, id) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	g := newSamplerTabGame(t)
	dv := g.drum
	want := dv.synthTabActiveInstrument()
	if want == "" {
		t.Fatal("test game has no active synth instrument")
	}
	dv.layoutSynthHeader(image.Rect(0, 0, 1200, 200), 64, want, "drum-kick", false)
	dv.instEditorBtns = dv.instEditorBtns[:0]
	dv.buildSynthHeaderButtons(want)

	var preview *Button
	for _, b := range dv.instEditorBtns {
		if b != nil && b.Text == synthPreviewButtonTag {
			preview = b
		}
	}
	if preview == nil {
		t.Fatal("no Preview button in instEditorBtns")
	}
	preview.OnClick()
	if len(calls) != 1 || calls[0] != want {
		t.Fatalf("audition calls = %v, want exactly one for %q", calls, want)
	}
}

func TestSynthFooterLabelForPreviewTag(t *testing.T) {
	if got := synthHeaderButtonLabel(synthPreviewButtonTag); got != "Preview" {
		t.Errorf("label for preview tag = %q, want Preview", got)
	}
}
