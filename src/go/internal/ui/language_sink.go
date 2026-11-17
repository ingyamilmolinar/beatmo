package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// LanguageSaveSink is the persistence shim the settings menu writes the chosen
// UI language through, and reads it at startup. userprefs.LanguageStore
// satisfies it; tests use a stub. Mirrors KnobStepSaveSink (knob_step_sink.go):
// production bootstraps (cmd/beatmo.go, js_bootstrap_wasm.go) call
// SetLanguageSink after constructing the userprefs store.
type LanguageSaveSink interface {
	LoadLanguage() (string, error)
	SaveLanguage(lang string) error
}

var (
	languageSinkMu sync.RWMutex
	languageSink   LanguageSaveSink
)

// SetLanguageSink registers the global language persistence sink. Nil-safe:
// passing nil keeps language switching working in-session without persistence.
func SetLanguageSink(s LanguageSaveSink) {
	languageSinkMu.Lock()
	languageSink = s
	languageSinkMu.Unlock()
}

func activeLanguageSink() LanguageSaveSink {
	languageSinkMu.RLock()
	defer languageSinkMu.RUnlock()
	return languageSink
}

// ApplyStoredLanguage reads the persisted locale (if any) and applies it before
// the first frame. Empty/absent or no sink ⇒ English (the i18n default).
// Called from the production bootstraps after SetLanguageSink.
func ApplyStoredLanguage() {
	s := activeLanguageSink()
	if s == nil {
		return
	}
	if lang, err := s.LoadLanguage(); err == nil && lang != "" {
		i18n.SetLocale(i18n.ParseLocale(lang))
	}
}

// persistLanguage writes the chosen locale through the sink (used by the
// settings menu). Nil-safe no-op when no sink is registered.
func persistLanguage(loc i18n.Locale) {
	if s := activeLanguageSink(); s != nil {
		_ = s.SaveLanguage(loc.String())
	}
}
