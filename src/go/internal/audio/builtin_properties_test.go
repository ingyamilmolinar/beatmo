package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestBuiltinPropertiesCoverTheRoster: every shipped instrument has its
// category and kit role written down, so none of them is classified by the
// spelling of its id.
func TestBuiltinPropertiesCoverTheRoster(t *testing.T) {
	for _, id := range roster.All {
		if _, ok := builtinProperties[id]; !ok {
			t.Errorf("%s has no row in builtinProperties", id)
		}
	}
	if len(builtinProperties) != len(roster.All) {
		t.Errorf("builtinProperties has %d rows for %d shipped instruments", len(builtinProperties), len(roster.All))
	}
}

// TestBuiltinPropertiesAreWhatCategoryOfAndRoleReport: CategoryOf and
// RoleForInstrument answer a built-in (and an instance of one) from the
// table, not from its name.
func TestBuiltinPropertiesAreWhatCategoryOfAndRoleReport(t *testing.T) {
	for id, p := range builtinProperties {
		if got := CategoryOf(id); got != p.Category {
			t.Errorf("CategoryOf(%s) = %s, want %s", id, got, p.Category)
		}
		if got := RoleForInstrument(id); got != p.Role {
			t.Errorf("RoleForInstrument(%s) = %q, want %q", id, got, p.Role)
		}
		inst := id + "--2"
		if got := CategoryOf(inst); got != p.Category {
			t.Errorf("CategoryOf(%s) = %s, want its base's %s", inst, got, p.Category)
		}
		if got := RoleForInstrument(inst); got != p.Role {
			t.Errorf("RoleForInstrument(%s) = %q, want its base's %q", inst, got, p.Role)
		}
	}
}

// TestNamedInstrumentsAreClassifiedByName: an instrument the user named has
// no row, so its name is what classifies it.
func TestNamedInstrumentsAreClassifiedByName(t *testing.T) {
	cases := []struct {
		id   instrument.ID
		cat  Category
		role instrument.Role
	}{
		{"user.open-hihat.a1b2c3", CatHiHat, instrument.RoleHat},
		{"user.my-kick.a1b2c3", CatKick, instrument.RoleKick},
		{"user.tom-low.a1b2c3", CatTom, instrument.RoleTom},
		{"user.choir-bass.a1b2c3", CatVoice, instrument.RoleBass},
		{"user.sample.0000beef", CatOther, instrument.RoleNone},
	}
	for _, c := range cases {
		if got := CategoryOf(c.id); got != c.cat {
			t.Errorf("CategoryOf(%s) = %s, want %s", c.id, got, c.cat)
		}
		if got := RoleForInstrument(c.id); got != c.role {
			t.Errorf("RoleForInstrument(%s) = %q, want %q", c.id, got, c.role)
		}
	}
	// A WAV sample's id is a hash: its display name is what classifies it.
	wav := instrument.UserSampleID(0xfeed)
	SetInstrumentDisplayName(wav, "My Kick")
	t.Cleanup(func() { ClearInstrumentDisplayName(wav) })
	if got := CategoryOf(wav); got != CatKick {
		t.Errorf("CategoryOf(a WAV named My Kick) = %s, want kick", got)
	}
	if got := RoleForInstrument(wav); got != instrument.RoleKick {
		t.Errorf("RoleForInstrument(a WAV named My Kick) = %s, want kick", got)
	}
	if CategoryOf("") != CatOther || RoleForInstrument("") != instrument.RoleNone {
		t.Error(`the empty id has no category or role`)
	}
}
