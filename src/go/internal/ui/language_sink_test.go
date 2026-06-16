package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

type fakeLangSink struct {
	lang  string
	saved string
}

func (f *fakeLangSink) LoadLanguage() (string, error) { return f.lang, nil }
func (f *fakeLangSink) SaveLanguage(l string) error   { f.saved = l; return nil }

func TestApplyStoredLanguageSetsLocale(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	defer SetLanguageSink(nil)
	SetLanguageSink(&fakeLangSink{lang: "es"})
	ApplyStoredLanguage()
	if i18n.ActiveLocale() != i18n.LocaleES {
		t.Fatalf("active=%q want es", i18n.ActiveLocale())
	}
}

func TestApplyStoredLanguageEmptyKeepsEnglish(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	defer SetLanguageSink(nil)
	i18n.SetLocale(i18n.LocaleEN)
	SetLanguageSink(&fakeLangSink{lang: ""})
	ApplyStoredLanguage()
	if i18n.ActiveLocale() != i18n.LocaleEN {
		t.Fatalf("active=%q want en (empty stored ⇒ default)", i18n.ActiveLocale())
	}
}

func TestApplyStoredLanguageNilSinkNoPanic(t *testing.T) {
	defer SetLanguageSink(nil)
	SetLanguageSink(nil)
	ApplyStoredLanguage() // must not panic
}

func TestPersistLanguageWritesThroughSink(t *testing.T) {
	defer SetLanguageSink(nil)
	fs := &fakeLangSink{}
	SetLanguageSink(fs)
	persistLanguage(i18n.LocaleES)
	if fs.saved != "es" {
		t.Fatalf("persisted=%q want es", fs.saved)
	}
}
