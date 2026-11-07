package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

type LanguageSaveSink interface {
	LoadLanguage() (string, error)
	SaveLanguage(lang string) error
}

var (
	languageSinkMu sync.RWMutex
	languageSink   LanguageSaveSink
)

// Nil keeps in-session switching working without persistence.
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

// Before the first frame; nothing stored means English.
func ApplyStoredLanguage() {
	s := activeLanguageSink()
	if s == nil {
		return
	}
	if lang, err := s.LoadLanguage(); err == nil && lang != "" {
		i18n.SetLocale(i18n.ParseLocale(lang))
	}
}

func persistLanguage(loc i18n.Locale) {
	if s := activeLanguageSink(); s != nil {
		_ = s.SaveLanguage(loc.String())
	}
}
