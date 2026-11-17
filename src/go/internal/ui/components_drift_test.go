package ui

import (
	"image/color"
	"testing"
)

// TestComponentSpecsDrift asserts that each generated ComponentSpec's
// resolved fill+border matches the corresponding hand-coded ButtonStyle /
// TextInputStyle byte-for-byte. This is the byte-equivalence guard from
// the Phase 2 plan — it catches generator bugs that the freshness test
// can't (where committed and regenerated agree but both are wrong against
// the runtime reality).
//
// Coverage now spans every named ButtonStyle/TextInputStyle whose value
// is fully expressible in the schema. Active variants and runtime-alpha
// composites are included via the optional-alpha mechanism (omitted
// border.alpha → opaque 255).
//
// Comparison strategy: use the color.Color interface RGBA() method so
// RGBA{R,G,B,255} and NRGBA{R,G,B,255} compare equal — the runtime can
// hold either concrete type interchangeably and renders identically.
func TestComponentSpecsDrift(t *testing.T) {
	cases := []struct {
		name     string
		id       ComponentID
		wantFill color.Color
		wantBdr  color.Color
	}{
		// ── button-secondary family (8 hand-coded vars share this recipe) ──
		{"button-secondary ↔ TransportPlayStyle", ComponentButtonSecondary, colSurface2, colBorderSubtle},
		{"button-row-control ↔ InstButtonStyle", ComponentButtonRowControl, colSurface2, colBorderSubtle},

		// ── input-field family ──
		{"input-field ↔ BPMBoxStyle / EQDBBoxStyle", ComponentInputField, colSurface2, colBorderMedium},
		{"input-field-focused ↔ TextInputStyle focused border", ComponentInputFieldFocused, colSurface2, colBorderStrong},

		// ── desktop play / stop ──
		{"button-play-desktop ↔ PlayButtonStyle", ComponentButtonPlayDesktop, colPlayButton, color.RGBA{63, 214, 122, 255}},
		{"button-stop-desktop ↔ StopButtonStyle", ComponentButtonStopDesktop, colStopButton, color.RGBA{255, 92, 42, 255}},

		// ── numeric steppers ── (now neutral, same recipe as button-secondary)
		{"button-stepper ↔ BPMIncStyle / BPMDecStyle / LenIncStyle / LenDecStyle", ComponentButtonStepper, colSurface2, colBorderSubtle},

		// ── dropdown ──
		{"button-dropdown ↔ DropdownStyle", ComponentButtonDropdown, colSurface2, colDropdownEdge},

		// ── disabled ──
		{"button-disabled ↔ DisabledButtonStyle", ComponentButtonDisabled, color.RGBA{40, 26, 64, 255}, colBorderSubtle},

		// ── destructive (delete + confirm) ──
		{"button-destructive ↔ DeleteButtonStyle", ComponentButtonDestructive, colDeleteFill, colDeleteBorder},
		{"button-destructive-confirm ↔ DeleteConfirmButtonStyle", ComponentButtonDestructiveConfirm, color.RGBA{232, 74, 31, 255}, color.RGBA{255, 92, 42, 255}},

		// ── missing instrument ──
		{"button-missing-instrument ↔ MissingInstStyle", ComponentButtonMissingInstrument, colError, colBorderMedium},

		// ── row-control active variants ──
		{"button-row-control-mute-active ↔ MuteActiveStyle", ComponentButtonRowControlMuteActive, colMuteActive, colMuteActiveBdr},
		{"button-row-control-solo-active ↔ SoloActiveStyle", ComponentButtonRowControlSoloActive, colSoloActive, colSoloActiveBdr},
		{"button-row-control-fx-active ↔ FXActiveStyle", ComponentButtonRowControlFxActive, color.RGBA{40, 26, 64, 255}, colAccent},

		// ── EQ band-mute chips ──
		{"button-eq-mute ↔ EQMuteButtonStyle", ComponentButtonEqMute, colSurface3, colBorderMedium},
		{"button-eq-mute-active ↔ EQMuteButtonActiveStyle", ComponentButtonEqMuteActive, color.RGBA{232, 74, 31, 255}, colBorderMedium},

		// ── EQ filter active ──
		{"button-eq-filter-active ↔ EQFilterButtonActiveStyle", ComponentButtonEqFilterActive, color.RGBA{255, 158, 31, 255}, color.RGBA{255, 179, 10, 255}},

		// ── transport follow-on ──
		{"button-transport-follow-on ↔ TransportFollowOnStyle", ComponentButtonTransportFollowOn, colSurface3, colBorderSubtle},

		// ── primary FAB ──
		{"button-primary ↔ FABStyle", ComponentButtonPrimary, colAccent, color.NRGBA{255, 255, 255, 30}},

		// ── mobile row label ──
		{"button-row-label-mobile ↔ MobileRowLabelStyle", ComponentButtonRowLabelMobile, colSurface1, colBorderSubtle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := Spec(tc.id)
			if !sameColor(spec.Fill, tc.wantFill) {
				t.Errorf("Fill: got %+v, want %+v", spec.Fill, tc.wantFill)
			}
			gotBorder := spec.Border.Resolve()
			if !sameColor(gotBorder, tc.wantBdr) {
				t.Errorf("Border: got %+v, want %+v", gotBorder, tc.wantBdr)
			}
		})
	}
}

