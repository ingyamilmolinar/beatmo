package i18n

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInstrumentDisplayNameLocalizesDefault(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	if got := InstrumentDisplayName(roster.Kick, "Kick"); got != "Bombo" {
		t.Fatalf("kick default ES = %q want Bombo", got)
	}
	if got := InstrumentDisplayName(roster.Kick, "My Boom"); got != "My Boom" {
		t.Fatalf("renamed = %q want My Boom", got)
	}
	if got := InstrumentDisplayName(roster.Kick, ""); got != "Bombo" {
		t.Fatalf("empty = %q want Bombo", got)
	}
	if got := InstrumentDisplayName("unknownid", "Weird"); got != "Weird" {
		t.Fatalf("unknown = %q want Weird", got)
	}
}
