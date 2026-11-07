package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// A long label ran under its right-aligned caption. The test font (6 px/glyph) is narrower than the real one,
// so these pin the invariant under geometry that forces the clip; the real overflow needs the screenshot
// scene template_menu_long_titles.

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
	// Keep the front, the part that identifies the row.
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

// With no room the label degrades to nothing rather than overprinting.
func TestClipMenuRowLabel_NoRoomYieldsNoOverprint(t *testing.T) {
	got := clipMenuRowLabel("Beethoven — Symphony No. 5", RoleBody, 300, 300)
	if w := StyledTextWidth(got, RoleBody); w > 0 && got != "" {
		if w > SpaceSM {
			t.Errorf("label %q (%d px) drawn into zero available width", got, w)
		}
	}
}

// Every shipped title at the real menu width fits beside its BPM caption or is clipped to.
func TestTemplateMenuTitlesFitBesideBPM(t *testing.T) {
	// The desktop template menu is ~400 px wide, its BPM caption ~340 px in.
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
