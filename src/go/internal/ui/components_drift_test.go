package ui

import (
	"image/color"
	"testing"
)

// Each generated spec's fill and border match the hand-coded style byte for byte, catching a generator that is
// consistently wrong. Compared through RGBA(), so RGBA and NRGBA of the same colour are equal.
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

// The runtime may hold either flavour.
func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, bA := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == bA
}

// Changing one means changing DESIGN.md and regenerating, so both move together.
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
