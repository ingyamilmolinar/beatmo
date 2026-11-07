//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Preview plays a one-shot of the current unsaved state through synthAuditionFn, the same trigger plumbing, and is
// the first header action to collapse into ⋯.

func TestSynthHeaderPreviewLaidOutOnWidePanel(t *testing.T) {
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	h := synthHeaderForTest(t, image.Rect(0, 0, 1200, 200), "kick-1", false)
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

// On mobile Preview lives on the sticky bar.

func TestSynthPreviewButtonFiresAuditionOnce(t *testing.T) {
	assertDefaultParityState(t)
	var calls []string
	prev := SwapSynthAuditionFnForTest(func(id instrument.ID) { calls = append(calls, string(id)) })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(prev) })

	g := newSamplerTabGame(t)
	dv := g.drum
	want := dv.synthTabActiveInstrument()
	if want == "" {
		t.Fatal("test game has no active synth instrument")
	}
	dv.layoutSynthHeader(image.Rect(0, 0, 1200, 200), 64, want, false)
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
	if len(calls) != 1 || instrument.ID(calls[0]) != want {
		t.Fatalf("audition calls = %v, want exactly one for %q", calls, want)
	}
}

func TestSynthFooterLabelForPreviewTag(t *testing.T) {
	if got := synthHeaderButtonLabel(synthPreviewButtonTag); got != "Preview" {
		t.Errorf("label for preview tag = %q, want Preview", got)
	}
}
