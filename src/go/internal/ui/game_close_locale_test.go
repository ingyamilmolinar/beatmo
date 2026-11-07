package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A scene (or any test) that switches the language leaves it switched for
// every later test unless closing its Game puts it back: under sharding that
// turned an English undo notification Spanish in whichever shard came next.
func TestClosingAGameRestoresTheLocaleItStartedWith(t *testing.T) {
	i18n.SetLocale(i18n.LocaleEN)
	g := New(testLogger)
	i18n.SetLocale(i18n.LocaleES)
	g.CloseForTest()
	if got := i18n.ActiveLocale(); got != i18n.LocaleEN {
		t.Errorf("after Close the locale is %v, want the %v the Game started with", got, i18n.LocaleEN)
	}
}
