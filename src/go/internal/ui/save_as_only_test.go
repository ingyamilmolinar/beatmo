//go:build test

package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Save As is the only save. Its confirm says what happens: the edited instrument's name saves a new version of it,
// a new name makes a new instrument, and another instrument's name, in any language, is refused.

func menuLabels(items []actionMenuItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func TestNoSaveResetOrRestoreInEitherTab(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	for _, b := range dv.instEditorBtns {
		if l := synthHeaderButtonLabel(b.Text); l == i18n.T(i18n.KeySave) || l == i18n.T(i18n.KeyReset) {
			t.Errorf("the Synth header still has %q", l)
		}
	}
	for name, items := range map[string][]actionMenuItem{
		"synth":   dv.synthActionMenuItems(),
		"sampler": dv.samplerActionMenuItems(),
	} {
		labels := menuLabels(items)
		for _, gone := range []string{i18n.T(i18n.KeyReset), "Restore shipped", i18n.T(i18n.KeySave)} {
			if slices.Contains(labels, gone) {
				t.Errorf("the %s ⋯ menu still offers %q: %v", name, gone, labels)
			}
		}
		if !slices.Contains(labels, i18n.T(i18n.KeySaveAs)) {
			t.Errorf("the %s ⋯ menu lost Save As: %v", name, labels)
		}
	}
	mobile := setupSynthFooterEnv(t, 390, 844, true)
	for _, b := range mobile.g.drum.synthPanelMobileActions() {
		if b.Text == i18n.T(i18n.KeySave) {
			t.Error("the mobile Synth action cluster still has Save")
		}
	}
}

func TestSaveAsConfirmSaysWhatWillHappen(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	withPrivateInstrumentLibrary(t)
	dv.openSaveAsDialog()
	dlg := dv.SynthSaveAsDialog()
	if dlg == nil {
		t.Fatal("Save As did not open its dialog")
	}
	current := dlg.Value()
	if current == "" {
		t.Fatal("the dialog is not prefilled with the instrument's current name")
	}

	cases := []struct {
		typed, wantLabel string
		refused          bool
	}{
		{current, i18n.Tf(i18n.KeySaveOverFmt, current), false},
		{"  " + strings.ToUpper(current) + " ", i18n.Tf(i18n.KeySaveOverFmt, current), false},
		{"Brand New Snare", i18n.T(i18n.KeySaveAsNew), false},
		{"Kick", "", true},  // another shipped instrument
		{"Bombo", "", true}, // the same one, in Spanish
	}
	for _, c := range cases {
		dlg.SetValue(c.typed)
		dv.updateSaveAsDialog()
		if c.refused {
			if dlg.Refused() == "" {
				t.Errorf("%q is another instrument's name but was not refused", c.typed)
			}
			continue
		}
		if dlg.Refused() != "" {
			t.Errorf("%q refused: %s", c.typed, dlg.Refused())
		}
		if got := dlg.ConfirmLabel(); got != c.wantLabel {
			t.Errorf("typed %q: confirm reads %q, want %q", c.typed, got, c.wantLabel)
		}
	}

	dlg.SetValue("Kick")
	dv.updateSaveAsDialog()
	dv.ConfirmSaveAsDialog()
	if dv.SynthSaveAsDialog() == nil {
		t.Fatal("a refused name was confirmed (the dialog closed)")
	}
}

func TestSaveOverAppendsAVersionWithTheBundle(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	withPrivateInstrumentLibrary(t)
	t.Cleanup(func() { audio.ApplyAudioBundle(roster.Snare, audio.AudioBundle{}) })

	saveOver := func(decay float64) {
		t.Helper()
		audio.SetInstrumentParam(env.instID, "post_decay", decay)
		dv.openSaveAsDialog()
		dv.ConfirmSaveAsDialog() // the prefilled name: save over
	}
	audio.EQSettingsOf(roster.Snare).GainsDB[4] = 3
	saveOver(0.6)
	saveOver(0.4)

	if n := len(audio.LibraryVersions(roster.Snare)); n != 2 {
		t.Fatalf("two saves over Snare kept %d versions, want both", n)
	}
	if v := libraryField(t, string(roster.Snare), "post_decay"); v != 0.4 {
		t.Fatalf("the library plays post_decay %v, want the newest save's 0.4", v)
	}
	if b, ok := audio.LibraryBundle(roster.Snare); !ok || b.EQ == nil || b.EQ.GainsDB[4] != 3 {
		t.Fatalf("the saved version lacks the instrument's EQ: %+v", b)
	}
	if dv.Rows[0].Instrument != roster.Snare {
		t.Fatalf("saving over moved the row to %s", dv.Rows[0].Instrument)
	}
}

func TestSaveAsNewCarriesTheBundle(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	withPrivateInstrumentLibrary(t)
	t.Cleanup(func() { audio.ApplyAudioBundle(roster.Snare, audio.AudioBundle{}) })
	audio.EQSettingsOf(roster.Snare).GainsDB[2] = -5
	dv.openSaveAsDialog()
	dv.SynthSaveAsDialog().SetValue("Gritty Snare")
	dv.updateSaveAsDialog()
	dv.ConfirmSaveAsDialog()
	doc, _ := audio.InstrumentDoc(roster.Snare)
	if doc.Origin == "" || doc.Origin == roster.Snare {
		t.Fatalf("Save as new did not make a new instrument (origin %q)", doc.Origin)
	}
	if b, ok := audio.LibraryBundle(doc.Origin); !ok || b.EQ == nil || b.EQ.GainsDB[2] != -5 {
		t.Fatalf("the new instrument's first version lacks the EQ: %+v", b)
	}
}

func TestSaveAsFailureKeepsLibraryAndNotifies(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	dv := env.g.drum
	lib := withPrivateInstrumentLibrary(t)
	audio.SetInstrumentParam(env.instID, "post_decay", 0.7)
	dv.openSaveAsDialog()
	dv.ConfirmSaveAsDialog()
	before := len(audio.LibraryVersions(roster.Snare))

	lib.fail = errors.New("quota exceeded")
	audio.SetInstrumentParam(env.instID, "post_decay", 0.3)
	dv.openSaveAsDialog()
	dv.ConfirmSaveAsDialog()
	if n := len(audio.LibraryVersions(roster.Snare)); n != before {
		t.Fatalf("a save that could not be stored changed the library (%d → %d versions)", before, n)
	}
	if v := libraryField(t, string(roster.Snare), "post_decay"); v != 0.7 {
		t.Fatalf("the library plays %v after a failed save, want the stored 0.7", v)
	}
	found := false
	for _, n := range dv.notifStore.History() {
		if n.isErr && n.key == i18n.KeySaveFailed {
			found = true
		}
	}
	if !found {
		t.Fatal("the user was not told the save failed")
	}
}

// A copy whose document names no origin saves over its instance base, the instrument the circuit-open rule
// resolves, never a library entry for the instance id.
func TestSaveOverAnInstanceSavesItsLibraryInstrument(t *testing.T) {
	withPrivateInstrumentLibrary(t)
	copyID, err := instrument.Instance(roster.Kick, 2)
	if err != nil {
		t.Fatal(err)
	}
	audio.EnsureInstanceInstrument(copyID)
	d, _ := audio.InstrumentDoc(copyID)
	d.Origin = ""
	if err := audio.SetInstrumentDoc(copyID, d); err != nil {
		t.Fatal(err)
	}
	if got := saveRecipeForInstrument(copyID); got != roster.Kick {
		t.Fatalf("saving over %s stored under %q, want its library instrument %q", copyID, got, roster.Kick)
	}
	if n := len(audio.LibraryVersions(copyID)); n != 0 {
		t.Fatalf("the instance got a library history of its own (%d versions)", n)
	}
}