// sameColor compares two color.Color values via the interface RGBA()
// method, which normalizes RGBA / NRGBA / etc. to identical premultiplied
// uint32 components. This sidesteps concrete-type mismatch (the runtime
// holds Border as color.Color and accepts either flavor).
func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, bA := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == bA
}

// TestComponentSpecMatchesHandCodedSibling spot-checks that every
// hand-coded ButtonStyle var that resolves to button-secondary's recipe
// produces the same Fill/Border bytes as the spec. If a future edit
// breaks this drift, both the hand-coded var and the spec must move in
// lockstep — typically by changing only DESIGN.md and regenerating.
func TestComponentSpecMatchesHandCodedSibling(t *testing.T) {
	spec := Spec(ComponentButtonSecondary)
	specBorder := spec.Border.Resolve()

	siblings := map[string]ButtonStyle{
		"TransportPlayStyle": TransportPlayStyle,
		"TransportStopStyle": TransportStopStyle,
		"TransportIncStyle":  TransportIncStyle,
		"TransportDecStyle":  TransportDecStyle,
		"TransportMiscStyle": TransportMiscStyle,
		"InstButtonStyle":    InstButtonStyle,
		"UploadBtnStyle":     UploadBtnStyle,
		"PopupButtonStyle":   PopupButtonStyle,
	}
	for name, sib := range siblings {
		if !sameColor(sib.Fill, spec.Fill) {
			t.Errorf("%s.Fill = %+v, want spec %+v", name, sib.Fill, spec.Fill)
		}
		if !sameColor(sib.Border, specBorder) {
			t.Errorf("%s.Border = %+v, want spec %+v", name, sib.Border, specBorder)
		}
	}

	// Stepper family: 4 hand-coded vars share button-stepper.
	stepperSpec := Spec(ComponentButtonStepper)
	stepperBorder := stepperSpec.Border.Resolve()
	steppers := map[string]ButtonStyle{
		"BPMIncStyle": BPMIncStyle,
		"BPMDecStyle": BPMDecStyle,
		"LenIncStyle": LenIncStyle,
		"LenDecStyle": LenDecStyle,
	}
	for name, st := range steppers {
		if !sameColor(st.Fill, stepperSpec.Fill) {
			t.Errorf("%s.Fill = %+v, want spec %+v", name, st.Fill, stepperSpec.Fill)
		}
		if !sameColor(st.Border, stepperBorder) {
			t.Errorf("%s.Border = %+v, want spec %+v", name, st.Border, stepperBorder)
		}
	}
}
