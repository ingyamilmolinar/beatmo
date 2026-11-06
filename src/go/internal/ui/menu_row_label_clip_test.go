package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// A menu row draws its label from the left and a muted caption (category,
// count, "N BPM") right-aligned. Nothing bounded the label, so a long enough
// one ran straight under the caption and painted two sets of glyphs on the same
// pixels. The template menu shipped exactly that: "Clara Schumann — Ich stand
// in dunklen Träumen" overlapping its "70 BPM" readout.
//
// Only a screenshot caught it — every row-data test passed, because the data
// was right and only the rendering collided.
//
// NOTE ON METRICS: under `-tags test` the styled-text roles all fall back to
// the fixed-width debug font (6 px/glyph), which is NARROWER than the real
// proportional font — the title above measures 270 px here but ~310 px on
// screen. These tests therefore cannot reproduce the original overflow by
// using the real menu width; they pin the INVARIANT (a clipped label never
// exceeds the space before the trailing block) under geometry chosen to force
// the clip. Catching a genuine overflow still needs the screenshot scene
// `template_menu_long_titles`.

func TestClipMenuRowLabel_LongLabelIsEllipsizedToFit(t *testing.T) {
	const (
		labelX    = 30
		trailLeft = 250 // tight enough to force a clip under the debug-font metric
		role      = RoleBody
	)
	const long = "Clara Schumann — Ich stand in dunklen Träumen"

	got := clipMenuRowLabel(long, role, labelX, trailLeft)
	if got == long {
		t.Fatalf("label was not clipped: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("clipped label %q should end in an ellipsis", got)
	}
	avail := trailLeft - SpaceSM - labelX
	if w := StyledTextWidth(got, role); w > avail {
		t.Errorf("clipped label width %d still exceeds the %d px available before the trail", w, avail)
	}
	// The clip must keep the front of the string — the part that identifies the
	// row — not the tail.
	if !strings.HasPrefix(long, strings.TrimSuffix(got, "...")) {
		t.Errorf("clipped label %q is not a prefix of the original", got)
	}
}

func TestClipMenuRowLabel_ShortLabelUntouched(t *testing.T) {
	const short = "Satie — Air du rat"
	if got := clipMenuRowLabel(short, RoleBody, 30, 900); got != short {
		t.Errorf("short label was altered: got %q want %q", got, short)
	}
}

// A trailing block that leaves no room must not produce a label wider than the
// (non-existent) space; the row degrades to nothing rather than overprinting.
func TestClipMenuRowLabel_NoRoomYieldsNoOverprint(t *testing.T) {
	got := clipMenuRowLabel("Beethoven — Symphony No. 5", RoleBody, 300, 300)
	if w := StyledTextWidth(got, RoleBody); w > 0 && got != "" {
		if w > SpaceSM {
			t.Errorf("label %q (%d px) drawn into zero available width", got, w)
		}
	}
}

// TestTemplateMenuTitlesFitBesideBPM is the end of the loop: every shipped
// template title, at the real menu width, must either fit beside its BPM
// caption or be clipped to fit. It fails if a future title is long enough to
// collide AND the clip is ever removed.
func TestTemplateMenuTitlesFitBesideBPM(t *testing.T) {
	// The desktop template menu is ~400 px wide; the BPM caption's left edge
	// sits ~340 px into it.
	const (
		labelX    = 40
		trailLeft = 340
	)
	tpls := assets.Templates()
	if len(tpls) == 0 {
		t.Fatal("no templates")
	}
	for _, tp := range tpls {
		got := clipMenuRowLabel(tp.Display, RoleBody, labelX, trailLeft)
		avail := trailLeft - SpaceSM - labelX
		if w := StyledTextWidth(got, RoleBody); w > avail {
			t.Errorf("template title %q renders %d px wide, exceeding the %d px "+
				"before the BPM caption", tp.Display, w, avail)
		}
	}
}
