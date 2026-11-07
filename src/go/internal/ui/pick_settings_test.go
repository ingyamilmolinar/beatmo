package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Choosing an instrument never drops or silently changes settings: a bare row takes the instrument's, a row's
// settings carry to an instrument with none, and when both differ, or keeping the row's would change another
// row, the user is asked; cancel changes nothing.

func eqBundle(band int, db float64) audio.AudioBundle {
	s := audio.NewEQSettings()
	s.GainsDB[band] = db
	return audio.AudioBundle{EQ: s}
}

// Every touched instrument starts and ends with no settings.
func pickGame(t *testing.T, touched ...instrument.ID) *Game {
	t.Helper()
	withPrivateInstrumentLibrary(t)
	g := newMelodicCircuitGame(t, 0)
	for _, id := range append(touched, testMelodicInst, roster.Kick) {
		audio.ApplyAudioBundle(id, audio.AudioBundle{})
		t.Cleanup(func() { audio.ApplyAudioBundle(id, audio.AudioBundle{}) })
	}
	return g
}

func saveLibraryBundle(t *testing.T, id instrument.ID, b audio.AudioBundle) {
	t.Helper()
	d, _ := audio.ShippedDoc(id)
	if err := audio.SaveInstrumentVersion(id, d, b); err != nil {
		t.Fatal(err)
	}
}

func sameBundle(a, b audio.AudioBundle) bool { return a.Digest() == b.Digest() }

// answer picks the question's item whose label contains want.
func answer(t *testing.T, g *Game, want string) {
	t.Helper()
	m := g.drum.actionMenu
	if m == nil || !m.IsOpen() {
		t.Fatal("no question is open")
	}
	for _, b := range m.Buttons() {
		if strings.Contains(b.Text, want) && b.OnClick != nil {
			b.OnClick()
			return
		}
	}
	t.Fatalf("the question has no %q choice", want)
}

func questionOpen(g *Game) bool { return g.drum.actionMenu != nil && g.drum.actionMenu.IsOpen() }

func TestPickRowWithoutSettingsTakesTheInstruments(t *testing.T) {
	g := pickGame(t, roster.Snare)
	lib := eqBundle(3, 4)
	saveLibraryBundle(t, roster.Snare, lib)
	g.drum.pickInstrument(1, roster.Snare)
	if questionOpen(g) {
		t.Fatal("a row with nothing to lose was asked")
	}
	if g.drum.Rows[1].Instrument != roster.Snare {
		t.Fatalf("row 1 plays %s, want the pick", g.drum.Rows[1].Instrument)
	}
	if got := audio.CurrentAudioBundle(roster.Snare); !sameBundle(got, lib) {
		t.Fatalf("the picked instrument did not bring its saved settings: %+v", got.EQ)
	}
}

func TestPickCarriesRowSettingsToABareInstrument(t *testing.T) {
	g := pickGame(t, roster.Snare)
	row := eqBundle(2, -5)
	audio.ApplyAudioBundle(roster.Kick, row)
	g.drum.pickInstrument(1, roster.Snare)
	if questionOpen(g) {
		t.Fatal("asked although the picked instrument has no settings")
	}
	if got := audio.CurrentAudioBundle(roster.Snare); !sameBundle(got, row) {
		t.Fatalf("the row's settings did not carry over: %+v", got.EQ)
	}
}

func TestPickConflictAsksThenKeepsOrUses(t *testing.T) {
	for _, keep := range []bool{true, false} {
		g := pickGame(t, roster.Snare)
		row, lib := eqBundle(2, -5), eqBundle(7, 6)
		audio.ApplyAudioBundle(roster.Kick, row)
		saveLibraryBundle(t, roster.Snare, lib)
		g.drum.pickInstrument(1, roster.Snare)
		if !questionOpen(g) {
			t.Fatal("both have different settings, yet nobody asked")
		}
		if g.drum.Rows[1].Instrument != roster.Kick {
			t.Fatal("the row switched before the question was answered")
		}
		want := lib
		if keep {
			answer(t, g, i18n.T(i18n.KeyKeepRowSettings))
			want = row
		} else {
			answer(t, g, i18n.Tf(i18n.KeyUseInstrumentSettingsFmt, audio.InstrumentDisplayName(roster.Snare)))
		}
		if g.drum.Rows[1].Instrument != roster.Snare {
			t.Fatalf("keep=%v: the row did not switch after the answer", keep)
		}
		if got := audio.CurrentAudioBundle(roster.Snare); !sameBundle(got, want) {
			t.Fatalf("keep=%v: settings %+v, want %+v", keep, got.EQ, want.EQ)
		}
	}
}

