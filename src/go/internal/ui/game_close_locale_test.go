package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A scene switches the language for its Game's life and never back; without a
// reset, whichever shard came next saw "Deshacer: añadir nodo".
func TestClosingAGameThatStartedInEnglishReturnsToEnglish(t *testing.T) {
	other := New(testLogger) // another live Game must not hold the reset back
	t.Cleanup(other.CloseForTest)
	g := New(testLogger)
	i18n.SetLocale(i18n.LocaleES)
	g.CloseForTest()
	if got := i18n.ActiveLocale(); got != i18n.LocaleEN {
		t.Errorf("after the Game closed the locale is %v, want %v", got, i18n.LocaleEN)
	}
}

// A test that set Spanish before creating its Game and English again itself
// must not get Spanish back when that Game closes in cleanup.
func TestClosingAGameNeverRestoresALocaleTheTestUndid(t *testing.T) {
	i18n.SetLocale(i18n.LocaleES)
	g := New(testLogger)
	i18n.SetLocale(i18n.LocaleEN)
	g.CloseForTest()
	if got := i18n.ActiveLocale(); got != i18n.LocaleEN {
		t.Errorf("closing the Game switched the locale to %v", got)
	}
}