func TestPickConflictCancelChangesNothing(t *testing.T) {
	g := pickGame(t, roster.Snare)
	row, lib := eqBundle(2, -5), eqBundle(7, 6)
	audio.ApplyAudioBundle(roster.Kick, row)
	saveLibraryBundle(t, roster.Snare, lib)
	g.drum.pickInstrument(1, roster.Snare)
	if !questionOpen(g) {
		t.Fatal("no question")
	}
	g.drum.portal().CloseTop()
	if g.drum.Rows[1].Instrument != roster.Kick {
		t.Fatal("a cancelled question still switched the row")
	}
	if got := audio.CurrentAudioBundle(roster.Kick); !sameBundle(got, row) {
		t.Fatal("a cancelled question changed the row's settings")
	}
}

func TestPickSaysWhenKeepingChangesAnotherRow(t *testing.T) {
	g := pickGame(t)
	row, shared := eqBundle(2, -5), eqBundle(7, 6)
	audio.ApplyAudioBundle(testMelodicInst, row)
	audio.ApplyAudioBundle(roster.Kick, shared) // row 1 plays kick with its own settings
	g.drum.pickInstrument(0, roster.Kick)
	if !questionOpen(g) {
		t.Fatal("keeping the row's settings would change row 2, yet nobody asked")
	}
	answer(t, g, i18n.Tf(i18n.KeyAlsoChangesRowFmt, "2"))
	if got := audio.CurrentAudioBundle(roster.Kick); !sameBundle(got, row) {
		t.Fatalf("keep did not apply the row's settings: %+v", got.EQ)
	}
}

func TestKitSwapAsksOnceAndNeverDrops(t *testing.T) {
	g := pickGame(t, roster.KickDeep)
	row, lib := eqBundle(1, 3), eqBundle(8, -4)
	audio.ApplyAudioBundle(roster.Kick, row)
	saveLibraryBundle(t, roster.KickDeep, lib)
	kit := audio.Kit{ID: "kit.pick-test", DisplayName: "Pick Test",
		Members: map[instrument.Role]instrument.ID{instrument.RoleKick: roster.KickDeep}}
	g.drum.ApplyKit(kit)
	if !questionOpen(g) {
		t.Fatal("a kit swap that would replace the row's settings did not ask")
	}
	if g.drum.Rows[1].Instrument != roster.Kick {
		t.Fatal("the kit swapped before the question was answered")
	}
	answer(t, g, i18n.T(i18n.KeyKeepEachRowSettings))
	if g.drum.Rows[1].Instrument != roster.KickDeep {
		t.Fatalf("after the answer the kick row plays %s", g.drum.Rows[1].Instrument)
	}
	if got := audio.CurrentAudioBundle(roster.KickDeep); !sameBundle(got, row) {
		t.Fatalf("keep did not carry the row's settings to the kit's kick: %+v", got.EQ)
	}
}

// The menu previews by picking; a cancelled pick must not leave it believing the instrument applied, or the next
// pick of it would only close the menu.
func TestCancelledPickCanBePickedAgain(t *testing.T) {
	g := pickGame(t, roster.Snare)
	audio.ApplyAudioBundle(roster.Kick, eqBundle(2, -5))
	saveLibraryBundle(t, roster.Snare, eqBundle(7, 6))
	g.drum.selRow = 1
	g.drum.openInstMenuForRow(1)
	m := g.drum.instMenuComp
	if m == nil || !m.IsOpen() {
		t.Fatal("the instrument menu did not open")
	}
	m.applyInstrument(roster.Snare)
	if !questionOpen(g) {
		t.Fatal("no question")
	}
	g.drum.portal().CloseTop() // cancel the question
	if m.ActiveInstrumentID() != roster.Kick {
		t.Fatalf("after a cancelled pick the menu highlights %s, want the row's kick", m.ActiveInstrumentID())
	}
	m.applyInstrument(roster.Snare)
	if !questionOpen(g) {
		t.Fatal("picking the instrument again after a cancel did not ask again")
	}
}

// One instrument has one channel and one set of settings: the question says whose it keeps and which row changes.
func TestKitSwapOntoOneInstrumentNamesWhoseSettingsWin(t *testing.T) {
	g := pickGame(t, roster.KickDeep, roster.Snare)
	first, second := eqBundle(1, 3), eqBundle(6, -2)
	g.drum.selRow = 0
	g.drum.SetInstrument(roster.Snare)
	g.drum.Rows[0].Role = instrument.RoleKick
	g.drum.Rows[1].Role = instrument.RoleKick
	audio.ApplyAudioBundle(roster.Snare, first)
	audio.ApplyAudioBundle(roster.Kick, second)
	kit := audio.Kit{ID: "kit.pick-test-2", DisplayName: "Pick Test 2",
		Members: map[instrument.Role]instrument.ID{instrument.RoleKick: roster.KickDeep}}
	g.drum.ApplyKit(kit)
	if !questionOpen(g) {
		t.Fatal("two rows with different settings moved onto one instrument without a question")
	}
	answer(t, g, i18n.Tf(i18n.KeyKeepRowNSettingsFmt, "1", "2"))
	if got := audio.CurrentAudioBundle(roster.KickDeep); !sameBundle(got, first) {
		t.Fatalf("the kept settings are not row 1's: %+v", got.EQ)
	}
}
